# prompt-7.md — gugacode 第三轮全方位审查与后续提升路线

> **审查日期**：2026-07-10  
> **审查基线**：当前工作区（大量未提交改动；含 prompt-5 / prompt-6 落地痕迹）  
> **审查依据**：`CHANGELOG.md` Unreleased（prompt-6 条目）、`docs/ai-windows.md`、`docs/qa-dual-window.md`、源码与测试；仓库内 **当前不存在** `prompt-6.md` / `prompt-5.md` 原文（以 CHANGELOG + 代码注释反推 DoD）  
> **目的**：闭环复审 prompt-6 交付；重新评估前后端缺陷 / 正确性 / 规范合规 / 质量；给出 **prompt-7** 可执行改进清单  

---

## 0. 审查说明

### 0.1 文档链

| 文档 | 角色 | 仓库现状 |
|---|---|---|
| prompt-5 | 第一轮审计 + P0 修复清单 | **文件缺失**（实现已在代码/CHANGELOG） |
| prompt-6 | 双窗 SSOT、streamId、Anthropic tools、空 root 写拒绝等 | **文件缺失**（实现已在代码/CHANGELOG） |
| **prompt-7（本文件）** | 第三轮复审 + 下一迭代任务 | 本次写入 |

> **建议（Task R）**：将 `prompt-*.md` 纳入版本库并随发版归档，避免审查基线丢失。

### 0.2 审查方法

1. 对照 CHANGELOG「Added/Fixed (prompt-6)」与源码逐项核对  
2. 静态阅读：双窗同步、streamId 路由、File 写拒绝、Anthropic tool_use、CI  
3. 动态验证（本机）：

| 检查 | 结果 |
|---|---|
| `go test ./services/...` | **PASS**（~46s） |
| `go test .` | **PASS**（~3s） |
| `vitest run` | **52 files / 1229 tests 全绿** |
| ESLint `src` | **exit 0** |
| `node scripts/check-bindings.mjs` | **OK，ByName=0** |
| `node scripts/check-doc-numbers.mjs` | **OK，MAX_TOOL_CALLS=20** |
| CI 是否跑 docs/bindings check | **未接入**（仅 Taskfile） |

### 0.3 总体评分（演进）

| 维度 | ~prompt-5 | ~prompt-6 目标 | **本轮** | 说明 |
|---|---|---|---|---|
| 功能完整度 | 8.0 | 8.2 | **8.3** | streamId、SSOT、Anthropic tools 齐备 |
| 实现正确性 | 7.0 → 8.0 | 8.3 | **8.3** | 关键双窗/流/写盘路径可信 |
| 安全性 / 合规 | 8.0 → 8.3 | 8.5 | **8.5** | 空 root 拒绝写；write 永不 auto；busy 信后端 |
| 代码规范 | 7.5 → 8.0 | 8.2 | **8.2** | ByName=0；文档事件表更新 |
| 代码质量 | 7.0 → 7.8 | 8.2 | **8.2** | 1229 全绿；Go race 测在 CI 设计中 |
| 可维护 / 可演进 | 6.5 → 7.2 | 7.5 | **7.4** | 仍服务膨胀 + 巨型脏工作树 |
| **综合** | **7.3 → 7.9** | **~8.2** | **~8.2** | **已达可演示 / 日用雏形**；生产加固与发版卫生仍是主战场 |

---

## 1. prompt-6 交付闭环复审

### 1.1 Task / Bug 落地状态

| 项 | 状态 | 证据 |
|---|---|---|
| **Task 1 / H4** 双窗 SSOT | ✅ | `settings:changed` / `conversation:saved` / `agent:pending-updated` + `windowOrigin` 防环；`docs/ai-windows.md` |
| **Task 2 / streamId** | ✅ | `StartStream() (string, error)`；事件 `{streamId,...}`；前端 `activeStreamId` + `isOwnedStreamEvent` |
| **Task 3 / H5** Anthropic tool_use | ✅ | `parseAnthropicSSEStreamWithTools` + 测试 fixture |
| **Task 4 / M5** 空 root 写拒绝 | ✅ | `validateMutatingPath` 用于 Write/Create/Delete/Rename |
| **Task 5 / M8** busy 仅信后端 | ✅ | done/Stop 不清 `globalStreamBusy`；单测覆盖 |
| **Task 6 / M10** status 透传 | ✅ | `responseRecorder` 存 status；`main_test.go` |
| **Task 7 / M11** ByName=0 | ✅ | `check-bindings.mjs` 强制 0；Window ByID |
| **Task 8 / M9** inline 与流 | ✅ | 前端 skip + 后端 `Complete` busy 拒绝 |
| **Task 10** 实验分组 | ✅ | Settings 导航 Computer Use / IM 实验区 |
| **Task 12** QA 清单 | ✅ | `docs/qa-dual-window.md` |
| **L8/L9 文档** | ✅ | ARCHITECTURE 事件表含双窗/tools |
| **L12 去重 content hash** | ✅（CHANGELOG） | write 去重键含 content |
| **多流真并发** | ⚠️ 协议有、产品仍单流互斥 | 可接受；属增强项 |
| **Agent 审批跨窗完整 UI** | ⚠️ 仅摘要 | 设计取舍；完整审批仍在发起窗 |
| **发版卫生 / tag** | ❌ | 工作树仍巨脏；tag 未切；prompt 文档未入库 |

### 1.2 prompt-6 DoD 对账（反推）

- [x] 双窗设置/会话同步可演示（代码路径完备；建议按 qa 清单手工再验）  
- [x] streamId 协议落地（默认仍单流互斥）  
- [x] Anthropic tool_use 有测试  
- [x] 空 root 无法 WriteFile  
- [x] busy / inline 行为可预期  
- [x] ByName=0、doc numbers 脚本通过  
- [x] Go / Vitest / ESLint 全绿  
- [x] CHANGELOG 记录 prompt-6  
- [ ] **工作树拆成可 review 提交 / 打 tag** — **未完成**  

**结论：prompt-6 功能与安全目标基本闭环；「工程发版」与「跨窗 Agent 审批深度」转入 prompt-7。**

---

## 2. 执行摘要

### 做得好的地方（累计 prompt-4→6）

1. **安全基线可测**：G-SEC 系列、pathsec、密钥不回前端、Agent 审批、CSP nonce、空 root 写拒绝。  
2. **双窗从孤岛走向协议化**：origin 防环、SSOT 事件、streamId 路由、Apply Diff、QA 清单。  
3. **测试纪律**：Go services + main 全绿；前端 **1229** 通过；bindings/docs 脚本可门禁。  
4. **Agent 工具链双轨**：OpenAI + Anthropic 原生 tools + fence 回退 + 去重。  
5. **实验特性诚实**：Computer Use 标注 stub；设置导航分组。  

### 仍制约 0.2 / 生产的主因

1. **巨型未整理 diff** — 审查与回滚成本极高；prompt 文档未入库。  
2. **CI 未跑** `docs:check` / `bindings:check` — 本地有脚本、流水线无强制。  
3. **Agent 审批非跨窗 SSOT** — 对端只见摘要，无法在 AI 窗批主窗发出的 tool call。  
4. **会话并发编辑仍有窗口** — 流式中跳过 reload，仍可能分叉后写覆盖。  
5. **产品面过宽** — 核心编辑体验与实验模块同仓同默认入口争抢注意力。  
6. **无自动化双窗 E2E** — 仅手工 QA 清单。  

---

## 3. Bug 与风险清单（本轮）

### 3.1 High（体验 / 数据一致性）

#### BUG-H6 — 会话 SSOT 在「对端正在 streaming」时不 reload

**位置**：`stores/ai.ts` `conversation:saved` 监听  

**行为**：仅当 `!streaming && !pendingAssistantMessage` 时 `loadConversation`。  
若 A 窗在生成中、B 窗保存同 id，A 结束后可能仍持旧分叉再 `persistConversation` **覆盖** B。  

**建议**：  
- 保存时带 `revision` / `updatedAt`；persist 前 CAS；  
- 或 streaming 结束强制 pull；冲突则 fork 新 id 并提示。

#### BUG-H7 — Agent 审批队列非跨窗可操作（设计债升级）

**文档已承认**：`agent:pending-updated` 仅摘要。  
**产品风险**：用户在 AI 窗对话触发工具后切到主窗，看不见完整审批卡 → 误以为 Agent 卡住。  

**建议（三选一）**：  
1. 后端持久化 pending tool calls + 两窗共享 store 重建；  
2. 有 pending 时强制聚焦发起窗并 toast；  
3. 产品声明「审批仅在发起窗」并做强 UI 指引。

### 3.2 Medium

#### BUG-M13 — `GenerateTitleWithAI` 未纳入流互斥 / busy 门闸

`persistConversation` 在 `ai:done` 后可能调用 `generateTitleWithAI`（非 StartStream）。与用户立刻再发消息并行时争用配额；Anthropic 路径 title 可能直接失败（有 reject 测试）。  

**建议**：title 生成走低优先级队列；busy 时跳过 AI title 用截断启发式。

#### BUG-M14 — Settings 仍 last-write-wins

`settings:changed` → `loadSettings` 可同步，但两窗几乎同时 debounce save（500ms）仍可能丢一方字段。  

**建议**：后端 settings 写带 `version` 字段；冲突时 merge 或拒绝并提示 reload。

#### BUG-M15 — CI 未强制 docs/bindings 脚本

Taskfile 有 `docs:check` / `bindings:check`，`.github/workflows/ci.yml` 的 frontend-test **未调用**。回归可能本地绿、合并后文档/绑定漂移。  

**建议**：frontend-test 增加一步 `node ../scripts/check-*.mjs`（或 task）。

#### BUG-M16 — `SetConfig` / Tools 等边界仍 `as unknown as`

`api/services.ts` 约 **10+** 处强转；bindings 与运行时形状漂移时 TS 无法保护。  

**建议**：再生 bindings 后收紧 Tools 类型；减少 cast。

#### BUG-M17 — 流式 chunk 在 `activeStreamId` 赋值前的竞态窗口

`isOwnedStreamEvent` 对「有 pending 但 streamId 未回」放行本窗 — 正确；但若 StartStream IPC 极慢而 busy 事件已广播，对端 UI 已锁、本窗 id 仍空，短暂不一致。  

**建议**：后端在返回 streamId 之前不发 chunk（已基本如此）；前端在拿到 id 前可缓冲 chunk。

#### BUG-M18 — Read/List 在空 root 下仍任意路径

写已拒绝；读仍开放。本地桌面威胁模型可接受，但恶意插件/扩展若拿到 FileService 读绑定仍可扫盘。  

**建议**：扩展/Agent 读也要求 root；或提供 `AllowReadOutsideRoot` 显式开关默认 false。

### 3.3 Low / 体验 / 工程

| ID | 描述 |
|---|---|
| BUG-L15 | 真多流并发未开放（互斥保留）— 产品决策，非缺陷 |
| BUG-L16 | Computer Use / 部分 VS Code API 仍 stub |
| BUG-L17 | Anthropic 无 inline completion |
| BUG-L18 | `prompt-5/6.md` 缺失；审查基线易丢 |
| BUG-L19 | 工作树 200+ 路径变更未逻辑拆分提交 |
| BUG-L20 | 无 Playwright/Wails GUI E2E；双窗仅手工清单 |
| BUG-L21 | `generateTitle` / 部分 AI 辅助调用错误体验不统一 |
| BUG-L22 | 大仓 FileTree 已虚拟化，但 Git 状态 / Search 超大结果集仍可能卡 UI |

---

## 4. 功能正确性审查

### 4.1 后端

| 模块 | 正确性 | 备注 |
|---|---|---|
| AI StartStream + streamId + 互斥 | **优秀** | 事件结构化；ErrStreamBusy |
| OpenAI / Anthropic tools 流 | **优秀** | 双侧 tool 解析 + 测试 |
| Complete / inline busy | **良好** | 与主会话协调 |
| File 变更路径空 root | **优秀** | validateMutatingPath |
| Agent 执行 / 审批模型 | **良好** | run/write 永不 auto |
| Window 双窗生命周期 | **良好** | 启动可配；关闭联动 |
| Settings / Secrets | **良好** | G-SEC-07 |
| responseRecorder status | **良好** | 已测 |
| Computer Use | **诚实 stub** | 实验分组 |
| 服务装配 bootstrap | **良好** | 可维护性↑ |

### 4.2 前端

| 模块 | 正确性 | 备注 |
|---|---|---|
| streamId 路由装配 | **良好** | isOwnedStreamEvent |
| globalStreamBusy | **良好** | 信后端 + 乐观发送 |
| 设置跨窗同步 | **良好** | origin 防环 |
| 会话跨窗同步 | **中上** | 见 H6 流中不 reload |
| Apply Diff | **良好** | prompt-5 修复保持 |
| Agent 原生+ fence | **良好** | 去重 |
| Agent 跨窗审批 UI | **弱** | 见 H7 |
| 设置实验分组 | **良好** | |
| 测试 | **优秀** | 1229 全绿 |

### 4.3 跨切

| 主题 | 评估 |
|---|---|
| 安全 G-SEC | 维持并增强（空 root 写） |
| i18n | 双窗/实验/busy 文案齐全 |
| 离线 LSP/工具链 | 仍需真机抽测（本轮未做） |
| 性能 | 消息 200、工具 20；大仓路径改善 |

---

## 5. 代码规范性与合规性

### 5.1 规范

**优点**  
- Task/BUG 编号贯穿注释与 CHANGELOG。  
- 脚本门禁（bindings/docs）可复用。  
- 双窗协议独立文档 + QA 清单。  
- ByName 清零，生成绑定优先。  

**不足**  
- **活文档 prompt-\*.md 未入库**。  
- CI 未跑 Taskfile 检查脚本。  
- `as unknown as` 边界仍多。  
- 巨型混合 diff 违反「可 review 提交」原则。  

### 5.2 合规 / 安全

| 门禁 | 状态 |
|---|---|
| G-SEC-01~12 主体 | ✅ |
| write 永不 auto-approve | ✅ |
| 空 root 禁止写 | ✅ |
| 密钥不回前端 | ✅ |
| race + govulncheck CI | ✅（workflow 存在） |
| docs/bindings CI | ⚠️ 缺失 |
| 扩展沙箱 / 默认禁用 | ✅ |
| 读路径空 root | ⚠️ 仍松（M18） |

### 5.3 许可证

MIT；建议 0.2.0 附第三方 NOTICE。

---

## 6. 代码质量评估

### 6.1 量化

| 指标 | 值 |
|---|---|
| Go services / main 测试 | 全绿 |
| Vitest | **1229 / 1229** |
| ESLint | 0 error |
| ByName | **0** |
| MAX_TOOL_CALLS / README | 对齐 |
| MAX_AI_MESSAGES | 200 |
| 工作区脏路径 | **200+ 行 git status** |

### 6.2 判断

- **正确性与测试**已进入「可对外演示」档。  
- **工程卫生**仍是最大质量拖累：无法干净 bisect、难做安全审计签署。  
- **架构**：双窗协议成型，但 Agent 状态与会话 CAS 仍偏「尽力同步」。  
- **复杂度**：Plan 11 全家桶仍在；实验入口已降调，可继续砍默认曝光。  

---

## 7. 后续提升打量（战略）

### 7.1 建议版本叙事

| 版本 | 主题 |
|---|---|
| **0.1.1** | prompt-5 修复集（若尚未 tag） |
| **0.2.0** | prompt-6 双窗 SSOT + streamId + Anthropic tools（功能已齐，差整理发布） |
| **0.2.1 / 0.3.0（prompt-7）** | 会话 CAS、Agent 审批可见性、CI 门禁、发版卫生、可选真多流 |

### 7.2 原则

1. **先发布，再加功能** — 当前价值被「不可 review 的巨 diff」锁死。  
2. **双窗以 SSOT 为中心** — 任何跨窗状态必须有 revision 或明确「仅发起窗」。  
3. **实验模块默认折叠** — 不占核心路径。  
4. **CI = 真相** — 本地脚本必须上流水线。  

---

## 8. prompt-7 可执行任务清单

### Task A — 发版卫生与活文档（P0）

1. 恢复并提交 `prompt-5.md` / `prompt-6.md` / 本文件（或归档到 `docs/prompts/`）。  
2. 将当前工作树按主题拆 commit（security / dual-window / agent-tools / docs / ci）。  
3. 确认 `.gitignore` 排除 `*.exe`、临时二进制、`node_modules`。  
4. 打 tag：`v0.2.0`（或按团队版本策略）。  

**验收**：干净 `git status`；tag 可检出构建。

### Task B — CI 接入脚本门禁（P0）

1. `frontend-test` job 增加：  
   - `node scripts/check-bindings.mjs`  
   - `node scripts/check-doc-numbers.mjs`  
2. 可选：覆盖率阈值告警（不阻断）  

**验收**：故意改坏 ByName 或 MAX_TOOL 文档 → CI 红。

### Task C — 会话 CAS / 冲突处理（P0，H6）

1. Conversation 元数据增加 `updatedAt` 或单调 `revision`。  
2. `Save` 带 If-Match；冲突返回明确错误。  
3. 前端 streaming 结束后 pull；冲突 UI：「保留本地 / 采用远端 / 另存」。  

**验收**：双窗交错保存同 id 不静默丢数据。

### Task D — Agent 审批跨窗策略（P1，H7）

**推荐 D1（低成本）**：pending 时 Emit 强提示 + `Focus` 发起窗；设置文案写清。  
**推荐 D2（完整）**：pending 队列落盘或后端内存 SSOT，两窗可渲染同一审批卡。  

**验收**：qa-dual-window 增补步骤并通过。

### Task E — Title 生成与配额（P1，M13）

1. busy 时跳过 AI title。  
2. 不与 StartStream 抢同一超时预算。  

### Task F — Settings version（P1，M14）

1. `settings.json` 增加 `version`。  
2. Save 冲突检测；Emit 带 version。  

### Task G — 读路径沙箱策略（P2，M18）

无 root 时拒绝 Agent/插件触发的 Read；IDE 打开文件走「用户点选」例外 API。  

### Task H — 类型边界收敛（P2，M16）

再生 bindings；消灭 Tools / StartStream 返回类型上的 `unknown` 强迫症式 cast。  

### Task I — 可选真多流（P2/P3）

在 streamId 协议上开放 N=2 并发流（主窗+AI 窗各一），去掉全局互斥或改为 per-window 配额。  

### Task J — GUI E2E 冒烟（P2）

将 `docs/qa-dual-window.md` 至少 3 步自动化（或 Wails 集成测试桩）。  

### Task K — 产品收敛（P3）

默认隐藏 IM；Computer Use 保持实验；README 功能表标注 Experimental。  

### Task L — 性能（P3）

Search/Git 大结果虚拟列表；对话导出/清理策略。  

---

## 9. prompt-7 Definition of Done

- [ ] Task A：工作树可 review；prompt 文档入库；可选 tag `v0.2.0`  
- [ ] Task B：CI 跑 bindings + docs check  
- [ ] Task C：会话冲突不再静默覆盖  
- [ ] Task D：Agent 审批跨窗策略落地（完整 UI 或强指引）  
- [ ] Task E/F：title 与 settings 竞态可预期  
- [ ] `go test ./services/...`、`go test .`、`vitest run`、eslint、check-scripts 全绿  
- [ ] CHANGELOG 增加 prompt-7 段  
- [ ] `docs/qa-dual-window.md` 同步新步骤  

---

## 10. 结论

**prompt-6 在功能与安全维度基本成功**：双窗 SSOT、streamId、Anthropic tool_use、空 root 写拒绝、busy/inline 协调、ByName 清零均已在代码与测试中体现。综合分稳定在约 **8.2/10**，相对 prompt-5 初审（7.3）有实质跃迁。

**prompt-7 的核心不是继续堆 AI 能力**，而是：

1. **把已实现能力「发布」出来**（提交卫生 + tag + 活文档）；  
2. **把双窗数据一致性从「尽力同步」升级到「可冲突检测」**；  
3. **补齐 CI 与 Agent 审批产品闭环**；  
4. 继续 **收敛实验面**，保护编辑器核心体验。  

完成 Task A–D 后，项目更接近可对外分发的 **0.2.x**；Task I–L 属于增强与体验抛光。

---

## 11. 附录：本轮验证原始结果

```
go test ./services/...     → ok (~45.7s)
go test .                  → ok (~3.0s)
vitest run                 → 52 passed | 1229 tests passed
eslint src                 → exit 0
check-bindings.mjs         → OK (ByName=0)
check-doc-numbers.mjs      → OK (MAX_TOOL_CALLS=20)
CI docs/bindings steps     → NOT wired in ci.yml
prompt-5.md / prompt-6.md  → missing from workspace
git status                 → 200+ changed/untracked paths
```

```
prompt-6 已闭环（代码级）：H4, H5, M5, M8, M9, M10, M11, L8/L9/L12, Task1–4,5–8,10,12
prompt-6 未闭环：发版卫生 / tag / 活文档入库
本轮新关注：H6 会话覆盖, H7 审批跨窗, M13–M18, L15–L22
```

---

*本文件由 prompt-7 审查流程生成，可直接作为下一迭代实现与 Code Review 检查清单。建议与 CHANGELOG、docs/ai-windows.md、docs/qa-dual-window.md 交叉引用。*
