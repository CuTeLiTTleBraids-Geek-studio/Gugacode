import { flushPromises, mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn(), Emit: vi.fn() },
}));

vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
  translate: (key: string) => key,
}));

vi.mock("@/api/services", () => ({
  settingsService: { loadSettings: vi.fn(), saveSettings: vi.fn() },
  conversationService: { updateTitle: vi.fn() },
  windowService: {
    isAIAlwaysOnTop: vi.fn().mockResolvedValue(true),
    setAIAlwaysOnTop: vi.fn(),
    openPathInExplorer: vi.fn(),
    openPathInVSCode: vi.fn(),
  },
  projectService: { getRecentProjects: vi.fn().mockResolvedValue([]) },
}));

vi.mock("@/stores/mcp", () => ({ agentMcpTools: [], refreshAgentMcpTools: vi.fn() }));
vi.mock("@/stores/skills", () => ({ skillsList: [], loadSkills: vi.fn() }));
vi.mock("@/stores/snapshot", () => ({ setSnapshotWorkspaceRoot: vi.fn(), listSnapshots: vi.fn() }));
vi.mock("@/lib/notifications", () => ({ notifyError: vi.fn(), notifySuccess: vi.fn(), notifyWarning: vi.fn() }));
vi.mock("@/components/layout/TerminalPanel.vue", () => ({ default: { template: "<div />" } }));

import { aiWindowState } from "@/stores/aiWindow";
import { appState } from "@/stores/app";
import AiWindowView from "./AiWindowView.vue";

const sidebarStub = {
  props: ["activeView", "width", "terminalVisible"],
  emits: ["select-view", "select-conversation", "toggle-terminal", "resize", "resize-commit"],
  template: `<aside data-test="workspace-sidebar">
    <button data-test="open-settings" @click="$emit('select-view', 'settings')" />
    <button data-test="toggle-terminal" @click="$emit('toggle-terminal')" />
  </aside>`,
};

const dockStub = {
  props: ["visible", "width", "maxWidth"],
  emits: ["close", "resize", "resize-commit"],
  template: '<aside v-if="visible" data-test="terminal-dock" />',
};

describe("AiWindowView workspace shell", () => {
  beforeEach(() => {
    aiWindowState.activeView = "assistant";
    aiWindowState.terminalVisible = false;
  });

  it("keeps the sidebar mounted while views change and docks terminal on the right", async () => {
    const wrapper = mount(AiWindowView, {
      global: {
        stubs: {
          AiWorkspaceSidebar: sidebarStub,
          AiTerminalDock: dockStub,
          MessageList: true,
          InputComposer: true,
          AiSkillsView: true,
          AiAutomationView: true,
          AiSettingsView: true,
          SnapshotTimeline: true,
          "el-icon": { template: "<span><slot /></span>" },
        },
      },
    });
    await flushPromises();

    expect(wrapper.find('[data-test="workspace-sidebar"]').exists()).toBe(true);
    await wrapper.get('[data-test="open-settings"]').trigger("click");
    expect(aiWindowState.activeView).toBe("settings");
    expect(wrapper.find('[data-test="workspace-sidebar"]').exists()).toBe(true);

    await wrapper.get('[data-test="toggle-terminal"]').trigger("click");
    expect(wrapper.find('[data-test="terminal-dock"]').exists()).toBe(true);
    expect(aiWindowState.activeView).toBe("settings");
  });

  it("keeps the AI theme independent when editor theme state changes", async () => {
    appState.aiWindowTheme = "claude-dark";
    aiWindowState.theme = "claude-dark";
    const wrapper = mount(AiWindowView, {
      global: {
        stubs: {
          AiWorkspaceSidebar: sidebarStub,
          AiTerminalDock: dockStub,
          MessageList: true,
          InputComposer: true,
          AiSkillsView: true,
          AiAutomationView: true,
          AiSettingsView: true,
          SnapshotTimeline: true,
          "el-icon": { template: "<span><slot /></span>" },
        },
      },
    });
    await flushPromises();
    appState.theme = "light";
    await flushPromises();

    expect(document.documentElement.getAttribute("data-mode")).toBe("dark");
    expect(document.documentElement.getAttribute("data-design-language")).toBe("claude");
    wrapper.unmount();
  });
});
