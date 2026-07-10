import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

// Mock @/stores/app so the connectivity module reads a controllable
// aiBaseUrl without pulling in the full app store (and its transitive
// deps: @wailsio/runtime, monaco-themes, rules, i18n, etc.).
vi.mock("@/stores/app", () => ({
  appState: {
    aiBaseUrl: "https://api.example.com",
  },
}));

import {
  connectivityState,
  checkAIReachable,
  initConnectivityListener,
  stopConnectivityListener,
  __resetConnectivityForTesting,
} from "./connectivity";
import { appState } from "@/stores/app";

/** Helper: stub navigator.onLine (jsdom defaults to true, configurable: false). */
function setNavigatorOnLine(value: boolean): void {
  Object.defineProperty(navigator, "onLine", {
    value,
    configurable: true,
    writable: true,
  });
}

/** Helper: stub global fetch with a resolved/rejected impl. */
function stubFetch(impl: () => Promise<unknown>): void {
  vi.stubGlobal("fetch", vi.fn(impl));
}

describe("connectivity (G-FEAT-02)", () => {
  beforeEach(() => {
    __resetConnectivityForTesting();
    setNavigatorOnLine(true);
    // Default: fetch resolves (server reachable).
    stubFetch(() => Promise.resolve(new Response()));
  });

  afterEach(() => {
    stopConnectivityListener();
    __resetConnectivityForTesting();
    vi.unstubAllGlobals();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  describe("initial state", () => {
    it("exposes online/aiReachable/checking fields", () => {
      expect(connectivityState).toHaveProperty("online");
      expect(connectivityState).toHaveProperty("aiReachable");
      expect(connectivityState).toHaveProperty("checking");
    });

    it("defaults aiReachable to true and checking to false", () => {
      expect(connectivityState.aiReachable).toBe(true);
      expect(connectivityState.checking).toBe(false);
    });

    it("reflects navigator.onLine on reset", () => {
      setNavigatorOnLine(false);
      __resetConnectivityForTesting();
      expect(connectivityState.online).toBe(false);
      setNavigatorOnLine(true);
      __resetConnectivityForTesting();
      expect(connectivityState.online).toBe(true);
    });
  });

  describe("checkAIReachable", () => {
    it("returns false when no baseUrl is configured", async () => {
      const saved = appState.aiBaseUrl;
      (appState as { aiBaseUrl: string }).aiBaseUrl = "";
      try {
        const result = await checkAIReachable();
        expect(result).toBe(false);
      } finally {
        (appState as { aiBaseUrl: string }).aiBaseUrl = saved;
      }
    });

    it("returns true when the heartbeat fetch resolves", async () => {
      const result = await checkAIReachable();
      expect(result).toBe(true);
      expect(fetch).toHaveBeenCalledTimes(1);
    });

    it("uses HEAD + no-cors + no-store", async () => {
      await checkAIReachable();
      expect(fetch).toHaveBeenCalledWith(
        "https://api.example.com",
        expect.objectContaining({
          method: "HEAD",
          mode: "no-cors",
          cache: "no-store",
        }),
      );
    });

    it("returns false when the fetch rejects (network error)", async () => {
      stubFetch(() => Promise.reject(new TypeError("Failed to fetch")));
      const result = await checkAIReachable();
      expect(result).toBe(false);
    });

    it("returns false when the fetch is aborted (timeout)", async () => {
      // The module aborts the request after HEARTBEAT_TIMEOUT_MS (5s). To
      // avoid waiting for the real timer in the test, we have fetch reject
      // immediately with an AbortError — the module's catch block treats
      // any rejection (including abort) as "not reachable" and returns false.
      stubFetch(() => Promise.reject(new DOMException("aborted", "AbortError")));
      const result = await checkAIReachable();
      expect(result).toBe(false);
    });

    it("never throws — swallows errors and returns false", async () => {
      stubFetch(() => {
        throw new Error("unexpected sync throw");
      });
      await expect(checkAIReachable()).resolves.toBe(false);
    });

    it("toggles checking during the probe and clears it after", async () => {
      let release!: () => void;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      stubFetch(() => held.then(() => new Response()));

      const promise = checkAIReachable();
      // While in flight, checking should be true.
      expect(connectivityState.checking).toBe(true);
      release();
      await promise;
      expect(connectivityState.checking).toBe(false);
    });

    it("clears checking even when the fetch rejects", async () => {
      stubFetch(() => Promise.reject(new Error("boom")));
      await checkAIReachable();
      expect(connectivityState.checking).toBe(false);
    });
  });

  describe("initConnectivityListener", () => {
    it("is idempotent — a second call is a no-op", () => {
      const addSpy = vi.spyOn(window, "addEventListener");
      initConnectivityListener();
      const firstCallCount = addSpy.mock.calls.length;
      initConnectivityListener();
      // No additional listeners should have been registered.
      expect(addSpy.mock.calls.length).toBe(firstCallCount);
    });

    it("registers online and offline window listeners", () => {
      initConnectivityListener();
      // Note: the spy was installed after init, so it only captures later
      // calls. Reset and re-init to capture cleanly.
      vi.spyOn(window, "addEventListener").mockRestore();
      const freshSpy = vi.spyOn(window, "addEventListener");
      stopConnectivityListener();
      initConnectivityListener();
      const registered = freshSpy.mock.calls.map((c) => c[0]);
      expect(registered).toContain("online");
      expect(registered).toContain("offline");
    });

    it("dispatching a window offline event sets online=false", () => {
      initConnectivityListener();
      expect(connectivityState.online).toBe(true);
      window.dispatchEvent(new Event("offline"));
      expect(connectivityState.online).toBe(false);
      expect(connectivityState.aiReachable).toBe(false);
    });

    it("dispatching a window online event re-probes (reachable → online stays true)", async () => {
      initConnectivityListener();
      // fetch defaults to resolving.
      window.dispatchEvent(new Event("online"));
      // Wait for the async refresh to complete.
      await vi.waitFor(() => {
        expect(connectivityState.aiReachable).toBe(true);
      });
      expect(connectivityState.online).toBe(true);
    });

    it("online event with an unreachable server keeps navigator.onLine as the authority", async () => {
      stubFetch(() => Promise.reject(new TypeError("net")));
      initConnectivityListener();
      window.dispatchEvent(new Event("online"));
      await vi.waitFor(() => {
        expect(connectivityState.aiReachable).toBe(false);
      });
      // online follows navigator.onLine (true), not the heartbeat.
      expect(connectivityState.online).toBe(true);
    });
  });

  describe("stopConnectivityListener", () => {
    it("removes the online and offline listeners", () => {
      const removeSpy = vi.spyOn(window, "removeEventListener");
      initConnectivityListener();
      stopConnectivityListener();
      const removed = removeSpy.mock.calls.map((c) => c[0]);
      expect(removed).toContain("online");
      expect(removed).toContain("offline");
    });

    it("allows re-initialisation after stop", () => {
      initConnectivityListener();
      stopConnectivityListener();
      // After stop, init should register fresh listeners (no longer idempotent-guarded).
      const addSpy = vi.spyOn(window, "addEventListener");
      initConnectivityListener();
      const registered = addSpy.mock.calls.map((c) => c[0]);
      expect(registered).toContain("online");
      expect(registered).toContain("offline");
    });
  });

  describe("offline state and AI send gating", () => {
    it("connectivityState.online flips to false on the offline event", () => {
      initConnectivityListener();
      expect(connectivityState.online).toBe(true);
      window.dispatchEvent(new Event("offline"));
      expect(connectivityState.online).toBe(false);
      // The StatusBar and AiChatPanel read connectivityState.online
      // directly; this flag drives the offline badge and the send-button
      // disable binding.
    });
  });
});
