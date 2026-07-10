/**
 * Plan 11 Task 10 Step 7 — Goal 模式前端 store。
 *
 * 后端 `services/ai_goal_service.go` 的前端对应物。职责：
 *   - 创建/运行/暂停/恢复/中止 Goal。
 *   - 检查点管理（创建/回滚/列表）。
 *   - 成本报告查询。
 *   - Goal 与 Plan 互斥（active 跟踪）。
 */

import { reactive, computed } from "vue";
import { errorMessage } from "@/lib/errors";

// ---------------------------------------------------------------------------
// 类型 — 镜像 Go 结构体
// ---------------------------------------------------------------------------

export type GoalStatus = "created" | "running" | "paused" | "completed" | "aborted" | "failed";

export interface Checkpoint {
  iteration: number;
  snapshot: string;
  cost: number;
  createdAt: string;
  note?: string;
}

export interface Goal {
  id: string;
  description: string;
  successCriteria: string;
  maxIterations: number;
  maxCost: number;
  maxDuration: number;
  checkpoints: Checkpoint[];
  status: GoalStatus;
  iteration: number;
  totalCost: number;
  totalTokens: number;
  startedAt?: string;
  finishedAt?: string;
  lastError?: string;
}

export interface CostReport {
  totalCost: number;
  maxCost: number;
  remainingCost: number;
  totalTokens: number;
  iteration: number;
  maxIterations: number;
}

interface GoalStoreState {
  activeGoal: Goal | null;
  goals: Goal[];
  costReport: CostReport | null;
  loading: boolean;
  error: string | null;
}

export const aiGoalState = reactive<GoalStoreState>({
  activeGoal: null,
  goals: [],
  costReport: null,
  loading: false,
  error: null,
});

export const activeGoal = computed(() => aiGoalState.activeGoal);
export const goalsList = computed(() => aiGoalState.goals);
export const costReport = computed(() => aiGoalState.costReport);
export const isLoadingGoals = computed(() => aiGoalState.loading);
export const goalError = computed(() => aiGoalState.error);

// ---------------------------------------------------------------------------
// Backend 适配层
// ---------------------------------------------------------------------------

export interface GoalBackend {
  createGoal(id: string, description: string, successCriteria: string, maxIterations: number, maxCost: number, maxDuration: number, explicitConfirmation: boolean): Promise<Goal | null>;
  getGoal(id: string): Promise<Goal | null>;
  getActiveGoal(): Promise<Goal | null>;
  runGoal(id: string): Promise<void>;
  pauseGoal(id: string): Promise<void>;
  resumeGoal(id: string): Promise<void>;
  abortGoal(id: string): Promise<void>;
  listGoals(): Promise<(Goal | null)[]>;
  createCheckpoint(id: string, snapshot: string, note: string): Promise<void>;
  rollbackToCheckpoint(id: string, checkpointIdx: number): Promise<void>;
  listCheckpoints(id: string): Promise<Checkpoint[]>;
  getCostReport(id: string): Promise<CostReport>;
}

let backend: GoalBackend | null = null;

export function setGoalBackend(b: GoalBackend | null): void {
  backend = b;
}

interface GoalBindingsShape {
  CreateGoal(id: string, description: string, successCriteria: string, maxIterations: number, maxCost: number, maxDuration: number, explicitConfirmation: boolean): Promise<Goal>;
  GetGoal(id: string): Promise<Goal>;
  GetActiveGoal(): Promise<Goal | null>;
  // executor/checker 为 Go 接口，前端传 null，后端回退到内部注入的 executor。
  RunGoal(id: string, executor: unknown, checker: unknown): Promise<void>;
  PauseGoal(id: string): Promise<void>;
  ResumeGoal(id: string, executor: unknown, checker: unknown): Promise<void>;
  AbortGoal(id: string): Promise<void>;
  ListGoals(): Promise<Goal[]>;
  CreateCheckpoint(id: string, snapshot: string, note: string): Promise<void>;
  RollbackToCheckpoint(id: string, checkpointIdx: number): Promise<void>;
  ListCheckpoints(id: string): Promise<Checkpoint[]>;
  GetCostReport(id: string): Promise<CostReport>;
}

let bindingsCache: GoalBindingsShape | null = null;

async function loadBindings(): Promise<GoalBindingsShape> {
  if (bindingsCache) return bindingsCache;
  // 使用字面量路径（无 @vite-ignore），让 vite 将 bindings 打包为 chunk。
  // 否则生产构建后 dist 中无 bindings 文件，运行时动态加载会 404。
  const mod = await import("../../bindings/gugacode/services/aigoalservice.js");
  // bindings 文件使用命名导出（export function CreateGoal ...），不是命名空间。
  // 直接将 mod 作为 GoalBindingsShape 使用（方法名一一对应）。
  bindingsCache = mod as unknown as GoalBindingsShape;
  return bindingsCache;
}

// normalizeGoal 确保 Goal 的切片字段不为 null（Go nil 切片序列化为 null）。
// 否则组件模板访问 goal.checkpoints.length 会报 "Cannot read properties of null"。
function normalizeGoal(g: Goal | null): Goal | null {
  if (!g) return null;
  return { ...g, checkpoints: g.checkpoints ?? [] };
}

function getDefaultBackend(): GoalBackend {
  return {
    async createGoal(id, description, successCriteria, maxIterations, maxCost, maxDuration, explicitConfirmation) {
      const b = await loadBindings();
      return normalizeGoal(await b.CreateGoal(id, description, successCriteria, maxIterations, maxCost, maxDuration, explicitConfirmation));
    },
    async getGoal(id) {
      const b = await loadBindings();
      return normalizeGoal(await b.GetGoal(id));
    },
    async getActiveGoal() {
      const b = await loadBindings();
      return normalizeGoal(await b.GetActiveGoal());
    },
    async runGoal(id) {
      const b = await loadBindings();
      // executor/checker 为 Go 接口，前端传 null，后端回退到内部注入的实现。
      await b.RunGoal(id, null, null);
    },
    async pauseGoal(id) {
      const b = await loadBindings();
      await b.PauseGoal(id);
    },
    async resumeGoal(id) {
      const b = await loadBindings();
      // executor/checker 为 Go 接口，前端传 null，后端回退到内部注入的实现。
      await b.ResumeGoal(id, null, null);
    },
    async abortGoal(id) {
      const b = await loadBindings();
      await b.AbortGoal(id);
    },
    async listGoals() {
      const b = await loadBindings();
      return (await b.ListGoals())?.map(normalizeGoal) ?? [];
    },
    async createCheckpoint(id, snapshot, note) {
      const b = await loadBindings();
      await b.CreateCheckpoint(id, snapshot, note);
    },
    async rollbackToCheckpoint(id, checkpointIdx) {
      const b = await loadBindings();
      await b.RollbackToCheckpoint(id, checkpointIdx);
    },
    async listCheckpoints(id) {
      const b = await loadBindings();
      return (await b.ListCheckpoints(id)) ?? [];
    },
    async getCostReport(id) {
      const b = await loadBindings();
      return b.GetCostReport(id);
    },
  };
}

function getBackend(): GoalBackend {
  if (backend) return backend;
  backend = getDefaultBackend();
  return backend;
}

// ---------------------------------------------------------------------------
// Store actions
// ---------------------------------------------------------------------------

export async function refreshActiveGoal(): Promise<void> {
  aiGoalState.error = null;
  try {
    aiGoalState.activeGoal = await getBackend().getActiveGoal();
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
  }
}

export async function createGoal(
  id: string,
  description: string,
  successCriteria: string,
  maxIterations: number,
  maxCost: number,
  maxDuration: number,
): Promise<boolean> {
  aiGoalState.error = null;
  try {
    // G-SEC-03（Step 10）：创建需显式确认 — 此处 explicitConfirmation=true。
    aiGoalState.activeGoal = await getBackend().createGoal(id, description, successCriteria, maxIterations, maxCost, maxDuration, true);
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export async function runGoal(id: string): Promise<boolean> {
  aiGoalState.error = null;
  try {
    await getBackend().runGoal(id);
    aiGoalState.activeGoal = await getBackend().getGoal(id);
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export async function pauseGoal(id: string): Promise<boolean> {
  aiGoalState.error = null;
  try {
    await getBackend().pauseGoal(id);
    aiGoalState.activeGoal = await getBackend().getGoal(id);
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export async function resumeGoal(id: string): Promise<boolean> {
  aiGoalState.error = null;
  try {
    await getBackend().resumeGoal(id);
    aiGoalState.activeGoal = await getBackend().getGoal(id);
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export async function abortGoal(id: string): Promise<boolean> {
  aiGoalState.error = null;
  try {
    await getBackend().abortGoal(id);
    aiGoalState.activeGoal = null;
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export async function refreshCostReport(id: string): Promise<void> {
  aiGoalState.error = null;
  try {
    aiGoalState.costReport = await getBackend().getCostReport(id);
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
  }
}

export async function createCheckpoint(id: string, snapshot: string, note: string): Promise<boolean> {
  aiGoalState.error = null;
  try {
    await getBackend().createCheckpoint(id, snapshot, note);
    aiGoalState.activeGoal = await getBackend().getGoal(id);
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export async function rollbackToCheckpoint(id: string, checkpointIdx: number): Promise<boolean> {
  aiGoalState.error = null;
  try {
    await getBackend().rollbackToCheckpoint(id, checkpointIdx);
    aiGoalState.activeGoal = await getBackend().getGoal(id);
    return true;
  } catch (e: unknown) {
    aiGoalState.error = errorMessage(e);
    return false;
  }
}

export function resetGoalStore(): void {
  aiGoalState.activeGoal = null;
  aiGoalState.goals = [];
  aiGoalState.costReport = null;
  aiGoalState.loading = false;
  aiGoalState.error = null;
  backend = null;
  bindingsCache = null;
}
