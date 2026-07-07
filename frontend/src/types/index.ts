export interface DirEntry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modified: number;
}

export interface Project {
  id: string;
  name: string;
  path: string;
  createdAt: number;
  lastOpened: number;
}

/**
 * A keyboard shortcut's key combination (N-8). Used as the persisted shape
 * for custom overrides and as the comparison key for conflict detection.
 */
export interface ShortcutKeys {
  key: string;
  ctrl: boolean;
  shift: boolean;
  alt: boolean;
}

export interface Settings {
  language: string;
  theme: string;
  fontSize: number;
  fontFamily: string;
  tabSize: number;
  wordWrap: boolean;
  lineNumbers: boolean;
  minimap: boolean;
  aiApiKey: string;
  aiBaseUrl: string;
  aiModel: string;
  aiSystemPrompt: string;
  // Plan 54: optional user overrides for the other three built-in prompts.
  // Empty string means "use the built-in".
  aiAgentSystemPrompt?: string;
  aiConversationTitlePrompt?: string;
  aiInlineCompletionPrompt?: string;
  cursorBlinking: string;
  cursorStyle: string;
  bracketColorization: boolean;
  autoSave: boolean;
  autoSaveDelay: string;
  aiProvider: string;
  temperature: number;
  maxTokens: number;
  defaultShell: string;
  terminalFontSize: number;
  terminalCursorStyle: string;
  scrollback: number;
  uiDensity: string;
  fontSizeScaling: number;
  inlineCompletionEnabled: boolean;
  // N-8: user-customized keyboard shortcuts, keyed by shortcut label.
  // Missing entries fall back to the default binding.
  customShortcuts?: Record<string, ShortcutKeys>;
  // N-20: layout state.
  aiChatPosition?: "left" | "right";
  activityBarVisible?: boolean;
  // Agent approval policy per tool kind (Plan 47). Missing entries default
  // to "always-ask". Keys are tool kinds (read/write/run/search + custom).
  toolApprovalConfig?: ToolApprovalConfig;
  // Plan 48: accent theme persistence. Can be a built-in key
  // ("blue"/"teal"/.../"indigo") or "custom".
  accentTheme?: string;
  // Plan 48: custom accent theme definition. Set when accentTheme === "custom".
  customAccent?: CustomAccentTheme | null;
  // N-29: plugin sandbox mode. When true, plugins run in isolated Web
  // Workers. Defaults to true. NOT optional — false must round-trip.
  enablePluginSandbox: boolean;
  // Design language: "apple" (Apple Design Language, default) or
  // "claude" (Anthropic Claude warm-canvas editorial style).
  designLanguage?: "apple" | "claude";
  // Multi-provider AI configs (CC Switch-style). Each entry is a named
  // configuration with its own provider/apiKey/baseUrl/model/temperature/
  // maxTokens/systemPrompt. activeAIConfigId points at the currently
  // active config. The legacy single-config fields (aiApiKey/aiBaseUrl/
  // aiModel/aiProvider/temperature/maxTokens/aiSystemPrompt) mirror the
  // active config so existing AI call paths work unchanged.
  aiProviderConfigs?: AIProviderConfig[];
  activeAIConfigId?: string;
}

/**
 * A single named AI provider configuration (CC Switch-style multi-provider).
 * Users can save any number of these and switch between them from the chat
 * panel or settings page. The `protocol` field controls which HTTP API
 * shape the backend uses:
 *   - "openai" (default): /v1/chat/completions + Bearer auth
 *   - "anthropic": /v1/messages + x-api-key + anthropic-version
 */
export interface AIProviderConfig {
  id: string;
  name: string;
  provider: string;
  /** "openai" | "anthropic". Empty defaults to "openai". */
  protocol?: string;
  apiKey: string;
  baseUrl: string;
  model: string;
  temperature?: number;
  maxTokens?: number;
  systemPrompt?: string;
}

/**
 * A user-defined custom accent theme (Plan 48). The base `color` is used to
 * derive the 6 accent CSS tokens and register a Monaco theme. Any token
 * override takes precedence over the derived value.
 */
export interface CustomAccentTheme {
  /** Display name shown in the UI. */
  name: string;
  /** Base accent hex color (e.g. "#ff6b35"). */
  color: string;
  // Optional token overrides. If not set, derived from color at apply time.
  primary?: string;
  primaryHover?: string;
  primaryLight?: string;
  primaryContainer?: string;
  onPrimary?: string;
  onPrimaryContainer?: string;
}

// ApprovalPolicy controls whether a tool call requires user approval.
// - "always-ask": user must approve each call (default, safest).
// - "auto-approve": call executes immediately without user interaction.
// - "never-approve": call is automatically rejected.
export type ApprovalPolicy = "always-ask" | "auto-approve" | "never-approve";

// ToolApprovalConfig maps a tool kind to its approval policy.
export type ToolApprovalConfig = Record<string, ApprovalPolicy>;

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export interface GitFileChange {
  path: string;
  status: string;
}

export interface BranchInfo {
  name: string;
  ahead: number;
  behind: number;
}

export interface SearchMatch {
  line: number;
  column: number;
  preview: string;
}

export interface SearchResult {
  path: string;
  matches: SearchMatch[];
}

export interface ConversationMessage {
  role: string;
  content: string;
}

export interface Conversation {
  id: string;
  title: string;
  created_at: number;
  messages: ConversationMessage[];
  // N-60: Per-conversation system prompt override. When non-empty, this
  // conversation uses a custom system prompt instead of the global default.
  system_prompt_override?: string;
}

export type AIActionName =
  | "explain"
  | "refactor"
  | "fix"
  | "generate_docs"
  | "generate_tests"
  | "optimize"
  | "review"
  | "security"
  | "commit_message";

export interface PresetMeta {
  name: string;
  label: string;
  description: string;
  icon: string;
}

// PresetSource identifies where a preset was loaded from (N-17).
export type PresetSource = "builtin" | "project" | "user";

// PresetFile is the on-disk JSON format for a custom preset (N-17).
export interface PresetFile {
  name: string;
  label: string;
  description: string;
  icon?: string;
  prompt: string;
}

// PresetWithSource is a PresetFile annotated with its source layer.
export interface PresetWithSource extends PresetFile {
  source: PresetSource;
}

export interface Command {
  id: string;
  label: string;
  shortcut?: string;
  action: () => void;
}

export interface AIContextAttachment {
  kind: "file" | "selection";
  filePath: string;
  language: string;
  content: string;
  startLine?: number;
  endLine?: number;
}

export interface FileContextEntry {
  filePath: string;
  language: string;
  content: string;
}

/** Request payload for AI inline code completion. */
export interface CompletionRequest {
  prefix: string;
  suffix: string;
  language: string;
  filePath: string;
}

/** Response from AI inline code completion. */
export interface CompletionResponse {
  text: string;
}

/**
 * Raw completion response shape from the Wails binding. The Wails binding
 * returns PascalCase fields (e.g. `Text`), but some runtime environments
 * return camelCase. This interface lets us handle both without resorting to
 * `any` (#25 / N-5).
 */
export interface RawCompletionResponse {
  Text?: string;
  text?: string;
}


export interface BranchRef {
  name: string;
  isHead: boolean;
}

export interface ReplaceResult {
  replacements: number;
}

export interface TerminalSessionInfo {
  id: string;
  title: string;
  active: boolean;
}

export interface TaskDef {
  label: string;
  command: string;
  args?: string[];
  cwd?: string;
  shell?: boolean;
}

/** A single step in a multi-step workflow (N-19). */
export interface WorkflowStep {
  name: string;
  command: string;
  args?: string[];
  cwd?: string;
  dependsOn?: string[];
  condition?: string;
  /** When false, a non-zero exit code does not abort the workflow. Defaults to true. */
  expectSuccess?: boolean;
  /**
   * Proposal F (prompt-5.md): Output templates to extract from the
   * step's stdout. Each key becomes accessible as
   * `steps.<name>.outputs.<key>` in subsequent step conditions and
   * command templates.
   *
   * Supported template values:
   *   - "{{stdout}}" — the entire stdout (trimmed)
   *   - "{{regex:pattern}}" — first match of the regex pattern
   *     (capturing group 1 if present, else full match)
   *
   * Example:
   *   outputs:
   *     tag: "{{stdout}}"
   *     major: "{{regex:v(\d+)}}"
   */
  outputs?: Record<string, string>;
}

/** An event trigger that auto-runs a workflow (Proposal B). */
export interface WorkflowTrigger {
  /**
   * Event name. Supported:
   *   - "file-saved": runs when a file matching `glob` is saved (Proposal B)
   *   - "startup": runs once when the IDE finishes loading (Proposal J / prompt-4.md)
   *   - "workflow-completed": runs when another workflow finishes (Proposal R / N-58)
   */
  event: string;
  /** Glob pattern matched against the file path relative to project root. */
  glob?: string;
  /**
   * When event is "workflow-completed", restricts the trigger to fire only
   * when the completed workflow's name matches this field. Empty means any
   * workflow completion triggers this workflow. Proposal R / N-58.
   */
  workflowName?: string;
}

/** A multi-step workflow loaded from .nknk/workflows/*.yml (N-19). */
export interface WorkflowDef {
  name: string;
  description?: string;
  steps: WorkflowStep[];
  watch?: string[];
  /** Auto-trigger on IDE events like file-saved (Proposal B). */
  runOn?: WorkflowTrigger;
  source: string;
}

// N-55: Workflow validation result types.
export interface WorkflowValidationError {
  field: string;
  message: string;
}

export interface WorkflowValidationResult {
  workflowName: string;
  valid: boolean;
  errors?: WorkflowValidationError[];
}

/** Status of a workflow step during execution. */
export type WorkflowStepStatus =
  | "pending"
  | "running"
  | "success"
  | "failed"
  | "skipped";

/** Runtime state of a workflow step being executed. */
export interface WorkflowStepState {
  name: string;
  status: WorkflowStepStatus;
  output?: string;
  error?: string;
  startedAt?: number;
  finishedAt?: number;
  /**
   * Proposal F (prompt-5.md): Extracted output values keyed by the
   * template name from WorkflowStep.outputs. Populated after the step
   * completes successfully. Accessible as `steps.<name>.outputs.<key>`
   * in subsequent step conditions and command templates.
   */
  outputs?: Record<string, string>;
}

export type RiskLevel = "safe" | "elevated" | "dangerous";

export interface ExecResult {
  command: string;
  cwd: string;
  stdout: string;
  stderr: string;
  exitCode: number;
  durationMs: number;
  riskLevel: RiskLevel;
  blocked: boolean;
  blockReason?: string;
}

export interface CommandCheck {
  riskLevel: RiskLevel;
  blocked: boolean;
  blockReason?: string;
}

/** Project-level AI rules file loaded from disk (#25). */
export interface RulesFile {
  path: string;
  content: string;
  source: string;
}

/** A candidate location for a rules file, with existence flag. */
export interface RulesFileCandidate {
  path: string;
  source: string;
  exists: boolean;
}

/** A configurable rules file candidate (N-18). Paths may contain globs. */
export interface RulesCandidateConfig {
  path: string;
  source: string;
}

/**
 * Rules configuration (N-18). Controls which rule files are probed and how
 * multiple files are combined.
 *  - mode "first" (default): only the first existing file is used
 *  - mode "merge": all existing files are concatenated in priority order
 */
export interface RulesConfig {
  candidates?: RulesCandidateConfig[];
  mode?: string;
}

/** Source layer a RulesConfig was loaded from (N-18). */
export type RulesConfigSource = "builtin" | "user" | "project";

// ============================================================================
// Plan 49 — Plugin System
// ============================================================================

/**
 * Permission scope declared by a plugin manifest and enforced by the
 * frontend nknk.* API before privileged calls (Plan 49). The frontend
 * checks the plugin's declared permissions before each privileged API
 * call. "commands.register" and "views.register" are always allowed.
 */
export type PluginPermission =
  | "fs.read"
  | "fs.write"
  | "shell.exec"
  | "net"
  | "ai.send";

/**
 * A command contributed by a plugin to the command palette (Plan 49).
 * The plugin's main.js registers a handler via `nknk.commands.register`
 * for the same ID declared here.
 */
export interface PluginCommandContribution {
  id: string;
  title: string;
  category?: string;
  keybinding?: string;
  /**
   * When true, other plugins may invoke this command via
   * `nknk.commands.execute` (Proposal E). When false/unset (the
   * default), only the owning plugin may execute it; cross-plugin
   * callers get a permission error.
   */
  public?: boolean;
}

/**
 * A view contributed by a plugin. The plugin's main.js registers a
 * Vue component via `nknk.views.register` for the same ID declared
 * here. Location controls which dock the view appears in.
 */
export interface PluginViewContribution {
  id: string;
  title: string;
  location?: "sidebar" | "panel" | "statusbar";
}

/**
 * IDE contributions declared by a plugin manifest (Plan 49). Each
 * contribution kind maps to a `nknk.*` registration API.
 */
export interface PluginContribution {
  commands?: PluginCommandContribution[];
  views?: PluginViewContribution[];
}

/**
 * The parsed plugin.json descriptor (Plan 49). Mirrors the Go
 * PluginManifest struct exactly so the Wails binding round-trips
 * without field-name mapping.
 */
export interface PluginManifest {
  /** Manifest format version. Currently 1. 0/unset = v1 for backward compat. */
  schemaVersion?: number;
  name: string;
  version: string;
  description?: string;
  author?: string;
  /** URL to the plugin's source repository (Proposal D). */
  repository?: string;
  /** URL to the plugin's homepage/documentation (Proposal D). */
  homepage?: string;
  /** SPDX license identifier, e.g. "MIT" (Proposal D). */
  license?: string;
  /** Entry point .js file relative to the plugin directory. */
  main: string;
  permissions?: PluginPermission[];
  /**
   * Events that trigger activation. v1 supports "onStartup" and
   * "onCommand:<id>". "onLanguage:<id>" is reserved for future use.
   */
  activationEvents?: string[];
  contributes?: PluginContribution;
}

/** Discovery layer for an installed plugin (Plan 49). */
export type PluginSource = "user" | "project";

/**
 * Runtime descriptor for an installed plugin (Plan 49). Pairs the
 * parsed manifest with discovery metadata. Mirrors the Go PluginInfo
 * struct.
 */
export interface PluginInfo {
  manifest: PluginManifest;
  /** Absolute path to the plugin directory on disk. */
  path: string;
  source: PluginSource;
  enabled: boolean;
  /** True if the manifest's main file exists on disk. */
  mainExists: boolean;
}

// ============================================================================
// Plan 50 — Profile System
// ============================================================================

/**
 * A user profile (Plan 50). Each profile is a directory under
 * <configDir>/gugacode/profiles/<name>/ containing settings.json.
 * The active profile is the one currently loaded by SettingsService.
 */
export interface ProfileInfo {
  name: string;
  description?: string;
  createdAt?: number;
  modifiedAt?: number;
  active: boolean;
}

/**
 * Exported profile blob (Plan 50). The frontend serializes this as a
 * .json file for download. ImportProfile accepts the same shape.
 */
export interface ProfileExport {
  name: string;
  description?: string;
  settings: unknown;
  exportedAt: number;
}

// ============================================================================
// N-49 — Secrets cross-platform migration
// ============================================================================

/**
 * Describes a secret entry discovered in the platform keyring (macOS
 * Keychain / Linux libsecret). Returned by settingsService.listSecrets()
 * so the settings UI can show users what's stored and let them clean up
 * orphan entries left behind when AIApiKey was cleared.
 */
export interface SecretInfo {
  account: string;
  method: string;
  stored: boolean;
}

// ============================================================================
// Plan 72 / N-25 — Layout Engine
// ============================================================================

/** Split orientation: horizontal = side-by-side, vertical = stacked. */
export type LayoutOrientation = "horizontal" | "vertical";

/**
 * A leaf node in the layout tree. Holds a single view (identified by
 * viewId). When viewId is null, the leaf is empty and can receive a
 * new view via drag-drop or the view picker.
 */
export interface LayoutLeaf {
  id: string;
  type: "leaf";
  viewId: string | null;
}

/**
 * A split node in the layout tree. Contains 2+ children arranged
 * horizontally or vertically. The `sizes` array holds relative
 * proportions (percentages) that should sum to 100; if absent, children
 * share equal space.
 */
export interface LayoutSplit {
  id: string;
  type: "split";
  orientation: LayoutOrientation;
  children: LayoutNode[];
  /** Relative sizes (percentages) per child. Optional; defaults to equal. */
  sizes?: number[];
}

/** A node in the layout tree: either a leaf or a split. */
export type LayoutNode = LayoutLeaf | LayoutSplit;

/**
 * The complete layout tree state. The root is the top-level node
 * (typically a split containing the sidebar + editor area). The
 * activeLeafId tracks which leaf currently has focus.
 */
export interface LayoutTree {
  root: LayoutNode;
  activeLeafId: string | null;
}

// --- N-44 / Proposal N: Wails event payload typing ---
//
// Wails delivers backend-emitted events to the frontend via
// `Events.On(name, cb)`. The callback receives an object whose shape
// depends on the event channel. Previously every handler used
// `any` and relied on runtime `typeof` checks — losing all compile-
// time type safety. These generic types restore it.
//
// The canonical payload mapping is mirrored in services/events.go
// (Go-side documentation). Update both files together when adding a
// new event channel.

/**
 * Generic shape of a Wails runtime event delivered to `Events.On`
 * callbacks. `data` carries the backend-emitted payload; `name` is
 * the event channel (always equal to the first arg of `Events.On`).
 */
export interface WailsEvent<T> {
  data: T;
  name?: string;
}

/**
 * Per-channel payload types. Each alias documents the exact shape
 * that the Go backend emits via `app.Event.Emit(name, payload)`.
 *
 * - `ai:chunk`     — a string token from the streaming response
 * - `ai:done`      — the final finish-reason string (may be empty)
 * - `ai:error`     — the error message string
 * - `file:saved`   — the absolute path of the saved file
 * - `terminal:output` — { sessionId, data } for a single PTY write
 * - `terminal:exited` — { sessionId } when the PTY process exits
 * - `workflow:completed` — { name } when a workflow finishes (Proposal R)
 */
export type AIChunkEvent = WailsEvent<string>;
export type AIDoneEvent = WailsEvent<string>;
export type AIErrorEvent = WailsEvent<string>;
export type FileSavedEvent = WailsEvent<string>;
export interface TerminalOutputPayload {
  sessionId: string;
  data: string;
}
export type TerminalOutputEvent = WailsEvent<TerminalOutputPayload>;
export interface TerminalExitedPayload {
  sessionId: string;
}
export type TerminalExitedEvent = WailsEvent<TerminalExitedPayload>;
export interface WorkflowCompletedPayload {
  name: string;
  /** Whether the workflow completed successfully (no failed steps). */
  success: boolean;
  /** Chain depth — how many workflow-completed triggers led here. 0 = direct. */
  chainDepth: number;
}
export type WorkflowCompletedEvent = WailsEvent<WorkflowCompletedPayload>;