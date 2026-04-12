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

Module name is `nova`. The nested `nova/nova/` path creates triple-nova import paths:

| Directory | Import path |
|-----------|-------------|
| `config/` | `nova/config` |
| `nova/db/` | `nova/nova/db` |
| `nova/directive/` | `nova/nova/directive` |
| `nova/discord/` | `nova/nova/discord` |
| `nova/session/` | `nova/nova/session` |
| `nova/swarm/` | `nova/nova/swarm` |
| `nova/nova/` | `nova/nova/nova` |
| `nova/nova/commands/` | `nova/nova/nova/commands` |
| `internal/testdiscord/` | `nova/internal/testdiscord` |

## Testing conventions

- No assert libraries. Use standard `testing` comparisons with `t.Errorf`/`t.Fatalf`.
- DB tests use `":memory:"` (pass to `db.New`).
- Discord tests use `testdiscord.New()` from `nova/internal/testdiscord`.
- Session tests that exercise hot/cold lifecycle use a fake `claudeBin` path (`/bin/sh`) with a shell one-liner as the subprocess; see `session_test.go` for the pattern.

## Key design decisions

- **Pre-assigned session IDs**: `Manager.Spawn` generates a UUID and passes it as `--session-id` to the Claude CLI. This avoids directory diffing to discover the session ID after launch.
- **Generation counter**: Each `Session.Warm()` call increments `gen`. Background goroutines capture their generation at start and call `coolIfGen(gen)` instead of `cool()` to prevent stale goroutines from interfering after a re-warm.
- **Directive protocol**: Claude agents emit one JSON object per line. Lines starting with `{` are intercepted and not posted to Discord. Lines not starting with `{` are buffered as content. `{"type":"done"}` flushes the buffer to Discord.
- **`cool()` before `OnIdle()`**: The session status is set to cold *before* calling the `OnIdle` callback so that any code in the callback that checks status sees the correct value.

## Pitfalls

- **`.gitignore` and the `nova/` directory**: The `.gitignore` has `/nova` (root binary) and `!/nova/` (un-ignore the package directory). Don't collapse these into a plain `nova` entry or `git add nova/` will fail.
- **`ChannelEdit.Topic`** is `string`, not `*string`, in discordgo v0.29.0.
- **SQLite timestamp precision**: `CURRENT_TIMESTAMP` has second-level granularity. The `TouchSession` query uses `strftime('%Y-%m-%d %H:%M:%f', 'now')` for millisecond precision.
