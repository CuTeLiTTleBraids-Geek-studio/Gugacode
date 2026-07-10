import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { nextTick, type App } from "vue";
import ElementPlus from "element-plus";

// 用 vi.hoisted 定义 mock 引用：vi.mock 调用会被提升到文件顶部，
// 早于任何 const 声明，因此普通顶层 const 会落入暂时性死区。
// 这里把所有需要在 mock 工厂中引用、又在测试中断言的变量集中定义。
const {
  mockAppState,
  gitState,
  branchState,
  conflictState,
  rebaseState,
  reviewState,
  refreshGitMock,
  loadBranchesMock,
  stageFileMock,
  unstageFileMock,
  commitChangesMock,
  pushChangesMock,
  pullChangesMock,
  createBranchMock,
  checkoutBranchMock,
  loadConflictsMock,
  resolveConflictAsOursMock,
  resolveConflictAsTheirsMock,
  markConflictResolvedMock,
  startRebaseMock,
  abortRebaseMock,
  continueRebaseMock,
  checkRebaseStatusMock,
  generateGitignoreMock,
  clearConflictStateMock,
  openFileFromPathMock,
  runReviewMock,
  clearReviewMock,
  renderMarkdownMock,
} = vi.hoisted(() => {
  // 所有异步 store 动作默认返回已解决的 Promise，
  // 避免 onMounted 中 `checkRebaseStatus().then(...)` 对 undefined 调用 .then 报错。
  const resolved = () => vi.fn().mockResolvedValue(undefined);
  return {
    // appState：组件通过 appState.currentProject 读取仓库路径
    mockAppState: { currentProject: "/proj" },

    // gitState：变更列表、分支名、ahead/behind、loading、error
    gitState: {
      changes: [] as Array<{ path: string; status: string }>,
      branchName: "main",
      ahead: 0,
      behind: 0,
      loading: false,
      error: null as string | null,
    },

    // branchState：分支列表，第一个为 HEAD
    branchState: {
      branches: [
        { name: "main", isHead: true },
        { name: "dev", isHead: false },
      ],
      loadingBranches: false,
    },

    // conflictState / rebaseState：默认无冲突、无进行中的 rebase
    conflictState: {
      conflicts: [] as Array<{ file: string; ours: string; theirs: string; base: string }>,
      loading: false,
      error: null as string | null,
    },
    rebaseState: {
      inProgress: false,
      loading: false,
      error: null as string | null,
      lastOutput: "",
    },

    // reviewState：默认无审查结果
    reviewState: {
      result: null as string | null,
      loading: false,
      error: null as string | null,
      reviewedFiles: [] as string[],
      reviewedAt: null as number | null,
    },

    // ---- @/stores/git 的动作 mock ----
    refreshGitMock: resolved(),
    loadBranchesMock: resolved(),
    stageFileMock: resolved(),
    unstageFileMock: resolved(),
    commitChangesMock: resolved(),
    pushChangesMock: resolved(),
    pullChangesMock: resolved(),
    createBranchMock: resolved(),
    checkoutBranchMock: resolved(),
    loadConflictsMock: resolved(),
    resolveConflictAsOursMock: resolved(),
    resolveConflictAsTheirsMock: resolved(),
    markConflictResolvedMock: resolved(),
    startRebaseMock: resolved(),
    abortRebaseMock: resolved(),
    continueRebaseMock: resolved(),
    checkRebaseStatusMock: resolved(),
    generateGitignoreMock: resolved(),
    clearConflictStateMock: vi.fn(),

    // ---- @/stores/editor ----
    openFileFromPathMock: resolved(),

    // ---- @/stores/review ----
    runReviewMock: resolved(),
    clearReviewMock: vi.fn(),

    // ---- @/lib/markdown ----
    renderMarkdownMock: vi.fn((content: string) => `<div>${content ?? ""}</div>`),
  };
});

// --- 双保险 mock：同时 mock store 与 service，确保任何代码路径都不会触达真实实现 ---

vi.mock("@/stores/app", () => ({
  appState: mockAppState,
}));

vi.mock("@/stores/git", () => ({
  gitState,
  branchState,
  conflictState,
  rebaseState,
  refreshGit: refreshGitMock,
  loadBranches: loadBranchesMock,
  stageFile: stageFileMock,
  unstageFile: unstageFileMock,
  commitChanges: commitChangesMock,
  pushChanges: pushChangesMock,
  pullChanges: pullChangesMock,
  createBranch: createBranchMock,
  checkoutBranch: checkoutBranchMock,
  loadConflicts: loadConflictsMock,
  resolveConflictAsOurs: resolveConflictAsOursMock,
  resolveConflictAsTheirs: resolveConflictAsTheirsMock,
  markConflictResolved: markConflictResolvedMock,
  startRebase: startRebaseMock,
  abortRebase: abortRebaseMock,
  continueRebase: continueRebaseMock,
  checkRebaseStatus: checkRebaseStatusMock,
  generateGitignore: generateGitignoreMock,
  clearConflictState: clearConflictStateMock,
}));

vi.mock("@/stores/editor", () => ({
  openFileFromPath: openFileFromPathMock,
}));

// hasReview 必须是真正的 ref，模板中 v-else-if="hasReview" 才会自动解包为 .value；
// 若用普通对象会被判定为 truthy。这里用动态导入 vue 创建 ref(false)。
vi.mock("@/stores/review", async () => {
  const { ref } = await import("vue");
  return {
    reviewState,
    hasReview: ref(false),
    runReview: runReviewMock,
    clearReview: clearReviewMock,
  };
});

vi.mock("@/api/services", () => ({
  gitService: {
    getStatus: vi.fn(),
    getBranchInfo: vi.fn(),
    stage: vi.fn(),
    unstage: vi.fn(),
    commit: vi.fn(),
    push: vi.fn(),
    pull: vi.fn(),
    listBranches: vi.fn(),
    createBranch: vi.fn(),
    checkoutBranch: vi.fn(),
    deleteBranch: vi.fn(),
    getDiff: vi.fn(),
    getFullDiff: vi.fn(),
    listMergeConflicts: vi.fn(),
    resolveConflict: vi.fn(),
    isRebaseInProgress: vi.fn(),
    rebase: vi.fn(),
    abortRebase: vi.fn(),
    continueRebase: vi.fn(),
    createGitignore: vi.fn(),
  },
  fileService: { readFile: vi.fn(), writeFile: vi.fn(), pickDirectory: vi.fn() },
  aiService: { getPresetPrompt: vi.fn(), send: vi.fn() },
  settingsService: {},
  windowService: {},
}));

vi.mock("@/lib/errors", () => ({
  errorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}));

// mock markdown 以规避 DOMPurify / highlight.js 的 DOM 副作用
vi.mock("@/lib/markdown", () => ({
  renderMarkdownWithApplyButtons: renderMarkdownMock,
}));

vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string) => {
      const map: Record<string, string> = {
        "git.stage": "Stage",
        "git.unstage": "Unstage",
        "git.diff": "Diff",
        "git.viewDiffAria": "View Diff",
        "git.commit": "Commit",
        "git.noChanges": "No changes",
        "git.review": "Review",
        "git.rerun": "Rerun",
        "git.reviewing": "Reviewing...",
        "git.noReviewYet": "No review yet",
        "git.gitignoreCreated": "Created",
        "git.gitignoreExists": "Exists",
        "common.loading": "Loading...",
        "common.retry": "Retry",
        "common.cancel": "Cancel",
      };
      return map[key] ?? key;
    },
    locale: { value: "en" },
  }),
}));

// mock DiffView 子组件，避免引入 Monaco 编辑器与真实 gitService
vi.mock("@/components/editor/DiffView.vue", () => ({
  default: {
    name: "DiffView",
    props: ["repoPath", "filePath", "visible"],
    emits: ["close"],
    template: '<div class="stub-diff-view" />',
  },
}));

// ElementPlus 图标在测试中无需渲染，安装一个空插件即可。
const iconPlugin = {
  install(_app: App) {},
};

// 在所有 mock 设置完成后再动态导入被测组件
const GitPanelModule = await import("./GitPanel.vue");
const GitPanel = GitPanelModule.default;

const flushPromises = () => new Promise((resolve) => setTimeout(resolve, 0));

// 真实合理的 GitFileChange 样本数据
const sampleChanges: Array<{ path: string; status: string }> = [
  { path: "src/main.ts", status: "Modified" },
  { path: "README.md", status: "Added" },
  { path: "old/deleted.go", status: "Deleted" },
];

function mountGitPanel() {
  return mount(GitPanel, {
    global: {
      plugins: [ElementPlus, iconPlugin],
    },
  });
}

function resetState() {
  mockAppState.currentProject = "/proj";
  gitState.changes = [];
  gitState.branchName = "main";
  gitState.ahead = 0;
  gitState.behind = 0;
  gitState.loading = false;
  gitState.error = null;
  branchState.branches = [
    { name: "main", isHead: true },
    { name: "dev", isHead: false },
  ];
  branchState.loadingBranches = false;
  conflictState.conflicts = [];
  conflictState.loading = false;
  conflictState.error = null;
  rebaseState.inProgress = false;
  rebaseState.loading = false;
  rebaseState.error = null;
  rebaseState.lastOutput = "";
  reviewState.result = null;
  reviewState.loading = false;
  reviewState.error = null;
  reviewState.reviewedFiles = [];
  reviewState.reviewedAt = null;
}

describe("GitPanel", () => {
  beforeEach(() => {
    resetState();
    vi.clearAllMocks();
    // 清除调用记录会重置 mockImplementation 吗？不会，clearAllMocks 只清调用记录。
    // 但需要保证默认 resolved 行为仍在：
    refreshGitMock.mockResolvedValue(undefined);
    loadBranchesMock.mockResolvedValue(undefined);
    stageFileMock.mockResolvedValue(undefined);
    unstageFileMock.mockResolvedValue(undefined);
    commitChangesMock.mockResolvedValue(undefined);
    pushChangesMock.mockResolvedValue(undefined);
    pullChangesMock.mockResolvedValue(undefined);
    checkRebaseStatusMock.mockResolvedValue(undefined);
    generateGitignoreMock.mockResolvedValue(undefined);
    runReviewMock.mockResolvedValue(undefined);
  });

  it("渲染初始状态，显示当前分支名", () => {
    const wrapper = mountGitPanel();
    // 分支栏应展示 HEAD 分支名 main
    expect(wrapper.find(".git-panel__branch-current").text()).toContain("main");
  });

  it("无变更时显示空状态", () => {
    gitState.changes = [];
    const wrapper = mountGitPanel();
    expect(wrapper.find(".git-panel__empty").exists()).toBe(true);
    expect(wrapper.text()).toContain("No changes");
  });

  it("渲染变更列表并显示状态标签", () => {
    gitState.changes = sampleChanges;
    const wrapper = mountGitPanel();
    const rows = wrapper.findAll(".git-panel__row");
    expect(rows).toHaveLength(3);
    expect(wrapper.text()).toContain("src/main.ts");
    expect(wrapper.text()).toContain("old/deleted.go");
  });

  it("点击暂存按钮调用 stageFile", async () => {
    gitState.changes = sampleChanges;
    const wrapper = mountGitPanel();
    // onMounted 已调用过若干 mock，清除后再触发交互以精确断言
    vi.clearAllMocks();
    stageFileMock.mockResolvedValue(undefined);

    const stageBtn = wrapper.find('button[aria-label="Stage"]');
    await stageBtn.trigger("click");
    await flushPromises();

    expect(stageFileMock).toHaveBeenCalledWith("/proj", "src/main.ts");
  });

  it("点击取消暂存按钮调用 unstageFile", async () => {
    gitState.changes = sampleChanges;
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    unstageFileMock.mockResolvedValue(undefined);

    const unstageBtn = wrapper.find('button[aria-label="Unstage"]');
    await unstageBtn.trigger("click");
    await flushPromises();

    expect(unstageFileMock).toHaveBeenCalledWith("/proj", "src/main.ts");
  });

  it("无提交信息时提交按钮被禁用", () => {
    const wrapper = mountGitPanel();
    const commitBtn = wrapper.find(".git-panel__commit-btn");
    expect(commitBtn.attributes("disabled")).toBeDefined();
  });

  it("输入提交信息并提交，调用 commitChanges 并清空输入框", async () => {
    gitState.changes = sampleChanges;
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    commitChangesMock.mockResolvedValue(undefined);

    const input = wrapper.find(".git-panel__commit-input");
    await input.setValue("feat: add new feature");
    const commitBtn = wrapper.find(".git-panel__commit-btn");
    expect(commitBtn.attributes("disabled")).toBeUndefined();

    await commitBtn.trigger("click");
    await flushPromises();

    expect(commitChangesMock).toHaveBeenCalledWith("/proj", "feat: add new feature");
    // 提交后输入框应被清空
    expect((input.element as HTMLTextAreaElement).value).toBe("");
  });

  it("点击刷新按钮调用 refreshGit 与 checkRebaseStatus", async () => {
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    refreshGitMock.mockResolvedValue(undefined);
    checkRebaseStatusMock.mockResolvedValue(undefined);

    await wrapper.find(".git-panel__refresh").trigger("click");
    await flushPromises();

    expect(refreshGitMock).toHaveBeenCalledWith("/proj");
    expect(checkRebaseStatusMock).toHaveBeenCalled();
    // 未处于 rebase 中，不应加载冲突
    expect(loadConflictsMock).not.toHaveBeenCalled();
  });

  it("点击 Diff 按钮打开 DiffView 并传入文件路径", async () => {
    gitState.changes = sampleChanges;
    const wrapper = mountGitPanel();
    const diffStub = wrapper.findComponent({ name: "DiffView" });
    expect(diffStub.props("visible")).toBe(false);

    await wrapper.find('button[aria-label="View Diff"]').trigger("click");
    await nextTick();

    expect(diffStub.props("visible")).toBe(true);
    expect(diffStub.props("filePath")).toBe("src/main.ts");
  });

  it(".gitignore 下拉菜单命令触发 generateGitignore 并刷新", async () => {
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    generateGitignoreMock.mockResolvedValue(undefined);
    refreshGitMock.mockResolvedValue(undefined);

    // 定位 .gitignore 触发按钮所在的 ElDropdown
    const dropdowns = wrapper.findAllComponents({ name: "ElDropdown" });
    const gitignoreDropdown = dropdowns.find((d) => d.text().includes(".gitignore"));
    expect(gitignoreDropdown).toBeTruthy();

    gitignoreDropdown!.vm.$emit("command", "typescript");
    await flushPromises();

    expect(generateGitignoreMock).toHaveBeenCalledWith("typescript");
    expect(refreshGitMock).toHaveBeenCalledWith("/proj");
  });

  it(".gitignore 已存在时走警告分支且不再调用 refreshGit", async () => {
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    generateGitignoreMock.mockRejectedValue(new Error(".gitignore already exists"));
    refreshGitMock.mockResolvedValue(undefined);

    const dropdowns = wrapper.findAllComponents({ name: "ElDropdown" });
    const gitignoreDropdown = dropdowns.find((d) => d.text().includes(".gitignore"));
    expect(gitignoreDropdown).toBeTruthy();

    gitignoreDropdown!.vm.$emit("command", "go");
    await flushPromises();

    expect(generateGitignoreMock).toHaveBeenCalledWith("go");
    // 错误分支不应继续调用 refreshGit
    expect(refreshGitMock).not.toHaveBeenCalled();
  });

  it("点击审查按钮打开模态框并触发 runReview", async () => {
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    runReviewMock.mockResolvedValue(undefined);

    await wrapper.find(".git-panel__review-btn").trigger("click");
    await nextTick();

    // 模态框应可见
    expect(wrapper.find(".review-modal").exists()).toBe(true);
    // 首次打开且无结果时应自动触发审查
    expect(runReviewMock).toHaveBeenCalledWith("/proj");
  });

  it("重新审查按钮调用 clearReview 与 runReview", async () => {
    const wrapper = mountGitPanel();
    // 先打开模态框
    await wrapper.find(".git-panel__review-btn").trigger("click");
    await nextTick();
    vi.clearAllMocks();
    runReviewMock.mockResolvedValue(undefined);

    const rerunBtn = wrapper.find(".review-modal__rerun");
    expect(rerunBtn.attributes("disabled")).toBeUndefined();
    await rerunBtn.trigger("click");
    await flushPromises();

    expect(clearReviewMock).toHaveBeenCalled();
    expect(runReviewMock).toHaveBeenCalledWith("/proj");
  });

  it("点击推送按钮调用 pushChanges", async () => {
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    pushChangesMock.mockResolvedValue(undefined);

    // 通过 title 定位推送按钮（推送按钮使用 Top 图标）
    const pushBtn = wrapper.find('button[title="git.pushTitle"]');
    await pushBtn.trigger("click");
    await flushPromises();

    expect(pushChangesMock).toHaveBeenCalledWith("/proj");
  });

  it("点击拉取按钮调用 pullChanges", async () => {
    const wrapper = mountGitPanel();
    vi.clearAllMocks();
    pullChangesMock.mockResolvedValue(undefined);

    const pullBtn = wrapper.find('button[title="git.pullTitle"]');
    await pullBtn.trigger("click");
    await flushPromises();

    expect(pullChangesMock).toHaveBeenCalledWith("/proj");
  });

  it("未设置项目时不触发刷新等动作", async () => {
    mockAppState.currentProject = "";
    const wrapper = mountGitPanel();
    vi.clearAllMocks();

    await wrapper.find(".git-panel__refresh").trigger("click");
    await flushPromises();

    // repoPath 为空时 handleRefresh 直接返回
    expect(refreshGitMock).not.toHaveBeenCalled();
    mockAppState.currentProject = "/proj";
  });
});
