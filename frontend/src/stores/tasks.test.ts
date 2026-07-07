import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn() },
}));

vi.mock("@/api/services", () => ({
  taskService: {
    loadTasks: vi.fn(),
  },
}));

vi.mock("@/stores/app", () => ({
  appState: { terminalVisible: false, currentProject: null },
}));

vi.mock("@/stores/terminal", () => ({
  createSession: vi.fn().mockResolvedValue("session-1"),
  writeToSession: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/stores/output", () => ({
  pushOutput: vi.fn(),
}));

vi.mock("@/lib/notifications", () => ({
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
  notifyWarning: vi.fn(),
}));

import { taskState, loadTasks, composeCommandLine, resolveCwd, runTask, hasTasks } from "./tasks";
import { taskService } from "@/api/services";
import { createSession, writeToSession } from "@/stores/terminal";
import { pushOutput } from "@/stores/output";

describe("tasks store", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    taskState.tasks = [];
    taskState.loading = false;
    taskState.errorMessage = null;
  });

  describe("loadTasks", () => {
    it("clears tasks when root is empty", async () => {
      taskState.tasks = [{ label: "x", command: "y" }];
      await loadTasks("");
      expect(taskState.tasks).toEqual([]);
      expect(taskState.errorMessage).toBeNull();
      expect(taskService.loadTasks).not.toHaveBeenCalled();
    });

    it("loads tasks from backend", async () => {
      (taskService.loadTasks as any).mockResolvedValue([
        { label: "build", command: "go", args: ["build", "./..."] },
      ]);
      await loadTasks("/proj");
      expect(taskState.tasks.length).toBe(1);
      expect(taskState.tasks[0].label).toBe("build");
      expect(taskState.loading).toBe(false);
      expect(taskState.errorMessage).toBeNull();
    });

    it("surfaces backend errors", async () => {
      (taskService.loadTasks as any).mockRejectedValue(new Error("parse failed"));
      await loadTasks("/proj");
      expect(taskState.tasks).toEqual([]);
      expect(taskState.errorMessage).toBe("parse failed");
      expect(taskState.loading).toBe(false);
    });
  });

  describe("hasTasks computed", () => {
    it("is false when no tasks", () => {
      expect(hasTasks.value).toBe(false);
    });
    it("is true when tasks exist", () => {
      taskState.tasks = [{ label: "x", command: "y" }];
      expect(hasTasks.value).toBe(true);
    });
  });

  describe("composeCommandLine", () => {
    it("returns command alone when no args", () => {
      expect(composeCommandLine({ label: "x", command: "ls" })).toBe("ls");
    });
    it("quotes args", () => {
      expect(composeCommandLine({ label: "x", command: "go", args: ["build", "./..."] }))
        .toBe("go 'build' './...'");
    });
    it("escapes embedded single quotes", () => {
      expect(composeCommandLine({ label: "x", command: "echo", args: ["it's"] }))
        .toBe("echo 'it'\\''s'");
    });
  });

  describe("resolveCwd", () => {
    it("returns project root when no cwd", () => {
      expect(resolveCwd({ label: "x", command: "y" }, "/proj")).toBe("/proj");
    });
    it("joins relative cwd to root", () => {
      expect(resolveCwd({ label: "x", command: "y", cwd: "src" }, "/proj")).toBe("/proj/src");
    });
    it("strips trailing slash from root before joining", () => {
      expect(resolveCwd({ label: "x", command: "y", cwd: "src" }, "/proj/")).toBe("/proj/src");
    });
    it("uses absolute cwd as-is", () => {
      expect(resolveCwd({ label: "x", command: "y", cwd: "/abs/path" }, "/proj")).toBe("/abs/path");
    });
    it("uses absolute windows cwd as-is", () => {
      expect(resolveCwd({ label: "x", command: "y", cwd: "C:\\abs" }, "/proj")).toBe("C:\\abs");
    });
  });

  describe("runTask", () => {
    it("creates a session and writes the command", async () => {
      await runTask({ label: "build", command: "go", args: ["build"] }, "/proj");
      expect(createSession).toHaveBeenCalledWith("/proj");
      // writeToSession is called via setTimeout; flush timers.
      await new Promise((r) => setTimeout(r, 120));
      expect(writeToSession).toHaveBeenCalledWith("session-1", "go 'build'\n");
      expect(pushOutput).toHaveBeenCalledWith("task", "info", expect.stringContaining("build"));
    });

    it("uses task cwd when provided", async () => {
      await runTask({ label: "x", command: "ls", cwd: "sub" }, "/proj");
      expect(createSession).toHaveBeenCalledWith("/proj/sub");
    });

    it("does not throw when session creation fails", async () => {
      (createSession as any).mockResolvedValueOnce("");
      await runTask({ label: "x", command: "ls" }, "/proj");
      // Should not throw; error is surfaced via notifyError.
      expect(writeToSession).not.toHaveBeenCalled();
    });
  });
});
