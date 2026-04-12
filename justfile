IMAGE    := "ghcr.io/anthropics/claude-code:latest"
BINARY   := "nova"
CONTAINER := "nova"

# Pull the latest Claude Code image
pull:
	docker pull {{IMAGE}}

# Build the nova binary (static, no CGO) for Docker deployment
build:
	CGO_ENABLED=0 go build -o {{BINARY}} .

# Run tests
test:
	go test ./...

# Build then run all tests
test-all: build test

# Format and lint all Go files
fmt:
	gofmt -s -w .
	goimports -w . 2>/dev/null || true

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

# Tail logs from the running nova Docker container
logs:
	docker logs -f {{CONTAINER}}

# Tail logs from the nova systemd service (non-Docker deployment)
service-logs:
	journalctl --user -u nova -f

# Restart the nova systemd service (non-Docker deployment)
restart:
	systemctl --user restart nova

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
	extra_path={{extra}}
	extra_mount=()
	if [[ -n "$extra_path" ]]; then
		name=$(basename "$extra_path")
		extra_mount=(-v "$extra_path:/workspace/$name")
	fi
	docker run -it --rm \
		-v "$HOME/.claude:/root/.claude" \
		"${mounts[@]}" \
		"${extra_mount[@]}" \
		-w /workspace/nova \
		{{IMAGE}} \
		claude
