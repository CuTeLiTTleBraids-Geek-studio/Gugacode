import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount } from "@vue/test-utils";
import type { App } from "vue";
import ElementPlus from "element-plus";
import type {
  ExtensionDetail,
  ExtensionSearchResult,
  InstalledExtension,
} from "@/types";

// ============================================================================
// vi.hoisted: 在 vi.mock 工厂提升执行前定义 mock 函数与数据。
// vi.mock 调用会被提升到文件顶部，早于普通 const 声明，
// 因此用 vi.hoisted 避免"暂时性死区"错误。
// ============================================================================
const {
  mockSearchResults,
  mockDetail,
  mockInstalled,
  searchExtensionsMock,
  getExtensionDetailMock,
  downloadAndInstallExtensionMock,
  uninstallExtensionMock,
  setExtensionEnabledMock,
  listInstalledExtensionsMock,
  requestEnableExtensionMock,
} = vi.hoisted(() => {
  // --- 真实合理的搜索结果数据（符合 ExtensionSearchResult 类型）---
  const searchResults: ExtensionSearchResult[] = [
    {
      id: "ms-python.python",
      name: "python",
      displayName: "Python",
      publisher: "ms-python",
      description: "IntelliSense, linting, debugging for Python",
      version: "2024.0.1",
      rating: 4.5,
      ratingCount: 1000,
      downloadCount: 50_000_000,
      iconUrl: "https://open-vsx.org/api/ms-python/python/icon",
    },
    {
      id: "golang.go",
      name: "Go",
      displayName: "Go",
      publisher: "golang",
      description: "Rich Go language support for Visual Studio Code",
      version: "0.41.0",
      rating: 4.2,
      ratingCount: 500,
      downloadCount: 10_000_000,
      iconUrl: "https://open-vsx.org/api/golang/go/icon",
    },
  ];

  // --- 真实合理的详情数据（符合 ExtensionDetail 类型）---
  const detail: ExtensionDetail = {
    id: "ms-python.python",
    name: "python",
    displayName: "Python",
    publisher: "ms-python",
    description: "IntelliSense, linting, debugging for Python",
    version: "2024.0.1",
    rating: 4.5,
    ratingCount: 1000,
    downloadCount: 50_000_000,
    iconUrl: "https://open-vsx.org/api/ms-python/python/icon",
    categories: ["Programming Languages", "Debuggers", "Linters"],
    tags: ["python", "linting", "debugging"],
    license: "MIT",
    repository: "https://github.com/microsoft/vscode-python",
    readme: "# Python Extension\n\nRich language support for Python.",
    versions: [
      {
        version: "2024.0.1",
        downloadUrl:
          "https://open-vsx.org/api/ms-python/python/2024.0.1/file/ms-python-python.vsix",
        date: "2024-01-15",
      },
      {
        version: "2023.22.0",
        downloadUrl:
          "https://open-vsx.org/api/ms-python/python/2023.22.0/file/ms-python-python.vsix",
        date: "2023-11-01",
      },
    ],
  };

  // --- 真实合理的已安装扩展数据（符合 InstalledExtension 类型）---
  const installed: InstalledExtension[] = [
    {
      publisher: "rust-lang",
      name: "rust-analyzer",
      version: "0.4.0",
      path: "/home/user/.gugacode/extensions/rust-lang-rust-analyzer",
      enabled: true,
    },
    {
      publisher: "dbaeumer",
      name: "vscode-eslint",
      version: "3.0.10",
      path: "/home/user/.gugacode/extensions/dbaeumer-vscode-eslint",
      enabled: false,
    },
  ];

  return {
    mockSearchResults: searchResults,
    mockDetail: detail,
    mockInstalled: installed,
    searchExtensionsMock: vi.fn(),
    getExtensionDetailMock: vi.fn(),
    downloadAndInstallExtensionMock: vi.fn(),
    uninstallExtensionMock: vi.fn(),
    setExtensionEnabledMock: vi.fn(),
    listInstalledExtensionsMock: vi.fn(),
    requestEnableExtensionMock: vi.fn(),
  };
});

// ============================================================================
// Mock 声明
// ============================================================================

// mock @/api/services: 提供 marketplaceService 的全部方法
vi.mock("@/api/services", () => ({
  marketplaceService: {
    searchExtensions: searchExtensionsMock,
    getExtensionDetail: getExtensionDetailMock,
    downloadAndInstallExtension: downloadAndInstallExtensionMock,
    uninstallExtension: uninstallExtensionMock,
    setExtensionEnabled: setExtensionEnabledMock,
    listInstalledExtensions: listInstalledExtensionsMock,
  },
}));

// mock @/stores/extensionSecurity: 提供 requestEnableExtension（安全审批入口）
vi.mock("@/stores/extensionSecurity", () => ({
  requestEnableExtension: requestEnableExtensionMock,
}));

// mock @/lib/errors: 提供 errorMessage
vi.mock("@/lib/errors", () => ({
  errorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}));

// mock @/lib/i18n: 提供 useI18n（t 函数直接返回 key 本身，便于断言）
vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, _params?: Record<string, string | number>) => key,
    locale: { value: "en" },
  }),
}));

// Element Plus 图标占位插件（测试中无需真实图标渲染）
const iconPlugin = {
  install(_app: App) {
    // no-op
  },
};

// ============================================================================
// 动态导入组件（确保 mock 在组件加载前就位）
// ============================================================================
const PanelModule = await import("./MarketplacePanel.vue");
const MarketplacePanel = PanelModule.default;

// ============================================================================
// 辅助函数
// ============================================================================

/** 挂载 MarketplacePanel */
function mountPanel() {
  return mount(MarketplacePanel, {
    global: {
      plugins: [ElementPlus, iconPlugin],
    },
  });
}

/** 刷新微任务队列，等待异步操作（setup / watch / promise 回调）完成 */
async function flush() {
  await new Promise((r) => setTimeout(r, 10));
}

/** 执行搜索：输入关键词并点击搜索按钮 */
async function doSearch(wrapper: ReturnType<typeof mountPanel>, query: string) {
  await wrapper.find(".marketplace__search input").setValue(query);
  await wrapper.find(".marketplace__search .el-button--primary").trigger("click");
  await flush();
}

/** 切换到"已安装"标签页（第二个 .marketplace__tab） */
async function switchToInstalled(wrapper: ReturnType<typeof mountPanel>) {
  const tabs = wrapper.findAll(".marketplace__tab");
  await tabs[1].trigger("click");
  await flush();
}

// ============================================================================
// 测试用例
// ============================================================================

describe("MarketplacePanel (G-VSC-01)", () => {
  beforeEach(() => {
    // 清除调用记录（保留实现），然后重置默认返回值
    vi.clearAllMocks();
    searchExtensionsMock.mockResolvedValue([...mockSearchResults]);
    getExtensionDetailMock.mockResolvedValue({ ...mockDetail });
    downloadAndInstallExtensionMock.mockResolvedValue(undefined);
    uninstallExtensionMock.mockResolvedValue(undefined);
    setExtensionEnabledMock.mockResolvedValue(undefined);
    listInstalledExtensionsMock.mockResolvedValue([...mockInstalled]);
    requestEnableExtensionMock.mockResolvedValue(true);
  });

  // --- 渲染初始状态 ---

  it("挂载时渲染安全横幅、搜索栏、标签页，并加载已安装列表", async () => {
    const wrapper = mountPanel();
    await flush();

    // 安全横幅（G-SEC-12）
    expect(wrapper.find(".marketplace__security").exists()).toBe(true);
    // 搜索栏
    expect(wrapper.find(".marketplace__search").exists()).toBe(true);
    // 两个标签：搜索结果 + 已安装
    const tabs = wrapper.findAll(".marketplace__tab");
    expect(tabs).toHaveLength(2);
    // 默认处于搜索视图
    expect(wrapper.find(".marketplace__results").exists()).toBe(true);
    // onMounted 调用 listInstalledExtensions
    expect(listInstalledExtensionsMock).toHaveBeenCalledTimes(1);
    // 已安装标签显示数量 (2)
    expect(tabs[1].text()).toContain("2");
  });

  it("未搜索时显示搜索提示", async () => {
    const wrapper = mountPanel();
    await flush();

    const empty = wrapper.find(".marketplace__empty");
    expect(empty.exists()).toBe(true);
    // t("marketplace.searchPrompt") → 返回 key 本身
    expect(empty.text()).toContain("marketplace.searchPrompt");
  });

  // --- 搜索扩展 ---

  it("输入关键词并点击搜索，调用 searchExtensions 并渲染结果列表", async () => {
    const wrapper = mountPanel();
    await flush();

    await doSearch(wrapper, "python");

    // 验证调用参数：query, page=1, pageSize=30
    expect(searchExtensionsMock).toHaveBeenCalledWith("python", 1, 30);
    // 渲染了两条搜索结果
    const items = wrapper.findAll(".marketplace__results .marketplace__item");
    expect(items).toHaveLength(2);
    // 第一条结果包含 displayName
    expect(items[0].text()).toContain("Python");
  });

  it("搜索关键词为空时不调用 searchExtensions，并清空结果", async () => {
    const wrapper = mountPanel();
    await flush();

    // 先执行一次有效搜索
    await doSearch(wrapper, "python");
    expect(searchExtensionsMock).toHaveBeenCalledTimes(1);

    // 再用空关键词搜索
    await doSearch(wrapper, "");
    // searchExtensions 不应被再次调用
    expect(searchExtensionsMock).toHaveBeenCalledTimes(1);
    // 结果列表已清空
    expect(wrapper.find(".marketplace__results .marketplace__list").exists()).toBe(false);
  });

  it("搜索失败时显示无结果提示", async () => {
    searchExtensionsMock.mockRejectedValue(new Error("network error"));
    const wrapper = mountPanel();
    await flush();

    await doSearch(wrapper, "python");

    // 结果为空，显示无结果提示（hasSearched=true → noResults）
    const empty = wrapper.find(".marketplace__empty");
    expect(empty.exists()).toBe(true);
    expect(empty.text()).toContain("marketplace.noResults");
  });

  // --- 标签切换 ---

  it("点击已安装标签切换到已安装列表视图", async () => {
    const wrapper = mountPanel();
    await flush();

    await switchToInstalled(wrapper);

    expect(wrapper.find(".marketplace__installed").exists()).toBe(true);
    expect(wrapper.find(".marketplace__results").exists()).toBe(false);
    // 渲染了两条已安装扩展
    const items = wrapper.findAll(".marketplace__installed .marketplace__item");
    expect(items).toHaveLength(2);
  });

  it("从已安装标签切换回搜索结果标签", async () => {
    const wrapper = mountPanel();
    await flush();

    // 先切到已安装
    await switchToInstalled(wrapper);
    expect(wrapper.find(".marketplace__installed").exists()).toBe(true);

    // 再切回搜索（第一个标签）
    const tabs = wrapper.findAll(".marketplace__tab");
    await tabs[0].trigger("click");
    await flush();

    expect(wrapper.find(".marketplace__results").exists()).toBe(true);
    expect(wrapper.find(".marketplace__installed").exists()).toBe(false);
  });

  // --- 安装扩展 ---

  it("从搜索结果点击安装按钮，调用 downloadAndInstallExtension 并刷新已安装列表", async () => {
    const wrapper = mountPanel();
    await flush();

    await doSearch(wrapper, "python");

    // 点击第一条结果的安装按钮
    const installBtn = wrapper.find(
      ".marketplace__results .marketplace__item-actions .el-button--primary",
    );
    await installBtn.trigger("click");
    await flush();

    // 验证安装调用参数
    expect(downloadAndInstallExtensionMock).toHaveBeenCalledWith(
      "ms-python",
      "python",
      "2024.0.1",
    );
    // 安装后刷新已安装列表（onMounted 一次 + 安装后一次 = 两次）
    expect(listInstalledExtensionsMock).toHaveBeenCalledTimes(2);
  });

  it("从详情页点击安装按钮，调用 downloadAndInstallExtension", async () => {
    const wrapper = mountPanel();
    await flush();

    await doSearch(wrapper, "python");

    // 点击第一条搜索结果打开详情
    await wrapper.find(".marketplace__results .marketplace__item-main").trigger("click");
    await flush();

    expect(wrapper.find(".marketplace__detail").exists()).toBe(true);
    expect(getExtensionDetailMock).toHaveBeenCalledWith("ms-python", "python");

    // 点击详情页安装按钮
    const detailInstallBtn = wrapper.find(
      ".marketplace__detail-header .el-button--primary",
    );
    await detailInstallBtn.trigger("click");
    await flush();

    expect(downloadAndInstallExtensionMock).toHaveBeenCalledWith(
      "ms-python",
      "python",
      "2024.0.1",
    );
  });

  it("已安装的扩展在搜索结果中显示已安装标签而非安装按钮", async () => {
    // 让已安装列表包含搜索结果中的第一条 (ms-python.python)
    listInstalledExtensionsMock.mockResolvedValue([
      {
        publisher: "ms-python",
        name: "python",
        version: "2024.0.1",
        path: "/home/user/.gugacode/extensions/ms-python-python",
        enabled: false,
      },
    ]);
    const wrapper = mountPanel();
    await flush();

    await doSearch(wrapper, "python");

    // 第一条结果不显示安装按钮，而是显示已安装标签
    const firstItem = wrapper.findAll(".marketplace__results .marketplace__item")[0];
    expect(firstItem.find(".el-button--primary").exists()).toBe(false);
    expect(firstItem.find(".el-tag").exists()).toBe(true);
  });

  // --- 卸载扩展 ---

  it("在已安装列表点击卸载按钮，调用 uninstallExtension 并刷新列表", async () => {
    const wrapper = mountPanel();
    await flush();

    await switchToInstalled(wrapper);

    // 点击第一个已安装扩展的卸载按钮（rust-lang.rust-analyzer）
    const uninstallBtn = wrapper.find(".marketplace__installed .el-button--danger");
    await uninstallBtn.trigger("click");
    await flush();

    expect(uninstallExtensionMock).toHaveBeenCalledWith("rust-lang", "rust-analyzer");
    // 卸载后刷新已安装列表（onMounted 一次 + 卸载后一次 = 两次）
    expect(listInstalledExtensionsMock).toHaveBeenCalledTimes(2);
  });

  // --- 启用/禁用切换（安全审批 G-SEC-12） ---

  it("启用已禁用的扩展时，调用 requestEnableExtension 进行安全审批", async () => {
    const wrapper = mountPanel();
    await flush();

    await switchToInstalled(wrapper);

    // 第二个扩展 dbaeumer.vscode-eslint 当前为禁用状态
    // 通过 el-switch 的 change 事件模拟用户开启
    const switches = wrapper.findAllComponents({ name: "ElSwitch" });
    expect(switches).toHaveLength(2);
    switches[1].vm.$emit("change", true);
    await flush();

    // 验证安全审批调用
    expect(requestEnableExtensionMock).toHaveBeenCalledWith("dbaeumer.vscode-eslint");
    // 审批通过后扩展被启用（is-disabled 类移除）
    const items = wrapper.findAll(".marketplace__installed .marketplace__item");
    expect(items[1].classes()).not.toContain("is-disabled");
  });

  it("安全审批拒绝时，扩展保持禁用状态", async () => {
    requestEnableExtensionMock.mockResolvedValue(false);
    const wrapper = mountPanel();
    await flush();

    await switchToInstalled(wrapper);

    const switches = wrapper.findAllComponents({ name: "ElSwitch" });
    switches[1].vm.$emit("change", true);
    await flush();

    // 审批被调用但返回 false
    expect(requestEnableExtensionMock).toHaveBeenCalledWith("dbaeumer.vscode-eslint");
    // 扩展仍为禁用状态
    const items = wrapper.findAll(".marketplace__installed .marketplace__item");
    expect(items[1].classes()).toContain("is-disabled");
  });

  it("禁用已启用的扩展时，调用 setExtensionEnabled", async () => {
    const wrapper = mountPanel();
    await flush();

    await switchToInstalled(wrapper);

    // 第一个扩展 rust-lang.rust-analyzer 当前为启用状态
    const switches = wrapper.findAllComponents({ name: "ElSwitch" });
    switches[0].vm.$emit("change", false);
    await flush();

    // 验证禁用调用参数
    expect(setExtensionEnabledMock).toHaveBeenCalledWith(
      "rust-lang",
      "rust-analyzer",
      false,
    );
    // 扩展被禁用（is-disabled 类添加）
    const items = wrapper.findAll(".marketplace__installed .marketplace__item");
    expect(items[0].classes()).toContain("is-disabled");
  });

  // --- 详情视图 ---

  it("点击搜索结果打开详情，点击返回关闭详情", async () => {
    const wrapper = mountPanel();
    await flush();

    await doSearch(wrapper, "python");

    // 打开详情
    await wrapper.find(".marketplace__results .marketplace__item-main").trigger("click");
    await flush();

    expect(wrapper.find(".marketplace__detail").exists()).toBe(true);
    expect(wrapper.find(".marketplace__detail-name").text()).toContain("Python");

    // 点击返回
    await wrapper.find(".marketplace__back").trigger("click");
    expect(wrapper.find(".marketplace__detail").exists()).toBe(false);
  });
});
