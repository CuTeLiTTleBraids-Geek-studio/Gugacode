# v0.3.0 发版说明（prompt-10）

## 主题

**语言 IDE 日用闭环 + 可信重构 + Delve/Coverage MVP**

## 校验

```bash
go test ./services/...
go test .
cd frontend && npx vitest run && npm run lint
node scripts/check-bindings.mjs
node scripts/check-doc-numbers.mjs
```

## 产物建议

| 平台 | 构建 | 校验 |
|---|---|---|
| Windows | `wails3 build` / `task windows:package` | `Get-FileHash -Algorithm SHA256` |
| macOS / Linux | 同仓库 Taskfile | `shasum -a 256` |

可选 SBOM：

```bash
# 脚本封装（需 syft 或 docker）
bash scripts/generate-sbom.sh sbom.spdx.json
# 或：
syft dir:. -o spdx-json > sbom.spdx.json
```

## 审计策略

- CI `npm audit --audit-level=high`：**阻断**（frontend-test + 独立 npm-audit job）  
- 可选 `lsp-integration` job：workflow_dispatch / schedule，真 gopls + tsserver PATH  

## Tag

- `v0.3.0-alpha` — prompt-9 日用闭环  
- `v0.3.0` — prompt-10 可信重构 + Coverage MVP  
- 下一标签：`v0.4.0-alpha` — prompt-11 内嵌 DAP + 诊断/覆盖率深化（见 CHANGELOG Unreleased）

## GitHub Release 自动化（prompt-11 11-E）

推送 `v*.*.*` tag 触发 `.github/workflows/release.yml`：

1. 矩阵构建 Windows / Linux / macOS(amd64+arm64)  
2. 每产物生成 `.sha256`，汇总 `SHA256SUMS`  
3. `softprops/action-gh-release`（或等价步骤）挂到 Release 页  

签名：有证书 secrets 则签；缺失时默认仍发布**未签名**产物 + 校验和（设 `REQUIRE_CODE_SIGN=true` 可强制失败）。

## 安装后 10 分钟

1. 打开含 `go.mod` 的仓库 → StatusBar 显示 Go 版本与 LSP  
2. 编辑后补全应反映新符号；Ctrl+S 触发 Format on Save  
3. F2 Rename 预览多文件 → Apply → Save All（Ctrl+K S）  
4. 在 `TestXxx` / `t.Run` 内 Ctrl+Shift+T 跑测试  
5. 命令面板「Coverage」/「Debug Package」（Delve headless 地址）  
6. Problems 点击跳转到行；JS/TS 存盘触发 eslint-file 
