# 安全策略 / Security Policy

## 支持版本 / Supported Versions

| 版本 / Version | 支持 / Supported | 说明 / Notes |
|---|---|---|
| **0.5.x** | ✅ | 当前开发线；安全修复优先 / current line |
| **0.4.x** | ✅ | 标签 `v0.4.0`；高危可回移 / critical backports |
| **0.3.x** | 🟡 | 仅高危 / high severity only |
| **0.2.x** | ✅ | 当前发行线含 `v0.2.0` 安装包 / release packages |
| 更早 / earlier | ❌ | 请升级 / please upgrade |
| 1.0.x | 规划中 / planned | 正式 1.0 后按 semver 更新本表 |

### 发版周期 / Release cadence

- **Patch**（`0.x.Y`）：按需，含安全与回归修复 / as needed for security & regressions  
- **Minor**（`0.X.0`）：功能里程碑；附 CHANGELOG 与 Release 资产 / feature milestones + release assets  
- **安全响应 / Response：** 确认后 48h 内 ACK；高危目标 7 日内修复或缓解说明  

## 漏洞报告 / Reporting a Vulnerability

### 中文
1. **不要**为安全漏洞公开开 GitHub Issue。  
2. 将描述、复现步骤、影响范围发送至 **security@gugacode.dev**（或维护者私信渠道）。  
3. 我们将在 **48 小时内**确认收到。  
4. 在 **7 天内**给出调查结论与修复时间表（视严重性调整）。  

请尽量包含：漏洞描述、复现步骤、受影响组件、潜在影响、建议修复（如有）。

### English
1. **Do NOT** open a public GitHub issue for security vulnerabilities.  
2. Email **security@gugacode.dev** with description, reproduction steps, and impact.  
3. You will receive an acknowledgment within **48 hours**.  
4. We aim to provide findings and a fix timeline within **7 days** (severity-dependent).  

Include: description, steps to reproduce, affected components, potential impact, suggested fix (optional).

## CI 安全门禁 / Continuous Integration Security Gates

CI（`.github/workflows/ci.yml`）在推送/PR 到 `main` 时执行下列门禁；**以 CI 实际步骤为准**。

| 门禁 / Gate | 要求 / Requirement |
|---|---|
| Race detector (G-SEC-04) | `go test -race ./services/... .` |
| govulncheck (G-SEC-04) | 扫描 Go 依赖已知 CVE |
| Frontend type check | `npx vue-tsc --noEmit` |
| go vet / golangci-lint | 静态分析 |
| ESLint / Vitest | 前端 lint 与测试 |
| npm audit high | `npm audit --audit-level=high`（阻断） |
| wails3 build | 生产构建验证 |

跨 Ubuntu / Windows / macOS 运行，以覆盖平台相关代码。

## 安全措施摘要 / Security Measures Summary

| ID | 中文 | English |
|---|---|---|
| G-SEC-01 | AI BaseURL 校验，防 SSRF / 禁 userinfo；非回环强制 HTTPS | BaseURL validation; SSRF / credential-leak prevention |
| G-SEC-02 | Agent 命令强制人工审批，无 run 自动批准 | All agent shell commands require manual approval |
| G-SEC-03 | 项目级工作流不可信，启动类不自动执行 | Untrusted workflows never auto-run on load |
| G-SEC-04 | CI 启用 race + govulncheck | Race detector + govulncheck in CI |
| G-SEC-05 | iframe `sandbox="allow-scripts"`，无 allow-same-origin | Extension iframes without same-origin |
| G-SEC-06 | 路径双侧 EvalSymlinks，防符号链接逃逸 | Symlink-aware path sandbox |
| G-SEC-07 | API Key 加密存储且不回传前端明文 | Encrypted API keys; never returned to frontend |
| G-SEC-08 | 错误响应体限制 64KB | Error body limited to 64 KB |
| G-SEC-09 | 关键 JSON 原子写 + 0600 | Atomic JSON writes, 0600 perms |
| G-SEC-10 | CSP nonce 使用 crypto/rand，无弱回退 | CSP nonces from crypto/rand only |
| G-SEC-11 | Markdown 外链强制 noopener | Links forced `rel=noopener` |
| G-SEC-12 | 扩展 SHA-256、默认禁用、权限分级与黑名单 | VSIX hash, default-disabled, classification, blacklist |

### 路径沙箱 / Path Sandboxing
所有文件操作限制在工作区根内；`pathsec` 防止目录遍历与符号链接逃逸。终端与 Agent CWD 同样校验。

All file operations are sandboxed to the workspace root with symlink evaluation. Terminal and agent CWDs are validated similarly.

### XSS 防护 / XSS Prevention
Markdown 经 DOMPurify 清洗；Vue 模板默认转义用户输入。  
Markdown is sanitized with DOMPurify; Vue escapes other UI input by default.

### API Key
静态加密（Windows DPAPI / macOS Keychain / Linux AES 或 Secret Service），仅发往用户配置的 AI 端点；不记入日志。  
Keys are encrypted at rest and only sent to the user-configured AI provider; never logged.

### 依赖安全 / Dependency Security
CI 跑 `govulncheck`；发版前建议 `frontend/` 下 `npm audit`。  
`govulncheck` in CI; run `npm audit` before releases.

## 安全响应头 / Security Headers

Wails 资源中间件注入：
- `Content-Security-Policy`（`script-src 'nonce-...'`，无 `unsafe-inline`）
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`

桌面 WebView 场景下不额外做浏览器 CSRF/CORS 方案；外链在系统浏览器打开。

## 披露时间线 / Disclosure Timeline

| 时间 / Day | 动作 / Action |
|---|---|
| 0 | 报告 / Report received |
| 1–2 | 确认与初评 / ACK + assessment |
| 3–7 | 修复开发与测试 / Fix & test |
| 7–14 | 视严重性发补丁 / Patch release |
| 30 | 视情况公开披露 / Public disclosure if applicable |

## 联系 / Contact

- 安全邮箱 / Security: security@gugacode.dev  
- 一般问题 / General: [GitHub Issues](https://github.com/CuTeLiTTleBraids-Geek-studio/Gugacode/issues)  
