<<<<<<< HEAD
# smart-food-manager
=======
# Food / Nutrition / Food-Loss — Monorepo

A single repository holding the **Go** backend API and the **React + TypeScript** frontend for the food / nutrition / food-loss app. Postgres is the database; auth is JWT with role-based access control (RBAC).

## Stack

| Layer    | Technology                                              |
| -------- | ------------------------------------------------------- |
| Frontend | React 18 + TypeScript, Vite                             |
| Backend  | Go 1.22, chi router, pgx (Postgres), golang-jwt         |
| Database | PostgreSQL 16                                           |
| Auth     | JWT (HS256) + RBAC middleware                           |
| Infra    | Docker Compose (Postgres, Adminer, optional app images) |

## Layout

```
food-app/
├── backend/                 Go API service
│   ├── cmd/api/             main entrypoint (graceful shutdown)
│   ├── internal/
│   │   ├── config/          env-based configuration
│   │   ├── server/          router + route wiring + CORS
│   │   ├── middleware/      JWT auth, RBAC, rate limiting
│   │   ├── handler/         health, auth, nutrients
│   │   └── store/           pgx connection pool
│   ├── migrations/          SQL migrations (0001_init.sql = full schema)
│   └── Dockerfile
├── frontend/                React + TS (Vite)
│   ├── src/
│   │   ├── api/             fetch client + shared types
│   │   ├── App.tsx          sample screen wired to the API
│   │   └── main.tsx
│   └── Dockerfile
├── docker-compose.yml       Postgres + Adminer (+ optional app profile)
├── Makefile                 task runner (make help)
└── .env.example             copy to .env
```

## Prerequisites

- Go 1.22+
- Node 20+
- Docker (for Postgres via Compose)

## Quickstart

```bash
cp .env.example .env

# 1. Start Postgres (migrations in backend/migrations auto-run on first init)
make db-up

# 2. Backend — resolve deps once, then run
make tidy
make backend          # serves on http://localhost:8080

# 3. Frontend — in a second terminal
make install
make frontend         # serves on http://localhost:5173
```

Open http://localhost:5173. The page calls `/healthz` and `/api/v1/nutrients`
through Vite's dev proxy, so the browser stays single-origin.
Adminer (DB browser) is at http://localhost:8081 (server `db`, user/password from `.env`).

To run everything in containers instead:

```bash
docker compose --profile app up --build
```

## API

| Method | Path                    | Auth         | Notes                                   |
| ------ | ----------------------- | ------------ | --------------------------------------- |
| GET    | `/healthz`              | none         | Liveness + DB ping                      |
| POST   | `/api/v1/auth/login`    | none         | **Demo stub** — issues a token          |
| GET    | `/api/v1/nutrients`     | none         | Lists active nutrients                  |
| GET    | `/api/v1/me`            | Bearer       | Returns the caller's claims             |
| GET    | `/api/v1/admin/ping`    | Bearer + `admin` | RBAC example                        |

Try the protected route:

```bash
TOKEN=$(curl -s localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"x"}' | sed 's/.*"token":"//;s/".*//')
curl localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
```

## Before shipping

- Replace the login stub in `backend/internal/handler/auth.go` with a real
  lookup against the `users` table and password-hash verification.
- Move the JWT secret out of defaults into a managed secret.
- Swap the in-memory rate limiter for a shared (e.g. Redis) limiter once you run
  more than one backend instance behind the API gateway.
- Add migration tooling (e.g. `golang-migrate`) for versioned, repeatable
  migrations beyond the Compose auto-init.
>>>>>>> a8d3cd7 (initial commit)
