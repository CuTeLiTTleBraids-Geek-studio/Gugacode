/**
 * G-VSC-02: The `vscode` API shim handed to VS Code extensions.
 *
 * This module exposes the `VscodeAPI` interface (a subset of the real
 * `vscode` namespace) and a factory that wires each method to the host's
 * bridging logic. Extensions receive the object returned by
 * `createVscodeAPI` as the argument to their `activate()` export.
 *
 * Bridging summary (see extensionHost.ts for the host side):
 *   - languages.register*Provider  → monaco.languages.register*Provider
 *   - workspace.fs.readFile/writeFile → FileService (permission-gated)
 *   - window.createWebviewPanel    → sandboxed iframe (G-SEC-05)
 *   - commands.registerCommand     → host command registry (disposable)
 *   - commands.executeCommand      → host registry + dangerous-cmd gate
 */

import type { ExtensionPermission } from "@/lib/extensionHost/permissions";

// ---------------------------------------------------------------------------
// Core types (subset of the vscode API surface)
// ---------------------------------------------------------------------------

/**
 * vscode-compatible Thenable (Promise-like). Defined locally so we do not
 * depend on the real `vscode` module or ambient Thenable globals.
 */
export type Thenable<T> = PromiseLike<T>;

/** A handle that releases a resource when disposed. */
export interface Disposable {
  dispose(): void;
}

/** A filesystem URI. Only the `file` scheme is bridged in v1. */
export interface Uri {
  fsPath: string;
  scheme: string;
  authority?: string;
  path?: string;
  query?: string;
  fragment?: string;
}

/**
 * Language filter that selects which documents a provider applies to.
 * Mirrors `vscode.DocumentSelector` (single-filter form). The bridge
 * extracts `language` and passes it to Monaco.
 */
export interface DocumentSelector {
  language: string;
  scheme?: string;
  pattern?: string;
}

/** A completion item returned by a CompletionItemProvider. */
export interface CompletionItem {
  label: string;
  kind?: number;
  detail?: string;
  documentation?: string;
  insertText?: string;
}

/** Result of a completion request. */
export interface CompletionList {
  items: CompletionItem[];
  isIncomplete?: boolean;
}

/** Provides completion items for a document position. */
export interface CompletionItemProvider {
  provideCompletionItems(
    document: TextDocument,
    position: Position,
    token?: unknown,
  ): CompletionList | Thenable<CompletionList>;
}

/** Provides hover info for a document position. */
export interface HoverProvider {
  provideHover(
    document: TextDocument,
    position: Position,
    token?: unknown,
  ): Hover | Thenable<Hover | null> | null;
}

/** Hover tooltip content. */
export interface Hover {
  contents: string[];
}

/** Provides go-to-definition. */
export interface DefinitionProvider {
  provideDefinition(
    document: TextDocument,
    position: Position,
    token?: unknown,
  ): Definition | Thenable<Definition> | null;
}

/** A location in a document. */
export interface Location {
  uri: Uri;
  range: Range;
}

/** A definition result (one or many locations). */
export type Definition = Location | Location[];

/** Provides code actions (quick fixes) for a range. */
export interface CodeActionProvider {
  provideCodeActions(
    document: TextDocument,
    range: Range,
    token?: unknown,
  ): CodeAction[] | Thenable<CodeAction[]> | null;
}

/** A code action (quick fix). */
export interface CodeAction {
  title: string;
  command?: string;
  arguments?: unknown[];
}

/** A text range (line/column pair). */
export interface Range {
  start: Position;
  end: Position;
}

/** A 0-based line/character position. */
export interface Position {
  line: number;
  character: number;
}

/** A text document exposed to providers. */
export interface TextDocument {
  uri: Uri;
  languageId: string;
  getText(): string;
}

/** A text editor (placeholder; activeTextEditor returns undefined in v1). */
export interface TextEditor {
  document: TextDocument;
}

/** Workspace configuration snapshot (read-only in v1). */
export interface WorkspaceConfiguration {
  get<T>(section: string, defaultValue?: T): T;
  has(section: string): boolean;
}

/** Configuration change event (stubbed). */
export interface ConfigurationChangeEvent {
  affectsConfiguration(section: string): boolean;
}

/** A webview backed by a sandboxed iframe. */
export interface Webview {
  /** HTML to render inside the sandboxed iframe. */
  html: string;
  /** The underlying iframe element (host-internal; tests inspect it). */
  readonly _iframe: HTMLIFrameElement;
}

/** A webview panel returned by createWebviewPanel. */
export interface WebviewPanel {
  viewType: string;
  title: string;
  webview: Webview;
  visible: boolean;
  active: boolean;
  dispose(): void;
  onDidDispose(listener: () => void): Disposable;
}

// ---------------------------------------------------------------------------
// API interface
// ---------------------------------------------------------------------------

/**
 * The `vscode` namespace subset exposed to extensions. Each method is
 * bridged by the ExtensionHost. Methods that touch privileged resources
 * check the extension's declared permissions before dispatching.
 */
export interface VscodeAPI {
  languages: {
    registerCompletionItemProvider(
      selector: DocumentSelector,
      provider: CompletionItemProvider,
    ): Disposable;
    registerHoverProvider(
      selector: DocumentSelector,
      provider: HoverProvider,
    ): Disposable;
    registerDefinitionProvider(
      selector: DocumentSelector,
      provider: DefinitionProvider,
    ): Disposable;
    registerCodeActionProvider(
      selector: DocumentSelector,
      provider: CodeActionProvider,
    ): Disposable;
  };
  commands: {
    registerCommand(
      command: string,
      callback: (...args: unknown[]) => unknown,
    ): Disposable;
    executeCommand(command: string, ...args: unknown[]): Thenable<unknown>;
  };
  workspace: {
    fs: {
      readFile(uri: Uri): Thenable<Uint8Array>;
      writeFile(uri: Uri, content: Uint8Array): Thenable<void>;
      exists(uri: Uri): Thenable<boolean>;
      createDirectory(uri: Uri): Thenable<void>;
    };
    getConfiguration(section?: string): WorkspaceConfiguration;
    onDidChangeConfiguration(
      listener: (e: ConfigurationChangeEvent) => void,
    ): Disposable;
  };
  window: {
    createWebviewPanel(
      viewType: string,
      title: string,
      showOptions: unknown,
      options?: unknown,
    ): WebviewPanel;
    showInformationMessage(
      message: string,
      ...items: string[]
    ): Thenable<string | undefined>;
    showWarningMessage(
      message: string,
      ...items: string[]
    ): Thenable<string | undefined>;
    showErrorMessage(
      message: string,
      ...items: string[]
    ): Thenable<string | undefined>;
    activeTextEditor: TextEditor | undefined;
  };
}

// ---------------------------------------------------------------------------
// Host interface (implemented by ExtensionHost; passed to the factory)
// ---------------------------------------------------------------------------

/**
 * The surface the vscode API shim uses to talk back to the ExtensionHost.
 * Keeping this as a structural interface lets the host and tests inject
 * different implementations without a circular import.
 */
export interface VscodeHostBridge {
  /** The extension id this API instance is bound to. */
  readonly extensionId: string;
  /** The extension's declared permissions. */
  readonly permissions: readonly ExtensionPermission[];

  /** Track a disposable so it is disposed when the extension deactivates. */
  trackDisposable(d: Disposable): void;

  /** Register a command handler in the host registry. */
  registerCommand(command: string, cb: (...args: unknown[]) => unknown): Disposable;

  /** Execute a command via the host (applies the dangerous-cmd gate). */
  executeCommand(command: string, ...args: unknown[]): Promise<unknown>;

  /** Bridge a language provider to Monaco and return the disposable. */
  bridgeLanguageProvider(
    kind: "completion" | "hover" | "definition" | "codeAction",
    selector: DocumentSelector,
    provider: unknown,
  ): Disposable;

  /** Bridge a workspace.fs read to the backend FileService. */
  bridgeReadFile(uri: Uri): Promise<Uint8Array>;
  /** Bridge a workspace.fs write to the backend FileService. */
  bridgeWriteFile(uri: Uri, content: Uint8Array): Promise<void>;
  /** Bridge a workspace.fs exists check. */
  bridgeExists(uri: Uri): Promise<boolean>;
  /** Bridge a workspace.fs createDirectory. */
  bridgeCreateDirectory(uri: Uri): Promise<void>;

  /** Create a sandboxed webview panel and track it. */
  createWebviewPanel(
    viewType: string,
    title: string,
    showOptions: unknown,
    options?: unknown,
  ): WebviewPanel;

  /** Show a host notification (no-op when ui.notifications absent). */
  notify(level: "info" | "warn" | "error", message: string): void;
}

/**
 * Create the `vscode` API object handed to an extension's `activate()`.
 * Each method delegates to the host bridge, which enforces permission
 * checks and disposable tracking. The factory is pure: it does not touch
 * module-level state, so each extension gets an isolated API object.
 */
export function createVscodeAPI(host: VscodeHostBridge): VscodeAPI {
  const api: VscodeAPI = {
    languages: {
      registerCompletionItemProvider(selector, provider) {
        return host.bridgeLanguageProvider("completion", selector, provider);
      },
      registerHoverProvider(selector, provider) {
        return host.bridgeLanguageProvider("hover", selector, provider);
      },
      registerDefinitionProvider(selector, provider) {
        return host.bridgeLanguageProvider("definition", selector, provider);
      },
      registerCodeActionProvider(selector, provider) {
        return host.bridgeLanguageProvider("codeAction", selector, provider);
      },
    },
    commands: {
      registerCommand(command, callback) {
        return host.registerCommand(command, callback);
      },
      executeCommand(command, ...args) {
        return host.executeCommand(command, ...args);
      },
    },
    workspace: {
      fs: {
        readFile(uri) {
          return host.bridgeReadFile(uri);
        },
        writeFile(uri, content) {
          return host.bridgeWriteFile(uri, content);
        },
        exists(uri) {
          return host.bridgeExists(uri);
        },
        createDirectory(uri) {
          return host.bridgeCreateDirectory(uri);
        },
      },
      getConfiguration(_section): WorkspaceConfiguration {
        // v1 stub: no live settings bridge. Return an empty snapshot.
        return {
          get<T>(_s: string, defaultValue?: T): T {
            return defaultValue as T;
          },
          has: () => false,
        };
      },
      onDidChangeConfiguration(_listener) {
        // v1 stub: configuration changes are not forwarded yet.
        return { dispose: () => undefined };
      },
    },
    window: {
      createWebviewPanel(viewType, title, showOptions, options) {
        return host.createWebviewPanel(viewType, title, showOptions, options);
      },
      showInformationMessage(message, ...items) {
        host.notify("info", message);
        return Promise.resolve(items[0]);
      },
      showWarningMessage(message, ...items) {
        host.notify("warn", message);
        return Promise.resolve(items[0]);
      },
      showErrorMessage(message, ...items) {
        host.notify("error", message);
        return Promise.resolve(items[0]);
      },
      activeTextEditor: undefined,
    },
  };
  return api;
}
