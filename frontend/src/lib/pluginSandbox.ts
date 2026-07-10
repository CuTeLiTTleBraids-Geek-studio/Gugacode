/**
 * Plugin Web Worker sandbox (N-26).
 *
 * Provides isolation for plugin code by running it in a Web Worker
 * instead of the main thread. The Worker has no access to the DOM,
 * window, localStorage, or Monaco — it can only communicate with the
 * main thread via postMessage. The main thread validates permissions
 * before executing any privileged operation on behalf of the Worker.
 *
 * Message protocol:
 *   Main → Worker:  { type: 'init', pluginUrl, manifest }
 *   Main → Worker:  { type: 'rpc-call', id, method, args }
 *   Worker → Main:  { type: 'rpc-result', id, result?, error? }
 *   Worker → Main:  { type: 'rpc-request', id, method, args }
 *   Main → Worker:  { type: 'rpc-response', id, result?, error? }
 *   Worker → Main:  { type: 'activated' } | { type: 'activation-error', error }
 *
 * The Worker initiates RPC requests when the plugin calls a nknk.*
 * API method (e.g. nknk.workspace.readFile). The host validates
 * permissions and dispatches to the real service, then sends the
 * result back as an RPC response.
 */
import type { PluginManifest, PluginPermission } from "@/types";

// ---------------------------------------------------------------------------
// Message protocol types
// ---------------------------------------------------------------------------

/** Messages sent from the host (main thread) to the Worker. */
export type HostToWorkerMessage =
  | { type: "init"; pluginUrl: string; manifest: PluginManifest }
  | { type: "rpc-response"; id: number; result?: unknown; error?: string }
  | { type: "rpc-call"; id: number; method: string; args: unknown[] }
  | { type: "terminate" };

/** Messages sent from the Worker to the host (main thread). */
export type WorkerToHostMessage =
  | { type: "activated" }
  | { type: "activation-error"; error: string }
  | { type: "rpc-request"; id: number; method: string; args: unknown[] }
  | { type: "rpc-result"; id: number; result?: unknown; error?: string }
  | { type: "log"; level: "info" | "warn" | "error"; message: string };

// ---------------------------------------------------------------------------
// RPC method types
// ---------------------------------------------------------------------------

/**
 * RPC methods that the Worker (plugin) can request the host to execute.
 * Each method maps to a nknk.* API call. The host validates permissions
 * before dispatching.
 *
 * The method names use dot notation matching the API surface:
 *   "workspace.readFile" → nknk.workspace.readFile
 *   "workspace.writeFile" → nknk.workspace.writeFile
 *   "commands.register" → nknk.commands.register
 *   "commands.execute" → nknk.commands.execute
 *   "views.register" → nknk.views.register
 */
export type RpcMethod =
  | "workspace.readFile"
  | "workspace.writeFile"
  | "commands.register"
  | "commands.execute"
  | "views.register"
  | "getPermissions";

/** Permission required for each RPC method. Undefined = always allowed. */
const METHOD_PERMISSIONS: Partial<Record<RpcMethod, PluginPermission>> = {
  "workspace.readFile": "fs.read",
  "workspace.writeFile": "fs.write",
  // commands.register, commands.execute, views.register, getPermissions
  // are always allowed (no permission required).
};

// ---------------------------------------------------------------------------
// Worker factory (injectable for testing)
// ---------------------------------------------------------------------------

/**
 * A Worker-like object that can send and receive messages. The real
 * Worker class satisfies this interface; tests provide a mock.
 */
export interface WorkerLike {
  postMessage(message: unknown): void;
  terminate(): void;
  onmessage: ((e: { data: unknown }) => void) | null;
  onerror: ((e: unknown) => void) | null;
}

/**
 * Factory function that creates a Worker for a given plugin URL.
 * The default implementation uses Vite's `?worker` import to bundle
 * the bootstrap script as a Worker entry. Tests inject a mock factory
 * to avoid spawning real Workers in jsdom.
 */
export type WorkerFactory = (workerScriptUrl: string) => WorkerLike;

// N-33: Use Vite's `?worker` import so the bootstrap is bundled as a
// separate Worker entry in production builds. This replaces the old
// approach of loading `/pluginWorkerBootstrap.js` from a URL, which
// would 404 in production because Vite doesn't auto-emit Worker entries
// without this import suffix.
//
// The import is lazy (via dynamic import wrapper) so that test
// environments (jsdom) don't try to instantiate a real Worker at
// module load time.
let PluginWorkerCtor: { new (): Worker } | null = null;

export async function loadPluginWorkerCtor(): Promise<{ new (): Worker }> {
  if (PluginWorkerCtor) return PluginWorkerCtor;
  // `?worker` tells Vite to bundle this as a Worker entry.
  const mod = await import("./pluginWorkerBootstrap.ts?worker");
  PluginWorkerCtor = mod.default;
  return PluginWorkerCtor;
}

/** Default Worker factory using Vite's `?worker` import. */
function defaultWorkerFactory(_workerScriptUrl: string): WorkerLike {
  // This factory is called synchronously from activate(). We can't
  // await the dynamic import here, so we fall back to a direct Worker
  // constructor. The `?worker` import is resolved at build time by
  // Vite, and in dev mode Vite serves the module on demand.
  //
  // For production: Vite replaces `?worker` imports with a bundled
  // Worker constructor. We use a synchronous fallback that loads
  // the Worker from the URL, which works because Vite emits the
  // Worker entry at the path referenced by the `?worker` import.
  //
  // The _workerScriptUrl parameter is kept for custom factories that
  // may still use URL-based loading.
  try {
    // Try the Vite-provided constructor first (available after the
    // dynamic import resolves; in production the import is inlined).
    if (PluginWorkerCtor) {
      return new PluginWorkerCtor() as unknown as WorkerLike;
    }
  } catch {
    // Fall through to URL-based construction.
  }
  // Fallback: construct from URL (works in dev mode where Vite serves
  // the bootstrap at the expected path).
  return new Worker(_workerScriptUrl, { type: "module" }) as unknown as WorkerLike;
}

// ---------------------------------------------------------------------------
// RPC handler (host-side dispatcher for Worker requests)
// ---------------------------------------------------------------------------

/**
 * RpcHandler processes an RPC request from a sandboxed plugin. It
 * validates permissions and dispatches to the real service. The
 * handler is injectable so tests can mock the backend services.
 *
 * Returns a Promise that resolves to the method's result, or rejects
 * with an error message string.
 */
export type RpcHandler = (
  pluginName: string,
  manifest: PluginManifest,
  method: RpcMethod,
  args: unknown[],
) => Promise<unknown>;

// ---------------------------------------------------------------------------
// PluginSandboxHost
// ---------------------------------------------------------------------------

/**
 * Structural interface for the sandbox host's public API. Allows tests
 * to inject mock implementations without subclassing PluginSandboxHost.
 */
export interface SandboxHost {
  activate(pluginName: string, manifest: PluginManifest, pluginUrl: string): Promise<void>;
  callMethod(pluginName: string, method: string, args: unknown[]): Promise<unknown>;
  terminate(pluginName: string): void;
  terminateAll(): void;
  has(pluginName: string): boolean;
  /** N-40 / Proposal J2: Get health snapshot for a sandboxed plugin. */
  getHealth?(pluginName: string): PluginHealth;
  /** N-40: Subscribe to health changes. Returns an unsubscribe function. */
  onHealthChange?(listener: HealthListener): () => void;
}

interface SandboxEntry {
  worker: WorkerLike;
  manifest: PluginManifest;
  /** Pending RPC requests sent TO the worker, keyed by request ID. */
  pendingCalls: Map<number, { resolve: (v: unknown) => void; reject: (e: Error) => void }>;
  /** Pending RPC requests FROM the worker, keyed by request ID. */
  pendingRequests: Map<number, number>;
  /** Next request ID to use when calling into the worker. */
  nextCallId: number;
  /** Resolves when the worker reports 'activated' or 'activation-error'. */
  activationPromise: Promise<void>;
  activationResolve?: () => void;
  activationReject?: (e: Error) => void;
  /**
   * N-40: Runtime crash tracking. Set to true when the Worker errors
   * after activation. The entry stays in the map so callers get a
   * clear "crashed" error instead of a generic "not sandboxed".
   */
  crashed: boolean;
  /** N-40: Last error message (activation or runtime). */
  lastError: string | null;
  /** N-40: Timestamp of the last crash (ms since epoch). */
  lastCrashAt: number | null;
}

/**
 * N-40 / Proposal J2: Health snapshot for a sandboxed plugin.
 * Returned by getHealth() and surfaced in the PluginsView health panel.
 */
export interface PluginHealth {
  status: "activating" | "running" | "crashed" | "terminated";
  lastError: string | null;
  lastCrashAt: number | null;
}

/**
 * N-40: Listener for sandbox health changes. Called when a plugin
 * crashes, recovers, or is terminated. Used by the PluginsView to
 * refresh the health panel without polling.
 */
export type HealthListener = (pluginName: string, health: PluginHealth) => void;

/**
 * PluginSandboxHost manages Web Worker sandboxes for plugins. Each
 * sandboxed plugin runs in its own Worker. The host:
 *   1. Creates Workers and sends init messages with the plugin URL
 *   2. Routes RPC requests from Workers through the permission-gated RpcHandler
 *   3. Routes RPC calls TO Workers (for command execution from other plugins)
 *   4. Tracks activation state and pending requests
 *
 * Usage:
 *   const host = new PluginSandboxHost(rpcHandler);
 *   await host.activate('my-plugin', manifest, pluginUrl);
 *   // Plugin is now running in a Worker.
 *   const result = await host.callMethod('my-plugin', 'myCommand', [args]);
 *   host.terminate('my-plugin');
 */
export class PluginSandboxHost implements SandboxHost {
  private sandboxes = new Map<string, SandboxEntry>();
  private rpcHandler: RpcHandler;
  private workerFactory: WorkerFactory;
  private workerScriptUrl: string;
  /** N-40: Health change listeners (Proposal J2 dashboard). */
  private healthListeners = new Set<HealthListener>();

  constructor(
    rpcHandler: RpcHandler,
    options?: {
      workerFactory?: WorkerFactory;
      workerScriptUrl?: string;
    },
  ) {
    this.rpcHandler = rpcHandler;
    this.workerFactory = options?.workerFactory ?? defaultWorkerFactory;
    // Default: load the bootstrap script from the standard location.
    // The bootstrap is bundled by Vite as a separate Worker entry.
    this.workerScriptUrl = options?.workerScriptUrl ?? "/pluginWorkerBootstrap.js";
  }

  /** N-40: Notify all health listeners of a change. */
  private notifyHealthChange(pluginName: string, entry: SandboxEntry): void {
    const health = this.entryHealth(entry);
    for (const listener of this.healthListeners) {
      try {
        listener(pluginName, health);
      } catch {
        // Listener errors are ignored — health notifications must not
        // crash the host.
      }
    }
  }

  /** N-40: Compute health snapshot from an entry. */
  private entryHealth(entry: SandboxEntry): PluginHealth {
    if (entry.crashed) {
      return {
        status: "crashed",
        lastError: entry.lastError,
        lastCrashAt: entry.lastCrashAt,
      };
    }
    // If activationResolve has been called, the worker is running.
    // We can't directly check if the promise resolved, but if
    // activationReject hasn't been called and crashed is false, we
    // assume running.
    return {
      status: "running",
      lastError: entry.lastError,
      lastCrashAt: entry.lastCrashAt,
    };
  }

  /**
   * Activate a plugin in a sandboxed Worker. Creates the Worker, sends
   * the init message, and waits for the 'activated' or 'activation-error'
   * response. Returns a promise that resolves on successful activation
   * or rejects on error.
   */
  activate(pluginName: string, manifest: PluginManifest, pluginUrl: string): Promise<void> {
    // If already sandboxed, terminate the old worker first.
    if (this.sandboxes.has(pluginName)) {
      this.terminate(pluginName);
    }

    const worker = this.workerFactory(this.workerScriptUrl);
    let activationResolve: () => void;
    let activationReject: (e: Error) => void;
    const activationPromise = new Promise<void>((resolve, reject) => {
      activationResolve = resolve;
      activationReject = reject;
    });

    const entry: SandboxEntry = {
      worker,
      manifest,
      pendingCalls: new Map(),
      pendingRequests: new Map(),
      nextCallId: 0,
      activationPromise,
      activationResolve: activationResolve!,
      activationReject: activationReject!,
      crashed: false,
      lastError: null,
      lastCrashAt: null,
    };

    this.sandboxes.set(pluginName, entry);

    // Wire up message handling.
    worker.onmessage = (e: { data: unknown }) => {
      this.handleWorkerMessage(pluginName, e.data as WorkerToHostMessage);
    };
    worker.onerror = (err: unknown) => {
      const msg = err instanceof Error ? err.message : String(err);
      // N-40: Always record the error on the entry so getHealth() can
      // surface it even after activation.
      entry.lastError = msg;
      entry.lastCrashAt = Date.now();
      entry.crashed = true;

      // Reject activation if it hasn't resolved yet.
      entry.activationReject?.(new Error(`Worker error: ${msg}`));

      // N-40: Reject all pending calls so callers don't hang forever.
      // The worker is dead — these promises will never resolve otherwise.
      for (const [, { reject }] of entry.pendingCalls) {
        reject(new Error(`Worker crashed: ${msg}`));
      }
      entry.pendingCalls.clear();

      // N-40: Notify health listeners (PluginView dashboard, etc.).
      this.notifyHealthChange(pluginName, entry);

      // Best-effort: terminate the dead worker to free resources.
      try {
        entry.worker.terminate();
      } catch {
        // Ignore — already dead.
      }
    };

    // Send the init message to start the plugin.
    const initMsg: HostToWorkerMessage = { type: "init", pluginUrl, manifest };
    worker.postMessage(initMsg);

    return activationPromise;
  }

  /**
   * Call a method on the sandboxed plugin (e.g. execute a command
   * handler). Returns a promise that resolves with the method's
   * return value. This is used when the main thread or another plugin
   * needs to invoke functionality inside the sandboxed plugin.
   *
   * N-31: Implemented. Sends an `rpc-call` message to the Worker. The
   * Worker's bootstrap script looks up the command handler in its
   * local `commandHandlers` map and returns the result via `rpc-result`.
   */
  callMethod(pluginName: string, method: string, args: unknown[]): Promise<unknown> {
    const entry = this.sandboxes.get(pluginName);
    if (!entry) {
      return Promise.reject(new Error(`Plugin "${pluginName}" is not sandboxed`));
    }
    // N-40: Fail fast if the worker has crashed. Without this, the
    // postMessage would silently no-op and the promise would hang.
    if (entry.crashed) {
      return Promise.reject(
        new Error(
          `Plugin "${pluginName}" Worker has crashed: ${entry.lastError ?? "unknown error"}`,
        ),
      );
    }

    const id = entry.nextCallId++;
    const msg: HostToWorkerMessage = { type: "rpc-call", id, method, args };

    return new Promise((resolve, reject) => {
      entry.pendingCalls.set(id, { resolve, reject });
      entry.worker.postMessage(msg);
    });
  }

  /** Terminate a plugin's Worker and clean up. */
  terminate(pluginName: string): void {
    const entry = this.sandboxes.get(pluginName);
    if (!entry) return;

    // Reject any pending calls.
    for (const [, { reject }] of entry.pendingCalls) {
      reject(new Error("Worker terminated"));
    }

    // Send terminate message (best-effort; worker may already be dead).
    try {
      entry.worker.postMessage({ type: "terminate" } satisfies HostToWorkerMessage);
    } catch {
      // Ignore — worker may be unresponsive.
    }
    entry.worker.terminate();

    // N-40: Mark as crashed/terminated so getHealth() reflects the state.
    entry.crashed = true;
    entry.lastError = "Worker terminated";
    this.notifyHealthChange(pluginName, entry);

    this.sandboxes.delete(pluginName);
  }

  /** Terminate all sandboxed plugins. */
  terminateAll(): void {
    for (const name of Array.from(this.sandboxes.keys())) {
      this.terminate(name);
    }
  }

  /** Check if a plugin is currently sandboxed. */
  has(pluginName: string): boolean {
    return this.sandboxes.has(pluginName);
  }

  /** N-40 / Proposal J2: Get health snapshot for a sandboxed plugin. */
  getHealth(pluginName: string): PluginHealth {
    const entry = this.sandboxes.get(pluginName);
    if (!entry) {
      return { status: "terminated", lastError: null, lastCrashAt: null };
    }
    return this.entryHealth(entry);
  }

  /** N-40: Subscribe to health changes. Returns an unsubscribe function. */
  onHealthChange(listener: HealthListener): () => void {
    this.healthListeners.add(listener);
    return () => {
      this.healthListeners.delete(listener);
    };
  }

  // -------------------------------------------------------------------------
  // Internal: message dispatch
  // -------------------------------------------------------------------------

  private handleWorkerMessage(pluginName: string, msg: WorkerToHostMessage): void {
    const entry = this.sandboxes.get(pluginName);
    if (!entry) return;

    switch (msg.type) {
      case "activated":
        // N-40: Clear any stale error from a previous failed attempt.
        entry.lastError = null;
        entry.crashed = false;
        entry.activationResolve?.();
        this.notifyHealthChange(pluginName, entry);
        break;

      case "activation-error":
        // N-40: Record the activation error on the entry.
        entry.lastError = msg.error;
        entry.lastCrashAt = Date.now();
        entry.crashed = true;
        entry.activationReject?.(new Error(msg.error));
        this.notifyHealthChange(pluginName, entry);
        break;

      case "rpc-request":
        this.handleRpcRequest(pluginName, entry, msg.id, msg.method as RpcMethod, msg.args);
        break;

      case "rpc-result": {
        // N-31: Result of a call we made INTO the worker (e.g. command
        // execution). Resolve or reject the pending call.
        const pending = entry.pendingCalls.get(msg.id);
        if (pending) {
          entry.pendingCalls.delete(msg.id);
          if (msg.error) {
            pending.reject(new Error(msg.error));
          } else {
            pending.resolve(msg.result);
          }
        }
        break;
      }

      case "log":
        // Forward plugin console output to the host's console for debugging.
        console[msg.level](`[plugin:${pluginName}] ${msg.message}`);
        break;
    }
  }

  /**
   * Handle an RPC request from a sandboxed plugin. Validates permissions
   * and dispatches to the real service via the RpcHandler.
   */
  private async handleRpcRequest(
    pluginName: string,
    entry: SandboxEntry,
    requestId: number,
    method: RpcMethod,
    args: unknown[],
  ): Promise<void> {
    try {
      // Permission check: verify the plugin declared the required permission.
      const requiredPerm = METHOD_PERMISSIONS[method];
      if (requiredPerm) {
        const declared = new Set(entry.manifest.permissions ?? []);
        if (!declared.has(requiredPerm)) {
          throw new Error(
            `Plugin "${pluginName}" cannot call ${method}: requires permission "${requiredPerm}" not declared in manifest`,
          );
        }
      }

      // Dispatch to the real service.
      const result = await this.rpcHandler(pluginName, entry.manifest, method, args);

      // Send success response.
      const response: HostToWorkerMessage = {
        type: "rpc-response",
        id: requestId,
        result,
      };
      entry.worker.postMessage(response);
    } catch (e: unknown) {
      const errorMsg = e instanceof Error ? e.message : String(e);
      const response: HostToWorkerMessage = {
        type: "rpc-response",
        id: requestId,
        error: errorMsg,
      };
      entry.worker.postMessage(response);
    }
  }
}

// ---------------------------------------------------------------------------
// Permission validation helper (exported for testing)
// ---------------------------------------------------------------------------

/**
 * Check if a plugin's manifest declares the permission required for a
 * given RPC method. Returns true if no permission is required or if the
 * permission is declared; false otherwise.
 */
export function hasPermissionForMethod(
  manifest: PluginManifest,
  method: RpcMethod,
): boolean {
  const required = METHOD_PERMISSIONS[method];
  if (!required) return true;
  const declared = manifest.permissions ?? [];
  return declared.includes(required);
}
