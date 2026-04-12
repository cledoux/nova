# Nova architecture

## Overview

Nova is a Go binary that bridges Discord and Claude Code CLI subprocesses. Each agent runs as a child process; Nova manages their lifecycles, routes Discord messages to their stdin, intercepts JSON directives from their stdout, and posts non-directive output back to Discord.

```
Discord
  │  (user message in channel)
  ▼
nova/nova/nova.go  ← RegisterMessageRouter
  │
  ▼
session.Manager.ByChannel  →  Session.Send(msg)
                                    │
                                    ▼
                              writeLoop  →  claude stdin
                                                │
                              readLoop  ←  claude stdout
                                │
                    ┌───────────┴────────────────────┐
                    │ directive line                  │ content line
                    ▼                                 ▼
            handleDirective                  OnContent → Discord channel
          (spawn / send / create_channel)
```

## Package structure

```
nova/
├── main.go                    — entry point: config, DB, Discord, bot.Run()
├── config/                    — TOML config loader
├── db/                        — SQLite store (sessions, swarms, messages)
├── directive/                 — JSON directive parser
├── discord/                   — Discord helper functions + Client interface
├── session/                   — Session and Manager (hot/cold lifecycle)
├── swarm/                     — Swarm Manager (create, dissolve, broadcast)
├── bot/                       — startup sequence, message router
│   └── commands/              — /nova slash command handlers
└── internal/
    └── testdiscord/           — fake discord.Client for tests
```

## Session lifecycle

```
Spawn()
  │
  ├─ create workspace dir
  ├─ create Discord channel
  ├─ insert DB record (status=cold)
  ├─ Session.Warm()  →  status=hot
  │    ├─ exec claude --resume <id> [--system-prompt-file ...]
  │    ├─ start readLoop goroutine
  │    └─ start writeLoop goroutine (idle timer)
  └─ (optional) Send(initialTask)

[idle timeout fires]
  writeLoop: coolIfGen(gen)  →  status=cold, subprocess killed
             OnIdle(id)      →  DB status=cold

[message arrives on cold session]
  WarmIfCold()  →  Session.Warm()  →  status=hot (subprocess restarts with --resume)

Kill()
  Session.Terminate()  →  status=terminated, subprocess killed
  ArchiveChannel()     →  channel renamed, moved to archive category, made read-only
```

### Generation counter

`Session.gen` is an `int64` incremented on every `Warm()` call. Background goroutines capture their generation `gen` at launch. Before transitioning the session to cold, they call `coolIfGen(gen)`, which is a no-op if `s.gen != gen`. This prevents a goroutine from a previous run from cooling a freshly re-warmed session.

## Directive protocol

Claude agents emit one JSON object per stdout line. Nova intercepts lines that start with `{`:

| Directive | Effect |
|-----------|--------|
| `{"type":"spawn","name":"<n>","task":"<msg>"}` | Spawn a new session in the same swarm |
| `{"type":"send","to":"<name>","message":"<msg>"}` | Send a message to a named session |
| `{"type":"create_channel","name":"<n>"}` | Create a Discord channel (no session) |
| `{"type":"done"}` | Flush accumulated content to Discord |

Non-directive lines are buffered and flushed to Discord when `done` is received or when stdout closes.

## Persistence

SQLite database at `data/nova.db` (relative to the working directory on startup).

**sessions** table: `id`, `name`, `claude_sid`, `workspace`, `channel_id`, `swarm_id`, `status`, `created_at`, `last_active`

**swarms** table: `id`, `name`, `category_id`, `orch_id`, `created_at`

**messages** table: `id`, `session_id`, `role`, `content`, `ts` — used by `/nova status` to report message count.

On startup, `ResetActiveSessions()` sets all `hot` rows to `cold`, since in-memory subprocess state was lost when the process exited.

## Discord layout

```
Server
├── #nova                  (control channel — created on startup if missing)
├── Nova: solo             (category for sessions with no swarm)
├── Nova: archived         (category for terminated sessions)
├── Nova: <swarm-name>     (one category per swarm, created by /nova swarm create)
│   ├── #amber-atlas       (session channel)
│   └── #bold-crane        (session channel)
└── ...
```

## Claude CLI integration

Nova calls the Claude Code CLI as a subprocess:

```sh
claude --resume <session-uuid> --system-prompt-file ~/.nova/system-prompt.txt
```

On first spawn, `--resume <uuid>` both sets and pre-assigns the session ID (Claude creates the session directory with that UUID on first run). Subsequent `Warm()` calls on cold sessions use the same `--resume <uuid>` to reconnect to the persisted conversation.

Input/output is plain text over stdio (one message per line). The system prompt instructs Claude to end every response with `{"type":"done"}` on its own line so Nova knows when a turn is complete.

## Concurrency notes

- `session.Manager` protects its maps with a `sync.RWMutex`.
- Each `Session` has its own `sync.Mutex` for subprocess state.
- `Session.msgCh` is a buffered channel (capacity 8); `Send()` returns an error if it is full rather than blocking.
- `readLoop` and `writeLoop` goroutines hold no locks while doing I/O; they acquire the session mutex only to read/write pointer fields.
