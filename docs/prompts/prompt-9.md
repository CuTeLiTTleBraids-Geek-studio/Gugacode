# prompt-9.md — prompt-8 闭环复审 + 迈向成熟 Go/TS/JS 开源 IDE

> **审查日期**：2026-07-10  
> **审查基线**：当前工作区（prompt-8 语言 IDE 任务大部分已合入 Unreleased）  
> **审查依据**：`docs/prompts/prompt-8.md`、`CHANGELOG.md`、`README` 语言矩阵、LSP/Toolchain 源码与测试  
> **命题**：在 prompt-8「语言服务可用」之后，如何把 gugacode 推到 **可日常写 Go/TS/JS 的成熟开源 IDE**  

---

## 0. 验证结果

| 检查 | 结果 |
|---|---|
| `go test ./services/...` | **PASS**（~35s） |
| `go test .` | **PASS** |
| LSP/Toolchain 相关测试 | **PASS**（含 `TestLSP_syncDocument_DidOpenThenDidChange`） |
| `vitest run` | **53 files / 1232 tests 全绿** |
| `check-bindings` / `check-doc-numbers` | **OK** |
| `git tag` | **无正式 tag** |
| 工作树 | 仍 **~200+** 脏路径 |

### 0.1 评分（相对 prompt-8 审查时）

| 维度 | prompt-8 审查时 | **本轮** | 说明 |
|---|---|---|---|
| 通用 AI/安全/双窗 | ~8.2–8.4 | **8.4** | 基线保持 |
| **Go 语言 IDE** | **6.5** | **7.6** | didChange、Definition、Format、test-pkg 到位 |
| **TS/JS 语言 IDE** | **5.5** | **7.3** | vtsls/ts-lang-server、tsx languageId、vitest-file |
| 开源工程成熟度 | **6.0** | **6.3** | 文档/矩阵/CONTRIBUTING 改善；仍无 tag/发版 |
| **综合（Go/TS/JS IDE）** | **~7.2** | **~7.8** | **M1 达标，M2 大半完成；距 v1.0 语言就绪仍差 Format-on-Save UX、Rename UI、调试、发版** |

---

## 1. prompt-8 任务闭环对账

### 1.1 里程碑 M1（语言服务可用）— **已闭环**

| Task | 状态 | 证据 |
|---|---|---|
| **8-A** syncDocument | ✅ | `didOpen`/`didChange`(full)/`didClose`/`didSave` + version |
| **8-B** TS LSP 选型 | ✅ | `typescript-language-server` / `vtsls --stdio`，非 raw tsserver |
| **8-C** 路径 | ✅ | 前端优先 `openFiles[].path`；`pathToURI` 绝对路径 |
| **8-D** StatusBar LSP | ✅ | `lspStatusLabel` / `lspStatusDetail` |
| **8-E** mock 测试 | ✅ | `TestLSP_syncDocument_DidOpenThenDidChange` |

**BUG-IDE-01/02/03/04** 在代码层标记为 Fixed。

### 1.2 里程碑 M2（编辑器闭环）— **大部分完成，有缺口**

| Task | 状态 | 缺口 |
|---|---|---|
| **8-F** Definition / References | ✅ | Monaco `registerDefinitionProvider` / `ReferenceProvider` |
| **8-G** Format Document | ✅ 半完成 | **LSP Format 已接**；**Format on Save 未真正写回**（save 只 `didSave`，注释写 *hooks later*） |
| **8-H** Rename | 🚧 | 后端 `RenameSymbol` **仅当前文件** edits；**无 Monaco rename provider / UI**（矩阵写 🚧 诚实） |
| **8-I** 工具链粒度 | ✅ | `gofmt-file` / `eslint-file` / `prettier-file`；整仓写不再默认 |
| **8-J** 测试命令 | ⚠️ | `go-test-pkg`、`vitest-file` 有；**缺「光标处 TestXxx」** |

### 1.3 里程碑 M3 / M4

| Task | 状态 |
|---|---|
| **8-L** 语言矩阵 | ✅ README |
| **8-N** CONTRIBUTING LSP 段 | ✅ |
| **8-K** 拆 commit + tag | ❌ |
| **8-M** SBOM / 校验和 / npm audit 发版门禁 | ❌ |
| **8-O～R** Delve / Coverage / 多根 / 真多流 | ❌ 路线图 |

### 1.4 prompt-8 DoD（v1.0 语言就绪）对照

| 项 | 状态 |
|---|---|
| 8-A～E + 真机可补全 | 代码 ✅；**真机 gopls/vtsls 手工验收未在本审查执行** |
| F12 Definition | ✅ 已实现 |
| Format on Save `.go`/`.ts` | ❌ **未做完** |
| 当前包/文件测试 + 失败点回行 | ⚠️ 命令有；**失败定位/测试 gutter 弱** |
| StatusBar Go 版本 + LSP | ⚠️ **LSP 有，Go 版本无** |
| README 矩阵一致 | ✅ |
| CI LSP mock + race/vitest | ✅ mock 在 go test |
| 正式 tag + 产物 | ❌ |
| 可 review 工作树 | ❌ |

---

## 2. 前后端 Bug / 调用问题（本轮）

### 2.1 High（语言体验）

#### BUG-IDE-05 — Format on Save 未接线

**调用链期望**：Save →（可选）`formatLSPDocument` → `updateContent` → `WriteFile` → `didSave`  

**现状**：`saveFile()` 仅 `writeFile` + `didSaveDocument`；`formatActiveDocument` 存在但未挂到保存路径；设置项未见强制 format-on-save。  

**影响**：矩阵写「Format Document ✅」，用户以为 Ctrl+S 会 gofmt — **不会**。  

#### BUG-IDE-06 — Rename 无编辑器入口且跨文件不完整

**后端**：`RenameSymbol` → `parseWorkspaceEditsForURI` **过滤为当前 URI**。  
**前端**：API 有 `renameSymbol`，**未** `registerRenameProvider`，用户无法 F2。  

**影响**：Go 接口/TS 符号重命名不可用，与成熟 IDE 差距明显。  

#### BUG-IDE-07 — 失败仍「空结果静默」

`GetCompletions`/`GetDefinition`/… 在 server 未运行或 RPC 失败时多返回 **空切片 + nil error**。  
StatusBar 可显示 idle，但 **protocol_error 与「真无结果」不可分** — 排障困难（prompt-8 已提，未根治）。  

### 2.2 Medium

| ID | 问题 |
|---|---|
| BUG-IDE-08 | **每次**补全/悬停 `syncDocument` full didChange — 大文件可能卡顿；缺 debounce / 增量 / 请求序号取消 |
| BUG-IDE-09 | `go-test-pkg` 非 **Test at Cursor**；失败输出未稳定解析为 Problems 可点击 |
| BUG-IDE-10 | 关闭 Tab 调 `closeLSPDocument`，但 **Monaco language 与 lsp key** 若不一致可能 didClose 漏发 |
| BUG-IDE-11 | initialize **processId: 0** — 部分 server 可接受；建议传真实 PID |
| BUG-IDE-12 | 无 **SignatureHelp / InlayHints / CodeAction(quickfix)** |
| BUG-IDE-13 | gopls 多根 **go.work**、TS **project references** 无一等 UX |
| BUG-M25 | 巨型未拆分 diff + **无 git tag** — 开源协作阻塞 |

### 2.3 Low

| ID | 描述 |
|---|---|
| L28 | StatusBar 无 `go version` / `node` 工具链版本 |
| L29 | 无测试资源管理器（文件树旁 Test 图标） |
| L30 | 无 Delve / 覆盖率 |
| L31 | snipped completion / auto-import 未开 |
| L32 | `typescriptreact` 是否在 Monaco 主语言列表完整注册需真机确认 |

---

## 3. 功能正确性

### 3.1 关键调用链（更新）

```
[OK]  编辑 → completion → syncDocument(didChange) → gopls/vtsls → 新语义
[OK]  F12 → GetDefinition → Monaco location
[OK]  Format Document → FormatDocument → TextEdit → buffer
[GAP] Ctrl+S → 无 format-on-save → 仅磁盘 + didSave
[GAP] F2 Rename → 无 provider
[OK]  关 Tab → closeLSPDocument → didClose
[OK]  TS 启动 → typescript-language-server | vtsls --stdio
[WEAK] 工具链 go-test-pkg / vitest-file → Output；Problems 点回弱
[OK]  AI/Agent/双窗/CAS — 前几轮基线仍在
```

### 3.2 模块表

| 模块 | 正确性 | 备注 |
|---|---|---|
| LSP sync + mock 测 | **优秀** | M1 核心债已还 |
| LSP Definition/Format API | **良好** | |
| Rename | **中** | 半截产品 |
| Toolchain 文件级 | **良好** | |
| 前端 Monaco providers | **良好** | 缺 rename / signature |
| Format on Save | **缺失** | |
| 安全/双窗/AI | **优秀** | |
| 测试（单测） | **优秀** | 缺真 gopls CI integration tag |

---

## 4. 规范与合规

**优点**  
- README 诚实矩阵；CONTRIBUTING 写明勿用 raw tsserver  
- CI：race、vitest、bindings、docs、govulncheck  
- 变更带 prompt-8 Task 编号  

**不足**  
- 无 release tag / SBOM / 校验和流程落地  
- 失败静默与类型边界 `as unknown as` 仍在  
- 支持版本表仍偏模板化  

---

## 5. 代码质量

| 指标 | 值 |
|---|---|
| Vitest | 1232 全绿 |
| Go services | 全绿 |
| LSP mock 契约测 | 有 |
| 发版 tag | 无 |
| 脏工作树 | ~238 行 status |

**判断**：语言服务从「不可信」升到「架构正确、功能过半」；质量瓶颈从「致命 LSP bug」转为 **产品闭环（Save/Rename/Test UX）+ 工程发版**。

---

## 6. 专精 Go / TypeScript / JavaScript 的改进建议

### 6.1 立即（P0）— 让「每天写业务代码」不疼

#### Go

1. **Format on Save**（设置默认开）：save 前 `textDocument/formatting` 或 `gofmt` 写 buffer，再落盘。  
2. **F2 Rename** + **WorkspaceEdit 多文件**（至少同 package）。  
3. **Run Test at Cursor**：解析 `func TestXxx`，`go test -run ^TestXxx$`。  
4. **StatusBar：`go1.xx` + gopls 版本**。  
5. **go test 失败 → Problems**：解析 `file:line: ` 可点击。  
6. **补全请求取消**：新请求使旧 RPC 超时/忽略，避免乱序。  

#### TypeScript / JavaScript

1. 同样 **Format on Save**（Prettier 或 LSP format，**仅当前文件**）。  
2. **F2 Rename** + auto-import（completion resolve / code action）。  
3. **Vitest at Cursor**（`it`/`test` 名）与失败堆栈跳转。  
4. **ESLint diagnostics** 推 Problems（或 LSP 诊断轮询展示）。  
5. 确保 **tsx/jsx** Monaco language 与 `languageId` 一致。  
6. 安装引导：打开 `package.json` 且无 vtsls 时提示 `npm i -D typescript-language-server`。  

### 6.2 短期（P1）— 接近 VS Code「够用」

| 能力 | Go | TS/JS |
|---|---|---|
| Signature Help | gopls | tsserver via LSP |
| Code Actions | organize imports, fill struct | organize imports, fix all |
| Inlay hints | 可选 | 可选 |
| 调试 | **Delve DAP** | 可选 node debug（后置） |
| 覆盖率 | `-coverprofile` gutter | vitest coverage（后置） |
| 项目模型 | go.work 切换 | pnpm/npm workspaces 根 |

### 6.3 中期（P2）— 成熟开源 IDE

1. **多根工作区** UI（go.work modules / monorepo packages）。  
2. **测试资源管理器** + 持续测试。  
3. **增量 didChange**（或节流 full sync：仅 content hash 变化且 ≥80ms）。  
4. **LSP 可观测**：Output 通道 `LSP` 打印 initialize/错误，禁止只 swallow。  
5. **集成测试 job**（可选）：有 gopls 的 runner 跑 smoke（build tag `integration`）。  

### 6.4 开源产品（与语言并行 P0）

| 项 | 行动 |
|---|---|
| **发版** | 拆 commit → `v0.3.0`（language IDE M1+M2） |
| **矩阵** | Format on Save / Rename 行改为 🚧 直至 UI 就绪（避免过度承诺） |
| **SBOM** | goreleaser/syft + SHA256 |
| **Issue 标签** | `area/lsp-go` `area/lsp-ts` `good first issue` |
| **默认路径** | 打开含 go.mod 的文件夹 → 自动 start gopls + 状态提示 |

### 6.5 调用契约规范（长期）

```
LSPResult =
  | { ok: true, items }
  | { ok: false, code: "not_running" | "timeout" | "rpc" | "unavailable", message }
```

前端 StatusBar / 通知只在 `not_running` 与 `rpc` 时提示；`ok empty` 不打扰。

---

## 7. prompt-9 可执行任务清单

### P0（下一迭代必做）

| ID | 任务 | 验收 |
|---|---|---|
| **9-A** | Format on Save（设置 + save 管线） | Ctrl+S 后 `.go`/`.ts` buffer 已格式化再写盘 |
| **9-B** | Monaco `registerRenameProvider` + 多文件 WorkspaceEdit 应用 | F2 跨文件 rename 可用 |
| **9-C** | Go Test at Cursor + 失败行可点 | 光标在 Test 内一键跑 |
| **9-D** | LSP 错误码区分 + StatusBar/Output | 协议失败可见 |
| **9-E** | 补全/定义请求序号取消 | 快速输入不乱序 |
| **9-F** | 发版：拆分提交 + tag `v0.3.0-alpha` 或 `v0.3.0` | `git tag` 可见 |

### P1

| ID | 任务 |
|---|---|
| **9-G** | SignatureHelp + Organize Imports |
| **9-H** | Vitest/Jest at cursor；ESLint→Problems |
| **9-I** | StatusBar Go/Node 版本 |
| **9-J** | go test / tsc 输出 → Problems 统一解析器 |
| **9-K** | didChange 节流（hash + 100ms） |

### P2+

| ID | 任务 |
|---|---|
| **9-L** | Delve 调试适配器 |
| **9-M** | Coverage gutter |
| **9-N** | go.work / pnpm workspace UX |
| **9-O** | SBOM + npm audit 发版 job |
| **9-P** | integration 真 gopls CI（可选） |

---

## 8. Definition of Done（本轮后的「日用就绪」）

在 README 宣称 **「可日用 Go/TS 开发」** 前：

- [ ] **9-A** Format on Save 默认可用  
- [ ] **9-B** F2 Rename 可用（至少同包多文件）  
- [ ] **9-C** Test at Cursor（Go）  
- [ ] **9-D/E** 失败可见 + 请求不乱序  
- [ ] 真机：中等 go module + TS 项目手工 30 分钟无阻塞  
- [ ] **9-F** 至少一个语义化 tag  
- [ ] 语言矩阵与行为一致（Format on Save 行更新）  
- [ ] 全量 go test + vitest + scripts 绿  

（Delve/Coverage 可留 v1.1，不阻塞「日用编辑」。）

---

## 9. 结论

**prompt-8 的核心目标已基本达成**：  
`syncDocument`、TS 正确 LSP 进程、路径、StatusBar、Definition/Format Document、文件级工具链、mock 测试 — 把语言层从 **5.5～6.5 拉到 ~7.5+**。

**prompt-9 的重心应是「编辑闭环与日用」**，而不是再开新 AI 能力：

1. **Save = 格式化 + 落盘 + didSave**  
2. **Rename / Test at Cursor 产品化**  
3. **错误可见、请求可取消**  
4. **真正做一次开源发版（tag）**  

然后再进入 Delve、覆盖率、多根 workspace（M4）。

**一句话**：  
> 语言服务已经「会说话」了；下一步要让它在 **保存、重构、跑测试** 三条肌肉记忆路径上像成熟 IDE 一样可靠，并把版本从「工作区草稿」变成「可安装的开源发行版」。

---

## 10. 附录

```
go test ./services/...     → ok
go test .                  → ok  
vitest                     → 1232 passed
check-bindings / docs      → ok
git tag                    → (empty)
```

| prompt-8 项 | 本轮结论 |
|---|---|
| M1 8-A～E | ✅ 闭环 |
| M2 8-F/I | ✅ |
| M2 8-G Format on Save | ❌ 缺口 → 9-A |
| M2 8-H Rename UI | 🚧 → 9-B |
| M2 8-J Test at cursor | ⚠️ → 9-C |
| M3 tag/SBOM | ❌ → 9-F/9-O |
| M4 Debug/Coverage | ❌ → 9-L/M |

*本文件作为 prompt-9 审查交付与下一迭代检查清单；与 `docs/prompts/prompt-8.md`、README 语言矩阵交叉维护。*
