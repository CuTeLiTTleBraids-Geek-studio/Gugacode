/**
 * prompt-6 Task 1 — per-webview origin id for dual-window sync.
 *
 * Main editor and AI companion are independent Webviews. Events such as
 * settings:changed / conversation:saved are broadcast app-wide; each window
 * tags its own emissions with this id and ignores echoes of itself to avoid
 * reload loops.
 */
const STORAGE_KEY = "gugacode.windowOriginId";

function createOriginId(): string {
  const rand =
    typeof crypto !== "undefined" && typeof crypto.randomUUID === "function"
      ? crypto.randomUUID()
      : `${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
  return `win_${rand}`;
}

/** Stable origin id for the current Webview process (sessionStorage when available). */
export function getWindowOriginId(): string {
  try {
    if (typeof sessionStorage !== "undefined") {
      const existing = sessionStorage.getItem(STORAGE_KEY);
      if (existing) return existing;
      const id = createOriginId();
      sessionStorage.setItem(STORAGE_KEY, id);
      return id;
    }
  } catch {
    // sessionStorage may throw in private mode / tests
  }
  // Module-level fallback for jsdom / tests without sessionStorage.
  if (!(globalThis as { __gugaWindowOrigin?: string }).__gugaWindowOrigin) {
    (globalThis as { __gugaWindowOrigin?: string }).__gugaWindowOrigin = createOriginId();
  }
  return (globalThis as { __gugaWindowOrigin?: string }).__gugaWindowOrigin!;
}

/** Unwrap Wails event data (may be raw value, {data}, or array-wrapped). */
export function unwrapEventData(event: unknown): unknown {
  if (event == null) return event;
  if (typeof event === "object" && event !== null && "data" in event) {
    const raw = (event as { data?: unknown }).data;
    return Array.isArray(raw) ? raw[0] : raw;
  }
  return Array.isArray(event) ? event[0] : event;
}

export function parseSyncOrigin(payload: unknown): string {
  if (payload && typeof payload === "object" && "origin" in payload) {
    const o = (payload as { origin?: unknown }).origin;
    return typeof o === "string" ? o : "";
  }
  return "";
}
