# 架构 / Architecture

## 概述 / Overview

**中文：** gugacode 是一款桌面 IDE，后端为 **Go（Wails v3）**，前端为 **Vue 3 + TypeScript**，最终编译为单文件可执行程序。后端通过 Wails IPC 暴露服务；前端通过自动生成的 TypeScript 绑定调用。

**English:** gugacode is a desktop IDE with a **Go (Wails v3)** backend and a **Vue 3 + TypeScript** frontend, compiled into a single binary. The backend exposes services via Wails IPC; the frontend uses auto-generated TypeScript wrappers.

## 技术栈 / Tech Stack

| 层级 / Layer | 技术 / Technology |
|---|---|
| 后端 / Backend | Go 1.25, Wails v3 (alpha2.111) |
| 前端 / Frontend | Vue 3, TypeScript 5, Vite 8, Tailwind v4 |
| 编辑器 / Editor | Monaco Editor 0.55 |
| 终端 / Terminal | ConPTY (Windows) / creack-pty (Unix) |
| Git | go-git v5.19.1 |
| UI | Element Plus 2.14 |
| Markdown | marked, DOMPurify, highlight.js |

## 项目结构 / Project Structure

```
gugacode/
├── main.go                    # 入口：服务注册、事件绑定 / entry, service registration
├── go.mod                     # 模块名 gugacode
├── services/                  # Go 后端服务（~35+）
├── frontend/                  # Vue 前端
│   ├── src/stores/            # 响应式状态
│   ├── src/components/        # UI 组件
│   └── bindings/              # Wails 自动生成绑定
└── build/                     # 多平台构建配置
```

## 服务架构 / Service Architecture

**中文：** 每个后端服务是一个 Go 结构体，通过 `application.NewService()` 注册。Wails v3 使用 FNV-1a 对 `{modulePath}.{TypeName}.{MethodName}` 计算绑定 ID；前端经 `$Call.ByID(bindingID, ...args)` 调用。

**English:** Each backend service is a Go struct registered with `application.NewService()`. Wails v3 hashes `{modulePath}.{TypeName}.{MethodName}` (FNV-1a) for binding IDs; the frontend calls `$Call.ByID(bindingID, ...args)`.

### 服务注册表 / Service Registry

核心服务包括：File、Project、Settings、Window、Terminal、AI、Conversation、Git、Search、Agent、Task、Workflow、Rules、Preset、Profile、Layout、Plugin、Marketplace、ExtensionSecurity、LSP、Toolchain、Diff、Snapshot、MCP、Skills、ComputerUse（实验）、IM、Persona、AIPlan、AIGoal、AIPermission 等。数量随版本增长，本表为代表性列表。

| 服务 / Service | 职责 / Responsibility |
|---|---|
| FileService | 工作区沙箱内的文件读写 / sandboxed file I/O |
| ProjectService | 最近项目 / recent projects |
| SettingsService | XDG 路径设置持久化 / settings persistence |
| WindowService | 窗口控制 / window controls |
| TerminalService | PTY 会话与输出缓冲 / PTY sessions |
| AIService | OpenAI/Anthropic 对话与 SSE 流 / chat + SSE |
| GitService | 状态/暂存/提交/分支/diff |
| SearchService | 正则搜索与替换 / search + replace |
| AgentService | 自治 Agent + 命令审批 / agent + approval |
| LSPService | gopls / tsserver 补全与诊断 |
| ToolchainService | 构建/格式化/测试工具链 |

### 事件系统 / Event System

Wails v3 事件用于流式数据与双窗同步 / used for streaming and dual-window sync：

- `terminal:output` — 终端输出
- `ai:chunk` / `ai:done` / `ai:error` — AI 流式片段
- `ai:stream-busy` — 全进程流互斥
- `ai:tool_calls` — 工具调用
- `settings:changed` / `conversation:saved` / `agent:pending-updated` — 双窗 SSOT（含 `origin` 防环）
- `file:saved` — 写盘成功后发出

### 路径沙箱 / Path Sandboxing

**中文：** `FileService.SetWorkspaceRoot` 设定工作区。写操作要求非空 root；路径经 `ValidatePathWithinRoot`（双侧 EvalSymlinks）校验。终端工作目录同样限制在工作区内。

**English:** Mutating file ops require a non-empty workspace root. Paths are validated with symlink-aware `ValidatePathWithinRoot`. Terminal CWDs are constrained similarly.

## 前端状态 / Frontend State

Vue 3 `reactive()` 模块级单例（非 Pinia），主要包括：`appState`、`editorState`、`aiState`、`gitState`、`searchState`、`terminalState`、`agentState`、`layoutState`、`outputState` 等。设置经 `saveSettings()`（约 500ms 防抖）持久化。

Vue 3 module-level `reactive()` singletons (not Pinia). Settings persist via debounced `saveSettings()`.

## 主题 / Theme

- 明暗：`data-mode` + Monaco 主题切换 / dark-light via `data-mode`
- 强调色：8 种 `data-theme` / 8 accents
- 跟随系统：`prefers-color-scheme`

## AI 集成 / AI Integration

- 双协议 SSE 对话 / dual-protocol streaming chat  
- Monaco 内联补全 / inline completions  
- 9 个右键代码动作 / 9 context-menu actions  
- `@文件` 提及注入上下文 / @-mentions  
- 会话历史 JSON 持久化 / conversation history JSON  

## 测试 / Testing

```bash
go test ./services/...
cd frontend && npx vitest run
cd frontend && npx vue-tsc --noEmit
```

## 构建 / Build

```bash
# 开发 / Dev
wails3 dev

# 生产 / Production
wails3 build

# 仅前端 / Frontend only
cd frontend && npm run dev
```
