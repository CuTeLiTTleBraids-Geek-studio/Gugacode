# 审查活文档归档（prompt-7 Task A）

| 文件 | 说明 |
|---|---|
| [prompt-5.md](./prompt-5.md) | 第一轮审计摘要（原全文可能缺失时的重建） |
| [prompt-6.md](./prompt-6.md) | 第二轮：双窗 SSOT、streamId、Anthropic tools 等 |
| [prompt-7.md](./prompt-7.md) | 第三轮：CAS、CI 门禁、Agent 审批策略、发版卫生 |
| [prompt-8.md](./prompt-8.md) | 第四轮：Go/TS/JS 语言 IDE 成熟度（LSP didChange、tsserver 选型等） |
| [prompt-9.md](./prompt-9.md) | 第五轮：Format on Save、Rename、Test at Cursor、发版 |
| [prompt-10.md](./prompt-10.md) | 第六轮：可信重构、Delve/Coverage MVP、正式发行 |

交叉引用：`CHANGELOG.md`、`docs/ai-windows.md`、`docs/qa-dual-window.md`、`ARCHITECTURE.md`、`README` 语言能力矩阵。

## 发版建议

1. 按主题拆 commit：`security` / `dual-window` / `agent-tools` / `docs` / `ci`  
2. 确认未跟踪 `*.exe`、`gugacode-server*`、`node_modules/`、`frontend/dist/`  
3. tag：`v0.2.0`（prompt-6 功能集）→ `v0.2.1`（prompt-7 CAS/CI）  
