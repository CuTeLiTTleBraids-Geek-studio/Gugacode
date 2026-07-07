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
  settingsService: {
    loadSettings: vi.fn().mockResolvedValue({}),
    saveSettings: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn() },
}));

import { appState, applyMode, resolveSystemMode } from "./app";

describe("Theme Mode", () => {
  beforeEach(() => {
    document.documentElement.removeAttribute("data-mode");
    appState.theme = "dark";
    appState.accentTheme = "blue";
  });

  it("resolveSystemMode returns 'dark' or 'light'", () => {
    const mode = resolveSystemMode();
    expect(["dark", "light"]).toContain(mode);
  });

  it("applyMode('dark') sets data-mode to dark", () => {
    applyMode("dark");
    expect(document.documentElement.getAttribute("data-mode")).toBe("dark");
  });

  it("applyMode('light') sets data-mode to light", () => {
    applyMode("light");
    expect(document.documentElement.getAttribute("data-mode")).toBe("light");
  });

  it("applyMode('system') sets data-mode to resolved system mode", () => {
    applyMode("system");
    const resolved = resolveSystemMode();
    expect(document.documentElement.getAttribute("data-mode")).toBe(resolved);
  });

  it("applyMode updates appState.theme", () => {
    applyMode("light");
    expect(appState.theme).toBe("light");
  });
});
