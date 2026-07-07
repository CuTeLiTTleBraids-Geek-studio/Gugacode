/**
 * errorMessage coerces a caught value (typed as `unknown` under strict
 * TypeScript catch variables) into a human-readable string. This is the
 * shared helper for the `catch (e: unknown)` refactor (N-4) — every
 * catch site that previously did `e?.message ?? String(e)` should call
 * this instead, so the narrowing lives in one place.
 *
 * Behavior:
 *   - Error instances → their .message
 *   - strings → returned as-is
 *   - anything else → String(value), which handles numbers, objects, etc.
 *
 * The function never throws: if .message access fails for any reason
 * (e.g. a malformed object throwing in a getter), it falls back to
 * String(e).
 */
export function errorMessage(e: unknown): string {
  if (e instanceof Error) {
    return e.message;
  }
  if (typeof e === "string") {
    return e;
  }
  try {
    return String(e);
  } catch {
    return "(unknown error)";
  }
}
