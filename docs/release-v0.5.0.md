# v0.5.0 发版说明（prompt-13）

## 主题

**TS/JS 与 Go 共用 Debug 面板（Node CDP）+ ESLint 长驻（eslint_d）+ 远程 Delve 半自动**

## 校验

```bash
go test ./services/...
go test .
cd frontend && npx vitest run
node scripts/check-bindings.mjs
node scripts/check-doc-numbers.mjs
```

## 亮点

1. Node `--inspect-brk` 后连接 **CDP WebSocket**，断点/栈/Continue 进入同一 Debug 面板  
2. `EslintService` + **eslint_d**（无则 CLI 单飞）  
3. 条件/watch 错误可见（lastError）  
4. 远程 Delve **Probe+Attach**  
5. Incremental didChange（服务器 Kind=2）  
6. Launch 配置 JSON 导入/导出  

## 诚实边界

- Node CDP 非完整 js-debug / Chrome DevTools 全功能  
- 远程 attach 依赖 dlv 监听地址可达；非自动 port-forward  
- eslint_d 需本机安装：`npm i -g eslint_d`  

## Tag

- `v0.4.0` — 可信 Go DAP  
- **`v0.5.0`** — Node 同面板 + eslint 长驻 + 远程探测  
