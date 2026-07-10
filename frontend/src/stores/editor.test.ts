import { describe, it, expect, beforeEach, vi } from "vitest";

const { readFileMock, writeFileMock, notifyErrorMock, notifySuccessMock, createSnapshotMock } =
  vi.hoisted(() => ({
    readFileMock: vi.fn().mockResolvedValue("file content"),
    writeFileMock: vi.fn().mockResolvedValue(undefined),
    notifyErrorMock: vi.fn(),
    notifySuccessMock: vi.fn(),
    createSnapshotMock: vi.fn().mockResolvedValue({ id: "snap-1" }),
  }));

vi.mock("@/api/services", () => ({
  fileService: {
    readFile: readFileMock,
    writeFile: writeFileMock,
  },
  // prompt-8: save/close may notify LSP (best-effort).
  lspService: {
    didSaveDocument: vi.fn().mockResolvedValue(undefined),
    closeDocument: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("@/stores/lsp", () => ({
  closeLSPDocument: vi.fn().mockResolvedValue(undefined),
  // prompt-10 10-D: saveFilePath dynamic-imports this after write
  refreshDiagnosticsToProblems: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/lib/notifications", () => ({
  notifyError: notifyErrorMock,
  notifyWarning: vi.fn(),
  notifySuccess: notifySuccessMock,
  notifyInfo: vi.fn(),
}));

vi.mock("@/lib/i18n", () => ({
  translate: (key: string, params?: Record<string, string>) =>
    params?.name ? `${key}:${params.name}` : key,
}));

vi.mock("@/stores/app", () => ({
  appState: {
    currentProject: "/proj",
    formatOnSave: false,
  },
}));

vi.mock("@/stores/toolchain", () => ({
  runToolchainCommand: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("@/stores/snapshot", () => ({
  createSnapshot: createSnapshotMock,
}));

import {
  editorState,
  applyDiffState,
  openFile,
  closeFile,
  updateContent,
  markSaved,
  saveFile,
  openFileFromPath,
  requestApplyToEditor,
  confirmApplyDiff,
  cancelApplyDiff,
} from "./editor";

describe("editor store", () => {
  beforeEach(() => {
    editorState.openFiles = [];
    editorState.activeFilePath = null;
    cancelApplyDiff();
    readFileMock.mockReset();
    readFileMock.mockResolvedValue("file content");
    writeFileMock.mockReset();
    writeFileMock.mockResolvedValue(undefined);
    notifyErrorMock.mockReset();
    notifySuccessMock.mockReset();
    createSnapshotMock.mockReset();
    createSnapshotMock.mockResolvedValue({ id: "snap-1" });
  });

  it("openFile adds a file and sets it active", () => {
    openFile("/src/app.ts", "const x = 1;");
    expect(editorState.openFiles).toHaveLength(1);
    expect(editorState.openFiles[0].name).toBe("app.ts");
    expect(editorState.activeFilePath).toBe("/src/app.ts");
    expect(editorState.openFiles[0].isDirty).toBe(false);
  });

  it("openFile does not duplicate an already-open file", () => {
    openFile("/src/app.ts", "const x = 1;");
    openFile("/src/app.ts", "const x = 1;");
    expect(editorState.openFiles).toHaveLength(1);
  });

  it("openFile reactivates an existing tab without changing content", () => {
    openFile("/src/app.ts", "const x = 1;");
    updateContent("/src/app.ts", "const x = 2;");
    openFile("/src/app.ts", "ignored — already open");
    expect(editorState.openFiles[0].content).toBe("const x = 2;");
  });

  it("updateContent marks file dirty when content changes", () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "changed");
    expect(editorState.openFiles[0].isDirty).toBe(true);
    expect(editorState.openFiles[0].content).toBe("changed");
  });

  it("updateContent does not mark dirty if content equals original", () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "original");
    expect(editorState.openFiles[0].isDirty).toBe(false);
  });

  it("markSaved clears dirty flag and updates original content", () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "new content");
    markSaved("/src/app.ts");
    expect(editorState.openFiles[0].isDirty).toBe(false);
    expect(editorState.openFiles[0].originalContent).toBe("new content");
  });

  it("closeFile removes the file from the list", () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    closeFile("/src/a.ts");
    expect(editorState.openFiles).toHaveLength(1);
    expect(editorState.openFiles[0].path).toBe("/src/b.ts");
  });

  it("closeFile of the active tab selects a neighbor", () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    closeFile("/src/b.ts");
    expect(editorState.activeFilePath).toBe("/src/a.ts");
  });

  it("closeFile of the only tab clears active path", () => {
    openFile("/src/a.ts", "a");
    closeFile("/src/a.ts");
    expect(editorState.openFiles).toHaveLength(0);
    expect(editorState.activeFilePath).toBeNull();
  });

  it("openFile sets language from extension", () => {
    openFile("/src/app.ts", "");
    expect(editorState.openFiles[0].language).toBe("typescript");
    openFile("/src/main.go", "");
    expect(editorState.openFiles[1].language).toBe("go");
  });

  it("saveFile writes active file to disk and clears dirty", async () => {
    openFile("/src/app.ts", "original");
    updateContent("/src/app.ts", "modified");
    expect(editorState.openFiles[0].isDirty).toBe(true);
    await saveFile();
    expect(editorState.openFiles[0].isDirty).toBe(false);
    expect(editorState.openFiles[0].originalContent).toBe("modified");
  });

  it("saveFile does nothing when no active file", async () => {
    await expect(saveFile()).resolves.toBeUndefined();
  });

  // prompt-5 Task A / BUG-H2
  it("updateContent returns false when file is not open", () => {
    expect(updateContent("/missing.ts", "x")).toBe(false);
  });

  it("updateContent returns true when file is open", () => {
    openFile("/src/app.ts", "original");
    expect(updateContent("/src/app.ts", "changed")).toBe(true);
    expect(editorState.openFiles[0].content).toBe("changed");
  });

  it("openFileFromPath opens file on success", async () => {
    readFileMock.mockResolvedValueOnce("from disk");
    await openFileFromPath("/src/from-disk.ts");
    expect(editorState.openFiles).toHaveLength(1);
    expect(editorState.openFiles[0].content).toBe("from disk");
    expect(editorState.activeFilePath).toBe("/src/from-disk.ts");
  });

  it("openFileFromPath rethrows and notifies on failure", async () => {
    readFileMock.mockRejectedValueOnce(new Error("ENOENT"));
    await expect(openFileFromPath("/missing.ts")).rejects.toThrow("ENOENT");
    expect(notifyErrorMock).toHaveBeenCalled();
    expect(editorState.openFiles).toHaveLength(0);
  });

  it("requestApplyToEditor fails without path", async () => {
    expect(await requestApplyToEditor("", "code")).toBe(false);
    expect(applyDiffState.visible).toBe(false);
    expect(notifyErrorMock).toHaveBeenCalled();
  });

  it("requestApplyToEditor opens file and shows diff on success", async () => {
    readFileMock.mockResolvedValueOnce("original body");
    const ok = await requestApplyToEditor("/src/app.ts", "new body");
    expect(ok).toBe(true);
    expect(applyDiffState.visible).toBe(true);
    expect(applyDiffState.path).toBe("/src/app.ts");
    expect(applyDiffState.original).toBe("original body");
    expect(applyDiffState.modified).toBe("new body");
    // Content not written until confirm
    expect(editorState.openFiles[0].content).toBe("original body");
  });

  it("requestApplyToEditor fails when open fails", async () => {
    readFileMock.mockRejectedValueOnce(new Error("permission denied"));
    const ok = await requestApplyToEditor("/locked.ts", "x");
    expect(ok).toBe(false);
    expect(applyDiffState.visible).toBe(false);
  });

  it("confirmApplyDiff writes content and reports success", async () => {
    openFile("/src/app.ts", "old");
    applyDiffState.visible = true;
    applyDiffState.path = "/src/app.ts";
    applyDiffState.original = "old";
    applyDiffState.modified = "new";
    applyDiffState.language = "typescript";
    const ok = await confirmApplyDiff();
    expect(ok).toBe(true);
    expect(editorState.openFiles[0].content).toBe("new");
    expect(editorState.openFiles[0].isDirty).toBe(true);
    expect(applyDiffState.visible).toBe(false);
    expect(notifySuccessMock).toHaveBeenCalled();
    expect(createSnapshotMock).toHaveBeenCalledWith("pre-apply");
  });
});
