# Architecture

## Overview

gugacode is a desktop IDE built with **Go (Wails v3)** backend and **Vue 3 + TypeScript** frontend, compiled into a single binary. The backend provides services via Wails IPC bindings; the frontend consumes them through auto-generated TypeScript wrappers.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, Wails v3 (alpha2.111) |
| Frontend | Vue 3, TypeScript 5, Vite 8, Tailwind v4 |
| Editor | Monaco Editor 0.55 |
| Terminal | ConPTY (Windows) / creack-pty (Unix) |
| Git | go-git v5.19.1 |
| UI Components | Element Plus 2.14 |
| Charts/Markdown | marked, DOMPurify, highlight.js |

## Project Structure

```
gugacode/
├── main.go                    # App entry: service registration, event wiring
├── go.mod                     # Module: gugacode
├── services/                  # Go backend services (17 services)
│   ├── file_service.go        # File I/O with workspace sandboxing
│   ├── project_service.go     # Recent projects management
│   ├── settings_service.go    # XDG-path settings persistence
│   ├── window_service.go      # Window controls (min/max/close/fullscreen)
│   ├── terminal_service.go    # ConPTY/pty terminal sessions
│   ├── ai_service.go          # OpenAI-compatible chat + streaming SSE
│   ├── ai_prompts.go          # Default system prompt + 10 preset actions
│   ├── ai_retry.go            # Retry with backoff for transient AI errors
│   ├── ai_urlsec.go           # URL validation for ListModels (N-73)
│   ├── conversation_service.go# AI conversation history persistence
│   ├── git_service.go         # Git status/stage/commit/branch/diff
│   ├── search_service.go      # Regex content search + replace
│   ├── agent_service.go       # Autonomous agent with command sandboxing
│   ├── task_service.go        # Build/test/run task definitions
│   ├── workflow_service.go    # Multi-step workflow orchestration
│   ├── rules_service.go       # .cursorrules/AGENTS.md rules loading
│   ├── preset_service.go      # AI prompt presets (user + project-scoped)
│   ├── profile_service.go     # Settings profiles (switch/import/export)
│   ├── layout_service.go      # Persistent layout profiles
│   ├── plugin_service.go      # Plugin discovery + asset serving
│   ├── loglevel_service.go    # Runtime log level control
│   ├── output_buffer.go       # Thread-safe terminal output buffer
│   ├── pathsec.go             # Shared path traversal validation
│   ├── myers_diff.go          # Myers diff algorithm for git diffs
│   ├── token_estimator.go     # Token count estimation
│   ├── secrets.go             # API key storage (keyring/DPAPI)
│   ├── logging.go             # Structured logging setup
│   └── *_test.go              # Go unit tests
├── frontend/
│   ├── src/
│   │   ├── api/services.ts    # Typed Wails binding wrappers
│   │   ├── stores/            # Vue reactive state (17 stores)
│   │   ├── components/
│   │   │   ├── editor/        # CodeEditor (Monaco), DiffView, TabBar
│   │   │   ├── explorer/      # FileTree with context menu
│   │   │   ├── layout/        # MainLayout, TitleBar, ActivityBar, SidePanel, GitPanel, SearchPanel, TerminalPanel, AiChatPanel, StatusBar, CommandPalette, QuickOpen
│   │   │   └── settings/      # Settings sections (General, Editor, AI, Agent, Presets, Prompts, Terminal, Shortcuts, Appearance, Profiles)
│   │   ├── views/             # WelcomeView, ProjectsView, EditorView, SettingsView, PluginsView
│   │   ├── lib/               # monaco-themes, language detection, markdown, i18n, notifications, pluginRegistry, pluginSandbox
│   │   ├── composables/       # useKeyboard (global shortcuts)
│   │   └── types/index.ts     # Shared TypeScript interfaces
│   └── bindings/              # Auto-generated Wails JS bindings
├── build/                     # Platform-specific build configs (Windows/macOS/Linux/iOS/Android)
└── docs/                      # Documentation and plans
```

## Service Architecture

Each backend service is a Go struct registered with `application.NewService()`. Wails v3 computes method-binding IDs using FNV-1a 32-bit hash of `{modulePath}.{TypeName}.{MethodName}`. The frontend calls these via `$Call.ByID(bindingID, ...args)`.

### Service Registry (main.go — 17 services)

| Service | Responsibility |
|---|---|
| FileService | File CRUD with path sandboxing (prevents traversal outside workspace) |
| ProjectService | Recent projects list, add/remove, sorted by LastOpened |
| SettingsService | JSON settings persisted to XDG config dir |
| WindowService | Window controls: minimise, maximise, close, fullscreen, set title |
| TerminalService | ConPTY/pty session management, output buffering |
| AIService | OpenAI-compatible chat (send + stream), preset prompts, config |
| GitService | Status, stage/unstage, commit, branch CRUD, diff |
| SearchService | Regex content search + find-and-replace |
| ConversationService | AI conversation save/load/list/delete/rename |
| TaskService | Build/test/run task definitions from .tasks.json |
| WorkflowService | Multi-step workflow orchestration from .workflows.json |
| AgentService | Autonomous agent with command sandboxing + audit logging |
| RulesService | .cursorrules/AGENTS.md rules loading + validation |
| LogLevelService | Runtime log level control |
| PluginService | Plugin discovery, manifest validation, asset serving |
| ProfileService | Settings profiles (switch/import/export) |
| LayoutService | Persistent layout profiles (JSON) |

### Event System

Wails v3 events are used for streaming data:
- `terminal:output` — terminal output chunks (emitted from Go poll loop)
- `ai:chunk` — AI streaming response chunks
- `ai:done` — AI stream completion
- `ai:error` — AI stream error
- `file:saved` — emitted after FileService.WriteFile succeeds
- `time` — clock tick for status bar

### Path Sandboxing

`FileService.SetWorkspaceRoot(path)` sets the allowed directory. All file operations (including `ListDirectory`) validate paths against this root using `filepath.Rel()` to prevent directory traversal. `TerminalService` has an equivalent `validateWorkingDir()` that ensures the working directory is within the workspace.

## Frontend State Management

State is managed via Vue 3 `reactive()` singletons in `stores/`:

- **appState** — global app settings (theme, editor config, AI config, panel visibility, language)
- **editorState** — open files, active file, dirty state, auto-save
- **aiState** — messages, streaming state, conversations, mentioned files
- **gitState** — changed files, branch info, branches list
- **searchState** — search results, replace state
- **terminalState** — terminal sessions, active session, output
- **agentState** — agent tool calls, approval state
- **inlineCompletionState** — inline AI completion state
- **layoutState** — layout tree (leaves, splits), active profile
- **outputState** — output panel entries, problems/diagnostics
- **presetsState** — AI prompt presets (user + project)
- **profilesState** — settings profiles
- **reviewState** — AI code review results
- **rulesState** — project rules files
- **tasksState** — build/test/run tasks
- **workflowsState** — multi-step workflows
- **pluginsState** — installed plugins

Settings are persisted to the backend via `saveSettings()` (debounced 500ms) and loaded on startup via `loadSettings()`.

## Theme System

- **Dark/Light mode** — `data-mode` attribute on `<html>` overrides CSS custom properties. Monaco themes are switched between `nknk-{accent}` (dark) and `nknk-light-{accent}` (light) sets.
- **Accent colors** — 8 accent themes (blue, teal, green, amber, pink, purple, cyan, indigo) via `data-theme` attribute. Each accent has coordinated Monaco editor themes.
- **System mode** — Listens to `prefers-color-scheme` media query and auto-switches.

## AI Integration

- **Chat** — OpenAI-compatible API (`/v1/chat/completions`). Supports streaming via SSE.
- **Inline Completion** — Monaco `InlineCompletionItemProvider` calls the AI service with the current file context.
- **Code Actions** — Right-click context menu in Monaco with 9 preset actions (explain, refactor, fix, generate docs, generate tests, optimize, review, security, commit message).
- **@-mention** — Chat input supports `@file` mentions to inject file content as context.
- **Conversation History** — Saved as JSON files in the XDG data directory.

## Testing

- **Go** — `go test ./services/...` (unit tests for all services)
- **Frontend** — `npx vitest run` (Vue component tests + store tests)
- **Type-check** — `npx vue-tsc --noEmit`

## Build

```bash
# Development (hot reload)
wails3 dev

# Production build
wails3 build

# Frontend only (browser dev)
cd frontend && npm run dev
```
