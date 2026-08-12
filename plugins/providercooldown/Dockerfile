# syntax=docker/dockerfile:1.7
#
# Builds an arm64 alpine image containing a single bifrost-http binary that
# has the providercooldown plugin statically linked in. The plugin is a
# regular Go package (not a -buildmode=plugin .so), so it shares the
# bifrost-http process and avoids the entire Go plugin ABI risk surface.
#
# Build from the repo root:
#   docker build -f plugins/providercooldown/Dockerfile \
#     --build-arg VERSION=v1.0.0-cooldown \
#     -t myco/bifrost:v1.0.0-cooldown \
#     .
#
# Multi-arch (arm64 + amd64) via buildx:
#   docker buildx build -f plugins/providercooldown/Dockerfile \
#     --platform linux/arm64,linux/amd64 \
#     --build-arg VERSION=v1.0.0-cooldown \
#     -t myco/bifrost:v1.0.0-cooldown \
#     --push \
#     .

# ---- Stage 1: UI build ----
FROM --platform=$BUILDPLATFORM node:25-alpine3.23 AS ui-builder
WORKDIR /app

# Mirror overrides for China / air-gapped environments.
# Usage: --build-arg APK_REPOS=https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.23/main,https://mirrors.tuna.tsinghua.edu.cn/alpine/v3.23/community
#        --build-arg NPM_REGISTRY=https://registry.npmmirror.com
#        --build-arg GOPROXY=https://proxy.golang.org,direct
ARG APK_REPOS=
ARG NPM_REGISTRY=
ARG GOPROXY=https://proxy.golang.org,direct

# Apply latest alpine security patches before installing packages
RUN if [ -n "$APK_REPOS" ]; then \
      echo "$APK_REPOS" | tr ',' '\n' > /etc/apk/repositories; \
    fi && \
    apk upgrade --no-cache

# Copy UI package files and install dependencies
COPY ui/package*.json ./
RUN if [ -n "$NPM_REGISTRY" ]; then npm config set registry "$NPM_REGISTRY"; fi && \
    npm ci

# Copy UI source code
COPY ui/ ./

# Build UI
RUN npm run build-enterprise

# ---- Stage 2: Go build ----
FROM --platform=$BUILDPLATFORM golang:1.26.5-alpine3.23 AS builder
ARG TARGETPLATFORM
ARG APK_REPOS=
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /build

# Install dependencies for CGO + SQLite static linking (with latest security patches)
RUN if [ -n "$APK_REPOS" ]; then \
      echo "$APK_REPOS" | tr ',' '\n' > /etc/apk/repositories; \
    fi && \
    apk upgrade --no-cache && \
    apk add --no-cache gcc musl-dev sqlite-dev binutils binutils-gold

ENV CGO_ENABLED=1 GOOS=linux GOPROXY=$GOPROXY

# Pull all the monorepo source we need. The providercooldown plugin lives
# under plugins/ and is wired into the transports/ build via the local
# `replace` directive in transports/go.mod (which points at
# ../plugins/providercooldown). This makes the plugin a transitive
# dependency of the bifrost-http binary — no .so, no plugin.Open.
COPY core/ ./core/
COPY framework/ ./framework/
COPY transports/ ./transports/
COPY plugins/ ./plugins/

# Pull UI build output
COPY --from=ui-builder /app/out ./transports/bifrost-http/ui

# Set up go.work to use LOCAL core/framework for every module — required
# because providercooldown is not in any published release, and the
# transports/go.mod `replace` directive resolves providercooldown to a
# sibling directory which only exists in the workspace layout.
RUN go work init && \
    go work use ./core && \
    go work use ./framework && \
    go work use ./plugins/compat && \
    go work use ./plugins/governance && \
    go work use ./plugins/jsonparser && \
    go work use ./plugins/logging && \
    go work use ./plugins/maxim && \
    go work use ./plugins/mocker && \
    go work use ./plugins/otel && \
    go work use ./plugins/prompts && \
    go work use ./plugins/providercooldown && \
    go work use ./plugins/semanticcache && \
    go work use ./plugins/modelcatalogresolver && \
    go work use ./plugins/telemetry && \
    go work use ./transports

# Build bifrost-http with static SQLite linking. providercooldown is
# linked in as part of the main binary via the `replace` directive in
# transports/go.mod.
ARG VERSION=dev-build
RUN cd /build/transports && \
    go build \
      -ldflags="-w -s -X main.Version=v${VERSION} -extldflags '-static'" \
      -a -trimpath \
      -tags "sqlite_static" \
      -o /out/bifrost-http \
      ./bifrost-http

RUN test -f /out/bifrost-http

# ---- Stage 3: Runtime (minimal alpine) ----
# NOTE: --platform is intentionally omitted here. When building natively on
# x86_64, the runtime stage runs on the host arch (amd64) and the binary
# built by the builder stage (which runs on BUILDPLATFORM) is compatible.
# For a true arm64 image, use buildx:
#   docker buildx build --platform linux/arm64 --push ...
FROM alpine:3.23.4
ARG APK_REPOS=
WORKDIR /app

# Install runtime dependencies. musl + libgcc are needed for the CGO
# binary; ca-certificates for HTTPS to upstream providers; zlib for the
# HTTP body decompression path used by integrations.
RUN if [ -n "$APK_REPOS" ]; then \
      echo "$APK_REPOS" | tr ',' '\n' > /etc/apk/repositories; \
    fi && \
    apk upgrade --no-cache && \
    apk add --no-cache musl libgcc ca-certificates zlib

COPY --from=builder /out/bifrost-http /app/main
COPY transports/docker-entrypoint.sh /app/docker-entrypoint.sh

# Build arguments with sensible defaults — override at build time or
# override the resulting ENV at runtime.
ARG ARG_APP_PORT=8080
ARG ARG_APP_HOST=0.0.0.0
ARG ARG_LOG_LEVEL=info
ARG ARG_LOG_STYLE=json
ARG ARG_APP_DIR=/app/data

ENV APP_PORT=$ARG_APP_PORT \
    APP_HOST=$ARG_APP_HOST \
    LOG_LEVEL=$ARG_LOG_LEVEL \
    LOG_STYLE=$ARG_LOG_STYLE \
    APP_DIR=$ARG_APP_DIR

# Go runtime performance tuning (override at runtime for your workload).
# GOGC: GC target percentage. Higher = less frequent GC, more memory usage.
#       Default: 100. For high-throughput with available memory, try 200-400.
# GOMEMLIMIT: Soft memory limit for Go runtime. Set to ~90% of container memory limit.
#             Example: "1800MiB" for a 2GB container, "3600MiB" for 4GB.
# Note: GOMAXPROCS is automatically detected from cgroup CPU limits via automaxprocs.
ENV GOGC="" \
    GOMEMLIMIT=""

# adduser -D (no -u) on this alpine base assigns the first free UID, which is
# 1000 on a fresh image — so the numeric USER 1000:0 below resolves to appuser.
# Guard the group-writable chmod so it never applies when APP_DIR resolves to
# /app itself (e.g. --build-arg ARG_APP_DIR=/app), which would make the binary
# group-0-writable.
RUN mkdir -p "$APP_DIR/logs" && \
    chmod 777 "$APP_DIR" "$APP_DIR/logs" && \
    chmod +x /app/docker-entrypoint.sh

# Numeric non-root USER (nobody 65534) so kubelet can verify runAsNonRoot.
# The data directory is world-writable (chmod 777 above) so the container
# can create config.db and logs.db without a dedicated user account.
# When mounting a host volume at APP_DIR, ensure the host directory is
# writable by UID 65534 or override with --user 0.
USER 65534

# Declare volume for data persistence
VOLUME ["${APP_DIR}"]
EXPOSE $APP_PORT

# Health check using busybox wget. start-period gives the Go service time
# to run migrations and load the config store before the first probe fires.
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:${APP_PORT}/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/main"]
