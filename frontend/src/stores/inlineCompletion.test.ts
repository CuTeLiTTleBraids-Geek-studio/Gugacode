import { describe, it, expect, beforeEach, vi } from "vitest";

vi.mock("@/lib/monaco-themes", () => ({
  accentThemes: {
    blue: { label: "Blue", color: "#4285f4", monacoTheme: "nknk-blue", monacoLightTheme: "nknk-light-blue" },
  },
  applyMonacoTheme: vi.fn(),
  applyMonacoThemeForMode: vi.fn(),
  registerAllThemes: vi.fn(),
}));

vi.mock("@/api/services", () => ({
  aiService: {
    complete: vi.fn(),
  },
  settingsService: {
    loadSettings: vi.fn(),
    saveSettings: vi.fn(),
  },
}));

vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn() },
}));

import { appState } from "./app";
import {
  inlineCompletionEnabled,
  requestCompletion,
  toggleInlineCompletion,
  cancelInlineCompletion,
  __resetInlineCompletionForTesting,
} from "./inlineCompletion";
import { aiService } from "@/api/services";

describe("inlineCompletion store (N-7)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    appState.inlineCompletionEnabled = true;
    // N-43: Reset module-level debounce + in-flight state between tests.
    __resetInlineCompletionForTesting();
  });

  describe("inlineCompletionEnabled", () => {
    it("reflects appState.inlineCompletionEnabled", () => {
      appState.inlineCompletionEnabled = true;
      expect(inlineCompletionEnabled.value).toBe(true);
      appState.inlineCompletionEnabled = false;
      expect(inlineCompletionEnabled.value).toBe(false);
    });
  });

  describe("toggleInlineCompletion", () => {
    it("toggles from true to false", () => {
      appState.inlineCompletionEnabled = true;
      toggleInlineCompletion();
      expect(appState.inlineCompletionEnabled).toBe(false);
    });

    it("toggles from false to true", () => {
      appState.inlineCompletionEnabled = false;
      toggleInlineCompletion();
      expect(appState.inlineCompletionEnabled).toBe(true);
    });
  });

  describe("requestCompletion", () => {
    it("returns empty string when disabled", async () => {
      appState.inlineCompletionEnabled = false;
      const result = await requestCompletion("a".repeat(20), "", "ts", "f.ts");
      expect(result).toBe("");
      expect(aiService.complete).not.toHaveBeenCalled();
    });

    it("returns empty string when prefix is too short", async () => {
      appState.inlineCompletionEnabled = true;
      const result = await requestCompletion("short", "", "ts", "f.ts");
      expect(result).toBe("");
      expect(aiService.complete).not.toHaveBeenCalled();
    });

    it("returns completion text when enabled with sufficient prefix", async () => {
      appState.inlineCompletionEnabled = true;
      (aiService.complete as any).mockResolvedValue({ text: "completion" });
      const result = await requestCompletion("a".repeat(20), "suffix", "ts", "f.ts");
      expect(result).toBe("completion");
      expect(aiService.complete).toHaveBeenCalledWith(
        { prefix: "a".repeat(20), suffix: "suffix", language: "ts", filePath: "f.ts" },
        expect.any(AbortSignal),
      );
    });

    it("returns empty string on API error", async () => {
      appState.inlineCompletionEnabled = true;
      (aiService.complete as any).mockRejectedValue(new Error("network"));
      const result = await requestCompletion("a".repeat(20), "", "ts", "f.ts");
      expect(result).toBe("");
    });

    it("returns empty string when response has no text", async () => {
      appState.inlineCompletionEnabled = true;
      (aiService.complete as any).mockResolvedValue({ text: "" });
      const result = await requestCompletion("a".repeat(20), "", "ts", "f.ts");
      expect(result).toBe("");
    });

    it("returns empty string when response is null", async () => {
      appState.inlineCompletionEnabled = true;
      (aiService.complete as any).mockResolvedValue(null);
      const result = await requestCompletion("a".repeat(20), "", "ts", "f.ts");
      expect(result).toBe("");
    });
  });

  // --- N-43: AbortController + dedup + per-file debounce ---

  describe("N-43: per-file debounce", () => {
    it("does not block requests for different files", async () => {
      (aiService.complete as any).mockResolvedValue({ text: "" });
      // Fire two requests for different files within the debounce window.
      await requestCompletion("a".repeat(20), "", "ts", "file-a.ts");
      await requestCompletion("b".repeat(20), "", "ts", "file-b.ts");
      // Both should have been sent (no debounce blocking).
      expect(aiService.complete).toHaveBeenCalledTimes(2);
    });

    it("blocks rapid consecutive requests for the same file", async () => {
      (aiService.complete as any).mockResolvedValue({ text: "" });
      // Fire two requests for the same file within the debounce window.
      // The first must be awaited so its in-flight state clears before the
      // second starts (otherwise the dedup path would apply, not debounce).
      await requestCompletion("a".repeat(20), "", "ts", "same.ts");
      await requestCompletion("b".repeat(20), "", "ts", "same.ts");
      // Only the first should have been sent (debounce blocks the second).
      expect(aiService.complete).toHaveBeenCalledTimes(1);
    });

    it("allows a new request for the same file after the debounce window", async () => {
      (aiService.complete as any).mockResolvedValue({ text: "" });
      // First request — uses real Date.now() to set the debounce timestamp.
      await requestCompletion("a".repeat(20), "", "ts", "debounce.ts");
      // Advance real time past the debounce window (300ms).
      await new Promise((r) => setTimeout(r, 310));
      // Second request for the same file — should pass debounce.
      await requestCompletion("b".repeat(20), "", "ts", "debounce.ts");
      expect(aiService.complete).toHaveBeenCalledTimes(2);
    });
  });

  describe("N-43: AbortController cancellation", () => {
    it("aborts the previous in-flight request when a new one starts", async () => {
      // First request: stays in-flight (controlled resolution).
      let resolveFirst: (v: { text: string }) => void = () => {};
      let firstAborted = false;
      (aiService.complete as any).mockImplementationOnce(
        (_req: unknown, signal: AbortSignal) =>
          new Promise((resolve, reject) => {
            resolveFirst = resolve;
            signal.addEventListener("abort", () => {
              firstAborted = true;
              reject(new DOMException("aborted", "AbortError"));
            });
          }),
      );
      // Second request: resolves immediately. Use a DIFFERENT file path so
      // the debounce check doesn't block it.
      (aiService.complete as any).mockResolvedValueOnce({ text: "second" });

      // Start the first request (don't await yet).
      const firstPromise = requestCompletion("a".repeat(20), "", "ts", "abort-first.ts");
      // Give the microtask queue a tick so the first request registers.
      await Promise.resolve();
      // Start the second request with a different file — this should abort
      // the first (different signature → abortInFlight is called).
      const secondResult = await requestCompletion("b".repeat(20), "", "ts", "abort-second.ts");
      // The second request should complete normally.
      expect(secondResult).toBe("second");
      // The first request should resolve to "" (aborted → caught → empty).
      const firstResult = await firstPromise;
      expect(firstResult).toBe("");
      expect(firstAborted).toBe(true);
      // Clean up the first request's resolver (in case it hasn't fired).
      resolveFirst({ text: "stale" });
    });

    it("cancelInlineCompletion aborts the in-flight request", async () => {
      let aborted = false;
      (aiService.complete as any).mockImplementationOnce(
        (_req: unknown, signal: AbortSignal) =>
          new Promise((_resolve, reject) => {
            signal.addEventListener("abort", () => {
              aborted = true;
              reject(new DOMException("aborted", "AbortError"));
            });
          }),
      );
      const promise = requestCompletion("a".repeat(20), "", "ts", "cancel.ts");
      await Promise.resolve();
      cancelInlineCompletion();
      const result = await promise;
      expect(result).toBe("");
      expect(aborted).toBe(true);
    });
  });

  describe("N-43: dedup concurrent calls with same signature", () => {
    it("reuses the in-flight promise for identical concurrent requests", async () => {
      let resolveReq: (v: { text: string }) => void = () => {};
      (aiService.complete as any).mockImplementationOnce(
        () => new Promise((resolve) => { resolveReq = resolve; }),
      );
      // Start the first request (don't await — it stays in-flight).
      const firstPromise = requestCompletion("a".repeat(20), "suf", "ts", "dedup.ts");
      // Immediately start a second request with the same signature.
      // The dedup check should reuse the in-flight request. Note: because
      // requestCompletion is `async`, each call returns a new wrapper Promise
      // regardless of dedup — so dedup is verified by the underlying HTTP
      // call count, not by promise identity.
      const secondPromise = requestCompletion("a".repeat(20), "suf", "ts", "dedup.ts");
      // Only one HTTP request should have been fired (deduped).
      expect(aiService.complete).toHaveBeenCalledTimes(1);
      // Resolve the request.
      resolveReq({ text: "shared" });
      const [first, second] = await Promise.all([firstPromise, secondPromise]);
      expect(first).toBe("shared");
      expect(second).toBe("shared");
    });

    it("does not dedup requests with different signatures", async () => {
      (aiService.complete as any).mockResolvedValue({ text: "" });
      // Use different file paths so the debounce doesn't block the second
      // AND the signatures differ.
      await requestCompletion("a".repeat(20), "", "ts", "dedup-a.ts");
      await requestCompletion("a".repeat(20), "", "ts", "dedup-b.ts");
      expect(aiService.complete).toHaveBeenCalledTimes(2);
    });
  });
});
