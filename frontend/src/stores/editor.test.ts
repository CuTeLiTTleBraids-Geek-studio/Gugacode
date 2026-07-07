import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/api/services", () => ({
  fileService: {
    readFile: vi.fn().mockResolvedValue("file content"),
    writeFile: vi.fn().mockResolvedValue(undefined),
  },
}));

import { editorState, openFile, closeFile, updateContent, markSaved, saveFile } from "./editor";

describe("editor store", () => {
  beforeEach(() => {
    editorState.openFiles = [];
    editorState.activeFilePath = null;
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
});
