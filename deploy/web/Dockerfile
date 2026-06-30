# Sunrise web client (Svelte) — built with Bun, served by nginx.
# Build context MUST be the repository root.
#   docker build -f deploy/web/Dockerfile -t sunrise-web .

# ---- Build stage -------------------------------------------------------------
FROM oven/bun:1 AS build
WORKDIR /app
COPY webapp-svelte/package.json webapp-svelte/bun.lock ./
RUN bun install --frozen-lockfile
COPY webapp-svelte/ ./

# VITE_* are baked at build time. Override with --build-arg or compose build.args.
ARG VITE_HOST=localhost:6060
ARG VITE_API_KEY=AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K
ARG VITE_TLS=false
ARG VITE_APP_NAME=Sunrise
ARG VITE_OIDC_ISSUER=http://localhost:4444/
ARG VITE_OIDC_CLIENT_ID=sunrise-messenger
ARG VITE_OIDC_SCOPES="openid profile email"
ARG VITE_OIDC_REDIRECT=http://localhost:8088/
ENV VITE_HOST=$VITE_HOST \
    VITE_API_KEY=$VITE_API_KEY \
    VITE_TLS=$VITE_TLS \
    VITE_APP_NAME=$VITE_APP_NAME \
    VITE_OIDC_ISSUER=$VITE_OIDC_ISSUER \
    VITE_OIDC_CLIENT_ID=$VITE_OIDC_CLIENT_ID \
    VITE_OIDC_SCOPES=$VITE_OIDC_SCOPES \
    VITE_OIDC_REDIRECT=$VITE_OIDC_REDIRECT
RUN bun run build

# ---- Runtime stage -----------------------------------------------------------
FROM nginx:1.27-alpine
COPY deploy/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
