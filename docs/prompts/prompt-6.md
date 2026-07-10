# prompt-6.md — gugacode 第二轮全方位审查与后续提升路线

> **审查日期**：2026-07-10  
> **审查基线**：当前工作区（相对 Initial commit 有大量未提交改动；含 prompt-5 落地痕迹）  
> **审查依据**：`prompt-5.md` 验收清单 + 当前源码 / 测试 / SECURITY.md / ARCHITECTURE.md / README / CHANGELOG / `docs/ai-windows.md`  
> **目的**：对 prompt-5 交付做闭环复审，评估前后端正确性 / 规范合规 / 质量，给出下一阶段（prompt-6）可执行改进清单  

---

## 0. 审查说明

### 0.1 与 prompt-5 的关系

| 文档 | 角色 |
|---|---|
| `prompt-5.md` | 第一轮全栈审计 + P0–P3 任务清单（Task A–J） |
| **本文件 `prompt-6.md`** | 复审 prompt-5 落地质量 + 残留债 + 新发现缺陷 + 下一迭代任务 |

### 0.2 审查方法

1. 对照 `prompt-5.md` §7–§8 逐项核对源码与 CHANGELOG  
2. 静态阅读：双窗协议、AI 流互斥、Apply Diff、Agent 审批、原生 tools、bootstrap  
3. 动态验证（本机，2026-07-10）：  

| 检查 | 结果 |
|---|---|
| `go test ./services/...` | **PASS**（~43s） |
| `go test .` | **PASS**（~3s） |
| `vitest run` | **52 files / 1226 tests 全绿**（较 prompt-5 的 3 failed 显著改善） |
| ESLint `src` | **exit 0** |
| `node scripts/check-doc-numbers.mjs` | **OK**（MAX_TOOL_CALLS=20 对齐） |
| `node scripts/check-bindings.mjs` | **OK**（仍提示 **14 处 ByName** 待收敛） |

### 0.3 总体评分（相对 prompt-5 的变化）

| 维度 | prompt-5 | **本轮** | 变化说明 |
|---|---|---|---|
| 功能完整度 | 8.0 | **8.2** | 原生 tools 双轨、工程脚本、虚拟文件树等 |
| 实现正确性 | 7.0 | **8.0** | H1/H2/M1–M4/M6/L1/L6 等关键缺陷已修 |
| 安全性 / 合规 | 8.0 | **8.3** | write 永不 auto-approve；路径打开校验；Computer Use 诚实标注 |
| 代码规范 | 7.5 | **8.0** | bootstrap 拆分、docs/ai-windows、doc/bindings 检查脚本 |
| 代码质量 | 7.0 | **7.8** | 前端测试全绿；vue-tsc 债清理；e2e-smoke |
| 可维护 / 可演进 | 6.5 | **7.2** | 服务注册抽出；仍服务膨胀 + 双窗状态孤岛 |
| **综合** | **7.3** | **~7.9** | 接近「可日常自用 / 可演示」线，未达生产加固线 |

---

## 1. prompt-5 交付闭环复审

### 1.1 Task / Bug 落地状态

| 项 | 状态 | 证据摘要 |
|---|---|---|
| **Task A / BUG-H2** Apply 假成功 | ✅ 已修 | `openFileFromPath` rethrow；`updateContent→boolean`；`requestApplyToEditor` + Diff；`lastSelectionPath`；可选 Snapshot |
| **Task B / BUG-H1** 双窗流冲突 | ✅ 最小方案已落地 | `ErrStreamBusy`；`ai:stream-busy`；`globalStreamBusy` 禁发；**完整 streamId 仍未做** |
| **Task C / BUG-M1** Ctrl+Shift+A | ✅ | Monaco keybinding 数值绑定 |
| **Task C / BUG-L6** 启动即开 AI 窗 | ✅ | `openAIWindowOnStartup` 默认 false |
| **Task D** 测试修复 | ✅ | 1226 全绿；ElMessageBox mock；monaco stub alias |
| **Task E / BUG-M4** write auto-approve | ✅ | `shouldAutoApprove` 对 write 恒 false；设置 UI 隐藏 |
| **Task E / BUG-M6** OpenPath* | ✅ | `validateOpenPath` 绝对路径 + Stat |
| **Task E / BUG-M5** 空 root 沙箱 | ⚠️ 部分 | Agent write/run 要求 project；**FileService 空 root 仍无限制** |
| **Task F** 文档 | ✅/⚠️ | MAX_TOOL_CALLS 对齐、ARCHITECTURE 计数说明、`docs/ai-windows.md`；README 树状目录仍写「21 个服务」 |
| **Task G** Computer Use 诚实化 | ✅ | i18n experimental/stub 文案 |
| **Task H** 原生 tools | ✅/⚠️ | OpenAI `tools` + `ai:tool_calls` + fence 去重；**Anthropic 仅注入 tools，未解析 tool_use 流** |
| **Task I** 工程化 | ✅/⚠️ | `bootstrap_services.go`、`scripts/*`、Taskfile 钩子；**ByName 仍 14 处** |
| **Task J** 质量 | ✅ | FileTree 虚拟窗口、`MAX_AI_MESSAGES=200`、e2e-smoke |

### 1.2 prompt-5 Definition of Done 对账

- [x] `go test ./services/...` 与 `go test .` 通过  
- [x] `vitest run` 0 failed  
- [x] lint 0 errors  
- [x] BUG-H2 链路已代码级修复（人工 GUI 建议再验一次）  
- [x] BUG-H1 至少完成互斥流  
- [x] README 工具次数与常量一致（脚本校验）  
- [x] write 审批策略与 SECURITY 一致  

**结论：prompt-5 以「最小可合并」标准基本闭环；剩余为深化项，转入 prompt-6。**

---

## 2. 执行摘要（给决策者）

### 本轮相对 prompt-5 的进步

1. **双窗正确性从「半成品」变为「可用互斥模型」**，并有文档 `docs/ai-windows.md`。  
2. **Apply-to-editor 从假成功变为 Diff 确认 + 路径强制 + 可选快照**——安全与体验双升。  
3. **测试债清零**（1226 全绿），工程质量可信度显著提高。  
4. **Agent 安全边界收紧**（run + write 永不 auto-approve）。  
5. **原生 function calling 双轨**迈出第一步（OpenAI 路径）。  

### 仍制约 0.2 / 生产化的主因

1. **双 Webview 状态孤岛**：设置 / 会话 / Agent 队列 / 打开文件列表不共享，易分叉或互相覆盖。  
2. **AI 流仍是进程单通道**：互斥正确，但无法真正并排双对话；无 streamId 路由。  
3. **Anthropic Agent 工具链不完整**：tools 已发，流式 `tool_use` 未解析。  
4. **空 workspace 时 FileService 仍「无沙箱」**（威胁模型弱但仍是合规缺口）。  
5. **产品面过宽**：IM / Computer Use stub / 扩展宿主与核心编辑器体验争抢注意力。  
6. **工作区未整理提交**：大量 modified + untracked，发布与 code review 成本高。  

---

## 3. 残留 / 新发现缺陷（按严重度）

### 3.1 High

#### BUG-H4 — 双窗会话与设置无同步（原 M7 升级）

**现象**：主窗与 AI 窗是独立 Webview，各有一份 `appState` / `aiState`。  

**风险**：  
- 两窗改模型 / BaseURL / 主题，后写 settings 覆盖先写。  
- 两窗各自 `persistConversation`，若误用同一 `conversationId` 会互相覆盖消息；若 ID 不同则「同一主题两份历史」。  
- Agent `pendingToolCalls` 仅在发起窗存在，另一窗看不见审批卡。  

**建议**：  
1. 后端 `settings:changed` / `conversation:updated` 事件总线；  
2. 或 AI 窗只读配置 + 「在主窗打开设置」；  
3. 会话列表以服务端为 SSOT，打开会话时强制 reload。

#### BUG-H5 — Anthropic 原生 tools「半实现」

**位置**：`ai_service.go` Anthropic 分支注入 `tools`，但注释写明 tool_use 仍靠文本 fence；仅 OpenAI 走 `parseSSEStreamWithTools` → `ai:tool_calls`。  

**影响**：Anthropic Agent 模式依赖模型是否仍输出 fence；与「Task H 双轨」宣传不完全一致。  

**建议**：实现 Anthropic SSE `content_block_start/delta` 的 `tool_use` 累积，复用 `ai:tool_calls` 事件形状。

### 3.2 Medium

#### BUG-M5（残留）— 空 workspace root 关闭 File 沙箱

`ValidatePathWithinRoot` / `FileService` 在 `root==""` 时放行任意路径。Agent 侧已要求 project，但 **直接调 FileService 的绑定仍可写盘**。  

**建议**：无 root 时拒绝 Write/Delete/Rename；或默认 root = 用户文档目录。

#### BUG-M8 — 客户端过早清除 `globalStreamBusy`

`stopGeneration` 与 `ai:done/error` 均在前端置 `globalStreamBusy=false`。Stop 后若后端仍短暂推 chunk，或两窗时序交错，可能出现短窗竞态。  

**建议**：busy 仅信任后端 `ai:stream-busy`；前端 Stop 只调 `StopStream`，不本地抢先清 busy（或 Stop 后等 busy 事件）。

#### BUG-M9 — 内联补全与主会话争用配额

`Complete` 不走 `StartStream` 互斥，可与主对话并行打同一 Provider，导致限流/账单暴增。  

**建议**：streaming 时暂停 inline provider；或共享后端 rate limiter。

#### BUG-M10 — `responseRecorder` 丢弃非 200 状态（原 L5）

中间件一律 `WriteHeader(200)` 写出缓冲体，AssetServer 404/500 可能被改成 200。桌面场景影响有限，但不利于调试与合规审计。  

**建议**：recorder 保存 `statusCode`，透传真实状态。

#### BUG-M11 — ByName 绑定技术债（14 处）

`api/services.ts` 仍对 Window/部分新 API 使用 `$Call.ByName`。bindings 检查脚本已提示。  

**建议**：`wails3 generate` 后改回 ByID 强类型；CI 将 ByName 数量阈值压到 0（或 allowlist）。

#### BUG-M12 — 双窗均收全局 `ai:chunk` 但无 owner id

互斥下通常只有发起窗有 `pendingAssistantMessage`，行为正确；一旦未来放开多流或状态错乱，无 id 会立刻串话。  

**建议**：即便单流，也在 payload 带 `streamId`，前端只装配自己的 id（为多流铺路）。

### 3.3 Low / 体验 / 文档

| ID | 描述 |
|---|---|
| BUG-L8 | README 项目结构仍写「21 个服务」，与 ARCHITECTURE「~35+」/实际不符 |
| BUG-L9 | ARCHITECTURE 事件表缺 `ai:stream-busy` / `ai:tool_calls` / `ai:selection` / `ai:apply-to-editor` |
| BUG-L10 | `main.go` 仍偏厚（~547 行，虽已抽 bootstrap） |
| BUG-L11 | Computer Use / IM 仍注册完整服务，增加攻击面与认知负担（虽 UI 已标实验） |
| BUG-L12 | Agent fence 与 native 双轨去重仅按 `kind+target`，同 target 不同 content 的 write 可能丢一条 |
| BUG-L13 | 大量未提交变更；缺清晰 release 切分（0.1.1 / 0.2.0） |
| BUG-L14 | 无真正 GUI E2E（仅 vitest smoke）；双窗 Apply 无自动化 |

---

## 4. 功能正确性审查（本轮）

### 4.1 后端

| 模块 | 正确性 | 备注 |
|---|---|---|
| AI 流互斥 | **良好** | ErrStreamBusy + busy 事件；非抢占取消 |
| AI OpenAI tools 流 | **良好** | tool_calls 累积 + 事件 |
| AI Anthropic tools | **中等** | 请求侧有 tools；响应侧未完整 |
| Apply 相关（后端路径打开） | **良好** | validateOpenPath |
| Agent 执行 | **良好** | 无 shell、审批、审计 |
| File/pathsec | **良好*** | *空 root 策略仍松 |
| Settings/Secrets | **良好** | G-SEC-07 |
| Snapshot / Diff | **良好** | Apply 前 best-effort 快照 |
| Computer Use | **诚实 stub** | UI 已标明 |
| bootstrap 装配 | **良好** | `bootstrap_services.go` 可读性↑ |
| 测试 | **优秀** | services 全绿 |

### 4.2 前端

| 模块 | 正确性 | 备注 |
|---|---|---|
| Apply Diff 主路径 | **良好** | MainLayout 模态 + confirmApplyDiff |
| AI 窗 selection/apply | **良好** | lastSelectionPath；无 path 拒绝 |
| 流 busy UI | **良好** | 发送禁用 + i18n |
| Agent 审批 / write 策略 | **良好** | 与后端安全叙事一致 |
| 原生 tools 入队 | **良好** | 与 fence 去重 |
| 双窗状态 | **弱** | 孤岛问题见 H4 |
| 设置持久化 | **良好** | 单窗内；跨窗见 H4 |
| FileTree 大目录 | **改善** | 虚拟窗口 |
| 测试 | **优秀** | 1226 全绿 |

### 4.3 跨切关注点

| 主题 | 评估 |
|---|---|
| 安全门禁 G-SEC-01~12 | 仍成立；write 审批更严 |
| i18n | 新文案（streamBusy / experimental / openAIWindowOnStartup）三语齐全 |
| 性能 | 消息上限 200；大目录虚拟化；终端/output 既有 cap |
| 离线 | LSP/工具链依赖本机二进制——需运行时抽测（本轮未做真机） |

---

## 5. 代码规范性与合规性

### 5.1 规范

**优点**  
- 缺陷修复带 prompt-5 Task / BUG 编号，可追溯。  
- 新增 `scripts/check-doc-numbers.mjs`、`check-bindings.mjs`——防文档与绑定漂移。  
- 服务注册与入口分离（`bootstrap_services.go`）。  
- 前端测试与实现 mock 对齐（ElMessageBox）。  

**不足**  
- README 结构树滞后。  
- ByName 兼容层仍在。  
- 工作区脏：review 难以按 PR 粒度消化。  
- `ARCHITECTURE` 事件列表未跟上双窗与 tools。  

### 5.2 合规 / 安全

| 项 | 状态 |
|---|---|
| 密钥不回前端 | ✅ |
| BaseURL SSRF | ✅ |
| 命令强制审批 | ✅ |
| write 不可 auto | ✅（本轮确认） |
| workflow 不自启 | ✅ |
| CSP nonce | ✅ |
| 扩展默认禁用 | ✅ |
| 空 root 写盘 | ⚠️ 建议收紧 |
| Computer Use | ✅ 默认关 + 文案诚实 |
| npm audit / govulncheck | 文档要求；本轮未重跑完整 CI 矩阵 |

### 5.3 许可证

MIT + 字体许可文件仍在；建议 0.2 发版附 `NOTICE` 汇总第三方 SPDX。

---

## 6. 代码质量评估

### 6.1 量化快照

| 指标 | 约值 |
|---|---|
| Go services 测试 | 全绿 |
| 前端 Vitest | **1226 / 1226** |
| ESLint | 0 error |
| ByName 调用 | **14**（脚本可跟踪） |
| AI 消息上限 | 200 |
| 工具调用上限 | 20（与 README 一致） |

### 6.2 质量判断

- **回归防护**：从「有测试但 3 红」→「全绿」，质量跃迁一级。  
- **架构债**：双窗产品形态领先于状态同步架构（H4）。  
- **复杂度**：Plan 11 模块仍多；实验特性应继续「默认折叠」。  
- **Wails v3 alpha**：运行时风险未变，需锁定版本与 bindings 再生纪律。  

---

## 7. 后续提升打量（战略）

### 7.1 0.2.0 建议主题：**「双窗可靠 + Agent 可信 + 产品收敛」**

不要再铺新能力面，优先：

1. **双窗 SSOT**（设置/会话/审批可见性）  
2. **streamId 级流路由**（互斥可保留为默认策略）  
3. **Anthropic tool_use 完整**  
4. **空 root 写拒绝**  
5. **发版与 git 卫生**（把 prompt-5 工作切成可 review 的提交/PR）  

### 7.2 刻意降级 / 实验仓

| 能力 | 建议 |
|---|---|
| Computer Use | 保持实验；无原生实现前不进默认设置首页 |
| IM | 默认隐藏或独立 settings 页「高级」 |
| 完整 VS Code 扩展兼容 | 维持最小 surface；不扩 API |
| Plan/Goal 自治 | 强化 Checkpoint + 审批可视化，勿加更多模式 |

### 7.3 质量门禁增强

1. CI 跑 `check-doc-numbers` + `check-bindings`（ByName 上限）。  
2. `npm audit --audit-level=high` 发版门禁。  
3. 可选：双窗关键路径 Playwright/手工 checklist 入 `docs/qa-dual-window.md`。  

---

## 8. prompt-6 可执行任务清单

> 完成项请更新 CHANGELOG `[Unreleased]` 并尽量拆 PR。

### Task 1 — 双窗状态同步（P0，对应 H4）

1. 定义事件：`settings:changed`、`conversation:saved`、`agent:pending-updated`（或合并为 `app:sync`）。  
2. 主窗 saveSettings 成功后 Emit；AI 窗监听后 `loadSettings`（防环：带 origin window id）。  
3. 会话：打开/保存后另一窗 invalidate 列表；禁止两窗同时编辑同一 conversationId 而不 reload。  
4. 文档更新 `docs/ai-windows.md`。  

**验收**：A 窗改模型后 B 窗下拉一致；A 窗存会话后 B 窗历史列表可见。

### Task 2 — streamId 事件协议（P0/P1）

1. `StartStream` 返回 `streamId`（或生成 UUID 写入事件）。  
2. `ai:chunk|done|error|tool_calls|stream-busy` payload 带 id。  
3. 前端只装配本窗 `activeStreamId`。  
4. 默认策略可仍「单流互斥」；协议先兼容多流。  

**验收**：单元测试模拟错误 id 的 chunk 不污染消息。

### Task 3 — Anthropic tool_use 完整解析（P1，H5）

1. 解析 Anthropic SSE tool_use 块 → 与 OpenAI 相同的 `NativeToolCallPayload[]`。  
2. 发 `ai:tool_calls`。  
3. 测试：fixture SSE → 期望 payload。  

### Task 4 — 空 root 写拒绝（P1，M5）

1. `FileService.WriteFile/Delete/Rename`：root 空则 error。  
2. 前端无项目时禁用危险 Agent 工具（已有部分）+ File 写入口提示。  
3. 单测覆盖。  

### Task 5 — busy 标志仅信任后端（P1，M8）

1. 前端 `stopGeneration` / done 路径不本地抢清 busy（或仅乐观 UI，以事件为准）。  
2. 补并发单测（若可在 Go 侧）。  

### Task 6 — responseRecorder 状态码透传（P2，M10）

保存并写出真实 status；补 `main_test.go`。  

### Task 7 — 绑定与文档收敛（P2）

1. 再生 bindings，消灭 WindowService ByName。  
2. README 树「21 服务」→ 与 ARCHITECTURE 一致表述。  
3. ARCHITECTURE 补全 AI 双窗与 tools 事件。  
4. CI 接入 `check-doc-numbers` + `check-bindings`。  

### Task 8 — 内联补全与主会话协调（P2，M9）

streaming 或 `globalStreamBusy` 时 disable/skip inline provider。  

### Task 9 — 发版卫生（P2）

1. 将当前脏工作树整理为逻辑提交（security / dual-window / agent-tools / docs）。  
2. 打 tag 规划：`0.1.1`（prompt-5 修复）→ `0.2.0`（双窗 SSOT）。  
3. 勿把 `gugacode.exe` / `node_modules` / 临时二进制提交进 git。  

### Task 10 — 实验特性收敛（P3）

Computer Use / IM 设置入口收到「实验」分组；默认不展示红点/推荐。  

### Task 11 — Agent 去重与协议硬化（P3）

1. native/fence 去重键纳入 content hash（write）。  
2. 评估强制 native-only（可配置）以降低双轨歧义。  

### Task 12 — GUI 验收清单（P3）

编写 `docs/qa-dual-window.md` 手工 10 步；可选后续自动化。  

---

## 9. prompt-6 Definition of Done

- [ ] Task 1 双窗设置/会话同步可演示  
- [ ] Task 2 streamId 协议落地（可仍单流互斥）  
- [ ] Task 3 Anthropic tool_use 有测试  
- [ ] Task 4 空 root 无法 WriteFile  
- [ ] Task 5/8 busy 与 inline 行为可预期  
- [ ] Task 6/7 文档与 ByName 收敛；CI 脚本接入  
- [ ] `go test ./services/...`、`go test .`、`vitest run`、eslint 全绿  
- [ ] CHANGELOG 记录 prompt-6 项  
- [ ] 工作树可拆成可 review 的提交集合  

---

## 10. 结论

**prompt-5 阶段目标基本达成**：致命的 Apply 假成功、双窗流互抢、测试全红、write 自动批准等已修复；工程脚本与文档对齐提升了可维护性。综合分从约 **7.3 → 7.9**。

**prompt-6 的核心不是继续堆功能**，而是：

1. 把双窗从「两个孤岛 Webview」做成 **有同步协议的产品**；  
2. 把 AI 事件从「全局广播」做成 **可路由的 stream 协议**；  
3. 补齐 Anthropic Agent 工具链与空 root 沙箱；  
4. 用发版纪律消化当前巨大 diff。  

完成 Task 1–4 后，项目更接近对外展示与日用的 **0.2.0** 质量线。

---

## 11. 附录：本轮验证原始结果

```
go test ./services/...     → ok (~43.4s)
go test .                  → ok (~3.1s)
vitest run                 → 52 passed | 1226 tests passed
eslint src                 → exit 0
check-doc-numbers.mjs      → OK (MAX_TOOL_CALLS=20)
check-bindings.mjs         → OK (14 ByName remaining)
```

```
prompt-5 关键缺陷：H1, H2, H3(诚实化), M1, M2, M3, M4, M6, L1, L2(部分), L6
prompt-5 残留/升级：M5(空 root), M7→H4(双窗同步), L5→M10(status), streamId 完整方案
本轮新关注：H5 Anthropic tool_use, M8 busy 竞态, M9 inline 争用, M11 ByName, L8–L14
```

---

*本文件由 prompt-6 审查流程生成，可直接作为下一迭代实现与 Code Review 检查清单。*
