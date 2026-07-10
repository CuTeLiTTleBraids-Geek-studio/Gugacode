import { shallowMount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import GeneralSection from "./GeneralSection.vue";

vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

describe("GeneralSection", () => {
  it("does not render AI-window startup settings", () => {
    const wrapper = shallowMount(GeneralSection);
    expect(wrapper.text()).not.toContain("general.openAIWindowOnStartup");
  });
});
