/**
 * Plugin Worker bootstrap script (N-26).
 *
 * This file runs inside a Web Worker. It receives an 'init' message
 * from the host containing the plugin URL and manifest, dynamically
 * imports the plugin module, and calls its activate() function with
 * a proxied nknk context.
 *
 * The proxied context forwards all nknk.* API calls to the host via
 * postMessage (rpc-request). The host validates permissions and
 * dispatches to the real service, then sends the result back via
 * rpc-response.
 *
 * The Worker has NO access to:
 *   - window, document, localStorage, indexedDB (not available in Workers)
 *   - Monaco editor (runs on the main thread)
 *   - Vue app root
 *   - Wails IPC bindings (window.go is on the main thread)
 *
 * This isolation is the core security benefit of the sandbox: a
 * malicious plugin cannot steal API keys or bypass permissions because
 * it can only communicate through the permission-gated postMessage bridge.
 */

/// <reference lib="webworker" />

import type { PluginManifest, PluginPermission } from "@/types";
import type { HostToWorkerMessage, WorkerToHostMessage, RpcMethod } from "./pluginSandbox";

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

let manifest: PluginManifest | null = null;
let nextRequestId = 0;
const pendingRequests = new Map<
  number,
  { resolve: (v: unknown) => void; reject: (e: Error) => void }
>();

// ---------------------------------------------------------------------------
// Message handling (from host)
// ---------------------------------------------------------------------------

self.onmessage = async (e: MessageEvent) => {
  const msg = e.data as HostToWorkerMessage;
  if (!msg || typeof msg !== "object" || !("type" in msg)) return;

  switch (msg.type) {
    case "init":
      await handleInit(msg.pluginUrl, msg.manifest);
      break;

    case "rpc-response":
      handleRpcResponse(msg.id, msg.result, msg.error);
      break;

    case "rpc-call":
      // N-31: The host is calling INTO the worker (e.g. to execute a
      // command handler). Look up the handler and return the result.
      await handleRpcCall(msg.id, msg.method, msg.args);
      break;

    case "terminate":
      // The host is terminating us. Clean up and stop.
      self.close();
      break;
  }
};

// ---------------------------------------------------------------------------
// Init: load and activate the plugin
// ---------------------------------------------------------------------------

async function handleInit(pluginUrl: string, mfst: PluginManifest): Promise<void> {
  manifest = mfst;
  try {
    // Dynamic import of the plugin's main.js entry point.
    // The URL is served by the backend's /_plugins/ asset handler.
    const module = await import(/* @vite-ignore */ pluginUrl);

    if (typeof module.activate !== "function") {
      throw new Error(
        `Plugin "${mfst.name}" main module does not export an activate() function`,
      );
    }

    // Create the proxied nknk context and call activate().
    const context = createProxyContext(mfst);
    await module.activate(context);

    // Notify the host that activation succeeded.
    sendToHost({ type: "activated" });
  } catch (e: unknown) {
    const errorMsg = e instanceof Error ? e.message : String(e);
    sendToHost({ type: "activation-error", error: errorMsg });
  }
}

// ---------------------------------------------------------------------------
// Proxy nknk context
// ---------------------------------------------------------------------------

interface ProxyNknkAPI {
  manifest: PluginManifest;
  commands: {
    register(id: string, handler: (...args: unknown[]) => unknown | Promise<unknown>): void;
    registerCommand(id: string, handler: (...args: unknown[]) => unknown | Promise<unknown>): void;
    execute(id: string, ...args: unknown[]): Promise<unknown>;
    executeCommand(id: string, ...args: unknown[]): Promise<unknown>;
  };
  views: {
    register(
      id: string,
      component: unknown,
      options?: { title?: string; location?: "sidebar" | "panel" | "statusbar" },
    ): void;
  };
  workspace: {
    readFile(relPath: string): Promise<string>;
    writeFile(relPath: string, content: string): Promise<void>;
  };
  getPermissions(): PluginPermission[];
}

/**
 * Create a proxy nknk context that forwards all API calls to the host
 * via postMessage. The host validates permissions before executing.
 *
 * Note: commands.register and views.register send the handler/component
 * reference to the host. Since functions can't be serialized via
 * structured clone, the host stores a reference and calls back via
 * rpc-call when the command needs to be executed. (Cross-sandbox command
 * execution is a future enhancement; for now, commands.register just
 * notifies the host of the command's metadata.)
 */
function createProxyContext(mfst: PluginManifest): ProxyNknkAPI {
  const callHost = (method: RpcMethod, args: unknown[]): Promise<unknown> => {
    return sendRpcRequest(method, args);
  };

  return {
    manifest: mfst,
    commands: {
      register(id, handler) {
        // Send the command registration to the host. The handler can't
        // be serialized, so we store it locally and the host calls
        // back via rpc-call when the command needs to execute.
        // For now, we just notify the host of the command metadata.
        // The handler is stored in a local map for later invocation.
        commandHandlers.set(id, handler);
        void callHost("commands.register", [id]).catch(() => {
          // Registration failed (e.g. duplicate ID). Ignore — the host
          // will have already thrown, and we can't undo the local
          // registration easily. The plugin will see the error when
          // it tries to execute the command.
        });
      },
      registerCommand(id, handler) {
        // Alias for register.
        commandHandlers.set(id, handler);
        void callHost("commands.register", [id]).catch(() => {});
      },
      async execute(id, ...args) {
        return callHost("commands.execute", [id, ...args]);
      },
      async executeCommand(id, ...args) {
        return callHost("commands.execute", [id, ...args]);
      },
    },
    views: {
      register(id, _component, options) {
        // The component can't be serialized across the Worker boundary.
        // The host will need to load the component separately (e.g. via
        // an iframe with the plugin's HTML/JS bundle). For now, we just
        // register the view metadata.
        void callHost("views.register", [id, options ?? {}]).catch(() => {});
      },
    },
    workspace: {
      async readFile(relPath) {
        return callHost("workspace.readFile", [relPath]) as Promise<string>;
      },
      async writeFile(relPath, content) {
        await callHost("workspace.writeFile", [relPath, content]);
      },
    },
    getPermissions() {
      return mfst.permissions ?? [];
    },
  };
}

// Local store of command handlers (can't be sent to the host).
const commandHandlers = new Map<
  string,
  (...args: unknown[]) => unknown | Promise<unknown>
>();

// ---------------------------------------------------------------------------
// RPC request/response (Worker → Host)
// ---------------------------------------------------------------------------

function sendRpcRequest(method: RpcMethod, args: unknown[]): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const id = nextRequestId++;
    pendingRequests.set(id, { resolve, reject });
    sendToHost({ type: "rpc-request", id, method, args });
  });
}

function handleRpcResponse(id: number, result: unknown, error?: string): void {
  const pending = pendingRequests.get(id);
  if (!pending) return;
  pendingRequests.delete(id);
  if (error) {
    pending.reject(new Error(error));
  } else {
    pending.resolve(result);
  }
}

/**
 * N-31: Handle an rpc-call from the host. The host calls INTO the worker
 * to execute a command handler stored in `commandHandlers`. The result
 * is sent back via `rpc-result`.
 *
 * Supported methods:
 *   - "executeCommand": args = [commandId, ...callArgs]
 */
async function handleRpcCall(id: number, method: string, args: unknown[]): Promise<void> {
  try {
    if (method === "executeCommand") {
      const commandId = args[0] as string;
      const callArgs = args.slice(1);
      const handler = commandHandlers.get(commandId);
      if (!handler) {
        sendToHost({
          type: "rpc-result",
          id,
          error: `Command "${commandId}" not found in worker`,
        });
        return;
      }
      const result = await handler(...callArgs);
      sendToHost({ type: "rpc-result", id, result });
    } else {
      sendToHost({
        type: "rpc-result",
        id,
        error: `Unknown rpc-call method: ${method}`,
      });
    }
  } catch (e: unknown) {
    const errorMsg = e instanceof Error ? e.message : String(e);
    sendToHost({ type: "rpc-result", id, error: errorMsg });
  }
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

function sendToHost(msg: WorkerToHostMessage): void {
  (self as unknown as Worker).postMessage(msg);
}
