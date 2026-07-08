.PHONY: dev postgres api web stop logs migrate docker docker-up up down help screenshots

# ─── config ──────────────────────────────────────────────────────────────────
COMPOSE     := docker compose -f infra/docker-compose.yml
API_DIR     := services/api
WEB_DIR     := apps/web
API_ENV     := $(API_DIR)/.env

# ─── default ─────────────────────────────────────────────────────────────────
help:
	@echo ""
	@echo "  make up        build + start the FULL stack in Docker (postgres, api, runner, web)"
	@echo "  make down      stop and remove the full Docker stack"
	@echo "  make dev       start postgres + api + web (local dev, hot-reload)"
	@echo "  make postgres  start postgres in Docker (if not already running)"
	@echo "  make api       start Go API locally (reads $(API_ENV))"
	@echo "  make web       start Next.js dev server"
	@echo "  make stop      stop Postgres Docker container"
	@echo "  make logs      tail Docker Compose logs"
	@echo "  make migrate   apply pending migrations (restarts API container)"
	@echo "  make docker    run everything in Docker (full stack)"
	@echo "  make docker-up rebuild images (no cache) + start full stack"
	@echo ""

# ─── up: one command, everything in Docker ───────────────────────────────────
up:
	@if [ ! -f infra/.env ]; then \
		echo "→ infra/.env not found — creating from example (edit it for real secrets)"; \
		cp infra/.env.example infra/.env; \
	fi
	$(COMPOSE) --env-file infra/.env up -d --build
	@echo ""
	@echo "✓ Full stack running:"
	@echo "    web     http://localhost:3000"
	@echo "    api     http://localhost:8080"
	@echo "    runner  internal (http://runner:8092, executor: $${RUNNER_EXECUTOR:-stub})"
	@echo ""
	@echo "  Jira→PR pipeline setup: see docs/jira-pipeline.md"

down:
	$(COMPOSE) --env-file infra/.env down
	@echo "✓ Stack stopped"

# ─── full local dev ──────────────────────────────────────────────────────────
dev: postgres
	@echo "→ Starting API and Web in parallel…"
	@$(MAKE) -j2 api web

# ─── postgres ────────────────────────────────────────────────────────────────
postgres:
	@if docker ps --format '{{.Names}}' | grep -q '^agent-nexus-postgres$$'; then \
		echo "✓ Postgres already running"; \
	else \
		echo "→ Starting Postgres…"; \
		$(COMPOSE) up postgres -d; \
		echo "  Waiting for Postgres to be healthy…"; \
		until docker exec agent-nexus-postgres pg_isready -U nexus -d agent_nexus -q 2>/dev/null; do \
			sleep 1; \
		done; \
		echo "✓ Postgres ready"; \
	fi

# ─── api ─────────────────────────────────────────────────────────────────────
api:
	@if [ ! -f "$(API_ENV)" ]; then \
		echo "✗ Missing $(API_ENV). Copy from example:"; \
		echo "    cp $(API_DIR)/.env.example $(API_ENV)"; \
		exit 1; \
	fi
	@if lsof -ti:8080 >/dev/null 2>&1; then \
		echo "✓ API already running on :8080"; \
	else \
		echo "→ Starting Go API…"; \
		cd $(API_DIR) && export $$(grep -v '^#' .env | grep -v '^$$' | xargs) && go run ./cmd/server; \
	fi

# ─── web ─────────────────────────────────────────────────────────────────────
web:
	@if lsof -ti:3000 >/dev/null 2>&1; then \
		echo "✓ Web already running on :3000"; \
	else \
		echo "→ Starting Next.js dev server…"; \
		cd $(WEB_DIR) && npm run dev; \
	fi

# ─── docker (full stack) ─────────────────────────────────────────────────────
docker:
	$(COMPOSE) up -d
	@echo "✓ Stack running — http://localhost:3000"

# ─── docker-up (rebuild + start) ─────────────────────────────────────────────
docker-up:
	$(COMPOSE) build --no-cache
	$(COMPOSE) up -d
	@echo "✓ app running at http://localhost:$${WEB_PORT:-3000}"
	@echo "✓ admin at http://localhost:$${WEB_PORT:-3000}/admin/"
	@echo "✓ api at http://localhost:$${API_PORT:-8080}"

# ─── stop ────────────────────────────────────────────────────────────────────
stop:
	@echo "→ Stopping Postgres…"
	$(COMPOSE) stop postgres
	@echo "✓ Done"

# ─── migrate ─────────────────────────────────────────────────────────────────
migrate:
	@echo "→ Restarting API to apply pending migrations…"
	@$(COMPOSE) restart api
	@echo "✓ Done — check logs with: make logs"

# ─── logs ────────────────────────────────────────────────────────────────────
logs:
	$(COMPOSE) logs -f

# ─── screenshots (refresh docs/screenshots with the current UI) ──────────────
# One-time: cd apps/web && npm i -D playwright && npx playwright install chromium
# Then:     NEXUS_EMAIL=you@example.com NEXUS_PASSWORD=•••• make screenshots
#           (or NEXUS_TOKEN=<jwt> make screenshots). Skips code-repo pages.
screenshots:
	cd apps/web && node scripts/capture-screenshots.mjs
