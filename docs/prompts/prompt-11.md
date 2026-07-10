# prompt-11 — AI 实施提示词（精简版）

> 项目：**gugacode**（Go + Wails v3 后端 / Vue3+TS+Monaco 前端）  
> 基线：**v0.3.0**（`master` 干净；日用闭环 + 可信重构 + Delve headless / Go coverage 已交付）  
> 目标：做成**可主驱动日常开发的成熟开源 Go / TypeScript / JavaScript IDE**  
> 约束：优先加深语言 IDE；**禁止**再堆 IM / Computer Use / 新 AI 模式等横向实验服务（见 `docs/architecture-boundaries.md`）

---

## 角色与工作方式

你是本仓库的实现 AI。修改前后端时：

1. 保持安全基线（pathsec、Agent 审批、密钥不回前端、空 root 禁写）。  
2. 改动配套测试：`go test ./services/...`、`go test .`、`cd frontend && npx vitest run`。  
3. 更新 README 语言矩阵与 CHANGELOG，**禁止功能虚标**。  
4. 小步可审查提交；不要无关重构。

---

## 已具备（勿重复建设）

- 编辑器 / 终端 / Git / 搜索；双窗 AI；Agent 沙箱审批  
- LSP：`gopls` + `typescript-language-server`/`vtsls`；didOpen/Change/Close/Save  
- 补全 / 悬停 / 定义 / 引用 / Signature / Format on Save / F2 Rename（路径列表确认）/ Save All  
- Test at Cursor（含 `t.Run`、`it.each`/`test.each`）  
- Go：`go test -coverprofile` gutter；Delve **headless** 起进程给外部 DAP attach  
- CI：race、vitest、bindings/docs check、npm audit high；tag `v0.3.0`

---

## 不足（按点）

### 调试

1. **无进程内 DAP 客户端**：只能 `dlv --headless` 吐地址，本 IDE 无断点/单步/调用栈/变量。  
2. **Debug Test at Cursor 未与内置调试 UI 打通**。  
3. **单会话、无会话管理 UI**；再 launch 直接失败。  
4. **无 Node/JS 调试**。

### 重构与保存

5. **Rename 预览仅文件路径列表**，无 diff/hunk，误操作成本高。  
6. Rename 后依赖用户 Save All；缺失败文件汇总与回滚策略。  
7. Format on Save / write 失败体验已改善，但与 **ESLint on save 异步竞态**仍可能卡顿。

### 诊断与质量

8. **诊断偏存盘后刷新**，typing 实时性不足。  
9. **ESLint 主要靠 save 跑 `eslint-file`**，非常驻诊断流。  
10. **Coverage 路径匹配过宽**（同名文件可能串 gutter）。  
11. **TS/JS 无 coverage gutter**（仅 Go）。  
12. auto-import 依赖 LSP `additionalTextEdits`，snippet/resolve 未做透。

### 项目与测试 UX

13. **go.work / pnpm workspaces 仅轻量列表**，无真正多根切换 / 多 gopls 根。  
14. **测试资源管理器轻量**，monorepo 多 vitest 根发现弱。  
15. 缺 **`go test -json` 结构化**驱动测试树状态。

### 性能与规模

16. **全量 didChange** 大文件/大仓可能卡；无增量 sync、无系统压测基线。  
17. Hover/Definition 已有 seq；其它 LSP 请求取消策略不统一。

### 开源工程

18. **多平台 Release 产物 + SHA256 上传未常态化**（仅有文档/脚本）。  
19. 调试能力若文案写满 ✅ 易**虚标**（实际 headless）。  
20. 支持周期 / SECURITY 版本表 / 完整 NOTICE+SBOM 随发行仍弱。

---

## 需求（按点，带优先级）

### P0 — 必须做（v0.4 核心）

| ID | 需求 | 验收标准 |
|---|---|---|
| **11-A** | **内嵌 DAP 客户端 MVP**（对接现有 Delve headless 或 `dlv dap`） | 不离开 gugacode：设断点、F5 启动/附加、continue/step in/out/over、看调用栈与局部变量 |
| **11-B** | Coverage 路径规范化 | 同名不同目录文件 gutter 不串；有单测 |
| **11-C** | Rename **改动摘要预览**（文件列表 + 每文件 edit 数或短 diff） | 确认前可见将改什么；Apply 后 dirty；Save All 落盘；失败进 Output |
| **11-D** | ESLint **常驻诊断**进 Problems（debounce 或语言服务，不只 save --fix） | 编辑后无需依赖「保存修复」才能看到问题 |
| **11-E** | GitHub Release **多平台 artifact + SHA256** 自动化 | Release 页可下载可校验 |

### P1 — 语言深度

| ID | 需求 | 验收标准 |
|---|---|---|
| **11-F** | `go test -json` + 测试树状态 | 通过/失败/运行中；点击跳转 |
| **11-G** | **Debug Test at Cursor** | 当前 `TestXxx`/`t.Run` 一键进 11-A 调试会话 |
| **11-H** | Vitest/Jest coverage → 可选 gutter | 与 Go coverage UX 对齐（可配置关） |
| **11-I** | go.work / pnpm 多根切换 UX | 切换根后 LSP/工具链 cwd 正确 |
| **11-J** | typing 诊断防抖刷新 | 改错代码约 1s 内 Problems 更新（非仅 save） |

### P2 — 成熟度

| ID | 需求 |
|---|---|
| **11-K** | 大 Go monorepo / 大 TS 仓性能基线 + 回归记录 |
| **11-L** | 稳定插件 API 子集文档（核心 only） |
| **11-M** | SECURITY 支持版本表 + 发版周期说明 |
| **11-N** | 贡献者一键 dev（脚本/文档 30 分钟跑通 gopls+测试） |

### 明确不做（本阶段）

- 新 AI Agent 模式、IM、Computer Use 功能扩展  
- 默认捆绑 Vue/Volar 进核心（若做必须插件化）  
- 为刷清单而虚标「完整调试器」（未完成 11-A 前 README 保持 headless 诚实描述）

---

## 实现锚点（改这些优先）

| 领域 | 路径 |
|---|---|
| Delve 进程 | `services/debug_service.go`、`frontend/src/stores/debug.ts` |
| Coverage | `services/coverage_service.go`、`frontend/src/stores/coverage.ts` |
| Rename / FoS | `frontend/src/lib/lspCompletion.ts`、`frontend/src/stores/editor.ts` |
| LSP | `services/lsp_service.go`、`frontend/src/stores/lsp.ts` |
| 测试/工具链 | `services/toolchain_service.go`、test explorer store |
| 矩阵/发版 | `README.md`、`docs/release-v0.3.0.md`、`.github/workflows/*` |
| 边界 | `docs/architecture-boundaries.md` |

---

## 完成定义（本 prompt）

- [ ] **11-A** 可用：纯 gugacode 完成一次 Go 断点调试  
- [ ] **11-B～E** 完成且测试/文档同步  
- [ ] README 调试/覆盖率分级标注与实现一致（无虚标）  
- [ ] `go test ./services/...`、`go test .`、`npx vitest run`、bindings/docs check 全绿  
- [ ] CHANGELOG 增加 prompt-11 / Unreleased 或 v0.4.0-alpha 条目  

**成功标准一句话：**  
用户打开 `go.mod` 仓库，能在 **本窗口内** 写代码、格式化、重构、跑测、看覆盖率，并 **F5 断点调试**；TS/JS 诊断常驻且 monorepo 不瞎；Release 资产可下载可校验。
