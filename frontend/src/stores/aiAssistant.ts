import { reactive } from "vue";
import { aiState } from "@/stores/ai";
import { windowService } from "@/api/services";

/**
 * Plan 11 Task 1 — AI 助手独立页面状态。
 *
 * `aiState`（@/stores/ai）管理对话内容（消息流/当前会话 ID），嵌入式
 * AiChatPanel 与独立页面 AiAssistantView 共享同一 `aiState` 实例，切换
 * 模式不丢失会话。本 store 只管独立页面的 UI 状态：当前模式、侧栏宽度、
 * 上下文面板折叠态、活动会话 ID 同步。
 *
 * 模式语义：
 *   - chat: 普通问答
 *   - plan: 先规划后执行（Task 9）
 *   - goal: 目标驱动自治（Task 10）
 *   - agent: 工具调用代理（既有 Agent mode）
 */

export type AiMode = "chat" | "plan" | "goal" | "agent";

export interface AiAssistantState {
  /** 当前交互模式，决定右侧面板与工具集。 */
  mode: AiMode;
  /** 左侧会话列表宽度（px），可拖拽调整。 */
  sidebarWidth: number;
  /** 右侧上下文面板是否折叠。 */
  contextPanelCollapsed: boolean;
  /** 独立页面当前活动会话 ID，与 aiState.currentConversationId 同步。 */
  activeConversationId: string | null;
}

export const aiAssistantState = reactive<AiAssistantState>({
  mode: "chat",
  sidebarWidth: 260,
  contextPanelCollapsed: false,
  activeConversationId: null,
});

/**
 * 打开 AI 助手独立页面 /ai。先将当前会话 ID 同步到独立页面状态，再通过
 * hash 路由导航。嵌入式 AiChatPanel 与独立页面共享 `aiState`，切换不丢失
 * 会话内容。
 */
export function openStandalonePage(): void {
  aiAssistantState.activeConversationId = aiState.currentConversationId;
  if (window.location.hash !== "#/ai") {
    window.location.hash = "#/ai";
  }
}

/**
 * prompt-4: 打开（或聚焦）OS 级 AI 伴侣窗口。
 * 与 /ai 路由页面并存：本函数创建第二个 WebviewWindow，不切换主窗口路由。
 */
export function openAIDesktopWindow(): void {
  aiAssistantState.activeConversationId = aiState.currentConversationId;
  void windowService.openAIWindow().catch(() => {
    // 后端未就绪时忽略
  });
}

/** 切换交互模式。Plan/Goal 互斥，切换时清空对端面板状态由各面板自行处理。 */
export function switchMode(mode: AiMode): void {
  aiAssistantState.mode = mode;
}

/** 设置左侧会话列表宽度（拖拽调整）。限制 200–480px 避免过窄/过宽。 */
export function setSidebarWidth(px: number): void {
  const clamped = Math.min(480, Math.max(200, px));
  aiAssistantState.sidebarWidth = clamped;
}

/** 折叠/展开右侧上下文面板。 */
export function toggleContextPanel(): void {
  aiAssistantState.contextPanelCollapsed = !aiAssistantState.contextPanelCollapsed;
}

/** 同步当前活动会话到独立页面状态（会话切换时调用）。 */
export function setActiveConversation(id: string | null): void {
  aiAssistantState.activeConversationId = id;
}

/** 返回独立页面是否已就绪（有活动会话或处于初始 chat 模式）。 */
export function isStandaloneReady(): boolean {
  return aiAssistantState.mode !== null;
}
