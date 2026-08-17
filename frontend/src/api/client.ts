// Thin fetch wrapper. In dev, requests are same-origin and proxied to the Go
// backend by Vite (see vite.config.ts). In production, set VITE_API_BASE_URL.
//
// Every call carries `traceparent` and `X-Session-Id` headers and emits
// api_request_* telemetry events, so frontend events and backend logs,
// spans, and metrics share one trace per user action.
import { newTraceContext, traceparent } from "../telemetry/ids";
import { getSessionId } from "../telemetry/session";
import { logEvent } from "../telemetry/logger";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "";

const TOKEN_STORAGE_KEY = "auth_token";

let authToken: string | null = localStorage.getItem(TOKEN_STORAGE_KEY);

export class ApiError extends Error {
  constructor(
    public readonly method: string,
    public readonly path: string,
    public readonly status: number,
  ) {
    super(`${method} ${path} failed: ${status}`);
    this.name = "ApiError";
  }
}

export function setToken(token: string | null): void {
  authToken = token;
  if (token) {
    localStorage.setItem(TOKEN_STORAGE_KEY, token);
  } else {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
  }
}

export function getToken(): string | null {
  return authToken;
}

export async function apiGet<T>(path: string): Promise<T> {
  return request<T>("GET", path);
}

export async function apiPost<T>(path: string, body: unknown): Promise<T> {
  return request<T>("POST", path, body);
}

export async function apiPut<T>(path: string, body: unknown): Promise<T> {
  return request<T>("PUT", path, body);
}

export async function apiDelete(path: string): Promise<void> {
  await request<void>("DELETE", path);
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const tc = newTraceContext();
  const action = `${method} ${path}`;
  const httpAttrs = { "http.request.method": method, "http.route": path };
  const started = performance.now();

  logEvent({
    message: `API request started: ${action}`,
    name: "api_request_started",
    category: "http",
    action,
    outcome: "started",
    traceId: tc.traceId,
    spanId: tc.spanId,
    attrs: httpAttrs,
  });

  const headers: Record<string, string> = {
    traceparent: traceparent(tc),
    "X-Session-Id": getSessionId(),
  };
  if (authToken) headers.Authorization = `Bearer ${authToken}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(`${BASE}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (err) {
    logEvent({
      severity: "ERROR",
      message: `API request failed: ${action}`,
      name: "api_request_failed",
      category: "http",
      action,
      outcome: "failure",
      traceId: tc.traceId,
      spanId: tc.spanId,
      durationMs: performance.now() - started,
      attrs: { ...httpAttrs, "error.type": "NetworkError" },
    });
    throw err;
  }

  if (!res.ok) {
    logEvent({
      severity: "ERROR",
      message: `API request failed: ${action}`,
      name: "api_request_failed",
      category: "http",
      action,
      outcome: "failure",
      traceId: tc.traceId,
      spanId: tc.spanId,
      durationMs: performance.now() - started,
      attrs: { ...httpAttrs, "http.response.status_code": res.status },
    });
    throw new ApiError(method, path, res.status);
  }

  logEvent({
    message: `API request completed: ${action}`,
    name: "api_request_completed",
    category: "http",
    action,
    outcome: "success",
    traceId: tc.traceId,
    spanId: tc.spanId,
    durationMs: performance.now() - started,
    attrs: { ...httpAttrs, "http.response.status_code": res.status },
  });
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}
