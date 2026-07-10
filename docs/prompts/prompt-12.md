# prompt-12 — AI 实施提示词（精简版）

> 项目：**gugacode**（Go + Wails v3 / Vue3+TS+Monaco）  
> 基线：**prompt-11 已合入**（`a5b01eb` 一带）；当前公开 tag 多为 **`v0.3.0`**，prompt-11 功能在 **Unreleased**  
> 目标：把 Go 调试从 **MVP** 打磨成 **可信**；治大仓 lint 性能；**发 v0.4.0**；规划 TS 调试与规模化  
> 约束：只加深 **Go / TypeScript / JavaScript IDE**；禁止再堆 IM / Computer Use / 新 AI 模式（`docs/architecture-boundaries.md`）

---

## 角色与工作方式

你是本仓库实现 AI。必须：

1. 保持安全基线（pathsec、Agent 审批、密钥不回前端、空 root 禁写）。  
2. 配套测试：`go test ./services/...`、`go test .`、`cd frontend && npx vitest run`。  
3. 更新 README 语言矩阵 + CHANGELOG；**禁止虚标**（未完成勿写满 ✅）。  
4. 小步提交；无无关重构。

---

## 已具备（勿重复建设）

- 编辑 / 终端 / Git / 搜索；双窗 AI；Agent 沙箱  
- LSP：gopls + typescript-language-server/vtsls；didOpen/Change/Close/Save  
- 补全/定义/引用/Signature/FoS/F2 摘要预览 Rename/Save All  
- Test@Cursor（`t.Run`、`test.each`）；`go test -json` 测试树  
- **内嵌 DAP**：`dlv dap` + 断点/F5/单步/栈/locals；Debug Test@Cursor（Go）  
- Coverage：Go coverprofile + 路径规范化；可选 vitest/lcov gutter  
- live ESLint debounce + typing 诊断防抖  
- Release SHA256 流水线；dev-setup；perf-baseline 模板；plugin-api-core；SECURITY 支持表  

---

## 不足（按点）

### 调试（Go DAP）

1. **未 verified 断点**无明确 UI（空心/警告）；用户以为断了实际未绑定。  
2. **无 Restart**；Stop 偏 Kill 进程，缺「断开保留 / 重启」。  
3. **无条件断点 / logpoint**。  
4. **无 watch / evaluate**；仅有 locals。  
5. **单会话**；无多配置会话管理。  
6. **无 launch 配置持久化**（args/env/build flags 等价 `launch.json`）。  
7. DAP **集成契约测不足**（仅有 framing 级单测，缺 mock 全流程）。  

### 调试（TS/JS）

8. **无 Node/tsx/Chrome 调试**；与 Go F5 能力不对称。  

### Lint / 诊断 / 性能

9. **live ESLint 可能反复起 `eslint-file` 进程**，大 monorepo CPU/磁盘尖刺。  
10. 诊断/ESLint **与 save 路径仍可能竞态**。  
11. **全量 didChange** 仍在；无增量 TextDocument sync。  
12. **`perf-baseline.md` 无强制实测数据**，性能回归无门禁。  

### 多根 / 测试 / 覆盖率

13. 多根 **只切 cwd**，不保证 **多 gopls/tsserver 实例/项目根**正确。  
14. 测试树 × Coverage × Debug **入口未统一**。  
15. lcov 与 go cover **双轨路径规则**文档/实现需再对齐。  

### 重构 UX

16. Rename 预览 **短 hunk 可能截断误导**；无完整 multi-diff。  

### 开源工程

17. 功能在 **Unreleased**，**未打 `v0.4.0`** → 装 v0.3.0 的用户与文档脱节。  
18. SBOM 是否 **强制随 Release 上传**未写死。  
19. 服务/store 仍膨胀风险，boundaries 需执行而非仅文档。  

---

## 需求（按点 + 验收）

### P0（v0.4 必做）

| ID | 需求 | 验收 |
|---|---|---|
| **12-A** | DAP：未 verified 断点可视化 + **Restart** + 停止原因展示 | 断点状态可信；可 Restart 当前配置 |
| **12-B** | DAP：**条件断点** + 简单 **watch/evaluate** | 不装外部 IDE 完成常见调试 |
| **12-C** | ESLint 常驻性能：长驻进程或 ESLint LSP，避免每键 `eslint-file` | 大 TS 仓输入不持续 100% CPU |
| **12-D** | 发布 **`v0.4.0`** + release notes（主题：内嵌 DAP 深化） | tag 与 CHANGELOG/矩阵一致 |
| **12-E** | DAP **集成契约测**（mock adapter：initialize→launch→stopped→stack→continue） | CI 防回归 |

### P1

| ID | 需求 | 验收 |
|---|---|---|
| **12-F** | **Node/TS debug MVP**（可后置独立里程碑，但须排期） | 至少能 launch 当前 TS/JS 入口或测试并断点 |
| **12-G** | launch 配置持久化（env/args/cwd/mode） | 可保存/选择配置再 F5 |
| **12-H** | multi-root：切换 active root 时 **LSP/工具链/测试根**正确 | 换根后补全/测试不指错项目 |
| **12-I** | 增量 didChange **或**更强节流 + 写入 perf-baseline **至少 1 组实测** | P1–P3 场景有数字 |
| **12-J** | 测试树 × Coverage × Debug **统一入口** | 同一用例可 Run / Coverage / Debug |

### P2

| ID | 需求 |
|---|---|
| **12-K** | 远程/容器 delve 文档与探测 |
| **12-L** | 可选 InlayHints（gopls/ts） |
| **12-M** | 插件与核心语言能力解耦说明落地 |
| **12-N** | v1.0 支持策略 / LTS 讨论稿 |

### 本阶段明确不做

- 新 AI Agent 模式、IM、Computer Use 扩功能  
- 默认塞 Vue/Volar 进核心  
- 未完成 12-A/B 前把调试宣传成「完整 GoLand 级」  

---

## 实现锚点

| 领域 | 路径 |
|---|---|
| DAP | `services/debug_service.go`、`frontend/src/stores/debug.ts`、Debug 面板组件 |
| ESLint/诊断 | `CodeEditor.vue` debounce、`toolchain`/`lsp` stores |
| Coverage | `services/coverage_service.go`、`frontend/src/stores/coverage.ts` |
| 多根/测试 | workspace store、test explorer、`toolchain_service.go` |
| Rename | `frontend/src/lib/lspCompletion.ts` |
| 发版 | `.github/workflows/release.yml`、`docs/release-*.md`、CHANGELOG、README 矩阵 |
| 边界 | `docs/architecture-boundaries.md`、`docs/perf-baseline.md` |

---

## 完成定义

- [ ] **12-A、12-B、12-C、12-E** 完成且有测试  
- [ ] **12-D** 存在 tag **`v0.4.0`** 与 release notes  
- [ ] README 调试/ESLint/多根描述与实现一致  
- [ ] `go test ./services/...`、`go test .`、`npx vitest run`、bindings/docs check 全绿  
- [ ] CHANGELOG 记录 prompt-12 / v0.4.0  

**成功标准一句话：**  
用户在 gugacode 内对 Go **可靠断点调试**（含条件断点与 watch），大 TS 仓 **live lint 不卡死**，版本号 **v0.4.0** 与功能对齐；TS 调试有明确交付或写进公开 Roadmap。
