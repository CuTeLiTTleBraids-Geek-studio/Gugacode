import { reactive } from "vue";
import { appState } from "@/stores/app";

/**
 * G-FEAT-02: Offline detection.
 *
 * Tracks network connectivity so the UI can:
 *   - Show a "离线补全" (offline completion) badge in the status bar
 *   - Disable the AI send button when offline
 *   - Let LSP-based completion keep working offline
 *
 * Signals:
 *   1. navigator.onLine + window online/offline events (primary, instant)
 *   2. Periodic heartbeat to the AI BaseURL (best-effort reachability check)
 *
 * The heartbeat uses a no-cors fetch with a short timeout. If the fetch fails
 * (network error, CSP block, or timeout), the online state falls back to
 * navigator.onLine. This never throws — it only updates connectivityState.
 */

export interface ConnectivityState {
  /** Whether the network is online (navigator.onLine + heartbeat). */
  online: boolean;
  /** Whether the AI BaseURL responded to the last heartbeat. */
  aiReachable: boolean;
  /** True while a heartbeat probe is in flight. */
  checking: boolean;
}

export const connectivityState = reactive<ConnectivityState>({
  online: typeof navigator !== "undefined" ? navigator.onLine : true,
  aiReachable: true,
  checking: false,
});

/** Heartbeat interval (ms). 30s balances responsiveness with resource use. */
const HEARTBEAT_INTERVAL_MS = 30_000;
/** Heartbeat request timeout (ms). */
const HEARTBEAT_TIMEOUT_MS = 5_000;

let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
let onlineListener: (() => void) | null = null;
let offlineListener: (() => void) | null = null;
let initialised = false;

/**
 * Probe the AI BaseURL for reachability. Uses no-cors mode so the request
 * succeeds (opaque response) if the server is reachable, regardless of auth.
 * Returns true if reachable, false otherwise. Never throws.
 */
export async function checkAIReachable(): Promise<boolean> {
  const baseUrl = appState.aiBaseUrl;
  if (!baseUrl) return false;
  connectivityState.checking = true;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), HEARTBEAT_TIMEOUT_MS);
  try {
    // no-cors: the response is opaque but the promise resolves if the server
    // is reachable. A network error or CSP block rejects the promise.
    await fetch(baseUrl, {
      method: "HEAD",
      mode: "no-cors",
      cache: "no-store",
      signal: controller.signal,
    });
    return true;
  } catch {
    // AbortError, network error, or CSP block — server not reachable from
    // the webview. Fall back to navigator.onLine for the online state.
    return false;
  } finally {
    clearTimeout(timer);
    connectivityState.checking = false;
  }
}

/**
 * Update the online state from navigator.onLine and a heartbeat probe.
 * Called on init, on online/offline events, and on each heartbeat tick.
 */
async function refreshOnlineState(): Promise<void> {
  const navOnline = typeof navigator !== "undefined" ? navigator.onLine : true;
  // If the browser reports offline, we're definitely offline.
  if (!navOnline) {
    connectivityState.online = false;
    connectivityState.aiReachable = false;
    return;
  }
  // Browser reports online — probe the AI BaseURL for a more precise signal.
  // If no BaseURL is configured, trust navigator.onLine.
  if (!appState.aiBaseUrl) {
    connectivityState.online = true;
    connectivityState.aiReachable = true;
    return;
  }
  const reachable = await checkAIReachable();
  connectivityState.aiReachable = reachable;
  // Stay online if either navigator says online OR the heartbeat succeeded.
  // We use navigator.onLine as the authority for the `online` flag so that
  // a single failed heartbeat doesn't falsely show "offline" when the network
  // is actually up (the AI server might just be slow or behind a firewall).
  connectivityState.online = navOnline;
}

/**
 * Initialize the connectivity listener. Sets up online/offline event
 * listeners and starts the periodic heartbeat. Idempotent — safe to call
 * multiple times (subsequent calls are no-ops).
 *
 * Call once during app bootstrap (after loadSettings so aiBaseUrl is set).
 */
export function initConnectivityListener(): void {
  if (initialised) return;
  initialised = true;
  if (typeof window === "undefined") return;

  const handleOnline = () => {
    void refreshOnlineState();
  };
  const handleOffline = () => {
    connectivityState.online = false;
    connectivityState.aiReachable = false;
  };

  window.addEventListener("online", handleOnline);
  window.addEventListener("offline", handleOffline);
  onlineListener = handleOnline;
  offlineListener = handleOffline;

  // Start the periodic heartbeat.
  heartbeatTimer = setInterval(() => {
    void refreshOnlineState();
  }, HEARTBEAT_INTERVAL_MS);

  // Do an initial check.
  void refreshOnlineState();
}

/**
 * Stop the connectivity listener and clean up event listeners + timer.
 * Intended for HMR teardown in dev and test cleanup.
 */
export function stopConnectivityListener(): void {
  if (onlineListener && typeof window !== "undefined") {
    window.removeEventListener("online", onlineListener);
    onlineListener = null;
  }
  if (offlineListener && typeof window !== "undefined") {
    window.removeEventListener("offline", offlineListener);
    offlineListener = null;
  }
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
  initialised = false;
}

/**
 * Test-only helper: reset the connectivity state and initialisation flag.
 */
export function __resetConnectivityForTesting(): void {
  stopConnectivityListener();
  connectivityState.online = typeof navigator !== "undefined" ? navigator.onLine : true;
  connectivityState.aiReachable = true;
  connectivityState.checking = false;
}
