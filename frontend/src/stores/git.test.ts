import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  gitService: {
    getStatus: vi.fn().mockResolvedValue([
      { path: "a.txt", status: "Modified" },
      { path: "b.txt", status: "Untracked" },
    ]),
    getBranchInfo: vi.fn().mockResolvedValue({
      name: "main",
      ahead: 2,
      behind: 0,
    }),
    stage: vi.fn().mockResolvedValue(undefined),
    unstage: vi.fn().mockResolvedValue(undefined),
    commit: vi.fn().mockResolvedValue(undefined),
  },
}));

import {
  gitState,
  refreshGit,
  stageFile,
  unstageFile,
  commitChanges,
} from "./git";

describe("git store", () => {
  beforeEach(() => {
    gitState.changes = [];
    gitState.branchName = "";
    gitState.ahead = 0;
    gitState.behind = 0;
    gitState.loading = false;
    gitState.error = null;
  });

  it("starts with empty state", () => {
    expect(gitState.changes).toHaveLength(0);
    expect(gitState.branchName).toBe("");
    expect(gitState.loading).toBe(false);
  });

  it("refreshGit loads changes and branch info", async () => {
    await refreshGit("/some/repo");
    expect(gitState.changes).toHaveLength(2);
    expect(gitState.changes[0].path).toBe("a.txt");
    expect(gitState.branchName).toBe("main");
    expect(gitState.ahead).toBe(2);
    expect(gitState.loading).toBe(false);
  });

  it("stageFile calls gitService.stage", async () => {
    await stageFile("/repo", "a.txt");
    const { gitService } = await import("@/api/services");
    expect(gitService.stage).toHaveBeenCalledWith("/repo", "a.txt");
  });

  it("unstageFile calls gitService.unstage", async () => {
    await unstageFile("/repo", "a.txt");
    const { gitService } = await import("@/api/services");
    expect(gitService.unstage).toHaveBeenCalledWith("/repo", "a.txt");
  });

  it("commitChanges calls gitService.commit and refreshes", async () => {
    await commitChanges("/repo", "fix: something");
    const { gitService } = await import("@/api/services");
    expect(gitService.commit).toHaveBeenCalledWith("/repo", "fix: something");
    expect(gitService.getStatus).toHaveBeenCalled();
  });

  it("stores error on failure", async () => {
    const { gitService } = await import("@/api/services");
    (gitService.getStatus as any).mockRejectedValueOnce(new Error("fail"));
    await refreshGit("/repo");
    expect(gitState.error).toBe("fail");
    expect(gitState.loading).toBe(false);
  });
});
