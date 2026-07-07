import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  PluginSandboxHost,
  hasPermissionForMethod,
  type WorkerLike,
  type RpcHandler,
  type RpcMethod,
  type WorkerToHostMessage,
  type HostToWorkerMessage,
} from "./pluginSandbox";
import type { PluginManifest } from "@/types";

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

/** A mock Worker that simulates message passing. */
class MockWorker implements WorkerLike {
  onmessage: ((e: { data: unknown }) => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  terminated = false;
  receivedMessages: HostToWorkerMessage[] = [];

  postMessage(message: unknown): void {
    this.receivedMessages.push(message as HostToWorkerMessage);
  }

  terminate(): void {
    this.terminated = true;
  }

  /** Simulate the worker sending a message to the host. */
  sendToHost(msg: WorkerToHostMessage): void {
    if (this.onmessage) {
      this.onmessage({ data: msg });
    }
  }
}

function makeManifest(overrides?: Partial<PluginManifest>): PluginManifest {
  return {
    name: "test-plugin",
    version: "1.0.0",
    main: "main.js",
    permissions: [],
    ...overrides,
  };
}

function makeMockWorkerFactory(): {
  factory: (url: string) => WorkerLike;
  workers: MockWorker[];
} {
  const workers: MockWorker[] = [];
  const factory = () => {
    const w = new MockWorker();
    workers.push(w);
    return w;
  };
  return { factory, workers };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("Plugin Sandbox (N-26)", () => {
  describe("hasPermissionForMethod", () => {
    it("returns true for methods that require no permission", () => {
      const manifest = makeManifest();
      expect(hasPermissionForMethod(manifest, "commands.register")).toBe(true);
      expect(hasPermissionForMethod(manifest, "commands.execute")).toBe(true);
      expect(hasPermissionForMethod(manifest, "views.register")).toBe(true);
      expect(hasPermissionForMethod(manifest, "getPermissions")).toBe(true);
    });

    it("returns false for fs.read when permission is not declared", () => {
      const manifest = makeManifest({ permissions: [] });
      expect(hasPermissionForMethod(manifest, "workspace.readFile")).toBe(false);
    });

    it("returns true for fs.read when permission is declared", () => {
      const manifest = makeManifest({ permissions: ["fs.read"] });
      expect(hasPermissionForMethod(manifest, "workspace.readFile")).toBe(true);
    });

    it("returns false for fs.write when only fs.read is declared", () => {
      const manifest = makeManifest({ permissions: ["fs.read"] });
      expect(hasPermissionForMethod(manifest, "workspace.writeFile")).toBe(false);
    });

    it("returns true for fs.write when permission is declared", () => {
      const manifest = makeManifest({ permissions: ["fs.write"] });
      expect(hasPermissionForMethod(manifest, "workspace.writeFile")).toBe(true);
    });

    it("returns true when both fs.read and fs.write are declared", () => {
      const manifest = makeManifest({ permissions: ["fs.read", "fs.write"] });
      expect(hasPermissionForMethod(manifest, "workspace.readFile")).toBe(true);
      expect(hasPermissionForMethod(manifest, "workspace.writeFile")).toBe(true);
    });
  });

  describe("PluginSandboxHost", () => {
    let rpcHandler: RpcHandler;
    let mockFactory: ReturnType<typeof makeMockWorkerFactory>;

    beforeEach(() => {
      rpcHandler = vi.fn().mockResolvedValue(undefined);
      mockFactory = makeMockWorkerFactory();
    });

    describe("activate", () => {
      it("creates a worker and sends init message", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const pluginUrl = "/_plugins/test-plugin/main.js";

        // Start activation (don't await yet — the worker hasn't responded).
        const promise = host.activate("test-plugin", manifest, pluginUrl);

        // The factory should have created one worker.
        expect(mockFactory.workers).toHaveLength(1);
        const worker = mockFactory.workers[0];

        // The host should have sent an init message.
        expect(worker.receivedMessages).toHaveLength(1);
        const initMsg = worker.receivedMessages[0];
        expect(initMsg.type).toBe("init");
        if (initMsg.type === "init") {
          expect(initMsg.pluginUrl).toBe(pluginUrl);
          expect(initMsg.manifest.name).toBe("test-plugin");
        }

        // Simulate the worker reporting successful activation.
        worker.sendToHost({ type: "activated" });

        await expect(promise).resolves.toBeUndefined();
      });

      it("rejects when the worker reports activation-error", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");

        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activation-error", error: "Plugin failed to load" });

        await expect(promise).rejects.toThrow("Plugin failed to load");
      });

      it("rejects when the worker errors", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");

        const worker = mockFactory.workers[0];
        worker.onerror?.(new Error("Worker crashed"));

        await expect(promise).rejects.toThrow("Worker error: Worker crashed");
      });

      it("terminates the old worker when re-activating the same plugin", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        // First activation.
        const promise1 = host.activate("test-plugin", manifest, "/url1");
        mockFactory.workers[0].sendToHost({ type: "activated" });
        await promise1;

        // Second activation — should terminate the first worker.
        const promise2 = host.activate("test-plugin", manifest, "/url2");
        expect(mockFactory.workers[0].terminated).toBe(true);
        mockFactory.workers[1].sendToHost({ type: "activated" });
        await promise2;

        expect(mockFactory.workers).toHaveLength(2);
      });
    });

    describe("RPC request handling", () => {
      it("routes rpc-request to the rpcHandler and sends the result back", async () => {
        const handler = vi.fn().mockResolvedValue("file contents");
        const host = new PluginSandboxHost(handler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest({ permissions: ["fs.read"] });

        const activatePromise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await activatePromise;

        // Simulate the worker requesting a file read.
        worker.sendToHost({
          type: "rpc-request",
          id: 42,
          method: "workspace.readFile",
          args: ["src/main.ts"],
        });

        // Wait for the async handler to complete.
        await new Promise((r) => setTimeout(r, 10));

        // The handler should have been called with the right args.
        expect(handler).toHaveBeenCalledWith(
          "test-plugin",
          manifest,
          "workspace.readFile",
          ["src/main.ts"],
        );

        // The host should have sent an rpc-response with the result.
        const response = worker.receivedMessages.find(
          (m) => m.type === "rpc-response",
        );
        expect(response).toBeDefined();
        if (response && response.type === "rpc-response") {
          expect(response.id).toBe(42);
          expect(response.result).toBe("file contents");
          expect(response.error).toBeUndefined();
        }
      });

      it("sends an error response when the handler throws", async () => {
        const handler = vi.fn().mockRejectedValue(new Error("File not found"));
        const host = new PluginSandboxHost(handler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest({ permissions: ["fs.read"] });

        const activatePromise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await activatePromise;

        worker.sendToHost({
          type: "rpc-request",
          id: 1,
          method: "workspace.readFile",
          args: ["missing.ts"],
        });

        await new Promise((r) => setTimeout(r, 10));

        const response = worker.receivedMessages.find(
          (m) => m.type === "rpc-response",
        );
        expect(response).toBeDefined();
        if (response && response.type === "rpc-response") {
          expect(response.id).toBe(1);
          expect(response.error).toBe("File not found");
        }
      });

      it("rejects fs.read when permission is not declared", async () => {
        const handler = vi.fn();
        const host = new PluginSandboxHost(handler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest({ permissions: [] }); // no fs.read

        const activatePromise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await activatePromise;

        worker.sendToHost({
          type: "rpc-request",
          id: 1,
          method: "workspace.readFile",
          args: ["file.ts"],
        });

        await new Promise((r) => setTimeout(r, 10));

        // The handler should NOT have been called (permission denied).
        expect(handler).not.toHaveBeenCalled();

        // The response should contain the permission error.
        const response = worker.receivedMessages.find(
          (m) => m.type === "rpc-response",
        );
        expect(response).toBeDefined();
        if (response && response.type === "rpc-response") {
          expect(response.error).toContain("fs.read");
          expect(response.error).toContain("not declared");
        }
      });

      it("allows commands.register without any permission", async () => {
        const handler = vi.fn().mockResolvedValue(undefined);
        const host = new PluginSandboxHost(handler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest({ permissions: [] });

        const activatePromise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await activatePromise;

        worker.sendToHost({
          type: "rpc-request",
          id: 1,
          method: "commands.register",
          args: ["my-plugin.cmd"],
        });

        await new Promise((r) => setTimeout(r, 10));

        expect(handler).toHaveBeenCalled();
        const response = worker.receivedMessages.find(
          (m) => m.type === "rpc-response",
        );
        expect(response).toBeDefined();
        if (response && response.type === "rpc-response") {
          expect(response.error).toBeUndefined();
        }
      });

      it("allows views.register without any permission", async () => {
        const handler = vi.fn().mockResolvedValue(undefined);
        const host = new PluginSandboxHost(handler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest({ permissions: [] });

        const activatePromise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await activatePromise;

        worker.sendToHost({
          type: "rpc-request",
          id: 1,
          method: "views.register",
          args: ["my-view", { title: "My View" }],
        });

        await new Promise((r) => setTimeout(r, 10));

        expect(handler).toHaveBeenCalled();
      });

      it("allows getPermissions without any permission", async () => {
        const handler = vi.fn().mockResolvedValue(["fs.read"]);
        const host = new PluginSandboxHost(handler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest({ permissions: ["fs.read"] });

        const activatePromise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await activatePromise;

        worker.sendToHost({
          type: "rpc-request",
          id: 1,
          method: "getPermissions",
          args: [],
        });

        await new Promise((r) => setTimeout(r, 10));

        expect(handler).toHaveBeenCalled();
        const response = worker.receivedMessages.find(
          (m) => m.type === "rpc-response",
        );
        if (response && response.type === "rpc-response") {
          expect(response.result).toEqual(["fs.read"]);
        }
      });
    });

    describe("log forwarding", () => {
      it("forwards log messages to console", async () => {
        const consoleSpy = vi.spyOn(console, "info").mockImplementation(() => {});
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        worker.sendToHost({ type: "log", level: "info", message: "Hello from plugin" });

        expect(consoleSpy).toHaveBeenCalledWith("[plugin:test-plugin] Hello from plugin");
        consoleSpy.mockRestore();
      });

      it("forwards warn messages to console.warn", async () => {
        const consoleSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        worker.sendToHost({ type: "log", level: "warn", message: "Warning!" });

        expect(consoleSpy).toHaveBeenCalledWith("[plugin:test-plugin] Warning!");
        consoleSpy.mockRestore();
      });
    });

    describe("terminate", () => {
      it("terminates the worker and removes the sandbox", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        expect(host.has("test-plugin")).toBe(true);

        host.terminate("test-plugin");

        expect(worker.terminated).toBe(true);
        expect(host.has("test-plugin")).toBe(false);
      });

      it("sends a terminate message before terminating", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        host.terminate("test-plugin");

        // The last message should be 'terminate'.
        const lastMsg = worker.receivedMessages[worker.receivedMessages.length - 1];
        expect(lastMsg.type).toBe("terminate");
      });

      it("is a no-op for non-existent plugin", () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        // Should not throw.
        host.terminate("nonexistent");
      });
    });

    describe("terminateAll", () => {
      it("terminates all sandboxed plugins", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });

        // Activate two plugins.
        const p1 = host.activate("plugin1", makeManifest({ name: "plugin1" }), "/url1");
        mockFactory.workers[0].sendToHost({ type: "activated" });
        await p1;

        const p2 = host.activate("plugin2", makeManifest({ name: "plugin2" }), "/url2");
        mockFactory.workers[1].sendToHost({ type: "activated" });
        await p2;

        expect(host.has("plugin1")).toBe(true);
        expect(host.has("plugin2")).toBe(true);

        host.terminateAll();

        expect(mockFactory.workers[0].terminated).toBe(true);
        expect(mockFactory.workers[1].terminated).toBe(true);
        expect(host.has("plugin1")).toBe(false);
        expect(host.has("plugin2")).toBe(false);
      });
    });

    describe("has", () => {
      it("returns false for non-sandboxed plugins", () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        expect(host.has("nonexistent")).toBe(false);
      });

      it("returns true for sandboxed plugins", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const promise = host.activate("test-plugin", makeManifest(), "/url");
        mockFactory.workers[0].sendToHost({ type: "activated" });
        await promise;

        expect(host.has("test-plugin")).toBe(true);
      });
    });

    describe("callMethod (N-31)", () => {
      it("rejects for non-sandboxed plugin", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        await expect(host.callMethod("nonexistent", "executeCommand", [])).rejects.toThrow(
          'Plugin "nonexistent" is not sandboxed',
        );
      });

      it("sends rpc-call to worker and resolves on rpc-result", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        // Initiate callMethod — this sends an rpc-call to the worker.
        const callPromise = host.callMethod("test-plugin", "executeCommand", ["my-cmd"]);
        // Verify the worker received the rpc-call message.
        const callMsg = worker.receivedMessages.find(
          (m) => m.type === "rpc-call",
        ) as { type: "rpc-call"; id: number; method: string; args: unknown[] } | undefined;
        expect(callMsg).toBeDefined();
        expect(callMsg!.method).toBe("executeCommand");
        expect(callMsg!.args).toEqual(["my-cmd"]);

        // Simulate the worker responding with a result.
        worker.sendToHost({ type: "rpc-result", id: callMsg!.id, result: "ok" });

        const result = await callPromise;
        expect(result).toBe("ok");
      });

      it("rejects on rpc-result with error", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        const callPromise = host.callMethod("test-plugin", "executeCommand", ["bad-cmd"]);
        const callMsg = worker.receivedMessages.find(
          (m) => m.type === "rpc-call",
        ) as { type: "rpc-call"; id: number } | undefined;
        expect(callMsg).toBeDefined();

        worker.sendToHost({ type: "rpc-result", id: callMsg!.id, error: "Command not found" });

        await expect(callPromise).rejects.toThrow("Command not found");
      });

      it("rejects pending calls on terminate", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        const callPromise = host.callMethod("test-plugin", "executeCommand", ["slow-cmd"]);
        // Terminate before the worker responds.
        host.terminate("test-plugin");

        await expect(callPromise).rejects.toThrow("Worker terminated");
      });
    });

    // -------------------------------------------------------------------------
    // N-40 (prompt-5.md): Worker crash handling
    // -------------------------------------------------------------------------

    describe("N-40: Worker crash handling", () => {
      it("getHealth returns 'running' after successful activation", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        mockFactory.workers[0].sendToHost({ type: "activated" });
        await promise;

        const health = host.getHealth("test-plugin");
        expect(health.status).toBe("running");
        expect(health.lastError).toBeNull();
        expect(health.lastCrashAt).toBeNull();
      });

      it("getHealth returns 'terminated' for unknown plugin", () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const health = host.getHealth("nonexistent");
        expect(health.status).toBe("terminated");
      });

      it("getHealth returns 'crashed' after runtime worker.onerror", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        // Simulate a runtime crash (after activation).
        worker.onerror?.(new Error("Uncaught TypeError: undefined is not a function"));

        const health = host.getHealth("test-plugin");
        expect(health.status).toBe("crashed");
        expect(health.lastError).toContain("Uncaught TypeError");
        expect(health.lastCrashAt).not.toBeNull();
      });

      it("rejects pending calls when worker crashes at runtime", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        // Start a call that the worker will never respond to.
        const callPromise = host.callMethod("test-plugin", "executeCommand", ["slow-cmd"]);

        // Crash the worker before it responds.
        worker.onerror?.(new Error("Worker crashed"));

        await expect(callPromise).rejects.toThrow("Worker crashed");
      });

      it("callMethod fails fast after crash", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        // Crash the worker.
        worker.onerror?.(new Error("OOM"));

        // Subsequent calls should reject immediately with the crash message.
        await expect(
          host.callMethod("test-plugin", "executeCommand", ["cmd"]),
        ).rejects.toThrow("Worker has crashed");
      });

      it("terminates the worker on crash", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        worker.onerror?.(new Error("crash"));

        expect(worker.terminated).toBe(true);
      });

      it("notifies health listeners on crash", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const listener = vi.fn();
        host.onHealthChange(listener);

        const promise = host.activate("test-plugin", manifest, "/url");
        const worker = mockFactory.workers[0];
        worker.sendToHost({ type: "activated" });
        await promise;

        // Listener should have been called with "running" on activation.
        expect(listener).toHaveBeenCalledWith(
          "test-plugin",
          expect.objectContaining({ status: "running" }),
        );

        // Crash the worker.
        worker.onerror?.(new Error("crash"));

        // Listener should have been called with "crashed".
        expect(listener).toHaveBeenCalledWith(
          "test-plugin",
          expect.objectContaining({
            status: "crashed",
            lastError: "crash",
          }),
        );
      });

      it("onHealthChange returns an unsubscribe function", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const listener = vi.fn();
        const unsubscribe = host.onHealthChange(listener);

        expect(typeof unsubscribe).toBe("function");
        unsubscribe();

        // After unsubscribe, the listener should not be called.
        const manifest = makeManifest();
        const promise = host.activate("test-plugin", manifest, "/url");
        mockFactory.workers[0].sendToHost({ type: "activated" });
        await promise;

        expect(listener).not.toHaveBeenCalled();
      });

      it("records lastError on activation-error", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();

        const promise = host.activate("test-plugin", manifest, "/url");
        mockFactory.workers[0].sendToHost({
          type: "activation-error",
          error: "Plugin failed to load: missing export",
        });

        await expect(promise).rejects.toThrow("Plugin failed to load");

        const health = host.getHealth("test-plugin");
        expect(health.status).toBe("crashed");
        expect(health.lastError).toContain("missing export");
      });

      it("notifies health listeners on terminate", async () => {
        const host = new PluginSandboxHost(rpcHandler, {
          workerFactory: mockFactory.factory,
        });
        const manifest = makeManifest();
        const listener = vi.fn();
        host.onHealthChange(listener);

        const promise = host.activate("test-plugin", manifest, "/url");
        mockFactory.workers[0].sendToHost({ type: "activated" });
        await promise;
        listener.mockClear();

        host.terminate("test-plugin");

        expect(listener).toHaveBeenCalledWith(
          "test-plugin",
          expect.objectContaining({ status: "crashed" }),
        );
      });
    });
  });
});
