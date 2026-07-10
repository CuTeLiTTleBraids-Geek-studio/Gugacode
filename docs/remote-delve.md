# 远程 / 容器 Delve（prompt-12 12-K）

gugacode 内嵌 DAP 客户端默认启动本机 `dlv dap`。远程调试可按下列探测步骤操作。

## 探测本机 Delve

```bash
dlv version
# 或
go install github.com/go-delve/delve/cmd/dlv@latest
```

IDE Status：`DebugService.IsAvailable()` / 状态栏 Delve 提示。

## 容器内 headless

```bash
# 容器内
dlv dap --listen=:2345 --headless --api-version=2 --accept-multiclient
# 或
dlv debug ./cmd/app --headless --listen=:2345 --api-version=2
```

宿主机端口转发后，未来版本将提供 **Attach** 到 `host:port`（当前可用 `ConnectMockDAP` 测试路径作为协议基础）。

## 注意

- 仅绑定 `127.0.0.1` 时需 sidecar 或 port-forward。  
- 交叉编译目标 GOOS/GOARCH 须与容器一致。  
- 条件断点与 watch 依赖适配器能力；远程 dlv 版本建议与本机一致。
