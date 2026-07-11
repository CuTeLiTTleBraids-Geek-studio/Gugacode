# 文档 / Documentation

本目录存放项目说明文档（不占用仓库根目录）。  
Project docs live here so the repository root stays focused on source code.

| 文件 / File | 说明 / Description |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | 架构概览 / Architecture overview |
| [CHANGELOG.md](CHANGELOG.md) | 版本变更记录 / Release changelog |

## 社区文档 / Community docs

GitHub 会自动识别 `.github/` 下的社区文件：  
GitHub auto-detects community files under `.github/`:

| 文件 / File | 用途 / Purpose |
|---|---|
| [../.github/CONTRIBUTING.md](../.github/CONTRIBUTING.md) | 贡献指南 / Contributing |
| [../.github/CODE_OF_CONDUCT.md](../.github/CODE_OF_CONDUCT.md) | 行为准则 / Code of Conduct |
| [../.github/SECURITY.md](../.github/SECURITY.md) | 安全策略 / Security policy |

根目录仅保留：`README.md`、`LICENSE`、以及构建所需入口（`main.go`、`go.mod` 等）。  
Root keeps only `README.md`, `LICENSE`, and build entrypoints (`main.go`, `go.mod`, …).
