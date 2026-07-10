import { reactive, computed, watch } from "vue";
import { detectLanguage } from "@/lib/language";
import { fileService } from "@/api/services";
import { notifyError, notifyWarning, notifySuccess } from "@/lib/notifications";
import { errorMessage } from "@/lib/errors";
import { translate } from "@/lib/i18n";
import { appState } from "@/stores/app";

export interface OpenFile {
  path: string;
  name: string;
  content: string;
  originalContent: string;
  language: string;
  isDirty: boolean;
}

interface EditorState {
  openFiles: OpenFile[];
  activeFilePath: string | null;
}

/**
 * prompt-5 Task A / BUG-H2: global Diff-preview state for apply-to-editor.
 * Used by the main window when the AI companion window (or side chat) asks
 * to write code into an editor buffer. Success only happens after the user
 * confirms in the Diff modal.
 */
export interface ApplyDiffState {
  visible: boolean;
  path: string;
  original: string;
  modified: string;
  language: string;
}

export const editorState = reactive<EditorState>({
  openFiles: [],
  activeFilePath: null,
});

export const applyDiffState = reactive<ApplyDiffState>({
  visible: false,
  path: "",
  original: "",
  modified: "",
  language: "",
});

export const activeFile = computed<OpenFile | null>(() =>
  editorState.openFiles.find((f) => f.path === editorState.activeFilePath) ?? null
);

export function openFile(path: string, content: string): void {
  const existing = editorState.openFiles.find((f) => f.path === path);
  if (existing) {
    editorState.activeFilePath = path;
    return;
  }
  const name = path.split(/[/\\]/).pop() ?? path;
  editorState.openFiles.push({
    path,
    name,
    content,
    originalContent: content,
    language: detectLanguage(path),
    isDirty: false,
  });
  editorState.activeFilePath = path;
}

export function closeFile(path: string): void {
  const idx = editorState.openFiles.findIndex((f) => f.path === path);
  if (idx === -1) return;
  const closing = editorState.openFiles[idx];
  // prompt-8 Task 8-A: notify LSP didClose.
  if (closing) {
    const lang = closing.language || "";
    const lspLang =
      lang === "go" || path.endsWith(".go")
        ? "go"
        : lang.includes("typescript") || path.endsWith(".ts") || path.endsWith(".tsx")
          ? "typescript"
          : lang.includes("javascript") || path.endsWith(".js") || path.endsWith(".jsx")
            ? "javascript"
            : "";
    if (lspLang) {
      void import("@/stores/lsp").then(({ closeLSPDocument }) => {
        void closeLSPDocument(lspLang, path);
      });
    }
  }
  editorState.openFiles.splice(idx, 1);
  if (editorState.activeFilePath === path) {
    const next = editorState.openFiles[idx] ?? editorState.openFiles[idx - 1] ?? null;
    editorState.activeFilePath = next?.path ?? null;
  }
}

/**
 * Updates an already-open file's buffer. Returns false when the file is not
 * open (prompt-5 Task A / BUG-H2: callers must not report success on no-op).
 */
export function updateContent(path: string, content: string): boolean {
  const file = editorState.openFiles.find((f) => f.path === path);
  if (!file) return false;
  file.content = content;
  file.isDirty = content !== file.originalContent;
  return true;
}

export function markSaved(path: string): void {
  const file = editorState.openFiles.find((f) => f.path === path);
  if (file) {
    file.originalContent = file.content;
    file.isDirty = false;
  }
}

export function getDirtyFiles(): OpenFile[] {
  return editorState.openFiles.filter((f) => f.isDirty);
}

function lspLangForFile(path: string, language: string): string {
  const lang = language || "";
  if (lang === "go" || path.endsWith(".go")) return "go";
  if (lang.includes("typescript") || path.endsWith(".ts") || path.endsWith(".tsx")) return "typescript";
  if (lang.includes("javascript") || path.endsWith(".js") || path.endsWith(".jsx")) return "javascript";
  return "";
}

/**
 * Saves a single open file by path (prompt-10 10-A Save All helper).
 */
export async function saveFilePath(path: string, opts?: { skipFormat?: boolean }): Promise<boolean> {
  const file = editorState.openFiles.find((f) => f.path === path);
  if (!file) return false;
  const { appState } = await import("@/stores/app");
  let content = file.content;
  const lspLang = lspLangForFile(file.path, file.language || "");
  if (!opts?.skipFormat && appState.formatOnSave && lspLang) {
    try {
      const { formatActiveDocument } = await import("@/lib/lspCompletion");
      const ok = await formatActiveDocument(lspLang, file.path, content);
      if (ok) {
        content = editorState.openFiles.find((f) => f.path === file.path)?.content ?? content;
      }
    } catch (e) {
      // prompt-10 10-B: surface format failure (still continue to write).
      notifyWarning(
        `Format on Save failed: ${e instanceof Error ? e.message : String(e)}`,
      );
    }
  }
  try {
    await fileService.writeFile(file.path, content);
    markSaved(file.path);
    if (lspLang) {
      void import("@/api/services").then(({ lspService }) => {
        void lspService.didSaveDocument({
          language: lspLang,
          filePath: file.path,
          line: 0,
          column: 0,
          content,
        });
      });
      // prompt-10 10-D / 10-J: refresh diagnostics after save (+ eslint for JS/TS)
      void import("@/stores/lsp").then(({ refreshDiagnosticsToProblems }) => {
        void refreshDiagnosticsToProblems(lspLang, file.path, content);
      });
      if (lspLang === "typescript" || lspLang === "javascript") {
        void import("@/stores/toolchain").then(({ runToolchainCommand }) => {
          void runToolchainCommand("eslint-file", file.path);
        });
      }
    }
    return true;
  } catch (e: unknown) {
    const msg = errorMessage(e);
    console.error("Failed to save file:", msg);
    // 10-B: keep dirty, hard error for write failure
    notifyError(`Save failed: ${msg}`);
    return false;
  }
}

/**
 * Saves the active file to disk and clears its dirty flag.
 * prompt-9 9-A + prompt-10 10-B: FoS with visible failures.
 */
export async function saveFile(): Promise<void> {
  const file = activeFile.value;
  if (!file) return;
  await saveFilePath(file.path);
}

/**
 * prompt-10 10-A: save every dirty buffer to disk.
 * Returns number of successfully saved files.
 */
export async function saveAllFiles(): Promise<number> {
  const dirty = getDirtyFiles();
  let n = 0;
  for (const f of dirty) {
    if (await saveFilePath(f.path)) n += 1;
  }
  return n;
}

/**
 * Opens a file by absolute/workspace path. On failure notifies the user and
 * rethrows so callers (apply-to-editor) can avoid false-success (prompt-5 Task A).
 */
export async function openFileFromPath(path: string): Promise<void> {
  try {
    const content = await fileService.readFile(path);
    openFile(path, content);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    notifyError(`Failed to open file: ${msg}`);
    throw err instanceof Error ? err : new Error(msg);
  }
}

/**
 * Opens (if needed) the target file and shows a Diff preview modal.
 * Does NOT write content until the user confirms via confirmApplyDiff.
 * Returns false when path is missing, open fails, or the file still isn't open.
 */
export async function requestApplyToEditor(path: string, code: string): Promise<boolean> {
  if (!path?.trim()) {
    notifyError(translate("aiWindow.noActiveFile"), translate("aiWindow.applyTitle"));
    return false;
  }
  if (typeof code !== "string") {
    notifyError(translate("aiWindow.noActiveFile"), translate("aiWindow.applyTitle"));
    return false;
  }
  try {
    let file = editorState.openFiles.find((f) => f.path === path);
    if (!file) {
      await openFileFromPath(path);
      file = editorState.openFiles.find((f) => f.path === path);
    }
    if (!file) {
      notifyError(translate("aiWindow.noActiveFile"), translate("aiWindow.applyTitle"));
      return false;
    }
    applyDiffState.path = path;
    applyDiffState.original = file.content;
    applyDiffState.modified = code;
    applyDiffState.language = file.language || detectLanguage(path);
    applyDiffState.visible = true;
    return true;
  } catch {
    // openFileFromPath already notified
    return false;
  }
}

export function cancelApplyDiff(): void {
  applyDiffState.visible = false;
  applyDiffState.path = "";
  applyDiffState.original = "";
  applyDiffState.modified = "";
  applyDiffState.language = "";
}

/**
 * Confirms the pending Diff apply: optional snapshot, then updateContent.
 * Reports success only when the buffer was actually updated.
 */
export async function confirmApplyDiff(): Promise<boolean> {
  if (!applyDiffState.path || !applyDiffState.visible) return false;
  // Optional safety snapshot before overwrite (prompt-5 Task A).
  if (appState.currentProject) {
    try {
      const { createSnapshot } = await import("@/stores/snapshot");
      await createSnapshot("pre-apply");
    } catch {
      // Snapshot is best-effort; apply still proceeds.
    }
  }
  const ok = updateContent(applyDiffState.path, applyDiffState.modified);
  if (!ok) {
    notifyError(translate("aiWindow.noActiveFile"), translate("aiWindow.applyTitle"));
    return false;
  }
  const name =
    applyDiffState.path.split(/[/\\]/).pop() ?? applyDiffState.path;
  notifySuccess(translate("aiChat.appliedTo", { name }));
  cancelApplyDiff();
  return true;
}

let autoSaveTimer: ReturnType<typeof setTimeout> | null = null;

export function setupAutoSave(autoSave: () => boolean, autoSaveDelay: () => string) {
  watch(
    () => activeFile.value?.content,
    (newContent, oldContent) => {
      if (!autoSave() || !activeFile.value || newContent === oldContent) return;
      if (autoSaveTimer) clearTimeout(autoSaveTimer);
      const delay = parseInt(autoSaveDelay(), 10) || 1000;
      autoSaveTimer = setTimeout(() => {
        saveFile();
      }, delay);
    }
  );
}

export function saveOnFocusChange(autoSave: () => boolean) {
  if (autoSave() && activeFile.value?.isDirty) {
    saveFile();
  }
}