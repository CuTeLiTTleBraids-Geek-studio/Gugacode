import { describe, it, expect, beforeEach, vi } from "vitest";

// 使用 vi.hoisted 提前定义 mock 对象，确保 vi.mock 工厂能安全引用。
// vi.mock 的工厂在模块加载前执行，只有 vi.hoisted 创建的变量可在工厂内使用。
const mocks = vi.hoisted(() => {
  return {
    lspService: {
      detectServers: vi.fn(),
      startServer: vi.fn(),
      stopServer: vi.fn(),
      getCompletions: vi.fn(),
      getHover: vi.fn(),
      getDiagnostics: vi.fn(),
    },
  };
});

// 核心 mock：拦截 @/api/services，让 store 内部调用走我们的 vi.fn。
vi.mock("@/api/services", () => ({
  lspService: mocks.lspService,
}));

// 避免加载真实 app store（其依赖 @wailsio/runtime、monaco 主题等副作用模块）。
// lsp.ts 仅 re-export appState，测试本身不依赖其行为，提供最小桩即可。
vi.mock("@/stores/app", () => ({
  appState: {},
}));

// 拦截 pushOutput，便于在错误处理用例中断言告警被记录。
vi.mock("@/stores/output", () => ({
  pushOutput: vi.fn(),
}));

import {
  lspState,
  anyLSPAvailable,
  anyLSPRunning,
  detectLSPServers,
  startLSPServer,
  stopLSPServer,
  ensureLSPRunning,
  monacoLanguageToLSP,
  getLSPCompletions,
  getLSPHover,
  stopAllLSPServers,
  initLSPStore,
  __resetLSPStoreForTesting,
} from "./lsp";
import { pushOutput } from "@/stores/output";

// 真实合理的 mock 数据：gopls 与 tsserver 已安装，javascript 未安装。
const goStatus = {
  language: "go",
  available: true,
  running: false,
  serverPath: "/usr/local/bin/gopls",
  version: "0.16.2",
};
const tsStatus = {
  language: "typescript",
  available: true,
  running: false,
  serverPath: "/usr/local/lib/node_modules/typescript/lib/tsserver.js",
  version: "5.4.5",
};
const jsStatus = {
  language: "javascript",
  available: false,
  running: false,
  serverPath: "",
  version: "",
};

const sampleCompletions = [
  {
    label: "fmt.Println",
    kind: 3,
    detail: "func(a ...any) (int, error)",
    insertText: "fmt.Println(${1:args})",
  },
  {
    label: "fmt.Printf",
    kind: 3,
    detail: "func(format string, a ...any) (int, error)",
    insertText: "fmt.Printf(${1:format}, ${2:args})",
  },
];

beforeEach(() => {
  // 每个用例前重置 store 状态与所有 mock 调用记录。
  __resetLSPStoreForTesting();
  vi.clearAllMocks();
  // 默认 mock 行为：检测到 go / typescript 已安装，javascript 未安装。
  // 注意：每次调用返回全新副本，避免被测代码对 statuses[x] 的写操作
  // （如 running=true）污染常量数据导致跨用例串扰。
  mocks.lspService.detectServers.mockImplementation(async () => [
    { ...goStatus },
    { ...tsStatus },
    { ...jsStatus },
  ]);
  mocks.lspService.startServer.mockResolvedValue(undefined);
  mocks.lspService.stopServer.mockResolvedValue(undefined);
  mocks.lspService.getCompletions.mockResolvedValue(sampleCompletions);
  mocks.lspService.getHover.mockResolvedValue("func fmt.Println(a ...any) (int, error)");
  mocks.lspService.getDiagnostics.mockResolvedValue([]);
});

describe("lsp store — 初始状态与 getters", () => {
  it("初始状态：statuses 为空、busy 为 false、enabled 为 true", () => {
    expect(lspState.statuses).toEqual({});
    expect(lspState.busy).toBe(false);
    expect(lspState.enabled).toBe(true);
  });

  it("无可用服务器时 anyLSPAvailable 与 anyLSPRunning 均为 false", () => {
    expect(anyLSPAvailable.value).toBe(false);
    expect(anyLSPRunning.value).toBe(false);
  });

  it("detectLSPServers 后 getters 能反映可用与运行状态", async () => {
    await detectLSPServers();
    // go / typescript 可用 -> anyLSPAvailable 为 true
    expect(anyLSPAvailable.value).toBe(true);
    // 均未运行 -> anyLSPRunning 为 false
    expect(anyLSPRunning.value).toBe(false);
  });
});

describe("lsp store — detectLSPServers", () => {
  it("调用 lspService.detectServers 并按 language 填充 statuses", async () => {
    await detectLSPServers();
    expect(mocks.lspService.detectServers).toHaveBeenCalledTimes(1);
    expect(Object.keys(lspState.statuses).sort()).toEqual(["go", "javascript", "typescript"]);
    expect(lspState.statuses["go"]).toEqual(goStatus);
    expect(lspState.statuses["typescript"].available).toBe(true);
    expect(lspState.statuses["javascript"].available).toBe(false);
  });

  it("执行期间 busy 为 true，完成后恢复为 false", async () => {
    // 在 mock 实现内部采样 busy，验证其被置位。
    let busyDuringCall: boolean | null = null;
    mocks.lspService.detectServers.mockImplementationOnce(async () => {
      busyDuringCall = lspState.busy;
      return [goStatus, tsStatus, jsStatus];
    });
    await detectLSPServers();
    expect(busyDuringCall).toBe(true);
    expect(lspState.busy).toBe(false);
  });

  it("detectServers 抛错时静默失败：记录告警、busy 复位、statuses 保持原样", async () => {
    // 源码在错误路径不清理 statuses（仅在成功路径先置空再填充），
    // 因此失败时应保留调用前的状态。
    const preExisting = { go: { ...goStatus, running: true } };
    lspState.statuses = { ...preExisting };
    mocks.lspService.detectServers.mockRejectedValueOnce(new Error("backend down"));
    await detectLSPServers();
    expect(pushOutput).toHaveBeenCalledWith(
      "ide",
      "warn",
      expect.stringContaining("LSP detect failed: backend down"),
    );
    expect(lspState.busy).toBe(false);
    expect(lspState.statuses).toEqual(preExisting);
  });
});

describe("lsp store — startLSPServer", () => {
  it("成功启动已安装但未运行的服务器：调用 service 并置 running=true，返回 true", async () => {
    await detectLSPServers();
    const ok = await startLSPServer("go");
    expect(ok).toBe(true);
    expect(mocks.lspService.startServer).toHaveBeenCalledWith("go");
    expect(lspState.statuses["go"].running).toBe(true);
    expect(lspState.busy).toBe(false);
  });

  it("服务器已运行时为 no-op：不调用 service 且直接返回 true", async () => {
    await detectLSPServers();
    lspState.statuses["go"].running = true;
    const ok = await startLSPServer("go");
    expect(ok).toBe(true);
    expect(mocks.lspService.startServer).not.toHaveBeenCalled();
  });

  it("服务器未安装时返回 false 且不调用 service", async () => {
    await detectLSPServers();
    const ok = await startLSPServer("javascript");
    expect(ok).toBe(false);
    expect(mocks.lspService.startServer).not.toHaveBeenCalled();
  });

  it("startServer 抛错时返回 false、记录告警、busy 复位", async () => {
    await detectLSPServers();
    mocks.lspService.startServer.mockRejectedValueOnce(new Error("port in use"));
    const ok = await startLSPServer("go");
    expect(ok).toBe(false);
    expect(pushOutput).toHaveBeenCalledWith(
      "ide",
      "warn",
      expect.stringContaining("LSP start go failed: port in use"),
    );
    expect(lspState.busy).toBe(false);
    expect(lspState.statuses["go"].running).toBe(false);
  });
});

describe("lsp store — stopLSPServer", () => {
  it("停止运行中的服务器：调用 service 并置 running=false", async () => {
    await detectLSPServers();
    lspState.statuses["go"].running = true;
    await stopLSPServer("go");
    expect(mocks.lspService.stopServer).toHaveBeenCalledWith("go");
    expect(lspState.statuses["go"].running).toBe(false);
  });

  it("未运行时为 no-op：不调用 service", async () => {
    await detectLSPServers();
    await stopLSPServer("go");
    expect(mocks.lspService.stopServer).not.toHaveBeenCalled();
  });

  it("stopServer 抛错时静默失败并记录告警", async () => {
    await detectLSPServers();
    lspState.statuses["go"].running = true;
    mocks.lspService.stopServer.mockRejectedValueOnce(new Error("timeout"));
    await stopLSPServer("go");
    expect(pushOutput).toHaveBeenCalledWith(
      "ide",
      "warn",
      expect.stringContaining("LSP stop go failed: timeout"),
    );
    // 出错路径不翻转 running，保持原状（仍为 true）。
    expect(lspState.statuses["go"].running).toBe(true);
  });
});

describe("lsp store — ensureLSPRunning", () => {
  it("已运行时直接返回 true 且不重复启动", async () => {
    await detectLSPServers();
    lspState.statuses["go"].running = true;
    const ok = await ensureLSPRunning("go");
    expect(ok).toBe(true);
    expect(mocks.lspService.startServer).not.toHaveBeenCalled();
  });

  it("可用但未运行时懒启动并返回 true", async () => {
    await detectLSPServers();
    const ok = await ensureLSPRunning("typescript");
    expect(ok).toBe(true);
    expect(mocks.lspService.startServer).toHaveBeenCalledWith("typescript");
    expect(lspState.statuses["typescript"].running).toBe(true);
  });

  it("不可用时返回 false 且不尝试启动", async () => {
    await detectLSPServers();
    const ok = await ensureLSPRunning("javascript");
    expect(ok).toBe(false);
    expect(mocks.lspService.startServer).not.toHaveBeenCalled();
  });

  it("未检测到的语言（status 不存在）返回 false", async () => {
    await detectLSPServers();
    const ok = await ensureLSPRunning("rust");
    expect(ok).toBe(false);
    expect(mocks.lspService.startServer).not.toHaveBeenCalled();
  });
});

describe("lsp store — monacoLanguageToLSP", () => {
  it("映射支持的语言到 LSP key", () => {
    expect(monacoLanguageToLSP("go")).toBe("go");
    expect(monacoLanguageToLSP("typescript")).toBe("typescript");
    expect(monacoLanguageToLSP("javascript")).toBe("javascript");
  });

  it("不支持的语言返回 null", () => {
    expect(monacoLanguageToLSP("python")).toBeNull();
    expect(monacoLanguageToLSP("rust")).toBeNull();
    expect(monacoLanguageToLSP("")).toBeNull();
  });
});

describe("lsp store — getLSPCompletions", () => {
  it("调用 lspService.getCompletions 并返回补全列表（含懒启动）", async () => {
    await detectLSPServers();
    const items = await getLSPCompletions(
      "go",
      "/repo/main.go",
      10,
      5,
      "package main\n",
    );
    expect(items).toEqual(sampleCompletions);
    expect(mocks.lspService.getCompletions).toHaveBeenCalledTimes(1);
    const req = mocks.lspService.getCompletions.mock.calls[0][0];
    expect(req).toMatchObject({
      language: "go",
      filePath: "/repo/main.go",
      line: 10,
      column: 5,
      content: "package main\n",
    });
  });

  it("enabled 为 false 时直接返回空列表且不调用 service", async () => {
    await detectLSPServers();
    lspState.enabled = false;
    const items = await getLSPCompletions("go", "/repo/main.go", 1, 1, "");
    expect(items).toEqual([]);
    expect(mocks.lspService.getCompletions).not.toHaveBeenCalled();
  });

  it("不支持的语言返回空列表且不调用 service", async () => {
    await detectLSPServers();
    const items = await getLSPCompletions("python", "/repo/main.py", 1, 1, "");
    expect(items).toEqual([]);
    expect(mocks.lspService.getCompletions).not.toHaveBeenCalled();
  });

  it("getCompletions 抛错时优雅降级返回空列表（不抛出）", async () => {
    await detectLSPServers();
    mocks.lspService.getCompletions.mockRejectedValueOnce(new Error("conn reset"));
    const items = await getLSPCompletions("go", "/repo/main.go", 1, 1, "");
    expect(items).toEqual([]);
  });
});

describe("lsp store — getLSPHover", () => {
  it("调用 lspService.getHover 并返回文本", async () => {
    await detectLSPServers();
    const hover = await getLSPHover("typescript", "/repo/index.ts", 3, 7, "const x = 1;");
    expect(hover).toBe("func fmt.Println(a ...any) (int, error)");
    expect(mocks.lspService.getHover).toHaveBeenCalledTimes(1);
    const req = mocks.lspService.getHover.mock.calls[0][0];
    expect(req).toMatchObject({ language: "typescript", filePath: "/repo/index.ts", line: 3, column: 7 });
  });

  it("getHover 抛错时优雅降级返回空字符串", async () => {
    await detectLSPServers();
    mocks.lspService.getHover.mockRejectedValueOnce(new Error("boom"));
    const hover = await getLSPHover("go", "/repo/main.go", 1, 1, "");
    expect(hover).toBe("");
  });

  it("enabled 为 false 时返回空字符串", async () => {
    await detectLSPServers();
    lspState.enabled = false;
    const hover = await getLSPHover("go", "/repo/main.go", 1, 1, "");
    expect(hover).toBe("");
    expect(mocks.lspService.getHover).not.toHaveBeenCalled();
  });
});

describe("lsp store — stopAllLSPServers", () => {
  it("停止所有正在运行的服务器，未运行的不调用 stop", async () => {
    await detectLSPServers();
    // 仅 go 与 typescript 处于运行态。
    lspState.statuses["go"].running = true;
    lspState.statuses["typescript"].running = true;
    // javascript 未安装且未运行。
    await stopAllLSPServers();
    expect(mocks.lspService.stopServer).toHaveBeenCalledWith("go");
    expect(mocks.lspService.stopServer).toHaveBeenCalledWith("typescript");
    expect(mocks.lspService.stopServer).not.toHaveBeenCalledWith("javascript");
    expect(lspState.statuses["go"].running).toBe(false);
    expect(lspState.statuses["typescript"].running).toBe(false);
  });

  it("无运行中服务器时不调用 stop", async () => {
    await detectLSPServers();
    await stopAllLSPServers();
    expect(mocks.lspService.stopServer).not.toHaveBeenCalled();
  });
});

describe("lsp store — initLSPStore 与 reset 工具", () => {
  it("initLSPStore 触发一次 detectLSPServers（即 detectServers）", async () => {
    await initLSPStore();
    expect(mocks.lspService.detectServers).toHaveBeenCalledTimes(1);
    expect(Object.keys(lspState.statuses).length).toBeGreaterThan(0);
  });

  it("__resetLSPStoreForTesting 将状态恢复到初始值", async () => {
    await detectLSPServers();
    lspState.statuses["go"].running = true;
    lspState.busy = true;
    lspState.enabled = false;
    __resetLSPStoreForTesting();
    expect(lspState.statuses).toEqual({});
    expect(lspState.busy).toBe(false);
    expect(lspState.enabled).toBe(true);
  });
});
