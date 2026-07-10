import type * as monacoEditor from "monaco-editor";
import {
  getLSPCompletions,
  getLSPHover,
  getLSPDefinition,
  getLSPReferences,
  formatLSPDocument,
  closeLSPDocument,
} from "@/stores/lsp";
import { editorState } from "@/stores/editor";
import { updateContent } from "@/stores/editor";

/**
 * G-FEAT-02 + prompt-8: Monaco LSP integration.
 *
 * Completion, hover, definition, references, format.
 * Paths: always prefer openFiles[].path (absolute disk path), not model.uri
 * alone (BUG-IDE-03).
 */

function lspKindToMonaco(kind: number): number {
  if (kind < 0 || kind > 25) return 0;
  return kind;
}

/**
 * Resolve absolute disk path for an editor model (prompt-8 Task 8-C).
 * Prefer active/open file path matching the model; fall back to URI path.
 */
export function resolveModelFilePath(
  model: monacoEditor.editor.ITextModel,
  preferredPath?: string,
): string {
  if (preferredPath && preferredPath.length > 0 && !preferredPath.startsWith("inmemory:")) {
    return preferredPath;
  }
  const uriPath = model.uri.path || model.uri.toString();
  // Match open file by basename or path suffix.
  const open = editorState.openFiles.find((f) => {
    if (!f.path) return false;
    if (f.path === uriPath) return true;
    const norm = f.path.replace(/\\/g, "/");
    const up = uriPath.replace(/\\/g, "/");
    return up.endsWith(norm) || norm.endsWith(up.replace(/^\//, ""));
  });
  if (open?.path) return open.path;
  // Windows Monaco may give /C:/...
  if (/^\/[A-Za-z]:\//.test(uriPath)) {
    return uriPath.slice(1).replace(/\//g, "\\");
  }
  return uriPath;
}

/** Map Monaco language id (+ path) to LSP language key. */
export function monacoLangToLSPKey(monacoLang: string, filePath: string): string | null {
  const lower = filePath.toLowerCase();
  if (monacoLang === "go" || lower.endsWith(".go")) return "go";
  if (
    monacoLang === "typescript" ||
    monacoLang === "typescriptreact" ||
    lower.endsWith(".ts") ||
    lower.endsWith(".tsx")
  ) {
    return "typescript";
  }
  if (
    monacoLang === "javascript" ||
    monacoLang === "javascriptreact" ||
    lower.endsWith(".js") ||
    lower.endsWith(".jsx")
  ) {
    return "javascript";
  }
  return null;
}

/**
 * Register LSP-backed providers. Returns IDisposable for unmount cleanup.
 */
export function registerLSPProviders(
  monaco: typeof import("monaco-editor"),
  preferredPath?: string,
): monacoEditor.IDisposable {
  const disposables: monacoEditor.IDisposable[] = [];
  const languages = ["go", "typescript", "javascript", "typescriptreact", "javascriptreact"];

  for (const lang of languages) {
    disposables.push(
      monaco.languages.registerCompletionItemProvider(lang, {
        triggerCharacters: [".", ":"],
        provideCompletionItems: async (model, position) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return { suggestions: [] };
          const line = position.lineNumber - 1;
          const column = position.column - 1;
          const content = model.getValue();
          const items = await getLSPCompletions(lspLang, filePath, line, column, content);
          if (items.length === 0) return { suggestions: [] };
          const word = model.getWordUntilPosition(position);
          const range = {
            startLineNumber: position.lineNumber,
            endLineNumber: position.lineNumber,
            startColumn: word.startColumn,
            endColumn: word.endColumn,
          };
          // prompt-10 10-I: map additionalTextEdits (auto-import) onto Monaco
          const suggestions: monacoEditor.languages.CompletionItem[] = items.map((item) => {
            const base: monacoEditor.languages.CompletionItem = {
              label: item.label,
              kind: lspKindToMonaco(item.kind) as monacoEditor.languages.CompletionItemKind,
              detail: item.detail || undefined,
              insertText: item.insertText || item.label,
              insertTextRules: 0,
              range,
            };
            if (item.additionalEdits?.length) {
              base.additionalTextEdits = item.additionalEdits.map((e) => ({
                range: {
                  startLineNumber: e.startLine + 1,
                  startColumn: e.startCol + 1,
                  endLineNumber: e.endLine + 1,
                  endColumn: e.endCol + 1,
                },
                text: e.newText,
              }));
            }
            return base;
          });
          return { suggestions };
        },
      }),
    );

    disposables.push(
      monaco.languages.registerHoverProvider(lang, {
        provideHover: async (model, position) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return null;
          const hover = await getLSPHover(
            lspLang,
            filePath,
            position.lineNumber - 1,
            position.column - 1,
            model.getValue(),
          );
          if (!hover) return null;
          return {
            range: {
              startLineNumber: position.lineNumber,
              endLineNumber: position.lineNumber,
              startColumn: position.column,
              endColumn: position.column,
            },
            contents: [{ value: hover }],
          };
        },
      }),
    );

    // prompt-8 Task 8-F: Go to Definition
    disposables.push(
      monaco.languages.registerDefinitionProvider(lang, {
        provideDefinition: async (model, position) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return null;
          const locs = await getLSPDefinition(
            lspLang,
            filePath,
            position.lineNumber - 1,
            position.column - 1,
            model.getValue(),
          );
          if (!locs.length) return null;
          return locs.map((loc) => ({
            uri: monaco.Uri.file(loc.filePath),
            range: {
              startLineNumber: loc.line + 1,
              startColumn: loc.column + 1,
              endLineNumber: (loc.endLine ?? loc.line) + 1,
              endColumn: (loc.endColumn ?? loc.column) + 1,
            },
          }));
        },
      }),
    );

    // prompt-8 Task 8-F: Find References
    disposables.push(
      monaco.languages.registerReferenceProvider(lang, {
        provideReferences: async (model, position) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return null;
          const locs = await getLSPReferences(
            lspLang,
            filePath,
            position.lineNumber - 1,
            position.column - 1,
            model.getValue(),
          );
          if (!locs.length) return null;
          return locs.map((loc) => ({
            uri: monaco.Uri.file(loc.filePath),
            range: {
              startLineNumber: loc.line + 1,
              startColumn: loc.column + 1,
              endLineNumber: (loc.endLine ?? loc.line) + 1,
              endColumn: (loc.endColumn ?? loc.column) + 1,
            },
          }));
        },
      }),
    );

    // prompt-8 Task 8-G: Document formatting
    disposables.push(
      monaco.languages.registerDocumentFormattingEditProvider(lang, {
        provideDocumentFormattingEdits: async (model) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return [];
          const edits = await formatLSPDocument(lspLang, filePath, model.getValue());
          return edits.map((e) => ({
            range: {
              startLineNumber: e.startLine + 1,
              startColumn: e.startCol + 1,
              endLineNumber: e.endLine + 1,
              endColumn: e.endCol + 1,
            },
            text: e.newText,
          }));
        },
      }),
    );

    // prompt-9 9-B + prompt-10 10-A: F2 Rename with preview confirm + apply dirty buffers
    disposables.push(
      monaco.languages.registerRenameProvider(lang, {
        provideRenameEdits: async (model, position, newName) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return null;
          const { renameSymbolWorkspace } = await import("@/stores/lsp");
          const files = await renameSymbolWorkspace(
            lspLang,
            filePath,
            position.lineNumber - 1,
            position.column - 1,
            model.getValue(),
            newName,
          );
          if (!files.length) {
            const { notifyWarning } = await import("@/lib/notifications");
            const { pushOutput } = await import("@/stores/output");
            notifyWarning("Rename produced no edits (server may not support it here)");
            pushOutput("LSP", "warn", `rename: no edits for ${filePath}`);
            return null;
          }
          // prompt-11 11-C: rename summary — path + edit count + short hunk preview
          const summaryLines = files.map((f) => {
            const n = f.edits?.length || 0;
            const first = f.edits?.[0];
            const hunk =
              first != null
                ? ` L${first.startLine + 1}:${(first.newText || "").slice(0, 40).replace(/\n/g, "⏎")}`
                : "";
            return `• ${f.filePath}  (${n} edit${n === 1 ? "" : "s"})${hunk}`;
          });
          const body =
            `Rename will modify ${files.length} file(s):\n\n` +
            summaryLines.slice(0, 16).join("\n") +
            (summaryLines.length > 16 ? "\n…" : "") +
            `\n\nApply → mark dirty. Save All (Ctrl+K S) writes disk. Failures → Output.`;
          try {
            const { ElMessageBox } = await import("element-plus");
            await ElMessageBox.confirm(body, "Rename preview", {
              type: "warning",
              confirmButtonText: "Apply",
              cancelButtonText: "Cancel",
              customClass: "rename-preview-box",
            });
          } catch {
            return null; // user cancelled
          }
          const { applied, failed } = await applyWorkspaceEditsDetailed(files);
          const { notifySuccess, notifyWarning } = await import("@/lib/notifications");
          const { pushOutput } = await import("@/stores/output");
          if (failed.length) {
            pushOutput("LSP", "error", `rename failed:\n${failed.join("\n")}`);
            notifyWarning(`Rename: ${applied} ok, ${failed.length} failed (see Output)`);
          } else if (applied > 0) {
            notifySuccess(`Rename applied to ${applied} file(s) (dirty — Save All to persist)`);
          } else {
            notifyWarning("Rename could not apply any file edits");
          }
          // Already applied to buffers; return empty so Monaco does not double-apply
          return { edits: [] };
        },
      }),
    );

    // prompt-9 9-G: Signature Help
    disposables.push(
      monaco.languages.registerSignatureHelpProvider(lang, {
        signatureHelpTriggerCharacters: ["(", ","],
        provideSignatureHelp: async (model, position) => {
          const filePath = resolveModelFilePath(model, preferredPath);
          const lspLang = monacoLangToLSPKey(lang, filePath);
          if (!lspLang) return null;
          const { getLSPSignatureHelp } = await import("@/stores/lsp");
          const help = await getLSPSignatureHelp(
            lspLang,
            filePath,
            position.lineNumber - 1,
            position.column - 1,
            model.getValue(),
          );
          if (!help?.label) return null;
          return {
            value: {
              signatures: [
                {
                  label: help.label,
                  documentation: help.documentation || undefined,
                  parameters: (help.parameters || []).map((p) => ({ label: p })),
                },
              ],
              activeSignature: help.activeSignature ?? 0,
              activeParameter: help.activeParameter ?? 0,
            },
            dispose: () => undefined,
          };
        },
      }),
    );
  }

  return {
    dispose() {
      for (const d of disposables) {
        try {
          d.dispose();
        } catch {
          /* already disposed */
        }
      }
    },
  };
}

/**
 * Apply a list of LSP text edits to a full document string (0-based lines/cols).
 * Applies from end-of-document backwards so offsets stay valid.
 */
export function applyTextEditsToContent(
  content: string,
  edits: Array<{ startLine: number; startCol: number; endLine: number; endCol: number; newText: string }>,
): string {
  if (!edits.length) return content;
  const lines = content.split("\n");
  const offsetAt = (line: number, col: number): number => {
    let o = 0;
    const max = Math.min(line, lines.length);
    for (let i = 0; i < max; i++) o += lines[i].length + 1; // +1 newline
    return o + Math.max(0, col);
  };
  const sorted = [...edits].sort((a, b) => {
    const oa = offsetAt(a.startLine, a.startCol);
    const ob = offsetAt(b.startLine, b.startCol);
    return ob - oa;
  });
  let result = content;
  for (const e of sorted) {
    const start = offsetAt(e.startLine, e.startCol);
    const end = offsetAt(e.endLine, e.endCol);
    result = result.slice(0, start) + e.newText + result.slice(Math.max(start, end));
  }
  return result;
}

/**
 * Apply LSP format edits to the open buffer (Format Document / Format on Save).
 */
export async function formatActiveDocument(
  language: string,
  filePath: string,
  content: string,
): Promise<boolean> {
  const lspLang = monacoLangToLSPKey(language, filePath) ?? language;
  if (!lspLang || !["go", "typescript", "javascript"].includes(lspLang)) return false;
  const edits = await formatLSPDocument(lspLang, filePath, content);
  if (!edits.length) return false;
  const next = applyTextEditsToContent(content, edits);
  if (next === content) return false;
  updateContent(filePath, next);
  return true;
}

/**
 * Apply multi-file rename edits (prompt-9 9-B / 11-C). Opens files as needed via updateContent/openFile.
 */
export async function applyWorkspaceEdits(
  files: Array<{ filePath: string; edits: Array<{ startLine: number; startCol: number; endLine: number; endCol: number; newText: string }> }>,
): Promise<number> {
  const { applied } = await applyWorkspaceEditsDetailed(files);
  return applied;
}

/** prompt-11 11-C: returns applied count + failed paths for Output. */
export async function applyWorkspaceEditsDetailed(
  files: Array<{ filePath: string; edits: Array<{ startLine: number; startCol: number; endLine: number; endCol: number; newText: string }> }>,
): Promise<{ applied: number; failed: string[] }> {
  let applied = 0;
  const failed: string[] = [];
  const { openFileFromPath } = await import("@/stores/editor");
  const { fileService } = await import("@/api/services");
  for (const f of files) {
    if (!f.edits?.length) continue;
    try {
      let content = editorState.openFiles.find((o) => o.path === f.filePath)?.content;
      if (content == null) {
        content = await fileService.readFile(f.filePath);
        await openFileFromPath(f.filePath);
      }
      const next = applyTextEditsToContent(content, f.edits);
      if (updateContent(f.filePath, next)) applied += 1;
      else failed.push(`${f.filePath}: updateContent failed`);
    } catch (e) {
      failed.push(`${f.filePath}: ${e instanceof Error ? e.message : String(e)}`);
    }
  }
  return { applied, failed };
}

export { closeLSPDocument };
