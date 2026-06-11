# Contributing to Agent Nexus

Thank you for your interest in contributing. This document covers how to get the project running locally and the conventions we follow.

---

## Prerequisites

| Tool | Version |
|------|---------|
| Go | 1.26+ |
| Node.js | 20+ |
| Docker + Docker Compose v2 | 24+ |

---

## Local Setup

```bash
# 1. Clone the repo
git clone https://github.com/deepaksingh/agent-nexus.git
cd agent-nexus

# 2. Create environment files
cp services/api/.env.example services/api/.env
# Edit services/api/.env — fill in JWT_SECRET and ENCRYPTION_KEY at minimum.
# Optional: set PUBLIC_APP_URL=http://localhost:3000 (default) so Nexus AI
# generates correct links. Set to your public domain when deploying.

# 3. Start everything (Postgres in Docker, API and web with hot-reload)
make dev
```

Individual targets:

```bash
make postgres   # start Postgres only
make api        # start Go API only
make web        # start Next.js only
make stop       # stop all services
make logs       # tail logs
```

The API runs on `http://localhost:8080` and the web app on `http://localhost:3000`.

---

## Running the Full Stack via Docker

```bash
cp infra/.env.example infra/.env
# Edit infra/.env
cd infra && docker compose up -d
```

---

## Go Code Standards

- Use `pgx/v5` directly — no ORM
- Use `chi` for routing — no gin, echo, or fiber
- Propagate `context.Context` through all DB and HTTP calls
- Structured logging with `log/slog`
- No `panic` in HTTP handlers — return typed errors via `pkg/errs`
- Wrap errors: `fmt.Errorf("context: %w", err)`
- One concern per file — keep handlers thin, logic in services/repositories
- Table-driven tests for business logic
- All secrets via environment variables, never hardcoded

---

## TypeScript / Next.js Standards

- App router only — no pages router
- Server components by default; `"use client"` only where interaction requires it
- All API calls go through the fetch wrapper in `src/lib/api.ts`
- Server state: TanStack Query (`useQuery`, `useMutation`)
- Client-only state: Zustand
- UI components: shadcn/ui — avoid writing raw HTML forms
- All shared types in `src/types/index.ts`

---

## Pull Request Process

1. Branch from `main`: `git checkout -b feat/your-feature`
2. Make focused, reviewable commits — one logical change per commit
3. Open a PR with the [pull request template](.github/pull_request_template.md) filled in
4. PRs are squash-merged into `main`

Keep PRs small. A 200-line PR gets reviewed in minutes; a 2000-line PR sits in the queue.

---

## Reporting Issues

Use the GitHub issue templates:
- **Bug report** — reproduction steps, expected vs actual behaviour, versions
- **Feature request** — problem statement, proposed solution

For security vulnerabilities, see [SECURITY.md](SECURITY.md) — please do not use public issues.
