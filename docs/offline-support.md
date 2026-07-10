# Offline Support (G-OFF-01 / G-OFF-02)

Gugacode is designed to work fully offline. The only feature that requires
network connectivity is the **AI online chat / inline completion** (which calls
a remote model API). Everything else — editing, LSP, completion, Git (local
repos), search, terminal, build, test — runs against local processes and the
local filesystem, with no network dependency.

This document describes what works offline, how offline detection works, what
the user sees when offline, and how to install the local toolchain
dependencies.

## Features that work offline

| Feature | Offline? | How it works without network |
| --- | --- | --- |
| File editing | ✅ | Pure local filesystem I/O (`FileService`). |
| Code completion (LSP) | ✅ | `gopls` / `tsserver` run as **local stdio processes** managed by `LSPService`. Completion, hover, and diagnostics are served over stdin/stdout JSON-RPC — no network. |
| Go to definition / hover | ✅ | Same LSP process; local. |
| Git (local repos) | ✅ | Backed by the `go-git` library (`GitService`). `Status`, `Commit`, `Branch`, `Log`, `Diff` operate on the local `.git` directory. No `git fetch` / `git pull` / `git push` is performed by the IDE. |
| Search | ✅ | `SearchService` walks the local filesystem and matches regexps in memory. |
| Terminal | ✅ | A local PTY / shell process (`TerminalService`). Run any local command. |
| Build (go build / tsc / make) | ✅ | `ToolchainService` spawns local toolchain binaries (`go`, `tsc`, `make`, …) resolved via `exec.LookPath`. |
| Test (go test / vitest) | ✅ | Same as build — local processes. |
| Lint (golangci-lint / eslint) | ✅ | Local linter processes. |
| Settings persistence | ✅ | `SettingsService` reads/writes a local JSON file. API keys are encrypted at rest locally (DPAPI / Keychain / AES). |
| Project / layout state | ✅ | Local files. |
| **AI chat / inline completion** | ❌ | Calls a remote model API (`AIService`). Disabled when offline — see below. |

> The guarantee (G-OFF-01): **all features work without internet except AI
> online calls.**

## How offline detection works

Offline state is tracked in the frontend (`frontend/src/lib/connectivity.ts`)
using two complementary signals:

1. **`navigator.onLine` + window `online`/`offline` events** (primary,
   instant). The browser/Wails webview fires these when the host network
   interface goes up/down. `connectivityState.online` follows
   `navigator.onLine` as the authority.
2. **Periodic heartbeat to the AI BaseURL** (best-effort reachability check).
   Every 30s, a `HEAD` request (`mode: "no-cors"`, 5s timeout) is sent to the
   configured AI BaseURL. If it resolves (opaque response), the AI server is
   marked reachable. If it fails (network error, CSP block, or timeout),
   `connectivityState.aiReachable` flips to `false`. This **never throws** —
   it only updates state. A single failed heartbeat does **not** mark the app
   offline (the `online` flag stays tied to `navigator.onLine` so a slow AI
   server doesn't produce a false "offline" badge).

The listener is initialized once during app bootstrap (after settings load, so
the AI BaseURL is known) via `initConnectivityListener()` in `main.ts`. It is
idempotent and safe to call multiple times.

## What happens when offline

When `connectivityState.online` is `false`:

- **Status bar badge** — `StatusBar.vue` shows an "Offline Completion"
  (`statusBar.offlineBadge`) badge, warning-tinted, with an accessible aria
  label and a tooltip explaining that AI is unavailable but LSP offline
  completion is active.
- **AI send button disabled** — `AiChatPanel.vue` binds the send button's
  `:disabled` to `!connectivityState.online` and shows the
  `aiChat.sendDisabledOffline` ("Network offline — AI chat unavailable")
  tooltip. The user cannot trigger an AI request that would fail.
- **LSP completion continues** — `LSPService` and the Monaco completion
  provider are independent of the connectivity state. `gopls` / `tsserver`
  keep serving completions, hover, and diagnostics from their local
  processes.
- **No unhandled errors** — failed network requests (e.g. a model-list
  refresh) are caught by the calling store and surfaced as a notification or
  silently swallowed, never as an unhandled promise rejection.

When the network returns, the `online` window event triggers
`refreshOnlineState()`, the badge clears, and the AI send button is
re-enabled.

## Single-binary distribution (G-OFF-02)

Gugacode ships as a **Wails single binary**. The Go/TS/JS toolchain
detection script is included in the package at
`build/scripts/deps/install_deps.go` and is runnable standalone:

```sh
go run build/scripts/deps/install_deps.go
```

On first run, the script detects the local toolchain via `exec.LookPath` and
prints a clear report. It checks:

- **Critical** (required for core offline operation): `go`, `node`, `git`.
- **Recommended** (enable richer language features): `gopls`,
  `tsserver` / `typescript-language-server`, `golangci-lint`, `eslint`.

### Exit codes

- `0` — all critical tools are installed (recommended tools may still warn).
- `1` — one or more critical tools are missing. The script prints an
  installation hint for each missing tool.

The script itself works fully offline — every check is a local `LookPath`,
which matches the G-OFF-01 guarantee.

## Installing toolchain dependencies

Run the detection script first to see exactly what's missing:

```sh
go run build/scripts/deps/install_deps.go
```

Then install the tools it reports as missing.

### Go toolchain (critical)

Download from <https://go.dev/dl/> (1.21+ recommended) and add `go` to your
`PATH`. Verify with `go version`.

### Node.js (critical)

Download the LTS from <https://nodejs.org/> (or use `nvm` / `fnm`). Verify
with `node --version`.

### Git (critical)

Download from <https://git-scm.com/downloads>. Verify with `git --version`.

### gopls — Go language server (recommended)

```sh
go install golang.org/x/tools/gopls@latest
```

### tsserver — TypeScript language server (recommended)

Install TypeScript locally in your project so `node_modules/.bin/tsserver`
resolves (the `LSPService` looks there first):

```sh
npm i -D typescript
```

Or install the LSP-compatible wrapper globally:

```sh
npm i -g typescript-language-server
```

### golangci-lint — Go linter (recommended)

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### eslint — JS/TS linter (recommended)

```sh
npm i -g eslint
```

## Graceful degradation

When a recommended tool is missing, the IDE degrades gracefully rather than
erroring:

- **No `gopls` / `tsserver`** → `LSPService.DetectLSPServers()` reports
  `Available=false`; `GetCompletions` / `GetHover` return empty results (not
  errors). The editor falls back to basic syntax highlighting.
- **No `golangci-lint` / `eslint`** → `ToolchainService.RunToolchainCommand`
  returns `NotInstalled=true` with the install hint; the frontend shows an
  install-command notification instead of a generic error.
- **No `go` / `node`** → build/test commands report `NotInstalled`; editing,
  search, and Git (local) still work.

This graceful-fallback behavior is verified by the offline integration tests
in `services/offline_test.go` (run with
`go test ./services/... -run Offline -v`).
