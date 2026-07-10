# Extension Security Model

This document describes the security model for VS Code-style extensions in gugacode (requirements G-VSC-02, G-VSC-03, G-SEC-12). It is the companion to the `Security Policy` section in [`../SECURITY.md`](../SECURITY.md) and reflects what the code in `services/extension_security_service.go`, `services/extension_blacklist.go`, `frontend/src/lib/extensionHost/`, and `frontend/src/components/layout/PluginViewIframe.vue` actually enforces.

The VS Code extension host is a **separate code path** from the native plugin system in `services/plugin_service.go`. Native plugins use a smaller permission set (`fs.read` / `fs.write` / `shell.exec` / `net` / `ai.send`) and run in Web Workers. This document covers only the VS Code-style extension system.

## 1. Security Levels

Every installed extension is classified into one of three tiers based on the permissions it declares in its `package.json` (`gugacode.permissions`, or derived from VS Code's `contributes`/activation events). Classification is performed by `ExtensionSecurityService.ClassifyExtension` (backend) and `classifyExtension` (frontend, `frontend/src/lib/extensionHost/permissions.ts`); both implementations agree.

| Level | Permissions allowed | Default state | Enable gate |
|---|---|---|---|
| **Trusted** | `fs.read`, `clipboard`, `ui.notifications`, `ui.webview` (or no permissions) | Disabled + pending review | Informational popup; may activate after review. |
| **Reviewed** | Adds `fs.write` or `shell.execute` | Disabled + pending review | Permission popup listing requested APIs before enable. |
| **Restricted** | Adds `network` (or any combination including a Restricted-tier permission) | Disabled + pending review | Hard gate: enabling requires **explicit user approval** (`ErrRestrictedRequiresApproval`). |

Classification takes the **max risk rank** across declared permissions — a single `network` declaration classifies the whole extension as `Restricted`, even if it also declares `fs.read`. Unknown permissions fail safe-ish: they don't elevate the level on their own, but the runtime gates check the exact permission string (not the tier), so they grant no privileged operation.

Newly installed extensions **always** start as `Enabled=false, PendingReview=true` (G-SEC-12). The pending-review flag is cleared on the first explicit enable.

## 2. Permission Types

The finite set of capabilities an extension may declare (mirrors `ExtensionPermission` in `permissions.ts` and `services/extension_security_service.go`):

| Permission | Risk rank | Tier | What it grants |
|---|---|---|---|
| `fs.read` | 0 | Trusted | Read files inside the workspace via `workspace.fs.readFile` / `readdir`. |
| `clipboard` | 0 | Trusted | Read/write the system clipboard. |
| `ui.notifications` | 0 | Trusted | `window.showInformationMessage` / `showWarningMessage` / `showErrorMessage`. |
| `ui.webview` | 0 (gated to Reviewed at the API level) | Trusted tier, but `createWebviewPanel` requires Reviewed | Create a sandboxed webview panel. |
| `fs.write` | 2 | Reviewed | `workspace.fs.writeFile`, `deleteFile`, `createDirectory`, `workspace.applyEdit`. |
| `shell.execute` | 2 | Reviewed (standalone); Restricted when combined with `network` | Execute shell commands. |
| `network` | 3 | Restricted | Make network requests. |

Permissions are registered per extension in a module-level registry (`registerExtensionPermissions`) on activation and removed on deactivation. The `vscode` API shim queries `hasPermission` before dispatching any privileged operation. **Fail-closed**: an unknown extension (no permission record) gets `false` for every `hasPermission` lookup.

## 3. API Surface Restriction

The `vscode`-compatible API object handed to an extension's `activate()` is built by `createVscodeAPI` (`frontend/src/lib/extensionHost/vscodeApi.ts`). The compatibility layer translates VS Code API calls into gugacode's permission-gated surface.

### Deny-by-default
Only the methods listed in `API_SURFACE` (`frontend/src/lib/extensionHost/apiSurface.ts`) are exposed. Any method not in the map is rejected with `"API method ... is not exposed to extensions."` — there is no implicit exposure.

### Level + permission gate
An extension at level L may call method M iff:
1. `LEVEL_RANK[L] >= LEVEL_RANK[API[M].minLevel]`, **and**
2. the extension declared `API[M].permission` (when non-null).

### Dangerous commands — always require confirmation
Regardless of the calling extension's security level, these VS Code built-in commands always trigger a confirmation prompt (G-SEC-12 req. 4):
- `workbench.action.terminal.sendSequence`
- `workbench.action.files.save`
- `_workbench.*` (prefix match — all internal workbench commands)

`shell.execute`, `network.request`, and `child_process.exec` also carry `requiresConfirmation: true` in the API surface.

### Exposed namespaces per level
Lower levels are subsets of higher levels:

| Level | Namespaces exposed |
|---|---|
| Trusted | `commands`, `languages`, `window` (show*Message + register*Provider only), `workspace` (readFile / readdir only) |
| Reviewed | adds `workspace` write APIs (`writeFile`, `applyEdit`, `createDirectory`, `deleteFile`) and `window.createWebviewPanel` |
| Restricted | adds `shell` (execute, with confirmation) and `network` (request, with per-request approval) |

Absent namespaces are `undefined` on the API object, so accessing them throws naturally.

## 4. Sandbox Boundaries

### No direct `window.go` access
Extensions run in an **isolated context** and never receive `appState` or `window.go` bindings directly (G-SEC-12 req. 5). The `vscode` API object is the only bridge to the host. All privileged operations go through the host bridge, which enforces permission checks and disposable tracking. The factory (`createVscodeAPI`) is pure: it does not touch module-level state, so each extension gets an isolated API object.

### `appState.aiApiKey` is unreachable
The decrypted AI API key is never sent to the frontend (G-SEC-07 — see [`../SECURITY.md`](../SECURITY.md) §"API key encrypted, not returned to frontend" and [`privacy.md`](privacy.md)). Even if an extension could reach `appState`, the `aiApiKey` field is empty there; only `aiApiKeyConfigured` (bool) and `aiApiKeyStorageMethod` (string) are present. There is no API in the extension surface that exposes the plaintext key.

### Extension webviews: `sandbox="allow-scripts"` only
Extension webviews (`window.createWebviewPanel`) and plugin views (`PluginViewIframe.vue`) render in an `<iframe sandbox="allow-scripts">`:
- **No `allow-same-origin`** → the iframe gets an opaque origin. It cannot access the parent's DOM, `localStorage`, cookies, or `window.go` bindings. It also cannot remove its own `sandbox` attribute (that requires same-origin).
- No `allow-forms`, `allow-top-navigation`, `allow-popups`, `allow-pointer-lock`, `allow-modals`.
- Every `postMessage` from the iframe is validated by **`event.source` equality** with the iframe's `contentWindow` — not by `event.origin`, because a sandboxed iframe without `allow-same-origin` emits `origin: "null"`, which any document can spoof.
- Every RPC from the iframe is permission-gated by the same `METHOD_PERMISSIONS` map as the Worker sandbox, so the security model is consistent across the two plugin/extension view code paths.
- Communication with the host is exclusively via the `postMessage` RPC bridge (`nknk:rpc-request` / `nknk:rpc-response` / `nknk:init` / `nknk:ready`).

### Dangerous commands require confirmation
As listed in §3, shell/network API methods and the dangerous-command list always surface a confirmation dialog before execution — even for Trusted extensions. This is enforced at the API surface layer, not just at the UI layer.

## 5. Signature Verification (SHA-256)

`ExtensionSecurityService.VerifyExtensionSignature` verifies a downloaded VSIX file against an expected SHA-256 hash published by the marketplace (or supplied out-of-band for self-hosted extensions).

- The expected hash must be **non-empty** — verification requires a hash to compare against.
- The computed hash (`crypto/sha256`, hex-encoded) is compared case-insensitively after trimming.
- On mismatch, `ErrSignatureMismatch` is returned and the install is rejected.
- An extension that has not passed signature verification **cannot be enabled** — `SetExtensionEnabled` returns `ErrNotVerified`.
- When the marketplace signature is what produced the expected hash, a matching SHA-256 sets `Verified=true`. A future implementation can layer an additional detached-signature check at the same call site without changing callers.

The computed SHA-256 is recorded in `ExtensionSecurityInfo.SHA256` for later audit.

## 6. Default Disabled + Pending Review

G-SEC-12 requirement 2: newly installed extensions are stored as `Enabled=false, PendingReview=true`. The first enable attempt surfaces a popup listing the requested API permissions. The frontend store (`frontend/src/stores/extensionSecurity.ts`) is responsible for showing the dialog before calling `SetExtensionEnabled`. For `Restricted` extensions the backend hard-blocks enable without `explicitApproval=true`; for `Reviewed` extensions the popup is informational and the backend does not hard-block (the frontend is expected to show the dialog first).

## 7. Malicious Extension Blacklist

`services/extension_blacklist.go` maintains a known-malicious extension ID set (G-VSC-03 req. 3, G-SEC-12 req. 3). IDs are lowercase `<publisher>.<name>`.

### Built-in default entries
These real-world malicious extensions are blocked permanently and **cannot be removed at runtime** (the entry is re-added on next start):

| ID | Reason |
|---|---|
| `anabarban.anabarban` | Exfiltrated environment variables and SSH keys. |
| `esbenp.prettier-vscode-stolen` | Typosquat / stolen repack of `esbenp.prettier-vscode` shipping malicious code. |
| `marinhobrandao.node-exec-stolen` | Stolen-repack malicious extension. |
| `markcoder.azure-pipeline-stolen` | Stolen-repack malicious extension. |

### User-extensible
Users can add entries via `ExtensionSecurityService.AddToBlacklist`, which persists to `<configDir>/gugacode/extension-blacklist.json`. User-added entries can be removed via `RemoveFromBlacklist`; built-in entries cannot.

### Enforcement
- `CanInstall(publisher, name)` — pre-install gate. Returns `ErrBlacklisted` if the ID is on the list. The frontend should call this before downloading a VSIX.
- `RegisterInstall` — refuses to record state for blacklisted IDs.
- `SetExtensionEnabled` — refuses to enable a blacklisted extension.
- `GetSecurityInfo` / `ListSecurityInfo` — refresh the `Blacklisted` flag from the in-memory set so newly-added entries are reflected without a re-register.

## 8. Known Unsupported VS Code APIs

gugacode's extension host is a **subset** of the VS Code extension API. Only the namespaces and methods listed in `vscodeApi.ts` are implemented; everything else is unavailable. The following is a non-exhaustive list of commonly-expected VS Code APIs that are **not** implemented and will throw (`TypeError: Cannot read properties of undefined`) or return stubs:

### `vscode.languages`
**Implemented**: `registerCompletionItemProvider`, `registerHoverProvider`, `registerDefinitionProvider`, `registerCodeActionProvider`.
**Implemented by the built-in IDE (not via extension host)** (prompt-8/9/10):  
`registerCompletionItemProvider`, `registerHoverProvider`, `registerDefinitionProvider`, `registerReferenceProvider`, `registerDocumentFormattingEditProvider`, `registerSignatureHelpProvider`, `registerRenameProvider` — wired for Go/TS/JS through the internal LSP service (gopls / typescript-language-server).  

**Not implemented in the VS Code extension shim**: `registerDocumentSymbolProvider`, `registerDocumentHighlightProvider`, `registerDocumentRangeFormattingEditProvider`, `registerOnTypeFormattingEditProvider`, `registerFoldingRangeProvider`, `registerDocumentLinkProvider`, `registerColorProvider`, `registerInlayHintsProvider`, `registerDocumentSemanticTokensProvider`, `registerWorkspaceSymbolProvider`, `createDiagnosticCollection`, `getDiagnostics`, `setTextDocumentLanguage`, `match`, `onDidChangeDiagnostics`.

### `vscode.workspace`
**Implemented**: `fs.readFile`, `fs.writeFile`, `fs.exists`, `fs.createDirectory`; `getConfiguration` (stub — returns defaults, never real settings); `onDidChangeConfiguration` (stub — no-op disposable).
**Not implemented**: `fs.stat`, `fs.delete`, `fs.rename`, `fs.copy`, `fs.readDirectory` (the typed method; `readdir` exists on the backend but is not wired through the shim), `openTextDocument`, `openNotebookDocument`, `save`, `saveAs`, `updateWorkspaceFolders`, `onDidChangeTextDocument`, `onDidOpenTextDocument`, `onDidCloseTextDocument`, `onDidSaveTextDocument`, `onDidChangeConfiguration` (real events), `onDidChangeWorkspaceFolders`, `findFiles`, `findTextInFiles`, `applyEdit`, `createFileSystemWatcher`, `asRelativePath`, `rootPath`, `workspaceFolders`, `name`, `notebookDocuments`, `textDocuments`.

### `vscode.window`
**Implemented**: `createWebviewPanel`, `showInformationMessage`, `showWarningMessage`, `showErrorMessage`; `activeTextEditor` (always `undefined` in v1).
**Not implemented**: `showInputBox`, `showQuickPick`, `showOpenDialog`, `showSaveDialog`, `createOutputChannel`, `createTerminal`, `createTextEditorDecorationType`, `setStatusBarMessage`, `createStatusBarItem`, `createWebviewView`, `registerWebviewViewProvider`, `registerTreeDataProvider`, `createTreeView`, `showTextDocument`, `visibleTextEditors`, `onDidChangeActiveTextEditor`, `onDidChangeTextEditorSelection`, `onDidChangeTextEditorVisibleRanges`, `terminal`, `activeTerminal`, `onDidOpenTerminal`, `onDidCloseTerminal`, `registerUriHandler`, `registerCustomEditorProvider`, `showNotebookDocument`.

### `vscode.commands`
**Implemented**: `registerCommand`, `executeCommand` (applies the dangerous-command gate).
**Not implemented**: `registerTextEditorCommand`, `getCommands`, `onDidExecuteCommand`.

### Entire namespaces not implemented
The following VS Code namespaces are **not exposed at all** (absent from the API object):
- `vscode.debug` — no Debug Adapter Protocol support.
- `vscode.extensions` — extensions cannot enumerate or activate other extensions.
- `vscode.scm` — no Source Control Management API.
- `vscode.tasks` — no Task system.
- `vscode.tests` — no Test Explorer API.
- `vscode.notebooks` — no Notebook API.
- `vscode.comments` — no Comment API.
- `vscode.authentication` — no Authentication Provider API.
- `vscode.env` — no environment API (`env.clipboard`, `env.openExternal`, `env.machineId`, etc.).
- `vscode.l10n` — no localization API.
- `vscode.clipboard` — clipboard is a permission, not a namespace.
- `vscode.tabGroups` / `vscode.tabs` — no tab model.

### Stubbed (present but inert)
- `workspace.getConfiguration(section)` — returns an empty snapshot; `get()` returns the default value, `has()` returns `false`. There is no live settings bridge.
- `workspace.onDidChangeConfiguration` — returns a no-op disposable; configuration changes are not forwarded.
- `window.activeTextEditor` — always `undefined` in v1.

Extensions that depend on these unimplemented APIs will fail at runtime. This is by design: the compatibility layer exposes only what gugacode can permission-gate and bridge to its own services.

## 9. Reporting Extension Security Issues

If you discover a security issue in an extension hosted on the gugacode marketplace, or a bypass of the extension security model itself:

1. **Do NOT open a public GitHub issue.**
2. Email **security@gugacode.dev** with:
   - The extension ID (`<publisher>.<name>`) and version.
   - The SHA-256 of the VSIX (shown in the extension's security info panel).
   - A description of the vulnerability and steps to reproduce.
   - Whether the extension is already on the blacklist (so we know whether to escalate to a takedown).
3. You will receive an acknowledgment within 48 hours.
4. Confirmed-malicious extension IDs are added to the built-in `defaultBlacklist` in `services/extension_blacklist.go` and shipped in the next release. User-added entries persist immediately in `<configDir>/gugacode/extension-blacklist.json`.

For gugacode-core vulnerabilities (not extension-specific), follow the general process in [`../SECURITY.md`](../SECURITY.md) §"Reporting a Vulnerability".
