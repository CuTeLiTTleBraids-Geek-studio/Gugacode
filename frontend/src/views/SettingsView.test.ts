import { shallowMount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import SettingsView from "./SettingsView.vue";

vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

describe("SettingsView", () => {
  it("keeps only editor-owned settings destinations", () => {
    const wrapper = shallowMount(SettingsView);
    const labels = wrapper.findAll(".settings-nav-btn").map((button) => button.attributes("aria-label"));
    expect(labels).toEqual([
      "settings.general",
      "settings.editor",
      "settings.terminal",
      "settings.shortcuts",
      "settings.appearance",
      "settings.profiles",
    ]);
  });
});
