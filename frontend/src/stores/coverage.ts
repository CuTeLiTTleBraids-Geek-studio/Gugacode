/**
 * prompt-10 10-H: go coverprofile → gutter decorations.
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
});

/** Hits for a given absolute or suffix path. */
export function coverageHitsForFile(filePath: string): CoverageLineHit[] {
  if (!filePath) return [];
  const norm = filePath.replace(/\\/g, "/");
  return coverageState.hits.filter((h) => {
    const f = h.file.replace(/\\/g, "/");
    return norm.endsWith(f) || f.endsWith(norm) || norm.includes(f) || f.includes(norm.split("/").pop() || "");
  });
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
      file: h.file,
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

export function clearCoverage(): void {
  coverageState.hits = [];
  coverageState.profile = "";
}
