import { reactive, computed } from "vue";
import { lspService } from "@/api/services";
import { appState } from "@/stores/app";
import { pushOutput } from "@/stores/output";
import type {
  LSPServerStatus,
  LSPCompletionRequest,
  LSPCompletionItem,
  LSPLocation,
  LSPTextEdit,
} from "@/types";

/**
 * G-FEAT-02: LSP store — manages offline language server status and provides
 * completions to the Monaco editor.
 *
 * Coexistence with AI inline completion:
 *   - AI uses registerInlineCompletionsProvider (ghost text)
 *   - LSP uses registerCompletionItemProvider (popup list)
 * These are different Monaco APIs and do not conflict by design.
 *
 * Graceful fallback: all query methods return empty results when the server is
 * not running or not installed, so the editor degrades smoothly.
 */

export interface LSPState {
  /** Per-language server status, keyed by "go" | "typescript" | "javascript". */
  statuses: Record<string, LSPServerStatus>;
  /** True while a detect/start/stop operation is in flight. */
  busy: boolean;
  /** Whether LSP-backed completion is enabled (bound to settings). */
  enabled: boolean;
}

export const lspState = reactive<LSPState>({
  statuses: {},
  busy: false,
  enabled: true,
});

/** Supported languages for LSP completion. */
export type LSPLanguage = "go" | "typescript" | "javascript";

/** Returns true if any language server is available on this machine. */
export const anyLSPAvailable = computed(() =>
  Object.values(lspState.statuses).some((s) => s.available),
);

/** Returns true if any language server is currently running. */
export const anyLSPRunning = computed(() =>
  Object.values(lspState.statuses).some((s) => s.running),
);

/**
 * prompt-8 Task 8-D: compact StatusBar label for LSP.
 * e.g. "LSP: gopls ✓" / "LSP: offline" / "LSP: error".
 */
export const lspStatusLabel = computed(() => {
  const list = Object.values(lspState.statuses);
  if (list.length === 0) return "LSP: —";
  const running = list.filter((s) => s.running);
  if (running.length > 0) {
    const kinds = running.map((s) => s.serverKind || s.language).join(",");
    return `LSP: ${kinds}`;
  }
  const err = list.find((s) => s.lastError);
  if (err?.lastError) return "LSP: error";
  const avail = list.some((s) => s.available);
  return avail ? "LSP: idle" : "LSP: n/a";
});

export const lspStatusDetail = computed(() => {
  return Object.values(lspState.statuses)
    .map((s) => {
      const st = s.running ? "running" : s.available ? "available" : "missing";
      const kind = s.serverKind || s.language;
      const err = s.lastError ? ` (${s.lastError})` : "";
      return `${s.language}: ${kind} ${st}${err}`;
    })
    .join(" · ");
});

/**
 * Detect installed language servers and populate lspState.statuses.
 * Safe to call repeatedly; does not start any server.
 */
export async function detectLSPServers(): Promise<void> {
  lspState.busy = true;
  try {
    const statuses = await lspService.detectServers();
    lspState.statuses = {};
    for (const st of statuses) {
      lspState.statuses[st.language] = st;
    }
  } catch (e) {
    // Backend may be unavailable in tests / early startup — fail silently.
    pushOutput("ide", "warn", `LSP detect failed: ${e instanceof Error ? e.message : String(e)}`);
  } finally {
    lspState.busy = false;
  }
}

/**
 * Start the LSP server for the given language. No-op if already running or
 * not installed. Returns true on success.
 */
export async function startLSPServer(language: string): Promise<boolean> {
  const st = lspState.statuses[language];
  if (st?.running) return true;
  if (st && !st.available) return false;
  lspState.busy = true;
  try {
    await lspService.startServer(language);
    if (lspState.statuses[language]) {
      lspState.statuses[language].running = true;
    }
    return true;
  } catch (e) {
    pushOutput("ide", "warn", `LSP start ${language} failed: ${e instanceof Error ? e.message : String(e)}`);
    return false;
  } finally {
    lspState.busy = false;
  }
}

/**
 * Stop a running LSP server. No-op if not running.
 */
export async function stopLSPServer(language: string): Promise<void> {
  if (!lspState.statuses[language]?.running) return;
  try {
    await lspService.stopServer(language);
    if (lspState.statuses[language]) {
      lspState.statuses[language].running = false;
    }
  } catch (e) {
    pushOutput("ide", "warn", `LSP stop ${language} failed: ${e instanceof Error ? e.message : String(e)}`);
  }
}

/**
 * Ensure the LSP server for a language is running, starting it lazily if it is
 * available but not yet started. Returns true if the server is running (or
 * became running), false if it is unavailable.
 */
export async function ensureLSPRunning(language: string): Promise<boolean> {
  const st = lspState.statuses[language];
  if (st?.running) return true;
  if (!st?.available) return false;
  return startLSPServer(language);
}

/**
 * Map a Monaco language id to the LSP language key. Monaco uses "typescript"
 * and "javascript" directly; Go is "go".
 */
export function monacoLanguageToLSP(monacoLang: string): string | null {
  const map: Record<string, string> = {
    go: "go",
    typescript: "typescript",
    javascript: "javascript",
  };
  return map[monacoLang] ?? null;
}

/**
 * Query the LSP server for completions at a position. Returns an empty list
 * if the server is not running or the language is unsupported — never throws.
 *
 * This is the function the Monaco completion provider calls. It auto-starts
 * the server on first use (lazy start) when an installed server is detected.
 */
/** prompt-9 9-E / prompt-10 10-K: request sequence — ignore stale responses. */
let completionSeq = 0;
let hoverSeq = 0;
let definitionSeq = 0;

export async function getLSPCompletions(
  language: string,
  filePath: string,
  line: number,
  column: number,
  content: string,
): Promise<LSPCompletionItem[]> {
  if (!lspState.enabled) return [];
  const lspLang =
    language === "go" || language === "typescript" || language === "javascript"
      ? language
      : monacoLanguageToLSP(language);
  if (!lspLang) return [];

  const seq = ++completionSeq;
  const running = await ensureLSPRunning(lspLang);
  if (!running) {
    pushOutput("LSP", "warn", `${lspLang}: not_running`);
    return [];
  }

  const req: LSPCompletionRequest = {
    language: lspLang,
    filePath,
    line,
    column,
    content,
  };
  try {
    const items = await lspService.getCompletions(req);
    // 9-E: drop out-of-order results
    if (seq !== completionSeq) return [];
    return items;
  } catch (e) {
    if (seq === completionSeq) {
      pushOutput("LSP", "error", `${lspLang}: rpc ${e instanceof Error ? e.message : String(e)}`);
    }
    return [];
  }
}

/**
 * Query the LSP server for hover info at a position. Returns "" if the server
 * is not running or the language is unsupported — never throws.
 */
export async function getLSPHover(
  language: string,
  filePath: string,
  line: number,
  column: number,
  content: string,
): Promise<string> {
  if (!lspState.enabled) return "";
  const lspLang =
    language === "go" || language === "typescript" || language === "javascript"
      ? language
      : monacoLanguageToLSP(language) ?? language;
  if (!lspLang || !["go", "typescript", "javascript"].includes(lspLang)) return "";

  const seq = ++hoverSeq;
  const running = await ensureLSPRunning(lspLang);
  if (!running) return "";

  const req: LSPCompletionRequest = { language: lspLang, filePath, line, column, content };
  try {
    const text = await lspService.getHover(req);
    if (seq !== hoverSeq) return "";
    return text;
  } catch {
    return "";
  }
}

/** prompt-8 Task 8-F + prompt-10 10-K seq cancel */
export async function getLSPDefinition(
  language: string,
  filePath: string,
  line: number,
  column: number,
  content: string,
): Promise<LSPLocation[]> {
  if (!lspState.enabled) return [];
  const lspLang = language;
  const seq = ++definitionSeq;
  if (!(await ensureLSPRunning(lspLang))) return [];
  try {
    const locs = await lspService.getDefinition({ language: lspLang, filePath, line, column, content });
    if (seq !== definitionSeq) return [];
    return locs;
  } catch {
    return [];
  }
}

/**
 * prompt-10 10-D: pull publishDiagnostics cache into Problems panel.
 */
export async function refreshDiagnosticsToProblems(
  language: string,
  filePath: string,
  content: string,
): Promise<void> {
  try {
    if (!(await ensureLSPRunning(language))) return;
    const diags = await lspService.getDiagnostics({
      language,
      filePath,
      line: 0,
      column: 0,
      content,
    });
    const { clearProblems, pushProblem } = await import("@/stores/output");
    // Keep other files' problems; drop this file's previous entries by filtering
    const { outputState } = await import("@/stores/output");
    const rest = outputState.problems.filter(
      (p) => p.file !== filePath && !filePath.endsWith(p.file) && !p.file.endsWith(filePath),
    );
    outputState.problems = rest;
    for (const d of diags ?? []) {
      const sev =
        d.severity === 1 ? "error" : d.severity === 2 ? "warning" : d.severity === 3 ? "info" : "hint";
      pushProblem(sev, filePath, (d.line ?? 0) + 1, (d.column ?? 0) + 1, d.message, d.source || language);
    }
  } catch {
    /* best-effort */
  }
}

/** prompt-8 Task 8-F */
export async function getLSPReferences(
  language: string,
  filePath: string,
  line: number,
  column: number,
  content: string,
): Promise<LSPLocation[]> {
  if (!lspState.enabled) return [];
  if (!(await ensureLSPRunning(language))) return [];
  try {
    return await lspService.getReferences({ language, filePath, line, column, content });
  } catch {
    return [];
  }
}

/** prompt-8 Task 8-G */
export async function formatLSPDocument(
  language: string,
  filePath: string,
  content: string,
): Promise<LSPTextEdit[]> {
  if (!lspState.enabled) return [];
  if (!(await ensureLSPRunning(language))) return [];
  try {
    return await lspService.formatDocument({
      language,
      filePath,
      line: 0,
      column: 0,
      content,
    });
  } catch {
    return [];
  }
}

/** prompt-8 Task 8-A: didClose when tab closes. */
export async function closeLSPDocument(language: string, filePath: string): Promise<void> {
  try {
    await lspService.closeDocument(language, filePath);
  } catch {
    /* best-effort */
  }
}

/** prompt-9 9-B multi-file rename */
export async function renameSymbolWorkspace(
  language: string,
  filePath: string,
  line: number,
  column: number,
  content: string,
  newName: string,
): Promise<Array<{ filePath: string; edits: LSPTextEdit[] }>> {
  if (!(await ensureLSPRunning(language))) return [];
  try {
    return await lspService.renameSymbolWorkspace(
      { language, filePath, line, column, content },
      newName,
    );
  } catch (e) {
    pushOutput("LSP", "error", `rename: ${e instanceof Error ? e.message : String(e)}`);
    return [];
  }
}

/** prompt-9 9-G */
export async function getLSPSignatureHelp(
  language: string,
  filePath: string,
  line: number,
  column: number,
  content: string,
): Promise<{
  label: string;
  documentation: string;
  parameters: string[];
  activeParameter: number;
  activeSignature: number;
} | null> {
  if (!(await ensureLSPRunning(language))) return null;
  try {
    return await lspService.getSignatureHelp({ language, filePath, line, column, content });
  } catch {
    return null;
  }
}

/** prompt-9 9-G organize imports */
export async function organizeLSPImports(
  language: string,
  filePath: string,
  content: string,
): Promise<LSPTextEdit[]> {
  if (!(await ensureLSPRunning(language))) return [];
  try {
    return await lspService.organizeImports({
      language,
      filePath,
      line: 0,
      column: 0,
      content,
    });
  } catch {
    return [];
  }
}

/**
 * Stop all running LSP servers. Called on app shutdown or project switch.
 */
export async function stopAllLSPServers(): Promise<void> {
  for (const lang of Object.keys(lspState.statuses)) {
    if (lspState.statuses[lang].running) {
      await stopLSPServer(lang);
    }
  }
}

/**
 * Initialize the LSP store: detect installed servers. Called once during app
 * bootstrap. Errors are swallowed (best-effort).
 */
export async function initLSPStore(): Promise<void> {
  await detectLSPServers();
}

/**
 * Test-only helper: reset the store to its initial state.
 */
export function __resetLSPStoreForTesting(): void {
  lspState.statuses = {};
  lspState.busy = false;
  lspState.enabled = true;
}

// Re-export appState touch so computed re-evaluate when the project changes.
// When a project is opened, the backend workspace root changes and previously
// detected servers may need re-detection. The caller (MainLayout / project
// open flow) should call detectLSPServers() after a project opens.
export { appState };
