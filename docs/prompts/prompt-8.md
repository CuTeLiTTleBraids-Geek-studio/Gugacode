# prompt-8.md — 面向「成熟开源 Go / TypeScript / JavaScript IDE」的全方位审查

> **审查日期**：2026-07-10  
> **审查基线**：当前工作区（含 prompt-5～7 落地痕迹；`prompt-7.md` 已归档至 `docs/prompts/`，根目录可缺省）  
> **审查依据**：`docs/prompts/prompt-7.md`、`CHANGELOG.md`、`docs/ai-windows.md`、`SECURITY.md`、`ARCHITECTURE.md`、源码与测试  
> **核心命题**：在通用安全/双窗/Agent 基线之上，**如何成为专精 Go + TS/JS 的成熟开源桌面 IDE**  

---

## 0. 审查说明与验证结果

### 0.1 本轮验证

| 检查 | 结果 |
|---|---|
| `go test ./services/...` | **PASS**（~44s） |
| `go test .` | **PASS**（~3s） |
| `vitest run` | **53 files / 1232 tests 全绿** |
| `check-bindings.mjs` | **OK，ByName=0** |
| `check-doc-numbers.mjs` | **OK** |
| CI `check-bindings` / `check-doc-numbers` | **已接入**（prompt-7 Task B） |

### 0.2 评分演进

| 维度 | prompt-5 | prompt-6/7 后 | **本轮（IDE 成熟度视角）** |
|---|---|---|---|
| 通用功能 / 安全 / 双窗 | 7.3 → 8.2 | **~8.4** | 协议与门禁扎实 |
| **Go 语言 IDE 能力** | — | — | **6.5**（有 gopls/工具链骨架，缺完整编辑闭环） |
| **TS/JS 语言 IDE 能力** | — | — | **5.5**（tsserver 协议路径可疑，LSP 功能面窄） |
| 开源工程成熟度 | — | — | **6.0**（测试好，发版/拆分/社区流程弱） |
| **综合（作为「Go/TS/JS 专用 IDE」）** | — | — | **~7.2** |

> 作为「带 AI 的桌面编辑器」约 **8.2**；作为「可替代日常 GoLand / VS Code 写 Go/TS 的开源 IDE」仍约 **7.0～7.2**。差距主要在 **语言服务深度与工具链产品化**，不在再堆 Agent 模式。

### 0.3 prompt-7 闭环摘要

| Task | 状态 |
|---|---|
| A 活文档 | ✅ `docs/prompts/*` |
| B CI 门禁 | ✅ ci.yml 跑 bindings/docs |
| C 会话 CAS | ✅ revision + conflict |
| D Agent 跨窗 D1 | ✅ 发起窗审批 + toast |
| E title 配额 | ✅ busy 跳过 AI title |
| F settings version | ✅ expectedVersion CAS |
| G Agent 无 root 读拒绝 | ✅ |
| H 类型边界 | ⚠️ 部分收紧，`as unknown as` 仍约 10 处 |
| I 真多流 | ⏸ 产品默认单流 |
| J 双窗 smoke | ✅ dual-window-smoke.test.ts |
| K 实验面 | ✅ |
| L UI caps | ✅ Search/Git 上限 |
| **发版 tag / 干净提交** | ❌ 工作树仍 ~200+ 路径 |

---

## 1. 执行摘要

### 1.1 已具备的「准 IDE」基石

- 桌面单二进制（Wails）+ Monaco + 终端 + Git + 搜索  
- 路径沙箱、密钥加密、Agent 强制审批、CSP、扩展默认禁用  
- 双窗 AI 协议（streamId、SSOT、CAS、QA 清单）  
- **Go/TS/JS 差异化声明**：LSP（gopls/tsserver）、工具链目录、五类脚手架模板  
- 测试纪律优秀（Go + 1232 Vitest）  

### 1.2 阻碍「成熟开源 Go/TS/JS IDE」的主因

1. **LSP 文档同步不完整** — 仅 `didOpen` 一次，**无 `didChange`/`didClose`/`didSave`** → 补全/悬停基于**打开时旧缓冲**，属功能级正确性缺陷（见 BUG-IDE-01）。  
2. **TS/JS 服务进程选型错误风险** — 直接启动 `tsserver` 却走 **LSP JSON-RPC**；标准做法是 `typescript-language-server`（或 `vtsls`）包装。当前路径在真实项目中极易「检测有、启动后补全空」。  
3. **语言功能面远低于 IDE 预期** — 有 Completion/Hover/Diagnostics 缓存；**缺 Definition / References / Rename / SignatureHelp / Format / CodeAction / Organize Imports / InlayHints**。  
4. **工具链「列表」多于「闭环」** — `gofmt`/`goimports` 多为 `-l` 列表；缺少一键 Format on Save、当前文件/包级 test、coverage、go.work 多模块。  
5. **Monaco URI / 工作区路径** — 前端用 `model.uri.path`，Windows 与 `file://` URI、工作区相对路径易错，导致 gopls 找不到文件。  
6. **开源产品化** — 巨型未拆分 diff、无稳定 tag、贡献者路径与「语言能力路线图」对外不清晰。  

---

## 2. 前后端 Bug 与调用问题（含 IPC 调用链）

### 2.1 Critical / High（语言 IDE）

#### BUG-IDE-01 — LSP 无 didChange：编辑后语义过期

**调用链**：  
`Monaco provideCompletionItems` → `getLSPCompletions` → `LSPService.GetCompletions` → `ensureOpen`  

**缺陷**：`ensureOpen` 仅在首次 `didOpen` 推送 `req.Content`；再次请求**不发送** `textDocument/didChange`，也**不重新 didOpen**。  

**影响**：用户改代码后补全/悬停/诊断仍对旧文本；Go 与 TS 同等受害。  

**修复方向**：  
- 每请求带 version++；已打开则 `didChange`（full 或 incremental）；  
- 关 Tab 时 `didClose`；保存时 `didSave`。  

#### BUG-IDE-02 — tsserver ≠ LSP server

**位置**：`startServerProcess` case typescript/javascript：`exec.Command(exePath)` 直接跑 tsserver。  

**事实**：`tsserver` 使用 **Microsoft 专有协议**；LSP 客户端应启动：  
- `typescript-language-server --stdio`，或  
- `vtsls --stdio`  

**调用失败表现**：initialize 超时/失败、静默空补全（代码多处「返回空而非 error」掩盖问题）。  

**修复**：Detect 优先 `typescript-language-server` / `vtsls`；fallback 文案提示 `npm i -D typescript-language-server`。  

#### BUG-IDE-03 — 前端 filePath / URI 不一致

**调用链**：`model.uri.path` → Go `pathToURI(req.FilePath)`  

**风险**：  
- Windows 下 path 可能缺盘符或与 workspace root 拼接错误；  
- gopls 对非 workspace 文件 / 错误 URI 返回空。  

**修复**：统一 `file://` 绝对路径；前端传 `app` 已知的磁盘绝对路径（`openFiles[].path`），不要仅依赖 Monaco URI。  

#### BUG-IDE-04 — GetCompletions 请求未带最新 content 更新到 server

即便前端每次传 `content`，服务端 `ensureOpen` 忽略后续 content（同 IDE-01）。属 **前后端契约误用**：API 收了 Content，实现未用。  

### 2.2 Medium（通用 / 调用）

| ID | 问题 |
|---|---|
| BUG-M19 | `SetConfig` Tools 等仍 `as unknown as` — 绑定漂移时运行时才爆 |
| BUG-M20 | `gopls serve` 未传 `-remote=auto` 时多实例资源；未设 `InitializeParams` 完整 capabilities |
| BUG-M21 | Toolchain `gofmt -l` / `goimports -l` 只列表不写回；用户以为「格式化」已生效 |
| BUG-M22 | `eslint --fix .` / `prettier --write .` 整仓改写风险高，缺 dry-run / 当前文件模式 |
| BUG-M23 | Agent `resolveProjectPath` 用字符串拼路径，Windows 混用 `/` 一般可用但未 `path.join` 语义 |
| BUG-M24 | 无 root 时 File **读**仍开放（Agent 工具已拒绝）— 插件面若暴露 Read 仍可扫盘 |
| BUG-M25 | 巨型工作树未拆 PR — 安全审计与 bisect 困难 |

### 2.3 Low

| ID | 问题 |
|---|---|
| L23 | 无 Go modules 浏览器 / go.work 切换 UI |
| L24 | 无 package.json scripts 树形面板（仅 toolchain npm-scripts 弱入口） |
| L25 | vitest/jest 无测试资源管理器（点击跑单测） |
| L26 | 真多流 / Agent 审批跨窗完整 UI 仍为明确产品限制 |
| L27 | Computer Use stub — 与语言 IDE 无关，勿进默认卖点 |

---

## 3. 功能正确性（按层）

### 3.1 后端 Go 服务

| 模块 | 正确性 | 对 Go/TS/JS IDE 的含义 |
|---|---|---|
| File / pathsec / 写拒绝空 root | **优秀** | 沙箱可信 |
| AI 流 streamId / 互斥 / tools | **优秀** | AI 辅助可靠 |
| Conversation/Settings CAS | **良好** | 双窗不丢数据 |
| Agent 审批 + 无 root 拒读写工具 | **良好** | 安全边界清晰 |
| **LSPService** | **中下** | 骨架在，**同步与 TS 进程选型拖垮正确性** |
| **ToolchainService** | **中上** | 命令目录合理；**写回/粒度/多模块不足** |
| Project 脚手架 templates | **中** | 有 go/ts/js/fullstack/monorepo；需持续对齐社区最佳实践 |
| Git / Search / Terminal | **良好** | 通用 IDE 标配达标 |

### 3.2 前端 Vue/TS

| 模块 | 正确性 | 备注 |
|---|---|---|
| Monaco 编辑 / Tab / Dirty / Save | **良好** | |
| `lspCompletion.ts` | **中** | 注册了补全+悬停；**缺定义跳转等**；依赖后端过期缓冲 |
| toolchain store / 命令面板 | **良好** | 有检测与运行 |
| Agent / AI 双窗 | **良好** | prompt-5～7 债已还大部分 |
| Problems 面板 | **中** | 依赖 LSP 诊断缓存 + 工具链解析，需真机联调 |
| 测试 | **优秀** | 1232 全绿，但 **缺真实 gopls/tsserver 集成测** |

### 3.3 关键调用链正确性清单

```
[OK]  用户保存 → FileService.WriteFile → pathsec → file:saved → workflow
[OK]  AI 发送 → setConfig(UseStoredKey) → StartStream → streamId 事件 → 本窗装配
[OK]  Agent run → CheckCommand → 审批 → ExecCommand（无 shell）
[BUG] 编辑器输入 → LSP Complete(content) → ensureOpen 忽略 content 更新 → 旧语义
[RISK] TS 补全 → StartLSPServer(tsserver) → LSP initialize → 可能协议不兼容
[WEAK] Format → toolchain gofmt -l → 仅列表，不更新编辑器 buffer
[WEAK] Go to Def → 无原生 LSP Definition 绑定到 Monaco
```

---

## 4. 代码规范与合规

### 4.1 规范

**优点**  
- Go：服务拆分、pathsec 集中、错误哨兵（Conflict/Busy）、race 友好测试。  
- TS：store 分域、i18n、事件类型化趋势、ByName=0。  
- 文档：SECURITY / ARCHITECTURE / ai-windows / qa / prompts 归档。  
- CI：多平台 go test -race、eslint、vitest、govulncheck、bindings/docs 门禁。  

**不足**  
- `as unknown as` 边界仍多；Tools/部分 Settings 形状依赖运行时。  
- `main` + 大量服务注册仍重（bootstrap 已缓解）。  
- 贡献流程有 CONTRIBUTING，但缺 **语言能力 Roadmap** 与 **Good First Issues** 分层。  
- 发版：无清晰 SemVer 产物说明（安装包、校验和、SBOM）。  

### 4.2 安全合规

| 项 | 状态 |
|---|---|
| G-SEC-01～12 主体 | ✅ |
| write/run 永不 auto-approve | ✅ |
| 空 root 禁止写 / Agent 无 root 禁读写工具 | ✅ |
| 密钥不回前端 | ✅ |
| 扩展默认禁用 + SHA-256 | ✅ |
| 读路径空 root | ⚠️ 仍松（插件面） |
| 依赖 SBOM / 发布签名 | ❌ 成熟开源标配缺失 |

---

## 5. 代码质量

| 指标 | 值 |
|---|---|
| Go 测试 | 全绿 |
| Vitest | **1232** 全绿 |
| ByName | 0 |
| 工作区脏路径 | **~200+** |
| 语言集成测试（真 gopls） | **几乎无** |

**质量判断**：  
- **应用层与安全层质量高**；  
- **语言服务层测试多为「未运行返回空」**，对真实 LSP 行为覆盖不足，导致 IDE-01/02 类缺陷能进主干。  

---

## 6. 成为成熟开源「Go + TypeScript + JavaScript IDE」的改进建议

下列建议按 **语言纵深** 与 **开源产品** 双轴展开，可直接作为 0.3.x 路线图。

### 6.1 Go 语言纵深（优先级最高）

| 优先级 | 项 | 说明 |
|---|---|---|
| P0 | **修复 didChange + version** | 否则 gopls 名存实亡 |
| P0 | **Definition / References / Hover 稳定** | F12 / Shift+F12 / 悬停文档 |
| P0 | **Format：gofmt/goimports 写回当前 buffer** | Format on Save；优先 gopls `textDocument/formatting` |
| P1 | **Organize Imports / Rename** | gopls code action + rename |
| P1 | **go test 当前包 / 当前函数** | 光标所在 `TestXxx` 一键跑；Output 解析 fail 定位 |
| P1 | **go.mod / go.work 感知** | 打开多模块 monorepo；`go list` 包树 |
| P1 | **Diagnose on save + Problems** | gopls + `go vet` 诊断汇入 |
| P2 | **Debug（Delve）** | 断点 / 单步 — 成熟 IDE 分水岭 |
| P2 | **Coverage 展示** | `go test -coverprofile` 装入 gutter |
| P2 | **golangci-lint 当前包** | 非整仓默认 |
| P3 | **生成：interface stub / mock / stringer** | 右键 codegen |

**Go 工具链产品化建议**  
- 命令默认粒度：`./...` 保留，但菜单分「Workspace / Package / File」。  
- 检测 `GOROOT`/`GOPATH`/`GOTOOLCHAIN` 并在 StatusBar 显示 Go 版本。  
- 内置「安装 gopls」引导：`go install golang.org/x/tools/gopls@latest`。  

### 6.2 TypeScript / JavaScript 纵深

| 优先级 | 项 | 说明 |
|---|---|---|
| P0 | **改用 typescript-language-server 或 vtsls** | 修 IDE-02 |
| P0 | **didChange + 正确 languageId（tsx/jsx）** | `typescriptreact` / `javascriptreact` |
| P0 | **Definition / Quick Info / Completions** | 与 VS Code 同级基础体验 |
| P1 | **tsc --noEmit 当前 project references** | 支持 solution style |
| P1 | **ESLint 诊断进 Problems** | 优先 eslint flat config；当前文件 `--fix` 可选 |
| P1 | **Prettier 当前文件** | 勿默认 `prettier --write .` |
| P1 | **Vitest/Jest 测试树** | 发现 `*.test.ts`，点击运行 |
| P2 | **npm/pnpm/yarn/bun 检测** | scripts 面板；安装依赖任务 |
| P2 | **Auto-import / Organize imports** | tsserver 源动作 |
| P3 | **JSX/Vue SFC** | 若定位含前端框架，再扩 Volar（谨慎 scope） |

**TS 项目模型**  
- 读取最近 `tsconfig.json` / `jsconfig.json`；`tsserver` 的 project 根与 Wails 工作区对齐。  
- monorepo 模板已有 pnpm-workspace — 应用 `typescript.tsserver.maxTsServerMemory` 类设置位。  

### 6.3 前后端契约与调用规范（防「假成功」）

1. **LSP 请求一律「content + version」**；服务端必须把 content 同步进 server。  
2. **禁止「失败返回空切片」掩盖可恢复错误** — 区分 `server_unavailable` / `protocol_error` / `ok_empty`，前端 StatusBar 显示 LSP 状态。  
3. **Toolchain 结果驱动编辑器**：若命令修改文件，应 `reload from disk` 或直接 `updateContent`。  
4. **Wails 边界类型**：`StartStream` 返回 string 已收紧；继续为 `Tools`、`Diagnostic` 生成完整 bindings。  
5. **路径 API 单一真相**：所有语言服务只接受 workspace 内绝对路径。  

### 6.4 前端 TypeScript / 工程成熟度

| 项 | 建议 |
|---|---|
| 类型 | 消灭业务路径 `as unknown as`；`vue-tsc` 保持 CI 阻断 |
| 状态 | 编辑器 buffer 与磁盘 `revision` 对齐（避免 LSP/诊断用错版本） |
| 性能 | 大 TS 项目防抖 completion（150～300ms）；取消过期请求（Abort/序号） |
| 测试 | 增加 **LSP 协议级假 server** 集成测（stdio mock），覆盖 didOpen/didChange/completion |
| 包管理 | 前端自身锁定 Node 20+；文档写明 |
| Monaco | 语言注册 `typescriptreact`；worker 内存配置文档化 |

### 6.5 后端 Go 工程成熟度

| 项 | 建议 |
|---|---|
| 模块 | 保持 `gugacode` 单 module；避免过早拆太多 repo |
| 并发 | LSP client 已有 mutex；补 **单 flight 初始化**、请求 id 取消 |
| 进程 | gopls/tsserver **崩溃自动重启** + 退避 |
| 安全 | 工具链命令仍不经 shell；参数数组化（已部分具备） |
| 可观测 | slog 结构化：`lsp_request_ms`、`toolchain_cmd`、失败原因 |
| 测试 | 对 pathsec/CAS/stream 保持；**增加 gopls 契约测**（可选 build tag `integration`） |

### 6.6 开源 IDE 产品与社区（成熟项目标配）

1. **发版**  
   - 拆逻辑 commit → tag `v0.2.0`（双窗/安全）→ `v0.3.0`（语言服务 P0）。  
   - Release 附：Windows/macOS/Linux 产物、SHA256、`go version`、校验脚本。  

2. **定位一句话**（README 置顶）  
   > *Offline-first desktop IDE for Go and TypeScript/JavaScript, with sandboxed AI agents.*  
   避免被理解为「又一个通用 Electron 壳 + ChatGPT」。  

3. **Roadmap 公开**  
   - Now：LSP 正确性（IDE-01/02）  
   - Next：Definition/Format/Test runner  
   - Later：Delve / 覆盖率 / 多根 workspace  

4. **贡献分层**  
   - `good first issue`：i18n、toolchain 解析器、文档  
   - `area/lsp-go`、`area/lsp-ts`、`area/security` 标签  

5. **行为准则与安全**  
   - 已有 CODE_OF_CONDUCT / SECURITY — 补 **支持版本表与实际发版节奏**。  

6. **默认体验「打开 go.mod 仓库」**  
   - 自动 detect gopls → 状态栏 Go 版本 → 建议安装  
   - 打开 `package.json` 同理  

---

## 7. prompt-8 可执行任务清单（建议迭代）

### 里程碑 M1 — 语言服务「可用」（P0，2～3 周量级）

| Task | 内容 | 验收 |
|---|---|---|
| **8-A** | `ensureOpen` → `syncDocument`：didOpen/didChange/didClose + version | 改代码后补全反映新符号 |
| **8-B** | TS 改用 `typescript-language-server`/`vtsls` 检测与启动 | 真实 TS 项目补全非空 |
| **8-C** | 前端传绝对路径；统一 `pathToURI` | Windows gopls 可补全 |
| **8-D** | StatusBar：LSP 状态（available/running/error） | 用户可知为何无补全 |
| **8-E** | stdio mock LSP 集成测试 | CI 覆盖 didChange |

### 里程碑 M2 — 编辑器闭环（P1）

| Task | 内容 |
|---|---|
| **8-F** | Definition + References 绑定 Monaco（Go/TS/JS） |
| **8-G** | Format Document / Format on Save（gopls 或 gofmt 写回 buffer） |
| **8-H** | Rename symbol（gopls/ts） |
| **8-I** | Toolchain：File/Package/Workspace 三级；去掉危险默认整仓写 |
| **8-J** | Go：Run Test at Cursor；TS：Vitest 单文件 |

### 里程碑 M3 — 开源成熟（与语言并行）

| Task | 内容 |
|---|---|
| **8-K** | 工作树逻辑拆分 + tag `v0.2.0` / `v0.3.0-alpha` |
| **8-L** | README 语言能力矩阵（✅/🚧/❌）与 VS Code 对比诚实表 |
| **8-M** | SBOM + 校验和发布；`npm audit` 发版门禁 |
| **8-N** | 贡献指南：如何跑 gopls 集成测、如何加 toolchain 解析器 |

### 里程碑 M4 — 深度 IDE（P2+）

| Task | 内容 |
|---|---|
| **8-O** | Delve 调试适配器 |
| **8-P** | Coverage gutter |
| **8-Q** | go.work / pnpm workspace 多根 |
| **8-R** | 可选真多流 AI；Agent 审批 SSOT（完整） |

---

## 8. Definition of Done（「成熟开源 Go/TS/JS IDE」最小标准）

在宣称 **v1.0 语言就绪** 前，至少满足：

- [ ] **8-A～8-E** 完成；真实 `go` 项目与 `tsc` 项目手工验收补全/悬停  
- [ ] F12 跳转定义在 Go 与 TS 均可  
- [ ] Format on Save 对 `.go` / `.ts` 生效且不整仓误写  
- [ ] 当前包/当前文件测试可一键运行，失败可点回源码行  
- [ ] StatusBar 显示 Go 版本 + LSP 状态  
- [ ] README 语言矩阵与实现一致  
- [ ] CI 含 LSP mock 测 + 现有 race/vitest/bindings  
- [ ] 至少一个正式 tag 与可安装产物  
- [ ] 工作树可按模块 review（非 200+ 混杂 diff）  

---

## 9. 结论

prompt-5～7 已把 gugacode 从「危险的原型」推到了 **安全基线扎实、AI 双窗可用、测试纪律良好的桌面 AI 编辑器（~8.2）**。

要成为 **成熟的开源 Go / TypeScript / JavaScript IDE**，下一阶段必须 **换重心**：

| 停止优先 | 开始优先 |
|---|---|
| 再堆 Agent/IM/Computer Use | **LSP 正确性（didChange、TS 服务器选型）** |
| 更多 AI 模式 | **Definition / Format / Test runner** |
| 扩扩展生态 | **go.mod / package.json 一等公民 UX** |
| 功能清单膨胀 | **发版、Roadmap、诚实能力矩阵** |

**一句话路线**：  
> *先让 gopls 与 TypeScript language server 在真实仓库里「改代码后仍然正确」；再谈调试与覆盖率；AI 继续做加速器，而不是掩盖语言服务空洞。*

---

## 10. 附录：本轮验证与关键代码锚点

```
go test ./services/...  → ok
go test .               → ok
vitest                  → 1232 passed
check-bindings / docs   → ok
CI bindings/docs steps  → present in ci.yml
```

| 问题 | 代码锚点 |
|---|---|
| 无 didChange | `services/lsp_service.go` `ensureOpen` |
| tsserver 直启 | `startServerProcess` typescript 分支 |
| 补全 URI | `frontend/src/lib/lspCompletion.ts` `model.uri.path` |
| gofmt 只列表 | `toolchain_service.go` `gofmt` Args `-l` |
| 模板 | `services/templates/{go,typescript,javascript,monorepo,fullstack}` |
| 双窗/CAS | `docs/ai-windows.md`，`conversation_service.go` |

---

*本文件为 prompt-8 审查交付物，可作为 0.3.x「语言 IDE 成熟度」专项的唯一检查清单。建议与 `docs/prompts/prompt-7.md`、`ARCHITECTURE.md`、`README` 语言能力表交叉维护。*
