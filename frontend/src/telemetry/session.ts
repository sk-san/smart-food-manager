import { randomHex } from "./ids";

const KEY = "sfm.session_id";

// Browser-session identifier (session_id on every log entry), stable for
// the lifetime of the tab session.
export function getSessionId(): string {
  let id = sessionStorage.getItem(KEY);
  if (!id) {
    id = `sess-${randomHex(8)}`;
    sessionStorage.setItem(KEY, id);
  }
  return id;
}
