# Smart Food Manager API

This document describes the HTTP contracts implemented by the Go backend, the
network API used by the React frontend, and the backend's outbound Gemini API
integration.

Implementation reviewed against the workspace API and frontend contracts on
2026-08-16.

## 1. Service boundaries

```text
React frontend
  ├─ instrumented JSON client ───────────────┐
  └─ best-effort telemetry sender ──────────┤
                                             ▼
                                   Go API (/api/v1)
                                     ├─ PostgreSQL
                                     └─ Gemini generateContent
```

Default local addresses:

| Service | URL | Notes |
| --- | --- | --- |
| Frontend | `http://localhost:5173` | Vite proxies `/api` and `/healthz` to the backend in development. |
| Backend | `http://localhost:8080` | `PORT` changes the listening port. |
| API base | `/api/v1` | `/healthz` is outside the versioned base path. |
| Gemini | `https://generativelanguage.googleapis.com/v1beta` | Overridable with `GEMINI_BASE_URL`. |

In a deployed frontend, set `VITE_API_BASE_URL` to the backend origin. If it is
unset, the frontend uses same-origin relative URLs.

## 2. Backend endpoint index

| Method | Path | Authentication | Success | Used by frontend |
| --- | --- | --- | --- | --- |
| `GET` | `/healthz` | None | `200` | No |
| `POST` | `/api/v1/auth/login` | None | `200` | Yes |
| `GET` | `/api/v1/nutrients` | None | `200` | No |
| `POST` | `/api/v1/telemetry/logs` | Optional Bearer token | `202` | Yes |
| `POST` | `/api/v1/nutrition/analyze` | Optional Bearer token | `200` | Yes |
| `GET` | `/api/v1/nutrition/quota` | Optional Bearer token | `200` | Yes |
| `GET/PATCH` | `/api/v1/me` | Bearer token | `200` | Yes |
| `GET/POST` | `/api/v1/meals` | Bearer token | `200` / `201` | Yes |
| `GET/PUT/DELETE` | `/api/v1/meals/{mealID}` | Bearer token | `200` / `204` | Partly |
| `GET/PUT/DELETE` | `/api/v1/goals` | Bearer token | `200` / `204` | Yes |
| `GET/POST` | `/api/v1/inventory` | Bearer token | `200` / `201` | Yes |
| `GET/PUT/DELETE` | `/api/v1/inventory/{itemID}` | Bearer token | `200` / `204` | Partly |
| `POST` | `/api/v1/inventory/scans` | Bearer token | `201` | Yes |
| `POST` | `/api/v1/inventory/{itemID}/consume` | Bearer token | `200` | Yes |
| `GET/POST` | `/api/v1/waste-events` | Bearer token | `200` / `201` | Yes |
| `GET/PUT/DELETE` | `/api/v1/waste-events/{eventID}` | Bearer token | `200` / `204` | Partly |
| `POST` | `/api/v1/nutrients/advice` | Bearer token | `200` | No |
| `POST` | `/api/v1/foods/from-label` | Bearer token | `201` | No |
| `GET` | `/api/v1/admin/ping` | Bearer token with `admin` role | `200` | No |

`POST /api/v1/companion/message` is called by the frontend but is **not**
implemented by the backend. The frontend catches the resulting failure and
generates a local message.

## 3. Common backend behavior

### 3.1 Headers and content types

JSON requests should send:

```http
Content-Type: application/json
```

Authenticated requests send:

```http
Authorization: Bearer <jwt>
```

The instrumented frontend client also sends:

```http
traceparent: 00-<32-lowercase-hex-trace-id>-<16-lowercase-hex-span-id>-01
X-Session-Id: sess-<random-hex>
```

`X-Session-Id` is accepted only when it contains 1–64 letters, digits, dots,
underscores, or hyphens. A valid W3C `traceparent` is consumed by the
OpenTelemetry middleware.

The label extraction endpoint is the exception to JSON input. It requires
`multipart/form-data` with a file field named `image`.

### 3.2 Authentication

The login endpoint issues an HS256 JWT. Its application claims are:

```json
{
  "uid": "user UUID",
  "roles": ["user"],
  "sub": "user UUID",
  "iat": 1785340800,
  "exp": 1785344400
}
```

The default token lifetime is 60 minutes and is controlled by
`JWT_EXPIRY_MINUTES`. The signing secret comes from `JWT_SECRET`.

Required-auth routes reject a missing token with `401 missing bearer token`
and an invalid or expired token with `401 invalid token`. These middleware
errors are plain text, not the JSON error envelope used by handlers.

Optional-auth routes behave differently: a valid token attaches user context,
while a missing, malformed, invalid, or expired token is silently ignored and
the request continues anonymously.

### 3.3 Error formats

Most handler errors are JSON:

```json
{
  "error": "human-readable message"
}
```

Authentication, authorization, and the shared rate limiter use plain text
(the guest AI quota in §3.6 is the one middleware that answers in JSON):

```text
missing bearer token
invalid token
forbidden
rate limit exceeded
```

Clients must therefore use the response `Content-Type` before assuming an
error-body shape.

### 3.4 CORS

Every backend response receives these CORS headers:

```http
Access-Control-Allow-Origin: <ALLOWED_ORIGIN>
Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS
Access-Control-Allow-Headers: Authorization, Content-Type, X-Session-Id, traceparent
Access-Control-Expose-Headers: X-AI-Quota-Limit, X-AI-Quota-Remaining, X-AI-Quota-Reset
```

`ALLOWED_ORIGIN` defaults to `http://localhost:5173`. Preflight `OPTIONS`
requests return `204 No Content`.

### 3.5 Rate limiting

All non-preflight routes share an in-memory token-bucket limiter keyed by
client IP:

- Default refill: 10 requests/second (`RATE_LIMIT_RPS`)
- Default burst: 20 requests (`RATE_LIMIT_BURST`)
- Rejection: `429 Too Many Requests`, plain-text body
- Scope: one process; buckets are not shared across backend replicas

The implementation trusts the first value in `X-Forwarded-For` when the
header is present. A trusted reverse proxy should replace client-supplied
forwarding headers.

### 3.6 Guest AI quota

`POST /api/v1/nutrition/analyze` additionally caps callers **without** a valid
token — the frontend's "continue as guest" path — because every analysis costs
a Gemini call:

- Default allowance: 3 analyses (`GUEST_AI_DAILY_LIMIT`); `-1` disables the
  cap, `0` closes the route to guests
- Key: client IP, so visitors behind one NAT share an allowance
- Window: UTC calendar day; counters reset at UTC midnight
- Exemption: a request with a valid token is never counted or capped
- Refund: an analysis that does not return `2xx` (rejected payload, provider
  failure) costs nothing
- Scope: one process; counters are not shared across backend replicas

Guest responses from the route carry the remaining allowance:

```http
X-AI-Quota-Limit: 3
X-AI-Quota-Remaining: 2
X-AI-Quota-Reset: 2026-08-18T00:00:00Z
```

Those three headers are listed in `Access-Control-Expose-Headers`, so a
browser client can read them. Rejection is `429` with a JSON body — unlike the
plain-text shared rate limiter, which also answers `429`:

```json
{
  "error": "guest AI analyses are limited to 3 per day; sign in to continue",
  "code": "guest_ai_daily_limit",
  "limit": 3,
  "remaining": 0,
  "resetAt": "2026-08-18T00:00:00Z"
}
```

Clients should branch on `code`, not on the status alone.

## 4. Endpoint contracts

### 4.1 Health

#### `GET /healthz`

Checks process liveness and pings PostgreSQL with a two-second timeout.

Success response — `200 OK`:

```json
{
  "status": "ok",
  "db": true,
  "time": "2026-07-30T10:15:30.123456Z"
}
```

Database-unavailable response — `503 Service Unavailable`:

```json
{
  "status": "ok",
  "db": false,
  "time": "2026-07-30T10:15:30.123456Z"
}
```

The `status` string remains `"ok"` when the database check fails; use the HTTP
status and `db` field to determine readiness.

### 4.2 Login

#### `POST /api/v1/auth/login`

Looks up an active user by email, verifies the bcrypt password hash, loads all
assigned roles, and returns a signed JWT.

Request:

```json
{
  "email": "me@example.com",
  "password": "correct-horse"
}
```

Validation:

- Email is trimmed and lowercased.
- Email and password are required.
- Email must be no more than 254 bytes and parse as a plain RFC email address.
- Password must be 8–72 bytes.
- The user must exist and have `is_active = true`.
- The password must match the stored bcrypt hash.

Success — `200 OK`:

```json
{
  "token": "<signed JWT>"
}
```

Errors:

| Status | Body |
| --- | --- |
| `400` | `{"error":"invalid request body"}` |
| `400` | `{"error":"email and password are required"}` |
| `400` | `{"error":"invalid input format"}` |
| `401` | `{"error":"invalid email or password"}` |
| `500` | `{"error":"could not issue token"}` |

Unknown users and incorrect passwords intentionally share the same `401`
response.

Example:

```bash
curl http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"correct-horse"}'
```

### 4.3 Nutrient master

#### `GET /api/v1/nutrients`

Returns all active nutrients ordered by `sort_order`, then name.

Success — `200 OK`:

```json
[
  {
    "id": 1,
    "code": "protein",
    "name": "Protein",
    "unit": "g",
    "focus": "deficiency_watch",
    "reference_daily_amount": 50
  },
  {
    "id": 2,
    "code": "sodium",
    "name": "Sodium",
    "unit": "mg",
    "focus": "excess_watch",
    "reference_daily_amount": null
  }
]
```

`focus` is one of `deficiency_watch`, `excess_watch`, or `caution`.

Database failures return `500` with one of `query failed`, `scan failed`, or
`row iteration failed` in the standard JSON error envelope.

### 4.4 Frontend telemetry ingest

#### `POST /api/v1/telemetry/logs`

Accepts a batch of browser events and relays valid events to the backend
logging pipeline. The endpoint is public. A valid optional Bearer token binds
accepted events to the authenticated user; a client-supplied `user_id` is
never trusted.

Limits:

- Maximum request body: 256 KiB
- Maximum events per batch: 50
- Invalid events are dropped individually rather than rejecting the batch

Request:

```json
{
  "events": [
    {
      "time": "2026-07-30T10:15:30.123Z",
      "severity": "INFO",
      "message": "Screen viewed: dashboard",
      "service.version": "0.1.0",
      "event.name": "screen_view",
      "event.action": "dashboard",
      "event.outcome": "success",
      "session_id": "sess-a1b2c3d4",
      "trace_id": "0123456789abcdef0123456789abcdef",
      "span_id": "0123456789abcdef",
      "trace_sampled": true,
      "duration_ms": 12,
      "screen.name": "dashboard"
    }
  ]
}
```

Fields required for an event to be accepted:

| Field | Rules |
| --- | --- |
| `event.name` | Must be in the event catalog below. |
| `severity` | `DEBUG`, `INFO`, `NOTICE`, `WARNING`, or `ERROR`; `CRITICAL` is rejected. |
| `message` | Non-empty; truncated to 512 bytes. |
| `event.outcome` | `started`, `success`, or `failure`. |

Accepted event names:

```text
screen_view
navigation
screen_rendered
frontend_error
api_request_started
api_request_completed
api_request_failed
login_clicked
logout_clicked
delete_account_requested
delete_account_confirmed
file_upload_started
file_upload_completed
file_upload_failed
```

Accepted event-specific attributes:

```text
screen.name
screen.from
screen.to
client.locale
client.timezone
client.viewport_width
client.viewport_height
http.request.method
http.route
http.response.status_code
error.type
error.code
error.message
```

All other event-specific attributes are dropped. The server derives
`event.category` from `event.name`; the payload's category is not trusted.
Client timestamps are used only when they fall between ten minutes before
ingest and one minute after ingest. Trace and span IDs must be lowercase
hexadecimal strings of exactly 32 and 16 characters, respectively.

Success — `202 Accepted`:

```json
{
  "accepted": 1,
  "dropped": 0
}
```

Errors:

| Status | Body |
| --- | --- |
| `400` | `{"error":"invalid payload"}` |
| `400` | `{"error":"too many events in batch"}` |

An oversized body is currently reported as `400 invalid payload`, not `413`.

### 4.5 Analyze food from text or an image

#### `POST /api/v1/nutrition/analyze`

Sends a food description or image to Gemini and returns estimated nutrition
for each identified food. Authentication is optional.

Text request:

```json
{
  "type": "text",
  "text": "grilled salmon fillet and a baked potato",
  "scanType": "food"
}
```

Image request:

```json
{
  "type": "image",
  "mimeType": "image/jpeg",
  "data": "<standard base64 without a data-URL prefix>",
  "scanType": "product"
}
```

`scanType` is `food`, `product`, or `ingredient`; an omitted value defaults to
`food`. For image input, `mimeType` may be omitted and the backend detects it
from the decoded bytes. The declared and detected types must agree and must be
JPEG, PNG, or WebP. The JSON body is limited to 9 MiB and decoded image data to
6 MiB.

Success — `200 OK`:

```json
[
  {
    "name": "Grilled salmon",
    "calories": 230,
    "protein": 25,
    "carbs": 0,
    "fat": 14,
    "sodium": 60,
    "calcium": 15,
    "iron": 0.5,
    "scanType": "food",
    "quantityGrams": 180,
    "category": "seafood",
    "estimatedExpiryDays": 3
  }
]
```

The response is a **bare array**, not `{ "items": [...] }`. Nutrients describe
the entire `quantityGrams` portion, not 100 g. The selected `scanType` always
wins over a conflicting model value. Missing or invalid quantity/category/
expiry estimates receive deterministic defaults and all fields remain editable
in the frontend before persistence.

Units:

| Field | Unit |
| --- | --- |
| `calories` | kcal |
| `protein`, `carbs`, `fat` | g |
| `sodium`, `calcium`, `iron` | mg |

Errors:

| Status | Body |
| --- | --- |
| `400` | `{"error":"invalid request body"}` |
| `400` | `{"error":"text is required"}` |
| `400` | `{"error":"invalid image data"}` |
| `400` | `{"error":"scanType must be food, product, or ingredient"}` |
| `400` | `{"error":"type must be \"text\" or \"image\""}` |
| `413` | Request body exceeds 9 MiB or decoded image exceeds 6 MiB |
| `415` | Declared/detected image content is unsupported or mismatched |
| `429` | `{"error":"guest AI analyses are limited to 3 per day","code":"guest_ai_daily_limit",…}` |
| `502` | `{"error":"analysis failed"}` |
| `502` | `{"error":"could not parse analysis result"}` |
| `503` | `{"error":"analyzer not configured"}` |

In normal server wiring the analyzer object always exists. If
`GEMINI_API_KEY` is unset, the Gemini client fails the call and this endpoint
returns `502 analysis failed`, not the `503` reserved for a nil analyzer.

The `429` applies only to callers without a valid token and is described in
§3.6; a failed analysis does not consume the guest's allowance.

#### `GET /api/v1/nutrition/quota`

Reports the caller's remaining guest AI analyses without spending one, so the
frontend can show the allowance before a scan is attempted. Authentication is
optional.

Success — `200 OK` (guest):

```json
{
  "unlimited": false,
  "limit": 3,
  "used": 1,
  "remaining": 2,
  "resetAt": "2026-08-18T00:00:00Z"
}
```

Success — `200 OK` (valid token, or `GUEST_AI_DAILY_LIMIT` below zero):

```json
{
  "unlimited": true,
  "limit": 0,
  "used": 0,
  "remaining": 0
}
```

When `unlimited` is `true` the numeric fields carry no meaning and `resetAt`
is absent. The response repeats the allowance in the `X-AI-Quota-*` headers.

### 4.6 Current user account

#### `GET /api/v1/me`

Requires a valid Bearer token. `user_id` and `roles` come from the verified
token; `email` and `display_name` are read from the `users` row.

Success — `200 OK`:

```json
{
  "user_id": "42a11369-1cda-4d41-9858-d71d5177f442",
  "roles": ["user"],
  "email": "jane.doe@example.com",
  "display_name": "jane.doe"
}
```

`display_name` is never blank. An account that has never been renamed falls
back to the local part of its address — everything before the `@` — so
`jane.doe@example.com` starts out as `jane.doe`. Migration
[`0004_user_display_name.sql`](../backend/migrations/0004_user_display_name.sql)
wrote that same value into the accounts that predate the field.

A token whose subject is not a `users` row — malformed, deleted, or
deactivated — is answered with the JSON `401` envelope rather than a row.
Authentication failures raised by the middleware itself keep using the common
plain-text `401` responses.

#### `PATCH /api/v1/me`

Renames the account. The display name is the only field a caller may change;
the request body must contain nothing else.

```json
{ "display_name": "Ada Lovelace" }
```

The name is trimmed before it is stored. Success returns the full account
payload documented for `GET /api/v1/me`, carrying the saved name.

| Status | Cause |
| --- | --- |
| `400` | Missing, blank, or over-long (> 60 characters) name; control characters; unknown fields in the body. |
| `401` | Missing or invalid token, or a token whose account no longer exists. |
| `500` | The update could not be written. |

### 4.7 Pantry, intake, expiry, and waste lifecycle

All lifecycle routes require a Bearer token and scope every resource to the
authenticated user. The normalized model keeps one scanned `food_id` shared by
pantry stock, nutrient composition, linked meal items, and waste events.

#### `POST /api/v1/inventory/scans`

Atomically saves an analyzed scan. Nutrient values in this request describe
the complete scanned quantity; the server normalizes them to a per-100-g food
snapshot.

```json
{
  "source_type": "product",
  "name": "Strawberry yogurt",
  "category": "dairy",
  "quantity_g": 400,
  "expiry_date": "2026-08-28",
  "expiry_is_estimated": false,
  "date_label": "best_before",
  "storage": "fridge",
  "package": "unopened",
  "consumed_at": "2026-08-16T12:30:00+02:00",
  "nutrients": {
    "calories": 380,
    "protein": 20,
    "carbs": 52,
    "fat": 10,
    "sodium": 240,
    "calcium": 480,
    "iron": 0.4
  }
}
```

`source_type` is `food`, `product`, or `ingredient`; quantity must be positive
and all nutrients must be finite and non-negative. Dates use `YYYY-MM-DD` and
`consumed_at`, when supplied, uses RFC 3339.

For `food` and `product`, the scan creates a linked provisional meal for the
whole quantity, so nutrition is visible immediately. For `ingredient`, it
creates pantry stock and the hidden nutrient snapshot only; no meal is created
until consumption is recorded.

Success — `201 Created`:

```json
{
  "inventory": {
    "id": "6b327324-bdf0-434a-ab90-93cfbe7ee9d0",
    "food_id": "e055d403-6c43-4a9b-8e8e-930322a378d0",
    "name": "Strawberry yogurt",
    "category": "dairy",
    "source_type": "product",
    "provisional_meal_id": "a4581437-47fa-4702-868c-e219e74fded6",
    "expiry_is_estimated": false,
    "quantity_purchased": 400,
    "quantity_consumed": 0,
    "quantity_wasted": 0,
    "best_before_date": "2026-08-28",
    "date_label": "best_before",
    "storage": "fridge",
    "package": "unopened",
    "is_wasted": false,
    "is_resolved": false,
    "consumed_pct": 0,
    "wasted_pct": 0,
    "nutrition_per_100g": {
      "calories": 95,
      "protein": 5,
      "carbs": 13,
      "fat": 2.5,
      "sodium": 60,
      "calcium": 120,
      "iron": 0.1
    },
    "created_at": "2026-08-16T10:30:00Z",
    "updated_at": "2026-08-16T10:30:00Z"
  },
  "meal": {
    "id": "a4581437-47fa-4702-868c-e219e74fded6",
    "name": "Strawberry yogurt",
    "consumed_at": "2026-08-16T10:30:00Z",
    "calories": 380,
    "protein": 20,
    "carbs": 52,
    "fat": 10,
    "sodium": 240,
    "calcium": 480,
    "iron": 0.4
  }
}
```

The `meal` field is omitted for ingredient scans.

#### `POST /api/v1/inventory/{itemID}/consume`

Records an incremental consumed amount and optionally discards everything
still left after that amount.

```json
{
  "quantity_g": 150,
  "discard_remaining": true,
  "waste_reason": "leftover_not_eaten"
}
```

On the first food/product consumption, the provisional meal is resized to the
explicit consumed amount (or deleted when zero was consumed), and its
provisional link is cleared. Later food/product portions and every ingredient
portion create a new linked meal at the time consumption is recorded. When
`discard_remaining` is true, the remainder and its reason are written to waste
in the same transaction; otherwise the remainder stays in the pantry.

Success — `200 OK` returns the updated `inventory`, plus any affected `meal`,
`deleted_meal_id`, and `waste_event`:

```json
{
  "inventory": {
    "id": "6b327324-bdf0-434a-ab90-93cfbe7ee9d0",
    "quantity_purchased": 400,
    "quantity_consumed": 150,
    "quantity_wasted": 250,
    "is_resolved": true
  },
  "meal": {
    "id": "a4581437-47fa-4702-868c-e219e74fded6",
    "name": "Strawberry yogurt",
    "calories": 142.5
  },
  "waste_event": {
    "id": "73852d1a-56bd-4f35-b1a8-bdc2783c1756",
    "food_name": "Strawberry yogurt",
    "quantity_g": 250,
    "reason": "leftover_not_eaten",
    "impact_kg_co2e": 0.7875,
    "virtual_water_l": 250,
    "tree_equivalents": 0.013125,
    "impact_factor_version": "poore-nemecek-2018+wfn-global-average+epa-2024"
  }
}
```

The abbreviated nested objects above show fields relevant to reconciliation;
actual inventory, meal, and waste objects use their complete normal shapes.

#### Automatic expiry and impact

`GET /api/v1/inventory` and `GET /api/v1/waste-events` first reconcile stock
whose use-by or best-before date is earlier than the user's current business
date. That date is computed in `user_profiles.timezone`, falling back to
`Asia/Tokyo` to match the schema default. Each remaining quantity becomes one
expiry waste event, the inventory row is marked resolved, and any
still-provisional meal is deleted. Row locking and the resolved flag make
repeated or concurrent reconciliation idempotent. Resolved rows are omitted
from the inventory list but retained as history.

Every waste response adds estimates calculated from food category/name and
quantity:

```json
{
  "impact_kg_co2e": 0.7875,
  "virtual_water_l": 250,
  "tree_equivalents": 0.013125,
  "impact_factor_version": "poore-nemecek-2018+wfn-global-average+epa-2024"
}
```

The coefficients, sources, units, fallback behavior, and tree-year definition
are documented in [`ENVIRONMENTAL_IMPACT.md`](ENVIRONMENTAL_IMPACT.md). These
values are feedback estimates, not a product-specific lifecycle assessment.

Common lifecycle errors:

| Status | Meaning |
| --- | --- |
| `400` | Invalid UUID, enum, date/timestamp, quantity, nutrition, or discard reason |
| `401` | Missing or invalid Bearer token |
| `404` | The user does not own the referenced pantry item |
| `409` | Item is already resolved or the requested amount exceeds the remainder |
| `500` | Atomic persistence, reconciliation, or response reload failed |

### 4.8 Nutrition advice

#### `POST /api/v1/nutrients/advice`

Requires a valid Bearer token and forwards the prompt to Gemini.

Request:

```json
{
  "prompt": "How much protein should I eat per day?"
}
```

Success — `200 OK`:

```json
{
  "advice": "Aim for roughly 0.8 g of protein per kg of body weight."
}
```

Errors:

| Status | Body |
| --- | --- |
| `400` | `{"error":"invalid request body"}` |
| `400` | `{"error":"prompt is required"}` |
| `401` | Plain-text authentication error |
| `502` | `{"error":"advice generation failed"}` |
| `502` | `{"error":"could not parse advice"}` |
| `502` | `{"error":"advice result is empty"}` |
| `503` | `{"error":"advisor not configured"}` |

The Gemini system instruction asks for a JSON object. The handler parses and
validates that object before returning it, so the public response contains one
normal `advice` string rather than JSON serialized inside another string.

As with analysis, a missing `GEMINI_API_KEY` currently produces `502`, not
`503`, in the normal server configuration.

#### `POST /api/v1/nutrients/advice/panel`

Requires a valid Bearer token. Puts the same question to every configured provider at once and
returns the merged answer with each model's draft beside it. Registered only when a provider
key is configured.

Request:

```json
{
  "prompt": "How do I get more iron without red meat?"
}
```

Success — `200 OK`:

```json
{
  "answer": "Eat iron-rich non-red-meat foods like 1 cup of cooked lentils (6.6 mg)...",
  "merged": true,
  "totalTokens": 1431,
  "drafts": [
    {
      "agent": "gemini-draft",
      "provider": "gcp.gemini",
      "model": "gemini-2.5-flash",
      "answer": "...",
      "inputTokens": 56,
      "outputTokens": 1181
    },
    {
      "agent": "mistral-draft",
      "provider": "mistral",
      "model": "mistral-small-latest",
      "error": "unavailable",
      "inputTokens": 0,
      "outputTokens": 0
    }
  ]
}
```

`merged` is `false` when the merge step failed and `answer` is a single draft rather than a
combination — the caller is told which, instead of being handed one model's answer dressed up
as a consensus. A draft that failed carries `error` instead of `answer`; the provider's message
stays server-side, in the dependency logs and on the span.

Errors:

| Status | Body |
| --- | --- |
| `400` | `{"error":"invalid request body"}` |
| `400` | `{"error":"prompt is required"}` |
| `400` | `{"error":"prompt is too long"}` |
| `401` | Plain-text authentication error |
| `502` | `{"error":"the model panel is unavailable"}` — every agent failed |

### 4.9 Create a food from a nutrition label

#### `POST /api/v1/foods/from-label`

Requires a valid Bearer token. Extracts a product name, category, food type,
and per-100g nutrients from an image, then writes a `foods` row and matching
`food_nutrients` rows in one transaction.

Request:

```bash
curl http://localhost:8080/api/v1/foods/from-label \
  -H "Authorization: Bearer $TOKEN" \
  -F 'image=@nutrition-label.png;type=image/png'
```

Constraints:

- Multipart field name: `image`
- Maximum file size: 10 MiB
- The file must be non-empty.
- The part's content type, or detected content type when absent, must start
  with `image/`.
- Extracted nutrient codes must exist in the active nutrient master.
- Extracted amounts must be non-negative.
- `food_type` is saved as `raw_material` only for that exact value; every
  other model value is normalized to `prepared_food`.

Success — `201 Created`:

```json
{
  "food_id": "2c81c74e-c55b-47e6-9069-07a10e265c44",
  "name": "Protein Bar",
  "food_type": "prepared_food",
  "saved_nutrients": 8,
  "skipped_codes": ["unknown_nutrient"]
}
```

Unknown nutrient codes and negative amounts are skipped and reported in
`skipped_codes`; they do not fail the whole request.

Errors:

| Status | Body |
| --- | --- |
| `400` | Missing multipart field, unreadable upload, or empty image |
| `401` | Plain-text authentication error |
| `413` | `{"error":"image exceeds size limit"}` |
| `415` | `{"error":"uploaded file is not an image"}` |
| `422` | `{"error":"no product name found on the label"}` |
| `500` | Nutrient-master load or database persistence failure |
| `502` | Gemini extraction failure or invalid Gemini JSON |
| `503` | `{"error":"extractor not configured"}` |

The created private food is attributed through `foods.created_by` to the
authenticated user UUID.

### 4.10 Admin RBAC probe

#### `GET /api/v1/admin/ping`

Requires a valid JWT whose `roles` claim contains the exact string `admin`.

Success — `200 OK`:

```json
{
  "admin": "pong"
}
```

The current handler writes JSON text directly without setting
`Content-Type: application/json`.

Failures:

- `401 unauthorized` if the RBAC middleware has no claims context
- `401 missing bearer token` or `401 invalid token` from authentication
- `403 forbidden` when authenticated without the `admin` role

## 5. Frontend API layer

### 5.1 Instrumented JSON client

`frontend/src/api/client.ts` exports:

```ts
apiGet<T>(path: string): Promise<T>
apiPost<T>(path: string, body: unknown): Promise<T>
apiPut<T>(path: string, body: unknown): Promise<T>
apiDelete(path: string): Promise<void>
setToken(token: string | null): void
getToken(): string | null
```

Behavior:

- Reads the token from local storage key `auth_token` once when the module
  loads.
- `setToken` updates both the in-memory token and local storage.
- Prefixes every request with `VITE_API_BASE_URL`, or an empty string for
  same-origin requests.
- Sends `Authorization: Bearer <token>` when a token exists.
- Sends a new `traceparent` and the current `X-Session-Id` on every call.
- JSON-serializes every defined request body.
- Emits `api_request_started`, `api_request_completed`, and
  `api_request_failed` telemetry events.
- Parses JSON for successful responses and handles `204 No Content` without a
  response body.
- On a non-2xx response, throws `ApiError` — for example
  `POST /api/v1/auth/login failed: 401`. A JSON error envelope is parsed onto
  `error.body`, and its `code` is exposed as `error.code`, so callers can tell
  errors that share a status apart (a spent guest AI quota versus the shared
  rate limiter, both `429`). A plain-text or malformed body leaves `body`
  empty.
- Does not retry, refresh an expired token, or automatically log the user out
  after `401`.

### 5.2 Frontend call map

| Frontend source | Method and path | Trigger | Failure behavior |
| --- | --- | --- | --- |
| `accountService.getCurrentUser` | `GET /api/v1/me` | Startup with a persisted token, and after a sign-in that skipped that check | Clears the token on `401`; preserves it and presents a retry screen on network or server failure. After sign-in the identity is chrome, so a failure leaves the placeholder standing. |
| `accountService.updateDisplayName` | `PATCH /api/v1/me` | Save a new name on the account page | Keeps the editor open and reports that the name could not be saved. |
| `LoginView` | `POST /api/v1/auth/login` | Sign-in form submit | Shows “Invalid email or password” for every error type. |
| `nutritionService.analyzeFoodInput` | `POST /api/v1/nutrition/analyze` | Analyze text or a food photo | Text falls back to a deterministic estimate; image failures stay explicit. A spent guest quota throws `AiQuotaExceededError` in both modes. |
| `nutritionService.getAiQuota` | `GET /api/v1/nutrition/quota` | `AddEntryModal` opens for a guest | Reports the caller as unlimited, leaving the cap to the backend. |
| `persistenceService` | `GET /api/v1/inventory` | Authenticated startup and expiry reconciliation | Loads unresolved pantry stock before the dependent meal and waste ledgers. |
| `persistenceService` | `POST /api/v1/inventory/scans` | Save a reviewed food/product/ingredient scan | Saves pantry and provisional intake atomically. |
| `persistenceService` | `POST /api/v1/inventory/{itemID}/consume` | Save consumed quantity and optional remainder waste | Upserts/deletes the returned meal and removes resolved stock locally. |
| `persistenceService` | `GET/POST /api/v1/meals` | Load intake history | Uses persisted meals to drive Today and Stats. |
| `persistenceService` | `GET/PUT /api/v1/goals` | Startup and Account goal save | Retains the previous client state on failure. |
| `persistenceService` | `GET/POST /api/v1/waste-events` | Load or manually record waste | Aggregates impact fields in the Pantry view. |
| `nutritionService.getCompanionMessage` | `POST /api/v1/companion/message` | User pets the companion or nutrition totals change | Uses a local progress-based message. The backend route does not exist. |
| `telemetry/logger` | `POST /api/v1/telemetry/logs` | Queue flush after five seconds or page hide | Drops the batch on network failure and does not disturb the UI. |

The current frontend does **not** call:

```text
GET  /healthz
GET  /api/v1/nutrients
POST /api/v1/nutrients/advice
POST /api/v1/foods/from-label
GET  /api/v1/admin/ping
```

`Health`, `Nutrient`, and `LoginResponse` TypeScript interfaces exist in
`frontend/src/api/types.ts`, but only `LoginResponse` is used.

### 5.3 Food analysis fallback

If a **text** `/api/v1/nutrition/analyze` call fails, including rate limiting,
a missing Gemini key, or a network failure, the frontend returns one local
item. An image failure is surfaced to the user because a generic estimate
would falsely imply that the photo was analyzed.

An exhausted guest quota (`429` with `code: "guest_ai_daily_limit"`) is the
exception: it raises `AiQuotaExceededError` for text as well, since a local
estimate would hide the fact that no analysis ran.

```json
{
  "name": "Estimated item",
  "calories": 95,
  "protein": 0.5,
  "carbs": 25,
  "fat": 0.3,
  "sodium": 1,
  "calcium": 6,
  "iron": 0.1,
  "scanType": "<the selected type>",
  "quantityGrams": 100,
  "category": "other",
  "estimatedExpiryDays": 3
}
```

For text input, `name` becomes the first 40 characters of the trimmed input.
Default expiry is 3 days for food, 30 for a product, and 7 for an ingredient.

### 5.4 Companion request expected by the frontend

The frontend currently expects this unimplemented contract:

```http
POST /api/v1/companion/message
Content-Type: application/json
```

```json
{
  "stats": {
    "calories": 1200,
    "protein": 80,
    "carbs": 135,
    "fat": 38,
    "sodium": 900,
    "calcium": 500,
    "iron": 8
  },
  "goals": {
    "calories": 2200,
    "protein": 150,
    "carbs": 250,
    "fat": 70,
    "sodium": 2300,
    "calcium": 1000,
    "iron": 18
  }
}
```

Expected success shape:

```json
{
  "message": "Yummy progress! :3"
}
```

This shape is inferred from the frontend call and tests; there is no backend
handler or route implementing it.

### 5.5 Frontend telemetry transport

The telemetry sender is intentionally separate from the instrumented API
client to avoid recursively logging its own requests.

- Queues at most 100 events in memory.
- Sends at most 50 events per batch.
- Flushes after five seconds.
- Uses `fetch(..., { keepalive: true })` during normal flushes.
- Uses `navigator.sendBeacon` when the page is hidden or closes.
- Removes a batch from the queue before sending and does not requeue it on
  failure or non-2xx status.

The telemetry transport currently does not include `Authorization`,
`X-Session-Id`, or `traceparent` headers. Each event does include its own
`session_id`, and events associated with an instrumented API request include
their own trace fields.

## 6. Backend outbound Gemini API

The backend uses one Google endpoint for text and image generation:

```http
POST <GEMINI_BASE_URL>/models/<GEMINI_MODEL>:generateContent
x-goog-api-key: <GEMINI_API_KEY>
Content-Type: application/json
```

Defaults:

| Setting | Default |
| --- | --- |
| `GEMINI_BASE_URL` | `https://generativelanguage.googleapis.com/v1beta` |
| `GEMINI_MODEL` | `gemini-2.5-flash` |
| `GEMINI_TIMEOUT_SECONDS` | `30` |
| Maximum response read | 4 MiB |
| Retry count | 3 retries after the first attempt |

Text request shape:

```json
{
  "contents": [
    {
      "role": "user",
      "parts": [{ "text": "<prompt>" }]
    }
  ],
  "systemInstruction": {
    "parts": [{ "text": "<system instruction>" }]
  },
  "generationConfig": {
    "temperature": 0.2,
    "responseMimeType": "application/json"
  }
}
```

Image calls add a second part:

```json
{
  "inlineData": {
    "mimeType": "image/png",
    "data": "<base64>"
  }
}
```

Image generation uses temperature `0.1`.

The client reads the first text part of the first candidate. It treats a
reported prompt block, an empty candidate list, or malformed response JSON as
an error.

Network errors and HTTP `429`, `500`, `502`, `503`, and `504` are retried with
200 ms, 400 ms, and 800 ms backoffs. Other non-`200` statuses fail
immediately. Prompt and response text are not written to application logs.

## 7. Integration gaps and contract risks

These findings are implementation observations, not additional API promises.

| Priority | Finding | Impact |
| --- | --- | --- |
| High | The frontend calls `POST /api/v1/companion/message`, but the backend has no such route. | Every companion request falls back locally. |
| Resolved | The advice handler parses Gemini's structured JSON and rejects malformed or empty output with `502`. | Advice is returned as a single normal string without double encoding. |
| Resolved | `/api/v1/nutrition/analyze` caps JSON requests at 9 MiB and decoded images at 6 MiB, and validates detected JPEG/PNG/WebP content. | Image memory and request budget now have explicit bounds. |
| Resolved | On startup, the frontend treats a stored token as a session candidate and validates it with `/api/v1/me` before rendering private UI. | A `401` clears the token; temporary network/server failures preserve it and show a retry screen. |
| Medium | The telemetry sender omits the optional Bearer token, and `setTelemetryUserId` is never called. | Current frontend telemetry cannot be bound to an authenticated user by the backend. |
| Medium | Frontend file events send `file.mime_type` and `file.size_bytes`, but the ingest allowlist drops both fields. | File telemetry loses the metadata its frontend comments say is retained. |
| Medium | `VITE_ANALYZE_PATH` and `VITE_COMPANION_PATH` are declared in TypeScript but ignored; both paths are hard-coded. | Deployments cannot actually override those paths as the type comments suggest. |
| Resolved | Label and lifecycle-created foods set `created_by` from the authenticated UUID. | Private catalog rows remain attributable and authorization-aware. |
| Resolved | Expiry boundaries are calculated in `user_profiles.timezone`, with the schema's `Asia/Tokyo` default when no profile exists. | Server-host timezone no longer changes a user's expiry date. |
| Medium | Inventory-linked meals are protected from generic meal edits, but there is no dedicated consumption-correction endpoint yet. | A mistakenly submitted consumed quantity cannot currently be amended through the UI/API without a purpose-built ledger correction. |
| Medium | Handler errors are JSON, while auth, RBAC, and rate-limit errors are plain text. | Clients need two error parsers; the current frontend discards both bodies. |
| Low | The frontend has types for health and nutrients but no calls to those endpoints. | README claims about startup health/nutrient calls do not match the app. |
| Low | Missing `GEMINI_API_KEY` becomes `502` in normal wiring, although older README text described `503`. | Operators and clients may monitor or handle the wrong status. |
| Low | The admin ping writes JSON-looking text without an application/json content type. | Strict clients may not parse it as JSON. |

## 8. Source map

| Concern | Implementation |
| --- | --- |
| Route wiring and CORS | [`backend/internal/server/server.go`](../backend/internal/server/server.go) |
| Login and `/me` | [`backend/internal/handler/auth.go`](../backend/internal/handler/auth.go) |
| Nutrients and advice | [`backend/internal/handler/nutrients.go`](../backend/internal/handler/nutrients.go) |
| Food analysis | [`backend/internal/handler/nutrition.go`](../backend/internal/handler/nutrition.go) |
| Label extraction | [`backend/internal/handler/labels.go`](../backend/internal/handler/labels.go) |
| Pantry scan, consumption, and expiry | [`backend/internal/handler/inventory_lifecycle.go`](../backend/internal/handler/inventory_lifecycle.go) |
| Pantry reads and manual CRUD | [`backend/internal/handler/inventory.go`](../backend/internal/handler/inventory.go) |
| Waste ledger and impact response | [`backend/internal/handler/waste.go`](../backend/internal/handler/waste.go) |
| Environmental factors | [`backend/internal/handler/environmental_impact.go`](../backend/internal/handler/environmental_impact.go) |
| Lifecycle schema migration | [`backend/migrations/0003_inventory_lifecycle.sql`](../backend/migrations/0003_inventory_lifecycle.sql) |
| Telemetry ingest | [`backend/internal/handler/telemetry.go`](../backend/internal/handler/telemetry.go) |
| JWT and optional auth | [`backend/internal/middleware/auth.go`](../backend/internal/middleware/auth.go) |
| RBAC | [`backend/internal/middleware/rbac.go`](../backend/internal/middleware/rbac.go) |
| Rate limiting | [`backend/internal/middleware/ratelimit.go`](../backend/internal/middleware/ratelimit.go) |
| Gemini client | [`backend/internal/gemini/client.go`](../backend/internal/gemini/client.go) |
| Frontend JSON client | [`frontend/src/api/client.ts`](../frontend/src/api/client.ts) |
| Frontend AI services | [`frontend/src/services/nutritionService.ts`](../frontend/src/services/nutritionService.ts) |
| Frontend telemetry | [`frontend/src/telemetry/logger.ts`](../frontend/src/telemetry/logger.ts) |
| Frontend API types | [`frontend/src/api/types.ts`](../frontend/src/api/types.ts) |
