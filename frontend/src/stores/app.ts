import { reactive, computed, watch } from "vue";
import { Events } from "@wailsio/runtime";
import { settingsService, windowService } from "@/api/services";
import type { Settings, ShortcutKeys, ToolApprovalConfig, CustomAccentTheme, AIProviderConfig, PersonalizationConfig, AIWindowTheme } from "@/types";
import type { AccentTheme } from "@/lib/monaco-themes";
import { accentThemes, applyMonacoTheme, applyMonacoThemeForMode, registerAllThemes, registerCustomTheme } from "@/lib/monaco-themes";
import { PROVIDER_PRESETS } from "@/lib/aiProviders";
import { loadRules, clearRules } from "@/stores/rules";
import { translate } from "@/lib/i18n";
import { notifyError } from "@/lib/notifications";
import {
  loadCustomShortcuts,
  getCustomShortcuts,
} from "@/composables/useKeyboard";
// Plan 57 / N-23: theme editor functions extracted to a dedicated module.
import {
  applyCustomAccentTokens,
  clearCustomAccentTokens,
} from "@/stores/themeEditor";
// prompt-6 Task 1: dual-window settings sync origin.
import {
  getWindowOriginId,
  unwrapEventData,
  parseSyncOrigin,
} from "@/lib/windowOrigin";
// Re-export theme editor helpers for settings/UI consumers.
export {
  deriveAccentTokens,
  serializeCustomAccent,
  deserializeCustomAccent,
} from "@/stores/themeEditor";

export type PanelTab = "explorer" | "search" | "git" | "extensions" | "ai";
export type ThemeMode = "dark" | "light" | "system";
// G-VSC-01: sub-view of the "extensions" activity tab. "installed" shows the
// unified plugin/extension management panel (G-VSC-04); "marketplace" shows
// the Open VSX search/browse/install panel (G-VSC-01).
export type ExtensionsSubview = "installed" | "marketplace";

export interface AppState {
  sidebarCollapsed: boolean;
  sidebarWidth: number;
  activityBarWidth: number;
  panelTab: PanelTab;
  extensionsSubview: ExtensionsSubview;
  terminalVisible: boolean;
  terminalHeight: number;
  aiChatVisible: boolean;
  aiChatWidth: number;
  currentFilePath: string | null;
  currentProject: string | null;
  projectName: string | null;
  theme: string;
  accentTheme: AccentTheme;
  fontSize: number;
  fontFamily: string;
  tabSize: number;
  wordWrap: boolean;
  minimap: boolean;
  lineNumbers: boolean;
  statusBarVisible: boolean;
  breadcrumbVisible: boolean;
  // General settings
  language: string;
  autoUpdate: boolean;
  dataFolderPath: string;
  // N-29: plugin sandbox enabled (default true for v2).
  enablePluginSandbox: boolean;
  // Status bar state
  branchName: string;
  errors: number;
  warnings: number;
  cursorLine: number;
  cursorColumn: number;
  /** prompt-10 10-D: bump to force Monaco jump (Problems / search). */
  editorJumpSeq: number;
  encoding: string;
  languageMode: string;
  // AI config
  aiApiKey: string;
  // G-SEC-07: aiApiKey is no longer populated from LoadSettings (the backend
  // clears it). aiApiKeyConfigured signals whether a key is stored on disk;
  // aiApiKeyStorageMethod labels how it is stored. The plaintext key is kept
  // out of the JS heap where feasible.
  aiApiKeyConfigured: boolean;
  aiApiKeyStorageMethod: string;
  aiBaseUrl: string;
  aiModel: string;
  aiSystemPrompt: string;
  // Plan 54: optional user overrides for the other three built-in prompts.
  // Empty string means "use the built-in".
  aiAgentSystemPrompt: string;
  aiConversationTitlePrompt: string;
  aiInlineCompletionPrompt: string;
  cursorBlinking: string;
  cursorStyle: string;
  bracketColorization: boolean;
  autoSave: boolean;
  autoSaveDelay: string;
  aiProvider: string;
  temperature: number;
  maxTokens: number;
  defaultShell: string;
  terminalFontSize: number;
  terminalCursorStyle: string;
  scrollback: number;
  uiDensity: string;
  fontSizeScaling: number;
  inlineCompletionEnabled: boolean;
  // N-8: cached mirror of useKeyboard's customBindings, kept in appState so
  // the settings UI can react to changes. The source of truth lives in
  // useKeyboard's customBindings map; this field is updated whenever
  // customizations change.
  customShortcuts: Record<string, ShortcutKeys>;
  // N-20: layout state. aiChatPosition controls which side the AI chat panel
  // docks to. activityBarVisible and statusBarVisible toggle those bars.
  aiChatPosition: "left" | "right";
  activityBarVisible: boolean;
  // Plan 47: per-tool-kind agent approval policy. Missing keys default to
  // "always-ask". Mirrored from settings on load and persisted on save.
  toolApprovalConfig: ToolApprovalConfig;
  // Plan 48: custom accent theme definition. Set when accentTheme === "custom".
  customAccent: CustomAccentTheme | null;
  // Design language: "apple" or "claude". Defaults to "apple".
  designLanguage: "apple" | "claude";
  // Multi-provider AI configs (CC Switch-style).
  aiProviderConfigs: AIProviderConfig[];
  activeAIConfigId: string;
  // N-152: 窗口是否已最大化。标题栏据此切换放大 ↔ 还原图标。
  // 由 main.go 的 window:maximised 事件驱动，初始化时从后端查询。
  isWindowMaximised: boolean;
  // G-FEAT-03: which bottom-panel tab to focus after a toolchain run.
  // Empty string means "don't change". TerminalPanel watches this and
  // switches its activeView when it changes to a known value.
  bottomPanelView: "terminal" | "output" | "problems" | "debug" | "tasks" | "workflows" | "";
  // G-FEAT-03: toolchain binary path overrides, mirrored from settings so
  // they round-trip through save and are pushed to the backend on load.
  toolPaths: Record<string, string>;
  // Plan 11 Task 15: personalization config (background images, avatars, fonts, bubble styles).
  personalization: PersonalizationConfig;
  // prompt-5 Task C / BUG-L6: open AI companion window on app startup.
  openAIWindowOnStartup: boolean;
  aiWindowTheme: AIWindowTheme;
  aiSidebarWidth: number;
  aiTerminalWidth: number;
  /** prompt-9 9-A: format via LSP before save. */
  formatOnSave: boolean;
  /** prompt-7 Task F: last known settings.json version for CAS. */
  settingsVersion: number;
}

export const appState = reactive<AppState>({
  sidebarCollapsed: false,
  sidebarWidth: 260,
  activityBarWidth: 48,
  panelTab: "explorer",
  extensionsSubview: "installed",
  terminalVisible: true,
  terminalHeight: 220,
  aiChatVisible: false,
  aiChatWidth: 380,
  currentFilePath: null,
  currentProject: null,
  projectName: null,
  theme: "dark",
  accentTheme: "blue",
  fontSize: 14,
  fontFamily: "JetBrains Mono",
  tabSize: 2,
  wordWrap: true,
  minimap: false,
  lineNumbers: true,
  statusBarVisible: true,
  breadcrumbVisible: true,
  language: "en",
  autoUpdate: true,
  dataFolderPath: "",
  // N-29: sandbox enabled by default (v2 behavior).
  enablePluginSandbox: true,
  branchName: "main",
  errors: 0,
  warnings: 0,
  cursorLine: 1,
  cursorColumn: 1,
  editorJumpSeq: 0,
  encoding: "UTF-8",
  languageMode: "TypeScript",
  aiApiKey: "",
  // G-SEC-07: presence flag + storage method replace holding the plaintext key.
  aiApiKeyConfigured: false,
  aiApiKeyStorageMethod: "none",
  aiBaseUrl: "https://api.openai.com",
  aiModel: "gpt-4o",
  aiSystemPrompt: "",
  // Plan 54: empty = use built-in prompt.
  aiAgentSystemPrompt: "",
  aiConversationTitlePrompt: "",
  aiInlineCompletionPrompt: "",
  cursorBlinking: "blink",
  cursorStyle: "line",
  bracketColorization: true,
  autoSave: false,
  autoSaveDelay: "afterDelay",
  aiProvider: "",
  temperature: 0.7,
  maxTokens: 4096,
  defaultShell: "",
  terminalFontSize: 13,
  terminalCursorStyle: "block",
  scrollback: 10000,
  uiDensity: "comfortable",
  fontSizeScaling: 100,
  inlineCompletionEnabled: true,
  customShortcuts: {},
  aiChatPosition: "right",
  activityBarVisible: true,
  toolApprovalConfig: {},
  customAccent: null,
  designLanguage: "apple",
  aiProviderConfigs: [],
  activeAIConfigId: "",
  // N-152: 初始假设未最大化，initWindowMaximiseListener 会从后端同步真实状态。
  isWindowMaximised: false,
  // G-FEAT-03: no panel-view override until a toolchain run sets one.
  bottomPanelView: "",
  // G-FEAT-03: empty until LoadSettings populates it.
  toolPaths: {},
  // prompt-5 Task C: default off — user opens AI window on demand.
  openAIWindowOnStartup: false,
  aiWindowTheme: "apple-dark",
  aiSidebarWidth: 288,
  aiTerminalWidth: 440,
  formatOnSave: true,
  settingsVersion: 0,
  // Plan 11 Task 15: personalization defaults (empty = no background/avatar override).
  personalization: {
    codeEditorBgOpacity: 0,
    codeEditorBgBlur: 0,
    chatBgOpacity: 0,
    chatBgBlur: 0,
    bubbleStyle: "rounded",
    bubbleOpacity: 1,
    messageSpacing: 12,
  } as PersonalizationConfig,
});

export const isEditorReady = computed(() => appState.currentProject !== null);

export const currentAccentMeta = computed(() => accentThemes[appState.accentTheme]);

/**
 * Apply accent theme to both DOM and Monaco editor. For "custom", the custom
 * accent tokens are applied via inline CSS variables and the Monaco theme is
 * registered dynamically (Plan 48).
 */
export function applyAccentTheme(accent: AccentTheme): void {
  appState.accentTheme = accent;
  if (accent === "custom" && appState.customAccent) {
    applyCustomAccentTokens(appState.customAccent);
    registerCustomTheme(appState.customAccent.color);
  } else {
    clearCustomAccentTokens();
    document.documentElement.setAttribute("data-theme", accent);
  }
  applyMonacoTheme(accent);
}

/**
 * Resolve "system" theme mode to actual "dark" or "light" based on OS preference.
 */
export function resolveSystemMode(): "dark" | "light" {
  if (typeof window !== "undefined" && window.matchMedia) {
    return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  return "dark";
}

/**
 * Apply theme mode (dark/light/system) to DOM and Monaco editor.
 * Sets data-mode attribute on <html> and switches Monaco theme set.
 */
export function applyMode(mode: ThemeMode): void {
  appState.theme = mode;
  const resolved: "dark" | "light" = mode === "system" ? resolveSystemMode() : mode;
  document.documentElement.setAttribute("data-mode", resolved);
  // For custom accent, ensure the Monaco theme is registered before applying.
  if (appState.accentTheme === "custom" && appState.customAccent) {
    registerCustomTheme(appState.customAccent.color);
  }
  applyMonacoThemeForMode(appState.accentTheme, resolved);
}

/**
 * Apply design language ("apple" or "claude") to the document root.
 * The CSS variable overrides live in main.css under
 * [data-design-language="claude"]. The default (apple) needs no attribute
 * since the base tokens are already Apple-style.
 */
export function applyDesignLanguage(lang: "apple" | "claude"): void {
  appState.designLanguage = lang;
  if (lang === "apple") {
    document.documentElement.removeAttribute("data-design-language");
  } else {
    document.documentElement.setAttribute("data-design-language", lang);
  }
}

/**
 * Apply font size scaling (80–150%) to the document root.
 * Sets a `--font-scale` custom property (e.g. 0.8–1.5) that composes with
 * each design language's `--font-base-size` in main.css. Using a CSS variable
 * (rather than a data attribute or inline font-size) lets design-language
 * font-size overrides and user scaling combine multiplicatively.
 */
export function applyFontSizeScaling(scale: number): void {
  appState.fontSizeScaling = scale;
  // Clamp to a safe range to avoid pathological layouts.
  const clamped = Math.max(80, Math.min(150, Math.round(scale)));
  document.documentElement.style.setProperty("--font-scale", (clamped / 100).toFixed(2));
}

/**
 * Apply UI density ("compact" | "comfortable" | "spacious") to the document
 * root. main.css reads `data-density` to override spacing/radius tokens.
 */
export function applyUiDensity(density: string): void {
  appState.uiDensity = density;
  const valid = density === "compact" || density === "comfortable" || density === "spacious"
    ? density
    : "comfortable";
  document.documentElement.setAttribute("data-density", valid);
}

// Apply mode whenever theme changes (e.g. after loadSettings populates appState)
watch(
  () => appState.theme,
  (newMode) => {
    applyMode(newMode as ThemeMode);
  }
);

// Load/clear project-level AI rules (#25) when the active project changes.
// Rules are appended to the AI system prompt by the rules store.
watch(
  () => appState.currentProject,
  (newPath) => {
    if (newPath) {
      void loadRules(newPath);
    } else {
      clearRules();
    }
  }
);

/**
 * Initialize Monaco themes and apply current accent.
 * Call once at app startup.
 */
export function initThemes(): void {
  registerAllThemes();
  if (appState.accentTheme === "custom" && appState.customAccent) {
    applyCustomAccentTokens(appState.customAccent);
    registerCustomTheme(appState.customAccent.color);
  } else {
    document.documentElement.setAttribute("data-theme", appState.accentTheme);
  }
  const resolved: "dark" | "light" = appState.theme === "system" ? resolveSystemMode() : (appState.theme as "dark" | "light");
  document.documentElement.setAttribute("data-mode", resolved);
  applyMonacoThemeForMode(appState.accentTheme, resolved);
  // Apply the persisted design language to the document root.
  applyDesignLanguage(appState.designLanguage);
  // Apply font size scaling and UI density persisted in settings.
  applyFontSizeScaling(appState.fontSizeScaling);
  applyUiDensity(appState.uiDensity);
}

/**
 * Start listening for OS color scheme changes.
 * Only applies when theme mode is "system".
 * Call once at app startup.
 */
let systemModeCleanup: (() => void) | null = null;

export function startSystemModeListener(): void {
  if (typeof window === "undefined" || !window.matchMedia) return;
  const mq = window.matchMedia("(prefers-color-scheme: light)");
  const handler = () => {
    if (appState.theme === "system") {
      const resolved = resolveSystemMode();
      document.documentElement.setAttribute("data-mode", resolved);
      if (appState.accentTheme === "custom" && appState.customAccent) {
        registerCustomTheme(appState.customAccent.color);
      }
      applyMonacoThemeForMode(appState.accentTheme, resolved);
    }
  };
  mq.addEventListener("change", handler);
  systemModeCleanup = () => mq.removeEventListener("change", handler);
}

export function stopSystemModeListener(): void {
  if (systemModeCleanup) {
    systemModeCleanup();
    systemModeCleanup = null;
  }
}

// N-152: 窗口最大化状态监听。bootstrap() 调用一次：先从后端查询初始状态，
// 再注册 window:maximised 事件回调，后续状态变化由 Go 端 OnWindowEvent 推送。
// 使用幂等守卫避免 HMR 或重复调用时注册多个监听器。
let windowMaximiseListenerInitialised = false;
let windowMaximiseCancel: (() => void) | null = null;

export function initWindowMaximiseListener(): void {
  if (windowMaximiseListenerInitialised) return;
  windowMaximiseListenerInitialised = true;
  // 查询初始状态：窗口可能在启动时已被最大化（例如系统记忆位置）。
  void windowService.isMaximised().then((max) => {
    appState.isWindowMaximised = !!max;
  }).catch(() => {
    // 后端尚未就绪时忽略——事件监听器会接管后续状态。
  });
  // 监听 Go 端推送的 window:maximised 事件。
  try {
    windowMaximiseCancel = Events.On("window:maximised", (event: unknown) => {
      const data = (event as { data?: unknown } | null)?.data;
      appState.isWindowMaximised = data === true;
    }) as (() => void) | null;
  } catch {
    // Events 可能在测试/jsdom 环境不可用；忽略错误。
  }
}

/** N-152: 清理监听器（HMR / 测试用）。 */
export function stopWindowMaximiseListener(): void {
  if (windowMaximiseCancel) {
    try { windowMaximiseCancel(); } catch { /* noop */ }
    windowMaximiseCancel = null;
  }
  windowMaximiseListenerInitialised = false;
}

export function toggleSidebar() {
  appState.sidebarCollapsed = !appState.sidebarCollapsed;
}

export function toggleTerminal() {
  appState.terminalVisible = !appState.terminalVisible;
}

export function toggleAiChat() {
  appState.aiChatVisible = !appState.aiChatVisible;
}

/** N-20: move the AI chat panel to the other side. */
export function toggleAiChatPosition() {
  appState.aiChatPosition = appState.aiChatPosition === "right" ? "left" : "right";
}

/** N-20: toggle ActivityBar visibility. */
export function toggleActivityBar() {
  appState.activityBarVisible = !appState.activityBarVisible;
}

/** N-20: toggle StatusBar visibility. */
export function toggleStatusBar() {
  appState.statusBarVisible = !appState.statusBarVisible;
}

// --- Plan 48: Custom Accent Theme ---
// Plan 57 / N-23: pure functions and DOM helpers extracted to
// @/stores/themeEditor. setCustomAccent stays here because it depends on
// appState, registerCustomTheme, applyMonacoTheme, and saveSettings.

/**
 * Set a custom accent theme: update appState, apply to DOM + Monaco, and
 * persist. Pass null to revert to the last built-in accent.
 */
export function setCustomAccent(custom: CustomAccentTheme): void {
  appState.customAccent = custom;
  appState.accentTheme = "custom";
  applyCustomAccentTokens(custom);
  registerCustomTheme(custom.color);
  applyMonacoTheme("custom");
  saveSettings();
}

export function setPanelTab(tab: PanelTab) {
  appState.panelTab = tab;
}

// G-VSC-01: switch the extensions tab between "installed" management and
// "marketplace" browse/install. Used by the SidePanel sub-tab toggle and the
// "Browse Marketplace" command palette entry.
export function setExtensionsSubview(view: ExtensionsSubview) {
  appState.extensionsSubview = view;
}

let saveTimer: ReturnType<typeof setTimeout> | null = null;

export async function loadSettings(): Promise<void> {
  try {
    const settings = await settingsService.loadSettings();
    appState.settingsVersion = settings.version ?? 0;
    appState.language = settings.language;
    appState.theme = settings.theme;
    appState.fontSize = settings.fontSize;
    appState.fontFamily = settings.fontFamily;
    appState.tabSize = settings.tabSize;
    appState.wordWrap = settings.wordWrap;
    appState.lineNumbers = settings.lineNumbers;
    appState.minimap = settings.minimap;
    // G-SEC-07: the backend no longer returns the decrypted key, neither in
    // the legacy field nor in aiProviderConfigs (apiKey is stripped to "").
    // Track presence via the boolean + storage method. appState.aiApiKey
    // stays empty - AI calls use useStoredKey=true instead.
    appState.aiApiKey = "";
    appState.aiApiKeyConfigured = settings.aiApiKeyConfigured ?? false;
    appState.aiApiKeyStorageMethod = settings.aiApiKeyStorageMethod ?? "none";
    appState.aiBaseUrl = settings.aiBaseUrl;
    appState.aiModel = settings.aiModel;
    appState.aiSystemPrompt = settings.aiSystemPrompt;
    // Plan 54: optional prompt overrides.
    appState.aiAgentSystemPrompt = settings.aiAgentSystemPrompt ?? "";
    appState.aiConversationTitlePrompt = settings.aiConversationTitlePrompt ?? "";
    appState.aiInlineCompletionPrompt = settings.aiInlineCompletionPrompt ?? "";
    appState.cursorBlinking = settings.cursorBlinking;
    appState.cursorStyle = settings.cursorStyle;
    appState.bracketColorization = settings.bracketColorization;
    appState.autoSave = settings.autoSave;
    appState.autoSaveDelay = settings.autoSaveDelay;
    appState.aiProvider = settings.aiProvider;
    appState.temperature = settings.temperature;
    appState.maxTokens = settings.maxTokens;
    appState.defaultShell = settings.defaultShell;
    appState.terminalFontSize = settings.terminalFontSize;
    appState.terminalCursorStyle = settings.terminalCursorStyle;
    appState.scrollback = settings.scrollback;
    appState.uiDensity = settings.uiDensity;
    appState.fontSizeScaling = settings.fontSizeScaling;
    appState.inlineCompletionEnabled = settings.inlineCompletionEnabled;
    // prompt-9 9-A: default true when field missing (legacy settings.json).
    appState.formatOnSave = settings.formatOnSave !== false;
    // N-8: hydrate custom shortcut overrides into useKeyboard and mirror in
    // appState for reactivity.
    const custom = settings.customShortcuts ?? {};
    loadCustomShortcuts(custom);
    appState.customShortcuts = { ...custom };
    // N-20: layout state.
    appState.aiChatPosition = settings.aiChatPosition === "left" ? "left" : "right";
    appState.activityBarVisible = settings.activityBarVisible !== false;
    // Plan 47: agent approval policy per tool kind.
    appState.toolApprovalConfig = { ...(settings.toolApprovalConfig ?? {}) };
    // Plan 48: accent theme + custom accent persistence.
    if (settings.accentTheme) {
      appState.accentTheme = settings.accentTheme as AccentTheme;
    }
    appState.customAccent = settings.customAccent ?? null;
    // N-29: plugin sandbox toggle. Default to true for backward compat
    // (older settings.json without this field → sandbox on).
    appState.enablePluginSandbox = settings.enablePluginSandbox !== false;
    // Design language: default to "apple" if not persisted.
    if (settings.designLanguage === "claude" || settings.designLanguage === "apple") {
      appState.designLanguage = settings.designLanguage;
    }
    // Multi-provider AI configs (CC Switch-style). Migrate legacy single-config
    // settings into a default "Default" entry on first load.
    const loadedConfigs = settings.aiProviderConfigs ?? [];
    if (loadedConfigs.length > 0) {
      appState.aiProviderConfigs = loadedConfigs;
      appState.activeAIConfigId = settings.activeAIConfigId ?? loadedConfigs[0].id;
    } else {
      // Migration: pack the legacy single-config fields into one config entry
      // so the user starts with their existing setup and can add more.
      const migrated = migrateLegacyAIConfig(settings);
      appState.aiProviderConfigs = [migrated];
      appState.activeAIConfigId = migrated.id;
    }
    // G-SEC-07: the backend no longer returns plaintext keys in aiProviderConfigs
    // (apiKey is stripped to "", apiKeyConfigured signals presence). The legacy
    // appState.aiApiKey stays empty - AI calls use useStoredKey=true instead.
    appState.aiApiKey = "";
    // G-FEAT-03: mirror tool path overrides into appState (for round-trip
    // save) and push them to the backend ToolchainService.
    appState.toolPaths = { ...(settings.toolPaths ?? {}) };
    // Plan 11 Task 15: mirror personalization config into appState.
    if (settings.personalization) {
      appState.personalization = { ...appState.personalization, ...settings.personalization };
    }
    // prompt-5 Task C: AI companion window on startup (default false).
    appState.openAIWindowOnStartup = settings.openAIWindowOnStartup === true;
    appState.aiWindowTheme = settings.aiWindowTheme ?? "apple-dark";
    appState.aiSidebarWidth = settings.aiSidebarWidth ?? 288;
    appState.aiTerminalWidth = settings.aiTerminalWidth ?? 440;
    try {
      const { syncAIWindowPreferences } = await import("@/stores/aiWindow");
      syncAIWindowPreferences({
        theme: appState.aiWindowTheme,
        sidebarWidth: appState.aiSidebarWidth,
        terminalWidth: appState.aiTerminalWidth,
      });
    } catch {
      // The AI-window UI store may be unavailable in isolated unit tests.
    }
    try {
      const { toolchainService } = await import("@/api/services");
      void toolchainService.setToolPaths(appState.toolPaths);
    } catch {
      // Backend may be unavailable in test/jsdom; ignore.
    }
  } catch (e) {
    console.error("Failed to load settings:", e);
    notifyError(translate("settings.loadFailed", { error: e instanceof Error ? e.message : String(e) }));
  }
}

export function saveSettings(): void {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(async () => {
    const settings: Settings = {
      // prompt-7 Task F: CAS token + version round-trip.
      expectedVersion: appState.settingsVersion > 0 ? appState.settingsVersion : undefined,
      version: appState.settingsVersion,
      language: appState.language,
      theme: appState.theme,
      fontSize: appState.fontSize,
      fontFamily: appState.fontFamily,
      tabSize: appState.tabSize,
      wordWrap: appState.wordWrap,
      lineNumbers: appState.lineNumbers,
      minimap: appState.minimap,
      aiApiKey: appState.aiApiKey,
      // G-SEC-07: signal whether a key is stored so the backend preserves it
      // when aiApiKey is empty (unrelated saves no longer wipe the key).
      aiApiKeyConfigured: appState.aiApiKeyConfigured,
      aiApiKeyStorageMethod: appState.aiApiKeyStorageMethod,
      aiBaseUrl: appState.aiBaseUrl,
      aiModel: appState.aiModel,
      aiSystemPrompt: appState.aiSystemPrompt,
      // Plan 54: optional prompt overrides.
      aiAgentSystemPrompt: appState.aiAgentSystemPrompt,
      aiConversationTitlePrompt: appState.aiConversationTitlePrompt,
      aiInlineCompletionPrompt: appState.aiInlineCompletionPrompt,
      cursorBlinking: appState.cursorBlinking,
      cursorStyle: appState.cursorStyle,
      bracketColorization: appState.bracketColorization,
      autoSave: appState.autoSave,
      autoSaveDelay: appState.autoSaveDelay,
      aiProvider: appState.aiProvider,
      temperature: appState.temperature,
      maxTokens: appState.maxTokens,
      defaultShell: appState.defaultShell,
      terminalFontSize: appState.terminalFontSize,
      terminalCursorStyle: appState.terminalCursorStyle,
      scrollback: appState.scrollback,
      uiDensity: appState.uiDensity,
      fontSizeScaling: appState.fontSizeScaling,
      inlineCompletionEnabled: appState.inlineCompletionEnabled,
      formatOnSave: appState.formatOnSave,
      // N-8: pull the live customizations from useKeyboard so the persisted
      // snapshot reflects the current bindings even if appState.customShortcuts
      // hasn't been re-synced.
      customShortcuts: getCustomShortcuts(),
      // N-20: layout state.
      aiChatPosition: appState.aiChatPosition,
      activityBarVisible: appState.activityBarVisible,
      // Plan 47: agent approval policy per tool kind.
      toolApprovalConfig: { ...appState.toolApprovalConfig },
      // Plan 48: accent theme + custom accent.
      accentTheme: appState.accentTheme,
      customAccent: appState.customAccent,
      // N-29: plugin sandbox toggle.
      enablePluginSandbox: appState.enablePluginSandbox,
      // Design language selection.
      designLanguage: appState.designLanguage,
      // Multi-provider AI configs (CC Switch-style).
      aiProviderConfigs: appState.aiProviderConfigs,
      activeAIConfigId: appState.activeAIConfigId,
      // G-FEAT-03: toolchain binary path overrides.
      toolPaths: { ...appState.toolPaths },
      // Plan 11 Task 15: personalization config.
      personalization: { ...appState.personalization },
      // prompt-5 Task C: AI companion window on startup.
      openAIWindowOnStartup: appState.openAIWindowOnStartup,
      aiWindowTheme: appState.aiWindowTheme,
      aiSidebarWidth: appState.aiSidebarWidth,
      aiTerminalWidth: appState.aiTerminalWidth,
    };
    try {
      await settingsService.saveSettings(settings);
      // Optimistic version bump; peer reload is SSOT.
      appState.settingsVersion =
        appState.settingsVersion > 0 ? appState.settingsVersion + 1 : 1;
      // prompt-6 Task 1: notify peer webviews (AI window / main) to reload.
      try {
        void Events.Emit("settings:changed", {
          origin: getWindowOriginId(),
          version: appState.settingsVersion,
          at: Date.now(),
        });
      } catch {
        // Events may be unavailable in unit tests.
      }
    } catch (e) {
      console.error("Failed to save settings:", e);
      const msg = e instanceof Error ? e.message : String(e);
      if (/settings version conflict|version conflict/i.test(msg)) {
        notifyError(
          translate("settings.versionConflict"),
          translate("settings.versionConflictTitle"),
        );
        void loadSettings();
      } else {
        notifyError(translate("settings.saveFailed", { error: msg }));
      }
    }
  }, 500);
}

// prompt-6 Task 1: listen for peer settings saves and re-hydrate appState.
let settingsSyncListenerRegistered = false;
let settingsSyncCancel: (() => void) | null = null;
/** Prevent re-entrancy when loadSettings triggers another save path. */
let applyingRemoteSettings = false;

export function initSettingsSyncListener(): void {
  if (settingsSyncListenerRegistered) return;
  settingsSyncListenerRegistered = true;
  try {
    settingsSyncCancel = Events.On("settings:changed", (event: unknown) => {
      const payload = unwrapEventData(event);
      const origin = parseSyncOrigin(payload);
      if (origin && origin === getWindowOriginId()) return;
      if (applyingRemoteSettings) return;
      applyingRemoteSettings = true;
      void loadSettings()
        .catch(() => {
          /* best-effort */
        })
        .finally(() => {
          applyingRemoteSettings = false;
        });
    });
  } catch {
    settingsSyncListenerRegistered = false;
  }
}

export function cleanupSettingsSyncListener(): void {
  if (settingsSyncCancel) {
    try {
      settingsSyncCancel();
    } catch {
      /* ignore */
    }
    settingsSyncCancel = null;
  }
  settingsSyncListenerRegistered = false;
}

// Register at module load so both main and AI windows pick up peer changes.
initSettingsSyncListener();

/**
 * Migrate legacy single-config AI settings into a CC Switch-style config entry.
 * Called on first load when aiProviderConfigs is absent from settings.json.
 * Preserves the user's existing provider/key/url/model so AI keeps working
 * after the migration, and they can add more configs from the settings page.
 */
function migrateLegacyAIConfig(settings: Settings): AIProviderConfig {
  const provider = settings.aiProvider || "openai";
  const preset = PROVIDER_PRESETS.find((p) => p.id === provider);
  return {
    id: generateAIConfigId(),
    name: preset?.label ?? "Default",
    provider,
    protocol: preset?.protocol ?? "openai",
    apiKey: settings.aiApiKey ?? "",
    baseUrl: settings.aiBaseUrl ?? "",
    model: settings.aiModel ?? "",
    temperature: settings.temperature ?? 0.7,
    maxTokens: settings.maxTokens ?? 4096,
    systemPrompt: settings.aiSystemPrompt ?? "",
  };
}

/** Generate a unique ID for a new AI provider config. */
export function generateAIConfigId(): string {
  return `cfg_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 8)}`;
}

/**
 * Activate a saved AI provider config by ID. Syncs the config's fields into
 * the legacy single-config fields (aiApiKey/aiBaseUrl/aiModel/aiProvider/
 * temperature/maxTokens/aiSystemPrompt) so existing AI call paths work
 * unchanged, then persists.
 */
export function activateAIConfig(id: string): void {
  const cfg = appState.aiProviderConfigs.find((c) => c.id === id);
  if (!cfg) return;
  appState.activeAIConfigId = id;
  // G-SEC-07: the backend strips apiKey from configs. Keep aiApiKey empty
  // (plaintext never lives in the JS heap). apiKeyConfigured signals whether
  // a key is stored on disk for this config.
  appState.aiApiKey = "";
  appState.aiApiKeyConfigured = !!cfg.apiKeyConfigured;
  appState.aiBaseUrl = cfg.baseUrl;
  appState.aiModel = cfg.model;
  appState.aiProvider = cfg.provider;
  appState.temperature = cfg.temperature ?? 0.7;
  appState.maxTokens = cfg.maxTokens ?? 4096;
  appState.aiSystemPrompt = cfg.systemPrompt ?? "";
  saveSettings();
}

/**
 * Save or update a provider config in the list. If `cfg.id` matches an
 * existing entry, it is replaced; otherwise the entry is appended. If the
 * saved config is the active one, its fields are synced to the legacy
 * single-config fields via activateAIConfig.
 */
export function saveAIConfig(cfg: AIProviderConfig): void {
  const idx = appState.aiProviderConfigs.findIndex((c) => c.id === cfg.id);
  if (idx >= 0) {
    appState.aiProviderConfigs[idx] = cfg;
  } else {
    appState.aiProviderConfigs.push(cfg);
  }
  if (appState.activeAIConfigId === cfg.id) {
    activateAIConfig(cfg.id);
  } else {
    saveSettings();
  }
}

/**
 * Delete a provider config by ID. If the deleted config was active, switch
 * to the first remaining config (or leave unset if none remain). The user
 * cannot delete the last remaining config — they must have at least one.
 * Returns true if deleted, false if not found or if it's the last config.
 */
export function deleteAIConfig(id: string): boolean {
  if (appState.aiProviderConfigs.length <= 1) return false;
  const idx = appState.aiProviderConfigs.findIndex((c) => c.id === id);
  if (idx < 0) return false;
  appState.aiProviderConfigs.splice(idx, 1);
  if (appState.activeAIConfigId === id) {
    const next = appState.aiProviderConfigs[0];
    if (next) activateAIConfig(next.id);
  } else {
    saveSettings();
  }
  return true;
}

/** Create a new blank provider config with sensible defaults. */
export function createNewAIConfig(provider: string = "openai"): AIProviderConfig {
  const preset = PROVIDER_PRESETS.find((p) => p.id === provider);
  return {
    id: generateAIConfigId(),
    name: preset?.label ?? "New Config",
    provider,
    protocol: preset?.protocol ?? "openai",
    apiKey: "",
    baseUrl: preset?.baseUrl ?? "",
    model: preset?.models[0] ?? "",
    temperature: 0.7,
    maxTokens: 4096,
    systemPrompt: "",
  };
}

/** Returns the currently active AI provider config, or undefined. */
export function activeAIConfig(): AIProviderConfig | undefined {
  return appState.aiProviderConfigs.find((c) => c.id === appState.activeAIConfigId);
}

export function openProject(name: string, path: string): void {
  appState.currentProject = path;
  appState.projectName = name;
  // prompt-4 Task 10: 打开项目时同步快照工作区根，激活智能回滚。
  void import("@/stores/snapshot").then(({ setSnapshotWorkspaceRoot }) => {
    setSnapshotWorkspaceRoot(path);
  });
  // 同步加载工作流（若 store 已就绪）
  void import("@/stores/workflows").then(({ loadWorkflows }) => {
    void loadWorkflows(path);
  });
}
