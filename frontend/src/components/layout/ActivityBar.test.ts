import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  toggleAIWindow: vi.fn().mockResolvedValue(undefined),
  isAIWindowOpen: vi.fn().mockResolvedValue(true),
  isAIWindowVisible: vi.fn().mockResolvedValueOnce(true).mockResolvedValue(false),
  openAIWindow: vi.fn(),
}));

vi.mock("vue-router", () => ({
  useRoute: () => ({ path: "/editor" }),
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
  translate: (key: string) => key,
}));

vi.mock("@/api/services", () => ({
  windowService: {
    toggleAIWindow: mocks.toggleAIWindow,
    isAIWindowOpen: mocks.isAIWindowOpen,
    isAIWindowVisible: mocks.isAIWindowVisible,
  },
}));

vi.mock("@/stores/aiAssistant", () => ({
  openAIDesktopWindow: mocks.openAIWindow,
}));

vi.mock("@/lib/notifications", () => ({ notifyError: vi.fn() }));

import ActivityBar from "./ActivityBar.vue";

describe("ActivityBar AI window state", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mocks.toggleAIWindow.mockClear();
    mocks.isAIWindowOpen.mockClear();
    mocks.isAIWindowVisible.mockReset();
    mocks.isAIWindowVisible.mockResolvedValueOnce(true).mockResolvedValue(false);
    mocks.openAIWindow.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("clears the AI active state after toggling the window hidden", async () => {
    const wrapper = mount(ActivityBar, {
      global: {
        stubs: {
          "el-icon": { template: "<span><slot /></span>" },
          component: { template: "<span />" },
        },
      },
    });
    await flushPromises();

    const aiButton = wrapper.findAll("button")[4];
    expect(aiButton.classes()).toContain("activity-bar__item--active");

    await aiButton.trigger("click");
    await flushPromises();

    expect(mocks.toggleAIWindow).toHaveBeenCalledTimes(1);
    expect(aiButton.classes()).not.toContain("activity-bar__item--active");
    expect(mocks.openAIWindow).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
