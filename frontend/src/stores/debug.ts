/**
 * prompt-11/12: in-IDE DAP client store (Delve + Node MVP).
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
  condition?: string;
  logMessage?: string;
  message?: string;
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

export interface DebugLaunchConfig {
  name: string;
  kind: "package" | "test" | "node";
  dir: string;
  program?: string;
  runRegex?: string;
  args?: string[];
  env?: Record<string, string>;
  stopOnEntry?: boolean;
  mode?: string;
}

const LAUNCH_CFG_KEY = "gugacode.debug.launchConfigs";

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
  watches: [] as DebugVariable[],
  watchInput: "",
  evaluateInput: "",
  evaluateResult: "" as string,
  launchConfigs: [] as DebugLaunchConfig[],
  activeConfigName: "" as string,
  pollTimer: 0 as number | ReturnType<typeof setInterval>,
});

export function loadLaunchConfigs(): void {
  try {
    const raw = localStorage.getItem(LAUNCH_CFG_KEY);
    if (raw) {
      debugState.launchConfigs = JSON.parse(raw) as DebugLaunchConfig[];
    }
  } catch {
    debugState.launchConfigs = [];
  }
  if (!debugState.launchConfigs.length) {
    debugState.launchConfigs = [
      {
        name: "Go: Package",
        kind: "package",
        dir: "",
        mode: "debug",
      },
      {
        name: "Go: Test package",
        kind: "test",
        dir: "",
        mode: "test",
      },
    ];
  }
}

export function saveLaunchConfigs(): void {
  try {
    localStorage.setItem(LAUNCH_CFG_KEY, JSON.stringify(debugState.launchConfigs));
  } catch {
    /* ignore */
  }
}

export function upsertLaunchConfig(cfg: DebugLaunchConfig): void {
  const i = debugState.launchConfigs.findIndex((c) => c.name === cfg.name);
  if (i >= 0) debugState.launchConfigs[i] = cfg;
  else debugState.launchConfigs.push(cfg);
  saveLaunchConfigs();
}

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
  watches?: DebugVariable[];
  stopReason?: string;
}): void {
  debugState.running = !!st.session?.running;
  debugState.address = st.session?.address || "";
  debugState.mode = st.session?.mode || "";
  debugState.message = st.session?.message || "";
  debugState.stopped = !!st.session?.stopped;
  debugState.stopReason = st.session?.stopReason || st.stopReason || "";
  debugState.breakpoints = st.breakpoints || [];
  debugState.stack = st.stack || [];
  debugState.locals = st.locals || [];
  debugState.watches = st.watches || [];
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
    applySession(session);
    pushOutput("Debug", "info", session.message);
    notifySuccess("Debug session started (DAP)");
    startPolling();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
    pushOutput("Debug", "error", String(e));
  } finally {
    debugState.busy = false;
  }
}

function applySession(session: {
  running: boolean;
  address: string;
  mode: string;
  message: string;
  stopped?: boolean;
  stopReason?: string;
}): void {
  debugState.running = session.running;
  debugState.address = session.address;
  debugState.mode = session.mode;
  debugState.message = session.message;
  debugState.stopped = !!session.stopped;
  debugState.stopReason = session.stopReason || "";
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
    applySession(session);
    pushOutput("Debug", "info", session.message + (runRegex ? ` run=${runRegex}` : ""));
    notifySuccess(`Debug test: ${runRegex || "(all)"}`);
    startPolling();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function launchWithConfig(cfg: DebugLaunchConfig): Promise<void> {
  const dir = cfg.dir || appState.currentProject || "";
  debugState.busy = true;
  try {
    if (cfg.kind === "node") {
      const prog = cfg.program || "";
      if (!prog) {
        notifyError("Node launch needs program path");
        return;
      }
      const session = await debugService.launchNode(prog, cfg.args || []);
      applySession(session);
    } else if (cfg.kind === "test") {
      const session = await debugService.launchTest(dir, cfg.runRegex || "");
      applySession(session);
    } else {
      const session = await debugService.launchPackage(dir);
      applySession(session);
    }
    debugState.activeConfigName = cfg.name;
    notifySuccess(`Launched: ${cfg.name}`);
    startPolling();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function launchNodeProgram(program: string, args: string[] = []): Promise<void> {
  debugState.busy = true;
  try {
    const session = await debugService.launchNode(program, args);
    applySession(session);
    notifySuccess("Node inspect-brk started");
    pushOutput("Debug", "info", session.message);
    startPolling();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function debugTestAtCursor(
  language: string,
  filePath: string,
  line: number,
  content: string,
): Promise<void> {
  if (language !== "go") {
    // 12-F: node test debug via launch node on file when JS
    if (language === "typescript" || language === "javascript") {
      await launchNodeProgram(filePath, []);
      return;
    }
    notifyError("Debug Test at Cursor: Go or TS/JS file");
    return;
  }
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
  const dir = filePath.replace(/[\\/][^\\/]+$/, "") || appState.currentProject || "";
  debugState.busy = true;
  try {
    const session = await debugService.launchTest(dir, `^${regex}$`);
    applySession(session);
    pushOutput("Debug", "info", `Debug Test at Cursor: ${regex}`);
    notifySuccess(`Debugging ${regex}`);
    startPolling();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function restartDebugSession(): Promise<void> {
  debugState.busy = true;
  try {
    const session = await debugService.restart();
    applySession(session);
    notifySuccess("Debug session restarted");
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
    debugState.stopReason = "";
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

export async function setBreakpointCondition(file: string, line: number, condition: string): Promise<void> {
  try {
    await debugService.setBreakpointCondition(file, line, condition);
    await refreshDebugStatus();
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export function breakpointsForFile(filePath: string): DebugBreakpoint[] {
  if (!filePath) return [];
  const norm = filePath.replace(/\\/g, "/").toLowerCase();
  return debugState.breakpoints.filter((b) => {
    const f = (b.file || "").replace(/\\/g, "/").toLowerCase();
    return f === norm || norm.endsWith(f) || f.endsWith(norm);
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

export async function addWatch(expr: string): Promise<void> {
  try {
    const list = await debugService.addWatch(expr);
    debugState.watches = list || [];
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function removeWatch(expr: string): Promise<void> {
  try {
    const list = await debugService.removeWatch(expr);
    debugState.watches = list || [];
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function evaluateExpression(expr: string): Promise<void> {
  try {
    const v = await debugService.evaluate(expr);
    debugState.evaluateResult = `${v.name} = ${v.value}${v.type ? ` (${v.type})` : ""}`;
  } catch (e) {
    debugState.evaluateResult = String(e);
  }
}

loadLaunchConfigs();
