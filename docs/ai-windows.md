# AI 双窗事件协议（prompt-5 Task F + prompt-6 Task 1/2）

gugacode 支持**主编辑器窗口**与**独立 AI 伴侣 OS 窗口**（`/#/ai-window`）并排使用。两窗是独立 Webview，**不共享 Vue 状态**，通过 Wails 应用级事件 + 服务端文件 SSOT 同步。

## 窗口生命周期

| API / 行为 | 说明 |
|---|---|
| `WindowService.OpenAIWindow` | 创建或聚焦 AI 窗；URL hash 为 `/#/ai-window` |
| `WindowService.CloseAIWindow` | 关闭 AI 窗 |
| `WindowService.ToggleAIWindow` | 切换显示 |
| `IsAIAlwaysOnTop` / `SetAIAlwaysOnTop` | 置顶 |
| 主窗 `WindowClosing` | 联动关闭 AI 窗 |
| 设置 `openAIWindowOnStartup` | 默认 `false`；为 `true` 时启动即打开 AI 窗 |

## 跨窗事件

| 事件名 | 方向 | Payload | 用途 |
|---|---|---|---|
| `ai:selection` | 主窗 → AI 窗 | `{ code, language, filePath }` | 编辑器选中代码注入 AI 上下文 chip |
| `ai:apply-to-editor` | AI 窗 → 主窗 | `{ code, filePath, language? }` | 代码块「应用到编辑器」 |
| `ai:chunk` / `ai:done` / `ai:error` | 后端 → 所有窗 | `{ streamId, data }` | AI 流式响应（按 streamId 路由） |
| `ai:stream-busy` | 后端 → 所有窗 | `{ streamId, busy }` | 全局流占用（互斥） |
| `ai:tool_calls` | 后端 → 所有窗 | `{ streamId, data }` | 原生 tool_calls JSON 数组字符串 |
| `settings:changed` | 任窗 → 所有窗 | `{ origin, at }` | 设置保存后对端 `loadSettings` |
| `conversation:saved` | 任窗 → 所有窗 | `{ origin, id, title, at }` | 会话列表刷新；同 id 打开时 reload |
| `agent:pending-updated` | 任窗 → 所有窗 | `{ origin, count, kinds }` | 审批队列可见性（摘要） |

### origin 防环

每个 Webview 在 sessionStorage 中持有 `windowOriginId`（见 `frontend/src/lib/windowOrigin.ts`）。  
发送同步事件时带上 `origin`；接收方若 `origin === 本窗` 则忽略，避免 reload 环。

### 发送选中到 AI 窗

- 编辑器右键 / 快捷键 **Ctrl/Cmd+Shift+A**
- 调用 `WindowService.SendSelectionToAI(code, language, filePath)`，后端 `Emit("ai:selection", …)`
- AI 窗缓存 `lastSelectionPath`，供后续 Apply 使用

### 应用到编辑器（prompt-5 Task A）

1. AI 窗点击代码块 Apply → `Emit("ai:apply-to-editor", { code, filePath })`
2. **必须**携带 `filePath`（来自上次 selection 缓存）
3. 主窗 `requestApplyToEditor`：打开文件（失败 rethrow，不假成功）→ 显示 Diff 预览
4. 用户确认后 `updateContent`；可选 Snapshot（`apply-to-editor`）

## 流式互斥与 streamId（prompt-5 Task B + prompt-6 Task 2）

后端 `AIService` 默认为**进程级单流**：

- 已有流时再次 `StartStream` → 返回错误 `another AI stream is already in progress`（**不再**取消旧流）
- `StartStream` 返回 `streamId`；启动时 `ai:stream-busy={streamId, busy:true}`，结束/Stop 时 `busy:false`
- 所有 `ai:chunk|done|error|tool_calls` 携带同一 `streamId`
- 前端 `aiState.activeStreamId`：只装配本窗拥有的流；错误 id 的 chunk 不污染消息
- 前端 `aiState.globalStreamBusy`：**仅**由 `ai:stream-busy` 更新（Stop/done 不本地抢清，见 prompt-6 Task 5）

### 真多流（prompt-7 Task I — 产品决策）

**默认不开放**真并发双流（主窗+AI 窗各一路）。协议层已带 `streamId`，但产品仍单流互斥以保证：

- 配额/限流可控  
- 审批与会话 CAS 语义简单  
- 不出现双窗抢同一 Provider 的账单暴增  

后续若开放：在保留 streamId 路由前提下，将互斥改为 per-window 配额（N=2），并更新本表。

## 设置 / 会话 SSOT（prompt-6 Task 1 + prompt-7 C/F）

| 数据 | SSOT | 同步方式 |
|---|---|---|
| Settings | 磁盘 `settings.json` + `version` | 保存带 `expectedVersion` CAS；冲突 reload；Emit 带 version |
| Conversations | 磁盘 + `revision`/`updated_at` | 保存 CAS；流中 peer 保存标 stale；冲突则 fork 新 id |
| Agent 审批队列 | **仅发起窗** 可操作（Task D1） | `agent:pending-updated` 摘要 + 对端 toast；完整审批卡不跨窗 |

禁止两窗静默覆盖同一 `conversationId`：空闲时 reload；流中 stale + CAS/fork。

## 相关文件

- `services/window_service.go` — 双窗生命周期
- `services/ai_service.go` — 流式、互斥、streamId、Anthropic tool_use
- `frontend/src/lib/windowOrigin.ts` — origin id
- `frontend/src/views/AiWindowView.vue` — AI 窗 UI
- `frontend/src/stores/editor.ts` — Apply + Diff 状态
- `frontend/src/stores/ai.ts` — 流式 / streamId / globalStreamBusy
- `frontend/src/stores/app.ts` — settings 同步
- `frontend/src/main.ts` — 主窗监听 `ai:apply-to-editor`
- `docs/qa-dual-window.md` — 手工验收 10 步
