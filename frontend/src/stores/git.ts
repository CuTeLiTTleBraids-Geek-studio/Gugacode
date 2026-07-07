import { reactive } from "vue";
import { gitService } from "@/api/services";
import type { GitFileChange, BranchRef } from "@/types";
import { pushOutput } from "@/stores/output";
import { errorMessage } from "@/lib/errors";

export interface GitState {
  changes: GitFileChange[];
  branchName: string;
  ahead: number;
  behind: number;
  loading: boolean;
  error: string | null;
}

export const gitState = reactive<GitState>({
  changes: [],
  branchName: "",
  ahead: 0,
  behind: 0,
  loading: false,
  error: null,
});

export const branchState = reactive({
  branches: [] as BranchRef[],
  loadingBranches: false,
});

export async function refreshGit(repoPath: string): Promise<void> {
  gitState.loading = true;
  gitState.error = null;
  try {
    const [changes, info] = await Promise.all([
      gitService.getStatus(repoPath),
      gitService.getBranchInfo(repoPath),
    ]);
    gitState.changes = changes;
    gitState.branchName = info.name;
    gitState.ahead = info.ahead;
    gitState.behind = info.behind;
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
  } finally {
    gitState.loading = false;
  }
}

export async function stageFile(repoPath: string, filePath: string): Promise<void> {
  try {
    await gitService.stage(repoPath, filePath);
    await refreshGit(repoPath);
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
  }
}

export async function unstageFile(repoPath: string, filePath: string): Promise<void> {
  try {
    await gitService.unstage(repoPath, filePath);
    await refreshGit(repoPath);
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
  }
}

export async function commitChanges(repoPath: string, message: string): Promise<void> {
  try {
    await gitService.commit(repoPath, message);
    pushOutput("git", "success", `Committed: ${message}`);
    await refreshGit(repoPath);
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
    pushOutput("git", "error", `Commit failed: ${errorMessage(e)}`);
  }
}

export function clearGitState(): void {
  gitState.changes = [];
  gitState.branchName = "";
  gitState.ahead = 0;
  gitState.behind = 0;
  gitState.loading = false;
  gitState.error = null;
}

export async function loadBranches(repoPath: string) {
  if (!repoPath) return;
  branchState.loadingBranches = true;
  try {
    branchState.branches = await gitService.listBranches(repoPath);
  } catch (e) {
    console.error("Failed to load branches:", e);
    branchState.branches = [];
  } finally {
    branchState.loadingBranches = false;
  }
}

export async function createBranch(repoPath: string, name: string) {
  await gitService.createBranch(repoPath, name);
  await loadBranches(repoPath);
  await refreshGit(repoPath);
}

export async function checkoutBranch(repoPath: string, name: string) {
  await gitService.checkoutBranch(repoPath, name);
  await loadBranches(repoPath);
  await refreshGit(repoPath);
}

export async function deleteBranch(repoPath: string, name: string) {
  await gitService.deleteBranch(repoPath, name);
  await loadBranches(repoPath);
}

export async function pushChanges(repoPath: string): Promise<void> {
  try {
    await gitService.push(repoPath);
    pushOutput("git", "success", "Pushed to origin");
    await refreshGit(repoPath);
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
    pushOutput("git", "error", `Push failed: ${errorMessage(e)}`);
    throw e;
  }
}

export async function pullChanges(repoPath: string): Promise<void> {
  try {
    await gitService.pull(repoPath);
    pushOutput("git", "success", "Pulled from origin");
    await refreshGit(repoPath);
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
    pushOutput("git", "error", `Pull failed: ${errorMessage(e)}`);
    throw e;
  }
}
