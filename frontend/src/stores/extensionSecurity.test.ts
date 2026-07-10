/**
 * Resource isolation tests (G-SEC-12 requirement 5).
 *
 * Verifies the helpers that block extension contexts from accessing
 * appState.aiApiKey and other internal state:
 *   - isExtensionContext() detects Worker / sandboxed-iframe contexts.
 *   - getAiApiKeyForContext() returns "" in extension contexts.
 *   - assertNotExtensionContext() throws in extension contexts.
 *
 * These are the defense-in-depth guards layered on top of the sandbox
 * (Web Worker / sandboxed iframe with sandbox="allow-scripts"). Even if
 * an extension obtains a reference to appState, the API surface uses
 * these guards as a choke point.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock @/stores/app so the store module loads without the full app state.
// The mock factory is hoisted by vitest, so it must not reference top-level
// variables — we inline the object and read it back via the import below.
vi.mock("@/stores/app", () => ({
  appState: { aiApiKey: "sk-secret-key-12345" },
}));

import {
  isExtensionContext,
  getAiApiKeyForContext,
  assertNotExtensionContext,
  resetExtensionSecurityStore,
} from "@/stores/extensionSecurity";

describe("Resource isolation (G-SEC-12 req. 5) — main webview context", () => {
  beforeEach(() => {
    resetExtensionSecurityStore();
  });

  it("isExtensionContext returns false in the main webview", () => {
    // jsdom is the main window context.
    expect(isExtensionContext()).toBe(false);
  });

  it("getAiApiKeyForContext returns the key in the main webview", () => {
    expect(getAiApiKeyForContext()).toBe("sk-secret-key-12345");
  });

  it("assertNotExtensionContext does not throw in the main webview", () => {
    expect(() => assertNotExtensionContext("test-op")).not.toThrow();
  });
});

describe("Resource isolation (G-SEC-12 req. 5) — extension context simulation", () => {
  const originalWindow = globalThis.window;

  afterEach(() => {
    // Restore the main-webview context after each test.
    (globalThis as unknown as { window: typeof window }).window = originalWindow;
    delete (globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean })
      .__GUGACODE_EXTENSION_CONTEXT__;
  });

  it("isExtensionContext returns true when the extension-context marker is set", () => {
    (globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean })
      .__GUGACODE_EXTENSION_CONTEXT__ = true;
    expect(isExtensionContext()).toBe(true);
  });

  it("getAiApiKeyForContext returns empty string in extension context", () => {
    (globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean })
      .__GUGACODE_EXTENSION_CONTEXT__ = true;
    // G-SEC-12 req. 5: extensions cannot access appState.aiApiKey.
    expect(getAiApiKeyForContext()).toBe("");
  });

  it("assertNotExtensionContext throws in extension context", () => {
    (globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean })
      .__GUGACODE_EXTENSION_CONTEXT__ = true;
    expect(() => assertNotExtensionContext("read-ai-key")).toThrow(
      /blocked in extension contexts/,
    );
  });

  it("getAiApiKeyForContext returns the key again after the marker is cleared", () => {
    (globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean })
      .__GUGACODE_EXTENSION_CONTEXT__ = true;
    expect(getAiApiKeyForContext()).toBe("");
    delete (globalThis as unknown as { __GUGACODE_EXTENSION_CONTEXT__?: boolean })
      .__GUGACODE_EXTENSION_CONTEXT__;
    expect(getAiApiKeyForContext()).toBe("sk-secret-key-12345");
  });
});

describe("Resource isolation — sandboxed iframe detection", () => {
  const originalWindow = globalThis.window;

  afterEach(() => {
    (globalThis as unknown as { window: typeof window }).window = originalWindow;
  });

  it("isExtensionContext returns true when window.parent !== window (sandboxed iframe)", () => {
    // Simulate a sandboxed iframe: window exists but window.parent is a
    // different object (cross-origin due to sandbox="allow-scripts" without
    // allow-same-origin).
    const fakeParent = {} as typeof window;
    (globalThis as unknown as { window: typeof window }).window = {
      ...originalWindow,
      parent: fakeParent,
    } as unknown as typeof window;
    expect(isExtensionContext()).toBe(true);
  });
});
