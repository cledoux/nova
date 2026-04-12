# Docker Sandbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a Docker-based sandbox for running the Nova bot and Claude Code subprocesses, with a `justfile` for ergonomic local operation.

**Architecture:** Use the official `ghcr.io/anthropics/claude-code:latest` image directly — no custom Dockerfile. The Nova binary is compiled on the host (static Go binary) and bind-mounted into the container. Host paths visible inside the container are controlled by `sandbox.conf`. Two paths are always mounted: `~/.claude` (credentials) and the nova binary. Full internet access, no firewall.

**Tech Stack:** Docker, `just`, bash (for mount-list construction in recipes)

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `sandbox.conf` | Create | Allow-list of host paths to mount into the container |
| `justfile` | Create | `pull`, `build`, `start`, `stop`, `logs`, `sandbox` recipes |

> **Note on justfile:** The nova implementation plan (Task 1) also adds recipes to `justfile` (`test`, `fmt`, etc.). This plan creates the file first with sandbox recipes only. When the nova implementation plan runs, it should **add** to the existing justfile rather than replace it.

---

### Task 1: Create sandbox.conf and justfile

**Goal:** Produce a working `justfile` with all six sandbox recipes and a `sandbox.conf` with the default allow list.

**Files:**
- Create: `sandbox.conf`
- Create: `justfile`

**Acceptance Criteria:**
- [ ] `just --list` shows all six recipes without errors
- [ ] `sandbox.conf` contains the nova project path and a comment header
- [ ] `justfile` compiles cleanly (no syntax errors)

**Verify:** `just --list` → lists pull, build, start, stop, logs, sandbox

**Steps:**

- [ ] **Step 1: Create sandbox.conf**

Create `sandbox.conf` in the nova project root:

```
# sandbox.conf — host paths to mount into the sandbox container
# One absolute path per line. Lines starting with # are ignored.
# Each path is mounted at /workspace/<basename> inside the container.
/home/cledoux/workspace/discord/nova
```

- [ ] **Step 2: Create justfile**

Create `justfile` in the nova project root:

```just
IMAGE    := "ghcr.io/anthropics/claude-code:latest"
BINARY   := "nova"
CONTAINER := "nova"

# Pull the latest Claude Code image
pull:
    docker pull {{IMAGE}}

# Build the nova binary (static, no CGO)
build:
    CGO_ENABLED=0 go build -o {{BINARY}} .

# Start nova in the container (daemon mode). Reads mount list from sandbox.conf.
start: build
    #!/usr/bin/env bash
    set -euo pipefail
    mounts=()
    while IFS= read -r line; do
        if [[ "$line" =~ ^# ]] || [[ -z "$line" ]]; then continue; fi
        name=$(basename "$line")
        mounts+=(-v "$line:/workspace/$name")
    done < sandbox.conf
    docker run -d \
        --name {{CONTAINER}} \
        --restart unless-stopped \
        -v "$(pwd)/{{BINARY}}:/usr/local/bin/{{BINARY}}:ro" \
        -v "$HOME/.claude:/root/.claude" \
        "${mounts[@]}" \
        -w /workspace/{{BINARY}} \
        {{IMAGE}} \
        /usr/local/bin/{{BINARY}}

# Stop and remove the nova container
stop:
    docker stop {{CONTAINER}} || true
    docker rm   {{CONTAINER}} || true

# Tail logs from the nova container
logs:
    docker logs -f {{CONTAINER}}

# Start an interactive Claude Code session. Optional: pass an extra host path as argument.
# Example: just sandbox /home/user/some-project
sandbox extra="":
    #!/usr/bin/env bash
    set -euo pipefail
    mounts=()
    while IFS= read -r line; do
        if [[ "$line" =~ ^# ]] || [[ -z "$line" ]]; then continue; fi
        name=$(basename "$line")
        mounts+=(-v "$line:/workspace/$name")
    done < sandbox.conf
    extra_mount=()
    if [[ -n "{{extra}}" ]]; then
        name=$(basename "{{extra}}")
        extra_mount=(-v "{{extra}}:/workspace/$name")
    fi
    docker run -it --rm \
        -v "$HOME/.claude:/root/.claude" \
        "${mounts[@]}" \
        "${extra_mount[@]}" \
        -w /workspace/nova \
        {{IMAGE}} \
        claude
```

- [ ] **Step 3: Verify justfile syntax**

```bash
just --list
```

Expected output (order may vary):
```
Available recipes:
    build           # Build the nova binary (static, no CGO)
    logs            # Tail logs from the nova container
    pull            # Pull the latest Claude Code image
    sandbox extra="" # Start an interactive Claude Code session...
    start           # Start nova in the container (daemon mode)...
    stop            # Stop and remove the nova container
```

If `just --list` errors, check for syntax issues: missing colons after recipe names, bad indentation (must be tabs not spaces), or unclosed string literals.

- [ ] **Step 4: Commit**

```bash
git add sandbox.conf justfile
git commit -m "feat: add Docker sandbox justfile and allow-list config"
```

---

### Task 2: Smoke test — verify image and interactive sandbox

**Goal:** Confirm the official image pulls and that `just sandbox` correctly launches an interactive Claude Code session with the expected mounts.

**Files:** (none created — verification only)

**Acceptance Criteria:**
- [ ] `just pull` completes without error
- [ ] Image version matches expected (`claude --version` inside container)
- [ ] `~/.claude` is visible inside the container at `/root/.claude`
- [ ] `/workspace/nova` is visible inside the container
- [ ] Container exits cleanly after session ends

**Verify:** `docker run --rm -v "$HOME/.claude:/root/.claude" ghcr.io/anthropics/claude-code:latest claude --version` → prints version string

**Steps:**

- [ ] **Step 1: Pull the image**

```bash
just pull
```

Expected: image layers download, `Status: Downloaded newer image for ghcr.io/anthropics/claude-code:latest` (or "Image is up to date" if already pulled).

- [ ] **Step 2: Verify the image runs claude**

```bash
docker run --rm ghcr.io/anthropics/claude-code:latest claude --version
```

Expected: prints a version string like `1.x.x`. Confirms the image is healthy and `claude` is on PATH.

- [ ] **Step 3: Verify mounts are correct**

Run a non-interactive check of mount visibility:

```bash
docker run --rm \
    -v "$HOME/.claude:/root/.claude" \
    -v "$(pwd):/workspace/nova" \
    ghcr.io/anthropics/claude-code:latest \
    bash -c "ls /root/.claude && ls /workspace/nova"
```

Expected: contents of `~/.claude` and the nova project directory are listed. If either directory is empty or errors, check that the source paths exist on the host.

- [ ] **Step 4: Launch interactive sandbox (manual verification)**

```bash
just sandbox
```

Expected: drops into an interactive `claude` session. Type `/help` to verify Claude Code is responsive, then `exit` or Ctrl+D to quit. Container removes itself automatically (`--rm`).

- [ ] **Step 5: Commit (if any fixups were needed)**

If any corrections were made to `justfile` or `sandbox.conf` during testing:

```bash
git add justfile sandbox.conf
git commit -m "fix: correct sandbox mount paths after smoke test"
```

If no corrections needed, skip this step.
