# Nova Docker Sandbox — Design Spec

**Date:** 2026-04-09
**Status:** Approved

---

## Overview

A Docker-based sandbox for running the Nova bot and its Claude Code subprocesses in an isolated container. The container provides full internet access while restricting filesystem visibility to an explicit allow list of host paths. A `justfile` in the nova project provides ergonomic recipes for building, launching, and operating the container.

The same setup doubles as a local interactive Claude Code sandbox for personal use.

---

## Architecture

```
Host
├── just build              → nova binary (static Go binary)
├── just start              → docker run ...
│     ├── mounts ~/.claude/ (rw)
│     ├── mounts nova binary (ro)
│     └── mounts allow-listed paths (rw) from sandbox.conf
│
└── Container (ghcr.io/anthropics/claude-code:latest)
      ├── /usr/local/bin/nova   ← bind-mounted binary
      ├── /root/.claude/        ← bind-mounted from host
      ├── /workspace/<name>/    ← one mount per allow-listed path
      └── nova (PID 1)
            └── spawns: claude --session ... (subprocesses)
```

**Base image:** `ghcr.io/anthropics/claude-code:latest` — official Anthropic image with Node.js 20 LTS and the Claude Code CLI pre-installed. No custom Dockerfile required.

**Network:** Default Docker bridge network. Full internet access. No firewall. The container boundary is the sole isolation mechanism.

**No Go runtime needed in the container** — the nova binary is a static Go binary compiled on the host.

---

## Allow List Config

A plain text file `sandbox.conf` in the nova project root. One host path per line. Lines starting with `#` are comments.

```
# sandbox.conf — paths to mount into the sandbox container
/home/cledoux/workspace/discord/nova
```

Inside the container, each path is mounted at `/workspace/<basename>` (e.g., `/workspace/nova`).

Two mounts are always included regardless of config:
- `~/.claude` → `/root/.claude` (read-write) — Claude credentials and session state
- `./nova` (built binary) → `/usr/local/bin/nova` (read-only) — Nova entrypoint

The `just start` recipe reads `sandbox.conf`, constructs the `-v` flags, and passes them to `docker run`. Extra paths can be passed as arguments to `just sandbox` for one-off interactive sessions without editing the config.

---

## `just` Recipes

All recipes live in `nova/justfile`.

| Recipe | Mode | Description |
|--------|------|-------------|
| `just pull` | — | Pull latest `ghcr.io/anthropics/claude-code` image |
| `just build` | — | Compile nova binary on host (`go build`) |
| `just start` | daemon | Build nova binary, then start container named `nova` with nova as entrypoint |
| `just stop` | — | Stop and remove the running `nova` container |
| `just logs` | — | Tail logs from the running `nova` container |
| `just sandbox [path]` | interactive | Start a foreground `claude` session; optional extra host path mounted at `/workspace/<basename>` |

`just start` always runs `just build` first so the binary in the container is always current — no stale binary risk.

---

## Container Details

- **Container name:** `nova` (allows `docker stop nova`, `docker logs nova`, etc.)
- **Entrypoint for `start`:** `/usr/local/bin/nova`
- **Entrypoint for `sandbox`:** `claude`
- **Restart policy:** `--restart unless-stopped` for `just start`; none for `just sandbox`
- **User:** root (required by Claude Code CLI)
- **Working directory:** `/workspace/nova`

---

## Local Interactive Use

`just sandbox` launches an interactive Claude Code session in the same container image and with the same mounts. An optional path argument mounts an additional directory for one-off use:

```sh
just sandbox ~/workspace/some-other-project
```

This mounts `~/workspace/some-other-project` at `/workspace/some-other-project` in addition to the allow list.

---

## Future Considerations

- Per-session workspace isolation (separate subdirectories under `/workspace/nova`) can be added when Nova implements session management
- The allow list could be extended to support per-session overrides passed by Nova at subprocess spawn time
- A custom `FROM ghcr.io/anthropics/claude-code:latest` Dockerfile can be introduced if additional tooling is needed (e.g., `git`, language runtimes)
