<script setup lang="ts">
/**
 * prompt-4 Task 2–7 — AI 助手独立 OS 窗口根视图。
 *
 * 布局对标 Codex 桌面端 + DeepSeek + VS Code ActivityBar：
 *   左 48px 活动栏 | 中部：顶栏标题 + 消息流 + 底部输入区
 *   侧抽屉：会话列表 / 设置 / 快照回滚
 *   底部弹出：精简终端
 *
 * 不复用主布局（hideLayout: true）。与主窗口通过 wails 事件联动：
 *   ai:selection       ← 主窗口选中代码注入
 *   ai:apply-to-editor → 代码块「应用到编辑器」回写主窗口
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { Events } from "@wailsio/runtime";
import {
  ChatDotRound,
  Monitor,
  FolderOpened,
  Document,
  Setting,
  Cpu,
  Top,
  Close,
  Plus,
} from "@element-plus/icons-vue";
import MessageList from "@/components/ai-assistant/MessageList.vue";
import InputComposer from "@/components/ai-assistant/InputComposer.vue";
import ConversationSidebar from "@/components/ai-assistant/ConversationSidebar.vue";
import SnapshotTimeline from "@/components/ai-assistant/SnapshotTimeline.vue";
import { aiState, loadConversation, clearMessages, addContextChip, sendMessage } from "@/stores/ai";
import { aiAssistantState } from "@/stores/aiAssistant";
import { appState, saveSettings } from "@/stores/app";
import { conversationService, windowService, projectService } from "@/api/services";
import { useI18n } from "@/lib/i18n";
import { notifyError, notifySuccess, notifyWarning } from "@/lib/notifications";
import type { Project } from "@/types";
import { agentMcpTools, refreshAgentMcpTools } from "@/stores/mcp";
import { skillsList, loadSkills } from "@/stores/skills";
import { setSnapshotWorkspaceRoot, listSnapshots } from "@/stores/snapshot";
import TerminalPanel from "@/components/layout/TerminalPanel.vue";

const { t } = useI18n();

type DrawerKind = "none" | "conversations" | "settings" | "rollback";
type ActiveView = "chat" | "terminal";

const activeView = ref<ActiveView>("chat");
const drawer = ref<DrawerKind>("none");
const drawerWidth = ref(280);
const alwaysOnTop = ref(true);
const editingTitle = ref(false);
const titleDraft = ref("");
const flashAction = ref<string | null>(null);
/** Exposed methods from InputComposer (avoid `as any` — prompt-5 Task C). */
type ComposerExpose = {
  handleAttach?: () => void;
};
const composer = ref<ComposerExpose | null>(null);
/** Cached path from last ai:selection (AI window does not share main Vue state). */
const lastSelectionPath = ref("");

// 底部工具栏：工作区 / MCP / Skills / 模型
const projects = ref<Project[]>([]);
const selectedWorkspace = ref(appState.currentProject || "");
const selectedMcp = ref<string[]>([]);
const selectedSkills = ref<string[]>([]);
const showWorkspaceMenu = ref(false);
const showMcpMenu = ref(false);
const showSkillsMenu = ref(false);
const showModelMenu = ref(false);

const conversationTitle = computed(() => {
  if (aiState.currentConversationTitle) return aiState.currentConversationTitle;
  if (aiState.currentConversationId) return t("aiWindow.untitledConversation");
  return t("aiWindow.newConversation");
});

const modelOptions = computed(() => {
  const opts: { label: string; value: string; configId: string }[] = [];
  for (const cfg of appState.aiProviderConfigs) {
    const models = cfg.model ? [cfg.model] : [];
    // 已配置的模型直接列出
    opts.push({
      label: `${cfg.provider || cfg.name || "AI"}: ${cfg.model || "—"}`,
      value: cfg.model || "",
      configId: cfg.id,
    });
    void models;
  }
  if (opts.length === 0 && appState.aiModel) {
    opts.push({ label: appState.aiModel, value: appState.aiModel, configId: appState.activeAIConfigId });
  }
  return opts;
});

const currentModelLabel = computed(() => appState.aiModel || t("aiAssistant.noModel"));

const isNarrow = ref(false);
function updateNarrow(): void {
  isNarrow.value = window.innerWidth < 400;
}

// ---- 活动栏操作 ----

function flash(key: string): void {
  flashAction.value = key;
  window.setTimeout(() => {
    if (flashAction.value === key) flashAction.value = null;
  }, 400);
}

function toggleDrawer(kind: Exclude<DrawerKind, "none">): void {
  drawer.value = drawer.value === kind ? "none" : kind;
  if (kind === "conversations" || kind === "settings" || kind === "rollback") {
    activeView.value = "chat";
  }
  if (kind === "rollback" && selectedWorkspace.value) {
    setSnapshotWorkspaceRoot(selectedWorkspace.value);
    void listSnapshots();
  }
}

function goChat(): void {
  activeView.value = "chat";
  drawer.value = "none";
}

function toggleTerminal(): void {
  if (activeView.value === "terminal") {
    activeView.value = "chat";
  } else {
    activeView.value = "terminal";
    drawer.value = "none";
  }
}

async function openInExplorer(): Promise<void> {
  flash("explorer");
  const path = selectedWorkspace.value || appState.currentProject;
  if (!path) {
    notifyWarning(t("aiWindow.noWorkspace"));
    return;
  }
  try {
    await windowService.openPathInExplorer(path);
  } catch (e: unknown) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

async function openInVSCode(): Promise<void> {
  flash("vscode");
  const path = selectedWorkspace.value || appState.currentProject;
  if (!path) {
    notifyWarning(t("aiWindow.noWorkspace"));
    return;
  }
  try {
    await windowService.openPathInVSCode(path);
  } catch (e: unknown) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

async function toggleAlwaysOnTop(): Promise<void> {
  alwaysOnTop.value = !alwaysOnTop.value;
  try {
    await windowService.setAIAlwaysOnTop(alwaysOnTop.value);
  } catch (e: unknown) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

// ---- 会话 ----

async function handleSelectConversation(id: string): Promise<void> {
  if (id === "") {
    aiState.currentConversationId = null;
    aiState.currentConversationTitle = null;
    aiState.messages = [];
    drawer.value = "none";
    return;
  }
  aiState.currentConversationId = id;
  await loadConversation(id);
  drawer.value = "none";
}

function startEditTitle(): void {
  titleDraft.value = conversationTitle.value;
  editingTitle.value = true;
}

async function commitTitle(): Promise<void> {
  editingTitle.value = false;
  const next = titleDraft.value.trim();
  if (!next || !aiState.currentConversationId) return;
  try {
    await conversationService.updateTitle(aiState.currentConversationId, next);
    aiState.currentConversationTitle = next;
  } catch (e: unknown) {
    notifyError(e instanceof Error ? e.message : String(e));
  }
}

function newConversation(): void {
  aiState.currentConversationId = null;
  aiState.currentConversationTitle = null;
  clearMessages();
  drawer.value = "none";
}

// ---- 工作区 / MCP / Skills / 模型 ----

function selectWorkspace(path: string): void {
  selectedWorkspace.value = path;
  appState.currentProject = path;
  const p = projects.value.find((x) => x.path === path);
  if (p) appState.projectName = p.name;
  setSnapshotWorkspaceRoot(path);
  showWorkspaceMenu.value = false;
}

function toggleMcp(ns: string): void {
  const i = selectedMcp.value.indexOf(ns);
  if (i >= 0) selectedMcp.value.splice(i, 1);
  else selectedMcp.value.push(ns);
}

function toggleSkill(id: string): void {
  const i = selectedSkills.value.indexOf(id);
  if (i >= 0) selectedSkills.value.splice(i, 1);
  else selectedSkills.value.push(id);
}

function selectModel(model: string, configId: string): void {
  if (model) appState.aiModel = model;
  if (configId) appState.activeAIConfigId = configId;
  saveSettings();
  showModelMenu.value = false;
}

// ---- 跨窗口联动 ----

function onSelectionEvent(data: unknown): void {
  const payload = (Array.isArray(data) ? data[0] : data) as
    | { code?: string; language?: string; filePath?: string }
    | undefined;
  if (!payload?.code) return;
  const lang = payload.language || "text";
  const path = payload.filePath || "selection";
  // prompt-5 Task A: cache real path for Apply payload (AI webview has no main editor state).
  if (payload.filePath && payload.filePath !== "untitled") {
    lastSelectionPath.value = payload.filePath;
  }
  addContextChip({
    id: `sel-${Date.now()}`,
    kind: "codeblock",
    label: path.split(/[/\\]/).pop() || path,
    content: payload.code,
    language: lang,
  });
  // 预填输入提示，方便用户直接补充指令后发送
  notifySuccess(t("aiWindow.selectionReceived"));
}

function handleMessageClick(e: MouseEvent): void {
  const target = e.target as HTMLElement | null;
  if (!target?.classList.contains("code-block-apply-btn")) return;
  const wrap = target.closest(".code-block-wrap") as HTMLElement | null;
  const pre = wrap?.querySelector("pre");
  if (!pre) return;
  const code = pre.textContent ?? "";
  // Prefer last selection path; fall back to local appState (usually empty in AI window).
  const filePath = lastSelectionPath.value || appState.currentFilePath || "";
  if (!filePath) {
    notifyError(t("aiWindow.noActiveFile"), t("aiWindow.applyTitle"));
    return;
  }
  void Events.Emit("ai:apply-to-editor", {
    code,
    filePath,
    language: "",
  });
  // 仅表示请求已发出；真正成功由主窗 Diff 确认后提示
  notifySuccess(t("aiWindow.applySent"));
}

// ---- 抽屉拖拽宽度 ----

let resizing = false;
function onDrawerResizeStart(e: MouseEvent): void {
  e.preventDefault();
  resizing = true;
  const onMove = (ev: MouseEvent): void => {
    if (!resizing) return;
    // 活动栏 48px + 抽屉从左侧开始
    const w = Math.min(480, Math.max(200, ev.clientX - 48));
    drawerWidth.value = w;
  };
  const onUp = (): void => {
    resizing = false;
    window.removeEventListener("mousemove", onMove);
    window.removeEventListener("mouseup", onUp);
  };
  window.addEventListener("mousemove", onMove);
  window.addEventListener("mouseup", onUp);
}

// ---- 生命周期 ----

let unsubSelection: (() => void) | null = null;

onMounted(async () => {
  updateNarrow();
  window.addEventListener("resize", updateNarrow);

  try {
    alwaysOnTop.value = await windowService.isAIAlwaysOnTop();
  } catch {
    alwaysOnTop.value = true;
  }

  try {
    projects.value = await projectService.getRecentProjects();
  } catch {
    projects.value = [];
  }
  if (!selectedWorkspace.value && appState.currentProject) {
    selectedWorkspace.value = appState.currentProject;
  }
  if (selectedWorkspace.value) {
    setSnapshotWorkspaceRoot(selectedWorkspace.value);
  }

  void refreshAgentMcpTools();
  void loadSkills();

  // 监听主窗口选中代码
  try {
    const off = Events.On("ai:selection", (e: unknown) => {
      // wails Events.On callback receives event object with data
      const ev = e as { data?: unknown } | unknown;
      const data = ev && typeof ev === "object" && "data" in (ev as object)
        ? (ev as { data: unknown }).data
        : e;
      onSelectionEvent(data);
    });
    unsubSelection = typeof off === "function" ? off : null;
  } catch {
    // Events.On may be unavailable in pure browser dev
  }
});

onBeforeUnmount(() => {
  window.removeEventListener("resize", updateNarrow);
  unsubSelection?.();
});

// 同步工作区到快照
watch(selectedWorkspace, (root) => {
  if (root) setSnapshotWorkspaceRoot(root);
});

// 暴露给模板的 mode 标签
const modeLabel = computed(() => {
  const m = aiAssistantState.mode;
  if (m === "plan") return t("aiAssistant.modePlan");
  if (m === "goal") return t("aiAssistant.modeGoal");
  if (m === "agent") return t("aiAssistant.modeAgent");
  return t("aiAssistant.modeChat");
});

// 避免 unused sendMessage 警告（保留以便未来快捷发送）
void sendMessage;
</script>

<template>
  <div class="ai-window" :class="{ 'ai-window--narrow': isNarrow }">
    <!-- 左侧活动栏 48px -->
    <aside class="ai-window__activity" role="toolbar" :aria-label="t('aiWindow.activityAria')">
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-active': activeView === 'chat' && drawer === 'none' }"
        :title="t('aiWindow.actChat')"
        :aria-label="t('aiWindow.actChat')"
        @click="goChat"
      >
        <el-icon :size="20"><Cpu /></el-icon>
      </button>
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-active': drawer === 'conversations' }"
        :title="t('aiWindow.actConversations')"
        :aria-label="t('aiWindow.actConversations')"
        @click="toggleDrawer('conversations')"
      >
        <el-icon :size="20"><ChatDotRound /></el-icon>
      </button>
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-active': activeView === 'terminal' }"
        :title="t('aiWindow.actTerminal')"
        :aria-label="t('aiWindow.actTerminal')"
        @click="toggleTerminal"
      >
        <el-icon :size="20"><Monitor /></el-icon>
      </button>
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-flash': flashAction === 'explorer' }"
        :title="t('aiWindow.actExplorer')"
        :aria-label="t('aiWindow.actExplorer')"
        @click="openInExplorer"
      >
        <el-icon :size="20"><FolderOpened /></el-icon>
      </button>
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-flash': flashAction === 'vscode' }"
        :title="t('aiWindow.actVSCode')"
        :aria-label="t('aiWindow.actVSCode')"
        @click="openInVSCode"
      >
        <el-icon :size="20"><Document /></el-icon>
      </button>
      <div class="ai-window__act-spacer" />
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-active': alwaysOnTop }"
        :title="t('aiWindow.alwaysOnTop')"
        :aria-label="t('aiWindow.alwaysOnTop')"
        @click="toggleAlwaysOnTop"
      >
        <el-icon :size="18"><Top /></el-icon>
      </button>
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-active': drawer === 'rollback' }"
        :title="t('aiWindow.actRollback')"
        :aria-label="t('aiWindow.actRollback')"
        @click="toggleDrawer('rollback')"
      >
        <span class="ai-window__act-text">↺</span>
      </button>
      <button
        type="button"
        class="ai-window__act"
        :class="{ 'is-active': drawer === 'settings' }"
        :title="t('aiWindow.actSettings')"
        :aria-label="t('aiWindow.actSettings')"
        @click="toggleDrawer('settings')"
      >
        <el-icon :size="20"><Setting /></el-icon>
      </button>
    </aside>

    <!-- 侧抽屉 -->
    <aside
      v-if="drawer !== 'none'"
      class="ai-window__drawer"
      :style="{ width: `${drawerWidth}px` }"
    >
      <div class="ai-window__drawer-head">
        <span class="ai-window__drawer-title">
          {{
            drawer === "conversations"
              ? t("aiWindow.drawerConversations")
              : drawer === "rollback"
                ? t("aiWindow.drawerRollback")
                : t("aiWindow.drawerSettings")
          }}
        </span>
        <button
          v-if="drawer === 'conversations'"
          type="button"
          class="ai-window__icon-btn"
          :title="t('aiWindow.newConversation')"
          @click="newConversation"
        >
          <el-icon :size="16"><Plus /></el-icon>
        </button>
        <button type="button" class="ai-window__icon-btn" @click="drawer = 'none'">
          <el-icon :size="16"><Close /></el-icon>
        </button>
      </div>
      <div class="ai-window__drawer-body">
        <ConversationSidebar
          v-if="drawer === 'conversations'"
          :width="drawerWidth"
          @select="handleSelectConversation"
        />
        <SnapshotTimeline v-else-if="drawer === 'rollback'" />
        <div v-else class="ai-window__settings-mini">
          <p>{{ t("aiWindow.settingsHint") }}</p>
          <p class="ai-window__settings-meta">
            {{ t("aiWindow.mode") }}: {{ modeLabel }}
          </p>
          <p class="ai-window__settings-meta">
            {{ t("aiWindow.model") }}: {{ currentModelLabel }}
          </p>
          <p class="ai-window__settings-meta">
            {{ t("aiWindow.workspace") }}: {{ selectedWorkspace || "—" }}
          </p>
          <label class="ai-window__toggle">
            <input type="checkbox" :checked="alwaysOnTop" @change="toggleAlwaysOnTop" />
            {{ t("aiWindow.alwaysOnTop") }}
          </label>
        </div>
      </div>
      <div class="ai-window__drawer-resize" @mousedown="onDrawerResizeStart" />
    </aside>

    <!-- 主内容区 -->
    <main class="ai-window__main">
      <!-- 顶栏：会话标题 -->
      <header class="ai-window__top">
        <div
          v-if="!editingTitle"
          class="ai-window__title"
          :title="t('aiWindow.editTitleHint')"
          @dblclick="startEditTitle"
        >
          {{ conversationTitle }}
        </div>
        <input
          v-else
          v-model="titleDraft"
          class="ai-window__title-input"
          autofocus
          @keydown.enter="commitTitle"
          @keydown.esc="editingTitle = false"
          @blur="commitTitle"
        />
        <span class="ai-window__mode-badge">{{ modeLabel }}</span>
      </header>

      <!-- 消息流 / 终端 -->
      <div
        v-show="activeView === 'chat'"
        class="ai-window__messages"
        @click="handleMessageClick"
      >
        <MessageList />
      </div>
      <div v-show="activeView === 'terminal'" class="ai-window__terminal">
        <TerminalPanel />
      </div>

      <!-- 底部输入区 -->
      <footer v-show="activeView === 'chat'" class="ai-window__footer">
        <!-- 第一行：工具栏 -->
        <div class="ai-window__toolbar">
          <button
            type="button"
            class="ai-window__tool"
            :title="t('aiAssistant.attach')"
            @click="composer?.handleAttach?.()"
          >
            📎
          </button>

          <!-- 工作区 -->
          <div class="ai-window__dropdown">
            <button
              type="button"
              class="ai-window__tool"
              @click="showWorkspaceMenu = !showWorkspaceMenu; showMcpMenu = false; showSkillsMenu = false; showModelMenu = false"
            >
              {{ isNarrow ? "📁" : t("aiWindow.workspace") }}
              <span v-if="selectedWorkspace" class="ai-window__badge">1</span>
              ▾
            </button>
            <ul v-if="showWorkspaceMenu" class="ai-window__menu">
              <li
                v-for="p in projects"
                :key="p.path"
                :class="{ 'is-selected': p.path === selectedWorkspace }"
                @click="selectWorkspace(p.path)"
              >
                {{ p.name }}
              </li>
              <li v-if="projects.length === 0" class="ai-window__menu-empty">
                {{ t("aiWindow.noProjects") }}
              </li>
            </ul>
          </div>

          <!-- MCP -->
          <div class="ai-window__dropdown">
            <button
              type="button"
              class="ai-window__tool"
              @click="showMcpMenu = !showMcpMenu; showWorkspaceMenu = false; showSkillsMenu = false; showModelMenu = false"
            >
              MCP
              <span v-if="selectedMcp.length" class="ai-window__badge">{{ selectedMcp.length }}</span>
              ▾
            </button>
            <ul v-if="showMcpMenu" class="ai-window__menu">
              <li
                v-for="tool in agentMcpTools"
                :key="tool.namespace"
                @click.stop="toggleMcp(tool.namespace)"
              >
                <input type="checkbox" :checked="selectedMcp.includes(tool.namespace)" readonly />
                {{ tool.namespace }}
              </li>
              <li v-if="agentMcpTools.length === 0" class="ai-window__menu-empty">
                {{ t("aiAssistant.noMcpTools") }}
              </li>
            </ul>
          </div>

          <!-- Skills -->
          <div class="ai-window__dropdown">
            <button
              type="button"
              class="ai-window__tool"
              @click="showSkillsMenu = !showSkillsMenu; showWorkspaceMenu = false; showMcpMenu = false; showModelMenu = false"
            >
              Skills
              <span v-if="selectedSkills.length" class="ai-window__badge">{{ selectedSkills.length }}</span>
              ▾
            </button>
            <ul v-if="showSkillsMenu" class="ai-window__menu">
              <li
                v-for="sk in skillsList"
                :key="sk.id"
                @click.stop="toggleSkill(sk.id)"
              >
                <input type="checkbox" :checked="selectedSkills.includes(sk.id)" readonly />
                {{ sk.name }}
              </li>
              <li v-if="skillsList.length === 0" class="ai-window__menu-empty">
                {{ t("aiAssistant.noSkillsAvailable") }}
              </li>
            </ul>
          </div>

          <!-- 模型 -->
          <div class="ai-window__dropdown ai-window__dropdown--model">
            <button
              type="button"
              class="ai-window__tool"
              @click="showModelMenu = !showModelMenu; showWorkspaceMenu = false; showMcpMenu = false; showSkillsMenu = false"
            >
              {{ isNarrow ? "⚙" : currentModelLabel }}
              ▾
            </button>
            <ul v-if="showModelMenu" class="ai-window__menu">
              <li
                v-for="m in modelOptions"
                :key="m.configId + m.value"
                :class="{ 'is-selected': m.value === appState.aiModel }"
                @click="selectModel(m.value, m.configId)"
              >
                {{ m.label }}
              </li>
              <li v-if="modelOptions.length === 0" class="ai-window__menu-empty">
                {{ t("aiAssistant.noModel") }}
              </li>
            </ul>
          </div>
        </div>

        <!-- 第二行：输入框（复用 InputComposer） -->
        <InputComposer ref="composer" />
      </footer>
    </main>
  </div>
</template>

<style scoped>
.ai-window {
  display: flex;
  height: 100vh;
  width: 100vw;
  overflow: hidden;
  background: var(--color-bg-base, #06070f);
  color: var(--color-text-primary, #e0e0e0);
  font-family: var(--font-sans, system-ui, sans-serif);
}

/* Activity bar */
.ai-window__activity {
  width: 48px;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 8px 0;
  gap: 4px;
  background: var(--color-bg-surface, #0c0d16);
  border-right: 1px solid var(--color-border-default, #1e2030);
}
.ai-window__act {
  width: 40px;
  height: 40px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--color-text-secondary, #8b8fa3);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}
.ai-window__act:hover {
  background: var(--color-sidebar-hover, #1a1b2e);
  color: var(--color-text-primary, #e0e0e0);
}
.ai-window__act.is-active {
  background: var(--color-primary-container, #1e3a5f);
  color: var(--color-primary, #60a5fa);
}
.ai-window__act.is-flash {
  background: var(--color-primary, #3b82f6);
  color: #fff;
}
.ai-window__act-spacer {
  flex: 1;
}
.ai-window__act-text {
  font-size: 18px;
  line-height: 1;
}

/* Drawer */
.ai-window__drawer {
  position: relative;
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--color-bg-surface, #0c0d16);
  border-right: 1px solid var(--color-border-default, #1e2030);
  overflow: hidden;
}
.ai-window__drawer-head {
  height: 48px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-bottom: 1px solid var(--color-border-default, #1e2030);
  flex-shrink: 0;
}
.ai-window__drawer-title {
  flex: 1;
  font-size: 13px;
  font-weight: 500;
}
.ai-window__drawer-body {
  flex: 1;
  overflow: auto;
}
.ai-window__drawer-resize {
  position: absolute;
  top: 0;
  right: 0;
  width: 4px;
  height: 100%;
  cursor: col-resize;
}
.ai-window__drawer-resize:hover {
  background: var(--color-accent, #3b82f6);
}
.ai-window__icon-btn {
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
}
.ai-window__icon-btn:hover {
  background: var(--color-sidebar-hover, #1a1b2e);
  color: var(--color-text-primary);
}
.ai-window__settings-mini {
  padding: 16px;
  font-size: 13px;
  line-height: 1.6;
}
.ai-window__settings-meta {
  color: var(--color-text-secondary);
  margin: 4px 0;
  word-break: break-all;
}
.ai-window__toggle {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 12px;
  cursor: pointer;
}

/* Main column */
.ai-window__main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}
.ai-window__top {
  height: 48px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 16px;
  border-bottom: 1px solid var(--color-border-default, #1e2030);
  background: var(--color-bg-surface, #0c0d16);
}
.ai-window__title {
  flex: 1;
  font-size: 14px;
  color: var(--color-text-secondary, #a0a4b8);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  cursor: text;
  user-select: none;
}
.ai-window__title-input {
  flex: 1;
  font-size: 14px;
  background: var(--color-bg-elevated, #151625);
  border: 1px solid var(--color-border-default, #2a2d40);
  border-radius: 6px;
  color: var(--color-text-primary);
  padding: 4px 8px;
  outline: none;
}
.ai-window__mode-badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--color-bg-elevated, #151625);
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.ai-window__messages {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  /* personalization chat bg support */
  background-image: var(--personalization-chat-bg, none);
  background-size: cover;
  background-position: center;
}
.ai-window__terminal {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.ai-window__footer {
  flex-shrink: 0;
  border-top: 1px solid var(--color-border-default, #1e2030);
  background: var(--color-bg-surface, #0c0d16);
}
.ai-window__toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding: 8px 12px 0;
}
.ai-window__tool {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid var(--color-border-default, #2a2d40);
  background: var(--color-bg-elevated, #151625);
  color: var(--color-text-secondary, #a0a4b8);
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 6px;
  cursor: pointer;
  max-width: 160px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.ai-window__tool:hover {
  color: var(--color-text-primary);
  border-color: var(--color-primary, #3b82f6);
}
.ai-window__badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--color-primary, #3b82f6);
  color: #fff;
  font-size: 10px;
}
.ai-window__dropdown {
  position: relative;
}
.ai-window__menu {
  position: absolute;
  bottom: 100%;
  left: 0;
  margin-bottom: 4px;
  min-width: 180px;
  max-height: 240px;
  overflow: auto;
  list-style: none;
  padding: 4px;
  margin-left: 0;
  background: var(--color-bg-elevated, #1a1b2e);
  border: 1px solid var(--color-border-default, #2a2d40);
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  z-index: 50;
}
.ai-window__menu li {
  padding: 6px 10px;
  font-size: 12px;
  border-radius: 4px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
}
.ai-window__menu li:hover,
.ai-window__menu li.is-selected {
  background: var(--color-primary-container, #1e3a5f);
}
.ai-window__menu-empty {
  color: var(--color-text-secondary);
  cursor: default !important;
}

/* Message cards polish (Codex / DeepSeek style via deep selectors) */
.ai-window__messages :deep(.ai-msg) {
  padding: 16px;
  border-radius: 12px;
  max-width: 92%;
}
.ai-window__messages :deep(.ai-msg--user) {
  align-self: flex-end;
  background: var(--color-primary, #3b82f6);
  color: #fff;
}
.ai-window__messages :deep(.ai-msg--assistant) {
  align-self: flex-start;
  background: var(--color-bg-elevated, #151625);
  border: 1px solid var(--color-border-subtle, #1e2030);
}

.ai-window--narrow .ai-window__tool {
  max-width: 48px;
  padding: 4px 6px;
}
</style>
