import { createApp } from "vue";
import ElementPlus from "element-plus";
import "element-plus/dist/index.css";
import "element-plus/theme-chalk/dark/css-vars.css";
import * as ElementPlusIconsVue from "@element-plus/icons-vue";
import "animate.css";
import "./assets/styles/main.css";
import App from "./App.vue";
import router from "./router";
import { loadSettings, initThemes, startSystemModeListener, initWindowMaximiseListener, appState } from "@/stores/app";
import { setSandboxMode } from "@/lib/pluginRegistry";
import { loadLayoutFromBackend } from "@/stores/layout";
import { loadWorkflows } from "@/stores/workflows";
import { loadPlugins, activateStartupPlugins } from "@/stores/plugins";
import { layoutService } from "@/api/services";
import { notifyError } from "@/lib/notifications";
import { pushOutput } from "@/stores/output";
import { initConnectivityListener } from "@/lib/connectivity";
import { initLSPStore } from "@/stores/lsp";
// Plan 11 Task 15: 个性化运行时（背景图/字体/气泡 CSS 变量应用）
import { initPersonalization } from "@/composables/usePersonalization";
import { Events } from "@wailsio/runtime";
import { requestApplyToEditor } from "@/stores/editor";
import { setSnapshotWorkspaceRoot } from "@/stores/snapshot";
import { translate } from "@/lib/i18n";

// Monaco editor: configure the @guolao/vue-monaco-editor loader to use the
// locally bundled monaco-editor package instead of loading from CDN.
// Without this, the loader tries to fetch the AMD bundle from
// https://cdn.jsdelivr.net/npm/monaco-editor@.../min/vs/loader.js, which
// the app's strict CSP (connect-src 'self', script-src 'self' 'nonce-<N>')
// blocks — causing a "load failed" error when opening any file.
// We also set up MonacoEnvironment so web workers are bundled by Vite
// via ?worker imports instead of being fetched from CDN at runtime.
import { loader } from "@guolao/vue-monaco-editor";
import * as monaco from "monaco-editor";
import editorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import jsonWorker from "monaco-editor/esm/vs/language/json/json.worker?worker";
import cssWorker from "monaco-editor/esm/vs/language/css/css.worker?worker";
import htmlWorker from "monaco-editor/esm/vs/language/html/html.worker?worker";
import tsWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker";

loader.config({ monaco });

self.MonacoEnvironment = {
  getWorker(_workerId: string, label: string) {
    if (label === "json") return new jsonWorker();
    if (label === "css") return new cssWorker();
    if (label === "html") return new htmlWorker();
    if (label === "typescript" || label === "javascript") return new tsWorker();
    return new editorWorker();
  },
};

/**
 * Proposal F (prompt-4.md): Explicit bootstrap sequence.
 *
 * Each stage is clearly named so that "island code" (modules with init
 * functions that exist but are never called) is visible at a glance.
 * Stages are ordered by dependency: themes → settings → sandbox → plugins → layout.
 * Each stage's failure is logged but does not block subsequent stages
 * (except settings, which sandbox depends on).
 *
 * N-118 / Proposal AD: the whole bootstrap() is wrapped in try/catch so
 * that a failure in any stage (loadSettings, loadLayoutFromBackend,
 * loadWorkflows, etc.) is surfaced to the user instead of being silently
 * swallowed by the `void bootstrap()` fire-and-forget call.
 */
async function bootstrap(): Promise<void> {
  try {
    // Stage 1: Initialize themes (sync, no I/O).
    initThemes();

    // Stage 2: Start system mode listener (sync, listens for OS theme changes).
    startSystemModeListener();
    // N-152: 订阅窗口最大化状态事件，标题栏据此切换放大/还原图标。
    // 放在 bootstrap 早期，确保 TitleBar 挂载时状态已就绪。
    initWindowMaximiseListener();

    // Stage 3: Load settings (async, reads settings.json from profile dir).
    // This also hydrates appState with the user's preferences.
    await loadSettings();

    // Stage 3.1 (Task 15): apply personalization CSS variables now that
    // appState.personalization is hydrated, and watch for later changes.
    try {
      initPersonalization();
    } catch (e: unknown) {
      console.error("Personalization init failed:", e);
    }

    // Stage 3.5 (G-FEAT-02): Start connectivity listener now that aiBaseUrl
    // is hydrated, and probe for installed LSP servers (gopls / tsserver).
    // Both are best-effort and must never block bootstrap — failures only
    // mean offline completion is unavailable, not that the IDE won't start.
    try {
      initConnectivityListener();
    } catch (e: unknown) {
      console.error("Connectivity listener init failed:", e);
    }
    try {
      void initLSPStore();
    } catch (e: unknown) {
      console.error("LSP store init failed:", e);
    }

    // Stage 4: Enable plugin sandbox based on the loaded setting.
    // N-29: sandbox is on by default; users can disable it in Settings.
    setSandboxMode(appState.enablePluginSandbox);

    // Stage 4.5 (N-41 / Proposal K): Load and activate startup plugins.
    // This MUST happen before layout loading because plugins may register
    // views that the layout engine needs to render. Best-effort — errors
    // are logged to the Output panel's Plugins channel but do not block
    // subsequent bootstrap stages. Without this stage, the entire plugin
    // system stays dormant until the user manually opens PluginsView.
    try {
      await loadPlugins();
      await activateStartupPlugins();
    } catch (e: unknown) {
      // loadPlugins/activateStartupPlugins already capture errors into
      // pluginStore.error; this catch is a defensive net for unexpected throws.
      console.error("Plugin bootstrap failed:", e);
    }

    // Stage 5: Load persisted layout tree (async, reads layout.json).
    // N-30: each profile has its own layout; falls back to default on error.
    await loadLayoutFromBackend(layoutService.loadLayout);

    // Stage 6: Load workflow definitions.
    // G-SEC-03: Startup workflows are NOT auto-run on project load. They are
    // exposed via the pendingStartupWorkflows computed in the workflows store
    // so the UI can present them as "Pending Confirmation" and require the
    // user to explicitly click "Run". This prevents malicious startup
    // workflows in cloned repositories from auto-running shell commands.
    if (appState.currentProject) {
      await loadWorkflows(appState.currentProject);
      // prompt-4 Task 10: 项目打开后激活快照工作区根
      setSnapshotWorkspaceRoot(appState.currentProject);
    }

    // Stage 7 (prompt-4 Task 5 / prompt-5 Task A): 主窗口监听 AI 伴侣窗口的
    // 「应用到编辑器」事件。AI 窗口是独立 Webview，不共享 Vue 状态，只能通过
    // Wails 事件回写。打开文件失败会 rethrow；写入前走 Diff 预览确认，避免假成功。
    // 仅在非 /ai-window 路由下注册，避免 AI 窗口自身重复处理。
    try {
      if (!window.location.hash.includes("ai-window")) {
        Events.On("ai:apply-to-editor", (event: unknown) => {
          const raw = (event as { data?: unknown } | null)?.data;
          const payload = (Array.isArray(raw) ? raw[0] : raw) as
            | { code?: string; filePath?: string; language?: string }
            | undefined;
          if (!payload?.code) return;
          // filePath 必须由发送方携带（选中发送时缓存）；不再静默写空路径。
          const path = (payload.filePath || "").trim();
          if (!path) {
            notifyError(translate("aiWindow.noActiveFile"), translate("aiWindow.applyTitle"));
            return;
          }
          void requestApplyToEditor(path, payload.code);
        });
      }
    } catch (e: unknown) {
      console.error("ai:apply-to-editor listener failed:", e);
    }
  } catch (e: unknown) {
    // N-118: surface bootstrap failures to the user instead of letting
    // them vanish as unhandled rejections. The app still mounts (the UI
    // is responsive), but the user is told that startup was incomplete.
    const msg = e instanceof Error ? e.message : String(e);
    console.error("Bootstrap failed:", e);
    pushOutput("ide", "error", `Startup failed: ${msg}`);
    try {
      notifyError(`Startup failed: ${msg}`, "Bootstrap error");
    } catch {
      // notifyError may itself fail if Element Plus hasn't mounted yet;
      // the pushOutput above still records the error.
    }
  }
}

const app = createApp(App);

// Register all Element Plus icons
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component);
}

app.use(ElementPlus, {
  size: "default",
});
app.use(router);

// N-97 / Proposal AD: Global Vue error handler. Catches errors from
// component render, setup, watchers, and lifecycle hooks that would
// otherwise only appear in the browser console. We log to the Output
// panel and show a notification so the user knows something went wrong.
app.config.errorHandler = (err, _instance, info) => {
  const msg = err instanceof Error ? err.message : String(err);
  console.error("[Vue errorHandler]", err, info);
  pushOutput("ide", "error", `Vue error (${info}): ${msg}`);
  try {
    notifyError(`${msg}`, "Vue error");
  } catch {
    // notification may fail during early startup; the Output log still records it
  }
};

// N-98 / Proposal AD: window-level error and rejection handlers. These
// catch errors that escape Vue's errorHandler (e.g. errors in event
// listeners, async callbacks, or third-party scripts) and rejected
// promises that nothing else caught. Without these, the default browser
// behavior is to log to console only — users have no idea something broke.
if (typeof window !== "undefined") {
  window.addEventListener("error", (event) => {
    // event.error may be undefined for cross-origin script errors; fall
    // back to event.message.
    const msg = event.message || (event.error instanceof Error ? event.error.message : "Unknown error");
    console.error("[window error]", event.error ?? event.message);
    pushOutput("ide", "error", `Uncaught error: ${msg}`);
    try {
      notifyError(msg, "Uncaught error");
    } catch {
      // notification may fail during early startup
    }
  });

  window.addEventListener("unhandledrejection", (event) => {
    const reason = event.reason;
    const msg = reason instanceof Error ? reason.message : String(reason);
    console.error("[unhandledrejection]", reason);
    pushOutput("ide", "error", `Unhandled promise rejection: ${msg}`);
    try {
      notifyError(msg, "Unhandled rejection");
    } catch {
      // notification may fail during early startup
    }
  });
}

// Mount the app immediately for a responsive first paint, then run the
// async bootstrap sequence. Settings/layout will update the UI when ready.
// N-118: bootstrap() now has its own try/catch, so `void` is safe — no
// unhandled rejection will escape.
void bootstrap();
app.mount("#app");
