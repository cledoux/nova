# Nova

<img src="img/nova_icon.png" alt="Nova icon" width="200" align="right" />

Nova is a Discord bot that runs a [Claude Code](https://claude.ai/code) CLI subprocess as a persistent agent. You interact with it by chatting in its `#nova` channel, or by @-mentioning it in any channel (Nova replies in-place). Additional sessions can be spawned via `/nova spawn`; each gets its own Discord channel.

## Requirements

- Go 1.25+
- Claude Code CLI (`claude`) installed and authenticated
- A Discord bot token with the **Message Content** intent enabled

## Quick start

```sh
# Build
go build -o /usr/local/bin/nova .

# Create config
mkdir -p ~/.nova
cat > ~/.nova/config.toml <<EOF
bot_token = "your-bot-token-here"
guild_id  = "your-guild-id-here"
EOF

# Run
nova
```

Nova will create (or reuse) a `#nova` channel in your server and announce itself there.

## Configuration

Config file location: `~/.nova/config.toml` (override with `--config`).

| Key | Default | Description |
|-----|---------|-------------|
| `bot_token` | _(required)_ | Discord bot token |
| `guild_id` | _(required)_ | Discord server (guild) ID |
| `control_channel_name` | `nova` | Name of the bot control channel |
| `session_root` | `~/.nova/sessions` | Directory where session workspaces are created |
| `idle_timeout_minutes` | `10` | Minutes of inactivity before a session goes cold |
| `claude_bin` | `claude` | Path to the Claude Code CLI binary |
| `debug` | `false` | Enable verbose logging |

## Slash commands

All commands are under `/nova` and respond ephemerally (only visible to you).

| Command | Description |
|---------|-------------|
| `/nova spawn [name]` | Spawn a new Claude session |
| `/nova list` | List active sessions |
| `/nova kill <name>` | Terminate a session |
| `/nova resume <name>` | Force-warm a cold session |
| `/nova status <name>` | Show session details (status, workspace, message count) |
| `/nova clean` | Delete workspaces of terminated sessions |
| `/nova restart` | Restart the nova bot process |
| `/nova help` | Show command reference |

## Session lifecycle

Sessions have three states:

- **hot** — Claude subprocess is running; messages route to its stdin immediately.
- **cold** — subprocess exited after the idle timeout; resumes automatically on next message using `claude --resume <id>`.
- **terminated** — permanently stopped via `/nova kill`.

## @mention routing

Mentioning Nova in any channel (or including "nova" in your message) triggers a response directly in that channel. The first mention spawns a dedicated session bound to that channel; subsequent mentions resume the same session.

## Directive protocol

Nova emits responses as plain text. It can also emit JSON directives on their own line to control its own process:

```
{"type":"restart"}   — nova exits; Docker restarts it with the same binary
{"type":"done"}      — no-op (turn completion is signalled by the stream-json protocol)
```

Directive lines are intercepted and never posted to Discord.

## Deployment

### Docker (recommended)

```sh
# Build image, build nova binary, and start the container (detached)
just up

# Tail logs
just logs-docker

# Stop
just down

# Restart without rebuilding (e.g. after a config change)
just restart-docker
```

Nova and its Claude subprocesses run as a non-root `agent` user (UID 1000).

To apply self-improvements made by an agent: `just up` (rebuilds the binary from the modified source and restarts the container).

### systemd (user service)

```sh
cp nova.service ~/.config/systemd/user/nova.service
systemctl --user enable --now nova
```

## Development

```sh
go test ./...    # run tests
just fmt         # format + lint
just build       # build binary
```
