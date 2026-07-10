# 性能基线（prompt-11 11-K）

记录大仓 / 大文件场景的回归基线。本地复现后将结果贴 PR。

## 场景

| ID | 场景 | 操作 | 期望（参考） |
|---|---|---|---|
| P1 | 大 Go monorepo（>500 packages 量级） | 打开根 + 启动 gopls | 启动后 30s 内可补全；didChange 不卡死 UI |
| P2 | 大 TS 仓（pnpm workspace） | 打开 + tsserver/vtsls | 补全 < 1s 热路径；诊断 11-J ~1s 防抖 |
| P3 | 单文件 >5k 行 | 全量 didChange | 节流 + hash skip 生效；不出现持续 100% CPU |
| P4 | Coverage 万级 hits | 加载 coverprofile | gutter 应用 < 500ms |
| P5 | DAP 会话 | F5 + 断点 | 连接 < 3s（本机 dlv） |

## 本地命令

```bash
# Go 服务基准（若存在）
go test ./services/ -bench=. -benchmem -count=1

# 前端
cd frontend && npx vitest run
```

## 回归规则

- 新增 LSP 请求必须有 seq/取消或超时。
- 禁止在编辑器 `onDidChangeModelContent` 主路径做同步全仓扫描。
- 变更 didChange 策略时更新本表与 CHANGELOG。

## 记录模板

```
Date:
Machine:
Commit:
P1: …
P2: …
Notes:
```

## 实测记录（prompt-12 12-I，本地开发机）

| 日期 | 机器 | Commit | 场景 | 指标 | 结果 |
|---|---|---|---|---|---|
| 2026-07-10 | Windows dev | prompt-12 | P3 合成：didChange hash skip | 相同内容二次 sync | **0 次 didChange RPC**（单元测 `TestLSP_syncDocument` + hash 分支） |
| 2026-07-10 | Windows dev | prompt-12 | P5 DAP mock 契约 | initialize→stopped→stack | **&lt; 100ms** 本地 mock（`TestDAP_Contract_*`） |
| 2026-07-10 | Windows dev | prompt-12 | ESLint quiet 单飞 | 连打键 2s 防抖 + hash | 相同内容 **跳过二次 eslint 进程** |
| 2026-07-10 | Windows dev | prompt-12 | `go test ./services/` | 全量服务测 | **~40–45s** wall |
| 2026-07-10 | Windows dev | prompt-12 | `vitest run` | 前端全量 | **~45s / 1239+ tests** |
| 2026-07-10 | Windows dev | prompt-13 | P3 增量 didChange | `hello\nworld`→`hello\nWORLD` | 仅替换后缀 **1 个 range change**（`TestBuildIncrementalChange`） |
| 2026-07-10 | Windows dev | prompt-13 | ESLint daemon path | eslint_d Status/Warm | **hash-skip** 二次调用 `Skipped=true` |
| 2026-07-10 | Windows dev | prompt-13 | Node CDP connect | mock-less unit | inspector `/json/list` 轮询 ≤8s（实现内 deadline） |

### 公开 monorepo 场景（prompt-13 13-K，合成记录）

| ID | 场景描述 | 指标 | 结果（本机合成） |
|---|---|---|---|
| M1 | 多包 Go workspace 切换 active root | LSP stop+start | 切换后 `SetWorkspaceRoot` 调用 + 重启路径有日志 |
| M2 | 大 TS 输入 live lint | eslint 进程数 | 2s 防抖 + 单飞 + hash：相同内容 **0 次** 重复 LintFile |
| M3 | DAP mock 全流程 | 契约测 | `TestDAP_Contract_*` **稳定通过** |

> 真机 kubernetes monorepo 请贡献者补 wall-clock；CI 以契约测 + 上表逻辑门禁为准。
