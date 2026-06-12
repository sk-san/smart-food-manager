// Typed helpers for the frontend event catalog (UX events, browser errors).
// API request events are emitted by the instrumented client in src/api.
import { logEvent } from "./logger";

export function logScreenView(screen: string): void {
  logEvent({
    message: `Screen viewed: ${screen}`,
    name: "screen_view",
    category: "ux",
    action: screen,
    outcome: "success",
    attrs: { "screen.name": screen },
  });
}

export function logNavigation(from: string, to: string): void {
  logEvent({
    message: `User navigated from ${from} to ${to}`,
    name: "navigation",
    category: "ux",
    action: `${from}_to_${to}`,
    outcome: "success",
    attrs: { "screen.from": from, "screen.to": to },
  });
}

export function logScreenRendered(screen: string, durationMs?: number): void {
  logEvent({
    message: `Screen rendered: ${screen}`,
    name: "screen_rendered",
    category: "ux",
    action: screen,
    outcome: "success",
    durationMs,
    attrs: { "screen.name": screen },
  });
}

/** Wires window error/unhandledrejection to frontend_error events. */
export function installGlobalErrorLogging(): void {
  window.addEventListener("error", (ev) => {
    logEvent({
      severity: "ERROR",
      message: "Unhandled error",
      name: "frontend_error",
      category: "ux",
      action: "unhandled_error",
      outcome: "failure",
      attrs: {
        "error.type": ev.error instanceof Error ? ev.error.name : "Error",
        "error.message": trim(ev.message),
      },
    });
  });
  window.addEventListener("unhandledrejection", (ev) => {
    const reason: unknown = ev.reason;
    logEvent({
      severity: "ERROR",
      message: "Unhandled promise rejection",
      name: "frontend_error",
      category: "ux",
      action: "unhandled_rejection",
      outcome: "failure",
      attrs: {
        "error.type": reason instanceof Error ? reason.name : typeof reason,
        "error.message": trim(reason instanceof Error ? reason.message : String(reason)),
      },
    });
  });
}

function trim(s: string): string {
  return s.length > 512 ? s.slice(0, 512) : s;
}
