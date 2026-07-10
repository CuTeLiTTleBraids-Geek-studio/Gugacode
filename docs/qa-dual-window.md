# 双窗手工验收清单（prompt-6 Task 12 + prompt-7 增补）

主编辑器窗口 + AI 伴侣窗口（`/#/ai-window`）并排时的关键路径。无需自动化即可回归；另见 vitest `dual-window-smoke.test.ts` 的 3 步协议冒烟。

## 前置

1. 启动 gugacode，打开任意项目（保证 File 写有 root）。
2. 配置至少一个可用 AI Provider（模型下拉可见）。
3. 通过菜单/快捷键打开 **AI 独立窗口**（`WindowService.OpenAIWindow`）。

## 步骤

| # | 操作 | 期望 |
|---|---|---|
| 1 | 主窗 Settings → 切换 AI 模型或 BaseURL 并保存 | AI 窗模型下拉/显示与主窗一致（`settings:changed` + version 同步） |
| 2 | 主窗 AI 面板发送一条消息并完成 | AI 窗发送按钮在流期间禁用（`globalStreamBusy`）；结束后两窗均可再发 |
| 3 | AI 窗发送一条消息 | 主窗发送禁用；互斥，不串字 |
| 4 | 主窗流式中途点 Stop | 流停止；busy 由后端 `ai:stream-busy=false` 清除；两窗可再发 |
| 5 | 主窗保存会话后查看 AI 窗历史列表 | 新会话出现在列表（`conversation:saved`） |
| 6 | AI 窗打开同一会话 ID，主窗再发并保存 | AI 窗在空闲时应 reload；流中 peer 保存会标 stale，结束后 CAS/分叉不静默丢数据 |
| 7 | 主窗选中代码 → Ctrl/Cmd+Shift+A | AI 窗出现 selection chip；Apply 带 `filePath` |
| 8 | AI 窗代码块 Apply | 主窗打开 Diff 确认；取消不写盘；确认后内容更新 |
| 9 | Agent 模式（OpenAI tools） | 原生 `ai:tool_calls` 或 fence 出现审批卡；write/run 不可 auto-approve |
| 10 | 无项目时尝试写文件 / Agent write/read | FileService 拒绝空 root 写；Agent read/search 无项目时报错 |
| **11** | 两窗几乎同时改不同设置字段并保存 | 后写可能触发 version conflict；失败方 reload 提示，不静默丢盘上版本 |
| **12** | 主窗 Agent 发出 pending 工具后切到 AI 窗 | AI 窗 toast 提示「审批在另一窗」；完整审批卡仍在发起窗（prompt-7 D1） |

## 可选（Anthropic）

| # | 操作 | 期望 |
|---|---|---|
| A1 | Provider 协议设为 Anthropic，Agent 模式 | 流式 `tool_use` 解析为与 OpenAI 同形的 `ai:tool_calls` |

## 产品声明

- **Agent 审批仅在发起窗可操作**（prompt-7 Task D1）。跨窗仅摘要 + 强提示。
- **默认仍单流互斥**；streamId 协议已就位，真多流为后续可选（Task I）。
- IM / Computer Use 在设置「实验性功能」分组，默认不推荐。

## 记录

- 日期：
- 构建/提交：
- 结果：通过 / 失败项：
