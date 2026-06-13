// W3C Trace Context helpers. The frontend generates trace/span IDs and
// propagates them via the `traceparent` header so backend spans and logs
// join the same trace as the originating browser action.

export interface TraceContext {
  traceId: string;
  spanId: string;
  sampled: boolean;
}

export function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes);
  crypto.getRandomValues(buf);
  return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("");
}

export function newTraceContext(): TraceContext {
  return { traceId: randomHex(16), spanId: randomHex(8), sampled: true };
}

export function traceparent(tc: TraceContext): string {
  return `00-${tc.traceId}-${tc.spanId}-${tc.sampled ? "01" : "00"}`;
}
