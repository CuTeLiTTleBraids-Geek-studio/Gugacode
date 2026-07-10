# v0.4.0 发版说明（prompt-12）

## 主题

**可信 Go 内嵌调试（条件断点 / watch / Restart）+ 大仓 live ESLint 不卡死 + 版本对齐**

## 校验

```bash
go test ./services/...
go test .
cd frontend && npx vitest run
node scripts/check-bindings.mjs
node scripts/check-doc-numbers.mjs
```

## 用户可见亮点

1. Debug 面板：Restart、停止原因、未 verified 断点警告  
2. 条件断点 + Watch / Evaluate  
3. F5 / F9；Debug Test at Cursor（Go）  
4. ESLint 输入 2s 防抖 + 内容 hash 跳过重复进程  
5. 多根切换后 LSP 重启  
6. Node 当前文件 `inspect-brk` MVP  
7. 测试发现统一 Run / Coverage / Debug 入口  

## 资产

推送 tag `v0.4.0` → GitHub Actions `release.yml`：

- Windows / Linux / macOS amd64+arm64 产物  
- 每文件 `.sha256` + 汇总 `SHA256SUMS`  
- 可选 SBOM（`scripts/generate-sbom.sh` 工作流步骤若启用）  

## Tag

- `v0.3.0` — 日用闭环 + Coverage MVP  
- **`v0.4.0`** — 可信 DAP 深化 + live lint + Node inspect MVP  

## 诚实边界

- 非 GoLand 级完整调试器（无多会话 UI、无完整远程 attach UI）  
- Node 调试为 inspect-brk MVP，非完整 js-debug  
- 增量 TextDocument sync 仍以 full + hash/节流为主  
