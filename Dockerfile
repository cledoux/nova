FROM debian:trixie-slim

ARG GO_VERSION=1.25.1
ARG NODE_MAJOR=22

# System dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    exuberant-ctags \
    git \
    gnupg \
    jq \
    just \
    pipx \
    python3 \
    python3-dev \
    python3-pip \
    python3-venv \
    vim \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Debug tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    procps \
    htop \
    && rm -rf /var/lib/apt/lists/*

# Go
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz \
    | tar -C /usr/local -xz
ENV PATH=$PATH:/usr/local/go/bin

# Non-root user — UID 1000 matches most primary Linux users for clean volume permissions
RUN useradd -m -u 1000 -s /bin/bash agent

USER agent

ENV HOME=/home/agent
WORKDIR $HOME
ENV GOPATH=/home/agent/go
ENV PATH=$PATH:/home/agent/go/bin:/home/agent/.local/bin
ENV PIPX_HOME=/home/agent/.pipx
ENV PIPX_BIN_DIR=/home/agent/.local/bin

# Claude Code — installed as agent so it lands in /home/agent/.local/bin (already in PATH)
RUN curl -fsSL https://claude.ai/install.sh | bash

# Seed .claude.json so the first-run wizard is skipped on fresh deploys.
# Live state accumulates in the agent-home named volume; this only applies
# when the volume is first created.
RUN echo '{"numStartups":1,"installMethod":"native","autoUpdates":false,"hasCompletedOnboarding":true,"lastOnboardingVersion":"2.1.110","migrationVersion":11,"opusProMigrationComplete":true,"sonnet1m45MigrationComplete":true,"projects":{"/home/agent":{"hasTrustDialogAccepted":true}}}' \
    > /home/agent/.claude.json

# Beans — flat-file issue tracker for agent+human collaboration
# https://github.com/hmans/beans
RUN curl -fsSL $(curl -fsSL https://api.github.com/repos/hmans/beans/releases/latest \
        | python3 -c "import sys,json; print(next(a['browser_download_url'] for a in json.load(sys.stdin)['assets'] if 'Linux_x86_64' in a['name']))") \
    | tar -xz -C /home/agent/.local/bin beans \
    && chmod +x /home/agent/.local/bin/beans

# Vim config and plugins — run prepare-build-image first to stage these files
COPY --chown=agent:agent build/.vimrc /home/agent/.vimrc
COPY --chown=agent:agent build/.vim-autoload /home/agent/.vim/autoload
RUN vim --cmd "let g:plug_threads=1" -u /home/agent/.vimrc -i NONE -es \
    -c "PlugInstall" -c "qa" < /dev/null || true
