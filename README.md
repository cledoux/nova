# Nova

<img src="img/nova_icon.png" alt="Nova icon" width="200" align="right" />

Nova is a Discord bot that spawns and orchestrates [Claude Code](https://claude.ai/code) CLI subprocesses. Each session gets its own Discord channel; you chat with the agent by typing in that channel, and the agent can spawn additional agents, send them messages, and create new channels — all via a JSON directive protocol embedded in its output.

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

Nova will create a `#nova` control channel in your server and a **Nova: solo** category for solo sessions. Use `/nova spawn` to start your first agent.

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
| `/nova spawn [name] [swarm]` | Spawn a new Claude session |
| `/nova list [swarm]` | List active sessions |
| `/nova kill <name>` | Terminate a session and archive its channel |
| `/nova resume <name>` | Force-warm a cold session |
| `/nova status <name>` | Show session details (status, workspace, message count) |
| `/nova clean` | Delete workspaces of terminated sessions |
| `/nova broadcast <swarm> <message>` | Send a message to all sessions in a swarm |
| `/nova swarm create <name>` | Create a swarm (gets its own Discord category) |
| `/nova swarm dissolve <name>` | Kill all sessions in a swarm and remove it |

## Session lifecycle

Sessions have three states:

- **hot** — Claude subprocess is running; messages route to its stdin immediately.
- **cold** — subprocess exited after the idle timeout; resumes automatically on next message using `claude --resume <id>`.
- **terminated** — permanently stopped; Discord channel is archived and made read-only.

## Swarms

A swarm is a named group of sessions that share a Discord category. Agents within a swarm can spawn additional agents and send messages to each other by name using the directive protocol. Create one with `/nova swarm create <name>`, then `/nova spawn --swarm <name>` to add sessions.

## Deployment

### Docker (recommended)

Runs nova in a container that survives machine restarts.

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

Volumes mounted into the container:

| Host path | Container path | Purpose |
|-----------|---------------|---------|
| `~/.claude` | `/home/agent/.claude` | Claude Code auth |
| `~/.nova` | `/home/agent/.nova` | Config and session data |
| nova source dir | `/workspace` | Nova source (writable for self-improvement) |

Nova and its Claude subprocesses run as a non-root `agent` user (UID 1000).

To apply self-improvements made by an agent to the running bot: `just up`
(rebuilds the binary from the modified source and restarts the container).

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
