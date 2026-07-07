# Changelog

All notable changes to gugacode are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
