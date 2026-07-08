import { reactive, computed, watch } from "vue";
import { detectLanguage } from "@/lib/language";
import { fileService } from "@/api/services";
import { notifyError, notifyWarning } from "@/lib/notifications";
import { errorMessage } from "@/lib/errors";

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

export const editorState = reactive<EditorState>({
  openFiles: [],
  activeFilePath: null,
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
  editorState.openFiles.splice(idx, 1);
  if (editorState.activeFilePath === path) {
    const next = editorState.openFiles[idx] ?? editorState.openFiles[idx - 1] ?? null;
    editorState.activeFilePath = next?.path ?? null;
  }
}

export function updateContent(path: string, content: string): void {
  const file = editorState.openFiles.find((f) => f.path === path);
  if (file) {
    file.content = content;
    file.isDirty = content !== file.originalContent;
  }
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

/**
 * Saves the active file to disk and clears its dirty flag.
 * N-96: surfaces save failures to the user (was silent console.error only).
 * The autoSave path calls this, so autoSave failures are now visible too.
 */
export async function saveFile(): Promise<void> {
  const file = activeFile.value;
  if (!file) return;
  try {
    await fileService.writeFile(file.path, file.content);
    markSaved(file.path);
  } catch (e: unknown) {
    const msg = errorMessage(e);
    console.error("Failed to save file:", msg);
    // N-96: autoSave failures are shown as a warning (less intrusive than
    // an error, since autoSave retries on the next change). The explicit
    // Ctrl+S path in EditorView.vue shows a full error notification.
    notifyWarning(`Auto-save failed: ${msg}`);
  }
}

export async function openFileFromPath(path: string): Promise<void> {
  try {
    const content = await fileService.readFile(path);
    openFile(path, content);
  } catch (err) {
    notifyError(`Failed to open file: ${err instanceof Error ? err.message : String(err)}`);
  }
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