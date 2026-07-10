/**
 * prompt-10/11/12: test explorer with go test -json + unified Run/Coverage/Debug (12-J).
 */
import { reactive } from "vue";
import { fileService, toolchainService } from "@/api/services";
import { appState } from "@/stores/app";
import { runTestAtCursor } from "@/stores/toolchain";
import { openFileFromPath } from "@/stores/editor";
import { pushOutput } from "@/stores/output";
import { notifyError, notifySuccess } from "@/lib/notifications";

export type TestRunStatus = "idle" | "run" | "pass" | "fail" | "skip";

export interface TestEntry {
  id: string;
  file: string;
  line: number; // 0-based
  name: string;
  language: "go" | "typescript" | "javascript";
  status: TestRunStatus;
}

export const testExplorerState = reactive({
  entries: [] as TestEntry[],
  loading: false,
  running: false,
  error: "",
  lastJSONOutput: "",
});

const goTestRe = /^\s*func\s+(Test[A-Za-z0-9_]+)/;
const jsTestRe = /^\s*(?:it|test)(?:\.\w+)?\s*(?:\([^)]*\)\s*)?\(\s*['"`]([^'"`]+)['"`]/;

export async function discoverTests(): Promise<void> {
  // prompt-12 12-H: prefer active workspace root
  let root = appState.currentProject;
  try {
    const { workspaceModulesState } = await import("@/stores/workspaceModules");
    if (workspaceModulesState.activeRoot) root = workspaceModulesState.activeRoot;
  } catch {
    /* ignore */
  }
  if (!root) {
    testExplorerState.entries = [];
    return;
  }
  testExplorerState.loading = true;
  testExplorerState.error = "";
  try {
    const files = await fileService.listAllFiles(root);
    const candidates = files
      .filter(
        (f) =>
          f.endsWith("_test.go") ||
          f.endsWith(".test.ts") ||
          f.endsWith(".test.tsx") ||
          f.endsWith(".spec.ts") ||
          f.endsWith(".test.js") ||
          f.endsWith(".spec.js"),
      )
      .slice(0, 120);
    const entries: TestEntry[] = [];
    for (const rel of candidates) {
      const path =
        rel.includes(":") || rel.startsWith("/") || rel.startsWith(root)
          ? rel
          : root.replace(/[\\/]$/, "") + "/" + rel.replace(/^[\\/]/, "");
      try {
        const content = await fileService.readFile(path);
        const lines = content.split(/\r?\n/);
        const isGo = path.endsWith(".go");
        for (let i = 0; i < lines.length; i++) {
          if (isGo) {
            const m = lines[i].match(goTestRe);
            if (m) {
              entries.push({
                id: `${path}:${i}:${m[1]}`,
                file: path,
                line: i,
                name: m[1],
                language: "go",
                status: "idle",
              });
            }
          } else {
            const m = lines[i].match(jsTestRe);
            if (m) {
              entries.push({
                id: `${path}:${i}:${m[1]}`,
                file: path,
                line: i,
                name: m[1],
                language: path.endsWith(".js") ? "javascript" : "typescript",
                status: "idle",
              });
            }
          }
        }
      } catch {
        /* skip */
      }
    }
    testExplorerState.entries = entries.slice(0, 500);
  } catch (e) {
    testExplorerState.error = e instanceof Error ? e.message : String(e);
  } finally {
    testExplorerState.loading = false;
  }
}

/** prompt-12 12-J: Run this test */
export async function runDiscoveredTest(entry: TestEntry): Promise<void> {
  try {
    entry.status = "run";
    const content = await fileService.readFile(entry.file);
    const result = await runTestAtCursor(entry.language, entry.file, entry.line, content);
    entry.status = result?.success ? "pass" : "fail";
  } catch (e) {
    entry.status = "fail";
    testExplorerState.error = e instanceof Error ? e.message : String(e);
  }
}

/** prompt-12 12-J: Debug this test */
export async function debugDiscoveredTest(entry: TestEntry): Promise<void> {
  try {
    const content = await fileService.readFile(entry.file);
    const { debugTestAtCursor } = await import("@/stores/debug");
    await debugTestAtCursor(entry.language, entry.file, entry.line, content);
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

/** prompt-12 12-J: Coverage for package containing this test */
export async function coverageDiscoveredTest(entry: TestEntry): Promise<void> {
  try {
    const dir = entry.file.replace(/[\\/][^\\/]+$/, "");
    if (entry.language === "go") {
      const { coverageService } = await import("@/api/services");
      const { coverageState, normalizeCoveragePath } = await import("@/stores/coverage");
      coverageState.loading = true;
      const result = await coverageService.runPackageCoverage(dir);
      coverageState.hits = (result.hits || []).map((h) => ({
        file: normalizeCoveragePath(h.file),
        line: h.line,
        covered: h.covered,
      }));
      coverageState.loading = false;
      notifySuccess(`Coverage for ${dir}`);
    } else {
      const { runVitestCoverage } = await import("@/stores/coverage");
      await runVitestCoverage();
    }
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

export async function runGoTestsJSON(packageDir?: string, runRegex?: string): Promise<void> {
  let dir = packageDir || appState.currentProject || "";
  try {
    const { workspaceModulesState } = await import("@/stores/workspaceModules");
    if (!packageDir && workspaceModulesState.activeRoot) dir = workspaceModulesState.activeRoot;
  } catch {
    /* ignore */
  }
  if (!dir) {
    notifyError("Open a Go project first");
    return;
  }
  testExplorerState.running = true;
  for (const e of testExplorerState.entries) {
    if (e.language === "go") e.status = "run";
  }
  try {
    const result = await toolchainService.runGoTestsJSON(dir, runRegex || "");
    testExplorerState.lastJSONOutput = result.output || "";
    const status = result.statusByTest || {};
    for (const e of testExplorerState.entries) {
      if (e.language !== "go") continue;
      const st = status[e.name] as TestRunStatus | undefined;
      if (st === "pass" || st === "fail" || st === "skip" || st === "run") {
        e.status = st;
      } else if (result.success) {
        e.status = e.status === "run" ? "pass" : e.status;
      } else {
        e.status = e.status === "run" ? "fail" : e.status;
      }
    }
    pushOutput("go test -json", result.success ? "info" : "error", result.output || "");
    if (result.success) notifySuccess("go test -json completed");
    else notifyError("Some tests failed (see Output / tree status)");
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    testExplorerState.running = false;
  }
}

export async function jumpToTest(entry: TestEntry): Promise<void> {
  try {
    await openFileFromPath(entry.file);
    appState.cursorLine = entry.line + 1;
    appState.cursorColumn = 1;
    appState.editorJumpSeq = (appState.editorJumpSeq || 0) + 1;
  } catch {
    /* notified */
  }
}
