import { computed } from "vue";
import { appState, saveSettings } from "@/stores/app";
import { aiService } from "@/api/services";

/**
 * Minimum milliseconds between completion requests per file (N-43).
 * Per-file debounce prevents A tab's request from blocking B tab's request.
 */
const DEBOUNCE_MS = 300;

/** Minimum prefix length before requesting a completion. */
const MIN_PREFIX_LENGTH = 10;

/**
 * Whether inline completion is enabled. Bound to appState so the setting is
 * persisted (N-7). Use this in templates/computed for reactivity.
 */
export const inlineCompletionEnabled = computed(() => appState.inlineCompletionEnabled);

/**
 * N-43: Per-file last-request timestamps for debounce. Keys are filePaths,
 * values are epoch milliseconds. Using a Map (instead of a single global
 * timestamp) prevents A tab's request from blocking B tab's request when
 * the user is editing multiple files.
 */
const lastRequestByFile = new Map<string, number>();

/**
 * N-43: The currently in-flight completion request's promise + abort
 * controller. Concurrent callers share this promise so we don't fire
 * duplicate HTTP requests for the same (prefix, suffix, language, filePath).
 * When a new request arrives, the previous AbortController is aborted,
 * cancelling the old Wails binding call.
 */
interface InFlight {
  promise: Promise<string>;
  controller: AbortController | null;
  // Signature used to dedup: if a concurrent call has the same signature,
  // it reuses the in-flight promise instead of starting a new request.
  signature: string;
}
let inFlight: InFlight | null = null;

/**
 * N-43: Abort the in-flight request (if any) and clear it. Called when a
 * new request starts so the old request's result is discarded (preventing
 * stale ghost text from flashing in after the user has typed more).
 */
function abortInFlight(): void {
  if (inFlight) {
    if (inFlight.controller) {
      inFlight.controller.abort();
    }
    inFlight = null;
  }
}

/**
 * Request an inline completion from the AI service.
 * Returns the completion text or empty string if no completion is available.
 *
 * N-43 (Proposal M):
 * - Per-file debounce: lastRequestByFile tracks the last request time per
 *   filePath, so editing file A doesn't block requests for file B.
 * - AbortController: each new request aborts the previous one, preventing
 *   stale ghost text from flashing in after the user types more.
 * - Dedup: concurrent callers with the same (prefix, suffix, language,
 *   filePath) signature reuse the in-flight promise, avoiding duplicate
 *   HTTP requests.
 */
export async function requestCompletion(
  prefix: string,
  suffix: string,
  language: string,
  filePath: string
): Promise<string> {
  if (!appState.inlineCompletionEnabled) return "";
  if (prefix.length < MIN_PREFIX_LENGTH) return "";

  // N-43: Dedup — check BEFORE debounce. If a concurrent caller already
  // started the same request, reuse its promise regardless of the debounce
  // window. This is safe because reusing an in-flight promise costs nothing
  // (no new HTTP request). This must come before the debounce check so
  // that rapid re-render passes (which fire requestCompletion for the same
  // position) get the in-flight result instead of "".
  const signature = `${filePath}\0${prefix}\0${suffix}\0${language}`;
  if (inFlight && inFlight.signature === signature) {
    return inFlight.promise;
  }

  // N-43: Per-file debounce. Check AFTER dedup so that an in-flight request
  // with the same signature is reused even within the debounce window.
  const now = Date.now();
  const last = lastRequestByFile.get(filePath) ?? 0;
  if (now - last < DEBOUNCE_MS) return "";
  lastRequestByFile.set(filePath, now);

  // N-43: Abort any previous in-flight request before starting a new one.
  // This prevents stale completions from flashing in after the user types
  // more (the old request's result is discarded via AbortController).
  abortInFlight();

  const controller = new AbortController();
  const promise = (async () => {
    try {
      const response = await aiService.complete(
        { prefix, suffix, language, filePath },
        controller.signal,
      );
      return response?.text ?? "";
    } catch {
      // Silently fail — inline completion is best-effort. This includes
      // AbortError when the request was cancelled by a newer request.
      return "";
    } finally {
      // Clear inFlight only if it's still us — a newer request may have
      // already replaced it. Check by controller identity (rather than
      // promise identity) to avoid the temporal-dead-zone issue of
      // referencing `promise` inside its own initializer.
      if (inFlight?.controller === controller) {
        inFlight = null;
      }
    }
})();

  inFlight = { promise, controller, signature };
  return promise;
}

/**
 * Toggle inline completion on/off and persist the setting (N-7).
 */
export function toggleInlineCompletion(): void {
  appState.inlineCompletionEnabled = !appState.inlineCompletionEnabled;
  saveSettings();
}

/**
 * N-43: Cancel any in-flight completion request. Called when the user
 * explicitly dismisses ghost text or switches files, so the pending
 * HTTP request is aborted rather than completing silently.
 */
export function cancelInlineCompletion(): void {
  abortInFlight();
}

/**
 * Test-only helper: reset the per-file debounce Map and clear any
 * in-flight request. Exported so tests can isolate themselves from
 * each other without relying on fake timers.
 */
export function __resetInlineCompletionForTesting(): void {
  lastRequestByFile.clear();
  abortInFlight();
}
