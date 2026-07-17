# Smart Food Manager — Monorepo

A single repository holding the **Go** backend API and the **React + TypeScript** frontend for a
food / nutrition / food-loss app. It pairs a nutrition-logging dashboard (with a draggable AI
companion) with AI-powered food analysis, and ships with a full local observability stack.

Highlights:

- **AI features** via Google Gemini — analyze a meal from text or a photo, extract nutrition facts
  from a product-label image, and generate nutrition advice / in-character companion messages.
- **Structured observability** — every request emits OpenTelemetry traces, metrics, and structured
  logs, exported to a local Grafana LGTM stack (Loki / Tempo / Prometheus / Grafana).
- **JWT auth + RBAC** middleware, a per-IP rate limiter, and PII-safe hashed identifiers in logs.

## Stack

| Layer         | Technology                                                            |
| ------------- | --------------------------------------------------------------------- |
| Frontend      | React 18 + TypeScript, Vite, Tailwind CSS, Recharts, lucide-react     |
| Backend       | Go 1.25, chi router, pgx (Postgres), golang-jwt                       |
| Database      | PostgreSQL 16                                                         |
| Auth          | JWT (HS256) + RBAC middleware                                         |
| AI            | Google Gemini (`generateContent`) via an instrumented HTTP client     |
| Observability | OpenTelemetry (OTLP/gRPC) → Collector → Loki, Tempo, Prometheus, Grafana |
| Infra         | Docker Compose (Postgres, Adminer, optional app + monitoring profiles) |

## Layout

```
smart-food-manager/
├── backend/                    Go API service
│   ├── cmd/api/                main entrypoint (graceful shutdown)
│   ├── internal/
│   │   ├── config/             env-based configuration
│   │   ├── server/             router + route wiring + CORS
│   │   ├── middleware/         JWT auth, RBAC, rate limiting, request logging
│   │   ├── handler/            health, auth, nutrients, nutrition, labels, telemetry
│   │   ├── gemini/             Google Gemini API client (AI features)
│   │   ├── logging/            structured event logging (bridged to OTel)
│   │   ├── telemetry/          OpenTelemetry setup + metric instruments
│   │   └── store/              pgx connection pool
│   ├── migrations/             SQL migrations (0001_init.sql = full schema)
│   ├── e2e/                    end-to-end API tests (build tag: e2e)
│   └── Dockerfile
├── frontend/                   React + TS (Vite + Tailwind)
│   ├── src/
│   │   ├── api/                fetch client + shared types
│   │   ├── components/         dashboard, stats, settings, AI companion, add-entry modal
│   │   ├── services/           nutrition analysis + companion messages
│   │   ├── telemetry/          batched frontend event logging
│   │   ├── types/              shared TS types
│   │   └── App.tsx, main.tsx   app shell + entry
│   └── Dockerfile
├── observability/              LGTM stack configs + provisioned Grafana dashboards
├── docker-compose.yml          Postgres + Adminer (+ `app` / `monitoring` profiles)
└── Makefile                    task runner (make help)
```

## Prerequisites

- Go 1.25+
- Node 20+
- Docker (for Postgres via Compose)
- A Google Gemini API key — **optional**; the AI endpoints return `503` until `GEMINI_API_KEY`
  is set, and the frontend falls back to local estimates so the UI stays usable without it.

## Configuration

The backend reads its config from environment variables and ships with sensible development
defaults (see [`backend/internal/config/config.go`](backend/internal/config/config.go)), so it
runs without any `.env`. The `Makefile` and `docker-compose.yml` both load a root `.env` when
present. There is no committed `.env.example`; create a git-ignored `.env` if you need to override
defaults — a working starting point:

```bash
# Database — Compose reads POSTGRES_*/DB_HOST_PORT; the backend reads DATABASE_URL.
POSTGRES_USER=app
POSTGRES_PASSWORD=app
POSTGRES_DB=foodapp
DB_HOST_PORT=5433                                              # host port for the Compose Postgres

# Native `make backend` connects to the Compose DB on DB_HOST_PORT (note: 5433, not 5432).
DATABASE_URL=postgres://app:app@localhost:5433/foodapp?sslmode=disable
JWT_SECRET=dev-secret-change-me
LOG_HASH_SALT=dev-salt-change-me

# OpenTelemetry — only needed when the monitoring stack is running (see below).
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
OTEL_EXPORTER_OTLP_INSECURE=true                              # plaintext collector; TLS is the default otherwise

# Gemini AI — leave blank to run without AI features.
GEMINI_API_KEY=
```

> **Port note:** `make db-up` exposes Postgres on host port **5433** by default (to avoid clashing
> with a local 5432 instance), while the backend's built-in `DATABASE_URL` default points at 5432.
> When running the API natively against the Compose database, set `DATABASE_URL` (or
> `DB_HOST_PORT=5432`) in `.env` so the two agree.

Other backend variables (all optional, with defaults): `PORT`, `JWT_EXPIRY_MINUTES`,
`RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`, `ALLOWED_ORIGIN`, `SERVICE_NAME`, `SERVICE_VERSION`,
`DEPLOYMENT_ENVIRONMENT`, `GEMINI_BASE_URL`, `GEMINI_MODEL`, `GEMINI_TIMEOUT_SECONDS`.
The frontend accepts `VITE_API_BASE_URL` (empty in dev — see the Vite proxy) and `VITE_APP_VERSION`.

## Quickstart

```bash
# 1. Start Postgres + Adminer (migrations in backend/migrations auto-run on first init)
make db-up

# 2. Backend — resolve deps once, then run
make tidy
make backend          # serves on http://localhost:8080

# 3. Frontend — in a second terminal
make install
make frontend         # serves on http://localhost:5173
```

Open http://localhost:5173. The page calls `/healthz` and `/api/v1/nutrients` through Vite's dev
proxy (see [`frontend/vite.config.ts`](frontend/vite.config.ts)), so the browser stays single-origin.
Adminer (DB browser) is at http://localhost:8081 (server `db`, user/password from `.env`).

To run the whole app in containers instead:

```bash
docker compose --profile app up --build
```

## Observability

Bring up the local LGTM stack, then run the backend (natively or via the `app` profile) so it
exports to the collector:

```bash
make obs-up           # OTel Collector + Loki + Tempo + Prometheus + Grafana
# ... run the backend with OTEL_EXPORTER_OTLP_ENDPOINT / _INSECURE set (see Configuration) ...
make obs-down         # tear the stack down and remove its volumes
```

- Grafana:    http://localhost:3000 (anonymous admin; provisioned datasources + dashboards)
- Prometheus: http://localhost:9090
- Tempo:      http://localhost:3200
- Loki:       http://localhost:3100

Telemetry is best-effort: if the collector is unreachable, the backend logs a warning and falls
back to stderr logging. See [`observability/README.md`](observability/README.md) for the logging
blueprint and dashboard details.

## Tests

```bash
make test                 # backend unit tests
make test-e2e             # end-to-end API tests (needs the db from `make db-up`)
make test-e2e-frontend    # Playwright UI tests (boots Vite itself; no backend needed)
```

Run `make help` for the full list of targets (also: `migrate`, `build`, `clean`).

## API

Base path `/api/v1`. AI endpoints (marked ¹) require `GEMINI_API_KEY` and return `503` when it is unset.

| Method | Path                       | Auth             | Notes                                             |
| ------ | -------------------------- | ---------------- | ------------------------------------------------- |
| GET    | `/healthz`                 | none             | Liveness + DB ping                                |
| POST   | `/api/v1/auth/login`       | none             | **Demo stub** — issues a token for any non-empty email |
| GET    | `/api/v1/nutrients`        | none             | Lists the active nutrient master                  |
| POST   | `/api/v1/telemetry/logs`   | optional Bearer  | Frontend telemetry sink; a token binds events to the user |
| POST   | `/api/v1/nutrition/analyze`| optional Bearer  | AI food analysis from text or an image ¹          |
| GET    | `/api/v1/me`               | Bearer           | Returns the caller's claims                       |
| POST   | `/api/v1/nutrients/advice` | Bearer           | AI nutrition advice ¹                             |
| POST   | `/api/v1/foods/from-label` | Bearer           | Extract nutrients from a label image and save a food ¹ |
| GET    | `/api/v1/admin/ping`       | Bearer + `admin` | RBAC example                                      |

Try the protected route:

```bash
TOKEN=$(curl -s localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"x"}' | sed 's/.*"token":"//;s/".*//')
curl localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
```

## Before shipping

- Replace the login stub in [`backend/internal/handler/auth.go`](backend/internal/handler/auth.go)
  with a real lookup against the `users` table and password-hash verification.
- Move the JWT secret and `LOG_HASH_SALT` out of defaults into managed secrets, and set a real
  `GEMINI_API_KEY`.
- Swap the in-memory rate limiter for a shared (e.g. Redis) limiter once you run more than one
  backend instance behind the API gateway.
- Add migration tooling (e.g. `golang-migrate`) for versioned, repeatable migrations beyond the
  Compose auto-init.
