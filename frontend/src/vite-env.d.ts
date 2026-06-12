/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  /** service.version stamped on telemetry events; defaults to 0.1.0. */
  readonly VITE_APP_VERSION?: string;
}
interface ImportMeta {
  readonly env: ImportMetaEnv;
}
