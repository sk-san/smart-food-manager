/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  /** service.version stamped on telemetry events; defaults to 0.1.0. */
  readonly VITE_APP_VERSION?: string;
  /** Backend path for food analysis; defaults to /api/v1/nutrition/analyze. */
  readonly VITE_ANALYZE_PATH?: string;
  /** Backend path for companion messages; defaults to /api/v1/companion/message. */
  readonly VITE_COMPANION_PATH?: string;
}
interface ImportMeta {
  readonly env: ImportMetaEnv;
}
