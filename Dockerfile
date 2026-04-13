FROM debian:trixie-slim

ARG GO_VERSION=1.25.1
ARG NODE_MAJOR=22

# System dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    git \
    gnupg \
    jq \
    pipx \
    python3 \
    python3-dev \
    python3-pip \
    python3-venv \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Debug tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    procps \
    htop \
    && rm -rf /var/lib/apt/lists/*

# Node.js via NodeSource
RUN curl -fsSL https://deb.nodesource.com/setup_${NODE_MAJOR}.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Go
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz \
    | tar -C /usr/local -xz
ENV PATH=$PATH:/usr/local/go/bin

# Claude Code (global, installed as root before switching user)
RUN npm install -g @anthropic-ai/claude-code

# Non-root user — UID 1000 matches most primary Linux users for clean volume permissions
RUN useradd -m -u 1000 -s /bin/bash agent

USER agent
WORKDIR /workspace

ENV HOME=/home/agent
ENV GOPATH=/home/agent/go
ENV PATH=$PATH:/home/agent/go/bin:/home/agent/.local/bin
ENV PIPX_HOME=/home/agent/.pipx
ENV PIPX_BIN_DIR=/home/agent/.local/bin
