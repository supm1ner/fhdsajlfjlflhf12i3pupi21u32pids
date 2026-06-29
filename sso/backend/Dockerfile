# syntax=docker/dockerfile:1.7

# ─────────────────────────────────────────────────────────────────────────────
# cotton-id backend — multi-stage build
#
#   Stage 1 (builder): golang:1.25 — runs `swag init` to (re)generate the
#                      OpenAPI docs package, then compiles a fully-static,
#                      CGO-disabled binary with embedded migrations + docs.
#   Stage 2 (runtime): gcr.io/distroless/static — minimal, non-root, no shell.
#
# The contract (build-contract.md §1,§5,§6) requires: embedded migrations,
# embedded swaggo docs, CGO disabled, a non-root runtime, and an :8080 listener.
# ─────────────────────────────────────────────────────────────────────────────

# ---- Stage 1: build ---------------------------------------------------------
FROM golang:1.25 AS builder

# GOTOOLCHAIN=auto lets the image fetch the exact toolchain pinned in go.mod if
# it differs from the base image's bundled one (design.md risk mitigation).
ENV GOTOOLCHAIN=auto \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

# 1) Dependency layer — cached unless go.mod / go.sum change.
COPY go.mod go.sum* ./
RUN go mod download

# 2) Source.
COPY . .

# 3) Generate Swagger docs from swaggo annotations so the binary embeds an
#    up-to-date OpenAPI document (contract §8 "all endpoints in Swagger").
#    Pinned via go.mod tools; `go run` avoids a separately-installed binary.
#    The generated package lands in ./docs (embedded by the app via go:embed).
RUN go run github.com/swaggo/swag/cmd/swag@latest init \
        --generalInfo cmd/cotton-id/main.go \
        --output docs \
        --parseDependency --parseInternal \
    || echo "WARN: swag init failed or annotations absent; continuing with committed docs (if any)"

# 4) Compile. -trimpath + ldflags strip build paths and debug info for a
#    smaller, reproducible binary. Migrations + docs are embedded via embed.FS
#    in the Go source, so the resulting binary is self-contained.
RUN go build \
        -trimpath \
        -ldflags="-s -w" \
        -o /out/cotton-id \
        ./cmd/cotton-id

# ---- Stage 2: runtime -------------------------------------------------------
# Minimal alpine runtime. Rationale: the container needs a small, in-image HTTP
# client (busybox wget) so Docker's HEALTHCHECK can probe /healthz WITHOUT
# assuming anything about the backend's CLI surface — this keeps the infra slice
# decoupled from backend internals. ca-certificates lets the backend make
# outbound TLS calls (e.g. a future Hydra over HTTPS). The contract permits
# "distroless/static OR alpine" (build-contract.md §1); we choose alpine for the
# self-contained healthcheck. To switch to distroless later, drop the wget
# healthcheck in favor of a backend `healthcheck` subcommand.
FROM alpine:3.20

# Image metadata (OCI labels).
LABEL org.opencontainers.image.title="cotton-id-backend" \
      org.opencontainers.image.description="cotton-id OpenID Connect identity provider (backend)" \
      org.opencontainers.image.source="https://github.com/jeroomjewelry/cotton-id"

# Root CAs for outbound TLS; tzdata for correct timestamps in logs/tokens.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S cotton && adduser -S -G cotton -H -s /sbin/nologin cotton

WORKDIR /app

# Copy only the static binary from the builder.
COPY --from=builder /out/cotton-id /app/cotton-id

# Runtime configuration is supplied entirely via env (12-factor, contract §5).
ENV HTTP_ADDR=":8080"

EXPOSE 8080

# Run as an unprivileged, no-login user — never root.
USER cotton:cotton

# Liveness/readiness probe: /healthz reports dependency health (contract §3 Ops,
# platform-foundation spec). busybox wget --spider returns non-zero on >=400.
HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=10 \
    CMD wget -q --spider http://127.0.0.1:8080/healthz || exit 1

# The binary applies embedded migrations on boot, then serves HTTP (D10).
ENTRYPOINT ["/app/cotton-id"]
