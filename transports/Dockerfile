# transports/Dockerfile - single source of truth for all pg-gateway container builds.
#
# Build-args:
#   USE_LOCAL_MODULES=1   use go.work + local source tree (pre-release CI, local dev)
#   (empty or 0)          use transports/go.mod + published modules (release)
#   VERSION               version string baked into the binary via -X main.Version
#
# Supports linux/amd64 + linux/arm64 from any buildx host:
#   - All builder stages pin --platform=$BUILDPLATFORM (native host arch)
#   - Cross-compilation uses GOARCH=$TARGETARCH + aarch64-linux-gnu-gcc for arm64
#   - Pure static linking via -extldflags -static (binary runs on musl Alpine without glibc)
#
# Builder uses Debian bookworm (not alpine) because alpine's repos lack a
# glibc/musl aarch64 cross-toolchain — gcc-aarch64-linux-gnu is the simplest
# source of aarch64 cross-gcc that produces binaries compatible with Alpine musl
# runtime when fully statically linked. Runtime stays on alpine for size.

ARG VERSION=unknown
ARG USE_LOCAL_MODULES=

# --- UI Build Stage (native host arch via BUILDPLATFORM) ---
FROM --platform=$BUILDPLATFORM node:25-alpine3.23@sha256:cf38e1f3c28ac9d81cdc0c51d8220320b3b618780e44ef96a39f76f7dbfef023 AS ui-builder
WORKDIR /app
COPY ui/package*.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build-enterprise

# --- Go Build Stage (native host arch, cross-compile to TARGETARCH) ---
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
WORKDIR /build
ARG TARGETARCH
ARG USE_LOCAL_MODULES

# On native build (target == host arch) gcc itself works.
# On amd64 host cross-compiling to arm64, install gcc-aarch64-linux-gnu.
# On arm64 host cross-compiling to amd64, install gcc-x86-64-linux-gnu.
# binutils-<arch>-linux-gnu supplies the matching linker/objcopy/readelf.
# libc6-dev-<arch>-cross supplies the cross libc headers (otherwise gcc
# fails with "bits/libc-header-start.h: No such file or directory").
RUN if [ "$TARGETARCH" = "arm64" ]; then \
      apt-get update -qq && \
      apt-get install -y --no-install-recommends \
        gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu libc6-dev-arm64-cross; \
    elif [ "$TARGETARCH" = "amd64" ]; then \
      apt-get update -qq && \
      apt-get install -y --no-install-recommends \
        gcc-x86-64-linux-gnu binutils-x86-64-linux-gnu libc6-dev-amd64-cross; \
    else \
      echo "Unsupported TARGETARCH: $TARGETARCH" && exit 1; \
    fi && \
    rm -rf /var/lib/apt/lists/*

ENV CGO_ENABLED=1 GOOS=linux GOARCH=$TARGETARCH

# Module source selection:
#   USE_LOCAL_MODULES=1 -> go.work with ./core ./framework ./plugins/* ./transports
#                          (for pre-release CI / local builds before modules are published)
#   empty/0             -> use transports/go.mod only (release builds using published versions)
#
# Declare all go.mod/go.sum files UP-FRONT (they change rarely) so their COPY
# + `go mod download` layer in the build cache survives source-only edits. Only
# after deps are fetched do we COPY the full source tree — a source change then
# rebuilds just the changed packages instead of re-downloading every dependency.
COPY core/go.mod core/go.sum ./core/
COPY framework/go.mod framework/go.sum ./framework/
COPY plugins/compat/go.mod plugins/compat/go.sum ./plugins/compat/
COPY plugins/governance/go.mod plugins/governance/go.sum ./plugins/governance/
COPY plugins/jsonparser/go.mod plugins/jsonparser/go.sum ./plugins/jsonparser/
COPY plugins/logging/go.mod plugins/logging/go.sum ./plugins/logging/
COPY plugins/maxim/go.mod plugins/maxim/go.sum ./plugins/maxim/
COPY plugins/mocker/go.mod plugins/mocker/go.sum ./plugins/mocker/
COPY plugins/modelcatalogresolver/go.mod plugins/modelcatalogresolver/go.sum ./plugins/modelcatalogresolver/
COPY plugins/otel/go.mod plugins/otel/go.sum ./plugins/otel/
COPY plugins/prompts/go.mod plugins/prompts/go.sum ./plugins/prompts/
COPY plugins/providercooldown/go.mod plugins/providercooldown/go.sum ./plugins/providercooldown/
COPY plugins/semanticcache/go.mod plugins/semanticcache/go.sum ./plugins/semanticcache/
COPY plugins/rtk/go.mod plugins/rtk/go.sum ./plugins/rtk/
COPY plugins/telemetry/go.mod plugins/telemetry/go.sum ./plugins/telemetry/
COPY transports/go.mod transports/go.sum ./transports/

RUN if [ "$USE_LOCAL_MODULES" = "1" ]; then \
      go work init && \
      go work use \
        ./core \
        ./framework \
        ./plugins/compat \
        ./plugins/governance \
        ./plugins/jsonparser \
        ./plugins/logging \
        ./plugins/maxim \
        ./plugins/mocker \
        ./plugins/modelcatalogresolver \
        ./plugins/otel \
        ./plugins/prompts \
        ./plugins/providercooldown \
        ./plugins/rtk \
        ./plugins/semanticcache \
        ./plugins/telemetry \
        ./transports && \
      go mod download; \
    else \
      cd transports && go mod download; \
    fi

COPY core/ ./core/
COPY framework/ ./framework/
COPY plugins/ ./plugins/
COPY transports/ ./transports/

COPY --from=ui-builder /app/out ./transports/pg-gateway-http/ui

ARG VERSION=unknown
RUN cd /build/transports && \
    CC=$(if [ "$TARGETARCH" = "arm64" ]; then echo aarch64-linux-gnu-gcc; \
         elif [ "$TARGETARCH" = "amd64" ]; then echo x86_64-linux-gnu-gcc; \
         else echo "unknown"; fi) \
    go build \
      -ldflags="-w -s -X main.Version=v${VERSION} -extldflags '-static'" \
      -trimpath \
      -tags "sqlite_static" \
      -o /app/main \
      ./pg-gateway-http
RUN test -f /app/main || (echo "Build failed" && exit 1)

# --- Runtime Stage: Alpine ---
FROM alpine:3.23.4@sha256:5b10f432ef3da1b8d4c7eb6c487f2f5a8f096bc91145e68878dd4a5019afde11
WORKDIR /app

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

# Go runtime performance tuning (override at runtime for your workload)
# GOGC: GC target percentage. Default: 100. Higher = less frequent GC, more memory usage.
# GOMEMLIMIT: Soft memory limit for Go runtime. When set, Go is more aggressive about GC
# as it approaches this memory. Note: GOMAXPROCS is auto-detected via automaxprocs.
ENV GOGC="" \
    GOMEMLIMIT=""

RUN apk upgrade --no-cache && \
    apk add --no-cache musl libgcc ca-certificates zlib=1.3.2-r0

COPY --from=builder /app/main .
COPY --from=builder /build/transports/docker-entrypoint.sh .

# adduser -D (no -u) on this alpine base assigns the first free UID, which is
# 1000 on a fresh image — so the numeric USER 1000:0 below resolves to appuser.
# Guard the group-writable chmod so it never applies when APP_DIR resolves to
# /app itself (e.g. --build-arg ARG_APP_DIR=/app), which would make the binary
# group-0-writable.
RUN mkdir -p "$APP_DIR/logs" && \
    adduser -D -s /bin/sh appuser && \
    chown -R appuser:appuser /app && \
    chown -R appuser:0 "$APP_DIR" && \
    { [ "$APP_DIR" = "/app" ] || chmod -R g=rwX "$APP_DIR"; } && \
    chmod +x /app/docker-entrypoint.sh
# Numeric USER (uid:gid) so kubelet can verify runAsNonRoot; group 0 grants
# write access to the group-0-owned data dir under arbitrary OpenShift UIDs.
USER 1000:0

VOLUME ["${APP_DIR}"]
EXPOSE $APP_PORT

# Uses busybox wget (built into alpine) instead of the standalone wget package
# to avoid CVE-2025-69194; busybox wget supports -q/-O but not --no-verbose/--tries.
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:${APP_PORT}/health || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/main"]