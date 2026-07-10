/**
 * prompt-10 10-M: lightweight test explorer (discover TestXxx / it( in open project).
 * Not a full IDE test UI — lists candidates from known source roots for one-click run.
 */
import { reactive } from "vue";
import { fileService } from "@/api/services";
import { appState } from "@/stores/app";
import { runTestAtCursor } from "@/stores/toolchain";

export interface TestEntry {
  id: string;
  file: string;
  line: number; // 0-based
  name: string;
  language: "go" | "typescript" | "javascript";
}

export const testExplorerState = reactive({
  entries: [] as TestEntry[],
  loading: false,
  error: "",
});

const goTestRe = /^\s*func\s+(Test[A-Za-z0-9_]+)/;
const jsTestRe = /^\s*(?:it|test)(?:\.\w+)?\s*(?:\([^)]*\)\s*)?\(\s*['"`]([^'"`]+)['"`]/;

export async function discoverTests(): Promise<void> {
  const root = appState.currentProject;
  if (!root) {
    testExplorerState.entries = [];
    return;
  }
  testExplorerState.loading = true;
  testExplorerState.error = "";
  try {
    const files = await fileService.listAllFiles(root);
    const candidates = files.filter(
      (f) =>
        f.endsWith("_test.go") ||
        f.endsWith(".test.ts") ||
        f.endsWith(".test.tsx") ||
        f.endsWith(".spec.ts") ||
        f.endsWith(".test.js"),
    ).slice(0, 80);
    const entries: TestEntry[] = [];
    for (const rel of candidates) {
      const path = rel.includes(":") || rel.startsWith("/") || rel.startsWith(root)
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
              });
            }
          }
        }
      } catch {
        /* skip unreadable */
      }
    }
    testExplorerState.entries = entries.slice(0, 500);
  } catch (e) {
    testExplorerState.error = e instanceof Error ? e.message : String(e);
  } finally {
    testExplorerState.loading = false;
  }
}

export async function runDiscoveredTest(entry: TestEntry): Promise<void> {
  try {
    const content = await fileService.readFile(entry.file);
    await runTestAtCursor(entry.language, entry.file, entry.line, content);
  } catch (e) {
    testExplorerState.error = e instanceof Error ? e.message : String(e);
  }
}
