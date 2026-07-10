# 服务与 Store 边界（prompt-10 10-P）

避免实验服务继续横向膨胀。新增能力前先对照本表。

## 后端（`services/`）— 核心 vs 实验

| 类别 | 服务 | 约定 |
|---|---|---|
| **核心 IDE** | File, Project, Settings, Window, Terminal, Git, Search, LSP, Toolchain, Editor 路径相关 | 默认启用；变更需测试 |
| **AI** | AI, Conversation, Agent, Preset, Rules | 安全门禁（审批、密钥、沙箱）不可放松 |
| **双窗** | Window AI APIs + 事件 | 见 `docs/ai-windows.md` |
| **调试/质量** | Debug (Delve headless), Coverage | MVP；完整 DAP/UI 迭代 |
| **实验** | Computer Use, IM, Marketplace 深度 | 默认 Experimental UI；勿进主路径卖点 |
| **避免** | 再新增完整「模式」服务 | 优先扩展 Toolchain/LSP/Debug |

## 前端（`stores/`）

| 类别 | Store | 约定 |
|---|---|---|
| **编辑核心** | editor, app, lsp, toolchain, output | 保存/诊断/工具链闭环 |
| **AI** | ai, agent, … | 与 editor 解耦；事件 SSOT |
| **调试** | debug, coverage, testExplorer | 可演示 MVP |
| **实验** | computerUse, im, … | 设置「实验」分组 |

## 贡献检查

1. 新服务是否可用现有 LSP/Toolchain 表达？  
2. 是否有单测 / mock 契约测？  
3. README 矩阵是否需要更新？  
