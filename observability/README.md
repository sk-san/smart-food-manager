# Observability & Structured Logging

Vendor-neutral structured logging, tracing, and metrics across the React
frontend and Go backend, correlated through shared identifiers
(`trace_id`, `span_id`, `request_id`, `session_id`, `user_id`).

## Pipeline

```
React frontend ──(batched UX/event logs)──> POST /api/v1/telemetry/logs ─┐
       │  traceparent + X-Session-Id headers                            │
       v                                                                 v
Go backend ──(OTLP gRPC: logs, traces, metrics)──> OTel Collector
                                                      ├── logs    → Loki
                                                      ├── traces  → Tempo
                                                      └── metrics → Prometheus
                                                            └── Grafana (correlates all three)
```

Run it: `make obs-up` (Grafana at http://localhost:3000, anonymous admin).

## Log schema

Every entry is a common envelope plus event-specific fields:

```json
{
  "severity": "INFO",
  "message": "Request completed",
  "source": "backend",
  "service.name": "backend-api",
  "service.version": "0.1.0",
  "deployment.environment": "development",
  "event.name": "request_completed",
  "event.category": "http",
  "event.action": "GET /api/v1/nutrients",
  "event.outcome": "success",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "trace_sampled": true,
  "request_id": "host/abc123-000001",
  "session_id": "sess-1a2b3c4d",
  "user_id": "sha256:…",
  "http.route": "/api/v1/nutrients",
  "http.response.status_code": 200,
  "duration_ms": 184
}
```

The record timestamp carries `time`. Severities are `DEBUG INFO NOTICE
WARNING ERROR CRITICAL`; the `severity` attribute is authoritative (the
OTel bridge approximates NOTICE/CRITICAL in its numeric level).

Implementation: backend `internal/logging` (envelope, catalog, emit
helpers), frontend `src/telemetry` (session ID, traceparent, batched
logger). Frontend events reach Loki via the backend telemetry endpoint,
which validates against the catalog, allowlists fields, takes `user_id`
only from a verified token, and hashes the client IP.

## Event catalog

Backend: `request_received|completed|failed`,
`auth_login_started|completed|failed`, `auth_logout_completed`,
`user_account_delete_requested|completed|failed`,
`file_upload_started|completed|failed`,
`external_api_call_started|completed|failed` (use
`logging.StartExternalCall` for dependency calls such as the Gemini API).

Frontend: `screen_view`, `navigation`, `screen_rendered`,
`frontend_error`, `api_request_started|completed|failed`,
`login_clicked`, `logout_clicked`, `delete_account_requested|confirmed`,
`file_upload_started|completed|failed`.

Names are stable identifiers — dashboards and the ingestion allowlist key
on them.

## Label & cardinality policy

Loki indexed labels (set via collector hint processors):
`service_name`, `deployment_environment`, `source`, `severity`,
`event_category`, `event_name`, `event_outcome`, `dependency_provider`,
`dependency_service`. Everything else — especially `trace_id`,
`request_id`, `session_id`, `user_id`, hashes — stays in the log body.
The same rule applies to metric dimensions: route templates
(`/api/items/{item_id}`), methods, status codes, providers, outcomes only.

## Metrics

`http_server_requests_total`, `http_server_request_duration_ms`,
`auth_login_attempts_total`, `external_api_requests_total`,
`external_api_duration_ms`, `llm_input_tokens_total`,
`llm_output_tokens_total`, `file_uploads_total`,
`file_upload_duration_ms` — exported under the `foodapp_` namespace
(defined in `internal/telemetry/metrics.go`).

## Never log

Passwords, tokens, auth headers, cookies, API keys, raw prompts or LLM
responses, file contents or original filenames, base64 images, email
addresses, phone numbers, addresses, card data. Log metadata instead:
token counts, `sha256:` hashes (salted via `LOG_HASH_SALT`), sizes, MIME
types. `error.stack` only at ERROR+ severity and only after redaction.

## Dashboards

Provisioned automatically into the **Smart Food Manager** Grafana folder
from `grafana/provisioning/dashboards/foodapp/` (edits in the UI are
allowed but files are the source of truth; the provider re-reads every
30s):

- **API Overview** (`foodapp-api-overview`) — request rate, 4xx/5xx
  rates, latency quantiles, per-route breakdowns; filterable by route.
- **Dependencies & AI** (`foodapp-dependencies-ai`) — external API call
  rate/failures/latency by provider and service, LLM token throughput,
  file uploads, plus a live tail of dependency failures. The LLM-token
  and file-upload panels stay empty until handlers record those
  instruments (defined in `internal/telemetry/metrics.go` but not yet
  wired into any handler).
- **Logs, Auth & Errors** (`foodapp-logs-auth`) — log volume by
  severity, error events by name, login attempts by outcome, frontend
  events, and error log tails whose TraceID links open Tempo.

## Example queries

```logql
{service_name="backend-api", event_name="request_failed"} | json
{source="frontend", event_category="ux"} | json | session_id = `sess-…`
```

Log lines expose a TraceID derived field that links to the Tempo trace;
Tempo links back to Loki via trace-to-logs.
