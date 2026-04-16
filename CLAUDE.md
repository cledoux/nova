# Nova — Claude Code guidance

## Backlog

Features and bugs are tracked in [`BACKLOG.md`](BACKLOG.md) at the repo root. Check it when looking for work to do.

## Build and test

```sh
go build -o /tmp/nova .   # use /tmp to avoid polluting the repo root
go test ./...
```

After any Go changes, run format + lint:

```sh
just fmt
# or manually:
gofmt -s -w .
goimports -w . 2>/dev/null || true
```

## Module layout

Module name is `nova`. Packages are flat at the repo root:

| Directory | Import path |
|-----------|-------------|
| `config/` | `nova/config` |
| `db/` | `nova/db` |
| `directive/` | `nova/directive` |
| `discord/` | `nova/discord` |
| `session/` | `nova/session` |
| `swarm/` | `nova/swarm` |
| `bot/` | `nova/bot` |
| `bot/commands/` | `nova/bot/commands` |
| `internal/testdiscord/` | `nova/internal/testdiscord` |

## Testing conventions

- No assert libraries. Use standard `testing` comparisons with `t.Errorf`/`t.Fatalf`.
- DB tests use `":memory:"` (pass to `db.New`).
- Discord tests use `testdiscord.New()` from `nova/internal/testdiscord`.
- Session tests that exercise hot/cold lifecycle use a fake `claudeBin` path (`/bin/sh`) with a shell one-liner as the subprocess; see `session_test.go` for the pattern.

## Key design decisions

- **Pre-assigned session IDs**: `Manager.Spawn` generates a UUID and passes it as `--session-id` to the Claude CLI. This avoids directory diffing to discover the session ID after launch.
- **Generation counter**: Each `Session.Warm()` call increments `gen`. Background goroutines capture their generation at start and call `coolIfGen(gen)` instead of `cool()` to prevent stale goroutines from interfering after a re-warm.
- **Directive protocol**: Claude agents emit one JSON object per line. Lines starting with `{` are intercepted and not posted to Discord. Lines not starting with `{` are buffered as content. `{"type":"done"}` flushes the buffer to Discord. `{"type":"restart"}` posts a notice to Discord then calls `os.Exit(0)`; Docker's `restart: unless-stopped` brings the process back up.
- **`cool()` before `OnIdle()`**: The session status is set to cold *before* calling the `OnIdle` callback so that any code in the callback that checks status sees the correct value.

## Docker / deployment

- **`docker compose restart` vs `up -d`**: `restart` stops and starts the existing container without re-reading the compose file. Config changes (e.g. `working_dir`, volume mounts, environment) only take effect after `docker compose up -d`, which recreates the container.
- **SQLite DB location**: `workspace/data/nova.db` relative to the container's `working_dir` (`/home/agent`), so the live DB is at `/home/agent/workspace/data/nova.db` (inside the repo bind-mount). To force fresh session creation, delete `data/nova.db` and run `docker compose up -d`.
- **Session workspace**: `repo_path` in `mounts/nova/config.toml` controls the working directory passed to each Claude session (`--workspace` flag and `cmd.Dir`). Currently set to `/home/agent` (home directory). The code default is `/home/agent/workspace`.
- **Versioned Claude config**: specific files in `mounts/claude/` are bind-mounted into `~/.claude/` individually (see `docker-compose.yml`). Add a new line there for each new file to track. Everything else in `~/.claude/` (plugins, session state, credentials) lives in the `agent-home` named volume.

## Pitfalls

- **`.gitignore` and the `/nova` binary**: The `.gitignore` uses `/nova` (root binary) and `!/nova/` to un-ignore a `nova/` directory. The `nova/` package directory no longer exists, so this is a no-op and can be simplified to just `/nova` if desired.
- **`ChannelEdit.Topic`** is `string`, not `*string`, in discordgo v0.29.0.
- **SQLite timestamp precision**: `CURRENT_TIMESTAMP` has second-level granularity. The `TouchSession` query uses `strftime('%Y-%m-%d %H:%M:%f', 'now')` for millisecond precision.
