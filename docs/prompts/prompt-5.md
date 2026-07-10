# prompt-5.md — 归档摘要（prompt-7 Task A / BUG-L18）

> **说明**：原 `prompt-5.md` 全文在部分工作区副本中缺失。本文件为 **归档摘要**，依据 CHANGELOG Unreleased（prompt-5 条目）与代码注释重建，供审查基线与发版引用。完整复审请见 `docs/prompts/prompt-6.md` / `prompt-7.md`。

## 角色

第一轮全栈审计 + P0–P3 修复清单（Task A–J）。

## 已闭环（代码 / CHANGELOG 证据）

| 项 | 摘要 |
|---|---|
| BUG-H2 Apply 假成功 | Diff 确认 + 路径强制 + 可选 Snapshot |
| BUG-H1 双窗流冲突 | `ErrStreamBusy` + `ai:stream-busy` |
| BUG-M1 Ctrl+Shift+A | Monaco keybinding |
| BUG-L6 启动开 AI 窗 | `openAIWindowOnStartup` 默认 false |
| Task D 测试 | vitest 全绿 |
| BUG-M4 write auto-approve | write 永不 auto |
| BUG-M6 OpenPath* | 绝对路径 + Stat |
| Task F 文档 | MAX_TOOL_CALLS 对齐、`docs/ai-windows.md` |
| Task G Computer Use | 实验/stub 文案 |
| Task H 原生 tools | OpenAI tools + fence 双轨 |
| Task I 工程化 | bootstrap、check 脚本 |
| Task J 质量 | FileTree 虚拟、MAX_AI_MESSAGES、e2e-smoke |

## 转入后续

- 空 root 写拒绝 → prompt-6 Task 4  
- 双窗 SSOT / streamId / Anthropic tool_use → prompt-6  
- 发版卫生 → prompt-6/7  

## 相关

- `docs/prompts/prompt-6.md` — 第二轮复审与双窗深化  
- `docs/prompts/prompt-7.md` — 第三轮：CAS、CI、审批、发版  
