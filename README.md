# Smart Food Manager

**Make a way of eating that is kind to both the planet and yourself an everyday habit.**

An app that loosely visualizes daily meals and household food inventory, so people can eat
better and waste less without taking on a second job to do it.

> **This is an issue-driven project, and this README is written to show that.**
> It was not built feature-first. It started from research into why health-management apps
> get abandoned and where household food waste actually comes from, turned that into a
> stated issue and an insight, and only then into a schema, an API, and a UI.
> The sections below follow that chain in order — **research → issue → insight → concept →
> shipped code** — and end with an honest account of what is *not* built yet.
> If you are evaluating the engineering rather than the reasoning, jump to
> [Stack](#stack) or [Quickstart](#quickstart).

---

## Demo

**AI nutrient analysis.** A visitor takes *continue as guest*, scans the bundled sample meal,
and gets a review screen rather than a verdict: four separate foods, each with editable
nutrients, an estimated expiry date, and a storage location. Confirming writes all four to
today's intake **and** to the pantry in one action — the two payoffs from one photo that
[§4](#4-from-issue-to-implementation) argues for — and the day's total, the recent-log list,
and the weekly stats fill in behind it.

https://github.com/user-attachments/assets/7f783abd-e608-4827-9fc3-e6fe85a7f5fb

**The environmental cost of food waste.** The larder opens with its footprint at zero.
Discarding 250 g of rice and beans — reason: *forgot it was there*, one of the seven the
picker offers — turns that panel into **1.11 kg CO₂e**, **635 L of virtual water**, and
**0.019 urban tree-years**, attributed back to the item that caused it. The numbers are
computed at read time from published category factors, not stored
([§6](#6-food-loss-instrumentation-the-most-opinionated-part-of-the-schema)).

https://github.com/user-attachments/assets/d271a020-a509-4a1e-9237-8bc59d2b9981

---

## 1. Research: what the numbers said

Three findings shaped the product. They are working estimates used to *size* the problem
and choose a direction — not verified primary-source claims, and the app never presents
them to users as fact.

**Health apps are abandoned fast.** Retention benchmarks for the Health & Fitness category
sit at roughly **D1 28% / D7 18% / D30 8.5%**. Any design here has to assume most people
churn within a month of installing — so retention is a design constraint, not a growth
task bolted on later.

**Household food waste is large, and only partly about dates.** In the EU, food waste is
estimated at about **130 kg per person per year, of which ~69 kg is household waste**.
Date marking is associated with up to ~10% of food waste, but "date marking" spans
*best before*, *use by*, and *display until* — it is **not** a clean measure of "thrown
away because the date passed." For a Japan-focused estimate, waste caused by expired
best-before dates is provisionally taken as **~7–8% of household food loss**.
*This ambiguity is the reason the app logs why food was discarded instead of assuming it
([§6](#6-food-loss-instrumentation-the-most-opinionated-part-of-the-schema)).*

**Nutrient risk is directional, not diagnostic.** In the EU/Germany, insufficient intake
risks have been identified for vitamin D, iodine, calcium, iron, and folate, alongside
excessive salt, free sugars, and sugar/fat/salt from processed foods. Enough to say
"you've logged few iron-containing foods." Not enough to say "you are iron deficient."

## 2. The issue and the insight

**Issue.** People want to eat healthily and avoid wasting food, but daily management is
tedious, feedback is weak, and doing it alone is easy to quit.

**Insight.** Nutrition management and food-waste reduction look like two problems. They are
one: *everyday food-related behavior is hard to manage.* Nutrition apps demand heavy input
and give little back; inventory apps have no reason to be opened daily and no link to
health. Neither side alone answers the question people actually have.

**The question the product answers:** *what should I eat today* — judged from both health
and what's about to go bad in my kitchen.

## 3. Concept and core experience

| | |
| --- | --- |
| **Vision** | Make a way of eating that is kind to both the planet and yourself an everyday habit. |
| **Concept** | Design a healthy way of eating with minimal waste. |
| **Core experience** | Use what you already have at home to make today's meals healthier — without overcomplicating things. |
| **Primary user** | People who care about healthy eating and environmental responsibility, but found existing nutrition or inventory apps too cumbersome to keep using — busy single-person households, couples, and small households who buy ingredients and struggle to finish them. |

## 4. From issue to implementation

The traceability that makes this "issue-driven" rather than "issue-flavored." Every row runs
from a user problem to the code that answers it.

| User problem | Design decision | Where it lives |
| --- | --- | --- |
| Manual entry is tedious, and input burden is the biggest driver of churn | Log a meal or a product from a **photo**; AI extracts the nutrition, the user confirms | [`nutrition.go`](backend/internal/handler/nutrition.go), [`labels.go`](backend/internal/handler/labels.go), [`imageProcessing.ts`](frontend/src/services/imageProcessing.ts) |
| Recording gives nothing back, so people stop | A scan **immediately** credits nutrition totals and creates pantry stock in one transaction — one action, two payoffs | [`inventory.go`](backend/internal/handler/inventory.go) (`POST /inventory/scans`) |
| "I forgot it was in the fridge" | Pantry groups stock by urgency, surfacing an **"Eat soon"** set (today/tomorrow) ahead of everything else | [`PantryView.tsx:62`](frontend/src/components/PantryView.tsx:62) |
| Food quietly expires and the record silently rots | Listing pantry or waste **reconciles overdue stock transactionally**: the remainder is resolved and copied to waste exactly once, history retained, never double-counted | [`inventory_lifecycle.go`](backend/internal/handler/inventory_lifecycle.go) |
| "Thrown away because it expired" hides the real cause | Every discard captures **reason, date-label type, days past date, package state, and spoilage evidence**, bucketed into expiry-caused / expiry-related / expiry-unrelated | [`0001_init.sql:58`](backend/migrations/0001_init.sql:58), [`waste.go:144`](backend/internal/handler/waste.go:144) |
| Waste feels abstract, so it doesn't motivate | Waste events return **CO2e, virtual water, and urban-tree-year equivalents**, computed deterministically at read time from published category factors | [`environmental_impact.go`](backend/internal/handler/environmental_impact.go), [`ENVIRONMENTAL_IMPACT.md`](docs/ENVIRONMENTAL_IMPACT.md) |
| Managing your health alone feels lonely | A draggable companion character ("Nutri") reacts to the day's logging — positioned as a **retention mechanism**, not decoration | [`CompanionCharacter.tsx`](frontend/src/components/CompanionCharacter.tsx) |
| People return to the app on their own schedule | Expiry re-checks at **browser-local midnight and on resume**; the server applies the profile timezone when deciding what expired | [`App.tsx`](frontend/src/App.tsx), [`inventory_lifecycle.go`](backend/internal/handler/inventory_lifecycle.go) |
| An app you can't keep open on your phone isn't a daily habit | Installable **PWA** with offline app shell, rear-camera capture, and EXIF-stripped, downscaled uploads | [`sw.js`](frontend/public/sw.js), [`imageProcessing.ts`](frontend/src/services/imageProcessing.ts) |
| Signing up before knowing whether the app is worth it is its own churn point | **Continue as guest** — the AI flow works without an account, capped server-side at `GUEST_AI_DAILY_LIMIT` analyses per day per IP, since each one costs a real Gemini call and a client-side cap is trivially bypassed | [`aiquota.go`](backend/internal/middleware/aiquota.go), [`LoginView.tsx`](frontend/src/components/LoginView.tsx) |
| The one feature worth trying first asks you to go find a photo before you can try it | A **bundled sample meal photo** (public-domain USDA image, chosen because it holds four separable items) loads straight into the scanner | [`sampleMealPhoto.ts`](frontend/src/services/sampleMealPhoto.ts) |


## Stack

| Layer         | Technology                                                            |
| ------------- | --------------------------------------------------------------------- |
| Frontend      | React 18 + TypeScript, Vite, Tailwind CSS, Recharts, lucide-react     |
| Backend       | Go 1.25, chi router, pgx (Postgres), golang-jwt                       |
| Database      | PostgreSQL 16                                                         |
| Auth          | JWT (HS256) + RBAC middleware                                         |
| AI            | Google Gemini (`generateContent`); Mistral and OpenAI join the fan-out |
| Observability | OpenTelemetry (OTLP/gRPC) → Collector → Loki, Tempo, Prometheus, Grafana |
| LLM tracing   | LangSmith (optional), fed by the same OTel tracer provider            |
| Deployment    | Railway (API + Postgres), Cloudflare Pages (frontend)                 |
| Infra         | Docker Compose (Postgres, Adminer, optional app + monitoring profiles) |

Also in place: per-IP rate limiting, PII-safe hashed identifiers in logs, CI via GitHub
Actions, and end-to-end tests on both sides.

## Layout

```
smart-food-manager/
├── backend/                    Go API service
│   ├── cmd/api/                main entrypoint (graceful shutdown)
│   ├── cmd/migrate/            migration runner (also the deploy's preDeployCommand)
│   ├── cmd/orchestrate/        CLI that runs one prompt through the fan-out
│   ├── internal/
│   │   ├── config/             env-based configuration
│   │   ├── server/             router + route wiring + CORS
│   │   ├── middleware/         JWT auth, RBAC, rate limiting, guest AI quota, request logging
│   │   ├── handler/            health, auth, meals, goals, inventory + lifecycle, waste,
│   │   │                       environmental impact, nutrition, labels, advice panel, telemetry
│   │   ├── gemini/             Google Gemini API client (AI features)
│   │   ├── mistral/            Mistral chat-completions client (fan-out agent)
│   │   ├── agent/              one LLM call behind an interface (Gemini, Mistral, OpenAI)
│   │   ├── llm/                traces the model calls the handlers make
│   │   ├── orchestrator/       fans a prompt across agents, merges the answers
│   │   ├── tracing/            LangSmith-shaped spans for LLM runs
│   │   ├── logging/            structured event logging (bridged to OTel)
│   │   ├── telemetry/          OpenTelemetry setup + metric instruments
│   │   └── store/              pgx connection pool
│   ├── migrations/             SQL migrations (0001_init.sql = full schema)
│   ├── e2e/                    end-to-end API tests (build tag: e2e)
│   └── Dockerfile
├── frontend/                   React + TS (Vite + Tailwind)
│   ├── src/
│   │   ├── api/                fetch client + shared types
│   │   ├── components/         Today (three layouts), stats, pantry, account,
│   │   │                       companion, add-entry modal, planned-action stub
│   │   ├── services/           nutrition analysis, image processing, persistence, account
│   │   ├── telemetry/          batched frontend event logging
│   │   ├── hooks/, types/      media queries; nutrition and pantry models
│   │   ├── preferences.ts      per-browser display choices (Today layout)
│   │   └── App.tsx, main.tsx   app shell + entry
│   ├── public/                 PWA manifest, icons, offline service worker
│   ├── e2e/                    Playwright UI tests
│   └── Dockerfile
├── observability/              LGTM stack configs + provisioned Grafana dashboards
├── docs/                       API reference, environmental-impact method, as-built DDL
├── docker-compose.yml          Postgres + Adminer (+ `app` / `monitoring` profiles)
└── Makefile                    task runner (make help)
```


## Prerequisites

- Go 1.25+
- Node 20+
- Docker (for Postgres via Compose)
- A Google Gemini API key — **optional**; AI-backed endpoints return `502` until
  `GEMINI_API_KEY` is set, and the frontend falls back to local estimates so the
  implemented UI flows stay usable without it.


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

## Observability

Bring up the local LGTM stack, then run the backend (natively or via the `app` profile) so it
exports to the collector:

```bash
make obs-up           # OTel Collector + Loki + Tempo + Prometheus + Grafana
```

- Grafana:    http://localhost:3000 (anonymous admin; provisioned datasources + dashboards)
- Prometheus: http://localhost:9090
- Tempo:      http://localhost:3200
- Loki:       http://localhost:3100

Tear it down with `make obs-down`. Telemetry is best-effort. The collector is used only when
an `OTEL_EXPORTER_OTLP_*` endpoint is configured — `.env` sets one for local development. With
none set, as in the Railway deployment, traces still reach LangSmith over its own exporter
while metrics are skipped and logs stay on stderr for the platform to collect. That last part
is load-bearing: bridging `slog` to a collector that is not there would swallow every log
entry rather than surface it. See
[`observability/README.md`](observability/README.md) for the logging blueprint and dashboards.


## Tests

```bash
make test                 # backend unit tests
```

```bash
make test-e2e             # end-to-end API tests (needs the db from `make db-up`)
```

```bash
make test-e2e-frontend    # Playwright UI tests (boots Vite itself; no backend needed)
```

Run `make help` for the full list of targets (also: `migrate`, `build`, `clean`).

## API

Base path `/api/v1`. See the [full API reference](docs/API.md) for request and response schemas,
validation, status codes, frontend call coverage, the outbound Gemini contract, and known
integration gaps. AI-backed routes require `GEMINI_API_KEY`; in the current wiring a missing key
is reported as `502` by those handlers.

| Method | Path | Auth | Notes |
| ------ | ---- | ---- | ----- |
| GET | `/healthz` | none | Liveness + DB ping |
| POST | `/api/v1/auth/login` | none | Verifies an active database user and issues a JWT |
| GET | `/api/v1/nutrients` | none | Lists the active nutrient master |
| POST | `/api/v1/telemetry/logs` | optional Bearer | Frontend telemetry sink; a token binds events to the user |
| POST | `/api/v1/nutrition/analyze` | optional Bearer | AI food analysis from text or an image; guests get `GUEST_AI_DAILY_LIMIT` a day |
| GET | `/api/v1/nutrition/quota` | optional Bearer | Remaining guest AI analyses; reading it spends none |
| GET/PATCH | `/api/v1/me` | Bearer | Reads the account (claims + email + display name); `PATCH` renames it |
| GET/POST | `/api/v1/meals` | Bearer | List and create meals (`GET/PUT/DELETE /{mealID}`) |
| GET/PUT/DELETE | `/api/v1/goals` | Bearer | Daily nutrition goals |
| GET/POST | `/api/v1/inventory` | Bearer | Lists active pantry stock (reconciles expiry) and creates items |
| POST | `/api/v1/inventory/scans` | Bearer | Atomically saves a scan to pantry and nutrition |
| POST | `/api/v1/inventory/{id}/consume` | Bearer | Reconciles consumed, remaining, and wasted amounts |
| GET/POST | `/api/v1/waste-events` | Bearer | Waste with environmental-impact estimates (`GET/PUT/DELETE /{eventID}`) |
| POST | `/api/v1/nutrients/advice` | Bearer | AI nutrition advice |
| POST | `/api/v1/nutrients/advice/panel` | Bearer | Same question to every configured model, merged; registered only when a provider key is set |
| POST | `/api/v1/foods/from-label` | Bearer | Extract nutrients from a label image and save a food |
| GET | `/api/v1/admin/ping` | Bearer + `admin` | RBAC example |

Try the protected route:

```bash
TOKEN=$(curl -s localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"email":"me@example.com","password":"correct-horse"}' | sed 's/.*"token":"//;s/".*//') && curl localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
```


```bash
make migrate            # apply what is outstanding
make migrate-status     # dry run: report only, change nothing
make migrate-baseline   # record migrations as applied WITHOUT running them
```

`make migrate-baseline` is for a database whose schema predates the runner — the development
database built by docker-compose, which applied the same files through
`docker-entrypoint-initdb.d` before `schema_migrations` existed. Run it once per such database;
a freshly provisioned Railway database needs only the pre-deploy command.

### One-time setup

```bash
# 1. Project and database
railway init --name smart-food-manager
railway add --database postgres

# 2. API service — create it, then set its Root Directory to `backend` so
#    Railway picks up backend/railway.toml and backend/Dockerfile. The deploy
#    workflow uploads from the repository root and relies on this setting.
railway variables --set 'DATABASE_URL=${{Postgres.DATABASE_URL}}' \
                  --set JWT_SECRET=… --set LOG_HASH_SALT=… \
                  --set GEMINI_API_KEY=… --set MISTRAL_API_KEY=… \
                  --set LANGSMITH_API_KEY=…
railway up            # first deploy; generates the public domain

# 3. Site — create a Cloudflare Pages project named smart-food-manager,
#    then point the API at its origin:
railway variables --set ALLOWED_ORIGIN="https://smart-food-manager.pages.dev"
```

`DATABASE_URL` is a *reference* variable: Railway resolves `${{Postgres.DATABASE_URL}}` from the
database service, so credentials are never copied around and rotate with it.

Then add to the repository: secrets `RAILWAY_TOKEN` (a project token), `CLOUDFLARE_API_TOKEN`,
`CLOUDFLARE_ACCOUNT_ID`, and the variables `RAILWAY_SERVICE` (the API service's name) and
`API_BASE_URL` (its public URL).

Leave the Railway service **disconnected from GitHub**. Deploys go through
[`deploy.yml`](.github/workflows/deploy.yml) so both halves stay in one path-filtered workflow;
connecting the repo in Railway as well would deploy the backend twice on every push.

### Two things that must line up

- **`VITE_API_BASE_URL` is baked in at build time.** Vite inlines it, so pointing the site at a
  different API is a rebuild, not an environment change — run the deploy workflow manually with
  `target: frontend`.
- **`ALLOWED_ORIGIN` must name every origin that calls the API**, comma-separated. Production and
  each Cloudflare preview deployment are separate origins; the API echoes back only origins on
  the list and sends nothing for the rest.

Put the Railway project in a region near your users: the API and database talk over the private
network, so it is the browser round trip that dominates.

## Before shipping

- Move the JWT secret and `LOG_HASH_SALT` out of defaults into managed secrets, and set a real
  `GEMINI_API_KEY`.
- Swap the in-memory rate limiter for a shared (e.g. Redis) limiter once you run more than one
  backend instance behind the API gateway.
- Wire the companion message endpoint server-side, or remove the client call and keep the local
  message generator as the intended behavior.
- Give the multi-model panel a caller. The route and the `orchestrate` CLI both exist and are
  traced; nothing in the UI puts a question to them yet.
