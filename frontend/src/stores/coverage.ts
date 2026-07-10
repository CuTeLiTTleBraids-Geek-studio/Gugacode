/**
 * prompt-10/11: go coverprofile + optional vitest coverage → gutter.
 * 11-B: path matching never uses basename-only collision.
 */
import { reactive } from "vue";
import { coverageService } from "@/api/services";
import { appState } from "@/stores/app";
import { pushOutput } from "@/stores/output";
import { notifyError, notifySuccess } from "@/lib/notifications";

export interface CoverageLineHit {
  file: string;
  line: number;
  covered: boolean;
}

export const coverageState = reactive({
  hits: [] as CoverageLineHit[],
  loading: false,
  lastOutput: "",
  profile: "",
  /** prompt-11 11-H: enable vitest coverage gutter when true */
  vitestEnabled: true,
});

/** Normalize like backend NormalizeCoveragePath. */
export function normalizeCoveragePath(p: string): string {
  if (!p) return "";
  let s = p.replace(/\\/g, "/");
  s = s.replace(/^\.\//, "");
  // collapse // (except drive)
  s = s.replace(/([^:])\/+/g, "$1/");
  return s;
}

/**
 * prompt-11 11-B: match hit path to editor path without basename-only bleed.
 */
export function coveragePathsMatch(hitPath: string, editorPath: string): boolean {
  const h = normalizeCoveragePath(hitPath);
  const e = normalizeCoveragePath(editorPath);
  if (!h || !e) return false;
  if (h.toLowerCase() === e.toLowerCase()) return true;
  const hParts = h.split("/").filter(Boolean);
  const eParts = e.split("/").filter(Boolean);
  if (hParts.length === 1 || eParts.length === 1) {
    return hParts.length === 1 && eParts.length === 1 && h.toLowerCase() === e.toLowerCase();
  }
  const hl = h.toLowerCase();
  const el = e.toLowerCase();
  if (el.endsWith("/" + hl) || hl.endsWith("/" + el)) return true;
  return (
    hParts[hParts.length - 1].toLowerCase() === eParts[eParts.length - 1].toLowerCase() &&
    hParts[hParts.length - 2].toLowerCase() === eParts[eParts.length - 2].toLowerCase()
  );
}

/** Hits for a given absolute path. */
export function coverageHitsForFile(filePath: string): CoverageLineHit[] {
  if (!filePath) return [];
  return coverageState.hits.filter((h) => coveragePathsMatch(h.file, filePath));
}

export async function runPackageCoverage(): Promise<void> {
  const dir = appState.currentProject || "";
  if (!dir) {
    notifyError("Open a project first");
    return;
  }
  coverageState.loading = true;
  try {
    const result = await coverageService.runPackageCoverage(dir);
    coverageState.lastOutput = result.output || "";
    coverageState.profile = result.profile || "";
    coverageState.hits = (result.hits || []).map((h) => ({
      file: normalizeCoveragePath(h.file),
      line: h.line,
      covered: h.covered,
    }));
    pushOutput("Coverage", result.success ? "info" : "warn", result.output || "coverage done");
    notifySuccess(`Coverage: ${coverageState.hits.length} line hits`);
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    coverageState.loading = false;
  }
}

/**
 * prompt-11 11-H: parse a simple lcov-ish or istanbul summary into hits.
 * Accepts lines like: SF:path then DA:line,hits
 */
export function parseLcovToHits(lcovText: string): CoverageLineHit[] {
  const hits: CoverageLineHit[] = [];
  let file = "";
  for (const line of lcovText.split(/\r?\n/)) {
    if (line.startsWith("SF:")) {
      file = normalizeCoveragePath(line.slice(3).trim());
    } else if (line.startsWith("DA:") && file) {
      const body = line.slice(3);
      const [ln, cnt] = body.split(",");
      const lineNo = parseInt(ln, 10);
      const count = parseInt(cnt, 10);
      if (lineNo > 0) {
        hits.push({ file, line: lineNo, covered: count > 0 });
      }
    } else if (line.startsWith("end_of_record")) {
      file = "";
    }
  }
  return hits;
}

export async function runVitestCoverage(): Promise<void> {
  if (!coverageState.vitestEnabled) {
    notifyError("Vitest coverage gutter is disabled");
    return;
  }
  const dir = appState.currentProject || "";
  if (!dir) {
    notifyError("Open a project first");
    return;
  }
  coverageState.loading = true;
  try {
    const { runToolchainCommand } = await import("@/stores/toolchain");
    // Prefer project script; toolchain may expose vitest-coverage
    const result = await runToolchainCommand("vitest-coverage" as never, dir).catch(async () => {
      // Fallback: try reading coverage/lcov.info if present
      const { fileService } = await import("@/api/services");
      const root = dir.replace(/[\\/]$/, "");
      try {
        const text = await fileService.readFile(root + "/coverage/lcov.info");
        return { success: true, output: text, errors: [] };
      } catch {
        return { success: false, output: "no lcov.info — run vitest --coverage first", errors: [] };
      }
    });
    const text = (result as { output?: string })?.output || "";
    if (text.includes("SF:") || text.includes("DA:")) {
      coverageState.hits = parseLcovToHits(text);
      notifySuccess(`Vitest coverage: ${coverageState.hits.length} line hits`);
    } else {
      pushOutput("Coverage", "warn", text || "vitest coverage unavailable");
      notifyError("No lcov data — run vitest --coverage or add coverage/lcov.info");
    }
  } catch (e) {
    notifyError(e instanceof Error ? e.message : String(e));
  } finally {
    coverageState.loading = false;
  }
}

export function clearCoverage(): void {
  coverageState.hits = [];
  coverageState.profile = "";
}
