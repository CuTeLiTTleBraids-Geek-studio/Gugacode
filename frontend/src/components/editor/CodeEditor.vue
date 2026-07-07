<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import { VueMonacoEditor } from "@guolao/vue-monaco-editor";
import type * as monacoEditor from "monaco-editor";
import { appState } from "@/stores/app";
import { detectLanguage } from "@/lib/language";
import { runAIAction } from "@/stores/ai";
import { requestCompletion } from "@/stores/inlineCompletion";
import { notifyWarning } from "@/lib/notifications";
import type { AIActionName } from "@/types";
import { getMonacoThemeNameForMode } from "@/lib/monaco-themes";
import { useI18n } from "@/lib/i18n";

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
      contextMenuOrder: index,
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
    disposeInlineCompletions: () => {
      // Nothing to dispose — completions hold no external resources
    },
  });
}

// Track disposables created on mount so we can clean them up on unmount.
// N-64: the inline completion provider is registered on the global
// monaco.languages registry, so it survives editor dispose and must be
// explicitly disposed here.
const inlineCompletionProvider = ref<monacoEditor.IDisposable | null>(null);
const cursorListener = ref<monacoEditor.IDisposable | null>(null);

function handleMount(
  editor: monacoEditor.editor.IStandaloneCodeEditor,
  monaco: typeof import("monaco-editor")
) {
  cursorListener.value = editor.onDidChangeCursorPosition((e: monacoEditor.editor.ICursorPositionChangedEvent) => {
    emit("cursor-change", e.position.lineNumber, e.position.column);
  });
  registerContextMenuActions(editor);
  inlineCompletionProvider.value = registerInlineCompletionProvider(editor, monaco);
}

onBeforeUnmount(() => {
  // Dispose in reverse order of creation. Each .dispose() is guarded so
  // a throw in one doesn't skip the others.
  try { inlineCompletionProvider.value?.dispose(); } catch { /* already disposed */ }
  try { cursorListener.value?.dispose(); } catch { /* already disposed */ }
  inlineCompletionProvider.value = null;
  cursorListener.value = null;
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
