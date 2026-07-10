# 插件 API 核心子集（prompt-11 11-L）

稳定、推荐扩展使用的表面。实验性 API 可能变更，不在此列。

## 宿主能力（核心）

| 区域 | 方法 / 能力 | 说明 |
|---|---|---|
| `languages` | `registerCompletionItemProvider` | 补全 |
| | `registerHoverProvider` | 悬停 |
| | `registerDefinitionProvider` | 定义跳转 |
| | `registerReferenceProvider` | 引用 |
| | `registerDocumentFormattingEditProvider` | 格式化 |
| | `registerSignatureHelpProvider` | 签名 |
| | `registerRenameProvider` | 重命名 |
| `commands` | `registerCommand` / `executeCommand` | 命令面板 |
| `window` | 通知 / 输出通道（经宿主） | 用户反馈 |
| `workspace` | 受限文件读写（沙箱） | 需权限审批 |

## 安全

- 扩展默认禁用直至用户启用（见 `docs/extension-security.md`）。
- 禁止任意 `eval` / 远程代码加载（除已审批的 marketplace 包）。
- 与内置 Go/TS LSP 并存时，勿抢占默认 formatter（除非用户设置）。

## 非目标（本阶段）

- 完整 VS Code Debug Adapter 宿主 API
- Notebook / 完整 `workspaceFolders` 事件流
- 默认捆绑 Vue/Volar（应作为可选扩展）

## 与核心语言能力解耦（prompt-12 12-M）

| 能力 | 核心内置 | 插件路径 |
|---|---|---|
| Go/TS LSP | ✅ gopls / tsserver 包装 | 可注册额外 provider，勿默认覆盖 formatter |
| 调试 | ✅ 内嵌 DAP（Delve） | 勿再实现第二套调试器服务 |
| ESLint | 工具链 quiet lint | 可用扩展增强，但核心已 debounce |
| Vue/React 深支持 | ❌ 非默认 | **必须**插件化（Volar 等） |

核心卖点保持 **Go + TS/JS + AI 沙箱**；横向实验服务禁止进入默认路径（`architecture-boundaries.md`）。

## 贡献

新增表面前对照 `docs/architecture-boundaries.md`：优先扩展 LSP / Toolchain / Debug，而非新实验服务。
