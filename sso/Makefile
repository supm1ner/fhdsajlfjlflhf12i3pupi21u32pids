# ─────────────────────────────────────────────────────────────────────────────
# cotton-id — root Makefile
#
# Thin wrappers over Docker Compose and the backend/frontend toolchains so the
# whole stack is driveable with simple verbs. Compose lives in deploy/.
#
# Usage: `make help` lists targets. Most targets shell out to `docker compose`
# using the deploy/docker-compose.yml file and deploy/.env (if present).
# ─────────────────────────────────────────────────────────────────────────────

# Use a single shell per recipe and fail on errors/unset vars/pipe failures.
SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

# ---- Paths / tools ----------------------------------------------------------
COMPOSE_FILE := deploy/docker-compose.yml
ENV_FILE     := deploy/.env
# Include the env file only if it exists (so `make` works on a clean checkout).
ENV_FLAG     := $(shell test -f $(ENV_FILE) && echo "--env-file $(ENV_FILE)")
COMPOSE      := docker compose -f $(COMPOSE_FILE) $(ENV_FLAG)

BACKEND_DIR  := backend
FRONTEND_DIR := frontend

# Profiles: dev (seed) is on by default for local use; observability is opt-in.
DEV_PROFILE  := --profile dev

.PHONY: help up down logs build test test-backend test-frontend \
        swagger migrate fmt lint seed ps restart clean observability \
        backend-shell e2e

## help: Show this help.
help:
	@echo "cotton-id — make targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //' | awk -F': ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ---- Stack lifecycle --------------------------------------------------------

## up: Build images and start the full stack (incl. dev seed) in the background.
up:
	$(COMPOSE) $(DEV_PROFILE) up -d --build

## down: Stop and remove the stack containers and network (keeps named volumes).
down:
	$(COMPOSE) $(DEV_PROFILE) down

## clean: Down + remove named volumes (DESTROYS all DB data). Irreversible.
clean:
	$(COMPOSE) $(DEV_PROFILE) down -v

## restart: Recreate the stack (down then up).
restart: down up

## ps: Show stack container status.
ps:
	$(COMPOSE) $(DEV_PROFILE) ps

## logs: Tail logs from all services (Ctrl-C to stop).
logs:
	$(COMPOSE) $(DEV_PROFILE) logs -f --tail=100

## observability: Start the optional observability stack (Prometheus + Grafana).
observability:
	$(COMPOSE) --profile observability up -d

# ---- Build ------------------------------------------------------------------

## build: Build the backend and frontend images via Compose (no cache reuse off).
build:
	$(COMPOSE) build backend frontend

# ---- Database / migrations --------------------------------------------------

## migrate: Apply DB migrations. The backend applies its embedded migrations on
##          boot; this target restarts it so a fresh migration set is applied.
migrate:
	$(COMPOSE) up -d --build backend
	@echo "Backend restarted; embedded migrations applied on boot. Check 'make logs'."

## seed: Run the dev seed one-shot (demo admin user + demo relying-party client).
seed:
	$(COMPOSE) $(DEV_PROFILE) run --rm seed

# ---- Swagger / docs ---------------------------------------------------------

## swagger: Regenerate OpenAPI docs from swaggo annotations into backend/docs.
swagger:
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go run github.com/swaggo/swag/cmd/swag@latest init \
		--generalInfo cmd/cotton-id/main.go --output docs --parseDependency --parseInternal

# ---- Tests ------------------------------------------------------------------

## test: Run backend and frontend test suites.
test: test-backend test-frontend

## test-backend: Run Go unit/integration tests.
test-backend:
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go test ./... -race -count=1

## test-frontend: Run frontend tests (Vitest) if a frontend package is present.
test-frontend:
	@if [ -f $(FRONTEND_DIR)/package.json ]; then \
		cd $(FRONTEND_DIR) && npm ci && npm test --if-present; \
	else \
		echo "no frontend/package.json yet; skipping frontend tests"; \
	fi

## e2e: Run the end-to-end smoke against the RUNNING stack (start it with `make
##      up` first). Exercises CSRF → signup → login → reset, the OIDC RP flow,
##      providers, passkey begin, account gate, admin RBAC, and the audit Journal.
##      Exits non-zero on any failed step. Reads ADMIN_API_KEY from deploy/.env.
e2e:
	sh scripts/e2e-smoke.sh

# ---- Formatting / linting ---------------------------------------------------

## fmt: Format Go code (gofmt) and frontend code (prettier, if present).
fmt:
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto gofmt -w .
	@if [ -f $(FRONTEND_DIR)/package.json ]; then \
		cd $(FRONTEND_DIR) && npm run format --if-present; \
	fi

## lint: Vet Go code and lint the frontend (if present).
lint:
	cd $(BACKEND_DIR) && GOTOOLCHAIN=auto go vet ./...
	@if [ -f $(FRONTEND_DIR)/package.json ]; then \
		cd $(FRONTEND_DIR) && npm run lint --if-present; \
	fi

# ---- Convenience ------------------------------------------------------------

## backend-shell: Open a shell in the running backend container.
backend-shell:
	$(COMPOSE) exec backend /bin/sh
