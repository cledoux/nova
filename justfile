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
