import { describe, it, expect, beforeEach, vi } from "vitest";
import { mount } from "@vue/test-utils";
import { nextTick, type App } from "vue";
import ElementPlus from "element-plus";

// 用 vi.hoisted 定义 mock 引用：vi.mock 调用会被提升到文件顶部，
// 早于任何 const 声明，因此普通顶层 const 会落入暂时性死区。
// 这里把所有需要在 mock 工厂中引用、又在测试中断言的变量集中定义。
const {
  // ---- 响应式状态的原始对象（在 mock 工厂中用 reactive 包装）----
  appStateObj,
  terminalStateObj,
  outputStateObj,
  taskStateObj,
  workflowStateObj,
  // ---- @/stores/terminal 的动作 mock ----
  createSessionMock,
  writeToSessionMock,
  killSessionMock,
  resizeSessionMock,
  clearSessionOutputMock,
  // ---- @/stores/app 的动作 mock ----
  toggleTerminalMock,
  // ---- @/stores/output 的动作 mock ----
  clearOutputsMock,
  clearProblemsMock,
  problemCountsMock,
  // ---- @/stores/tasks 的动作 mock ----
  loadTasksMock,
  runTaskMock,
  composeCommandLineMock,
  // ---- @/stores/workflows 的动作 mock ----
  loadWorkflowsMock,
  runWorkflowMock,
  composeStepCommandLineMock,
  // ---- @/stores/editor 的动作 mock ----
  openFileFromPathMock,
  // ---- xterm onData 回调捕获（用于测试输入转发）----
  onDataCallbacks,
  // ---- 会话 id 计数器 ----
  sessionCounter,
} = vi.hoisted(() => ({
  // appState：组件读取 terminalVisible / terminalHeight / currentProject /
  // terminalFontSize / theme / bottomPanelView 等字段
  appStateObj: {
    terminalVisible: true,
    terminalHeight: 220,
    currentProject: "/proj",
    terminalFontSize: 13,
    theme: "dark",
    bottomPanelView: "" as string,
  },
  // terminalState：会话字典、顺序列表、当前激活会话
  terminalStateObj: {
    sessions: {} as Record<
      string,
      { id: string; output: string; running: boolean; cols: number; rows: number }
    >,
    sessionOrder: [] as string[],
    activeSessionId: null as string | null,
  },
  // outputState：输出条目与问题条目
  outputStateObj: {
    outputs: [] as unknown[],
    problems: [] as unknown[],
  },
  // taskState：任务列表与加载状态
  taskStateObj: {
    tasks: [] as unknown[],
    loading: false,
    errorMessage: null as string | null,
  },
  // workflowState：工作流列表与执行状态
  workflowStateObj: {
    workflows: [] as unknown[],
    loading: false,
    errorMessage: null as string | null,
    running: {} as Record<string, boolean>,
    stepStates: {} as Record<string, unknown[]>,
  },

  // ---- 动作 mock：默认返回已解决的 Promise，避免 onMounted 链路报错 ----
  createSessionMock: vi.fn(),
  writeToSessionMock: vi.fn().mockResolvedValue(undefined),
  killSessionMock: vi.fn(),
  resizeSessionMock: vi.fn().mockResolvedValue(undefined),
  clearSessionOutputMock: vi.fn(),
  toggleTerminalMock: vi.fn(),
  clearOutputsMock: vi.fn(),
  clearProblemsMock: vi.fn(),
  problemCountsMock: vi.fn().mockReturnValue({
    error: 0,
    warning: 0,
    info: 0,
    hint: 0,
  }),
  loadTasksMock: vi.fn().mockResolvedValue(undefined),
  runTaskMock: vi.fn().mockResolvedValue(undefined),
  composeCommandLineMock: vi.fn().mockImplementation((task: { command?: string }) =>
    task?.command ?? "echo hi",
  ),
  loadWorkflowsMock: vi.fn().mockResolvedValue(undefined),
  runWorkflowMock: vi.fn().mockResolvedValue(undefined),
  composeStepCommandLineMock: vi.fn().mockImplementation((step: { command?: string }) =>
    step?.command ?? "echo step",
  ),
  openFileFromPathMock: vi.fn().mockResolvedValue(undefined),

  // xterm onData 回调列表：每次 term.onData(cb) 被调用时收集 cb，
  // 测试可通过 onDataCallbacks[0]("ls\n") 模拟用户输入。
  onDataCallbacks: [] as Array<(data: string) => void>,
  // 会话 id 自增计数器
  sessionCounter: { value: 0 },
}));

// --- 双保险 mock：同时 mock store 与 service，确保任何代码路径都不会触达真实实现 ---

vi.mock("@/stores/app", async () => {
  const { reactive } = await import("vue");
  const appState = reactive(appStateObj);
  // toggleTerminal 翻转 terminalVisible，驱动 v-if="isVisible" 响应式更新
  toggleTerminalMock.mockImplementation(() => {
    appState.terminalVisible = !appState.terminalVisible;
  });
  return {
    appState,
    toggleTerminal: toggleTerminalMock,
  };
});

vi.mock("@/stores/terminal", async () => {
  const { reactive } = await import("vue");
  const terminalState = reactive(terminalStateObj);
  // createSession 的默认实现：生成新会话并更新响应式状态，
  // 模拟真实 store 中 createSession 对 terminalState 的副作用。
  createSessionMock.mockImplementation(async (_workingDir: string, _shell: string) => {
    const id = `term-${++sessionCounter.value}`;
    terminalState.sessions[id] = {
      id,
      output: "",
      running: true,
      cols: 80,
      rows: 24,
    };
    terminalState.sessionOrder.push(id);
    terminalState.activeSessionId = id;
    return id;
  });
  // killSession 的默认实现：删除会话并修正激活会话
  killSessionMock.mockImplementation(async (sessionId: string) => {
    delete terminalState.sessions[sessionId];
    terminalState.sessionOrder = terminalState.sessionOrder.filter(
      (id) => id !== sessionId,
    );
    if (terminalState.activeSessionId === sessionId) {
      terminalState.activeSessionId = terminalState.sessionOrder[0] ?? null;
    }
  });
  return {
    terminalState,
    createSession: createSessionMock,
    writeToSession: writeToSessionMock,
    killSession: killSessionMock,
    resizeSession: resizeSessionMock,
    clearSessionOutput: clearSessionOutputMock,
  };
});

vi.mock("@/stores/output", async () => {
  const { reactive } = await import("vue");
  const outputState = reactive(outputStateObj);
  return {
    outputState,
    clearOutputs: clearOutputsMock,
    clearProblems: clearProblemsMock,
    problemCounts: problemCountsMock,
  };
});

vi.mock("@/stores/tasks", async () => {
  const { reactive, computed } = await import("vue");
  const taskState = reactive(taskStateObj);
  // hasTasks 必须是真正的 computed ref，模板中 v-if="hasTasks" 才会自动解包
  const hasTasks = computed(() => taskState.tasks.length > 0);
  return {
    taskState,
    loadTasks: loadTasksMock,
    runTask: runTaskMock,
    composeCommandLine: composeCommandLineMock,
    hasTasks,
  };
});

vi.mock("@/stores/workflows", async () => {
  const { reactive, computed } = await import("vue");
  const workflowState = reactive(workflowStateObj);
  // hasWorkflows 必须是真正的 computed ref
  const hasWorkflows = computed(() => workflowState.workflows.length > 0);
  return {
    workflowState,
    loadWorkflows: loadWorkflowsMock,
    runWorkflow: runWorkflowMock,
    composeStepCommandLine: composeStepCommandLineMock,
    hasWorkflows,
  };
});

vi.mock("@/stores/editor", () => ({
  openFileFromPath: openFileFromPathMock,
}));

// mock @/api/services：提供组件可能经 store 间接触达的全部 service 方法
vi.mock("@/api/services", () => ({
  terminalService: {
    start: vi.fn().mockResolvedValue(undefined),
    write: vi.fn().mockResolvedValue(undefined),
    kill: vi.fn().mockResolvedValue(undefined),
    isRunning: vi.fn().mockReturnValue(false),
    resize: vi.fn().mockResolvedValue(undefined),
    startSession: vi.fn().mockResolvedValue(undefined),
    killSession: vi.fn().mockResolvedValue(undefined),
    writeSession: vi.fn().mockResolvedValue(undefined),
    resizeSession: vi.fn().mockResolvedValue(undefined),
    isSessionRunning: vi.fn().mockReturnValue(false),
    listSessions: vi.fn().mockReturnValue([]),
  },
  taskService: {
    loadTasks: vi.fn().mockResolvedValue([]),
  },
  workflowService: {
    loadWorkflows: vi.fn().mockResolvedValue([]),
    validateAllWorkflows: vi.fn().mockResolvedValue([]),
  },
  fileService: {
    readFile: vi.fn().mockResolvedValue(""),
  },
  settingsService: {},
  windowService: {},
}));

// mock i18n：提供 t 函数，覆盖模板中用到的 key
vi.mock("@/lib/i18n", () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const map: Record<string, string> = {
        "terminal.terminalTab": "Terminal",
        "terminal.outputTab": "Output",
        "terminal.problemsTab": "Problems",
        "terminal.tasksTab": "Tasks",
        "terminal.workflowsTab": "Workflows",
        "terminal.noOutput": "No output",
        "terminal.noProblems": "No problems",
        "terminal.loadingTasks": "Loading tasks...",
        "terminal.loadingWorkflows": "Loading workflows...",
      };
      if (key === "terminal.terminalLabel") {
        return `Terminal ${params?.n ?? 1}`;
      }
      return map[key] ?? key;
    },
    locale: { value: "en" },
  }),
}));

// mock @xterm/xterm：jsdom 中无法运行真实终端，提供 Terminal 类的桩实现。
// onData 回调被收集到 onDataCallbacks，测试可借此模拟用户输入。
vi.mock("@xterm/xterm", () => ({
  Terminal: class MockTerminal {
    options: Record<string, unknown> = {};
    constructor(_opts?: unknown) {}
    loadAddon(_addon: unknown) {}
    open(_el: HTMLElement) {}
    onData(cb: (data: string) => void) {
      onDataCallbacks.push(cb);
    }
    onResize(_cb: unknown) {}
    focus() {}
    write(_data: string) {}
    dispose() {}
  },
}));

// mock @xterm/addon-fit：提供 FitAddon 类的桩实现
vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class MockFitAddon {
    fit() {}
  },
}));

// ElementPlus 图标在测试中无需渲染，安装一个空插件即可。
const iconPlugin = {
  install(_app: App) {},
};

// 在所有 mock 设置完成后再动态导入被测组件
const TerminalPanelModule = await import("./TerminalPanel.vue");
const TerminalPanel = TerminalPanelModule.default;

// 导入响应式状态（已是被 mock 的响应式代理），用于在测试中读写
const { appState } = await import("@/stores/app");
const { terminalState } = await import("@/stores/terminal");
const { outputState } = await import("@/stores/output");
const { taskState } = await import("@/stores/tasks");
const { workflowState } = await import("@/stores/workflows");

const flushPromises = () => new Promise((resolve) => setTimeout(resolve, 0));

function mountTerminalPanel() {
  return mount(TerminalPanel, {
    global: {
      plugins: [ElementPlus, iconPlugin],
    },
  });
}

// 重置响应式状态并重新建立默认 mock 实现，
// 确保每个用例互不影响（即使前一个用例覆盖了实现）。
function resetStateAndMocks() {
  // ---- 重置 appState ----
  appState.terminalVisible = true;
  appState.terminalHeight = 220;
  appState.currentProject = "/proj";
  appState.terminalFontSize = 13;
  appState.theme = "dark";
  appState.bottomPanelView = "";

  // ---- 重置 terminalState ----
  Object.keys(terminalState.sessions).forEach((id) => delete terminalState.sessions[id]);
  terminalState.sessionOrder = [];
  terminalState.activeSessionId = null;

  // ---- 重置 outputState ----
  outputState.outputs = [];
  outputState.problems = [];

  // ---- 重置 taskState / workflowState ----
  taskState.tasks = [];
  taskState.loading = false;
  taskState.errorMessage = null;
  workflowState.workflows = [];
  workflowState.loading = false;
  workflowState.errorMessage = null;
  workflowState.running = {};
  workflowState.stepStates = {};

  // ---- 重置 xterm 回调与计数器 ----
  onDataCallbacks.length = 0;
  sessionCounter.value = 0;

  // ---- 清除调用记录并重新建立默认实现 ----
  vi.clearAllMocks();

  // createSession / killSession / toggleTerminal 需要操作响应式状态，
  // 在此重新绑定以防止被个别用例覆盖后影响后续用例。
  createSessionMock.mockImplementation(async (_workingDir: string, _shell: string) => {
    const id = `term-${++sessionCounter.value}`;
    terminalState.sessions[id] = {
      id,
      output: "",
      running: true,
      cols: 80,
      rows: 24,
    };
    terminalState.sessionOrder.push(id);
    terminalState.activeSessionId = id;
    return id;
  });
  killSessionMock.mockImplementation(async (sessionId: string) => {
    delete terminalState.sessions[sessionId];
    terminalState.sessionOrder = terminalState.sessionOrder.filter(
      (id) => id !== sessionId,
    );
    if (terminalState.activeSessionId === sessionId) {
      terminalState.activeSessionId = terminalState.sessionOrder[0] ?? null;
    }
  });
  toggleTerminalMock.mockImplementation(() => {
    appState.terminalVisible = !appState.terminalVisible;
  });

  // 其余动作恢复默认返回值
  writeToSessionMock.mockResolvedValue(undefined);
  resizeSessionMock.mockResolvedValue(undefined);
  loadTasksMock.mockResolvedValue(undefined);
  loadWorkflowsMock.mockResolvedValue(undefined);
  runTaskMock.mockResolvedValue(undefined);
  runWorkflowMock.mockResolvedValue(undefined);
  openFileFromPathMock.mockResolvedValue(undefined);
  problemCountsMock.mockReturnValue({ error: 0, warning: 0, info: 0, hint: 0 });
  composeCommandLineMock.mockImplementation((task: { command?: string }) =>
    task?.command ?? "echo hi",
  );
  composeStepCommandLineMock.mockImplementation((step: { command?: string }) =>
    step?.command ?? "echo step",
  );
}

describe("TerminalPanel", () => {
  beforeEach(() => {
    resetStateAndMocks();
  });

  it("可见时渲染终端面板及视图标签", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();

    // 面板根节点存在
    expect(wrapper.find(".terminal-panel").exists()).toBe(true);
    // 5 个视图标签：terminal / output / problems / tasks / workflows
    const viewTabs = wrapper.findAll(".terminal-panel__view-tab");
    expect(viewTabs.length).toBe(5);
    // 默认激活 terminal 视图
    expect(wrapper.text()).toContain("Terminal");
  });

  it("不可见时不渲染面板", async () => {
    appState.terminalVisible = false;
    const wrapper = mountTerminalPanel();
    await flushPromises();

    // v-if="isVisible" 为 false 时面板不渲染
    expect(wrapper.find(".terminal-panel").exists()).toBe(false);
    // 且不会创建终端会话
    expect(createSessionMock).not.toHaveBeenCalled();
  });

  it("挂载时创建首个终端会话", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();

    // onMounted 中 initFirstSession 调用 createSession(currentProject, "")
    expect(createSessionMock).toHaveBeenCalledWith("/proj", "");
    expect(terminalState.sessionOrder.length).toBe(1);
    // 终端会话标签栏显示 1 个会话标签
    const tabs = wrapper.findAll(".terminal-panel__tab");
    expect(tabs.length).toBe(1);
  });

  it("点击新建终端按钮创建新会话", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    // 挂载后已有 1 个会话
    expect(terminalState.sessionOrder.length).toBe(1);
    vi.clearAllMocks();

    // 点击 "+" 新建按钮
    const newBtn = wrapper.find(".terminal-panel__new");
    await newBtn.trigger("click");
    await flushPromises();

    // createSession 被调用，会话数增至 2
    expect(createSessionMock).toHaveBeenCalledWith("/proj", "");
    expect(terminalState.sessionOrder.length).toBe(2);
    // 新标签出现
    expect(wrapper.findAll(".terminal-panel__tab").length).toBe(2);
  });

  it("点击会话标签切换 activeSessionId", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    // 挂载后只有 1 个会话，手动追加第二个会话用于切换
    const id2 = "term-manual";
    terminalState.sessions[id2] = {
      id: id2,
      output: "",
      running: true,
      cols: 80,
      rows: 24,
    };
    terminalState.sessionOrder.push(id2);
    await flushPromises();

    const firstId = terminalState.sessionOrder[0];
    expect(terminalState.activeSessionId).toBe(firstId);

    // 点击第二个会话标签
    const tabs = wrapper.findAll(".terminal-panel__tab");
    expect(tabs.length).toBe(2);
    await tabs[1].trigger("click");
    await flushPromises();

    // activeSessionId 切换到第二个
    expect(terminalState.activeSessionId).toBe(id2);
  });

  it("点击会话关闭按钮调用 killSession 并移除会话", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    expect(terminalState.sessionOrder.length).toBe(1);
    const sessionId = terminalState.sessionOrder[0];
    vi.clearAllMocks();

    // 点击会话标签上的关闭图标
    const closeIcon = wrapper.find(".terminal-panel__tab-close");
    await closeIcon.trigger("click");
    await flushPromises();

    // killSession 被调用，会话被移除
    expect(killSessionMock).toHaveBeenCalledWith(sessionId);
    expect(terminalState.sessionOrder.length).toBe(0);
    expect(terminalState.activeSessionId).toBeNull();
  });

  it("终端 onData 回调将输入转发到 writeToSession", async () => {
    mountTerminalPanel();
    await flushPromises();

    // onMounted 初始化首个终端后应已注册 onData 回调
    expect(onDataCallbacks.length).toBeGreaterThan(0);
    const sessionId = terminalState.activeSessionId!;
    vi.clearAllMocks();

    // 模拟用户在终端输入 "ls\n"
    onDataCallbacks[0]("ls\n");
    await flushPromises();

    // 输入应转发到 writeToSession(sessionId, data)
    expect(writeToSessionMock).toHaveBeenCalledWith(sessionId, "ls\n");
  });

  it("点击关闭面板按钮触发 toggleTerminal 隐藏面板", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    expect(appState.terminalVisible).toBe(true);

    const closeBtn = wrapper.find(".terminal-panel__close");
    await closeBtn.trigger("click");

    // toggleTerminal 被调用，terminalVisible 翻转为 false
    expect(toggleTerminalMock).toHaveBeenCalled();
    expect(appState.terminalVisible).toBe(false);
  });

  it("切换到 tasks 视图触发 loadTasks", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    // onMounted 已调用过 loadTasks，清除后精确断言切换行为
    vi.clearAllMocks();
    loadTasksMock.mockResolvedValue(undefined);

    const viewTabs = wrapper.findAll(".terminal-panel__view-tab");
    const tasksTab = viewTabs.find((t) => t.text().includes("Tasks"));
    expect(tasksTab).toBeTruthy();
    await tasksTab!.trigger("click");
    await flushPromises();

    // 切换到 tasks 视图时 watch(activeView) 调用 loadTasks(currentProject)
    expect(loadTasksMock).toHaveBeenCalledWith("/proj");
  });

  it("切换到 workflows 视图触发 loadWorkflows", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    vi.clearAllMocks();
    loadWorkflowsMock.mockResolvedValue(undefined);

    const viewTabs = wrapper.findAll(".terminal-panel__view-tab");
    const workflowsTab = viewTabs.find((t) => t.text().includes("Workflows"));
    expect(workflowsTab).toBeTruthy();
    await workflowsTab!.trigger("click");
    await flushPromises();

    expect(loadWorkflowsMock).toHaveBeenCalledWith("/proj");
  });

  it("output 视图无输出时显示空状态", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();

    const viewTabs = wrapper.findAll(".terminal-panel__view-tab");
    const outputTab = viewTabs.find((t) => t.text().includes("Output"));
    await outputTab!.trigger("click");
    await nextTick();

    // 无输出时渲染空状态节点
    expect(wrapper.find(".terminal-panel__empty").exists()).toBe(true);
    expect(wrapper.text()).toContain("No output");
  });

  it("卸载组件时清理 xterm 实例（不抛错）", async () => {
    const wrapper = mountTerminalPanel();
    await flushPromises();
    // 卸载应触发 onBeforeUnmount 清理终端实例，不应抛出异常
    expect(() => wrapper.unmount()).not.toThrow();
  });
});
