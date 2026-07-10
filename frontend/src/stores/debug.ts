/**
 * prompt-11 11-A: in-IDE DAP client store (Delve dlv dap).
 */
import { reactive } from "vue";
import { debugService } from "@/api/services";
import { pushOutput } from "@/stores/output";
import { notifyError, notifySuccess, notifyInfo } from "@/lib/notifications";
import { appState } from "@/stores/app";

export interface DebugBreakpoint {
  id: number;
  file: string;
  line: number;
  verified: boolean;
}

export interface DebugStackFrame {
  id: number;
  name: string;
  file: string;
  line: number;
  column: number;
}

export interface DebugVariable {
  name: string;
  value: string;
  type: string;
}

export const debugState = reactive({
  available: false,
  running: false,
  stopped: false,
  address: "",
  mode: "",
  message: "",
  stopReason: "",
  busy: false,
  breakpoints: [] as DebugBreakpoint[],
  stack: [] as DebugStackFrame[],
  locals: [] as DebugVariable[],
  pollTimer: 0 as number | ReturnType<typeof setInterval>,
});

export async function refreshDebugStatus(): Promise<void> {
  try {
    debugState.available = await debugService.isAvailable();
    const st = await debugService.getState();
    applySnapshot(st);
  } catch {
    debugState.available = false;
  }
}

function applySnapshot(st: {
  session: {
    running: boolean;
    address: string;
    mode: string;
    message: string;
    stopped?: boolean;
    stopReason?: string;
  };
  breakpoints?: DebugBreakpoint[];
  stack?: DebugStackFrame[];
  locals?: DebugVariable[];
}): void {
  debugState.running = !!st.session?.running;
  debugState.address = st.session?.address || "";
  debugState.mode = st.session?.mode || "";
  debugState.message = st.session?.message || "";
  debugState.stopped = !!st.session?.stopped;
  debugState.stopReason = st.session?.stopReason || "";
  debugState.breakpoints = st.breakpoints || [];
  debugState.stack = st.stack || [];
  debugState.locals = st.locals || [];
}

function startPolling(): void {
  stopPolling();
  debugState.pollTimer = setInterval(() => {
    void debugService.getState().then(applySnapshot).catch(() => undefined);
  }, 400);
}

function stopPolling(): void {
  if (debugState.pollTimer) {
    clearInterval(debugState.pollTimer as number);
    debugState.pollTimer = 0;
  }
}

export async function launchDebugPackage(): Promise<void> {
  const dir = appState.currentProject || "";
  if (!dir) {
    notifyError("Open a Go project first");
    return;
  }
  debugState.busy = true;
  try {
    const session = await debugService.launchPackage(dir);
    debugState.running = session.running;
    debugState.address = session.address;
    debugState.mode = session.mode;
    debugState.message = session.message;
    debugState.stopped = !!session.stopped;
    pushOutput("Debug", "info", session.message);
    notifySuccess("Debug session started (in-IDE DAP)");
    startPolling();
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
    pushOutput("Debug", "error", String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function launchDebugTest(runRegex: string): Promise<void> {
  const dir = appState.currentProject || "";
  if (!dir) {
    notifyError("Open a Go project first");
    return;
  }
  debugState.busy = true;
  try {
    const session = await debugService.launchTest(dir, runRegex);
    debugState.running = session.running;
    debugState.address = session.address;
    debugState.mode = session.mode;
    debugState.message = session.message;
    debugState.stopped = !!session.stopped;
    pushOutput("Debug", "info", session.message + (runRegex ? ` run=${runRegex}` : ""));
    notifySuccess(`Debug test: ${runRegex || "(all)"}`);
    startPolling();
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

/** prompt-11 11-G: debug the test under cursor. */
export async function debugTestAtCursor(
  language: string,
  filePath: string,
  line: number,
  content: string,
): Promise<void> {
  if (language !== "go") {
    notifyError("Debug Test at Cursor currently supports Go only");
    return;
  }
  // Resolve test name via toolchain backend pattern: reuse RunTestAtCursor name finder via API if needed.
  // Frontend falls back to simple regex.
  const lines = content.split(/\r?\n/);
  let parent = "";
  let sub = "";
  const funcRe = /^\s*func\s+(Test[A-Za-z0-9_]+)/;
  const runRe = /\bt\.Run\(\s*['"`]([^'"`]+)['"`]/;
  const max = Math.min(line, lines.length - 1);
  for (let i = 0; i <= max; i++) {
    const m = lines[i].match(funcRe);
    if (m) {
      parent = m[1];
      sub = "";
    }
    const r = lines[i].match(runRe);
    if (r) sub = r[1];
  }
  const regex = parent ? (sub ? `${parent}/${sub}` : parent) : "";
  if (!regex) {
    notifyError("No TestXxx found at cursor");
    return;
  }
  // package dir = directory of test file
  const dir = filePath.replace(/[\\/][^\\/]+$/, "") || appState.currentProject || "";
  debugState.busy = true;
  try {
    const session = await debugService.launchTest(dir, `^${regex}$`);
    debugState.running = session.running;
    debugState.message = session.message;
    pushOutput("Debug", "info", `Debug Test at Cursor: ${regex}`);
    notifySuccess(`Debugging ${regex}`);
    startPolling();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function stopDebugSession(): Promise<void> {
  try {
    stopPolling();
    await debugService.stop();
    debugState.running = false;
    debugState.stopped = false;
    debugState.address = "";
    debugState.mode = "";
    debugState.stack = [];
    debugState.locals = [];
    notifyInfo("Debug session stopped");
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function debugContinue(): Promise<void> {
  try {
    await debugService.continue();
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function debugStepOver(): Promise<void> {
  try {
    await debugService.stepOver();
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function debugStepIn(): Promise<void> {
  try {
    await debugService.stepIn();
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function debugStepOut(): Promise<void> {
  try {
    await debugService.stepOut();
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function toggleBreakpoint(file: string, line: number): Promise<void> {
  try {
    const bps = await debugService.toggleBreakpoint(file, line);
    debugState.breakpoints = bps || [];
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export function breakpointsForFile(filePath: string): DebugBreakpoint[] {
  if (!filePath) return [];
  const norm = filePath.replace(/\\/g, "/").toLowerCase();
  return debugState.breakpoints.filter((b) => {
    const f = (b.file || "").replace(/\\/g, "/").toLowerCase();
    return f === norm || f.endsWith("/" + norm.split("/").pop()) || norm.endsWith(f);
  });
}

export async function selectDebugFrame(frameId: number): Promise<void> {
  try {
    await debugService.selectFrame(frameId);
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function refreshStackAndLocals(): Promise<void> {
  try {
    await debugService.refreshStackAndLocals();
    await refreshDebugStatus();
  } catch {
    /* ignore */
  }
}
