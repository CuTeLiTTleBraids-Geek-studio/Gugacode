# Changelog

All notable changes to gugacode are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added (prompt-10 — trusted refactor + debug/coverage MVP)

- **10-A Rename preview + Save All** — multi-file confirm dialog; buffers dirty; `saveAllFiles` command.
- **10-B FoS/write UX** — format failure toast; write failure `notifyError` keeps dirty.
- **10-C Test@Cursor** — Go `t.Run` subtests; vitest `it.each` / `test.each` patterns.
- **10-D Diagnostics** — `refreshDiagnosticsToProblems` after save; Problems click resolves path + line.
- **10-E docs** — extension-security rename/format providers corrected; architecture boundaries doc.
- **10-G Delve MVP** — headless DAP launch package/test, listen address, Stop.
- **10-H Coverage gutter** — `go test -coverprofile` + line decorations.
- **10-I auto-import + Organize Imports** — completion `additionalTextEdits` + Shift+Alt+O command.
- **10-J ESLint on save** for JS/TS via `eslint-file`.
- **10-K Hover/Definition seq cancel**.
- **10-L** go.work / package.json workspaces list store.
- **10-M** lightweight test explorer discover/run.
- **10-N** CI npm audit high blocking; `scripts/generate-sbom.sh`; release notes `docs/release-v0.3.0.md`.
- **10-O** optional CI `lsp-integration` (workflow_dispatch / schedule).
- **10-P** `docs/architecture-boundaries.md`.

### Added (prompt-9 — daily-use editor loop)

- **9-A Format on Save** — settings `formatOnSave` (default true); `saveFile` formats via LSP then writes + `didSave`.
- **9-B Rename** — Monaco `registerRenameProvider` (F2); `RenameSymbolWorkspace` multi-file WorkspaceEdit.
- **9-C Test at Cursor** — `RunTestAtCursor` for Go `TestXxx` and Vitest `it`/`test`; context menu + Ctrl+Shift+T; failure lines → Problems.
- **9-D LSP status codes** — `GetCallStatus` + Output channel notes for not_running/rpc/timeout.
- **9-E completion seq cancel** — stale completion responses dropped.
- **9-G SignatureHelp + OrganizeImports** — LSP APIs + Monaco signature provider.
- **9-H/I** — Vitest at cursor; StatusBar Go/Node versions + go.work flag.
- **9-J** — unified `parseToolOutputToProblems` for go/tsc/eslint/vitest output.
- **9-K** — didChange skip when content hash unchanged (+ 100ms throttle guard).
- **9-L/M scaffold** — `DebugService` (Delve detect), `CoverageService.ParseCoverProfile`.
- **9-O** — CI `npm audit --audit-level=high` (continue-on-error).
- **docs** — `docs/prompts/prompt-9.md`; README matrix updated for Format on Save / Rename.

### Added (prompt-8 — language IDE)

- **8-A document sync** — `syncDocument`: `didOpen` / `didChange` (full) / `didClose` / `didSave` with monotonic version (BUG-IDE-01/04).
- **8-B TypeScript LSP** — detect/start `typescript-language-server` or `vtsls --stdio` (not raw tsserver; BUG-IDE-02).
- **8-C paths** — `pathToURI` absolute-safe; frontend prefers `openFiles[].path` over Monaco URI (BUG-IDE-03).
- **8-D StatusBar LSP** — running/idle/error label + detail tooltip.
- **8-E mock LSP test** — `TestLSP_syncDocument_DidOpenThenDidChange` stdio mock covers didChange.
- **8-F Definition / References** — backend + Monaco providers.
- **8-G Format Document** — `textDocument/formatting` + Monaco formatting provider; file-scoped gofmt/goimports/prettier toolchain cmds.
- **8-H Rename** — `RenameSymbol` API (WorkspaceEdit → current file edits).
- **8-I Toolchain granularity** — workspace list/check vs `*-file` write; ESLint/Prettier no longer default whole-repo write.
- **8-J tests** — `go-test-pkg`, `vitest-file` commands.
- **8-L language matrix** — README capability table.
- **docs** — `docs/prompts/prompt-8.md` archived.

### Fixed (prompt-8)

- **BUG-IDE-01** completions used stale buffer without didChange.
- **BUG-IDE-02** TS server process selection.
- **BUG-IDE-03/04** path/URI + content sync contract.
- **BUG-M21/M22** format/lint defaults safer (file scope + check vs write).

### Added (prompt-7)

- **Task A living docs** — `docs/prompts/prompt-5.md`（摘要）、`prompt-6.md`、`prompt-7.md` 入库；`.gitignore` 强化排除 `*.run` / server 二进制。
- **Task B CI gates** — `.github/workflows/ci.yml` frontend-test 跑 `check-bindings.mjs` + `check-doc-numbers.mjs`。
- **Task C conversation CAS** — `revision` / `updated_at` / `expected_revision`；冲突 `ErrConversationConflict`；流中 stale 标记；冲突 UI 分叉新会话。
- **Task D agent dual-window policy (D1)** — pending 时本窗强提示；对端 toast「审批在另一窗」+ focusMain 尽力；产品声明仅发起窗可批。
- **Task E title quota** — streaming/busy 时跳过 `generateTitleWithAI`，改用截断启发式。
- **Task F settings version** — `settings.json` `version` + `expectedVersion` CAS；冲突 reload 提示。
- **Task G agent read sandbox** — Agent read/search 无项目 root 拒绝（写路径已在 prompt-6 收紧）。
- **Task J dual-window smoke** — `frontend/src/lib/dual-window-smoke.test.ts`（settings version / streamId / stale）。
- **Task K experimental surface** — IM/Computer Use 保持实验分组；QA/docs 标注 Experimental。
- **Task L UI caps** — Search `MAX_SEARCH_UI_RESULTS=500`；Git status `MAX_GIT_UI_CHANGES=1000`。

### Fixed (prompt-7)

- **BUG-H6** 会话流中不同步 → stale + CAS/fork，不再静默覆盖。
- **BUG-H7** 审批跨窗不可见 → D1 强指引与对端 toast。
- **BUG-M13** title 与主会话争配额。
- **BUG-M14** settings last-write-wins 无检测 → version CAS。
- **BUG-M15** CI 未跑 docs/bindings 脚本。
- **BUG-M18** Agent 无 root 读（工具路径）。
- **BUG-L18** prompt 文档基线归档。

### Notes (prompt-7)

- **Task I 真多流**：产品默认仍单流互斥；见 `docs/ai-windows.md`。
- **Task H**：StartStream 返回类型收紧；Workflow/Settings 边界 cast 仍受 bindings 形状限制。
- Tag 规划：功能就绪后本地整理提交再打 `v0.2.0` / `v0.2.1`（不自动 force-tag）。

### Added (prompt-6)

- **Task 1 dual-window SSOT** — `settings:changed` / `conversation:saved` / `agent:pending-updated` with per-webview `origin` anti-loop; docs in `docs/ai-windows.md`.
- **Task 2 streamId protocol** — `StartStream` returns streamId; all `ai:chunk|done|error|tool_calls|stream-busy` payloads carry `{streamId,...}`; frontend routes by `activeStreamId`.
- **Task 3 Anthropic tool_use** — SSE `content_block_start/delta` accumulation → same `NativeToolCall` / `ai:tool_calls` shape as OpenAI.
- **Task 12 QA checklist** — `docs/qa-dual-window.md` 10-step dual-window manual acceptance.
- **Task 10 experimental group** — Computer Use / IM settings nav under labeled experimental section (no recommendation badges).
- **Task 7 bindings regen** — `wails3 generate` Window/Workflow ByID; `check-bindings.mjs` enforces ByName=0.

### Fixed (prompt-6)

- **BUG-H4 dual-window settings/conversation drift** — peer reload after save.
- **BUG-H5 Anthropic tools half-impl** — full tool_use stream parse.
- **BUG-M5 empty root writes** — FileService Write/Delete/Rename/Create refuse when workspace root is empty.
- **BUG-M8 busy race** — frontend no longer clears `globalStreamBusy` on Stop/done; trusts `ai:stream-busy` only.
- **BUG-M9 inline vs chat quota** — skip inline Complete while stream busy (frontend + backend).
- **BUG-M10 responseRecorder status** — middleware preserves real AssetServer status codes.
- **BUG-L8/L9 docs** — README service count; ARCHITECTURE event table includes dual-window + tools events.
- **BUG-L12 dual-track dedup** — fence/native dedup key includes content hash for write.
- **BUG-M11 ByName debt** — WindowService / WorkflowService CRUD no longer use `$Call.ByName`.

### Notes (prompt-6 release hygiene)

- Tag plan: `0.1.1` (prompt-5 fixes) → `0.2.0` (dual-window SSOT + streamId + Anthropic tools).
- Do not commit `gugacode.exe`, `node_modules/`, or temporary binaries; prefer logical commits: security / dual-window / agent-tools / docs.

### Added (prompt-5 continued)

- **Task H native tools dual-track** — `AIConfig.Tools` + OpenAI `tools`/`tool_choice` streaming; SSE accumulates `tool_calls` and emits `ai:tool_calls`; frontend `buildNativeToolDefs` / `parseNativeToolCalls` / fence fallback.
- **Task I engineering** — `bootstrap_services.go` service registration list; `task bindings:check` / `docs:check` / `frontend:check`; scripts under `scripts/`.
- **Task J quality** — FileTree virtual window for large directories; `MAX_AI_MESSAGES=200`; vitest e2e-smoke of open-file → edit → run.
- **vue-tsc debt cleared** — 43 historical TS errors fixed (Thenable, bindings casts, Monaco freeInlineCompletions, etc.).

### Fixed (prompt-5)

- **BUG-H2 Apply-to-editor false success** — `openFileFromPath` rethrows on failure; `updateContent` returns false when the file is not open; AI window caches `lastSelectionPath` and requires it for Apply; main window opens a Diff confirm modal (optional snapshot) instead of silent overwrite.
- **BUG-H1 dual-window stream collision** — `StartStream` rejects when a stream is already active (`ErrStreamBusy`) instead of cancelling the previous stream; emits `ai:stream-busy`; frontend tracks `globalStreamBusy` and blocks concurrent send.
- **BUG-M1 Ctrl+Shift+A** — Monaco action `ai-send-to-window` now binds Ctrl/Cmd+Shift+A.
- **BUG-M2 / BUG-M3 frontend tests** — Skill approval tests mock `ElMessageBox.confirm`; vitest aliases `monaco-editor` to a stub so ExtensionPermissionDialog suite loads.
- **BUG-M4 write auto-approve** — `write` (like `run`) is never auto-approved; Settings UI hides auto-approve for run/write.
- **BUG-M6 OpenPath\*** — absolute + existence checks before launching explorer/VS Code.
- **BUG-L1/L2 docs** — README tool-call budget aligned to `MAX_TOOL_CALLS` (20); ARCHITECTURE service counts corrected; `docs/ai-windows.md` dual-window protocol.
- **BUG-L6 startup AI window** — `openAIWindowOnStartup` setting (default false); main no longer always opens the AI companion on launch.
- **Computer Use honesty (Task G)** — Settings UI labels the feature experimental / platform stub.

### Added

- **i18n (N-12)** — Full internationalization with English, Chinese (Simplified), and Japanese (N-59). All UI strings, AI system prompts, and ARIA labels are localized. Language switchable in Settings → General.
- **Layout Engine (N-25/N-30)** — Drag-and-drop panel layout with split views, persistent profiles, and JSON serialization via LayoutService. Recursive LayoutNode tree (LayoutLeaf | LayoutSplit) with tree operations in stores/layout.ts.
- **Quick Open (N-42)** — Ctrl+P fuzzy file search across the workspace with preview.
- **Inline AI Completion (N-20)** — Monaco InlineCompletionItemProvider for ghost-text code completions using the AI service with file context.
- **Agent Mode (N-26)** — Autonomous AI agent with tool use (read/write/run/search), per-tool approval policies, command sandboxing (cwd restriction, deny list, audit log), and risk classification (safe/elevated/dangerous).
- **Plugin System (N-26)** — Plugin discovery (user-global + project-scoped), manifest validation, command registration with public/private visibility (Proposal E), opt-in Web Worker sandbox (N-26), and asset serving via `/_plugins/` path prefix.
- **Settings Profiles (N-28)** — Multiple settings profiles with switch/import/export, ProfileService with JSON persistence.
- **AI Code Review (N-29)** — Git panel "AI Code Review" button analyzes uncommitted changes with AI and produces a structured review with file-by-file findings.
- **Tasks & Workflows (N-31/N-32)** — `.tasks.json` for build/test/run task definitions, `.workflows.json` for multi-step orchestration. Both runnable from the Terminal panel.
- **Project Rules (N-33)** — `.cursorrules` and `AGENTS.md` files loaded and appended to AI system prompt. RulesService with symlink-escape validation (N-112).
- **AI Prompt Presets (N-34)** — User-scoped and project-scoped preset prompts with save/load/delete. PresetService with path traversal protection.
- **Multi-tab Terminal (N-35)** — Multiple terminal sessions with create/switch/close, per-session xterm.js instance, and roving tabindex keyboard navigation (N-141).
- **Material You Theme Redesign (N-36)** — 8 accent themes (blue, teal, green, amber, pink, purple, cyan, indigo) with coordinated Monaco themes, dark/light/system mode, and CSS custom property tokens.
- **10 AI Preset Actions** — Explain, Refactor, Fix Bugs, Generate Docs, Generate Tests, Optimize, Code Review, Security Audit, Commit Message (added 4 new presets beyond original 5).
- **Localized AI System Prompts (N-59)** — Default system prompt, agent system prompt, conversation title prompt, and inline completion prompt all translated to Chinese and Japanese.
- **Security Hardening** — CSP nonce injection (N-14), URL validation for ListModels (N-73), API key storage via keyring/DPAPI (secrets.go with platform-specific backends), path traversal protection for ConversationService (N-91) and PresetService (N-92), workspace sandboxing for GitService and SearchService (N-67).
- **Accessibility (N-121~N-126, N-141, N-142, N-152, N-153)** — Keyboard navigation for all clickable elements (role="button", tabindex, keydown handlers), focus traps for CommandPalette and QuickOpen (role="dialog", aria-modal, roving tabindex), aria-live region for terminal output, localized ARIA labels, type="button" on all button elements.
- **Vue Global Error Handler (N-97/N-98)** — `app.config.errorHandler` and `window.addEventListener("error"/"unhandledrejection")` for user-visible error reporting.
- **Event-Based AI Streaming** — `StartStream` / `StopStream` Go methods using Wails event system (`ai:chunk`, `ai:done`, `ai:error`). Frontend event listeners for real-time streaming response assembly.
- **Global Ctrl+S keyboard shortcut** for saving files.
- **"Test Connection" button** in AI settings sends a test ping.
- **"Browse" button** for Data Folder Path opens native directory picker.
- **Theme selector** persists on change.

### Fixed

- **Critical: AI streaming was completely broken** — Wails IPC cannot pass JS callbacks to Go; replaced callback-based `SendStream` with event-driven `StartStream`/`StopStream`
- **Critical: Fake binding IDs** — Git, Search, Conversation, and AI preset services had hand-written sequential IDs (1-12) instead of FNV-1a hashes; all binding IDs now match Wails3 generator output
- **Missing Go methods** — `ConversationService.GenerateConversationID()` and `GenerateTitle()` methods added (frontend called them but they didn't exist as service methods)
- **N-61: ai_retry.go retried context.Canceled** — Now correctly skips retry for context cancellation and deadline exceeded. Removed dead code. Fixed returning closed body on retry exhaustion.
- **N-64: Monaco inline completion provider memory leak** — Provider now properly disposed on component unmount.
- **N-65: Terminal session memory leak** — Sessions now removed from map on natural exit.
- **N-66: Output buffer unbounded growth** — Output buffer now has size cap.
- **N-67: GitService/SearchService workspace sandbox** — Both services now validate paths against workspace root.
- **N-71: Rules content prompt injection** — Rules file content sanitized before appending to system prompt.
- **N-73: ListModels URL validation** — Base URL validated against SSRF and API key exfiltration.
- **N-91/N-92: Conversation/Preset path traversal** — Both services now use shared pathsec validation.
- **N-93: AIService field race** — All fields now protected by mutex.
- **N-94/N-95: Terminal TOCTOU and goroutine leak** — Running state now mutex-protected, readLoop has cancellation.
- **N-96: EditorView save failure silent** — User now notified on save failure.
- **N-108: SSE EOF comparison** — Uses `errors.Is(err, io.EOF)` instead of `==`.
- **N-112: Rules symlink escape** — SaveRules now calls `EvalSymlinks`.
- **N-119/N-139/N-140: Japanese prompt truncation** — All missing sections and items added to Japanese system prompts.
- SettingsView.vue: editor settings (fontSize, fontFamily, tabSize, wordWrap, lineNumbers, minimap) and General language now persist via `appState` reactive bindings with `saveSettings()` debounced save

## [0.1.0] - 2026-07-03

### Added

- **Core IDE Foundation (Plan 1)**
  - File explorer with tree navigation, create/rename/delete operations
  - Monaco code editor with syntax highlighting for 20+ languages
  - Tabbed editor with dirty-state tracking and unsaved-change indicators
  - Project switcher for managing multiple workspaces
  - Settings service with XDG-compliant config persistence
  - Custom title bar with window controls
  - Activity bar with sidebar panel switching
  - Status bar showing cursor position, language mode, and branch info

- **Terminal & AI Chat (Plan 2)**
  - Integrated terminal with PTY support (Windows ConPTY / Unix pty)
  - ANSI color rendering via xterm.js
  - AI chat panel with OpenAI-compatible API streaming (SSE)
  - Streaming response with abort/cancel support via AbortController
  - Conversation persistence (save/load/delete conversations)
  - Markdown rendering with syntax-highlighted code blocks and XSS sanitization
  - Settings view for AI configuration (API key, base URL, model, system prompt)

- **Git & Search (Plan 3)**
  - Git panel: branch display, status list, stage/unstage, commit
  - Ahead/behind tracking via BFS count implementation
  - Search panel: full-text file search with case-sensitivity toggle
  - Result navigation with file preview

- **Advanced AI Features (Plan 4)**
  - Editor right-click context menu AI actions: Explain, Refactor, Fix Bugs, Generate Docs, Generate Tests
  - Code context injection (selection + file path + language)
  - Conversation history sidebar with load/delete
  - Centralized AI prompt management (default system prompt + 5 preset actions)
  - Preset prompt API: `GetPresetPrompt`, `ListPresets`, `GetDefaultSystemPrompt`
  - Preset metadata with label, description, and icon for UI display
  - Custom system prompt configuration in settings

- **Editor & UX Polish (Plan 5)**
  - Dirty-state tracking with `saveFile()` and dirty-file enumeration
  - Dirty indicator dot (●) in editor tabs
  - Global keyboard shortcut system (`useKeyboard` composable)
  - Command palette with fuzzy search (Ctrl+Shift+P)
  - 6 built-in commands: Save, Toggle AI Chat, Toggle Terminal, Clear Chat, Toggle Sidebar, Toggle Minimap
  - Markdown live preview with split-editor layout
  - Find/Replace (Monaco built-in, Ctrl+F)

- **Open Source Readiness (Plan 6)**
  - MIT License
  - CONTRIBUTING.md with development setup and conventions
  - CODE_OF_CONDUCT.md (Contributor Covenant 2.1)
  - GitHub Actions CI workflow (Go + frontend tests)
  - Project README with feature list, install guide, and AI configuration

### Changed

- Rewrote `DefaultSystemPrompt` with structured sections (Role, Response Format, Code Quality, Context Awareness, Safety, Uncertainty)
- Refactored preset prompts to instruction-only format (frontend handles context injection)

### Security

- Path sandboxing for file operations (prevents directory traversal)
- XSS sanitization for AI-rendered Markdown (dompurify)
- Input validation for project IDs and terminal working directory
- HTTP request respects context cancellation for AI streaming
- AI HTTP client timeout (120s for non-streaming)
- AI API response status code validation (descriptive errors for 4xx/5xx)
- Terminal output buffer clears after read (prevents stale data)
- FileService workspace root sandboxing (all read/write/delete operations restricted to project directory)
