<div align="center">

<img src="icon.png" alt="gugacode" width="128" height="128">

# 咕咕嘎嘎code

**一款由 AI 驱动的轻量级跨平台代码编辑器**

*单文件分发 · 开箱即用* —— 主人专属的编码伙伴咕嘎~

基于 Go（Wails v3）+ Vue 3 + Monaco Editor 构建

集成 AI 助手 · 自治 Agent · 内置终端 · Git 面板 · 全文搜索

---

![License](https://img.shields.io/badge/license-MIT-blue.svg?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&style=flat-square)
![Vue](https://img.shields.io/badge/Vue-3-4FC08D?logo=vue.js&style=flat-square)
![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&style=flat-square)
![Wails](https://img.shields.io/badge/Wails-v3-red?style=flat-square)
![Monaco](https://img.shields.io/badge/Editor-Monaco-646CFF?style=flat-square)
![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-lightgrey?style=flat-square)

**[功能一览](#功能一览)** · **[下载](#下载)** · **[构建](#从源码构建)** · **[AI 配置](#ai-配置)** · **[联系方式](#联系方式)**

</div>

---

## 功能一览

### 代码编辑器

| 特性 | 说明 |
|---|---|
| Monaco 内核 | 与 VS Code 同款编辑器，支持 20+ 语言语法高亮（Go、TypeScript、Python、Rust、Java、C/C++、JSON、YAML、Markdown 等）咕咕嘎嘎~ |
| 多标签页 | 脏状态跟踪（● 指示器）、未保存提示、Ctrl+S 全局保存，主人不会丢失工作成果咕嘎 |
| 查找替换 | Ctrl+F 调用 Monaco 原生查找面板 |
| Markdown 预览 | 分栏渲染，代码块语法高亮（highlight.js 50+ 语言，80+ 别名映射） |
| 内联 AI 补全 | 幽灵文本代码补全，基于当前文件上下文，像有只小企鹅在旁边帮主人写代码咕嘎 |
| 快速打开 | Ctrl+P 模糊搜索工作区文件 |
| 文件切换动画 | 标签页切换时编辑器淡入脉冲，不重新挂载 Monaco，丝滑得像企鹅跳跃 |

### AI 助手

| 特性 | 说明 |
|---|---|
| 多 Provider 配置 | CC Switch 风格，无限保存多套 AI 配置，一键切换，想用哪家就用哪家咕嘎 |
| 双协议原生支持 | OpenAI（`/v1/chat/completions`）与 Anthropic（`/v1/messages`），两种协议本企鹅都懂咕嘎 |
| SSE 流式响应 | 实时打字机效果，事件驱动（`ai:chunk` / `ai:done` / `ai:error`），文字像企鹅踩键盘一样一个个蹦出来咕嘎 |
| 9 个右键代码操作 | 解释代码 · 重构 · 修复 Bug · 生成文档 · 生成测试 · 优化 · 代码审查 · 安全审计 · 提交信息 |
| 代码上下文注入 | 选中代码自动随提示词发送 |
| 对话历史 | 保存、加载、删除、重命名历史会话，主人的每段回忆本企鹅都好好保存 |
| 自定义系统提示词 | 全局默认 + 每会话独立覆盖 |
| 项目规则 | 自动加载 `.cursorrules` / `AGENTS.md` 追加到系统提示词 |
| Markdown 渲染 | XSS 防护（DOMPurify）+ 语法高亮（VS Code Dark+/Light+ 配色） |
| 温度 / 最大 Token | 用户可配置，真实透传至后端 |

### 自治 Agent

| 特性 | 说明 |
|---|---|
| 工具调用 | 读文件、写文件、运行命令、搜索代码，本企鹅是主人的全能小助手咕嘎 |
| 命令沙箱 | 工作目录限制、命令黑名单、审计日志，不会让主人的电脑出乱子咕嘎 |
| 风险分级 | Safe / Elevated / Dangerous，逐工具审批策略，危险操作本企鹅会先问主人咕嘎 |
| 审批循环 | Pending → Approved/Rejected → Executed，支持拒绝后继续对话 |
| 观测反馈 | 工具执行结果回灌给 AI 形成多轮自治 |

### 内置终端

| 特性 | 说明 |
|---|---|
| 完整 PTY 支持 | Windows ConPTY / Unix pty，真终端不是假的咕咕嘎嘎~ |
| 多标签终端 | 创建、切换、关闭多个会话 |
| ANSI 颜色渲染 | xterm.js，花花绿绿的本企鹅喜欢咕嘎 |
| 可配置 Shell | PowerShell、bash、zsh |
| 弹出动画 | 高度 + 透明度过渡，像企鹅从盒子里探头一样咕嘎 |

### Git 集成

- 分支显示与 ahead/behind 跟踪
- 文件状态列表（已修改、已暂存、未跟踪）
- 单文件暂存 / 取消暂存
- 提交（带消息）
- **AI 代码审查** — 分析未提交变更，逐文件输出结构化审查意见，本企鹅帮主人挑 Bug 咕咕嘎

### 全文搜索

- 工作区文件内容搜索
- 大小写敏感切换
- 替换与全部替换
- 结果导航与文件预览

### 命令面板

> Ctrl+Shift+P 模糊搜索命令列表

10+ 内置命令：保存 · 切换 AI 面板 · 切换终端 · 清空对话 · 切换侧栏 · 切换缩略图 · 切换内联补全 · 切换活动栏 · 切换状态栏 · 移动 AI 面板

### 个性化

| 特性 | 说明 |
|---|---|
| 三套设计语言 | Material You · Apple HIG · Claude 风格，主人喜欢哪种就换哪种咕咕嘎嘎~ |
| 8 种强调色 | Blue · Teal · Green · Amber · Pink · Purple · Cyan · Indigo |
| 明暗模式 | Dark / Light / System 跟随系统 |
| 国际化 | English · 简体中文 · 日本語 |
| 设置配置文件 | 多配置文件切换、导入、导出 |
| 布局引擎 | 拖拽分屏、持久化布局配置文件 |
| 丝滑过渡动画 | 侧边栏 Tab 切换、设置页导航、编辑器文件切换、终端弹出，尊重 `prefers-reduced-motion`，每个动画都像企鹅伸懒腰一样流畅咕咕嘎嘎~ |

### 安全

| 特性 | 说明 |
|---|---|
| API Key 加密存储 | Windows DPAPI / macOS Keychain / Linux Secret Service，主人的钥匙本企鹅锁得严严实实咕咕嘎嘎~ |
| 路径遍历防护 | ConversationService、PresetService、FileService 共享 pathsec 校验 |
| 工作区沙箱 | GitService、SearchService、AgentService 限制在工作区根目录内，本企鹅不会乱跑咕咕嘎嘎~ |
| CSP 头注入 | 通过 AssetOptions.Middleware 设置 |
| URL 校验 | ListModels 的 BaseURL 经过 SSRF 与 API Key 外泄检测 |
| 符号链接转义防护 | EvalSymlinks 校验 |

### 无障碍

- 所有可点击元素支持键盘导航
- 命令面板与快速打开的焦点陷阱
- 终端输出的 `aria-live` 区域
- 本地化的 ARIA 标签
- 所有动画尊重 `prefers-reduced-motion`

---

## 下载

主人请前往 [Releases](https://github.com/CuTeLiTTleBraids-Geek-studio/Gugacode/releases) 下载对应平台压缩包咕咕嘎嘎~

| 平台 | 文件 | 备注 |
|---|---|---|
| Windows x64 | `gugacode-<version>-windows-amd64.zip` | 需 WebView2（Win10/11 通常已内置） |
| Linux x64 | `gugacode-<version>-linux-amd64.tar.gz` | 需 WebKit2GTK |
| macOS x64 | `gugacode-<version>-darwin-amd64.zip` | Intel 芯片 |
| macOS ARM64 | `gugacode-<version>-darwin-arm64.zip` | Apple Silicon（M1/M2/M3） |

每个发布包均附带 `.sha256` 校验文件，建议下载后校验完整性咕咕嘎嘎~

<details>
<summary><b>Linux 依赖</b></summary>

```bash
# Debian/Ubuntu
sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev libgcc-12-dev libstdc++-12-dev pkg-config

# Fedora
sudo dnf install -y gtk3 webkit2gtk4.1 pkgconf-pkg-config

# Arch
sudo pacman -S gtk3 webkit2gtk pkgconf
```

</details>

<details>
<summary><b>macOS 注意事项</b></summary>

首次打开可能提示"无法验证开发者"。右键点击应用 → 选择"打开" → 在弹窗中确认"打开"即可咕咕嘎嘎~ 或执行：

```bash
xattr -cr /path/to/gugacode
```

</details>

---

## 从源码构建

### 环境要求

| 工具 | 最低版本 | 说明 |
|---|---|---|
| **Go** | 1.25 | 后端语言 |
| **Node.js** | 20 | 前端构建 |
| **Wails3 CLI** | v3.0.0-alpha2.111+ | 桌面框架命令行 |
| **Git** | 任意 | 版本控制 |

### 开发模式（热重载）

```bash
# 安装 Wails3 CLI
go install github.com/wailsapp/wails/v3/cmd/wails3@latest

# 克隆仓库
git clone https://github.com/CuTeLiTTleBraids-Geek-studio/Gugacode.git
cd gugacode

# 安装前端依赖
cd frontend && npm install && cd ..

# 启动开发模式（前端热重载 + 后端自动重编译）
wails3 dev -config ./build/config.yml -port 9245
```

<details>
<summary><b>未安装 Wails3 CLI 的替代方案</b></summary>

可分两个终端手动启动咕咕嘎嘎~

```bash
# 终端 1 — 前端
cd frontend && npm run dev

# 终端 2 — 后端
go run .
```

</details>

### 生产构建

| 平台 | 命令 | 产物 |
|---|---|---|
| Windows | `wails3 build -tags production` | `bin/gugacode.exe`（约 32 MB） |
| Linux | `wails3 build -tags production` | `bin/gugacode` |
| macOS | `wails3 build -tags production` | `bin/gugacode.app` |

<details>
<summary><b>跨平台构建说明</b></summary>

**无法从 Windows 主机交叉编译 Linux/macOS 二进制** —— 这是 Wails v3 alpha 阶段的已知限制咕咕嘎嘎，不是本企鹅的锅咕咕嘎嘎~

1. **构建约束 Bug** — `pkg/application` 中的 Linux/macOS 平台文件引用了被构建标签排除的 `pointer` 类型，交叉编译时报 `undefined: pointer`
2. **Taskfile 使用 Unix 命令** — `mkdir -p`、`uname` 等在 Windows PowerShell 上不可用
3. **CGO 依赖** — Linux 的 WebKit2GTK 绑定需要目标平台的 C 工具链

**正确的跨平台构建方式：**

| 方式 | 说明 |
|---|---|
| **GitHub Actions**（推荐） | 推送 `v*.*.*` 标签，`.github/workflows/release.yml` 自动在三大平台构建并发布 |
| **原生构建** | 在对应平台的机器上执行 `wails3 build -tags production` |
| **Docker 交叉编译** | `wails3 task setup:docker` 构建 `wails-cross` 镜像后构建（需 Docker） |

</details>

---

## AI 配置

gugacode 支持任何 OpenAI 兼容 API 与 Anthropic 原生 API。在 **设置 → AI** 中配置咕咕嘎嘎~

### 多 Provider 配置管理

采用 CC Switch 风格咕咕嘎嘎：

- **无限制配置数量** — 保存任意多套 Provider 配置
- **一键切换** — 聊天面板头部下拉框 + 设置页面，两处入口均可切换
- **每套配置独立** — apiKey / baseUrl / model / temperature / maxTokens / systemPrompt / protocol
- **旧配置自动迁移** — 首次加载时将旧的单配置打包为"Default"配置，主人不用担心旧设置丢失咕咕嘎嘎~

### 兼容 Provider

| Provider | Protocol | Base URL |
|---|---|---|
| **OpenAI** | `openai` | `https://api.openai.com` |
| **Anthropic Claude** | `anthropic` | `https://api.anthropic.com` |
| **Azure OpenAI** | `openai` | `https://<resource>.openai.azure.com` |
| **Ollama**（本地） | `openai` | `http://localhost:11434` |
| **LM Studio**（本地） | `openai` | `http://localhost:1234` |
| **DeepSeek** | `openai` | `https://api.deepseek.com` |
| **任意 OpenAI 兼容端点** | `openai` | 自定义 |

### 协议差异

| 协议 | 端点 | 认证头 | System 消息 | SSE 事件 |
|---|---|---|---|---|
| **OpenAI** | `/v1/chat/completions` | `Authorization: Bearer <key>` | `messages` 数组内 | `data: {...}` / `[DONE]` |
| **Anthropic** | `/v1/messages` | `x-api-key: <key>` + `anthropic-version` | 顶层 `system` 字段 | `content_block_delta` / `message_stop` |

### 内置 AI 操作

编辑器中右键即可访问咕咕嘎嘎~

| 操作 | 说明 |
|---|---|
| 解释代码 | 概述代码功能、关键逻辑与潜在问题 |
| 重构 | 在保持行为的前提下提升可读性 |
| 修复 Bug | 识别并修复逻辑错误、空指针、竞态等 |
| 生成文档 | 添加文档注释（godoc、JSDoc 等） |
| 生成测试 | 创建覆盖边界条件的单元测试 |
| 优化 | 性能优化建议 |
| 代码审查 | 结构化审查意见 |
| 安全审计 | 安全漏洞扫描 |
| 提交信息 | 根据 diff 生成 Conventional Commits 消息 |

---

## 项目结构

<details>
<summary><b>查看完整目录结构</b></summary>

```
gugacode/
├── main.go                          # Go 入口：服务注册、事件绑定、资源嵌入
├── go.mod                           # 模块名：gugacode
├── services/                        # Go 后端服务（20+ 服务）
│   ├── ai_service.go                # AI 对话（OpenAI + Anthropic 双协议）
│   ├── ai_prompts.go                # 系统提示词 + 10 个内置预设操作
│   ├── ai_retry.go                  # 瞬时错误重试与退避
│   ├── ai_urlsec.go                 # ListModels URL 安全校验
│   ├── agent_service.go             # 自治 Agent（命令沙箱）
│   ├── conversation_service.go      # 对话历史持久化
│   ├── file_service.go              # 文件读写（路径沙箱）
│   ├── git_service.go               # Git 操作（go-git）
│   ├── search_service.go            # 全文搜索
│   ├── terminal_service.go          # PTY 终端
│   ├── pty_windows.go               # Windows ConPTY 实现
│   ├── pty_unix.go                  # Linux/macOS pty 实现
│   ├── settings_service.go          # XDG 设置持久化 + 多 Provider 配置
│   ├── project_service.go           # 工作区/项目管理
│   ├── window_service.go            # 窗口控制
│   ├── task_service.go              # 构建/测试/运行任务
│   ├── workflow_service.go          # 多步骤工作流编排
│   ├── rules_service.go             # .cursorrules/AGENTS.md 规则加载
│   ├── preset_service.go            # AI 提示词预设（用户 + 项目级）
│   ├── profile_service.go           # 设置配置文件
│   ├── layout_service.go            # 布局配置文件持久化
│   ├── plugin_service.go            # 插件发现 + 资源服务
│   ├── pathsec.go                   # 共享路径遍历校验
│   ├── myers_diff.go                # Myers diff 算法
│   ├── token_estimator.go           # Token 计数估算
│   ├── secrets.go                   # API Key 存储（dispatcher）
│   ├── secrets_aes.go               # AES-256-GCM 共享加密
│   ├── secrets_windows.go           # Windows DPAPI
│   ├── secrets_darwin.go            # macOS Keychain
│   ├── secrets_linux.go             # Linux Secret Service
│   ├── shell_windows.go             # Windows Shell
│   ├── shell_unix.go                # Unix Shell
│   ├── logging.go                   # 结构化日志
│   └── *_test.go                    # Go 单元测试
├── frontend/
│   ├── src/
│   │   ├── api/services.ts          # Wails 服务绑定（类型安全）
│   │   ├── stores/                  # Vue 响应式状态（17 个 store）
│   │   ├── components/
│   │   │   ├── editor/              # CodeEditor、DiffView、TabBar
│   │   │   ├── explorer/            # FileTree
│   │   │   ├── layout/              # MainLayout、AiChatPanel、TerminalPanel 等
│   │   │   └── settings/            # 10 个设置分区组件
│   │   ├── composables/             # useKeyboard、useDragResize
│   │   ├── lib/                     # markdown、i18n、Monaco 主题、插件系统
│   │   ├── types/index.ts           # TypeScript 类型定义
│   │   └── views/                   # 5 个路由视图
│   ├── bindings/                    # Wails 自动生成的绑定
│   └── package.json
├── build/                           # 平台构建配置（windows/linux/darwin）
├── .github/workflows/               # CI + Release 流水线
└── docs/                            # 设计文档与实现计划
```

</details>

### 架构概览

<table>
<tr>
<td width="50%" valign="top">

**后端**（Go）

- 20+ 服务通过 Wails v3 FNV-1a 哈希绑定 ID 暴露给前端
- 服务间通过依赖注入解耦
- 平台特定代码通过构建标签分离

</td>
<td width="50%" valign="top">

**前端**（Vue 3）

- 17 个模块级单例 store（非 Pinia）
- AI 流式响应通过 Wails 事件系统驱动（`app.Event.On`）
- 避免 IPC 回调限制

</td>
</tr>
</table>

---

## 技术栈

| 层级 | 技术 |
|---|---|
| **后端** | Go 1.25 · Wails v3 (alpha2.111) |
| **前端** | Vue 3 · TypeScript 5 · Vite 8 |
| **编辑器** | Monaco Editor 0.55 |
| **UI 组件** | Element Plus 2.14 |
| **样式** | Tailwind CSS v4 · CSS 自定义属性 |
| **终端** | xterm.js 6 · ConPTY (Windows) · creack/pty (Unix) |
| **Git** | go-git v5.19 |
| **AI** | OpenAI 兼容 API + Anthropic 原生 API（SSE 流式） |
| **Markdown** | marked · highlight.js · DOMPurify |
| **测试** | Go testing · Vitest 4 · vue-tsc |
| **CI/CD** | GitHub Actions（三平台矩阵构建） |

---

## 测试

```bash
# Go 后端测试
go test ./services/... -v

# 前端测试（842+ 测试）
cd frontend && npx vitest run

# TypeScript 类型检查
cd frontend && npx vue-tsc --noEmit

# Go 竞态检测
go test ./services/... -race
```

<details>
<summary><b>测试覆盖范围</b></summary>

| 模块 | 覆盖内容 |
|---|---|
| AI 服务 | 双协议、流式、SSE 解析、重试、URL 安全、并发 |
| Agent | 命令执行、黑名单、沙箱、风险分级 |
| 终端 | 多会话、PTY、resize、并发 |
| 文件 | 路径遍历防护 |
| Git | 状态、暂存、提交、分支 |
| 搜索 | 正则匹配、替换 |
| 设置 | 持久化、多配置、迁移 |
| 插件 | 沙箱、注册表、命令执行 |

</details>

---

## 贡献

欢迎 Issue 与 PR 咕咕嘎嘎~ (*^▽^*)

- 提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/)
- Go：`gofmt` + `golangci-lint`（配置见 `.golangci.yml`）
- TypeScript/Vue：ESLint（配置见 `frontend/eslint.config.js`）
- 安全漏洞：请按 [SECURITY.md](SECURITY.md) 流程私下报告，勿公开 Issue

<details>
<summary><b>贡献流程</b></summary>

1. Fork 本仓库
2. 创建特性分支：`git checkout -b feature/amazing-feature`
3. 提交更改：`git commit -m 'feat: add amazing feature'`
4. 推送分支：`git push origin feature/amazing-feature`
5. 提交 Pull Request

</details>

---

## 联系方式

如果主人觉得这个项目对你有帮助，欢迎通过以下渠道联系本企鹅咕咕嘎嘎~ 

<table>
<tr>
<td width="25%" align="center">

**QQ 群**

`603299757`

</td>
<td width="25%" align="center">

**Telegram**

[https://t.me/nknkmiao]

</td>
<td width="25%" align="center">

**个人 QQ**

`3870374387`

</td>
<td width="25%" align="center">

**邮箱**

[dianasoylu423@gmail.com]

</td>
</tr>
</table>

> 加群请注明"gugacode 用户"，方便本企鹅认出主人咕咕嘎嘎~

---

<div align="center">

## 许可证

[MIT](LICENSE) · Copyright (c) 2026 gugacode contributors

---

<sub>构建于以下开源项目之上咕咕嘎嘎~</sub>

<sub>

[Wails](https://wails.io/) · [Monaco Editor](https://microsoft.github.io/monaco-editor/) · [Element Plus](https://element-plus.org/) · [go-git](https://github.com/go-git/go-git) · [xterm.js](https://xtermjs.org/) · [highlight.js](https://highlightjs.org/) · [marked](https://marked.js.org/) · [DOMPurify](https://github.com/cure53/DOMPurify)

</sub>

---

<sub>Made with love by gugacode contributors · Gugu Gaga~</sub>

</div>
