/**
 * Extension security store (G-VSC-03 / G-SEC-12) — frontend orchestration
 * for VS Code extension security gates.
 *
 * This store is the frontend counterpart to the Go ExtensionSecurityService.
 * It:
 *   - Manages extension security states (classification, verification,
 *     enabled, blacklist, pending-review).
 *   - Calls the backend to classify, verify, enable/disable extensions.
 *   - Shows the permission dialog (ExtensionPermissionDialog) when a user
 *     tries to enable a Reviewed or Restricted extension.
 *   - Blocks access to `appState.aiApiKey` from extension contexts
 *     (G-SEC-12 requirement 5: resource isolation).
 *
 * The store is intentionally separate from the native plugin store
 * (stores/plugins.ts) because VS Code extensions use a richer permission
 * model and a separate install path (MarketplaceService).
 */

import { reactive, computed, ref } from "vue";
import { errorMessage } from "@/lib/errors";
import { appState } from "@/stores/app";

// ---------------------------------------------------------------------------
// Types — mirror the Go ExtensionSecurityInfo / ExtensionSecurityLevel /
// ExtensionPermission structs (services/extension_security_service.go).
// ---------------------------------------------------------------------------

export type ExtensionSecurityLevel = "trusted" | "reviewed" | "restricted";

export type ExtensionPermission =
  | "fs.read"
  | "fs.write"
  | "shell.execute"
  | "network"
  | "clipboard"
  | "ui.notifications"
  | "ui.webview";

export interface ExtensionSecurityInfo {
  extensionId: string;
  level: ExtensionSecurityLevel;
  permissions: ExtensionPermission[];
  sha256: string;
  verified: boolean;
  enabled: boolean;
  blacklisted: boolean;
  pendingReview: boolean;
}

// ---------------------------------------------------------------------------
// Permission metadata — human-readable descriptions and risk tiers used by
// the permission dialog.
// ---------------------------------------------------------------------------

const PERMISSION_DESCRIPTIONS: Record<ExtensionPermission, string> = {
  "fs.read":
    "Read files in your workspace.",
  "fs.write":
    "Create, modify, or delete files in your workspace.",
  "shell.execute":
    "Execute shell commands on your machine.",
  network:
    "Make outbound network requests to any destination.",
  clipboard:
    "Read from and write to the system clipboard.",
  "ui.notifications":
    "Show notification messages in the IDE.",
  "ui.webview":
    "Render web content (HTML/CSS/JS) in a sandboxed panel.",
};

const PERMISSION_RISK: Record<ExtensionPermission, "low" | "medium" | "high"> = {
  "fs.read": "low",
  "ui.notifications": "low",
  "ui.webview": "low",
  clipboard: "medium",
  "fs.write": "medium",
  "shell.execute": "high",
  network: "high",
};

/**
 * Human-readable description of a permission, shown in the approval dialog.
 */
export function permissionDescription(perm: ExtensionPermission): string {
  return PERMISSION_DESCRIPTIONS[perm] ?? "Unknown permission.";
}

/**
 * Risk tier for a permission: "low" (read-only), "medium" (write/clipboard),
 * "high" (shell/network). Used by the dialog to sort and color-code the list.
 */
export function permissionRisk(perm: ExtensionPermission): "low" | "medium" | "high" {
  return PERMISSION_RISK[perm] ?? "medium";
}

// ---------------------------------------------------------------------------
// Store state
// ---------------------------------------------------------------------------

interface ExtensionSecurityStoreState {
  /** All known extension security infos, keyed by extensionId. */
  infos: Record<string, ExtensionSecurityInfo>;
  loading: boolean;
  error: string | null;
}

export const extensionSecurityStore = reactive<ExtensionSecurityStoreState>({
  infos: {},
  loading: false,
  error: null,
});

export const extensionSecurityInfos = computed(() =>
  Object.values(extensionSecurityStore.infos),
);
export const isLoadingExtensionSecurity = computed(
  () => extensionSecurityStore.loading,
);
export const extensionSecurityError = computed(
  () => extensionSecurityStore.error,
);

// ---------------------------------------------------------------------------
// Permission dialog state
//
// The dialog is shown when the user attempts to enable a Reviewed or
// Restricted extension. The store exposes a reactive `pendingApproval`
// ref that the host component (App.vue or PluginsView) binds to the
// ExtensionPermissionDialog's `visible` + `info` props.
// ---------------------------------------------------------------------------

export const pendingApproval = ref<ExtensionSecurityInfo | null>(null);

/**
 * Show the permission dialog for an extension. Called internally by
 * `requestEnableExtension` when the backend reports the extension is
 * Reviewed/Restricted or pending review. The host component renders
 * <ExtensionPermissionDialog :visible="!!pendingApproval" :info="pendingApproval" />
 * and listens for @approve / @close.
 */
export function showPermissionDialog(info: ExtensionSecurityInfo): void {
  pendingApproval.value = info;
}

/**
 * Dismiss the permission dialog without enabling.
 */
export function dismissPermissionDialog(): void {
  pendingApproval.value = null;
}

// ---------------------------------------------------------------------------
// Backend integration
//
// The actual Wails bindings for ExtensionSecurityService are generated at
// build time. To avoid a hard dependency on bindings that may not yet exist
// in the repo (the service is registered in main.go but the bindings are
// regenerated by the Wails Vite plugin), we use a thin RPC shim that calls
// the generated bindings lazily. Tests inject a mock backend.
// ---------------------------------------------------------------------------

/**
 * Backend adapter interface. The default implementation calls the Wails
 * bindings; tests inject a mock.
 */
export interface ExtensionSecurityBackend {
  classifyExtension(permissions: ExtensionPermission[]): Promise<ExtensionSecurityLevel>;
  registerInstall(
    extensionId: string,
    permissions: ExtensionPermission[],
    vsixPath: string,
    expectedSHA256: string,
  ): Promise<ExtensionSecurityInfo>;
  getSecurityInfo(extensionId: string): Promise<ExtensionSecurityInfo>;
  setExtensionEnabled(
    extensionId: string,
    enabled: boolean,
    explicitApproval?: boolean,
  ): Promise<void>;
  listSecurityInfo(): Promise<ExtensionSecurityInfo[]>;
  isBlacklisted(publisher: string, name: string): Promise<boolean>;
  addToBlacklist(publisher: string, name: string): Promise<void>;
  canInstall(publisher: string, name: string): Promise<void>;
}

// Lazy-loaded default backend that calls the Wails bindings.
let backend: ExtensionSecurityBackend | null = null;

/**
 * Inject the backend adapter. Tests call this with a mock; the app calls
 * it once on startup with the default Wails-backed adapter.
 */
export function setExtensionSecurityBackend(b: ExtensionSecurityBackend | null): void {
  backend = b;
}

/**
 * Cache for the lazily-loaded bindings module. Typed as a minimal shape
 * so the default backend can call the methods without a hard type
 * dependency on the generated file.
 */
interface ExtensionSecurityBindingsShape {
  ClassifyExtension(permissions: ExtensionPermission[]): Promise<string>;
  RegisterInstall(
    extensionId: string,
    permissions: ExtensionPermission[],
    vsixPath: string,
    expectedSHA256: string,
  ): Promise<ExtensionSecurityInfo>;
  GetSecurityInfo(extensionId: string): Promise<ExtensionSecurityInfo>;
  SetExtensionEnabled(
    extensionId: string,
    enabled: boolean,
    explicitApproval: boolean,
  ): Promise<void>;
  ListSecurityInfo(): Promise<ExtensionSecurityInfo[]>;
  IsBlacklisted(publisher: string, name: string): Promise<boolean>;
  AddToBlacklist(publisher: string, name: string): Promise<void>;
  CanInstall(publisher: string, name: string): Promise<void>;
}

let bindingsCache: ExtensionSecurityBindingsShape | null = null;

async function loadBindings(): Promise<ExtensionSecurityBindingsShape> {
  if (bindingsCache) return bindingsCache;
  // 使用字面量路径（无 @vite-ignore），让 vite 将 bindings 打包为 chunk。
  const mod = await import("../../bindings/gugacode/services/extensionsecurityservice.js");
  // bindings 文件使用命名导出，直接将 mod 作为 ExtensionSecurityBindingsShape 使用。
  bindingsCache = mod as unknown as ExtensionSecurityBindingsShape;
  return bindingsCache;
}

/**
 * Default backend that lazily imports the Wails-generated bindings. If the
 * bindings are not available (e.g. in unit tests), calls throw — tests
 * should inject a mock via setExtensionSecurityBackend before exercising
 * the store.
 */
function getDefaultBackend(): ExtensionSecurityBackend {
  return {
    async classifyExtension(permissions) {
      const b = await loadBindings();
      const level = (await b.ClassifyExtension(permissions)) as string;
      return level as ExtensionSecurityLevel;
    },
    async registerInstall(extensionId, permissions, vsixPath, expectedSHA256) {
      const b = await loadBindings();
      return (await b.RegisterInstall(
        extensionId,
        permissions,
        vsixPath,
        expectedSHA256,
      )) as ExtensionSecurityInfo;
    },
    async getSecurityInfo(extensionId) {
      const b = await loadBindings();
      return (await b.GetSecurityInfo(extensionId)) as ExtensionSecurityInfo;
    },
    async setExtensionEnabled(extensionId, enabled, explicitApproval) {
      const b = await loadBindings();
      await b.SetExtensionEnabled(extensionId, enabled, explicitApproval ?? false);
    },
    async listSecurityInfo() {
      const b = await loadBindings();
      return (await b.ListSecurityInfo()) as ExtensionSecurityInfo[];
    },
    async isBlacklisted(publisher, name) {
      const b = await loadBindings();
      return (await b.IsBlacklisted(publisher, name)) as boolean;
    },
    async addToBlacklist(publisher, name) {
      const b = await loadBindings();
      await b.AddToBlacklist(publisher, name);
    },
    async canInstall(publisher, name) {
      const b = await loadBindings();
      await b.CanInstall(publisher, name);
    },
  };
}

function getBackend(): ExtensionSecurityBackend {
  if (backend) return backend;
  backend = getDefaultBackend();
  return backend;
}

// ---------------------------------------------------------------------------
// Store actions
// ---------------------------------------------------------------------------

/**
 * Load all extension security infos from the backend and sync the local
 * store. Safe to call repeatedly.
 */
export async function loadExtensionSecurityInfos(): Promise<void> {
  extensionSecurityStore.loading = true;
  extensionSecurityStore.error = null;
  try {
    const list = await getBackend().listSecurityInfo();
    extensionSecurityStore.infos = {};
    for (const info of list) {
      extensionSecurityStore.infos[info.extensionId] = info;
    }
  } catch (e: unknown) {
    extensionSecurityStore.error = errorMessage(e);
  } finally {
    extensionSecurityStore.loading = false;
  }
}

/**
 * Get the security info for a single extension. Returns undefined if the
 * extension has no recorded security state.
 */
export function getExtensionSecurityInfo(
  extensionId: string,
): ExtensionSecurityInfo | undefined {
  return extensionSecurityStore.infos[extensionId];
}

/**
 * Request to enable an extension. This is the main entry point for the
 * G-VSC-03 / G-SEC-12 permission gate:
 *
 * 1. Fetch the extension's security info from the backend.
 * 2. If the extension is blacklisted → reject with an error.
 * 3. If the extension is unverified → reject with an error.
 * 4. If the extension is Restricted OR pending review → show the
 *    permission dialog. The actual enable happens in
 *    `confirmEnableExtension` after the user approves.
 * 5. Otherwise (Trusted/Reviewed, already reviewed) → enable directly.
 *
 * Returns true if the extension was enabled (or the dialog was shown for
 * user approval), false if the request was rejected.
 */
export async function requestEnableExtension(
  extensionId: string,
): Promise<boolean> {
  extensionSecurityStore.error = null;
  let info: ExtensionSecurityInfo;
  try {
    info = await getBackend().getSecurityInfo(extensionId);
  } catch (e: unknown) {
    extensionSecurityStore.error = errorMessage(e);
    return false;
  }

  // Blacklist gate — never show the dialog for blacklisted extensions.
  if (info.blacklisted) {
    extensionSecurityStore.error =
      "Extension is on the known-malicious blacklist and cannot be enabled.";
    return false;
  }

  // Verification gate — unverified extensions cannot be enabled, but we
  // still show the dialog so the user sees *why* (the Enable button is
  // disabled when !verified).
  if (
    info.level === "restricted" ||
    info.pendingReview ||
    !info.verified
  ) {
    // Show the permission dialog. The host component renders
    // ExtensionPermissionDialog bound to pendingApproval.
    extensionSecurityStore.infos[extensionId] = info;
    showPermissionDialog(info);
    return false; // Not yet enabled — waiting for user approval.
  }

  // Trusted/Reviewed, verified, already reviewed → enable directly.
  return confirmEnableExtension(extensionId, false);
}

/**
 * Confirm enabling an extension after the user approves the permission
 * dialog (or directly for Trusted extensions). `explicitApproval` must be
 * true for Restricted extensions (the backend enforces this).
 *
 * Returns true on success, false on failure (error is stored in
 * extensionSecurityStore.error).
 */
export async function confirmEnableExtension(
  extensionId: string,
  explicitApproval: boolean,
): Promise<boolean> {
  extensionSecurityStore.error = null;
  try {
    await getBackend().setExtensionEnabled(
      extensionId,
      true,
      explicitApproval,
    );
    // Refresh the info so the UI reflects the new enabled state.
    const info = await getBackend().getSecurityInfo(extensionId);
    extensionSecurityStore.infos[extensionId] = info;
    return true;
  } catch (e: unknown) {
    extensionSecurityStore.error = errorMessage(e);
    return false;
  }
}

/**
 * Disable an extension. Always succeeds for non-blacklisted extensions.
 */
export async function disableExtension(extensionId: string): Promise<boolean> {
  extensionSecurityStore.error = null;
  try {
    await getBackend().setExtensionEnabled(extensionId, false);
    const info = await getBackend().getSecurityInfo(extensionId);
    extensionSecurityStore.infos[extensionId] = info;
    return true;
  } catch (e: unknown) {
    extensionSecurityStore.error = errorMessage(e);
    return false;
  }
}

/**
 * Handle the dialog's "approve" event: enable the extension with explicit
 * approval. Dismisses the dialog afterwards.
 */
export async function handleApprove(extensionId: string): Promise<void> {
  const info = pendingApproval.value;
  const explicitApproval = info?.level === "restricted";
  dismissPermissionDialog();
  await confirmEnableExtension(extensionId, explicitApproval);
}

/**
 * Check if an extension can be installed (blacklist pre-check). Returns
 * true if installation may proceed, false if the extension is blacklisted.
 */
export async function checkCanInstall(
  publisher: string,
  name: string,
): Promise<boolean> {
  try {
    await getBackend().canInstall(publisher, name);
    return true;
  } catch {
    return false;
  }
}

// ---------------------------------------------------------------------------
// G-SEC-12 requirement 5: Resource isolation — block access to
// appState.aiApiKey from extension contexts.
//
// Extensions run in an isolated host (Web Worker sandbox or sandboxed
// iframe) and must never access the main webview's appState, which holds
// the AI API key. This guard is a defense-in-depth measure: even if an
// extension somehow obtains a reference to appState, this function (used
// by the API surface compatibility layer) refuses to return the key.
// ---------------------------------------------------------------------------

/**
 * Marker symbol attached to objects that are exposed to extension
 * contexts. The API surface checks for this before returning any value
 * that could leak appState.
 */
export const EXTENSION_CONTEXT_MARKER = Symbol("gugacode.extension.context");

/**
 * Returns true if the current execution context is an extension sandbox
 * (Worker or sandboxed iframe). The API surface uses this to enforce the
 * aiApiKey access block.
 *
 * Detection: extension contexts are created by the extensionHost, which
 * sets a global flag. In the main webview this is always false.
 */
export function isExtensionContext(): boolean {
  // Worker context: self is a DedicatedWorkerGlobalScope, not Window.
  if (typeof self !== "undefined" && typeof window === "undefined") {
    return true;
  }
  // Sandboxed iframe: window.parent !== window (cross-origin), and the
  // iframe cannot access window.parent's properties.
  if (typeof window !== "undefined" && window.parent !== window) {
    return true;
  }
  // Explicit marker set by the extension host bootstrap.
  if (typeof globalThis !== "undefined") {
    const g = globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean };
    if (g.__GUGACODE_EXTENSION_CONTEXT__ === true) return true;
  }
  return false;
}

/**
 * Get the AI API key. Returns an empty string when called from an
 * extension context (G-SEC-12 requirement 5). This is the single choke
 * point the API surface uses — extensions that try to read the key via
 * any compatibility-layer API hit this guard.
 *
 * Defense-in-depth: even if appState.aiApiKey is non-empty in the main
 * webview, this function returns "" for extension contexts.
 */
export function getAiApiKeyForContext(): string {
  if (isExtensionContext()) {
    // G-SEC-12 req. 5: extensions cannot access appState.aiApiKey.
    return "";
  }
  return appState.aiApiKey ?? "";
}

/**
 * Assert that the current context is NOT an extension context. Used by
 * API surface methods that must never be callable from extensions (e.g.
 * reading raw secrets). Throws if called from an extension context.
 */
export function assertNotExtensionContext(operation: string): void {
  if (isExtensionContext()) {
    throw new Error(
      `Operation "${operation}" is blocked in extension contexts (G-SEC-12 resource isolation).`,
    );
  }
}

/**
 * Reset the store state. Used in tests.
 */
export function resetExtensionSecurityStore(): void {
  extensionSecurityStore.infos = {};
  extensionSecurityStore.loading = false;
  extensionSecurityStore.error = null;
  pendingApproval.value = null;
  backend = null;
}
