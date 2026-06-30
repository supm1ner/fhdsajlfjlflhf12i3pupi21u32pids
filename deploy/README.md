# Sunrise — full-stack deployment (everything, one command)

Brings up the **entire product** with Docker Compose: SSO (cotton-id + Ory Hydra), the Sunrise
messenger backend, LiveKit (SFU group calls), and the web client.

```
postgres ─┬─ cottonid (cotton-id)         hydra ── OIDC engine (4444/4445)
          ├─ hydra    (Hydra)             backend ── cotton-id SSO (8080)
          └─ sunrise  (messenger)         frontend ── SSO UI (3000)
                                          sunrise ── messenger API (6060)
                                          livekit ── SFU (7880)
                                          web ── messenger client (8088)
```

## Prerequisites
- Docker + Docker Compose v2 (`docker compose`, not the legacy `docker-compose`).
- ~3–4 GB free RAM, ports 3000/4444/4445/6060/7880/7881/7882/8080/8088 free.

## Run it

```bash
cd deploy
cp .env.example .env          # then edit the "change me" secrets
docker compose up -d --build  # or: make up
# wait until healthy (make ps), then register the messenger SSO client:
./register-rp.sh              # or: make register
```

Open the messenger at **http://localhost:8088**.

| URL | What |
|---|---|
| http://localhost:8088 | Sunrise messenger (web client) |
| http://localhost:3000 | cotton-id SSO (sign-up / login / consent UI) |
| http://localhost:8080/healthz | cotton-id backend health |
| http://localhost:4444/.well-known/openid-configuration | Hydra OIDC discovery |
| http://localhost:6060 | Sunrise messenger backend |
| ws://localhost:7880 | LiveKit SFU |

## First-run E2E checklist
1. `make ps` — all services `healthy`/`running` (sunrise has no healthcheck label; check `make smoke`).
2. Open http://localhost:8088 → **register** (login + password) → you land in the messenger.
3. Open a second browser/profile, register a second user.
4. Find the other user via the **＋ (new chat)** search → send messages both ways (realtime).
5. Send a photo, a voice message, a **round video note**.
6. Start a **1:1 call** (📞/🎥), then **screen share** and **device settings** (⚙️).
7. Start a **group call** (👥) — uses LiveKit; both users see each other in the grid.
8. **SSO:** click **Sign in with SSO** → cotton-id login/consent → back into the messenger.

## How the OIDC wiring works (important)
- The browser does PKCE against Hydra's **public** URL (`VITE_OIDC_ISSUER=http://localhost:4444/`).
- Hydra's token `iss` is its **self-issuer** (`http://hydra:4444`), so the Sunrise backend validates
  `iss` and fetches JWKS at `http://hydra:4444/` (in-network) — set via `SUNRISE_OIDC_ISSUER` /
  `SUNRISE_OIDC_DISCOVERY_ISSUER`. The `discovery_issuer` split is why this works behind Docker.
- `register-rp.sh` registers `sunrise-messenger` as a public PKCE client with redirect
  `http://localhost:8088/`.

## Run without SSO (messenger only)
Login by login/password, no Hydra/cotton-id. Leave `SUNRISE_OIDC_ISSUER=` empty (the `oidc`
scheme auto-disables) and start only the messenger services:

```bash
docker compose up -d --build postgres sunrise livekit web
```
Public ports then: **8088, 6060, 7880, 7881 (TCP) + 7882 (UDP)** — no 3000/4444/4445/8080.

## Single port (one 443) with Caddy
The `edge` profile adds Caddy, fronting web + chat + LiveKit signaling on **one domain / port 443**:

```bash
# set CADDY_DOMAIN in .env (a real domain gets auto-TLS)
docker compose --profile edge up -d --build
```
Then only **443/TCP** is needed for the app, **plus LiveKit media `7881/TCP` + `7882/UDP`**
(WebRTC media can't be HTTP-proxied). SSO services are HTTP too — expose them as subdomains on the
same 443 (see commented blocks in `Caddyfile`).

## Production notes
- Replace EVERY `dev-change-me*` secret in `.env` (`openssl rand -hex 32`).
- Terminate TLS in front (Hydra needs https without `--dev`; WebRTC needs wss). Put a reverse proxy
  in front and set the `*_URL` / `VITE_*` / `WEBAUTHN_RP_ORIGINS` to your real https origins.
- LiveKit: set `rtc.use_external_ip: true` (and open the UDP port) for non-localhost clients.
- Don't expose Hydra admin (4445) publicly.

## ⚠️ Status / honesty
This kit was authored against the repo and the upstream images but has **not been run here** (the
build sandbox has no Docker daemon / Postgres server). Expect to iterate on first `up`:
- the Sunrise image builds the server **from source** (the upstream Dockerfile pulls a placeholder
  release URL), so the first build is slow;
- image tags (`oryd/hydra:v2.2.0`, `livekit/livekit-server:v1.7`) and the cotton-id Dockerfiles are
  used as-is — adjust if your registry differs;
- if a service fails health, `docker compose logs <svc>` shows why (usually a secret missing in
  `.env` or a port already in use).

Report any failing service's logs and I'll fix the config.
