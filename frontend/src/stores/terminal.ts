import { reactive } from "vue";
import { terminalService } from "@/api/services";
import { Events } from "@wailsio/runtime";
import type { TerminalOutputPayload, TerminalExitedPayload } from "@/types";

export interface TerminalSessionState {
  id: string;
  output: string;
  running: boolean;
  cols: number;
  rows: number;
}

export interface TerminalStoreState {
  sessions: Record<string, TerminalSessionState>;
  sessionOrder: string[];
  activeSessionId: string | null;
}

export const terminalState = reactive<TerminalStoreState>({
  sessions: {},
  sessionOrder: [],
  activeSessionId: null,
});

let eventListenerRegistered = false;

// Per-session output listeners, used by runCommandInSession to detect
// exit-code sentinels without polling session.output.
type OutputListener = (data: string) => void;
const outputListeners = new Map<string, Set<OutputListener>>();

// N-47: Per-session exit listeners, used by runCommandInSession to detect
// when the backend PTY has exited (so it can return -1 immediately instead
// of waiting for the 5-minute timeout).
type ExitListener = () => void;
const exitListeners = new Map<string, Set<ExitListener>>();

// N-149: Wails Events.On returns a cancel function. Collected here so the
// global listeners can be torn down during HMR / tests to avoid duplicates.
const terminalEventCancellers: Array<() => void> = [];

/**
 * Register a callback that fires whenever terminal output arrives for the
 * given session. Returns an unsubscribe function. Used by
 * runCommandInSession to watch for the exit-code sentinel.
 */
export function onTerminalOutput(sessionId: string, cb: OutputListener): () => void {
  if (!outputListeners.has(sessionId)) {
    outputListeners.set(sessionId, new Set());
  }
  outputListeners.get(sessionId)!.add(cb);
  return () => {
    outputListeners.get(sessionId)?.delete(cb);
  };
}

/**
 * N-47: Register a callback that fires when the backend PTY for the given
 * session exits. Returns an unsubscribe function. Used by
 * runCommandInSession to return promptly when the PTY dies mid-step,
 * instead of waiting for the sentinel that will never arrive.
 */
export function onSessionExit(sessionId: string, cb: ExitListener): () => void {
  if (!exitListeners.has(sessionId)) {
    exitListeners.set(sessionId, new Set());
  }
  exitListeners.get(sessionId)!.add(cb);
  return () => {
    exitListeners.get(sessionId)?.delete(cb);
  };
}

function ensureEventListener() {
  if (eventListenerRegistered) return;
  eventListenerRegistered = true;
  // N-44: typed event payload (was `any`). Wails' built-in Events.On
  // typing uses a generic string-map for object payloads, so we cast
  // event.data to our specific TerminalOutputPayload shape inside the
  // callback. The payload contract is documented in services/events.go.
  terminalEventCancellers.push(
    Events.On("terminal:output", (event) => {
      const payload = (event?.data ?? {}) as Partial<TerminalOutputPayload>;
      const sessionId = payload.sessionId ?? "";
      const data = payload.data ?? "";
      if (sessionId && typeof data === "string") {
        const session = terminalState.sessions[sessionId];
        if (session) {
          session.output += data;
        }
        // Notify registered output listeners.
        const listeners = outputListeners.get(sessionId);
        if (listeners) {
          for (const cb of listeners) cb(data);
        }
      }
    }),
  );
  // N-47: Listen for terminal:exited events emitted by the backend when
  // the PTY process exits. Marks the session as not running and notifies
  // any registered exit listeners (e.g. runCommandInSession waiting for
  // a sentinel that will never arrive).
  // N-44: typed event payload (was `any`).
  terminalEventCancellers.push(
    Events.On("terminal:exited", (event) => {
      const payload = (event?.data ?? {}) as Partial<TerminalExitedPayload>;
      const sessionId = payload.sessionId ?? "";
      if (!sessionId) return;
      const session = terminalState.sessions[sessionId];
      if (session) {
        session.running = false;
      }
      const listeners = exitListeners.get(sessionId);
      if (listeners) {
        for (const cb of listeners) cb();
      }
    }),
  );
}

/**
 * N-149: Cancels all terminal event listeners. Intended for HMR teardown
 * in dev and test cleanup. After calling this, ensureEventListener() can
 * be invoked again to re-register fresh listeners.
 */
export function cleanupTerminalEventListeners(): void {
  for (const cancel of terminalEventCancellers) {
    try {
      cancel();
    } catch {
      // ignore — listener already removed
    }
  }
  terminalEventCancellers.length = 0;
  eventListenerRegistered = false;
}

function generateSessionId(): string {
  return (
    "term-" +
    Date.now().toString(36) +
    "-" +
    Math.random().toString(36).slice(2, 6)
  );
}

export async function createSession(
  workingDir: string,
  shell: string = "",
): Promise<string> {
  ensureEventListener();
  const id = generateSessionId();
  try {
    await terminalService.startSession(id, workingDir, shell);
    terminalState.sessions[id] = {
      id,
      output: "",
      running: true,
      cols: 80,
      rows: 24,
    };
    terminalState.sessionOrder.push(id);
    terminalState.activeSessionId = id;
    return id;
  } catch (e) {
    console.error("Failed to create terminal session:", e);
    return "";
  }
}

export async function writeToSession(
  sessionId: string,
  input: string,
): Promise<void> {
  const session = terminalState.sessions[sessionId];
  if (!session || !session.running) return;
  try {
    await terminalService.writeSession(sessionId, input);
  } catch (e) {
    console.error("Failed to write to terminal:", e);
  }
}

export async function killSession(sessionId: string): Promise<void> {
  try {
    await terminalService.killSession(sessionId);
  } catch (e) {
    console.error("Failed to kill terminal:", e);
  }
  delete terminalState.sessions[sessionId];
  terminalState.sessionOrder = terminalState.sessionOrder.filter(
    (id) => id !== sessionId,
  );
  if (terminalState.activeSessionId === sessionId) {
    terminalState.activeSessionId = terminalState.sessionOrder[0] ?? null;
  }
}

export async function resizeSession(
  sessionId: string,
  cols: number,
  rows: number,
): Promise<void> {
  const session = terminalState.sessions[sessionId];
  if (!session) return;
  session.cols = cols;
  session.rows = rows;
  if (!session.running) return;
  try {
    await terminalService.resizeSession(sessionId, cols, rows);
  } catch (e) {
    console.error("Failed to resize terminal:", e);
  }
}

export function setActiveSession(sessionId: string): void {
  terminalState.activeSessionId = sessionId;
}

export function getActiveSession(): TerminalSessionState | null {
  if (!terminalState.activeSessionId) return null;
  return terminalState.sessions[terminalState.activeSessionId] ?? null;
}

export function clearSessionOutput(sessionId: string): void {
  const session = terminalState.sessions[sessionId];
  if (session) {
    session.output = "";
  }
}

// ---------------------------------------------------------------------------
// Exit-code detection (Plan 61 / N-24)
// ---------------------------------------------------------------------------

const EXIT_SENTINEL_PREFIX = "__NKNK_EXIT_";

/**
 * Returns true if the current platform is Windows. Used to select the
 * correct shell variable for exit-code capture ($LASTEXITCODE on
 * PowerShell, $? on bash).
 */
function isWindowsPlatform(): boolean {
  if (typeof navigator !== "undefined" && navigator.platform) {
    return navigator.platform.indexOf("Win") >= 0;
  }
  return false;
}

/**
 * Wraps a shell command with a sentinel echo that reports the exit code.
 * The sentinel format is: __NKNK_EXIT_<marker>_<code>__
 *
 * On Windows (PowerShell), $LASTEXITCODE captures the exit code of native
 * commands. For cmdlets where $LASTEXITCODE is null, we fall back to $?
 * (True→0, False→1).
 *
 * On Unix (bash/sh), $? captures the exit code directly.
 */
export function wrapCommandWithExitMarker(command: string, marker: string): string {
  const sentinel = `${EXIT_SENTINEL_PREFIX}${marker}_`;
  if (isWindowsPlatform()) {
    return `${command}; $c = $LASTEXITCODE; if ($c -eq $null) { $c = if ($?) { 0 } else { 1 } }; echo "${sentinel}$c__"`;
  }
  return `${command}; echo "${sentinel}$?__"`;
}

/**
 * Searches a buffer for the exit-code sentinel and returns the parsed
 * exit code, or null if the sentinel hasn't appeared yet.
 */
export function extractExitCode(buffer: string, marker: string): number | null {
  const regex = new RegExp(`${EXIT_SENTINEL_PREFIX}${marker}_(\\d+)__`);
  const match = buffer.match(regex);
  if (match) {
    return parseInt(match[1], 10);
  }
  return null;
}

/**
 * Runs a command in a terminal session and waits for the exit-code
 * sentinel. Returns the exit code (0 = success), or -1 if the session
 * is not running or the command times out.
 *
 * The command is wrapped with a unique sentinel echo so the exit code
 * can be detected from the terminal output stream. This replaces the
 * old "dispatch and mark success" pattern (N-24).
 */
export async function runCommandInSession(
  sessionId: string,
  command: string,
  timeoutMs: number = 300000,
): Promise<number> {
  const session = terminalState.sessions[sessionId];
  if (!session || !session.running) {
    return -1;
  }

  const marker = Math.random().toString(36).slice(2, 10);
  const wrapped = wrapCommandWithExitMarker(command, marker);

  return new Promise<number>((resolve) => {
    let resolved = false;
    let buffer = "";

    const off = onTerminalOutput(sessionId, (data) => {
      if (resolved) return;
      buffer += data;
      const code = extractExitCode(buffer, marker);
      if (code !== null) {
        resolved = true;
        cleanup();
        resolve(code);
      }
    });

    // N-47: If the PTY exits before the sentinel arrives, resolve
    // immediately with -1 instead of waiting for the 5-minute timeout.
    const offExit = onSessionExit(sessionId, () => {
      if (resolved) return;
      resolved = true;
      cleanup();
      resolve(-1);
    });

    const timer = setTimeout(() => {
      if (!resolved) {
        resolved = true;
        cleanup();
        resolve(-1);
      }
    }, timeoutMs);

    function cleanup() {
      off();
      offExit();
      clearTimeout(timer);
    }

    void writeToSession(sessionId, wrapped + "\n");
  });
}

/**
 * Proposal F (prompt-5.md): Run a command in a terminal session and
 * capture both the exit code AND the stdout/stderr output. The output
 * is the raw terminal stream with the sentinel marker line stripped.
 *
 * Used by workflow steps that declare `outputs` templates — the host
 * parses the captured stdout to extract values like version strings,
 * commit hashes, etc.
 *
 * Returns { exitCode, output }. On timeout, exitCode is -1 and output
 * contains whatever was captured before the timeout.
 */
export async function runCommandInSessionCapturing(
  sessionId: string,
  command: string,
  timeoutMs: number = 300000,
): Promise<{ exitCode: number; output: string }> {
  const session = terminalState.sessions[sessionId];
  if (!session || !session.running) {
    return { exitCode: -1, output: "" };
  }

  const marker = Math.random().toString(36).slice(2, 10);
  const wrapped = wrapCommandWithExitMarker(command, marker);

  return new Promise<{ exitCode: number; output: string }>((resolve) => {
    let resolved = false;
    let buffer = "";

    const off = onTerminalOutput(sessionId, (data) => {
      if (resolved) return;
      buffer += data;
      const code = extractExitCode(buffer, marker);
      if (code !== null) {
        resolved = true;
        cleanup();
        const cleaned = stripExitMarker(buffer, marker);
        resolve({ exitCode: code, output: cleaned });
      }
    });

    // N-47: If the PTY exits before the sentinel arrives, resolve
    // immediately with -1 instead of waiting for the 5-minute timeout.
    const offExit = onSessionExit(sessionId, () => {
      if (resolved) return;
      resolved = true;
      cleanup();
      resolve({ exitCode: -1, output: stripExitMarker(buffer, marker) });
    });

    const timer = setTimeout(() => {
      if (!resolved) {
        resolved = true;
        cleanup();
        resolve({ exitCode: -1, output: stripExitMarker(buffer, marker) });
      }
    }, timeoutMs);

    function cleanup() {
      off();
      offExit();
      clearTimeout(timer);
    }

    void writeToSession(sessionId, wrapped + "\n");
  });
}

/**
 * Proposal F: Strip the sentinel marker line (and anything after it)
 * from the captured terminal output. The marker is the random string
 * inserted by wrapCommandWithExitMarker to delimit the exit code.
 */
function stripExitMarker(buffer: string, marker: string): string {
  const markerIdx = buffer.indexOf(`__NKNK_EXIT_${marker}`);
  if (markerIdx === -1) return buffer;
  return buffer.slice(0, markerIdx);
}
