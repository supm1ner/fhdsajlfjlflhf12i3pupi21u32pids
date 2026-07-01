# Sunrise messenger server — built from source (the upstream Dockerfile pulls a
# release tarball from a placeholder URL, so we compile instead).
#
# Build context MUST be the repository root:
#   docker build -f deploy/sunrise/Dockerfile -t sunrise-server .
# (docker compose in deploy/ sets context: .. accordingly.)

# ---- Build stage -------------------------------------------------------------
FROM golang:1.24-alpine AS build
RUN apk add --no-cache git build-base
WORKDIR /src
# Module files first for layer caching.
COPY chat/go.mod chat/go.sum ./chat/
WORKDIR /src/chat
RUN go mod download
WORKDIR /src
COPY chat/ ./chat/
WORKDIR /src/chat
# The server binary is "sunrise"; the schema initializer is "init-db".
RUN CGO_ENABLED=0 go build -tags postgres -o /out/sunrise ./server \
 && CGO_ENABLED=0 go build -tags postgres -o /out/init-db ./sunrise-db

# ---- Runtime stage -----------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache bash grep ca-certificates netcat-openbsd
WORKDIR /opt/tinode

COPY --from=build /out/sunrise /out/init-db ./
COPY chat/static ./static
COPY chat/sunrise-db/data.json chat/sunrise-db/credentials.sh ./
COPY chat/templ ./templ
COPY deploy/sunrise/config.template deploy/sunrise/ice-servers.json ./
COPY chat/docker/sunrise/entrypoint.sh ./entrypoint.sh
RUN mkdir -p /botdata uploads && chmod +x entrypoint.sh credentials.sh

# Defaults referenced by config.template (override via compose `environment:`).
# Booleans/numerics MUST be non-empty or the generated JSON is invalid.
ENV STORE_USE_ADAPTER=postgres \
    TARGET_DB=postgres \
    SAMPLE_DATA= \
    DEFAULT_COUNTRY_CODE=US \
    SERVER_STATUS_PATH= \
    MEDIA_HANDLER=fs \
    FS_CORS_ORIGINS='["*"]' \
    TLS_ENABLED=false \
    TLS_DOMAIN_NAME= \
    TLS_CONTACT_ADDRESS= \
    EMAIL_VERIFICATION_REQUIRED= \
    SMTP_HOST_URL='http://localhost:6060' \
    SMTP_SERVER= \
    SMTP_PORT= \
    SMTP_SENDER= \
    SMTP_LOGIN= \
    SMTP_PASSWORD= \
    SMTP_AUTH_MECHANISM= \
    SMTP_HELO_HOST= \
    SMTP_DOMAINS= \
    DEBUG_EMAIL_VERIFICATION_CODE= \
    ACC_GC_ENABLED=false \
    WEBRTC_ENABLED=true \
    ICE_SERVERS_FILE=./ice-servers.json \
    API_KEY_SALT='T713/rYYgW7g4m3vG6zGRh7+FM1t0T8j13koXScOAj4=' \
    AUTH_TOKEN_KEY='wfaY2RgF2S1OQI/ZlK+LSrp1KB2jwAdGAIHQ7JZn+Kc=' \
    UID_ENCRYPTION_KEY='la6YsO+bNX/+XIkOqc5Svw==' \
    OIDC_ISSUER='http://localhost:4444/' \
    OIDC_DISCOVERY_ISSUER='http://hydra:4444/' \
    OIDC_CLIENT_ID='sunrise-messenger' \
    NO_DB_INIT=false \
    RESET_DB=false \
    UPGRADE_DB=false \
    CLUSTER_SELF= \
    PPROF_URL= \
    EXT_STATIC_DIR= \
    PLUGIN_PYTHON_CHAT_BOT_ENABLED=false

HEALTHCHECK --interval=30s --timeout=3s --start-period=40s \
  CMD nc -z localhost 6060 || exit 1

EXPOSE 6060 16060
ENTRYPOINT ["./entrypoint.sh"]
