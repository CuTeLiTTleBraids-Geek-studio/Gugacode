/**
 * prompt-10 10-L: lightweight go.work / package.json workspace awareness.
 */
import { reactive } from "vue";
import { fileService } from "@/api/services";
import { appState } from "@/stores/app";
import { runtimeVersions } from "@/stores/toolchain";

export const workspaceModulesState = reactive({
  goWorkModules: [] as string[],
  packageWorkspaces: [] as string[],
  loading: false,
});

export async function refreshWorkspaceModules(): Promise<void> {
  const root = appState.currentProject;
  if (!root) {
    workspaceModulesState.goWorkModules = [];
    workspaceModulesState.packageWorkspaces = [];
    return;
  }
  workspaceModulesState.loading = true;
  try {
    // go.work
    if (runtimeVersions.hasGoWork) {
      try {
        const text = await fileService.readFile(root.replace(/[\\/]$/, "") + "/go.work");
        const mods: string[] = [];
        for (const line of text.split(/\r?\n/)) {
          const t = line.trim();
          if (!t || t.startsWith("//") || t.startsWith("go ") || t === "use (" || t === ")") continue;
          if (t.startsWith("use ")) {
            mods.push(t.slice(4).trim().replace(/^"|"$/g, ""));
          } else if (!t.includes(" ")) {
            mods.push(t.replace(/^"|"$/g, ""));
          }
        }
        workspaceModulesState.goWorkModules = mods;
      } catch {
        workspaceModulesState.goWorkModules = [];
      }
    } else {
      workspaceModulesState.goWorkModules = [];
    }
    // package.json workspaces
    try {
      const pkg = await fileService.readFile(root.replace(/[\\/]$/, "") + "/package.json");
      const j = JSON.parse(pkg) as { workspaces?: string[] | { packages?: string[] } };
      if (Array.isArray(j.workspaces)) {
        workspaceModulesState.packageWorkspaces = j.workspaces;
      } else if (j.workspaces && Array.isArray(j.workspaces.packages)) {
        workspaceModulesState.packageWorkspaces = j.workspaces.packages;
      } else {
        workspaceModulesState.packageWorkspaces = [];
      }
    } catch {
      workspaceModulesState.packageWorkspaces = [];
    }
  } finally {
    workspaceModulesState.loading = false;
  }
}
