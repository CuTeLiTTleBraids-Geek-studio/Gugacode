import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount } from "@vue/test-utils";
import type { App } from "vue";
import ElementPlus from "element-plus";
import * as ElementPlusIconsVue from "@element-plus/icons-vue";

// Mock monaco-themes so importing @/lib/i18n -> @/stores/app doesn't pull
// in the real Monaco editor (which calls document.queryCommandSupported,
// unavailable in jsdom). Same pattern as app.test.ts.
vi.mock("@/lib/monaco-themes", () => ({
  accentThemes: {
    blue: { label: "Blue", color: "#4285f4", monacoTheme: "nknk-blue", monacoLightTheme: "nknk-light-blue" },
  },
  applyMonacoTheme: vi.fn(),
  applyMonacoThemeForMode: vi.fn(),
  registerAllThemes: vi.fn(),
  registerCustomTheme: vi.fn(),
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

import TabBar from "./TabBar.vue";
import { editorState, openFile, updateContent } from "@/stores/editor";

const iconPlugin = {
  install(app: App) {
    for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
      app.component(key, component);
    }
  },
};

function mountBar() {
  return mount(TabBar, {
    global: {
      plugins: [ElementPlus, iconPlugin],
    },
  });
}

describe("TabBar", () => {
  beforeEach(() => {
    editorState.openFiles = [];
    editorState.activeFilePath = null;
  });

  it("renders nothing when there are no open files", () => {
    const wrapper = mountBar();
    expect(wrapper.find(".tab-bar").exists()).toBe(false);
  });

  it("renders a tab for each open file", () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    const wrapper = mountBar();
    expect(wrapper.findAll(".tab-bar__tab")).toHaveLength(2);
  });

  it("displays the file name in each tab", () => {
    openFile("/src/app.ts", "x");
    const wrapper = mountBar();
    expect(wrapper.find(".tab-bar__name").text()).toBe("app.ts");
  });

  it("applies active class to the active tab", () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    const wrapper = mountBar();
    const tabs = wrapper.findAll(".tab-bar__tab");
    expect(tabs[0].classes()).not.toContain("tab-bar__tab--active");
    expect(tabs[1].classes()).toContain("tab-bar__tab--active");
  });

  it("shows dirty indicator when file is dirty", () => {
    openFile("/src/a.ts", "original");
    updateContent("/src/a.ts", "changed");
    const wrapper = mountBar();
    expect(wrapper.find(".tab-bar__dirty").exists()).toBe(true);
  });

  it("does not show dirty indicator when file is clean", () => {
    openFile("/src/a.ts", "original");
    const wrapper = mountBar();
    expect(wrapper.find(".tab-bar__dirty").exists()).toBe(false);
  });

  it("emits select with the path when a tab is clicked", async () => {
    openFile("/src/a.ts", "a");
    openFile("/src/b.ts", "b");
    const wrapper = mountBar();
    const tabs = wrapper.findAll(".tab-bar__tab");
    await tabs[0].trigger("click");
    const selectEvents = wrapper.emitted("select");
    expect(selectEvents).toBeTruthy();
    expect(selectEvents![0]).toEqual(["/src/a.ts"]);
  });

  it("emits close with the path when the close button is clicked", async () => {
    openFile("/src/a.ts", "a");
    const wrapper = mountBar();
    await wrapper.find(".tab-bar__close").trigger("click");
    const closeEvents = wrapper.emitted("close");
    expect(closeEvents).toBeTruthy();
    expect(closeEvents![0]).toEqual(["/src/a.ts"]);
  });

  it("does not emit select when the close button is clicked", async () => {
    openFile("/src/a.ts", "a");
    const wrapper = mountBar();
    await wrapper.find(".tab-bar__close").trigger("click");
    expect(wrapper.emitted("select")).toBeFalsy();
  });
});
