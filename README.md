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

## 5. Scope discipline: what this deliberately does not do

The product is a companion, not a clinician. That boundary is a product decision with
concrete consequences for wording and features.

**In scope:** trends in nutrient intake from food, rough visualization of meal balance,
general tips for improving eating habits.

**Out of scope:** supplements, medical diagnosis, treatment, individualized nutrition
prescriptions, disease-specific dietary guidance.

Copy is held to the same line — *"you've logged relatively few iron-containing foods,"*
never *"you are iron deficient"* or *"you should take supplements."* Nutrients tracked for
**low** trends: protein, fiber, calcium, iron, potassium. For **excess** trends: energy,
salt/sodium, saturated fat, free sugars.

## 6. Food-loss instrumentation (the most opinionated part of the schema)

Because the research showed "expired" is a muddy category, the database refuses to collapse
it. `waste_events` records five independent dimensions per discard, as Postgres enums:

```sql
discard_reason   expired_best_before | expired_use_by | near_expiry_but_not_used
                 | spoiled_visible | smelled_or_tasted_bad | forgot_item_existed
                 | overbought | cooked_too_much | leftover_not_eaten
                 | unsure_if_safe | storage_failure | preference_changed | other
date_label_type  best_before | use_by | display_until | no_date_label | unknown
date_status      before_date | on_date | 1_3_days_after | 4_7_days_after
                 | 8_14_days_after | 15_plus_days_after | unknown
package_status   unopened | opened | cooked | leftover | unknown
spoilage         none | visual_mold | discoloration | smell | texture_change | taste | unknown
```

These roll up into an analysis bucket — `expiry_caused`, `expiry_related`, or
`expiry_unrelated` — derived server-side in [`waste.go:167`](backend/internal/handler/waste.go:167).
The payoff: the app can distinguish *"the date passed and I binned it unopened"* from
*"I forgot it existed"* from *"I cooked too much."* Those are three different problems with
three different interventions, and lumping them together would make the product's central
claim unmeasurable.

Environmental estimates are read-time and versioned
(`poore-nemecek-2018+wfn-global-average+epa-2024`), so updating the factor table
recomputes history rather than stranding it. Every waste response carries the active
`impact_factor_version`. These are directional product feedback, not a lifecycle assessment.

## 7. Honest status

Portfolio value comes from the reasoning being traceable — including where the code hasn't
caught up with the design yet. Nothing below is hidden in the UI either: planned-but-unwired
controls render through a dedicated [`PlannedAction`](frontend/src/components/PlannedAction.tsx)
component that stays keyboard-reachable, announces itself as unavailable via `aria-disabled`,
and prints the reason in plain text next to itself.

| Area | Status | Notes |
| --- | --- | --- |
| Photo → nutrition logging | **Built** | Gemini-backed; local estimate fallback when no API key |
| Scan → pantry + provisional intake, then reconciliation on consume | **Built** | Single transaction; ingredients defer intake until consumed |
| Expiry reconciliation → waste, exactly once | **Built** | Timezone-aware, history-preserving, covered by tests |
| Structured discard taxonomy (5 dimensions + analysis bucket) | **Built** | Full enum set in schema and API |
| Environmental impact estimates | **Built** | Versioned factors, deterministic, documented sources |
| Companion character | **Partial** | Animated and reactive in-app, but `POST /api/v1/companion/message` has **no backend route** — the client calls it and falls back to local messages ([`nutritionService.ts:172`](frontend/src/services/nutritionService.ts:172)) |
| Discard reasons in the UI | **Partial** | The picker offers 7 common reasons; the schema supports all 13 |
| Rough range language ("tends to be low") | **Not built** | The dashboard still shows precise values against goals ([`NutritionCard.tsx`](frontend/src/components/NutritionCard.tsx)). The stated design calls for range-based phrasing — this is the largest open gap between spec and code |
| Today's meal suggestions from near-expiry stock | **Not built** | "Eat soon" grouping exists; recipe suggestions are marked planned in the UI |
| Reminders / push notifications | **Not built** | The service worker is offline-shell only; no Web Push. Companion nudges are in-app only |
| Guest trial without an account | **Built** | "Continue as guest" plus a server-enforced daily AI allowance ([`aiquota.go`](backend/internal/middleware/aiquota.go)) |
| Signup / password recovery | **Not built** | Account creation needs a seeded row and bcrypt hash; guests can look around in the meantime |

## 8. How this would be measured

KPIs chosen to test the hypothesis rather than flatter it — retention first, because the
research says that is where these apps fail.

- **Retention** — D1 / D7 / D30, and share of users logging ≥3 days per week
- **Nutrition** — meal-log completion rate, logging days per week, and the rate of a
  corrective action following a "tends to be low" signal
- **Food loss** — items registered, consumption rate of near-expiry stock, expiry-related
  discard logs, items fully used up
- **Companion** — feedback view rate, and the rate of *resuming* logging after a lapse
  (the metric that decides whether the character is a retention mechanism or a mascot)

---

# Engineering

Monorepo holding the **Go** backend API and the **React + TypeScript** frontend, with a
full local observability stack.

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
│   ├── cmd/orchestrate/        CLI that runs one prompt through the fan-out
│   ├── internal/
│   │   ├── config/             env-based configuration
│   │   ├── server/             router + route wiring + CORS
│   │   ├── middleware/         JWT auth, RBAC, rate limiting, request logging
│   │   ├── handler/            health, auth, meals, goals, inventory, waste, labels, telemetry
│   │   ├── gemini/             Google Gemini API client (AI features)
│   │   ├── mistral/            Mistral chat-completions client (fan-out agent)
│   │   ├── agent/              one LLM call behind an interface (Gemini, Mistral, OpenAI)
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
│   │   ├── components/         dashboard, pantry, stats, settings, companion, add-entry modal
│   │   ├── services/           nutrition analysis, image processing, persistence
│   │   ├── telemetry/          batched frontend event logging
│   │   └── App.tsx, main.tsx   app shell + entry
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

## Configuration

The backend reads its config from environment variables and ships with sensible development
defaults (see [`backend/internal/config/config.go`](backend/internal/config/config.go)), so it
runs without any `.env`. The `Makefile` and `docker-compose.yml` both load a root `.env` when
present. There is no committed `.env.example`; create a git-ignored `.env` if you need to
override defaults — a working starting point:

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

# Mistral / OpenAI — optional; each adds a provider to the multi-agent fan-out.
MISTRAL_API_KEY=
OPENAI_API_KEY=

# LangSmith LLM tracing — leave blank to keep LLM spans on the collector only.
LANGSMITH_API_KEY=
LANGSMITH_PROJECT=smart-food-manager
```

> **Port note:** `make db-up` exposes Postgres on host port **5433** by default (to avoid clashing
> with a local 5432 instance), while the backend's built-in `DATABASE_URL` default points at 5432.
> When running the API natively against the Compose database, set `DATABASE_URL` (or
> `DB_HOST_PORT=5432`) in `.env` so the two agree.

Other backend variables (all optional, with defaults): `PORT`, `JWT_EXPIRY_MINUTES`,
`RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`, `ALLOWED_ORIGIN` (comma-separated for previews),
`GUEST_AI_DAILY_LIMIT`, `SERVICE_NAME`,
`SERVICE_VERSION`, `DEPLOYMENT_ENVIRONMENT`, `GEMINI_BASE_URL`, `GEMINI_MODEL`,
`GEMINI_TIMEOUT_SECONDS`, `GEMINI_ALT_MODEL`, `OPENAI_MODEL`, `OPENAI_TIMEOUT_SECONDS`,
`MISTRAL_MODEL`, `MISTRAL_BASE_URL`, `MISTRAL_TIMEOUT_SECONDS`,
`LANGSMITH_PROJECT`, `LANGSMITH_ENDPOINT`, `LANGSMITH_TRACING`,
`LANGSMITH_CAPTURE_CONTENT`.
`GUEST_AI_DAILY_LIMIT` (default `3`) caps how many AI analyses a visitor without an account may
run per UTC day, counted per client IP; `-1` removes the cap and `0` closes AI analysis to guests
entirely. Signed-in callers are never capped.
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

Open http://localhost:5173. Vite's development proxy exposes `/healthz` and
`/api/*` from the backend on the frontend origin (see
[`frontend/vite.config.ts`](frontend/vite.config.ts)). Adminer (DB browser) is
at http://localhost:8081 (server `db`, user/password from `.env`).

Existing databases need the inventory-lifecycle migration before starting the
updated API:

```bash
docker compose exec -T db psql -U "${POSTGRES_USER:-app}" -d "${POSTGRES_DB:-foodapp}" < backend/migrations/0003_inventory_lifecycle.sql
```

## Scan and pantry lifecycle

- **Food / product:** scanning stores the nutrient snapshot, creates pantry stock with an expiry
  date, and provisionally counts the whole scanned portion. Recording the actual consumed amount
  replaces that estimate; an explicitly discarded remainder becomes a waste event.
- **Ingredient:** scanning stores the nutrient snapshot in the pantry without changing intake.
  Each consumed portion becomes a meal entry, while an undiscarded remainder stays available until
  its estimated expiry.
- **Expiry:** listing pantry or waste data reconciles overdue unresolved stock transactionally.
  The remaining quantity is marked resolved and copied to waste once; history is retained rather
  than physically deleting the inventory row. An open client refreshes at browser-local midnight
  and when it resumes; the server applies the profile timezone when deciding what has expired.
- **Impact:** every waste event is converted from grams using category-level global-average
  coefficients. Values are estimates for feedback, not a formal lifecycle assessment; see
  [`docs/ENVIRONMENTAL_IMPACT.md`](docs/ENVIRONMENTAL_IMPACT.md) for formulas and sources.

## Install on Android

The frontend is an installable Progressive Web App. Its production build includes an Android-ready
manifest, maskable cat icons, standalone display mode, offline app-shell support, safe-area handling,
and a rear-camera capture flow.

```bash
cd frontend && npm run build && npm run preview
```

For local testing, open the preview in Chrome. For installation on a phone, host `frontend/dist`
over HTTPS, set `VITE_API_BASE_URL` to the HTTPS backend origin when the API is not served from the
same host, then choose **Install app** in Chrome. Camera input opens the rear camera when Android
supports it and always keeps a gallery fallback. Before upload, photos are re-oriented, stripped of
EXIF metadata, converted to JPEG, and downscaled to a maximum 1,600 px edge.

To run the whole app in containers instead:

```bash
docker compose --profile app up --build
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

Tear it down with `make obs-down`. Telemetry is best-effort: if the collector is unreachable,
the backend logs a warning and falls back to stderr logging. See
[`observability/README.md`](observability/README.md) for the logging blueprint and dashboards.

### LLM tracing (LangSmith)

Set `LANGSMITH_API_KEY` (and leave `LANGSMITH_TRACING` at `true`) and multi-agent LLM work
shows up in LangSmith as a run tree — a `chain` run for the fan-out with one `llm` run per
agent underneath, each carrying its model, prompt, answer, and token usage. Leave the key
blank and nothing changes: the spans are ordinary OTel spans and still reach Tempo.

Exercise it without an HTTP route:

```bash
set -a; . ./.env; set +a
cd backend && go run ./cmd/orchestrate "what should I cook with lentils and spinach?"
```

The fan-out compares two Gemini models (`GEMINI_MODEL` and `GEMINI_ALT_MODEL`), and adds a
Mistral or OpenAI agent for each of `MISTRAL_API_KEY` / `OPENAI_API_KEY` that is set. An agent
whose provider fails is dropped from the merge rather than failing the run, so the roster can
exceed what a given key can actually reach.

LangSmith attaches as a second span processor on the tracer provider `telemetry.Setup`
already builds, rather than through `langsmith.NewOTelTracer`, which would install a provider
of its own and take the collector export and the service resource attributes with it. Two
things follow from sharing one provider:

- **The exporter pins its own scheme.** `OTEL_EXPORTER_OTLP_INSECURE=true` is set here for the
  local collector, but that variable is global: left to it, the LangSmith exporter downgrades
  to `http://` and every export fails with `405 Method Not Allowed`. `telemetry` builds the
  exporter with `otlptracehttp.WithEndpointURL` so TLS is fixed per exporter. `LANGSMITH_ENDPOINT`
  may be a URL (`https://eu.api.smith.langchain.com`) or a bare host; both are normalised.
- **Every span is exported, not just LLM ones.** HTTP client and server spans land in the
  LangSmith project as generic `chain` runs. That is what keeps a run tree whole when its
  parent is an ordinary HTTP span, and it is why the export stays off unless a key is set. The
attribute conventions LangSmith reads (`langsmith.span.kind`, `gen_ai.prompt`,
`langsmith.usage_metadata`, …) live in one place, [`internal/tracing`](backend/internal/tracing/tracing.go).

Prompt and answer text is attached to spans only while LangSmith is enabled, since those
spans also reach the collector and the logging design otherwise keeps user text out of the
telemetry backend. `LANGSMITH_CAPTURE_CONTENT=false` records metadata and token counts alone.

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
| GET | `/api/v1/me` | Bearer | Returns the caller's claims |
| GET/POST | `/api/v1/meals` | Bearer | List and create meals (`GET/PUT/DELETE /{mealID}`) |
| GET/PUT/DELETE | `/api/v1/goals` | Bearer | Daily nutrition goals |
| GET/POST | `/api/v1/inventory` | Bearer | Lists active pantry stock (reconciles expiry) and creates items |
| POST | `/api/v1/inventory/scans` | Bearer | Atomically saves a scan to pantry and nutrition |
| POST | `/api/v1/inventory/{id}/consume` | Bearer | Reconciles consumed, remaining, and wasted amounts |
| GET/POST | `/api/v1/waste-events` | Bearer | Waste with environmental-impact estimates (`GET/PUT/DELETE /{eventID}`) |
| POST | `/api/v1/nutrients/advice` | Bearer | AI nutrition advice |
| POST | `/api/v1/foods/from-label` | Bearer | Extract nutrients from a label image and save a food |
| GET | `/api/v1/admin/ping` | Bearer + `admin` | RBAC example |

Try the protected route:

```bash
TOKEN=$(curl -s localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"email":"me@example.com","password":"correct-horse"}' | sed 's/.*"token":"//;s/".*//') && curl localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
```

The login example requires a matching active user and bcrypt password hash in the database;
the application does not currently expose a signup endpoint.

## Deployment

The two halves deploy independently from this one repository:

| Piece | Host | Cost |
| --- | --- | --- |
| Go API | Railway service, built from `backend/Dockerfile` | ~$1.40/mo usage |
| Postgres | Railway service in the same project | ~$2.90/mo usage |
| Frontend | Cloudflare Pages | $0 |

Railway's Hobby plan is **$5/mo including $5 of usage credit**, so the two services above fit
inside it — the bill is the plan fee. Keeping Postgres on Railway rather than a separate
provider costs a little more than a free external tier, and buys a private network hop instead
of a public-internet one, plus a database that is never cold. [`backend/fly.toml`](backend/fly.toml)
is kept as a working Fly.io + external-Postgres alternative, but nothing deploys it.

[`.github/workflows/deploy.yml`](.github/workflows/deploy.yml) decides what to ship from which
paths changed, so a frontend-only commit never restarts the API. It uses a `changes` job rather
than a top-level `paths:` filter: a job skipped by a path filter reports "pending" forever if it
is ever made a required check, which would block every merge that did not touch that half.

### Migrations

`backend/migrations/*.sql` are embedded in the image and applied by a small runner
([`cmd/migrate`](backend/cmd/migrate/main.go)) that records each file in a `schema_migrations`
table and wraps it in a transaction. [`railway.toml`](backend/railway.toml) runs it as
`preDeployCommand`, in a separate container off the new image with the service's variables,
before the new version takes traffic — so a failed migration aborts the release instead of
leaving the API running against a schema it does not expect.

It has to be a Go binary rather than a `psql` loop for two reasons: the runtime image is
distroless, so there is no shell and no `psql`; and the migrations are not idempotent
(`CREATE TYPE` without `IF NOT EXISTS`), so re-running them all on every deploy would fail on
the second one. Both constraints are platform-independent, which is why moving between hosts
touches only the deploy config.

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
- Add migration tooling (e.g. `golang-migrate`) for versioned, repeatable migrations beyond the
  Compose auto-init.
- Wire the companion message endpoint server-side, or remove the client call and keep the local
  message generator as the intended behavior.
