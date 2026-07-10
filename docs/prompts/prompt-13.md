# prompt-13 — AI 实施提示词（精简 · 按点）

> 项目：**gugacode**（Go + Wails v3 / Vue3+TS+Monaco）  
> 基线：**v0.4.0**（内嵌 Go DAP 含条件断点/watch/Restart；ESLint 2s+hash+单飞；Release SHA256；测试全绿）  
> 目标：冲击 **v1.0 级成熟开源 Go/TS/JS IDE**  
> 约束：只加深语言 IDE；**禁止** IM / Computer Use / 新 AI 模式（`docs/architecture-boundaries.md`）

---

## 角色

1. 保持安全基线（pathsec、Agent 审批、密钥不回前端、空 root 禁写）。  
2. 测试：`go test ./services/...`、`go test .`、`cd frontend && npx vitest run`。  
3. 更新 README 矩阵 + CHANGELOG；**禁止虚标**。  
4. 小步提交；无无关重构。

---

## 已具备（勿重建）

- 编辑/终端/Git/搜索；双窗 AI；Agent 沙箱  
- LSP：gopls + vtsls/ts-lang-server；同步完整  
- FoS / F2 摘要 Rename / Save All / Test@Cursor / go test -json 测试树  
- **Go 内嵌 DAP**：断点（条件/logpoint）、verified UI、F5/Restart、单步、栈、locals、watch/evaluate  
- Debug Test@Cursor（Go）；Coverage（Go + 可选 lcov）  
- ESLint：debounce + content-hash + single-flight  
- 多根切换 cwd+LSP 重启；测试统一 Run/Coverage/Debug  
- Node **`inspect-brk` 浅 MVP**（非完整 js-debug）  
- DAP 契约测；v0.4.0 + release 诚实边界  

---

## 不足（按点）

### 调试

1. Node/TS 调试与 Go **不同面板体验**（多为 inspect 地址级，非同一 DAP UI）。  
2. **单会话**；无并行多 debug session。  
3. **远程/容器 delve** 仅文档，无一键探测/端口转发 UX。  
4. 条件断点/watch **错误表达式**反馈可能不足。  
5. Restart/Stop 长跑稳定性、泄漏、重连未系统压测。  
6. 无 attach pid / core；无多线程精细 UI。  

### Lint / 语言服务 / 性能

7. ESLint 仍是 **CLI 单飞**，非长驻 language server；大仓冷启动尖刺。  
8. 仍以 **full didChange + hash** 为主，无真正增量 TextDocument sync。  
9. multi-root 切换会 **重启 LSP**，补全可能闪断数秒。  
10. InlayHints 仅 optional hook，未产品化默认体验。  
11. perf-baseline 有少量记录，**公开大仓实测仍少**。  

### 测试 / 覆盖 / 入口

12. TS debug 未与测试树深度打通。  
13. Coverage 双轨（go cover / lcov）规则需持续对齐。  

### 开源 / 1.0

14. SBOM 随 Release **可选**，未强制。  
15. v1.0 支持承诺仍是讨论稿，未完全写入执行流程。  
16. 服务/store 膨胀风险仍在，boundaries 需执行。  

---

## 需求（按点）

### P0（v0.5 核心）

| ID | 需求 | 验收 |
|---|---|---|
| **13-A** | Node/TS 调试 **并入同一 Debug 面板**（断点/停住/栈至少一项真可用） | 非仅 toast inspect 地址 |
| **13-B** | ESLint **长驻**（eslint_d 或 ESLint LSP），替代每触发起 CLI 进程 | 大 TS 仓输入无周期性进程风暴 |
| **13-C** | DAP 条件断点/watch **错误可见** + 契约测扩展 | 坏表达式 → UI 或 Output 明确错误 |
| **13-D** | 矩阵/发版说明与实现一致；需要时 **v0.4.1** 文档热修 | 无虚标 |

### P1

| ID | 需求 | 验收 |
|---|---|---|
| **13-E** | 远程/容器 delve **半自动**（文档+探测/端口提示或按钮） | 按文档 5 分钟可挂上 |
| **13-F** | 增量 didChange 或协议协商 full/incremental | 大文件不持续满核 |
| **13-G** | multi-root：按根 **隔离/指向正确 language server** | 换根后补全/测试不指错包 |
| **13-H** | launch 配置导入/导出（团队共享） | JSON 可复制 |
| **13-I** | 测试树 × Coverage × Debug 更深联动（TS 含 debug 配置模板） | 同一用例三入口 |

### P2（v1.0 准备）

| ID | 需求 |
|---|---|
| **13-J** | DAP 长跑：泄漏、重连、异常退出恢复 |
| **13-K** | 公开 ≥3 组 monorepo perf 实测写入 `docs/perf-baseline.md` |
| **13-L** | 插件 API 冻结候选 + 兼容策略 |
| **13-M** | v1.0 RC：支持周期写入 SECURITY 并执行 |

### 本阶段不做

- 新 AI 模式 / IM / Computer Use 扩容  
- 默认捆绑 Volar/Vue 进核心  
- 未完成 13-A 前把 Node 调试写成「完整 Chrome DevTools」  

---

## 实现锚点

| 领域 | 路径 |
|---|---|
| DAP/Node | `services/debug_service.go`、`frontend/src/stores/debug.ts`、Debug 面板 |
| ESLint | `CodeEditor.vue` debounce、toolchain/lsp stores |
| 多根/测试 | workspace / test explorer / `toolchain_service.go` |
| Coverage | `coverage_service.go`、`stores/coverage.ts` |
| 文档发版 | README 矩阵、`docs/release-*.md`、CHANGELOG、SECURITY、`perf-baseline.md` |
| 边界 | `docs/architecture-boundaries.md` |

---

## 完成定义

- [ ] **13-A、13-B、13-C** 完成且有测试  
- [ ] README 对 Node/ESLint 描述与实现一致  
- [ ] `go test ./services/...`、`go test .`、`npx vitest run`、bindings/docs 全绿  
- [ ] CHANGELOG 记 prompt-13；视完成度打 **v0.5.0** 或保持 Unreleased 但矩阵诚实  

**成功标准一句话：**  
Go 调试继续可信；**TS/JS 能在同一 Debug 面板里断住**；**大仓 ESLint 长驻不卡**；为 v1.0 备好稳定与支持证据，而不是再加横向功能。
