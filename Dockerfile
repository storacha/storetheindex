# ============================================
# Build stage (shared)
# ============================================
FROM golang:1.24-bookworm AS build

WORKDIR /go/src/storetheindex

COPY go.* .
RUN go mod download
COPY . .

# Production build - with symbol stripping
RUN make storetheindex-prod

# ============================================
# Debug build stage
# ============================================
FROM build AS build-debug

# Install delve debugger
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Debug build - no optimizations, no inlining
RUN make storetheindex-debug

# ============================================
# Production image
# ============================================
FROM debian:bookworm-slim AS prod

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /go/src/storetheindex/storetheindex /usr/local/bin/storetheindex

# Default port configuration:
#  - 3000 Finder interface
#  - 3001 Ingest interface
#  - 3002 Admin interface
#  - 3003 libp2p interface
EXPOSE 3000-3003

ENTRYPOINT ["/usr/local/bin/storetheindex"]
CMD ["daemon"]

# ============================================
# Development image
# ============================================
FROM debian:bookworm-slim AS dev

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    # Shell experience
    bash-completion \
    less \
    vim-tiny \
    # Process debugging
    procps \
    htop \
    strace \
    # Network debugging
    iputils-ping \
    dnsutils \
    net-tools \
    tcpdump \
    # Data tools
    jq \
    && rm -rf /var/lib/apt/lists/*

# Delve debugger
COPY --from=build-debug /go/bin/dlv /usr/bin/dlv

# Debug binary (with symbols, no optimizations)
COPY --from=build-debug /go/src/storetheindex/storetheindex /usr/local/bin/storetheindex

# Default port configuration:
#  - 3000 Finder interface
#  - 3001 Ingest interface
#  - 3002 Admin interface
#  - 3003 libp2p interface
EXPOSE 3000-3003

# Shell niceties
ENV TERM=xterm-256color
RUN echo 'alias ll="ls -la"' >> /etc/bash.bashrc && \
    echo 'PS1="\[\e[32m\][storetheindex-dev]\[\e[m\] \w# "' >> /etc/bash.bashrc

SHELL ["/bin/bash", "-c"]
ENTRYPOINT ["/usr/local/bin/storetheindex"]
CMD ["daemon"]
