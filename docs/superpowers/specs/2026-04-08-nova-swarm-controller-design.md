# Nova: Discord-Native Claude Swarm Controller — Design Spec

**Date:** 2026-04-08
**Status:** Approved

---

## Overview

Nova is a standalone Discord bot (Go + discordgo + SQLite) that spawns and orchestrates multiple Claude Code CLI instances, using Discord channels as the communication medium. Users can manually control agents via slash commands, or seed an orchestrator agent that autonomously spawns and directs workers.

---

## Core Concepts

**Session** — one Claude Code instance: a workspace directory under `~/.nova/sessions/<id>/`, a Discord channel, and a conversation backed by the Claude CLI's own session storage.

**Swarm** — a named group of sessions working toward a shared goal. Has a dedicated Discord category. May have one orchestrator session that directs workers via directives.

**Control channel** — a single `#nova` channel (name configurable) where the user issues slash commands. The swarm creates and owns all other channels.

**Directive** — a single-line JSON object emitted by a Claude session to control the swarm. Intercepted by the bot before posting to Discord.

---

## Session Lifecycle

Sessions move through three states:

```
              /nova spawn
                   │
                   ▼
              ┌─────────┐   idle timeout    ┌─────────┐
              │   HOT   │ ────────────────► │  COLD   │
              │(subprocess│ ◄──────────────  │(no proc)│
              │  running) │  message arrives └─────────┘
              └─────────┘                        │
                   │                             │
              /nova kill                    /nova kill
                   │                             │
                   ▼                             ▼
              ┌────────────────────────────────────┐
              │            TERMINATED              │
              └────────────────────────────────────┘
```

- **HOT → COLD**: After `idle_timeout_minutes` (default: 10) with no messages, the bot sends SIGTERM to the subprocess. The Claude CLI's own session storage preserves the conversation.
- **COLD → HOT**: The next message to that session spawns `claude --resume <claude-sid>`. The new process inherits the full prior conversation.
- **TERMINATED**: `/nova kill` terminates the subprocess (if HOT), marks the session terminated in SQLite, renames the Discord channel with a `✓` prefix, makes it read-only, and moves it to `[Nova: archived]`. Workspace remains on disk until `/nova clean`.

---

## Subprocess Management

Each HOT session owns:

```go
type Session struct {
    ID        string
    Name      string
    ClaudeSID string          // claude's own session ID (for --resume)
    Workspace string          // ~/.nova/sessions/<id>/
    ChannelID string
    SwarmID   string

    cmd       *exec.Cmd
    stdin     io.WriteCloser
    stdout    *bufio.Reader
    idleTimer *time.Timer
    msgCh     chan string      // buffered(8): incoming Discord messages
    mu        sync.Mutex
}
```

### Spawning

First spawn (no prior session):
```
cd ~/.nova/sessions/<id>/
claude --system-prompt-file ~/.nova/system-prompt.txt
```

Resuming a COLD session:
```
cd ~/.nova/sessions/<id>/
claude --resume <claude-sid> --system-prompt-file ~/.nova/system-prompt.txt
```

The Claude session ID is captured from the first response and stored in SQLite.

### System Prompt

Written to `~/.nova/system-prompt.txt` at bot startup (see `bot/nova.go`):

```
You are Nova, a Discord-native AI agent. Your responses are posted to a Discord channel.

## Nova's own codebase

Nova's source code is at /workspace (Go module: nova).
You can update nova's code and restart it yourself:

  1. Edit files under /workspace as needed.
  2. Run tests:  cd /workspace && go test ./...
  3. Rebuild:    cd /workspace && CGO_ENABLED=0 go build -o bin/nova .
  4. Restart:    emit {"type":"restart"} — nova exits and Docker restarts it with the new binary.

When nova comes back online it announces itself in the control channel.

## Swarm directives

To issue directives, emit one JSON object per line with a "type" field.
Directives are intercepted by the bot and not posted to Discord.

Available directive types:
  {"type":"spawn","name":"<name>","task":"<initial message>"}
  {"type":"send","to":"<name>","message":"<msg>"}
  {"type":"create_channel","name":"<name>"}
  {"type":"restart"}

All other output is posted to your Discord channel verbatim.
```

### Response Reading

A goroutine per HOT session reads stdout line-by-line:
- Lines that parse as JSON with a `"type"` field are dispatched to the directive handler.
- `{"type":"done"}` flushes the accumulated buffer to Discord (chunked at 2000 chars) and resets.
- All other lines append to the buffer.

### Message Writing

Incoming Discord messages are pushed onto `msgCh`. A writer goroutine drains `msgCh`, writes each to stdin with a trailing newline, and resets the idle timer.

---

## Discord Structure

### Fixed (bot ensures on startup)

```
Server
└── #nova                     ← control channel; slash commands only
```

### Dynamic (created at runtime)

```
Server
├── #nova
├── [Nova: solo]              ← category for sessions not in a swarm
│   └── #<session-name>
└── [Nova: <swarm-name>]      ← category per swarm
    ├── #<orchestrator-name>
    ├── #<worker-1>
    └── #<worker-2>
```

### Channel Lifecycle

- `/nova spawn` → creates channel in appropriate category, topic set to `[nova] <name> | hot`
- HOT → COLD → topic updated to `[nova] <name> | cold`
- `/nova kill` → channel renamed `✓-<name>`, permissions locked (read-only), moved to `[Nova: archived]`

### Permissions

Bot requires: `MANAGE_CHANNELS`, `MANAGE_ROLES`.
Any non-bot message posted to a session channel is routed to that session's stdin.
Commands issued from within a session channel respond ephemerally.

---

## Directive Protocol

Directives are single-line JSON objects emitted by Claude on their own line. The bot checks each line of stdout before appending to the Discord buffer.

| `type` | Payload fields | Bot action |
|---|---|---|
| `spawn` | `name`, `task` | Create session + workspace + channel, inject `task` as first stdin message |
| `send` | `to`, `message` | Write `message` to target session's stdin (warm if COLD) |
| `create_channel` | `name` | Create text channel in the emitting session's swarm category |
| `done` | — | Flush buffer to Discord, reset |
| `restart` | — | Post "Restarting nova... brb" to Discord, then call `os.Exit(0)`; Docker's `restart: unless-stopped` restarts the process. Nova posts "Nova is online." on comeback. |

Unknown `type` values are logged and ignored (not posted to Discord).

---

## Swarm & Orchestration

### Human-driven

```
/nova spawn --name worker-1
/nova spawn --name worker-2 --swarm backend
/nova broadcast backend "implement auth, split the work between yourselves"
```

### Claude-orchestrated

```
/nova swarm create backend
/nova spawn --name orchestrator --swarm backend
[type in #orchestrator]: "Refactor the auth module. Spawn workers and coordinate."
```

The orchestrator emits `spawn` and `send` directives. The bot executes them, creating sessions and routing messages.

### Broadcast

`/nova broadcast <swarm> <message>` writes to stdin of every session in the swarm simultaneously. COLD sessions are warmed before the message is sent.

### Dissolve

`/nova swarm dissolve <name>` kills all sessions, archives their channels, and deletes the Discord category.

---

## Persistence — SQLite Schema

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    claude_sid   TEXT,
    workspace    TEXT NOT NULL,
    channel_id   TEXT NOT NULL,
    swarm_id     TEXT,
    status       TEXT NOT NULL DEFAULT 'cold',  -- hot | cold | terminated
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_active  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS swarms (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    category_id  TEXT NOT NULL,
    orch_id      TEXT,   -- session id of orchestrator, nullable
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT NOT NULL,
    role         TEXT NOT NULL,   -- user | assistant
    content      TEXT NOT NULL,
    ts           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

On bot startup: all sessions with `status = 'hot'` are set to `'cold'` (subprocesses died with the bot). They will warm automatically on next message.

---

## Slash Command Interface

All commands are registered under `/nova`.

### Session commands

```
/nova spawn [name:<string>] [swarm:<string>]
    Creates session + workspace + channel. Responds with a link to the new channel.
    Name is auto-generated (adjective-noun) if omitted.

/nova list [swarm:<string>]
    Ephemeral table: name | status | swarm | last active.

/nova kill <name>
    Terminates session, archives channel.

/nova resume <name>
    Force warm up a COLD session immediately (otherwise auto-warms on next message).

/nova status [name:<string>]
    Ephemeral detail: status, workspace path, channel, message count, last active.
```

### Swarm commands

```
/nova swarm create <name>
    Creates swarm record + Discord category.

/nova swarm dissolve <name>
    Kills all sessions in swarm, archives channels, removes category.

/nova broadcast <swarm> <message>
    Sends message to all sessions in the swarm.
```

### Utility

```
/nova clean
    Deletes on-disk workspaces for all TERMINATED sessions.
```

---

## Configuration

Config file at `~/.nova/config.toml` (override with `--config` flag).

```toml
bot_token             = ""
control_channel_name  = "nova"
session_root          = "~/.nova/sessions/"
idle_timeout_minutes  = 10
debug                 = false
```

Parsed at startup into a `config.Config` struct. Bot refuses to start if `bot_token` is empty.

---

## Project Structure

```
nova/
├── main.go
├── go.mod                     (module nova)
├── justfile
├── nova.service
├── config/
│   └── config.go              (loads config.toml, exposes config.Config)
├── nova/
│   ├── nova.go                (Intents, Init, Run)
│   ├── db/
│   │   └── db.go              (Store: migrate, sessions, swarms, messages)
│   ├── session/
│   │   ├── session.go         (Session struct, HOT/COLD state, subprocess I/O)
│   │   └── manager.go         (SessionManager: spawn, kill, resume, idle GC)
│   ├── swarm/
│   │   └── swarm.go           (Swarm struct, create, dissolve, broadcast)
│   ├── directive/
│   │   └── directive.go       (JSON directive parsing and dispatch)
│   ├── discord/
│   │   └── discord.go         (channel/category create, archive, permission helpers)
│   └── commands/
│       └── commands.go        (slash command registration + interaction handlers)
└── internal/
    └── testdiscord/
        └── testdiscord.go     (Discord session stub for testing)
```

---

## Bot Startup Sequence

1. Load `config.toml`
2. Open SQLite, run migrations
3. Reset any `status = 'hot'` sessions to `'cold'`
4. Write `~/.nova/system-prompt.txt`
5. Connect to Discord
6. Ensure `#nova` control channel exists (create if missing)
7. Ensure `[Nova: solo]` and `[Nova: archived]` categories exist
8. Spawn or revive the control session
9. Register `/nova` slash command tree
10. Post "Nova is online." to the control channel (also serves as restart confirmation)
11. Block on SIGINT/SIGTERM

---

## Key Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Claude doesn't emit `{"type":"done"}` | Per-message read timeout (30s); post whatever is buffered |
| Subprocess hangs on stdin write | Non-blocking channel send with drop-and-log on full `msgCh` |
| Discord rate limits on channel creation | Exponential backoff in discord helper; swarm creation is not latency-sensitive |
| Claude session ID not captured on first spawn | Retry logic; fail session creation cleanly if not captured within first 3 responses |
| Workspace disk growth | `/nova clean` + documented cron recommendation |

## Implementation Assumptions to Verify

These must be confirmed against the actual `claude` CLI before implementing subprocess management:

1. **`--resume <session-id>` flag** — assumed to exist for continuing a prior Claude Code session. Verify exact flag name and behavior.
2. **`--system-prompt-file <path>` flag** — assumed to inject a system prompt from a file. Verify; fallback is to prepend the system prompt as the first stdin message on every new spawn.
3. **Session ID capture mechanism** — the Claude session ID must be stored in SQLite so COLD sessions can be resumed. Determine whether the CLI exposes this via: (a) stdout output on first run, (b) a flag like `--print-session-id`, or (c) the `~/.claude/projects/` directory structure (can be inspected before/after spawn to find the new session). Implementation of HOT→COLD resume depends on this.
