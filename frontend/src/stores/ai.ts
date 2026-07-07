import { reactive } from "vue";
import { Events } from "@wailsio/runtime";
import { aiService, conversationService } from "@/api/services";
import { appState, activeAIConfig } from "@/stores/app";
import { notifyError } from "@/lib/notifications";
import { pushOutput } from "@/stores/output";
import { agentState, getAgentSystemPrompt, onAssistantFinished } from "@/stores/agent";
import { rulesForPrompt } from "@/stores/rules";
import { errorMessage } from "@/lib/errors";
import { translate } from "@/lib/i18n";
import type { ChatMessage, AIContextAttachment, AIActionName, Conversation, FileContextEntry, AIChunkEvent, AIDoneEvent, AIErrorEvent } from "@/types";

export interface AIState {
  messages: ChatMessage[];
  streaming: boolean;
  error: string | null;
  context: AIContextAttachment | null;
  currentConversationId: string | null;
  currentConversationTitle: string | null;
  mentionedFiles: FileContextEntry[];
  // N-60: Per-conversation system prompt override. When non-null, this
  // conversation uses a custom system prompt instead of the global
  // appState.aiSystemPrompt. Null means "use the global default".
  currentSystemPromptOverride: string | null;
}

export const aiState = reactive<AIState>({
  messages: [],
  streaming: false,
  error: null,
  context: null,
  currentConversationId: null,
  currentConversationTitle: null,
  mentionedFiles: [],
  currentSystemPromptOverride: null,
});

// Track event listener cleanup
let eventListenersRegistered = false;
let pendingAssistantMessage: ChatMessage | null = null;

// N-149: Wails Events.On returns a cancel function. We collect them so
// they can be torn down on hot-reload (dev) or in tests to avoid leaking
// duplicate listeners across module re-imports.
const aiEventCancellers: Array<() => void> = [];

/**
 * Registers Wails event listeners for AI streaming.
 * Called once at module initialization.
 */
function ensureEventListeners(): void {
  if (eventListenersRegistered) return;
  eventListenersRegistered = true;

  // N-44: typed event payloads (was `any`).
  aiEventCancellers.push(
    Events.On("ai:chunk", (event: AIChunkEvent) => {
      const chunk = event?.data ?? "";
      if (pendingAssistantMessage && typeof chunk === "string") {
        pendingAssistantMessage.content += chunk;
      }
    }),
  );

  aiEventCancellers.push(
    Events.On("ai:done", (event: AIDoneEvent) => {
      // Backend emits the finish reason as data (may be empty string).
      void event;
      const lastContent = pendingAssistantMessage?.content ?? "";
      if (pendingAssistantMessage && lastContent === "") {
        // Remove empty assistant message if nothing was streamed
        const idx = aiState.messages.indexOf(pendingAssistantMessage);
        if (idx >= 0) aiState.messages.splice(idx, 1);
      }
      pendingAssistantMessage = null;
      aiState.streaming = false;
      void persistConversation();
      // Agent mode: parse tool calls from the just-finished assistant message
      // and add them to the pending approval queue.
      if (agentState.mode === "agent" && lastContent) {
        onAssistantFinished(lastContent);
      }
    }),
  );

  aiEventCancellers.push(
    Events.On("ai:error", (event: AIErrorEvent) => {
      const errMsg = event?.data ?? "AI request failed";
      aiState.error = typeof errMsg === "string" ? errMsg : "AI request failed";
      notifyError(errMsg, "AI Error");
      pushOutput("ai", "error", `AI error: ${aiState.error}`);
      if (pendingAssistantMessage && pendingAssistantMessage.content === "") {
        const idx = aiState.messages.indexOf(pendingAssistantMessage);
        if (idx >= 0) aiState.messages.splice(idx, 1);
      }
      pendingAssistantMessage = null;
      aiState.streaming = false;
    }),
  );
}

/**
 * N-149: Cancels all AI event listeners. Intended for HMR teardown in dev
 * and test cleanup. After calling this, ensureEventListeners() can be
 * invoked again to re-register fresh listeners.
 */
export function cleanupAIEventListeners(): void {
  for (const cancel of aiEventCancellers) {
    try {
      cancel();
    } catch {
      // ignore — listener already removed
    }
  }
  aiEventCancellers.length = 0;
  eventListenersRegistered = false;
}

// Register listeners at module load
ensureEventListeners();

/**
 * Builds the user message including context if attached.
 */
function buildUserMessage(content: string): string {
  let prefix = "";
  if (aiState.mentionedFiles.length > 0) {
    prefix += "Referenced files:\n\n";
    for (const file of aiState.mentionedFiles) {
      prefix += `File: ${file.filePath}\n\`\`\`${file.language}\n${file.content}\n\`\`\`\n\n`;
    }
    prefix += "---\n\n";
  }
  if (aiState.context) {
    const ctx = aiState.context;
    if (ctx.kind === "selection") {
      prefix += `File: ${ctx.filePath}\nSelected code (${ctx.startLine}-${ctx.endLine}):\n\`\`\`${ctx.language}\n${ctx.content}\n\`\`\`\n\n`;
    } else {
      prefix += `File: ${ctx.filePath}\n\`\`\`${ctx.language}\n${ctx.content}\n\`\`\`\n\n`;
    }
  }
  return prefix + content;
}

/**
 * Sends a user message. Respects attached context and persists the conversation.
 * Uses event-based streaming (ai:chunk, ai:done, ai:error events from backend).
 */
export async function sendMessage(content: string): Promise<void> {
  if (aiState.streaming) return;

  aiState.error = null;
  const fullContent = buildUserMessage(content);
  aiState.messages.push({ role: "user", content: fullContent });
  clearMentionedFiles();
  aiState.streaming = true;

  const assistantMessage: ChatMessage = { role: "assistant", content: "" };
  aiState.messages.push(assistantMessage);
  pendingAssistantMessage = assistantMessage;

  try {
    // In agent mode, use the agent system prompt instead of the user's
    // configured prompt. The agent prompt defines the tool-call protocol.
    // Project rules (#25) are appended to whichever prompt is in use so the
    // AI obeys project conventions even in agent mode.
    // N-60: In chat mode, if the conversation has a per-session system
    // prompt override, use it instead of the global appState.aiSystemPrompt.
    // N-59: When no custom prompt is set (neither override nor global),
    // fall back to the localized default from the i18n dictionary so zh/ja
    // users get a prompt in their language. The English i18n entry matches
    // the Go const in ai_prompts.go, so en users see the same prompt.
    let basePrompt: string;
    if (agentState.mode === "agent") {
      basePrompt = await getAgentSystemPrompt();
    } else if (aiState.currentSystemPromptOverride) {
      basePrompt = aiState.currentSystemPromptOverride;
    } else if (appState.aiSystemPrompt) {
      basePrompt = appState.aiSystemPrompt;
    } else {
      // N-59: localized default system prompt
      basePrompt = translate("prompts.defaultSystem");
    }
    const systemPrompt = basePrompt + rulesForPrompt.value;
    // Pass temperature and protocol from the active AI provider config so the
    // backend uses the right request shape (OpenAI vs Anthropic) and sampling.
    const activeCfg = activeAIConfig();
    aiService.setConfig({
      apiKey: appState.aiApiKey,
      baseUrl: appState.aiBaseUrl,
      model: appState.aiModel,
      systemPrompt,
      // Plan 54: pass prompt overrides so the backend's GetEffective*
      // methods return the user-configured prompt instead of the built-in.
      agentSystemPrompt: appState.aiAgentSystemPrompt,
      conversationTitlePrompt: appState.aiConversationTitlePrompt,
      inlineCompletionPrompt: appState.aiInlineCompletionPrompt,
      temperature: appState.temperature,
      protocol: activeCfg?.protocol ?? "openai",
      maxTokens: appState.maxTokens,
    });
    const history = aiState.messages.slice(0, -1);
    await aiService.startStream(history);
    // Stream continues async; ai:done/ai:error events handle completion
  } catch (e: unknown) {
    const msg = errorMessage(e) || "AI request failed";
    aiState.error = msg;
    notifyError(msg, "AI Error");
    if (assistantMessage.content === "") {
      aiState.messages.pop();
    }
    pendingAssistantMessage = null;
    aiState.streaming = false;
  }
}

/**
 * Stops an in-progress streaming request.
 */
export async function stopGeneration(): Promise<void> {
  try {
    await aiService.stopStream();
  } catch {
    // Ignore errors when stopping
  }
  aiState.streaming = false;
  pendingAssistantMessage = null;
}

/**
 * Attaches code context to the next message.
 */
export function attachContext(context: AIContextAttachment): void {
  aiState.context = context;
}

/**
 * Clears attached context.
 */
export function clearContext(): void {
  aiState.context = null;
}

/**
 * Adds a mentioned file to the AI context for the next message.
 */
export function addMentionedFile(entry: FileContextEntry): void {
  if (aiState.mentionedFiles.some(f => f.filePath === entry.filePath)) return;
  aiState.mentionedFiles.push(entry);
}

/**
 * Removes a mentioned file from the AI context by path.
 */
export function removeMentionedFile(filePath: string): void {
  const idx = aiState.mentionedFiles.findIndex(f => f.filePath === filePath);
  if (idx >= 0) aiState.mentionedFiles.splice(idx, 1);
}

/**
 * Clears all mentioned files.
 */
export function clearMentionedFiles(): void {
  aiState.mentionedFiles = [];
}

/**
 * Runs a preset AI action on the given code.
 * Fetches the instruction template from the backend (centralized prompt management).
 */
export async function runAIAction(
  action: AIActionName,
  code: string,
  language: string,
  filePath: string,
): Promise<void> {
  let instruction: string;
  try {
    instruction = await aiService.getPresetPrompt(action);
  } catch {
    instruction = action.replace(/_/g, " ");
  }
  const context: AIContextAttachment = {
    kind: "selection",
    filePath,
    language,
    content: code,
  };
  attachContext(context);
  await sendMessage(instruction);
  clearContext();
}

/**
 * Clears all messages and starts fresh.
 */
export function clearMessages(): void {
  if (aiState.streaming) return;
  aiState.messages = [];
  aiState.error = null;
  aiState.currentConversationId = null;
  aiState.currentConversationTitle = null;
  aiState.context = null;
  // N-60: Reset per-session system prompt override.
  aiState.currentSystemPromptOverride = null;
}

/**
 * Persists the current conversation to disk.
 */
async function persistConversation(): Promise<void> {
  if (aiState.messages.length === 0) return;
  try {
    const wasNew = !aiState.currentConversationId;
    let id = aiState.currentConversationId;
    if (!id) {
      id = await conversationService.generateId();
      aiState.currentConversationId = id;
    }
    // Generate a title only for new conversations; reuse the existing title
    // for subsequent persists to avoid redundant API calls and title churn.
    let title = aiState.currentConversationTitle;
    if (wasNew || !title) {
      const firstUser = aiState.messages.find((m) => m.role === "user");
      if (firstUser) {
        // Try AI-powered title generation first (Plan 52/53); fall back to
        // the legacy truncation heuristic on error or when no API key is set.
        try {
          title = await aiService.generateTitleWithAI(firstUser.content.slice(0, 500));
        } catch {
          title = await conversationService.generateTitle(firstUser.content.slice(0, 200));
        }
      } else {
        title = "(empty)";
      }
      aiState.currentConversationTitle = title;
    }
    await conversationService.save({
      id,
      title,
      created_at: Math.floor(Date.now() / 1000),
      messages: aiState.messages.map((m) => ({ role: m.role, content: m.content })),
      // N-60: Persist the per-conversation system prompt override so it
      // survives across sessions. Empty string is omitted (omitempty).
      system_prompt_override: aiState.currentSystemPromptOverride ?? undefined,
    });
  } catch (e) {
    console.error("Failed to persist conversation:", e);
  }
}

/**
 * Loads a saved conversation into the chat.
 */
export async function loadConversation(id: string): Promise<void> {
  try {
    const conv = await conversationService.load(id);
    aiState.messages = conv.messages.map((m) => ({ role: m.role as "user" | "assistant" | "system", content: m.content }));
    aiState.currentConversationId = conv.id;
    aiState.currentConversationTitle = conv.title;
    // N-60: Restore the per-conversation system prompt override.
    aiState.currentSystemPromptOverride = conv.system_prompt_override ?? null;
    aiState.error = null;
  } catch (e: unknown) {
    aiState.error = errorMessage(e) || "Failed to load conversation";
  }
}

/**
 * Rename the current conversation. Updates the backend.
 * Returns true on success, false on failure.
 */
export async function renameConversation(id: string, newTitle: string): Promise<boolean> {
  const trimmed = newTitle.trim();
  if (!trimmed) return false;
  try {
    await conversationService.updateTitle(id, trimmed);
    aiState.currentConversationTitle = trimmed;
    return true;
  } catch (e) {
    notifyError(`Failed to rename conversation: ${e instanceof Error ? e.message : String(e)}`);
    return false;
  }
}

/**
 * N-60: Sets or clears the per-conversation system prompt override.
 * When set to a non-empty string, subsequent sendMessage calls in chat
 * mode will use this prompt instead of the global appState.aiSystemPrompt.
 * Pass null or empty string to reset to the global default.
 * The override is persisted with the conversation on the next save.
 */
export function setSystemPromptOverride(prompt: string | null): void {
  const trimmed = prompt?.trim() ?? "";
  aiState.currentSystemPromptOverride = trimmed === "" ? null : trimmed;
}