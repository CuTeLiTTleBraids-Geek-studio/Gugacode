/**
 * Plan 11 Task 6 Step 5 — Computer Use 前端 store。
 *
 * 后端 `services/computer_use_service.go` 的前端对应物。职责：
 *   - 加载/保存 ComputerUseConfig（G-SEC-12：默认 Enabled=false）。
 *   - 查询审计日志（最近 N 条不可逆操作记录）。
 *   - 录制模式控制（StartRecording / StopRecording / IsRecording）。
 *
 * 安全（G-SEC-02 / G-SEC-06 / G-SEC-12）：
 *   - 启用 Computer Use 视同 Restricted 扩展能力，需 explicitApproval；
 *     UI 必须弹窗确认后调用 UpdateConfig(Enabled=true)。
 *   - 5 个工具的实际调用由后端 AgentService 经 CheckCommand 审批后
 *     分发，前端不绕过（ConfirmationRequired 强制每步截图+确认）。
 *   - 禁止快捷键黑名单 + 禁止区域由后端强制，前端仅展示。
 *
 * 采用与 `mcp.ts` / `skills.ts` 相同的「lazy bindings + 可注入 backend」
 * 模式，便于单元测试注入 mock。
 */

import { reactive, computed } from "vue";
import { errorMessage } from "@/lib/errors";

// ---------------------------------------------------------------------------
// 类型 — 镜像 Go 结构体（services/computer_use_service.go）
// ---------------------------------------------------------------------------

export interface ForbiddenZone {
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface ComputerUseConfig {
  /** G-SEC-12：默认 false，启用需 explicitApproval（视同 Restricted）。 */
  enabled: boolean;
  /** Step 3：为 true 时每次操作前必须截图 + AI 规划 + 用户确认。 */
  confirmationRequired: boolean;
  /** Step 2：0-100，PNG/JPEG 压缩质量。 */
  screenshotQuality?: number;
  /** Step 2：0.1-1.0，截图缩放比例（降低分辨率节省 token）。 */
  screenshotScale?: number;
  /** 应用白名单（进程名）；空表示不限制。 */
  appWhitelist?: string[];
  /** Step 5/6：屏幕上禁止操作的矩形区域（密码管理器等）。 */
  forbiddenZones?: ForbiddenZone[];
  /** Step 6：禁止的快捷键组合黑名单（含 OS 级危险快捷键）。 */
  forbiddenHotkeys?: string[];
  /** Step 4：录制模式开关。 */
  recordingEnabled?: boolean;
}

export interface AuditAction {
  timestamp: string;
  action: string;
  args: string;
  success: boolean;
  error?: string;
  confirmedByUser: boolean;
}

export interface RecordedAction {
  timestamp: string;
  action: string;
  args: string;
}

// ---------------------------------------------------------------------------
// Store state
// ---------------------------------------------------------------------------

interface ComputerUseStoreState {
  config: ComputerUseConfig;
  auditLog: AuditAction[];
  recording: boolean;
  loading: boolean;
  saving: boolean;
  error: string | null;
}

export const computerUseState = reactive<ComputerUseStoreState>({
  config: {
    enabled: false,
    confirmationRequired: true,
  },
  auditLog: [],
  recording: false,
  loading: false,
  saving: false,
  error: null,
});

export const computerUseConfig = computed(() => computerUseState.config);
export const computerUseEnabled = computed(() => computerUseState.config.enabled);
export const computerUseRecording = computed(() => computerUseState.recording);
export const computerUseAuditLog = computed(() => computerUseState.auditLog);
export const isLoadingComputerUse = computed(() => computerUseState.loading);
export const computerUseError = computed(() => computerUseState.error);

// ---------------------------------------------------------------------------
// Backend 适配层（lazy 加载 Wails bindings；测试可注入 mock）
// ---------------------------------------------------------------------------

export interface ComputerUseBackend {
  getConfig(): Promise<ComputerUseConfig>;
  updateConfig(cfg: ComputerUseConfig): Promise<void>;
  isEnabled(): Promise<boolean>;
  getAuditLog(limit: number): Promise<AuditAction[]>;
  startRecording(): Promise<void>;
  stopRecording(): Promise<RecordedAction[]>;
  isRecording(): Promise<boolean>;
}

let backend: ComputerUseBackend | null = null;

/** 注入 backend 适配器。测试注入 mock；应用启动时注入默认 Wails 适配器。 */
export function setComputerUseBackend(b: ComputerUseBackend | null): void {
  backend = b;
}

interface ComputerUseBindingsShape {
  GetConfig(): Promise<ComputerUseConfig>;
  UpdateConfig(cfg: ComputerUseConfig): Promise<void>;
  IsEnabled(): Promise<boolean>;
  GetAuditLog(limit: number): Promise<AuditAction[]>;
  StartRecording(): Promise<void>;
  StopRecording(): Promise<RecordedAction[]>;
  IsRecording(): Promise<boolean>;
}

let bindingsCache: ComputerUseBindingsShape | null = null;

async function loadBindings(): Promise<ComputerUseBindingsShape> {
  if (bindingsCache) return bindingsCache;
  // 使用字面量路径（无 @vite-ignore），让 vite 将 bindings 打包为 chunk。
  const mod = await import("../../bindings/gugacode/services/computeruseservice.js");
  // bindings 文件使用命名导出，直接将 mod 作为 ComputerUseBindingsShape 使用。
  bindingsCache = mod as unknown as ComputerUseBindingsShape;
  return bindingsCache;
}

// normalizeConfig 确保 ComputerUseConfig 的切片字段不为 null。
function normalizeConfig(c: ComputerUseConfig): ComputerUseConfig {
  return {
    ...c,
    appWhitelist: c.appWhitelist ?? [],
    forbiddenZones: c.forbiddenZones ?? [],
    forbiddenHotkeys: c.forbiddenHotkeys ?? [],
  };
}

function getDefaultBackend(): ComputerUseBackend {
  return {
    async getConfig() {
      const b = await loadBindings();
      return normalizeConfig(await b.GetConfig());
    },
    async updateConfig(cfg) {
      const b = await loadBindings();
      await b.UpdateConfig(cfg);
    },
    async isEnabled() {
      const b = await loadBindings();
      return b.IsEnabled();
    },
    async getAuditLog(limit) {
      const b = await loadBindings();
      return (await b.GetAuditLog(limit)) ?? [];
    },
    async startRecording() {
      const b = await loadBindings();
      await b.StartRecording();
    },
    async stopRecording() {
      const b = await loadBindings();
      return (await b.StopRecording()) ?? [];
    },
    async isRecording() {
      const b = await loadBindings();
      return b.IsRecording();
    },
  };
}

function getBackend(): ComputerUseBackend {
  if (backend) return backend;
  backend = getDefaultBackend();
  return backend;
}

// ---------------------------------------------------------------------------
// Store actions
// ---------------------------------------------------------------------------

/** 从后端加载配置 + 录制状态。 */
export async function loadComputerUseConfig(): Promise<void> {
  computerUseState.loading = true;
  computerUseState.error = null;
  try {
    computerUseState.config = await getBackend().getConfig();
    computerUseState.recording = await getBackend().isRecording();
  } catch (e: unknown) {
    computerUseState.error = errorMessage(e);
  } finally {
    computerUseState.loading = false;
  }
}

/**
 * 保存配置到后端（持久化 0600 + atomicWriteFile）。
 * G-SEC-12：从 enabled=false → true 是显式审批动作，调用方需确保用户已确认。
 */
export async function saveComputerUseConfig(cfg: ComputerUseConfig): Promise<boolean> {
  computerUseState.saving = true;
  computerUseState.error = null;
  try {
    await getBackend().updateConfig(cfg);
    computerUseState.config = await getBackend().getConfig();
    return true;
  } catch (e: unknown) {
    computerUseState.error = errorMessage(e);
    return false;
  } finally {
    computerUseState.saving = false;
  }
}

/** 拉取审计日志（最近 N 条）。 */
export async function refreshAuditLog(limit = 100): Promise<void> {
  computerUseState.error = null;
  try {
    computerUseState.auditLog = await getBackend().getAuditLog(limit);
  } catch (e: unknown) {
    computerUseState.error = errorMessage(e);
  }
}

/** 开始录制模式（Step 4）。需 Computer Use 已启用。 */
export async function startRecording(): Promise<boolean> {
  computerUseState.error = null;
  try {
    await getBackend().startRecording();
    computerUseState.recording = true;
    return true;
  } catch (e: unknown) {
    computerUseState.error = errorMessage(e);
    return false;
  }
}

/** 停止录制并返回捕获的操作序列（Step 4）。 */
export async function stopRecording(): Promise<RecordedAction[]> {
  computerUseState.error = null;
  try {
    const actions = await getBackend().stopRecording();
    computerUseState.recording = false;
    return actions;
  } catch (e: unknown) {
    computerUseState.error = errorMessage(e);
    return [];
  }
}

/** 重置 store 状态。测试专用。 */
export function resetComputerUseStore(): void {
  computerUseState.config = { enabled: false, confirmationRequired: true };
  computerUseState.auditLog = [];
  computerUseState.recording = false;
  computerUseState.loading = false;
  computerUseState.saving = false;
  computerUseState.error = null;
  backend = null;
  bindingsCache = null;
}
