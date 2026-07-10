/**
 * prompt-10 10-G: Delve headless DAP session store.
 */
import { reactive } from "vue";
import { debugService } from "@/api/services";
import { pushOutput } from "@/stores/output";
import { notifyError, notifySuccess, notifyInfo } from "@/lib/notifications";
import { appState } from "@/stores/app";

export const debugState = reactive({
  available: false,
  running: false,
  address: "",
  mode: "",
  message: "",
  busy: false,
});

export async function refreshDebugStatus(): Promise<void> {
  try {
    debugState.available = await debugService.isAvailable();
    const session = await debugService.getSession();
    debugState.running = !!session.running;
    debugState.address = session.address || "";
    debugState.mode = session.mode || "";
    debugState.message = session.message || "";
  } catch {
    debugState.available = false;
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
    pushOutput("Debug", "info", session.message);
    notifySuccess(`Delve listening on ${session.address}`);
    notifyInfo("Attach a DAP client (VS Code / GoLand) to this address");
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
    pushOutput("Debug", "info", session.message);
    notifySuccess(`Delve test session on ${session.address}`);
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    debugState.busy = false;
  }
}

export async function stopDebugSession(): Promise<void> {
  try {
    await debugService.stop();
    debugState.running = false;
    debugState.address = "";
    debugState.mode = "";
    notifyInfo("Debug session stopped");
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}
