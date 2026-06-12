// Structured frontend telemetry per the logging blueprint: every event is
// a common envelope (service identity, event triple, trace context,
// session/user context) plus event-specific fields. Events are batched and
// relayed through the backend telemetry endpoint into the shared logging
// pipeline — the browser never talks to Loki directly.
import { getSessionId } from "./session";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "";
const ENDPOINT = `${BASE}/api/v1/telemetry/logs`;
const SERVICE_NAME = "frontend-web";
const SERVICE_VERSION = import.meta.env.VITE_APP_VERSION ?? "0.1.0";
const ENVIRONMENT = import.meta.env.MODE;
const FLUSH_INTERVAL_MS = 5000;
const MAX_QUEUE = 100;
const MAX_BATCH = 50;

export type Severity = "DEBUG" | "INFO" | "WARNING" | "ERROR";
export type Outcome = "started" | "success" | "failure";
export type Category = "ux" | "http" | "auth" | "file" | "account";

export interface LogEventInput {
  severity?: Severity; // defaults to INFO
  message: string;
  name: string; // event.name from the catalog — unknown names are dropped server-side
  category: Category;
  action: string;
  outcome: Outcome;
  traceId?: string;
  spanId?: string;
  sampled?: boolean;
  durationMs?: number;
  attrs?: Record<string, string | number | boolean>;
}

let userId: string | null = null;

/** Set after login with the internal (non-PII) user identifier; never an email. */
export function setTelemetryUserId(id: string | null): void {
  userId = id;
}

type Envelope = Record<string, string | number | boolean>;

const queue: Envelope[] = [];
let timer: number | null = null;

export function logEvent(e: LogEventInput): void {
  const envelope: Envelope = {
    time: new Date().toISOString(),
    severity: e.severity ?? "INFO",
    message: e.message,
    source: "frontend",
    "service.name": SERVICE_NAME,
    "service.version": SERVICE_VERSION,
    "deployment.environment": ENVIRONMENT,
    "event.name": e.name,
    "event.category": e.category,
    "event.action": e.action,
    "event.outcome": e.outcome,
    session_id: getSessionId(),
    "client.locale": navigator.language,
    "client.timezone": Intl.DateTimeFormat().resolvedOptions().timeZone,
    "client.viewport_width": window.innerWidth,
    "client.viewport_height": window.innerHeight,
    ...e.attrs,
  };
  if (e.traceId) {
    envelope.trace_id = e.traceId;
    if (e.spanId) envelope.span_id = e.spanId;
    envelope.trace_sampled = e.sampled ?? true;
  }
  if (userId) envelope.user_id = userId;
  if (e.durationMs != null) envelope.duration_ms = Math.round(e.durationMs);

  queue.push(envelope);
  if (queue.length > MAX_QUEUE) queue.splice(0, queue.length - MAX_QUEUE);
  schedule();
}

function schedule(): void {
  if (timer !== null) return;
  timer = window.setTimeout(() => {
    timer = null;
    void flush();
  }, FLUSH_INTERVAL_MS);
}

async function flush(): Promise<void> {
  if (queue.length === 0) return;
  const events = queue.splice(0, MAX_BATCH);
  try {
    // Plain fetch on purpose: the instrumented API client would emit its
    // own api_request_* events and recurse.
    await fetch(ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ events }),
      keepalive: true,
    });
  } catch {
    // Telemetry is best-effort: drop the batch rather than disturb the app.
  }
  if (queue.length > 0) schedule();
}

// Flush synchronously when the tab is hidden or closing.
function flushNow(): void {
  if (queue.length === 0) return;
  const events = queue.splice(0, MAX_BATCH);
  const body = new Blob([JSON.stringify({ events })], { type: "application/json" });
  navigator.sendBeacon(ENDPOINT, body);
}

window.addEventListener("pagehide", flushNow);
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") flushNow();
});
