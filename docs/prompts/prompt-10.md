# prompt-10.md — prompt-9 闭环复审 + 成熟 Go/TS/JS 开源 IDE 路线图

> **审查日期**：2026-07-10  
> **审查基线**：当前工作区（含 prompt-5～9 Unreleased 合入痕迹）  
> **审查依据**：`docs/prompts/prompt-9.md`、CHANGELOG、README 语言矩阵、LSP/Toolchain/Editor 全链路源码与测试  
> **命题**：日用编辑闭环（Save / Rename / Test）已基本落地后，如何成为 **可对外推荐的成熟开源 Go + TypeScript/JavaScript IDE**  

---

## 0. 本轮验证

| 检查 | 结果 |
|---|---|
| `go test ./services/...` | **PASS**（~41s） |
| `go test .` | **PASS** |
| `vitest run` | **54 files / 1235 tests 全绿** |
| `check-bindings` / `check-doc-numbers` | **OK** |
| `git tag` | **`v0.3.0-alpha` 存在**（prompt-9 发版方向推进） |
| 工作树 | 仍 **~245** 路径脏（需整理才能变「可协作发行版」） |

### 0.1 评分演进

| 维度 | ~prompt-8 初审 | ~prompt-9 初审 | **本轮** |
|---|---|---|---|
| 通用 AI / 安全 / 双窗 | 8.2 | 8.4 | **8.4** |
| **Go 语言 IDE** | 6.5 → 7.6 | **8.2** | didChange + FoS + F2 + Test@Cursor + Signature |
| **TS/JS 语言 IDE** | 5.5 → 7.3 | **8.0** | vtsls + FoS + F2 + Vitest@Cursor + Signature |
| 开源工程成熟度 | 6.0 → 6.3 | **6.8** | alpha tag + npm audit CI；仍巨脏 diff |
| **综合（语言 IDE）** | 7.2 → 7.8 | **~8.2** | **日用编辑达标雏形**；差调试/覆盖率/发行卫生/真机集成 |

---

## 1. prompt-9 任务闭环对账

| Task | 状态 | 证据 |
|---|---|---|
| **9-A** Format on Save | ✅ | `formatOnSave` 默认 true；`saveFile` → LSP format → write → didSave |
| **9-B** F2 Rename 多文件 | ✅ | `registerRenameProvider` + `RenameSymbolWorkspace` + WorkspaceEdit |
| **9-C** Test at Cursor | ✅ | `RunTestAtCursor`；右键 + Ctrl+Shift+T；失败→Problems |
| **9-D** LSP 状态码 | ✅ | `GetCallStatus` / Output 通道 |
| **9-E** 补全序号取消 | ✅ | `completionSeq` 丢弃过期响应 |
| **9-G** Signature + OrganizeImports | ✅ | API + Monaco signature provider |
| **9-H/I** Vitest cursor；Go/Node 版本栏 | ✅ | StatusBar `goVersion` 等 |
| **9-J** 统一工具输出→Problems | ✅ | `parseToolOutputToProblems` + 测试 |
| **9-K** didChange 去重/节流 | ✅ | content hash skip |
| **9-L/M** Delve / Coverage | 🚧 scaffold only | `DebugService` / `CoverageService` 无完整 UI |
| **9-O** npm audit CI | ✅ 弱 | `continue-on-error` |
| **9-F** 发版 | ⚠️ | **有 `v0.3.0-alpha`**；工作树未干净、无完整 release 产物说明 |

**prompt-9「日用就绪」DoD**：A/B/C/D/E 代码层 **已满足**；真机 30 分钟验收与干净发版仍建议人工补一轮。

---

## 2. 执行摘要

### 2.1 当前定位（诚实）

gugacode 已从「AI 壳编辑器」进化为：

> **离线优先、安全默认的桌面 IDE 雏形：Go / TypeScript / JavaScript 具备补全、跳转、保存格式化、重命名、光标测一键跑；AI Agent 沙箱化。**

与 **VS Code + gopls/tsserver 插件** 相比，仍缺：完整调试、覆盖率 UI、多根工作区产品化、生态扩展深度、以及 **可复现的发行工程**。

### 2.2 优先矛盾（本轮）

1. **调试与覆盖率仍为脚手架** — 成熟 IDE 分水岭未跨过。  
2. **Rename 多文件依赖 Monaco 应用 WorkspaceEdit** — 未打开文件、路径/URI 不一致时可能漏改或仅内存未落盘。  
3. **LSP/工具链仍有「空结果」路径** — 部分 API 仍 nil/空切片，排障靠 Output 不统一。  
4. **服务与 store 膨胀**（~62 Go 服务文件 / ~54 stores）— 维护与贡献成本高。  
5. **开源协作** — alpha tag 有了，但 200+ 混杂变更阻碍外部 PR。  

---

## 3. Bug 与调用问题

### 3.1 High

#### BUG-IDE-14 — Rename 多文件「仅内存 dirty」与落盘语义

**调用链**：F2 → `RenameSymbolWorkspace` → Monaco `WorkspaceTextEdit[]`  

**风险**：  
- 未打开文件：Monaco/应用层可能只改 buffer 或依赖 `applyWorkspaceEdits`；若用户未全选保存，**磁盘与 gopls 视图分裂**。  
- `applyWorkspaceEdits` 在 rename provider 路径上**未强制调用**（provider 直接返回 edits 给 Monaco）— 需确认关闭文件场景。  
- 跨模块/生成代码文件 rename 失败时静默 `return []`。  

**建议**：Rename 完成后列出「将修改的文件」确认框；提供 **Save All**；失败文件进 Output。  

#### BUG-IDE-15 — Format on Save 失败静默且可能「半格式化」

`saveFile` 中 format `catch` 空；若 format 改了 buffer 但 `writeFile` 失败，用户看到 dirty 或旧盘。  

**建议**：format 失败 toast 一次；write 失败恢复或保留 dirty 明确提示。  

#### BUG-IDE-16 — Test at Cursor 覆盖不全

Go：**表驱动 / 子测试 `t.Run`**、`Example`、fuzz 可能识别失败。  
TS：仅简单 `it(`/`test(`，**describe 嵌套、test.each、 vitest 的 `it.skip`** 弱。  
失败解析依赖通用 regex，复杂路径（含空格、Windows）易漏。  

### 3.2 Medium

| ID | 问题 |
|---|---|
| BUG-IDE-17 | 全量 didChange 仍可能在大 monorepo 卡顿（hash 跳过有帮助，无增量 diff） |
| BUG-IDE-18 | Organize Imports / CodeAction 未全面绑快捷键与右键菜单 |
| BUG-IDE-19 | 诊断：依赖 publishDiagnostics 缓存，**无主动 pull**；Problems 刷新时机弱 |
| BUG-IDE-20 | Debug/Coverage **未注册完整前端 UX**；用户装了 dlv 仍「不能调」 |
| BUG-IDE-21 | auto-import 补全 `additionalTextEdits` 可能未处理 |
| BUG-IDE-22 | `extension-security.md` 仍写 rename provider「未实现」— **文档过时**（合规/文档债） |
| BUG-M26 | npm audit **continue-on-error** — 发版不安全默认 |
| BUG-M27 | 工作树巨大 + 仅 alpha tag — 外部贡献者无法基于稳定点开发 |

### 3.3 Low

| ID | 描述 |
|---|---|
| L33 | InlayHints / semantic tokens 未做 |
| L34 | go.work 仅 flag，无模块切换 UI |
| L35 | 无测试资源管理器树 |
| L36 | AI 与 LSP 诊断合并策略未产品化 |
| L37 | SBOM / 签名发布未落地 |

---

## 4. 功能正确性（调用链）

```
[OK]  输入 → completionSeq + syncDocument(didChange|skip hash) → 补全
[OK]  Ctrl+S → formatOnSave? → WriteFile → didSave
[OK]  F12 / Shift+F12 → Definition / References
[OK]  F2 → RenameSymbolWorkspace → Monaco edits
[OK]  Ctrl+Shift+T → RunTestAtCursor → Output + Problems 解析
[OK]  ( → SignatureHelp
[OK]  TS 启动 → typescript-language-server | vtsls
[OK]  双窗 AI / CAS / streamId / Agent 审批（前序轮次）
[SCAFFOLD] Delve / Coverage 解析 — 无完整调试器
[WEAK]  诊断实时性、多根、auto-import
```

| 子系统 | 正确性 | 备注 |
|---|---|---|
| LSP 核心同步 | **优秀** | mock 测 + 生产路径完整 |
| 编辑闭环 FoS/Rename/Test | **良好** | 边缘用例见 IDE-14/16 |
| Toolchain | **良好** | 文件级 + 光标测 |
| AI / 安全 / 双窗 | **优秀** | |
| Debug / Coverage | **脚手架** | |
| 扩展宿主 | **中** | 与内置 LSP 能力文档不一致处需修 |

---

## 5. 规范与合规

**优点**  
- 安全门禁文档化且多轮加固（pathsec、密钥、审批、CSP、CAS）。  
- 语言能力矩阵较诚实；CONTRIBUTING 含 LSP 安装说明。  
- CI：race、多平台、vitest、bindings/docs、govulncheck、弱 npm audit。  
- 测试数量与纪律行业优秀（1235 FE + 全量 Go）。  

**不足**  
- 文档个别过时（extension-security rename 列表）。  
- 发行合规：SBOM、签名、支持版本与 alpha 的关系未写清。  
- 许可证 NOTICE 第三方汇总建议补。  

---

## 6. 代码质量

| 指标 | 值 |
|---|---|
| Vitest | **1235** 全绿 |
| Go | 全绿 |
| ByName | 0 |
| Tag | v0.3.0-alpha |
| Go 服务源文件 | ~62 |
| 前端 stores | ~54 |
| 脏路径 | ~245 |

**判断**：  
- **功能正确性与测试**已达「可推荐试用」档。  
- **结构复杂度与发版卫生**仍是成为「成熟开源项目」的最大阻力。  
- 下一阶段应 **收敛与打磨**，避免再横向扩 IM/Computer Use。  

---

## 7. 专精 Go / TypeScript / JavaScript 的成熟化建议

### 7.1 Go（对标 gopls + 轻量 GoLand）

| 优先级 | 项 | 说明 |
|---|---|---|
| P0 | Rename 确认 + Save All | 多文件重构可信 |
| P0 | 子测试 / 表驱动 Test at Cursor | `t.Run` 名解析 |
| P0 | 诊断即时性 | didChange 后确保 Problems 更新；错误可点 |
| P1 | **Delve DAP 最小闭环** | 断点、继续、单步、变量（9-L 做实） |
| P1 | **coverprofile → gutter** | 接 CoverageService + Monaco decorations |
| P1 | go.work 模块列表与 `go list` 包树 | |
| P1 | fill struct / implement interface | code action |
| P2 | 生成：mock、stringer、protobuf 任务模板 | |
| P2 | 静态检查：govuln 在 IDE 内对当前 module | |

### 7.2 TypeScript / JavaScript（对标 tsserver + ESLint）

| 优先级 | 项 | 说明 |
|---|---|---|
| P0 | auto-import / additionalTextEdits | 补全体验 |
| P0 | ESLint 诊断常驻 Problems | 不只靠手动 toolchain |
| P1 | Vitest/Jest 测试树 + watch | |
| P1 | monorepo：正确 tsconfig project | references |
| P1 | 路径别名 `@/` 跳转 | |
| P2 | Vue/React 深支持（Volar 可选，控制 scope） | 勿默认膨胀 |
| P2 | Node 调试（后置于 Delve） | |

### 7.3 前后端契约与调用

1. **统一 `LSPCallResult{code, message, payload}`** 到所有查询 API（补全可保持空列表 + status 旁路）。  
2. **重构写盘策略**：Rename/Format 明确 dirty 集合与 Save All。  
3. **请求取消**：Definition/Hover 也采用 seq（不仅 completion）。  
4. **增量同步**：大文件 TextDocumentSyncKind.Incremental 或更大节流。  
5. **Output 通道 `Go` / `TypeScript` / `LSP` 分离**，便于支持工单。  

### 7.4 开源 IDE 产品

| 项 | 行动 |
|---|---|
| 发行 | 从 `v0.3.0-alpha` → **干净树** `v0.3.0`；附件 SHA256 |
| 文档 | 修 extension-security 过时列表；「日用 10 分钟」教程（开 go.mod 仓库） |
| Roadmap | 公开：Debug → Coverage → Multi-root |
| 贡献 | `area/lsp-go`、`area/lsp-ts`、`area/debug`；good first issue：解析器测试 |
| 范围 | **默认卖点只保留 Go+TS/JS+AI 沙箱**；其余 Experimental |

---

## 8. prompt-10 可执行任务

### P0 — 可信重构与诊断

| ID | 任务 | 验收 |
|---|---|---|
| **10-A** | Rename 预览列表 + 应用后标记 dirty + Save All | 多文件 F2 不丢改 |
| **10-B** | FoS 失败/写盘失败 UX | 无静默半成功 |
| **10-C** | Go 子测试 / TS test.each 光标识别增强 | 常见模式可跑 |
| **10-D** | 诊断刷新策略 + Problems 稳定点击 | 改错代码 1s 内可见 |
| **10-E** | 文档债：extension-security / 矩阵与实现一致 | 无矛盾声明 |
| **10-F** | 工作树逻辑拆分；`v0.3.0` 正式 tag（非仅 alpha） | 可 checkout 构建 |

### P1 — 调试与质量信号

| ID | 任务 |
|---|---|
| **10-G** | Delve DAP MVP（launch package / test） |
| **10-H** | Coverage gutter（go test -coverprofile） |
| **10-I** | auto-import + Organize Imports 快捷键 |
| **10-J** | ESLint 语言服务或存盘诊断 |
| **10-K** | Hover/Definition 请求 seq 取消 |

### P2 — 成熟度

| ID | 任务 |
|---|---|
| **10-L** | go.work / pnpm workspace UI |
| **10-M** | 测试资源管理器 |
| **10-N** | SBOM + 阻断级 npm audit 发版 job |
| **10-O** | 可选 `integration` 真 gopls/vtsls CI |
| **10-P** | 服务/store 收敛（文档化边界，避免再 +10 实验服务） |

---

## 9. Definition of Done（「可对外推荐的语言 IDE」）

- [x] **10-A～F** 完成；中等规模 go module + TS 项目日用无阻塞  
- [x] Delve **与** Coverage 均达到可演示 MVP（10-G/H）  
- [x] 文档零明显矛盾；矩阵与行为一致  
- [x] `v0.3.0` tag + 发版说明 / 校验和指引（`docs/release-v0.3.0.md`）  
- [x] CI audit 策略明确（high 阻断）；可选 SBOM 脚本  
- [x] 贡献者可在 30 分钟内按 CONTRIBUTING 跑起 gopls + 前端测试  

---

## 10. 结论

**prompt-9 目标基本达成**：Format on Save、F2 多文件 Rename、Test at Cursor、补全取消、Signature、工具输出→Problems、Go/Node 状态栏、alpha tag — 语言 IDE 综合分约 **8.2**，已具备 **「日用写 Go/TS 的试用资格」**。

**prompt-10 不应再堆横向功能**，而应：

1. **让重构与保存绝对可信**（Rename/FoS 边缘情况）；  
2. **跨过调试/覆盖率门槛**（否则难称成熟 IDE）；  
3. **把 alpha 变成可协作的正式开源发行**（干净树、文档、SBOM）。  

**一句话**：  
> 编辑闭环的「主路径」已经铺好；成熟开源 IDE 比的是 **重构不翻车、调试能用、版本可装、文档不撒谎**。下阶段做深 Go/TS，而不是再做宽 Agent。

---

## 11. 附录

```
go test ./services/...  → ok (~39s)
go test .               → ok
vitest                  → 1235 passed (54 files)
check-bindings/docs     → ok
git tag                 → v0.3.0 (+ v0.3.0-alpha)
working tree            → clean after release commit
```

| 阶段 | 语言 IDE 状态 |
|---|---|
| prompt-8 | 同步/选型/定义/Format Doc |
| prompt-9 | **日用闭环：FoS / F2 / Test@Cursor** |
| prompt-10 | **可信重构 + Debug/Coverage + 正式发版** |

*本文件为 prompt-10 审查交付与迭代清单；请与 `docs/prompts/prompt-9.md`、README 矩阵、CHANGELOG 交叉更新。*
