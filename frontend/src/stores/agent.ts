// Agent store: manages Agent mode state, tool-call parsing, and the
// approval→execute→feed-back loop. The AI emits tool calls as fenced code
// blocks whose first line is `read:`, `write:`, `run:`, or `search:`
// followed by the target. See AgentSystemPrompt in services/ai_prompts.go.
import { reactive, computed, ref } from "vue";
import { fileService, searchService, agentService, aiService } from "@/api/services";
import { appState } from "@/stores/app";
import { pushOutput } from "@/stores/output";
import { notifyError, notifySuccess, notifyWarning } from "@/lib/notifications";
import { errorMessage } from "@/lib/errors";
import type { RiskLevel, ApprovalPolicy } from "@/types";

export type AgentMode = "chat" | "agent";
// ToolCallKind is now `string` (N-16) to allow custom tools registered via
// registerTool(). BuiltinToolKind is exported for code that only handles the
// four built-in tools.
export type BuiltinToolKind = "read" | "write" | "run" | "search";
export type ToolCallKind = string;
export type ToolCallStatus =
  | "pending"
  | "approved"
  | "rejected"
  | "executed"
  | "error";

// MAX_TOOL_CALLS is the per-conversation tool call threshold (N-10).
// When the total number of tool calls reaches this limit, the agent
// warns the user that the conversation has grown long and suggests
// starting a new session to avoid token exhaustion and runaway API
// costs. The user can still approve additional calls, but the warning
// is surfaced each time new calls are emitted beyond the limit.
const MAX_TOOL_CALLS = 20;

export interface ToolCall {
  id: string;
  kind: ToolCallKind;
  // For read/write/run/search: the path or command or query on the first line.
  target: string;
  // For write: the full file content (rest of the code block).
  content?: string;
  status: ToolCallStatus;
  // Human-readable result summary, populated after execution.
  result?: string;
  // Error message, populated when status === "error".
  error?: string;
  // Risk level for `run` tool calls, populated asynchronously by
  // checkRunRisk() after the tool call is added to the pending queue
  // (N-1). Used by the approval UI to show a risk badge.
  riskLevel?: RiskLevel;
  // Block reason when the command matches the denylist (N-1).
  blockReason?: string;
  // Plan 47: in-flight risk check promise. Used by applyApprovalPolicy to
  // await the risk check without re-triggering it. Not persisted.
  _riskCheckPromise?: Promise<void>;
}

interface AgentStoreState {
  mode: AgentMode;
  pendingToolCalls: ToolCall[];
  // Total tool calls emitted in the current conversation (N-10).
  // Incremented in onAssistantFinished, reset in clearPendingToolCalls.
  toolCallCount: number;
}

export const agentState = reactive<AgentStoreState>({
  mode: "chat",
  pendingToolCalls: [],
  toolCallCount: 0,
});

export const isAgentMode = computed(() => agentState.mode === "agent");
export const hasPendingToolCalls = computed(
  () => agentState.pendingToolCalls.some((tc) => tc.status === "pending"),
);
export const maxIterationsReached = computed(
  () => agentState.toolCallCount >= MAX_TOOL_CALLS,
);

export function setMode(mode: AgentMode): void {
  agentState.mode = mode;
}

export function toggleMode(): void {
  agentState.mode = agentState.mode === "chat" ? "agent" : "chat";
  // Clear pending approvals when switching modes.
  agentState.pendingToolCalls = [];
}

let toolCallCounter = 0;
function nextToolCallId(): string {
  toolCallCounter += 1;
  return `tc-${Date.now().toString(36)}-${toolCallCounter}`;
}

// Cache the agent system prompt so we don't round-trip to the backend on
// every send. Fetched lazily on first use. Declared early (before
// registerTool) so that registerTool can safely invalidate it without hitting
// the temporal dead zone (N-16).
let agentSystemPromptCache: string | null = null;

// Tool call block regex. Matches fenced code blocks where the first line is
// `kind: target`. Supports both ``` and ~~~ fences, optional language tag.
// The opening fence is captured in group 1 and referenced via \1 backreference
// so the closing fence matches the opening one. The regex captures:
//   group 1 = fence (``` or ~~~)
//   group 2 = kind (e.g. read|write|run|search, plus any custom tools)
//   group 3 = target (rest of first line)
//   group 4 = content (rest of block, may be undefined)
//   group 5 = same as group 4 (kept for legacy index compatibility)
//
// N-16: The regex is built dynamically from the toolRegistry so that custom
// tools (registered via registerTool) are automatically recognized by the
// parser without code changes. The regex is cached and rebuilt only when the
// set of registered tools changes.

function escapeRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

let cachedToolCallRe: RegExp | null = null;

function getToolCallRegex(): RegExp {
  if (cachedToolCallRe !== null) return cachedToolCallRe;
  const kinds = Array.from(toolRegistry.keys());
  if (kinds.length === 0) {
    // No tools registered — a regex that never matches.
    cachedToolCallRe = /(?!)/g;
    return cachedToolCallRe;
  }
  const kindsPattern = kinds.map(escapeRegex).join("|");
  cachedToolCallRe = new RegExp(
    "(?:^|\\n)(```|~~~)[a-zA-Z]*\\n(" + kindsPattern + "):\\s*(.+?)(\\n([\\s\\S]*?))?\\1",
    "g",
  );
  return cachedToolCallRe;
}

function invalidateToolCallRegex(): void {
  cachedToolCallRe = null;
}

/**
 * parseToolCalls scans an assistant message for tool-call fenced blocks and
 * returns them as ToolCall objects. Non-matching code blocks are ignored
 * (they are normal code suggestions the user can apply manually).
 */
export function parseToolCalls(message: string): ToolCall[] {
  if (!message) return [];
  const calls: ToolCall[] = [];
  let match: RegExpExecArray | null;
  const re = getToolCallRegex();
  // Reset regex state (it's a global regex with /g flag).
  re.lastIndex = 0;
  while ((match = re.exec(message)) !== null) {
    const kind = match[2] as ToolCallKind;
    const target = match[3].trim();
    // match[5] is undefined when there's no newline after the target,
    // or "" when the content block is empty. Strip a trailing newline
    // (always present when content is non-empty due to the closing fence).
    const rawContent = match[5];
    const content =
      rawContent && rawContent.length > 0
        ? rawContent.replace(/\n+$/, "")
        : undefined;
    calls.push({
      id: nextToolCallId(),
      kind,
      target,
      content,
      status: "pending",
    });
  }
  return calls;
}

/**
 * extractToolCallBlocks returns the tool-call code blocks found in a message,
 * used by the UI to render approval cards. Also returns the message with
 * tool-call blocks removed (so they don't render as normal code blocks).
 */
export function extractToolCallBlocks(
  message: string,
): { toolCalls: ToolCall[]; cleanedMessage: string } {
  const toolCalls = parseToolCalls(message);
  // Remove the tool-call blocks from the rendered message.
  const re = getToolCallRegex();
  re.lastIndex = 0;
  const cleanedMessage = message.replace(re, "").trim();
  return { toolCalls, cleanedMessage };
}

/**
 * resolveProjectPath validates that `target` is a relative path within the
 * currently open project root, and returns the absolute path. Rejects absolute
 * paths and parent-traversal paths before they reach FileService, so the AI
 * gets a clear, structured error instead of a low-level validation failure
 * (#25 / N-3).
 */
function resolveProjectPath(
  target: string,
): { ok: true; absPath: string } | { ok: false; error: string } {
  const root = appState.currentProject;
  if (!root) {
    return { ok: false, error: "No project open" };
  }
  // Reject absolute paths (Windows drive letter or POSIX root).
  if (/^([a-zA-Z]:[\\/]|[\\/])/.test(target)) {
    return {
      ok: false,
      error: `Absolute paths are not allowed: ${target}. Use a path relative to the project root.`,
    };
  }
  // Normalize and reject parent traversal that escapes the root.
  const parts = target.replace(/\\/g, "/").split("/");
  const normalized: string[] = [];
  for (const p of parts) {
    if (p === "." || p === "") continue;
    if (p === "..") {
      if (normalized.length === 0) {
        return { ok: false, error: `Path escapes project root: ${target}` };
      }
      normalized.pop();
    } else {
      normalized.push(p);
    }
  }
  const relPath = normalized.join("/");
  if (!relPath) {
    return { ok: false, error: `Empty path after normalization: ${target}` };
  }
  const absPath = root.replace(/[\\/]+$/, "") + "/" + relPath;
  return { ok: true, absPath };
}

/**
 * ToolExecutor is the signature of a registered tool's execute function.
 * It receives the parsed ToolCall and returns a string observation to feed
 * back to the AI. Errors should be thrown; the caller (approveToolCall)
 * catches them and marks the call as failed.
 */
export type ToolExecutor = (tc: ToolCall) => Promise<string>;

/**
 * ToolSchema describes a tool's metadata for the AI prompt and UI (N-16).
 * The AI system prompt includes the list of registered tools with their
 * descriptions and danger levels so the model knows what tools are available.
 * The approval UI uses the danger level as a default risk badge for tools
 * that don't have a runtime risk classification (e.g. non-`run` tools).
 */
export interface ToolSchema {
  // Human-readable description of what the tool does, shown to the AI and UI.
  description: string;
  // Default danger level for this tool kind. `run` tools get a runtime risk
  // level from CheckCommand; other tools use this default for the UI badge.
  dangerLevel?: RiskLevel;
}

/**
 * ToolDef describes a registered agent tool. Custom tools (N-16) register
 * via the same shape, extending the toolRegistry Map. The `schema` field
 * provides metadata for the AI system prompt and approval UI.
 */
export interface ToolDef {
  kind: string;
  schema: ToolSchema;
  execute: ToolExecutor;
}

/**
 * toolRegistry maps tool kinds to their executors. Built-in tools are
 * registered here so that future custom tools (from plugins or config) can
 * be added via `registerTool` without modifying the dispatch logic (#25 / N-16).
 */
const toolRegistry = new Map<string, ToolDef>();

// N-151: toolRegistry itself is a plain Map (not reactive). To let UI
// consumers track registrations that happen after mount (e.g. plugin tools
// loaded asynchronously), we expose a reactive version counter. Reads of
// getRegisteredTools() / listRegisteredTools() touch it, so any computed
// that calls them re-evaluates when a tool is registered or unregistered.
const toolRegistryVersion = ref(0);

/**
 * registerTool adds (or replaces) a tool in the registry. Exposed so that
 * future plugin/config code can register custom agent tools. Invalidates the
 * regex and prompt caches so the new tool is immediately recognized by the
 * parser and included in the AI system prompt (N-16).
 */
export function registerTool(def: ToolDef): void {
  toolRegistry.set(def.kind, def);
  invalidateToolCallRegex();
  __resetAgentPromptCacheForTests();
  toolRegistryVersion.value++;
}

/**
 * unregisterTool removes a tool from the registry. Returns true if a tool
 * was removed, false if the kind was not registered. Invalidates caches.
 */
export function unregisterTool(kind: string): boolean {
  const removed = toolRegistry.delete(kind);
  if (removed) {
    invalidateToolCallRegex();
    __resetAgentPromptCacheForTests();
    toolRegistryVersion.value++;
  }
  return removed;
}

/**
 * listRegisteredTools returns the kinds of all currently registered tools.
 * Used by the UI to show available tools and by tests to verify registration.
 */
export function listRegisteredTools(): string[] {
  void toolRegistryVersion.value; // N-151: track reactive dependency
  return Array.from(toolRegistry.keys());
}

/**
 * getRegisteredTools returns the full ToolDef objects for all registered
 * tools. Used by the UI to display tool metadata (description, danger level)
 * and by getToolSchemaList to build the AI system prompt (N-16).
 */
export function getRegisteredTools(): ToolDef[] {
  void toolRegistryVersion.value; // N-151: track reactive dependency
  return Array.from(toolRegistry.values());
}

/**
 * getToolSchemaList builds a markdown-formatted list of all registered tools
 * for inclusion in the AI system prompt (N-16). The AI uses this list to know
 * which tools it can emit and what each tool does.
 */
export function getToolSchemaList(): string {
  const tools = getRegisteredTools();
  if (tools.length === 0) return "";
  const lines = tools.map((t) => {
    let line = `- \`${t.kind}:\` ${t.schema.description}`;
    if (t.schema.dangerLevel) {
      line += ` (risk: ${t.schema.dangerLevel})`;
    }
    return line;
  });
  return "Available tools:\n" + lines.join("\n");
}

// --- Built-in tool executors ---

async function executeReadTool(tc: ToolCall): Promise<string> {
  const resolved = resolveProjectPath(tc.target);
  if (!resolved.ok) {
    throw new Error(resolved.error);
  }
  const content = await fileService.readFile(resolved.absPath);
  // Truncate very large files so we don't blow the context window.
  const max = 8000;
  const truncated =
    content.length > max
      ? content.slice(0, max) + `\n... [truncated, ${content.length} total chars]`
      : content;
  return `Read ${tc.target}:\n\`\`\`\n${truncated}\n\`\`\``;
}

async function executeWriteTool(tc: ToolCall): Promise<string> {
  if (!tc.content) {
    throw new Error("write tool call missing file content");
  }
  const resolved = resolveProjectPath(tc.target);
  if (!resolved.ok) {
    throw new Error(resolved.error);
  }
  await fileService.writeFile(resolved.absPath, tc.content);
  notifySuccess(`Wrote ${tc.target}`);
  return `Wrote ${tc.target} (${tc.content.length} chars).`;
}

async function executeRunTool(tc: ToolCall): Promise<string> {
  const cwd = appState.currentProject ?? "";
  const result = await agentService.execCommand(tc.target, cwd);
  const summary =
    `Ran: ${result.command}\n` +
    `Exit code: ${result.exitCode} (${result.durationMs}ms)\n` +
    (result.stdout ? `stdout:\n\`\`\`\n${result.stdout}\n\`\`\`\n` : "") +
    (result.stderr ? `stderr:\n\`\`\`\n${result.stderr}\n\`\`\`\n` : "");
  pushOutput(
    "agent",
    result.exitCode === 0 ? "info" : "warn",
    `Agent ran "${result.command}" → exit ${result.exitCode}`,
  );
  return summary;
}

async function executeSearchTool(tc: ToolCall): Promise<string> {
  const root = appState.currentProject ?? "";
  const results = await searchService.search(root, tc.target, true);
  // Flatten results: each SearchResult has a path + matches[].
  const allMatches: { path: string; line: number; column: number; preview: string }[] = [];
  for (const r of results) {
    for (const m of r.matches) {
      allMatches.push({ path: r.path, line: m.line, column: m.column, preview: m.preview });
    }
  }
  if (allMatches.length === 0) {
    return `No matches found for "${tc.target}".`;
  }
  const maxResults = 10;
  const top = allMatches.slice(0, maxResults);
  const summary =
    `Found ${allMatches.length} match(es) for "${tc.target}" (showing top ${top.length}):\n` +
    top
      .map(
        (m) =>
          `- ${m.path}:${m.line}:${m.column}: ${m.preview.trim()}`,
      )
      .join("\n");
  return summary;
}

// Register built-in tools. Done at module load so they're available
// immediately. Custom tools can be registered later via registerTool().
// Each tool includes a schema (N-16) with a description and default danger
// level used in the AI system prompt and approval UI.
registerTool({
  kind: "read",
  schema: {
    description: "Read a file from the project. Target is a path relative to the project root.",
    dangerLevel: "safe",
  },
  execute: executeReadTool,
});
registerTool({
  kind: "write",
  schema: {
    description: "Write or overwrite a file in the project. Target is a relative path, content is the file body.",
    dangerLevel: "elevated",
  },
  execute: executeWriteTool,
});
registerTool({
  kind: "run",
  schema: {
    description: "Execute a shell command in the project root. Subject to sandbox validation and risk classification.",
    dangerLevel: "elevated",
  },
  execute: executeRunTool,
});
registerTool({
  kind: "search",
  schema: {
    description: "Search for a text pattern across the project. Target is the search query.",
    dangerLevel: "safe",
  },
  execute: executeSearchTool,
});

/**
 * executeToolCall runs the given tool call and returns a string summary that
 * should be fed back to the AI as the "observation" in the agent loop.
 * Dispatches via the toolRegistry Map (#25 / N-16).
 */
export async function executeToolCall(tc: ToolCall): Promise<string> {
  const def = toolRegistry.get(tc.kind);
  if (!def) {
    throw new Error(`unknown tool call kind: ${tc.kind}`);
  }
  return def.execute(tc);
}

/**
 * approveToolCall executes the tool call and returns the observation string.
 * The caller (AiChatPanel) is responsible for feeding it back to the AI.
 * Updates the tool call's status and result fields in place.
 */
export async function approveToolCall(
  tc: ToolCall,
): Promise<string | null> {
  tc.status = "approved";
  try {
    const observation = await executeToolCall(tc);
    tc.status = "executed";
    tc.result = observation;
    return observation;
  } catch (e: unknown) {
    tc.status = "error";
    tc.error = errorMessage(e);
    notifyError(`Tool call failed: ${tc.error}`);
    return `Error executing ${tc.kind} on "${tc.target}": ${tc.error}`;
  }
}

/**
 * rejectToolCall marks the tool call as rejected and returns a message the
 * caller can feed back to the AI so it knows the action was not performed.
 */
export function rejectToolCall(tc: ToolCall): string {
  tc.status = "rejected";
  return `User rejected the ${tc.kind} action on "${tc.target}". Choose a different approach or ask the user for guidance.`;
}

/**
 * clearPendingToolCalls removes all tool calls (e.g. when starting a new
 * conversation or switching modes).
 */
export function clearPendingToolCalls(): void {
  agentState.pendingToolCalls = [];
  agentState.toolCallCount = 0;
}

// --- Agent loop wiring ---

/**
 * getAgentSystemPrompt returns the agent system prompt, fetching it from the
 * backend on first call and caching the result. Appends the list of
 * registered tools (N-16) so the AI knows what tools are available. The cache
 * is invalidated when tools are registered/unregistered.
 *
 * Plan 54: when the user has configured an agent prompt override
 * (appState.aiAgentSystemPrompt), that string is used as the base instead of
 * the built-in const. The override is read fresh on every call so settings
 * changes take effect on the next message without needing cache invalidation.
 */
export async function getAgentSystemPrompt(): Promise<string> {
  // Plan 54: user override takes precedence and is NOT cached (so settings
  // changes apply immediately). The built-in fetch is cached as before.
  const override = appState.aiAgentSystemPrompt;
  if (override && override.trim() !== "") {
    const toolList = getToolSchemaList();
    return override + (toolList ? "\n\n" + toolList : "");
  }
  if (agentSystemPromptCache !== null) return agentSystemPromptCache;
  try {
    const base = await aiService.getAgentSystemPrompt();
    const toolList = getToolSchemaList();
    agentSystemPromptCache = base + (toolList ? "\n\n" + toolList : "");
  } catch {
    // N-59: Fall back to the localized agent prompt from i18n so zh/ja
    // users get a prompt in their language when the backend is unavailable.
    const { translate } = await import("@/lib/i18n");
    const toolList = getToolSchemaList();
    agentSystemPromptCache = translate("prompts.agentSystem") + (toolList ? "\n\n" + toolList : "");
  }
  return agentSystemPromptCache;
}

/**
 * Resets the agent system prompt cache. Exposed for test isolation only.
 * @internal
 */
export function __resetAgentPromptCacheForTests(): void {
  agentSystemPromptCache = null;
}

/**
 * getApprovalPolicy returns the configured approval policy for a tool kind
 * (Plan 47). Reads from appState.toolApprovalConfig; missing entries default
 * to "always-ask". Exposed for tests and the settings UI.
 */
export function getApprovalPolicy(kind: ToolCallKind): ApprovalPolicy {
  const cfg = appState.toolApprovalConfig[kind];
  if (cfg === "auto-approve" || cfg === "never-approve") return cfg;
  return "always-ask";
}

/**
 * shouldAutoApprove determines whether a tool call should be auto-approved
 * based on the configured policy. `run` tools are never auto-approved when
 * the command is blocked by the denylist (the user must see the block
 * reason). For other kinds, the policy applies directly.
 */
export function shouldAutoApprove(tc: ToolCall): boolean {
  if (getApprovalPolicy(tc.kind) !== "auto-approve") return false;
  // `run` tools: respect the denylist. Blocked commands stay in the
  // pending queue so the user sees the block reason.
  if (tc.kind === "run" && tc.blockReason) return false;
  return true;
}

/**
 * applyApprovalPolicy applies the configured approval policy to a pending
 * tool call (Plan 47). Called fire-and-forget from onAssistantFinished.
 *
 * - "auto-approve": executes the call and feeds the observation back to
 *   the AI without waiting for user interaction. Blocked `run` commands
 *   fall back to "always-ask" so the user sees the block reason.
 * - "never-approve": rejects the call and feeds the rejection back.
 * - "always-ask": no-op (the call stays in the pending queue).
 *
 * For `run` tools, this waits for the risk check to complete before
 * applying the policy, so the denylist has a chance to block auto-approval.
 */
export async function applyApprovalPolicy(tc: ToolCall): Promise<void> {
  // For `run` tools, wait for the risk check so we can respect the denylist.
  if (tc.kind === "run" && tc._riskCheckPromise) {
    await tc._riskCheckPromise;
  }
  if (tc.status !== "pending") return;
  const policy = getApprovalPolicy(tc.kind);
  if (policy === "auto-approve") {
    // Blocked `run` commands stay pending for user visibility.
    if (tc.kind === "run" && tc.blockReason) return;
    pushOutput(
      "agent",
      "info",
      `Auto-approving ${tc.kind} tool call: ${tc.target}`,
    );
    await approveAndFeed(tc);
  } else if (policy === "never-approve") {
    pushOutput(
      "agent",
      "info",
      `Auto-rejecting ${tc.kind} tool call per policy: ${tc.target}`,
    );
    await rejectAndFeed(tc);
  }
}

/**
 * onAssistantFinished should be called by the ai store when an assistant
 * message finishes streaming in agent mode. Parses tool calls from the
 * message and appends them to the pending queue for user approval.
 *
 * Returns the number of tool calls added (0 if none).
 */
export function onAssistantFinished(assistantContent: string): number {
  if (!assistantContent) return 0;
  const calls = parseToolCalls(assistantContent);
  if (calls.length === 0) return 0;
  agentState.pendingToolCalls.push(...calls);
  agentState.toolCallCount += calls.length;
  // Asynchronously classify risk level for `run` tool calls so the
  // approval UI can show a risk badge (N-1). Best-effort — if the
  // check fails, the risk level stays undefined and the UI shows no
  // badge. The promise is stored on the tool call so applyApprovalPolicy
  // can await it without re-triggering the check (Plan 47).
  for (const tc of calls) {
    if (tc.kind === "run" && tc.status === "pending") {
      tc._riskCheckPromise = checkRunRisk(tc);
    }
  }
  pushOutput(
    "agent",
    "info",
    `Agent emitted ${calls.length} tool call(s) awaiting approval`,
  );
  // N-10: warn the user when the conversation has accumulated too many
  // tool calls. The user can still approve, but the warning surfaces
  // the risk of token exhaustion and runaway API costs.
  if (maxIterationsReached.value) {
    notifyWarning(
      `Agent has emitted ${agentState.toolCallCount} tool calls. Consider starting a new conversation to avoid token exhaustion.`,
    );
    pushOutput(
      "agent",
      "warn",
      `Max iteration threshold reached (${agentState.toolCallCount}/${MAX_TOOL_CALLS}). Consider starting a new conversation.`,
    );
  }
  // Plan 47 / N-45 (Proposal T): apply per-tool approval policy.
  // N-45 fix: Previously, `void applyApprovalPolicy(tc)` fired all calls
  // in parallel. But sendMessage has a `if (aiState.streaming) return;`
  // guard, so concurrent auto-approved calls would silently lose
  // observations — the second call's observation was never sent to the
  // model. Now we serialize: each applyApprovalPolicy fully completes
  // (including feeding the observation back) before the next starts.
  // The for-loop is wrapped in an async IIFE because onAssistantFinished
  // returns the call count synchronously.
  void (async () => {
    for (const tc of calls) {
      if (tc.status === "pending") {
        await applyApprovalPolicy(tc);
      }
    }
  })();
  return calls.length;
}

/**
 * checkRunRisk calls the backend CheckCommand method to classify the
 * risk level of a `run` tool call and updates the tool call in place
 * (N-1). This populates the riskLevel and blockReason fields used by
 * the approval UI.
 */
export async function checkRunRisk(tc: ToolCall): Promise<void> {
  try {
    const check = await agentService.checkCommand(tc.target);
    tc.riskLevel = check.riskLevel;
    if (check.blocked) {
      tc.blockReason = check.blockReason;
    }
  } catch {
    // Best-effort — leave riskLevel undefined on error.
  }
}

/**
 * feedObservation sends an observation (tool-call result) back to the AI as a
 * new user message, continuing the agent loop. Imported lazily to avoid a
 * circular dependency with the ai store.
 */
export async function feedObservation(observation: string): Promise<void> {
  // Inline dynamic import breaks the circular dep (ai.ts imports this module).
  const { sendMessage } = await import("@/stores/ai");
  await sendMessage(`[Observation]\n${observation}`);
}

/**
 * feedRejection sends a rejection message back to the AI so it knows the
 * action was not performed and can choose a different approach.
 */
export async function feedRejection(rejection: string): Promise<void> {
  const { sendMessage } = await import("@/stores/ai");
  await sendMessage(`[Rejection]\n${rejection}`);
}

/**
 * approveAndFeed approves a tool call, executes it, and feeds the observation
 * back to the AI. Designed to be called directly from UI handlers.
 */
export async function approveAndFeed(tc: ToolCall): Promise<void> {
  const observation = await approveToolCall(tc);
  if (observation !== null) {
    await feedObservation(observation);
  }
}

/**
 * rejectAndFeed rejects a tool call and feeds the rejection back to the AI.
 */
export async function rejectAndFeed(tc: ToolCall): Promise<void> {
  const rejection = rejectToolCall(tc);
  await feedRejection(rejection);
}
