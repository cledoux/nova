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
WORKDIR /workspace

ENV HOME=/home/agent
ENV GOPATH=/home/agent/go
ENV PATH=$PATH:/home/agent/go/bin:/home/agent/.local/bin
ENV PIPX_HOME=/home/agent/.pipx
ENV PIPX_BIN_DIR=/home/agent/.local/bin

# Claude Code — installed as agent so it lands in /home/agent/.local/bin (already in PATH)
RUN curl -fsSL https://claude.ai/install.sh | bash

# Vim config and plugins — run bootstrap-vim first to stage build/.vimrc
COPY --chown=agent:agent build/.vimrc /home/agent/.vimrc
RUN vim +PlugInstall +qall! 2>&1 | tail -5
