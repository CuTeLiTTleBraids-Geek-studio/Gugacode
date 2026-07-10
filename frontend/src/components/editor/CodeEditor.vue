<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { VueMonacoEditor } from "@guolao/vue-monaco-editor";
import type * as monacoEditor from "monaco-editor";
import { appState } from "@/stores/app";
import { detectLanguage } from "@/lib/language";
import { runAIAction } from "@/stores/ai";
import { requestCompletion } from "@/stores/inlineCompletion";
import { runToolchainCommand } from "@/stores/toolchain";
import { notifyWarning, notifySuccess } from "@/lib/notifications";
import type { AIActionName } from "@/types";
import { getMonacoThemeNameForMode } from "@/lib/monaco-themes";
import { useI18n } from "@/lib/i18n";
import { registerLSPProviders } from "@/lib/lspCompletion";
import { windowService } from "@/api/services";

/** Active Monaco instance for Problems/search jump (prompt-10 10-D). */
const editorInstance = ref<monacoEditor.editor.IStandaloneCodeEditor | null>(null);

const { t } = useI18n();

const props = defineProps<{
  path: string;
  content: string;
  language?: string;
}>();

const emit = defineEmits<{
  (e: "update:content", value: string): void;
  (e: "cursor-change", line: number, column: number): void;
}>();

// Resolve the Monaco theme based on both the accent and the resolved mode
// (dark/light). We watch appState.theme (the user's choice) and read the
// effective <html data-mode> attribute that applyMode() keeps in sync, so
// the editor flips when the user switches mode or (for "system") when the
// OS preference changes.
function resolvedMode(): "dark" | "light" {
  const m = document.documentElement.getAttribute("data-mode");
  return m === "light" ? "light" : "dark";
}

const monacoTheme = computed(() => {
  // Touch appState.theme so this recomputes when the user switches mode.
  void appState.theme;
  return getMonacoThemeNameForMode(appState.accentTheme, resolvedMode());
});

const resolvedLanguage = computed(() => props.language ?? detectLanguage(props.path));

// G-PERF-01: Monaco virtualizes line rendering by default — only visible
// lines (plus a small overscan) are realized in the DOM, so opening large
// files does not scale the DOM with line count. No explicit option is
// required; this is inherent to Monaco's viewport-based renderer. We avoid
// disabling it (there is no opt-out flag) and rely on the default here.
const options = computed((): monacoEditor.editor.IStandaloneEditorConstructionOptions => ({
  fontSize: appState.fontSize,
  fontFamily: appState.fontFamily,
  tabSize: appState.tabSize,
  wordWrap: appState.wordWrap ? "on" : "off",
  lineNumbers: appState.lineNumbers ? "on" : "off",
  minimap: { enabled: appState.minimap },
  automaticLayout: true,
  scrollBeyondLastLine: false,
  smoothScrolling: true,
  cursorBlinking: "smooth",
  renderWhitespace: "selection",
  bracketPairColorization: { enabled: true },
}));

function registerContextMenuActions(editor: monacoEditor.editor.IStandaloneCodeEditor) {
  // prompt-4 Task 5: 发送选中代码到 AI 独立窗口（置顶快捷入口）
  editor.addAction({
    id: "ai-send-to-window",
    label: t("codeEditor.sendToAIWindow"),
    contextMenuGroupId: "ai-navigation",
    contextMenuOrder: 0,
    keybindings: [
      // prompt-5 Task C / BUG-M1: Ctrl/Cmd+Shift+A — send selection to AI window
      // monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.KeyA
      // Numeric form avoids depending on monaco namespace at action-register time.
      2048 /* CtrlCmd */ | 1024 /* Shift */ | 31 /* KeyA */,
    ],
    run: (ed: monacoEditor.editor.IStandaloneCodeEditor) => {
      const selection = ed.getSelection();
      const model = ed.getModel();
      if (!model) return;
      const selectedText =
        selection && !selection.isEmpty() ? model.getValueInRange(selection) : "";
      if (!selectedText) {
        notifyWarning(t("codeEditor.selectCodeFirst"));
        return;
      }
      const filePath = appState.currentFilePath ?? props.path ?? "untitled";
      const language = model.getLanguageId();
      void windowService
        .sendSelectionToAI(selectedText, language, filePath)
        .then(() => notifySuccess(t("codeEditor.sentToAIWindow")))
        .catch((e: unknown) =>
          notifyWarning(e instanceof Error ? e.message : String(e)),
        );
    },
  });

  const aiActions: Array<{ id: string; label: string; action: AIActionName }> = [
    { id: "ai-explain-ctx", label: t("codeEditor.aiExplain"), action: "explain" },
    { id: "ai-refactor-ctx", label: t("codeEditor.aiRefactor"), action: "refactor" },
    { id: "ai-fix-ctx", label: t("codeEditor.aiFix"), action: "fix" },
    { id: "ai-docs-ctx", label: t("codeEditor.aiDocs"), action: "generate_docs" },
    { id: "ai-tests-ctx", label: t("codeEditor.aiTests"), action: "generate_tests" },
    { id: "ai-optimize-ctx", label: t("codeEditor.aiOptimize"), action: "optimize" },
    { id: "ai-review-ctx", label: t("codeEditor.aiReview"), action: "review" },
    { id: "ai-security-ctx", label: t("codeEditor.aiSecurity"), action: "security" },
  ];

  aiActions.forEach((act, index) => {
    editor.addAction({
      id: act.id,
      label: act.label,
      contextMenuGroupId: "ai-navigation",
      contextMenuOrder: index + 1,
      run: (ed: monacoEditor.editor.IStandaloneCodeEditor) => {
        const selection = ed.getSelection();
        const model = ed.getModel();
        if (!model) return;
        const selectedText =
          selection && !selection.isEmpty() ? model.getValueInRange(selection) : "";
        if (!selectedText) {
          notifyWarning(t("codeEditor.selectCodeFirst"));
          return;
        }
        const filePath = appState.currentFilePath ?? "untitled";
        const language = model.getLanguageId();
        void runAIAction(act.action, selectedText, language, filePath);
      },
    });
  });

  // G-FEAT-03: right-click "Run <tool>" entries for the current file. The
  // offered commands depend on the file's language so only relevant tools
  // clutter the menu.
  const lang = resolvedLanguage.value;
  const toolchainCtx: Array<{ id: string; label: string; cmd: string }> = [];
  if (lang === "go") {
    toolchainCtx.push(
      { id: "tc-golangci-lint-ctx", label: t("toolchain.ctxGolangciLint"), cmd: "golangci-lint" },
      { id: "tc-go-build-ctx", label: t("toolchain.ctxGoBuild"), cmd: "go-build" },
      { id: "tc-go-vet-ctx", label: t("toolchain.ctxGoVet"), cmd: "go-vet" },
      { id: "tc-gofmt-ctx", label: t("toolchain.ctxGofmt"), cmd: "gofmt-file" },
    );
  } else if (lang === "typescript" || lang === "javascript") {
    toolchainCtx.push(
      { id: "tc-eslint-ctx", label: t("toolchain.ctxEslint"), cmd: "eslint-file" },
      { id: "tc-tsc-ctx", label: t("toolchain.ctxTsc"), cmd: "tsc" },
      { id: "tc-prettier-ctx", label: t("toolchain.ctxPrettier"), cmd: "prettier-file" },
    );
  }
  toolchainCtx.forEach((act, index) => {
    editor.addAction({
      id: act.id,
      label: act.label,
      contextMenuGroupId: "toolchain",
      contextMenuOrder: index,
      run: () => {
        const filePath = appState.currentFilePath ?? props.path ?? "";
        void runToolchainCommand(act.cmd, filePath);
      },
    });
  });
  // prompt-9 9-C / 9-H: Test at Cursor
  editor.addAction({
    id: "tc-test-at-cursor",
    label: t("toolchain.ctxTestAtCursor"),
    contextMenuGroupId: "toolchain",
    contextMenuOrder: 20,
    // Numeric form: CtrlCmd|Shift|KeyT (same pattern as Ctrl+Shift+A above)
    keybindings: [2048 /* CtrlCmd */ | 1024 /* Shift */ | 46 /* KeyT */],
    run: (ed) => {
      const filePath = appState.currentFilePath ?? props.path ?? "";
      const model = ed.getModel();
      if (!model || !filePath) return;
      const pos = ed.getPosition();
      const line = pos ? pos.lineNumber - 1 : 0;
      const lspLang =
        lang === "go" ? "go" : lang === "typescript" || lang === "javascript" ? lang : "";
      if (!lspLang) return;
      void import("@/stores/toolchain").then(({ runTestAtCursor }) => {
        void runTestAtCursor(lspLang, filePath, line, model.getValue());
      });
    },
  });
}

function registerInlineCompletionProvider(
  editor: monacoEditor.editor.IStandaloneCodeEditor,
  monaco: typeof import("monaco-editor")
): monacoEditor.IDisposable {
  // N-64: registerInlineCompletionsProvider returns an IDisposable that
  // must be disposed on unmount. Without disposal, every editor mount
  // (e.g. switching tabs in LayoutView) leaks a provider on the global
  // monaco.languages registry, causing duplicate completion requests
  // and memory growth over a long session.
  return monaco.languages.registerInlineCompletionsProvider({ pattern: "**" }, {
    provideInlineCompletions: async (model, position) => {
      const language = resolvedLanguage.value;
      const filePath = props.path;

      // Get prefix (code before cursor, up to ~50 lines for context)
      const startLine = Math.max(1, position.lineNumber - 50);
      const prefix = model.getValueInRange({
        startLineNumber: startLine,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      });

      // Get suffix (code after cursor, up to ~30 lines)
      const lineCount = model.getLineCount();
      const endLine = Math.min(lineCount, position.lineNumber + 30);
      const suffix = model.getValueInRange({
        startLineNumber: position.lineNumber,
        startColumn: position.column,
        endLineNumber: endLine,
        endColumn: model.getLineMaxColumn(endLine),
      });

      const text = await requestCompletion(prefix, suffix, language, filePath);
      if (!text) {
        return { items: [] };
      }

      return {
        items: [
          {
            insertText: text,
            range: {
              startLineNumber: position.lineNumber,
              startColumn: position.column,
              endLineNumber: position.lineNumber,
              endColumn: position.column,
            },
          },
        ],
      };
    },
    freeInlineCompletions: () => {
      // Completions hold no external resources; required by monaco 0.52+ API.
    },
  });
}

// Track disposables created on mount so we can clean them up on unmount.
// N-64: the inline completion provider is registered on the global
// monaco.languages registry, so it survives editor dispose and must be
// explicitly disposed here.
const inlineCompletionProvider = ref<monacoEditor.IDisposable | null>(null);
// G-FEAT-02: LSP completion-item + hover providers, also registered on the
// global monaco.languages registry, so they must be disposed on unmount.
const lspProvidersDisposable = ref<monacoEditor.IDisposable | null>(null);
const cursorListener = ref<monacoEditor.IDisposable | null>(null);

let coverageDecorations: string[] = [];

function applyCoverageDecorations(
  editor: monacoEditor.editor.IStandaloneCodeEditor,
  monaco: typeof import("monaco-editor"),
) {
  void import("@/stores/coverage").then(({ coverageHitsForFile }) => {
    const hits = coverageHitsForFile(props.path || "");
    const decs: monacoEditor.editor.IModelDeltaDecoration[] = hits.map((h) => ({
      range: new monaco.Range(h.line, 1, h.line, 1),
      options: {
        isWholeLine: true,
        className: h.covered ? "coverage-line--covered" : "coverage-line--uncovered",
        linesDecorationsClassName: h.covered ? "coverage-gutter--covered" : "coverage-gutter--uncovered",
        overviewRuler: {
          color: h.covered ? "rgba(46,160,67,0.6)" : "rgba(248,81,73,0.6)",
          position: monaco.editor.OverviewRulerLane.Left,
        },
      },
    }));
    coverageDecorations = editor.deltaDecorations(coverageDecorations, decs);
  });
}

function handleMount(
  editor: monacoEditor.editor.IStandaloneCodeEditor,
  monaco: typeof import("monaco-editor")
) {
  editorInstance.value = editor;
  cursorListener.value = editor.onDidChangeCursorPosition((e: monacoEditor.editor.ICursorPositionChangedEvent) => {
    emit("cursor-change", e.position.lineNumber, e.position.column);
  });
  registerContextMenuActions(editor);
  inlineCompletionProvider.value = registerInlineCompletionProvider(editor, monaco);
  // G-FEAT-02: register LSP-backed popup completion + hover providers. These
  // coexist with the AI inline completion above because they use a different
  // Monaco API (registerCompletionItemProvider vs registerInlineCompletionsProvider).
  // Only register once per editor instance; disposing happens on unmount.
  if (!lspProvidersDisposable.value) {
    // prompt-8 Task 8-C: pass absolute open-file path for URI correctness.
    lspProvidersDisposable.value = registerLSPProviders(monaco, props.path);
  }
  // prompt-10 10-H: coverage gutter
  applyCoverageDecorations(editor, monaco);
  void import("@/stores/coverage").then(({ coverageState }) => {
    watch(
      () => coverageState.hits.length,
      () => applyCoverageDecorations(editor, monaco),
    );
  });
}

// prompt-10 10-D: jump caret when Problems / external nav requests it
watch(
  () => appState.editorJumpSeq,
  () => {
    const ed = editorInstance.value;
    if (!ed || !appState.editorJumpSeq) return;
    const line = Math.max(1, appState.cursorLine || 1);
    const col = Math.max(1, appState.cursorColumn || 1);
    ed.focus();
    ed.setPosition({ lineNumber: line, column: col });
    ed.revealLineInCenter(line);
  },
);

onBeforeUnmount(() => {
  // Dispose in reverse order of creation. Each .dispose() is guarded so
  // a throw in one doesn't skip the others.
  try { inlineCompletionProvider.value?.dispose(); } catch { /* already disposed */ }
  try { lspProvidersDisposable.value?.dispose(); } catch { /* already disposed */ }
  try { cursorListener.value?.dispose(); } catch { /* already disposed */ }
  inlineCompletionProvider.value = null;
  lspProvidersDisposable.value = null;
  cursorListener.value = null;
  editorInstance.value = null;
});

function handleChange(value: string | undefined) {
  emit("update:content", value ?? "");
}
</script>

<template>
  <div class="code-editor">
    <VueMonacoEditor
      :value="content"
      :language="resolvedLanguage"
      :theme="monacoTheme"
      :options="options"
      @mount="handleMount"
      @change="handleChange"
    />
  </div>
</template>

<style scoped>
.code-editor {
  width: 100%;
  height: 100%;
}

.code-editor :deep(.monaco-editor) {
  background-color: var(--color-bg-base);
}

.code-editor :deep(.monaco-editor .margin) {
  background-color: var(--color-bg-base);
}
</style>
