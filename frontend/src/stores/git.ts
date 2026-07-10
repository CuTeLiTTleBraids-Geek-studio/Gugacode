import { reactive } from "vue";
import { gitService, fileService } from "@/api/services";
import type { GitFileChange, BranchRef, MergeConflict } from "@/types";
import { pushOutput } from "@/stores/output";
import { errorMessage } from "@/lib/errors";

export interface GitState {
  changes: GitFileChange[];
  branchName: string;
  ahead: number;
  behind: number;
  loading: boolean;
  error: string | null;
  /** prompt-7 Task L: status list was capped for UI. */
  truncated: boolean;
}

/** Cap Git status list in UI to avoid jank on huge dirty trees (prompt-7 Task L). */
export const MAX_GIT_UI_CHANGES = 1000;

export const gitState = reactive<GitState>({
  changes: [],
  branchName: "",
  ahead: 0,
  behind: 0,
  loading: false,
  error: null,
  truncated: false,
});

export const branchState = reactive({
  branches: [] as BranchRef[],
  loadingBranches: false,
});

// G-FEAT-04: merge/rebase conflict state.
export interface ConflictState {
  conflicts: MergeConflict[];
  loading: boolean;
  error: string | null;
}

export const conflictState = reactive<ConflictState>({
  conflicts: [],
  loading: false,
  error: null,
});

// G-FEAT-04: rebase state.
export interface RebaseState {
  inProgress: boolean;
  loading: boolean;
  error: string | null;
  lastOutput: string;
}

export const rebaseState = reactive<RebaseState>({
  inProgress: false,
  loading: false,
  error: null,
  lastOutput: "",
});

export async function refreshGit(repoPath: string): Promise<void> {
  gitState.loading = true;
  gitState.error = null;
  try {
    const [changes, info] = await Promise.all([
      gitService.getStatus(repoPath),
      gitService.getBranchInfo(repoPath),
    ]);
    if (changes.length > MAX_GIT_UI_CHANGES) {
      gitState.changes = changes.slice(0, MAX_GIT_UI_CHANGES);
      gitState.truncated = true;
    } else {
      gitState.changes = changes;
      gitState.truncated = false;
    }
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

// ---------------------------------------------------------------------------
// G-FEAT-04: merge conflict resolution
// ---------------------------------------------------------------------------

export async function loadConflicts(): Promise<void> {
  conflictState.loading = true;
  conflictState.error = null;
  try {
    conflictState.conflicts = await gitService.listMergeConflicts();
  } catch (e: unknown) {
    conflictState.error = errorMessage(e);
  } finally {
    conflictState.loading = false;
  }
}

export function clearConflictState(): void {
  conflictState.conflicts = [];
  conflictState.loading = false;
  conflictState.error = null;
}

/**
 * Resolve a conflict by accepting "ours": writes the ours-side blob content
 * to the working tree file, then stages it.
 */
export async function resolveConflictAsOurs(repoPath: string, conflict: MergeConflict): Promise<void> {
  try {
    const fullPath = repoPath + "/" + conflict.file;
    await fileService.writeFile(fullPath, conflict.ours);
    await gitService.resolveConflict(conflict.file);
    await loadConflicts();
    await refreshGit(repoPath);
  } catch (e: unknown) {
    conflictState.error = errorMessage(e);
    throw e;
  }
}

/**
 * Resolve a conflict by accepting "theirs": writes the theirs-side blob
 * content to the working tree file, then stages it.
 */
export async function resolveConflictAsTheirs(repoPath: string, conflict: MergeConflict): Promise<void> {
  try {
    const fullPath = repoPath + "/" + conflict.file;
    await fileService.writeFile(fullPath, conflict.theirs);
    await gitService.resolveConflict(conflict.file);
    await loadConflicts();
    await refreshGit(repoPath);
  } catch (e: unknown) {
    conflictState.error = errorMessage(e);
    throw e;
  }
}

/**
 * Mark a manually-resolved conflict as staged. The user is expected to have
 * edited and saved the file in the editor first.
 */
export async function markConflictResolved(repoPath: string, file: string): Promise<void> {
  try {
    await gitService.resolveConflict(file);
    await loadConflicts();
    await refreshGit(repoPath);
  } catch (e: unknown) {
    conflictState.error = errorMessage(e);
    throw e;
  }
}

// ---------------------------------------------------------------------------
// G-FEAT-04: rebase support
// ---------------------------------------------------------------------------

export async function checkRebaseStatus(): Promise<void> {
  try {
    rebaseState.inProgress = await gitService.isRebaseInProgress();
  } catch (e: unknown) {
    rebaseState.error = errorMessage(e);
  }
}

export async function startRebase(branch: string): Promise<string | null> {
  rebaseState.loading = true;
  rebaseState.error = null;
  try {
    const output = await gitService.rebase(branch);
    rebaseState.lastOutput = output;
    await checkRebaseStatus();
    if (rebaseState.inProgress) {
      await loadConflicts();
    }
    return output;
  } catch (e: unknown) {
    rebaseState.error = errorMessage(e);
    rebaseState.lastOutput = errorMessage(e);
    // A rebase conflict also produces a non-zero exit, so check if a rebase
    // is now in progress (conflicts waiting to be resolved).
    await checkRebaseStatus();
    if (rebaseState.inProgress) {
      await loadConflicts();
    }
    return null;
  } finally {
    rebaseState.loading = false;
  }
}

export async function abortRebase(): Promise<void> {
  rebaseState.loading = true;
  rebaseState.error = null;
  try {
    await gitService.abortRebase();
    rebaseState.inProgress = false;
    clearConflictState();
    pushOutput("git", "success", "Rebase aborted");
  } catch (e: unknown) {
    rebaseState.error = errorMessage(e);
    throw e;
  } finally {
    rebaseState.loading = false;
  }
}

export async function continueRebase(): Promise<void> {
  rebaseState.loading = true;
  rebaseState.error = null;
  try {
    await gitService.continueRebase();
    await checkRebaseStatus();
    if (rebaseState.inProgress) {
      await loadConflicts();
    } else {
      clearConflictState();
    }
    pushOutput("git", "success", "Rebase continued");
  } catch (e: unknown) {
    rebaseState.error = errorMessage(e);
    await checkRebaseStatus();
    if (rebaseState.inProgress) {
      await loadConflicts();
    }
    throw e;
  } finally {
    rebaseState.loading = false;
  }
}

// ---------------------------------------------------------------------------
// G-FEAT-04: .gitignore template generation
// ---------------------------------------------------------------------------

export async function generateGitignore(projectType: string): Promise<void> {
  try {
    await gitService.createGitignore(projectType);
    pushOutput("git", "success", `.gitignore created (${projectType})`);
  } catch (e: unknown) {
    gitState.error = errorMessage(e);
    throw e;
  }
}
