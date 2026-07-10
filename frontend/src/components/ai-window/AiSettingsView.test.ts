import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import AiSettingsView from "./AiSettingsView.vue";

vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock("@/stores/app", () => ({
  appState: {
    openAIWindowOnStartup: false,
    aiWindowTheme: "apple-dark",
    aiSidebarWidth: 288,
    aiTerminalWidth: 440,
  },
  saveSettings: vi.fn(),
}));

vi.mock("@/api/services", () => ({
  windowService: {
    isAIAlwaysOnTop: vi.fn().mockResolvedValue(true),
    setAIAlwaysOnTop: vi.fn().mockResolvedValue(undefined),
  },
}));

const sectionNames = [
  "AiSection", "AgentSection", "PersonaSection", "ModelPermissionSection",
  "McpSection", "SkillsSection", "PromptsSection", "PresetsSection",
  "ComputerUseSection", "DiffSection", "RollbackSection", "ImSection",
  "PersonalizationSection", "AiWindowThemePicker",
];

const stubs = Object.fromEntries(sectionNames.map((name) => [name, {
  template: `<div data-stub="${name}" />`,
}]));

describe("AiSettingsView", () => {
  it("groups models, context, execution, integrations, and window settings", async () => {
    const wrapper = mount(AiSettingsView, { global: { stubs } });
    expect(wrapper.find('[data-stub="AiSection"]').exists()).toBe(true);

    await wrapper.get('[data-group="context"]').trigger("click");
    expect(wrapper.find('[data-stub="McpSection"]').exists()).toBe(true);
    expect(wrapper.find('[data-stub="SkillsSection"]').exists()).toBe(true);

    await wrapper.get('[data-group="execution"]').trigger("click");
    expect(wrapper.find('[data-stub="ComputerUseSection"]').exists()).toBe(true);

    await wrapper.get('[data-group="integrations"]').trigger("click");
    expect(wrapper.find('[data-stub="ImSection"]').exists()).toBe(true);

    await wrapper.get('[data-group="window"]').trigger("click");
    expect(wrapper.find('[data-stub="AiWindowThemePicker"]').exists()).toBe(true);
  });
});
