default:
      just --list

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/nova .

test:
	go test ./...

test-all: build test

fmt:
	gofmt -s -w .
	goimports -w . 2>/dev/null || true

build-image: build prepare-build-image
	docker compose build

up: build-image
	docker compose up -d

# Rebuild image and do a clean volume reset — use to test fresh-deploy behavior
rebuild: build-image
	docker compose down -v
	docker compose up -d

down:
	docker compose down

restart: build
	docker compose restart nova

logs:
	docker compose logs -f

shell:
	docker compose exec nova /bin/bash

# -c continues the most recent session; falls back to a new session if none
# exists (e.g. after a clean rebuild). The claude CLI has no built-in
# "continue or new" flag, so we emulate it with shell fallback.
alias cc := claude
alias cld := claude
claude:
	docker compose exec nova bash -c 'claude --dangerously-skip-permissions -c || claude --dangerously-skip-permissions'

# Stage files into build/ for inclusion in the Docker image (re-runnable)
prepare-build-image:
    mkdir -p build
    cp ~/.vimrc build/.vimrc
    cp -r ~/.vim/autoload build/.vim-autoload
    cp ~/.claude/CLAUDE.md build/claude-CLAUDE.md

# Copy Claude config from ~/.claude into mounts/claude/ (re-runnable)
bootstrap-claude:
	rsync -a --delete \
		--include='.credentials.json' \
		--include='settings.json' \
		--include='keybindings.json' \
		--include='statusline-command.sh' \
		--include='plugins/' \
		--include='plugins/**' \
		--exclude='*' \
		~/.claude/ mounts/claude/
	sed -i 's|/home/cledoux/.claude/|/home/agent/.claude/|g' mounts/claude/settings.json
	sed -i 's|/home/cledoux/.claude/|/home/agent/.claude/|g' mounts/claude/plugins/*.json
