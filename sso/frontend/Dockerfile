# syntax=docker/dockerfile:1.7

# ─────────────────────────────────────────────────────────────────────────────
# cotton-id frontend — multi-stage build
#
#   Stage 1 (build):   node:24 — `npm ci` + `npm run build` (Vite → /dist).
#   Stage 2 (runtime): nginx:alpine — serves the static SPA and reverse-proxies
#                      /api, /oauth, /healthz, /swagger to the backend so the
#                      browser sees a single same-origin host (contract §7).
#
# The nginx config is supplied by frontend/nginx.conf. If the frontend agent
# has not committed one yet, the copy of a sensible default in this repo is used
# (see frontend/nginx.conf authored alongside this Dockerfile, flagged in the
# infra report).
# ─────────────────────────────────────────────────────────────────────────────

# ---- Stage 1: build ---------------------------------------------------------
FROM node:24-alpine AS build

WORKDIR /app

# Dependency layer — cached unless the lockfile changes. `npm ci` requires a
# committed package-lock.json and installs exactly what it pins (reproducible).
COPY package.json package-lock.json* ./
RUN npm ci

# Source + build. Vite emits the production bundle to /app/dist.
COPY . .
RUN npx vite build

# ---- Stage 2: runtime -------------------------------------------------------
FROM nginx:alpine

LABEL org.opencontainers.image.title="cotton-id-frontend" \
      org.opencontainers.image.description="cotton-id identity provider SPA (nginx)" \
      org.opencontainers.image.source="https://github.com/jeroomjewelry/cotton-id"

# Replace the default site config with ours (SPA history fallback + API proxy).
RUN rm -f /etc/nginx/conf.d/default.conf
COPY nginx.conf /etc/nginx/conf.d/cotton-id.conf

# Static assets built in stage 1.
COPY --from=build /app/dist /usr/share/nginx/html

# nginx:alpine serves on 80; compose maps host 3000 → container 80.
EXPOSE 80

# nginx's own healthcheck-friendly endpoint is the served index; compose uses a
# wget against http://localhost/ to confirm the static server is up.
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD wget -q -O /dev/null http://localhost/ || exit 1

# Default nginx entrypoint/cmd run nginx in the foreground.
