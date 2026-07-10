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
 * G-FEAT-01: A scaffolding template the New Project wizard can generate.
 * Mirrors the Go ProjectTemplate struct.
 */
export interface ProjectTemplate {
  id: string;
  name: string;
  description: string;
  language: string;
}

/**
 * G-FEAT-01: Request payload for creating a new project from a template.
 * Mirrors the Go CreateProjectRequest struct.
 */
export interface CreateProjectRequest {
  templateId: string;
  projectName: string;
  targetDir: string;
  moduleName: string;
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

export type AIWindowTheme =
  | "apple-dark"
  | "apple-light"
  | "claude-dark"
  | "claude-light"
  | "system";

export interface Settings {
  /** prompt-7 Task F: monotonic settings version for dual-window CAS. */
  version?: number;
  /** Write-intent only; not persisted. */
  expectedVersion?: number;
  language: string;
  theme: string;
  fontSize: number;
  fontFamily: string;
  tabSize: number;
  wordWrap: boolean;
  lineNumbers: boolean;
  minimap: boolean;
  aiApiKey: string;
  // G-SEC-07: the backend no longer returns the decrypted key in aiApiKey
  // (it is cleared to ""). aiApiKeyConfigured signals whether a key is stored,
  // and aiApiKeyStorageMethod labels how ("dpapi"/"aes"/"keyring"/"plain"/
  // "none"). The plaintext key never crosses the Wails binding.
  aiApiKeyConfigured?: boolean;
  aiApiKeyStorageMethod?: string;
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
  /** prompt-9 9-A: format via LSP before writing on save (default true). */
  formatOnSave?: boolean;
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
  /** G-FEAT-03: optional toolchain binary path overrides (e.g. { "golangci-lint": "/usr/local/bin/golangci-lint" }). */
  toolPaths?: Record<string, string>;
  /** Plan 11 Task 15: personalization config (background images, avatars, fonts, bubble styles). */
  personalization?: PersonalizationConfig;
  /** prompt-5 Task C: open AI companion OS window on startup (default false). */
  openAIWindowOnStartup?: boolean;
  /** Independent theme used only by the AI companion window. */
  aiWindowTheme?: AIWindowTheme;
  /** Persisted AI workspace sidebar width in pixels. */
  aiSidebarWidth?: number;
  /** Persisted right-docked AI terminal width in pixels. */
  aiTerminalWidth?: number;
}

/** Plan 11 Task 15 — 个性化配置（Step 1）。 */
export interface PersonalizationConfig {
  codeEditorBgImage?: string;
  codeEditorBgOpacity?: number;
  codeEditorBgBlur?: number;
  chatBgImage?: string;
  chatBgOpacity?: number;
  chatBgBlur?: number;
  userAvatar?: string;
  aiAvatar?: string;
  personaAvatars?: Record<string, string>;
  fontFamily?: string;
  fontSize?: number;
  bubbleStyle?: "rounded" | "sharp" | "bubble";
  bubbleOpacity?: number;
  messageSpacing?: number;
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
  // G-SEC-07: signals whether a key is stored on disk (backend strips the
  // plaintext apiKey from the response so it never lives in the JS heap).
  apiKeyConfigured?: boolean;
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
  // Plan 11 Task 2 — conversation organization metadata.
  tags?: string[];
  favorite?: boolean;
  group?: string;
  persona_id?: string;
  mode?: string;
  // DeletedAt: Unix timestamp when soft-deleted (0 = active, >0 = in trash).
  deleted_at?: number;
  // Plan 11 Task 2 Step 6: manual drag-and-drop sort order. 0 = fall back
  // to created_at-desc; non-zero values compared ascending by the sidebar.
  sort_order?: number;
  /** prompt-7 Task C: monotonic revision for dual-window CAS. */
  revision?: number;
  /** Unix seconds of last successful save. */
  updated_at?: number;
  /** Write-intent only; not persisted. */
  expected_revision?: number;
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
  /**
   * G-VSC-04: origin of the command for unified palette source labeling.
   * "native" = gugacode native plugin; "vscode" = VS Code extension.
   * Undefined means a built-in IDE command (no badge shown).
   */
  source?: "native" | "vscode";
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

// Plan 11 Task 3 — unified context chip for @mention + paste. A chip is any
// piece of context attached to the next message: a file, a symbol, a web
// search, a pasted image, a code block, etc. The InputComposer creates chips
// and the ContextChips panel lists/removes them. buildUserMessage (ai.ts)
// serializes them into the message prefix.
export type ContextChipKind =
  | "file"
  | "symbol"
  | "codebase"
  | "gitdiff"
  | "web"
  | "docs"
  | "mcp"
  | "skill"
  | "persona"
  | "url"
  | "image"
  | "codeblock";

export interface ContextChip {
  id: string;
  kind: ContextChipKind;
  label: string;
  // Optional payload depending on kind:
  filePath?: string; // for file/symbol
  language?: string; // for file/codeblock
  content?: string; // text content for file/codeblock/symbol/gitdiff
  imageUrl?: string; // for image (data URL)
  url?: string; // for web/url
  query?: string; // for web/docs search query
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

/** G-FEAT-04: A single file with unresolved merge/rebase conflicts.
 *  Mirrors the Go MergeConflict struct. */
export interface MergeConflict {
  file: string;
  ours: string;
  theirs: string;
  base: string;
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
   * Plan 11 Task 11 Step 1: Type specifies the step kind.
   * Supported: "command" (default), "ai", "git", "file", "mcp", "skill".
   */
  type?: WorkflowStepType;
  /**
   * Plan 11 Task 11 Step 1: OnFailure controls behavior when the step fails.
   * Supported: "abort" (default), "continue", "skip", "retry".
   */
  onFailure?: OnFailureAction;
  /**
   * Plan 11 Task 11 Step 1: Timeout is the maximum execution time in seconds.
   * 0 means no timeout.
   */
  timeout?: number;
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

/** Plan 11 Task 11 Step 1: Workflow step type. */
export type WorkflowStepType =
  | "command"
  | "ai"
  | "git"
  | "file"
  | "mcp"
  | "skill";

/** Plan 11 Task 11 Step 1: Step failure behavior. */
export type OnFailureAction =
  | "abort"
  | "continue"
  | "skip"
  | "retry";

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
  /**
   * Plan 11 Task 11 Step 2: Condition restricts when the trigger fires.
   * Branch matches git branch name; Language matches file language.
   */
  condition?: WorkflowTriggerCondition;
}

/** Plan 11 Task 11 Step 2: Trigger condition. */
export interface WorkflowTriggerCondition {
  /** Glob pattern matched against the current git branch. Empty matches all. */
  branch?: string;
  /** Language ID (e.g. "go", "typescript") matched against the changed file. Empty matches all. */
  language?: string;
  /** Additional glob pattern for file matching. */
  fileGlob?: string;
}

/** A multi-step workflow loaded from .nknk/workflows/*.yml (N-19). */
export interface WorkflowDef {
  name: string;
  description?: string;
  steps: WorkflowStep[];
  watch?: string[];
  /** Auto-trigger on IDE events like file-saved (Proposal B). */
  runOn?: WorkflowTrigger;
  /**
   * G-SEC-03: When true the workflow needs explicit user approval before
   * execution. Project-level workflows (.nknk/) default to true so that
   * untrusted startup workflows in cloned repositories cannot auto-run
   * shell commands. The UI must list these as "Pending Confirmation" and
   * require the user to click "Run".
   */
  requiresConfirmation?: boolean;
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
// G-VSC-04 — VS Code Extension coexistence
// ============================================================================

/**
 * G-VSC-04: Security level badge for a VS Code extension. Aligns with the
 * existing G-VSC-03 / G-SEC-12 security model in stores/extensionSecurity.ts
 * (ExtensionSecurityLevel). Native plugins run in a stricter, permission-gated
 * sandbox (nknk.* API), so they are labeled "Native Plugin"; VS Code extensions
 * run in the Extension Host and carry a trusted/reviewed/restricted level so
 * the user can distinguish risk in the management panel.
 */
export type VscodeExtensionSecurityLevel = "trusted" | "reviewed" | "restricted";

/**
 * G-VSC-04: A command contributed by a VS Code extension. The Extension
 * Host registers these via registerVscodeExtensionCommand(); they are
 * aggregated into the unified command palette as supplementary commands
 * (lower priority than native plugin commands).
 */
export interface VscodeExtensionCommand {
  id: string;
  /** Extension id that owns this command, e.g. "ms-python.python". */
  extensionId: string;
  label: string;
  category?: string;
  keybinding?: string;
  handler: (...args: unknown[]) => unknown | Promise<unknown>;
}

/**
 * G-VSC-04: Runtime descriptor for an installed VS Code extension. Populated
 * by the Extension Host bridge (a future module) via registerVscodeExtension().
 * Mirrors the relevant subset of VS Code's Extension<T> for management UI.
 */
export interface VscodeExtensionInfo {
  id: string;
  name: string;
  displayName?: string;
  description?: string;
  version: string;
  publisher?: string;
  enabled: boolean;
  /** Whether the extension is currently active in the Extension Host. */
  isActive: boolean;
  securityLevel: VscodeExtensionSecurityLevel;
}

// ============================================================================
// G-VSC-01 — VS Code Extension Marketplace (Open VSX)
// ============================================================================
// These types mirror the Go structs in services/marketplace_service.go. The
// Wails binding regenerates the JS/TS wrappers on the next dev/build; the
// shapes here let the frontend consume search/detail/install results with
// full typing.

/** A single hit from a registry search (G-VSC-01). */
export interface ExtensionSearchResult {
  id: string;
  name: string;
  displayName: string;
  publisher: string;
  description: string;
  version: string;
  rating: number;
  ratingCount: number;
  downloadCount: number;
  iconUrl: string;
}

/** A single published version of an extension. */
export interface ExtensionVersion {
  version: string;
  downloadUrl: string;
  date: string;
}

/** Full metadata for a single extension (detail view). */
export interface ExtensionDetail {
  id: string;
  name: string;
  displayName: string;
  publisher: string;
  description: string;
  version: string;
  rating: number;
  ratingCount: number;
  downloadCount: number;
  iconUrl: string;
  categories: string[];
  tags: string[];
  license: string;
  repository: string;
  readme: string;
  versions: ExtensionVersion[];
}

/** A locally installed VS Code extension (G-VSC-01). */
export interface InstalledExtension {
  publisher: string;
  name: string;
  version: string;
  path: string;
  enabled: boolean;
}

/**
 * Subset of extension/package.json parsed after VSIX extraction (Step 3).
 * engines.vscode gates compatibility; activationEvents/contributes/capabilities
 * drive the security classification and the management UI.
 */
export interface VSCodeExtensionManifest {
  name: string;
  publisher: string;
  version: string;
  displayName: string;
  description: string;
  engines: Record<string, string>;
  activationEvents: string[];
  contributes: unknown;
  capabilities: unknown;
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
 * prompt-6 Task 2: AI stream events are structured with streamId.
 * Legacy string payloads are still accepted by the frontend parser.
 *
 * - `ai:chunk`     — { streamId, data } token from the streaming response
 * - `ai:done`      — { streamId, data } finish-reason (data may be empty)
 * - `ai:error`     — { streamId, data } error message
 * - `ai:stream-busy` — { streamId, busy }
 * - `ai:tool_calls`  — { streamId, data } JSON array string
 * - `settings:changed` / `conversation:saved` / `agent:pending-updated` — dual-window SSOT
 * - `file:saved`   — the absolute path of the saved file
 * - `terminal:output` — { sessionId, data } for a single PTY write
 * - `terminal:exited` — { sessionId } when the PTY process exits
 * - `workflow:completed` — { name } when a workflow finishes (Proposal R)
 */
export interface AIStreamPayload {
  streamId?: string;
  data?: string;
  busy?: boolean;
}
export type AIChunkEvent = WailsEvent<AIStreamPayload | string>;
export type AIDoneEvent = WailsEvent<AIStreamPayload | string>;
export type AIErrorEvent = WailsEvent<AIStreamPayload | string>;
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

// ============================================================================
// G-FEAT-03 — Toolchain commands (Go/TS/JS build/test/lint/format)
// ============================================================================

/** A toolchain command exposed in the command palette / context menu. */
export interface ToolchainCommand {
  id: string;
  label: string;
  language: "go" | "typescript" | "javascript" | "general";
  command: string;
  args?: string[];
  description?: string;
}

/** A single parsed compiler/linter issue. */
export interface ToolchainDiagnostic {
  file: string;
  line: number;
  column: number;
  message: string;
  severity: "error" | "warning" | "info";
  source: string;
}

/** The outcome of running a toolchain command. */
export interface ToolchainResult {
  success: boolean;
  output: string;
  errors: ToolchainDiagnostic[];
  durationMs: number;
  notInstalled: boolean;
  installCmd?: string;
}

// G-FEAT-02: Offline LSP completion types.

/** Reports the availability and state of a language server (gopls/tsserver). */
export interface LSPServerStatus {
  language: string;
  available: boolean;
  running: boolean;
  serverPath: string;
  version: string;
  /** prompt-8 Task 8-D */
  lastError?: string;
  serverKind?: string;
}

/** Request payload for LSP completion/hover/diagnostics queries. */
export interface LSPCompletionRequest {
  language: string;
  filePath: string;
  line: number;
  column: number;
  content: string;
}

/** A single completion item returned by the LSP server. */
export interface LSPCompletionItem {
  label: string;
  kind: number;
  detail: string;
  insertText: string;
  /** prompt-10 10-I: auto-import / additionalTextEdits from LSP */
  additionalEdits?: LSPTextEdit[];
}

/** A single LSP diagnostic (error/warning). */
export interface Diagnostic {
  line: number;
  column: number;
  endLine: number;
  endColumn: number;
  severity: number;
  message: string;
  source: string;
}

/** prompt-8 Task 8-F: definition / references location. */
export interface LSPLocation {
  filePath: string;
  line: number;
  column: number;
  endLine: number;
  endColumn: number;
}

/** prompt-8 Task 8-G/H: text edit for format/rename. */
export interface LSPTextEdit {
  startLine: number;
  startCol: number;
  endLine: number;
  endCol: number;
  newText: string;
}

// ============================================================================
// Plan 11 Task 13 — Diff Enhancement（结构化多文件 diff / 三方合并 / AI 审查）
// 镜像 services/diff_service.go 的 JSON 标签，供 DiffViewer.vue + stores/diff.ts 使用。
// ============================================================================

/** 单行 diff 的变更类型。 */
export type DiffLineType = "context" | "added" | "removed" | "conflict";

/** AI 审查意见的严重级别（Step 8: severity 色标）。 */
export type AICommentSeverity = "info" | "warning" | "error" | "critical";

/** 行内评论（Step 4: 用户或 AI 添加）。 */
export interface InlineComment {
  author: string;
  body: string;
  createdAt: string;
  aiComment?: boolean;
}

/** AI 对 hunk 的审查意见（Step 3）。 */
export interface AIComment {
  severity: AICommentSeverity;
  message: string;
  suggestion?: string;
  /** 关联行号。 */
  line?: number;
}

/** 单行 diff。 */
export interface DiffLine {
  type: DiffLineType;
  /** 旧行号（removed/context 有）。 */
  oldNum?: number;
  /** 新行号（added/context 有）。 */
  newNum?: number;
  /** 行内容（不含前缀 +/-/空格）。 */
  content: string;
  /** Step 4: 行内评论。 */
  comments?: InlineComment[];
}

/** 一组连续的 diff 行。 */
export interface Hunk {
  oldStart: number;
  oldCount: number;
  newStart: number;
  newCount: number;
  lines: DiffLine[];
  /** Step 3: AI 审查标注。 */
  aiComments?: AIComment[];
}

/** 单个文件的 diff（Step 1）。 */
export interface FileDiff {
  path: string;
  /** 重命名时旧路径。 */
  oldPath?: string;
  oldContent: string;
  newContent: string;
  hunks: Hunk[];
  /** 统计。 */
  addedLines: number;
  removedLines: number;
}

/** 多文件 diff（Step 1）。 */
export interface MultiFileDiff {
  files: FileDiff[];
  totalAdded: number;
  totalRemoved: number;
}

/** 三方合并结果（Step 2）。 */
export interface ThreeWayMergeResult {
  merged: string;
  conflicts: number;
  hasConflict: boolean;
}

/** 单个文件输入。 */
export interface DiffFileInput {
  path: string;
  oldContent: string;
  newContent: string;
}

/** 单文件审查结果（Step 9）。 */
export interface FileReview {
  path: string;
  comments: AIComment[];
}

/** 审查统计（Step 9）。 */
export interface ReviewStats {
  filesReviewed: number;
  totalComments: number;
  critical: number;
  errors: number;
  warnings: number;
}

/** PR 审查结果（Step 9）。 */
export interface ReviewPRResult {
  summary: string;
  fileReviews: FileReview[];
  stats: ReviewStats;
}

/** 导出格式（Step 10）。 */
export type DiffExportFormat = "markdown" | "unified" | "html";

// ============================================================================
// Plan 11 Task 14 — 智能回滚（快照）类型
// ============================================================================

/** 快照创建原因（Step 1/3）。 */
export type SnapshotReason =
  | "manual"
  | "plan-step"
  | "goal-checkpoint"
  | "pre-apply"
  | "workflow-step";

/** 单个文件的快照元数据（Step 1/4）。 */
export interface FileSnapshot {
  path: string;
  hash: string; // SHA-256 内容哈希
  size: number;
}

/** 快照创建时的 Git 状态（Step 1/8）。 */
export interface GitState {
  branch: string;
  isClean: boolean;
  changes?: string[];
}

/** 完整快照（Step 1）。 */
export interface Snapshot {
  id: string;
  createdAt: string; // ISO 时间
  reason: SnapshotReason;
  workspaceRoot: string;
  files: FileSnapshot[];
  gitState?: GitState;
  fileCount: number;
}

/** 两个快照之间的差异（Step 2: DiffSnapshots）。 */
export interface SnapshotDiff {
  fromSnapshotId: string;
  toSnapshotId: string;
  added: string[];
  removed: string[];
  modified: string[];
}

/** 清理策略配置（Step 5）。 */
export interface CleanupConfig {
  keepN: number; // 保留最近 N 个（0 = 不限）
  maxAgeMs: number; // 最大保留时长毫秒（0 = 不过期）
}
