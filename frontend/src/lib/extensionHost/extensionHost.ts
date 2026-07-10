/**
 * G-VSC-02: Lightweight VS Code Extension Host.
 *
 * The Extension Host loads VS Code extensions, hands each one a subset of
 * the `vscode` API (see vscodeApi.ts), and bridges the extension's
 * registrations to gugacode services:
 *
 *   - vscode.languages.register*Provider → Monaco language providers
 *   - vscode.workspace.fs                → backend FileService (permission-gated,
 *                                           pathsec-validated on the backend)
 *   - vscode.window.createWebviewPanel   → sandboxed iframe (G-SEC-05)
 *   - vscode.commands.registerCommand    → host command registry (disposable)
 *   - vscode.commands.executeCommand     → registry + dangerous-cmd gate (G-SEC-12)
 *
 * Security model (G-SEC-12):
 *   - Each extension is classified Trusted / Reviewed / Restricted from its
 *     declared permissions (permissions.ts).
 *   - Restricted extensions are disabled by default and require explicit
 *     user approval before activation (approveExtension).
 *   - Privileged runtime operations (fs.read, fs.write) check the
 *     extension's declared permission before dispatching.
 *   - Dangerous commands (terminal sendSequence, _workbench.*) require a
 *     confirmation callback; default-deny when no callback is configured.
 */

import {
  classifyExtension,
  hasPermission,
  registerExtensionPermissions,
  unregisterExtensionPermissions,
  type ExtensionPermission,
  type SecurityLevel,
} from "@/lib/extensionHost/permissions";
import {
  createVscodeAPI,
  type Disposable,
  type DocumentSelector,
  type Uri,
  type VscodeAPI,
  type VscodeHostBridge,
  type Webview,
  type WebviewPanel,
} from "@/lib/extensionHost/vscodeApi";

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

/**
 * Descriptor for an extension to activate. Mirrors the fields the host
 * needs from the extension's `package.json` (id, main entry, permissions).
 */
export interface ExtensionDescriptor {
  id: string;
  mainPath: string;
  permissions: ExtensionPermission[];
}

/**
 * The shape of an extension's main module. `activate` receives the vscode
 * API shim; `deactivate` is optional and called on shutdown/disable.
 */
export interface ExtensionModule {
  activate(api: VscodeAPI): void | Promise<void>;
  deactivate?(): void | Promise<void>;
}

/** Factory that loads an extension's main module. Injectable for tests. */
export type ExtensionModuleLoader = (
  extensionId: string,
  mainPath: string,
) => Promise<ExtensionModule>;

/** Confirmation callback for dangerous commands (G-SEC-12). */
export type ConfirmHandler = (
  command: string,
  args: unknown[],
) => Promise<boolean>;

/** A minimal Monaco namespace subset used for language-provider bridging. */
export interface MonacoBridge {
  languages: {
    registerCompletionItemProvider(
      language: string,
      provider: unknown,
    ): Disposable;
    registerHoverProvider(language: string, provider: unknown): Disposable;
    registerDefinitionProvider?(language: string, provider: unknown): Disposable;
    registerCodeActionProvider?(language: string, provider: unknown): Disposable;
  };
}

/** Options for constructing an ExtensionHost. */
export interface ExtensionHostOptions {
  /** Loader for extension main modules. Required for activate(). */
  loadModule?: ExtensionModuleLoader;
  /** Confirmation callback for dangerous commands. Default-deny if unset. */
  confirmHandler?: ConfirmHandler;
  /** Monaco namespace for language-provider bridging. Optional. */
  monaco?: MonacoBridge;
}

// ---------------------------------------------------------------------------
// Internal registry types
// ---------------------------------------------------------------------------

interface RegisteredCommand {
  extensionId: string;
  handler: (...args: unknown[]) => unknown | Promise<unknown>;
}

interface ExtensionEntry {
  descriptor: ExtensionDescriptor;
  module: ExtensionModule;
  disposables: Disposable[];
  securityLevel: SecurityLevel;
}

// ---------------------------------------------------------------------------
// Dangerous command detection (G-SEC-12)
// ---------------------------------------------------------------------------

/**
 * Commands that can send arbitrary input to the terminal or invoke internal
 * workbench machinery. Executing these requires explicit user confirmation
 * because a malicious extension could use them to run arbitrary shell
 * commands without declaring the `shell.execute` permission.
 */
const DANGEROUS_COMMANDS = new Set<string>([
  "workbench.action.terminal.sendSequence",
]);

function isDangerousCommand(command: string): boolean {
  if (DANGEROUS_COMMANDS.has(command)) return true;
  // Internal workbench commands (prefixed with `_workbench.`) are not part
  // of the public API and can trigger privileged host actions.
  if (command.startsWith("_workbench.")) return true;
  return false;
}

// ---------------------------------------------------------------------------
// ExtensionHost
// ---------------------------------------------------------------------------

/**
 * Manages the lifecycle and API surface for VS Code extensions. Each
 * extension receives an isolated `vscode` API object whose methods bridge
 * to gugacode services. All disposables registered by an extension are
 * tracked and disposed on deactivation.
 */
export class ExtensionHost {
  private extensions = new Map<string, ExtensionEntry>();
  private commands = new Map<string, RegisteredCommand>();
  /** Extension ids the user has explicitly approved (Restricted gate). */
  private approved = new Set<string>();
  private options: ExtensionHostOptions;

  constructor(options: ExtensionHostOptions = {}) {
    this.options = options;
  }

  // -------------------------------------------------------------------------
  // Activation / deactivation
  // -------------------------------------------------------------------------

  /**
   * Activate an extension by loading its main module via the configured
   * loader. Throws if no loader is configured or the module has no
   * `activate()` export.
   */
  async activate(desc: ExtensionDescriptor): Promise<void> {
    if (!this.options.loadModule) {
      throw new Error(
        `ExtensionHost.activate: no loadModule configured; cannot load extension "${desc.id}" main at ${desc.mainPath}`,
      );
    }
    const module = await this.options.loadModule(desc.id, desc.mainPath);
    if (!module || typeof module.activate !== "function") {
      throw new Error(
        `Extension "${desc.id}" main module failed to load: no activate() export found at ${desc.mainPath}`,
      );
    }
    await this.activateWithModule(desc, module);
  }

  /**
   * Activate an extension with a pre-loaded module. This is the test entry
   * point (mirrors pluginRegistry's activatePluginWithModule) and the path
   * used when the host has already resolved the module.
   *
   * Steps:
   *   1. No-op if already active.
   *   2. Classify security level; refuse Restricted without approval.
   *   3. Register permissions so runtime gates can query them.
   *   4. Build the vscode API shim and call module.activate(api).
   *   5. On failure, roll back (dispose partials, unregister perms) and rethrow.
   */
  async activateWithModule(
    desc: ExtensionDescriptor,
    module: ExtensionModule,
  ): Promise<void> {
    if (this.isActive(desc.id)) return;

    const level = classifyExtension(desc.permissions);
    if (level === "Restricted" && !this.approved.has(desc.id)) {
      throw new Error(
        `Extension "${desc.id}" is Restricted and disabled by default; explicit user approval required (G-SEC-12)`,
      );
    }

    const entry: ExtensionEntry = {
      descriptor: desc,
      module,
      disposables: [],
      securityLevel: level,
    };
    this.extensions.set(desc.id, entry);
    registerExtensionPermissions(desc.id, desc.permissions);

    const bridge = this.createBridge(desc);
    const api = createVscodeAPI(bridge);
    try {
      await module.activate(api);
    } catch (e) {
      // Roll back: dispose anything the extension registered before failing,
      // drop its permissions, and remove the tentative entry.
      this.disposeEntryDisposables(entry);
      unregisterExtensionPermissions(desc.id);
      this.extensions.delete(desc.id);
      throw e;
    }
  }

  /**
   * Deactivate an extension: call its `deactivate()` export (if any), then
   * dispose every tracked disposable, unregister its permissions, and drop
   * the entry. Idempotent — a no-op for extensions that are not active.
   */
  async deactivate(extensionId: string): Promise<void> {
    const entry = this.extensions.get(extensionId);
    if (!entry) return;

    // Call the extension's own deactivate() first so it can release
    // resources while its registered providers/commands still exist.
    try {
      if (typeof entry.module.deactivate === "function") {
        await entry.module.deactivate();
      }
    } catch {
      // An extension's deactivate throwing must not prevent host cleanup.
    }

    this.disposeEntryDisposables(entry);
    unregisterExtensionPermissions(extensionId);
    this.extensions.delete(extensionId);
  }

  /** Deactivate every active extension. Used on shutdown / project switch. */
  async disposeAll(): Promise<void> {
    const ids = Array.from(this.extensions.keys());
    for (const id of ids) {
      await this.deactivate(id);
    }
  }

  // -------------------------------------------------------------------------
  // State queries
  // -------------------------------------------------------------------------

  /** Whether an extension is currently active. */
  isActive(extensionId: string): boolean {
    return this.extensions.has(extensionId);
  }

  /** The classified security level, or undefined for inactive extensions. */
  getSecurityLevel(extensionId: string): SecurityLevel | undefined {
    return this.extensions.get(extensionId)?.securityLevel;
  }

  /**
   * Explicitly approve a Restricted extension so it may activate. Trusted
   * and Reviewed extensions do not need approval to activate (their
   * privileged operations are still permission-gated at runtime).
   */
  approveExtension(extensionId: string): void {
    this.approved.add(extensionId);
  }

  // -------------------------------------------------------------------------
  // Disposable tracking
  // -------------------------------------------------------------------------

  /**
   * Track a disposable for an extension so it is disposed when the
   * extension deactivates. Returns a disposable (the same handle) the
   * extension can dispose early if it wishes.
   */
  trackDisposable(extensionId: string, disposable: Disposable): Disposable {
    const entry = this.extensions.get(extensionId);
    if (!entry) {
      // Extension is not active (e.g. registering after deactivation).
      // Dispose immediately to avoid a leak.
      try {
        disposable.dispose();
      } catch {
        // ignore
      }
      return disposable;
    }
    entry.disposables.push(disposable);
    return disposable;
  }

  /** Dispose all tracked disposables for an entry in reverse registration order. */
  private disposeEntryDisposables(entry: ExtensionEntry): void {
    for (let i = entry.disposables.length - 1; i >= 0; i--) {
      try {
        entry.disposables[i].dispose();
      } catch {
        // One disposable throwing must not skip the rest.
      }
    }
    entry.disposables.length = 0;
  }

  // -------------------------------------------------------------------------
  // Command registry (host-level)
  // -------------------------------------------------------------------------

  /**
   * Execute a registered command. Dangerous commands (G-SEC-12) require
   * confirmation via the configured confirmHandler; default-deny when no
   * handler is set. Throws if the command is not registered.
   */
  async executeCommand(command: string, ...args: unknown[]): Promise<unknown> {
    // Dangerous commands are gated BEFORE the registry lookup (G-SEC-12):
    // a malicious extension must not learn whether a dangerous command is
    // registered, and the default-deny must fire even for unregistered ids.
    if (isDangerousCommand(command)) {
      const approved = this.options.confirmHandler
        ? await this.options.confirmHandler(command, args)
        : false;
      if (!approved) {
        throw new Error(
          `Command "${command}" was denied: dangerous command requires user confirmation (G-SEC-12)`,
        );
      }
    }
    const cmd = this.commands.get(command);
    if (!cmd) {
      throw new Error(`Command "${command}" is not registered`);
    }
    return cmd.handler(...args);
  }

  /** Register a command handler on behalf of an extension. */
  private registerCommandImpl(
    extensionId: string,
    command: string,
    callback: (...args: unknown[]) => unknown | Promise<unknown>,
  ): Disposable {
    const existing = this.commands.get(command);
    if (existing && existing.extensionId !== extensionId) {
      throw new Error(
        `Command "${command}" is already registered by extension "${existing.extensionId}"`,
      );
    }
    this.commands.set(command, { extensionId, handler: callback });
    const disposable: Disposable = {
      dispose: () => {
        // Only remove if still owned by this extension (avoids removing a
        // re-registered command owned by another extension).
        const cur = this.commands.get(command);
        if (cur && cur.extensionId === extensionId) {
          this.commands.delete(command);
        }
      },
    };
    this.trackDisposable(extensionId, disposable);
    return disposable;
  }

  // -------------------------------------------------------------------------
  // Monaco language-provider bridging
  // -------------------------------------------------------------------------

  /**
   * Bridge a vscode language provider to Monaco. The vscode DocumentSelector
   * is converted to a Monaco language id (the `language` field). The Monaco
   * disposable is tracked for cleanup on deactivation.
   */
  private bridgeLanguageProviderImpl(
    extensionId: string,
    kind: "completion" | "hover" | "definition" | "codeAction",
    selector: DocumentSelector,
    provider: unknown,
  ): Disposable {
    const monaco = this.options.monaco;
    const language = selector.language;
    if (!monaco) {
      // No Monaco available (e.g. test without monaco option). Return a
      // no-op disposable so the extension can still call dispose().
      const noop: Disposable = { dispose: () => undefined };
      this.trackDisposable(extensionId, noop);
      return noop;
    }
    let monacoDisposable: Disposable;
    switch (kind) {
      case "completion":
        monacoDisposable = monaco.languages.registerCompletionItemProvider(
          language,
          provider,
        );
        break;
      case "hover":
        monacoDisposable = monaco.languages.registerHoverProvider(
          language,
          provider,
        );
        break;
      case "definition":
        if (!monaco.languages.registerDefinitionProvider) {
          monacoDisposable = { dispose: () => undefined };
        } else {
          monacoDisposable = monaco.languages.registerDefinitionProvider(
            language,
            provider,
          );
        }
        break;
      case "codeAction":
        if (!monaco.languages.registerCodeActionProvider) {
          monacoDisposable = { dispose: () => undefined };
        } else {
          monacoDisposable = monaco.languages.registerCodeActionProvider(
            language,
            provider,
          );
        }
        break;
    }
    this.trackDisposable(extensionId, monacoDisposable);
    return monacoDisposable;
  }

  // -------------------------------------------------------------------------
  // workspace.fs bridging (permission-gated, backend pathsec-validated)
  // -------------------------------------------------------------------------

  /**
   * Resolve a vscode Uri.fsPath against the workspace root. Absolute paths
   * (POSIX leading `/` or Windows drive letter) are used as-is; relative
   * paths are joined under the current project root. The backend
   * FileService re-validates the resolved path against the workspace root
   * via pathsec (ValidatePathWithinRoot) before touching disk, so even a
   * malicious fsPath cannot escape the workspace.
   */
  private resolveWorkspacePath(fsPath: string, root: string): string {
    if (fsPath.startsWith("/") || /^[A-Za-z]:[\\/]/.test(fsPath)) {
      return fsPath;
    }
    return root ? `${root}/${fsPath}` : fsPath;
  }

  /** Bridge vscode.workspace.fs.readFile → FileService.readFile. */
  private async bridgeReadFileImpl(
    extensionId: string,
    uri: Uri,
  ): Promise<Uint8Array> {
    if (!hasPermission(extensionId, "fs.read")) {
      throw new Error(
        `Extension "${extensionId}" cannot read files: requires permission "fs.read" not declared`,
      );
    }
    const { fileService } = await import("@/api/services");
    const { appState } = await import("@/stores/app");
    const fullPath = this.resolveWorkspacePath(
      uri.fsPath,
      appState.currentProject ?? "",
    );
    const content = await fileService.readFile(fullPath);
    // Re-construct via the current realm's Uint8Array so `instanceof Uint8Array`
    // holds across jsdom/Node realm boundaries (TextEncoder may produce a
    // Uint8Array backed by a different realm's constructor).
    const encoded = new TextEncoder().encode(content);
    return new Uint8Array(encoded);
  }

  /** Bridge vscode.workspace.fs.writeFile → FileService.writeFile. */
  private async bridgeWriteFileImpl(
    extensionId: string,
    uri: Uri,
    content: Uint8Array,
  ): Promise<void> {
    if (!hasPermission(extensionId, "fs.write")) {
      throw new Error(
        `Extension "${extensionId}" cannot write files: requires permission "fs.write" not declared`,
      );
    }
    const { fileService } = await import("@/api/services");
    const { appState } = await import("@/stores/app");
    const fullPath = this.resolveWorkspacePath(
      uri.fsPath,
      appState.currentProject ?? "",
    );
    const text = new TextDecoder().decode(content);
    await fileService.writeFile(fullPath, text);
  }

  /** Bridge vscode.workspace.fs.exists → FileService (via readFile probe). */
  private async bridgeExistsImpl(
    extensionId: string,
    uri: Uri,
  ): Promise<boolean> {
    if (!hasPermission(extensionId, "fs.read")) {
      throw new Error(
        `Extension "${extensionId}" cannot stat files: requires permission "fs.read" not declared`,
      );
    }
    try {
      await this.bridgeReadFileImpl(extensionId, uri);
      return true;
    } catch {
      return false;
    }
  }

  /** Bridge vscode.workspace.fs.createDirectory → FileService.createDirectory. */
  private async bridgeCreateDirectoryImpl(
    extensionId: string,
    uri: Uri,
  ): Promise<void> {
    if (!hasPermission(extensionId, "fs.write")) {
      throw new Error(
        `Extension "${extensionId}" cannot create directories: requires permission "fs.write" not declared`,
      );
    }
    const { fileService } = await import("@/api/services");
    const { appState } = await import("@/stores/app");
    const fullPath = this.resolveWorkspacePath(
      uri.fsPath,
      appState.currentProject ?? "",
    );
    await fileService.createDirectory(fullPath);
  }

  // -------------------------------------------------------------------------
  // Webview panel bridging (G-SEC-05 sandboxed iframe)
  // -------------------------------------------------------------------------

  /**
   * Create a webview panel backed by a sandboxed iframe. The iframe uses
   * `sandbox="allow-scripts"` (no allow-same-origin) so the extension's
   * HTML cannot reach the parent DOM, localStorage, or Wails bindings —
   * only the postMessage RPC bridge (mirrors PluginViewIframe.vue).
   */
  private createWebviewPanelImpl(
    extensionId: string,
    viewType: string,
    title: string,
    _showOptions: unknown,
    _options: unknown,
  ): WebviewPanel {
    const iframe = document.createElement("iframe");
    iframe.setAttribute("sandbox", "allow-scripts");
    // The host mounts the iframe visibly when it decides where to show the
    // panel; until then it lives in the DOM (hidden) so tests and the host
    // can interact with it.
    iframe.style.display = "none";
    iframe.title = title;
    document.body.appendChild(iframe);

    let html = "";
    let disposed = false;
    const disposeListeners: Array<() => void> = [];

    const webview: Webview = {
      get html() {
        return html;
      },
      set html(value: string) {
        html = value;
        // srcdoc renders the HTML inside the sandboxed iframe.
        iframe.srcdoc = value;
      },
      get _iframe() {
        return iframe;
      },
    };

    const panel: WebviewPanel = {
      viewType,
      title,
      webview,
      visible: true,
      active: true,
      dispose() {
        if (disposed) return;
        disposed = true;
        iframe.remove();
        for (const l of disposeListeners) {
          try {
            l();
          } catch {
            // ignore listener errors
          }
        }
        disposeListeners.length = 0;
      },
      onDidDispose(listener: () => void): Disposable {
        disposeListeners.push(listener);
        return {
          dispose: () => {
            const i = disposeListeners.indexOf(listener);
            if (i >= 0) disposeListeners.splice(i, 1);
          },
        };
      },
    };

    // Track the panel so it is disposed (iframe removed) on deactivation.
    this.trackDisposable(extensionId, { dispose: () => panel.dispose() });
    return panel;
  }

  // -------------------------------------------------------------------------
  // Notifications
  // -------------------------------------------------------------------------

  /**
   * Surface an extension notification. v1 best-effort: log to the console.
   * A richer surface (lib/notifications) is gated behind the
   * `ui.notifications` permission in a future iteration; for now we log so
   * the API is usable without pulling the notification module's deps.
   */
  private notifyImpl(
    extensionId: string,
    level: "info" | "warn" | "error",
    message: string,
  ): void {
    const fn =
      level === "error" ? console.error : level === "warn" ? console.warn : console.log;
    fn(`[ext:${extensionId}] ${message}`);
  }

  // -------------------------------------------------------------------------
  // Bridge factory
  // -------------------------------------------------------------------------

  /**
   * Build the VscodeHostBridge for an extension. Each method closes over
   * the extension id so the vscode API shim can call back into the host
   * without the extension needing to pass its own id.
   */
  private createBridge(desc: ExtensionDescriptor): VscodeHostBridge {
    const extensionId = desc.id;
    const permissions = desc.permissions;
    return {
      extensionId,
      permissions,
      trackDisposable: (d: Disposable) => {
        this.trackDisposable(extensionId, d);
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- VS Code API contract: registerCommand callback signature is (...args: any[]) => any
      registerCommand: (command: string, cb: (...args: any[]) => any) =>
        this.registerCommandImpl(
          extensionId,
          command,
          cb as (...args: unknown[]) => unknown,
        ),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- VS Code API contract: executeCommand accepts arbitrary arguments
      executeCommand: (command: string, ...args: any[]) =>
        this.executeCommand(command, ...args),
      bridgeLanguageProvider: (
        kind: "completion" | "hover" | "definition" | "codeAction",
        selector: DocumentSelector,
        provider: unknown,
      ) => this.bridgeLanguageProviderImpl(extensionId, kind, selector, provider),
      bridgeReadFile: (uri: Uri) => this.bridgeReadFileImpl(extensionId, uri),
      bridgeWriteFile: (uri: Uri, content: Uint8Array) =>
        this.bridgeWriteFileImpl(extensionId, uri, content),
      bridgeExists: (uri: Uri) => this.bridgeExistsImpl(extensionId, uri),
      bridgeCreateDirectory: (uri: Uri) =>
        this.bridgeCreateDirectoryImpl(extensionId, uri),
      createWebviewPanel: (
        viewType: string,
        title: string,
        showOptions: unknown,
        options?: unknown,
      ) =>
        this.createWebviewPanelImpl(
          extensionId,
          viewType,
          title,
          showOptions,
          options,
        ),
      notify: (level: "info" | "warn" | "error", message: string) =>
        this.notifyImpl(extensionId, level, message),
    };
  }
}
