# Nova — Claude Code guidance

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

## Pitfalls

- **`.gitignore` and the `/nova` binary**: The `.gitignore` uses `/nova` (root binary) and `!/nova/` to un-ignore a `nova/` directory. The `nova/` package directory no longer exists, so this is a no-op and can be simplified to just `/nova` if desired.
- **`ChannelEdit.Topic`** is `string`, not `*string`, in discordgo v0.29.0.
- **SQLite timestamp precision**: `CURRENT_TIMESTAMP` has second-level granularity. The `TouchSession` query uses `strftime('%Y-%m-%d %H:%M:%f', 'now')` for millisecond precision.
