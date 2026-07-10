# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| 1.0.x | ✅ |
| < 1.0 | ❌ |

## Reporting a Vulnerability

If you discover a security vulnerability in gugacode, please report it responsibly:

1. **Do NOT open a public GitHub issue** for security vulnerabilities.
2. Email security@gugacode.dev with a description of the vulnerability, steps to reproduce, and potential impact.
3. You will receive an acknowledgment within 48 hours.
4. We will investigate and provide a fix timeline within 7 days.

Please include:
- Description of the vulnerability
- Steps to reproduce
- Affected components (backend service, frontend component, etc.)
- Potential impact
- Suggested fix (if any)

## Continuous Integration Security Gates

The CI workflow (`.github/workflows/ci.yml`) enforces the security measures below on every push and pull request to `main`/`master`. What CI actually executes is the source of truth for this document — the gates listed here are run in CI, not just documented.

| Gate | CI job | Requirement |
|---|---|---|
| **Race detector** (G-SEC-04) | `go-test` (ubuntu/windows/macos) | `go test -race ./services/... .` — data-race detection across all three platforms. |
| **govulncheck** (G-SEC-04) | `govulncheck` (ubuntu) | `go run golang.org/x/vuln/cmd/govulncheck@latest ./services/... .` — scans Go dependencies for known CVEs. |
| **Frontend type check** (G-QUAL-03) | `frontend-test` (ubuntu/windows/macos) | `npx vue-tsc --noEmit` — TypeScript type safety across all three platforms. |
| `go vet` | `go-test` | Static analysis on `./services/... .`. |
| `golangci-lint` | `go-lint` | Additional linters (errcheck, ineffassign, staticcheck, unused). |
| ESLint | `frontend-test` | `npm run lint` on the frontend. |
| Vitest | `frontend-test` | `npx vitest run` across all three platforms. |
| `wails3 build` | `wails-build` | Full production build (frontend bundling + asset embedding + bindings). |

CI runs Go build/test **and** frontend checks on Ubuntu, Windows, and macOS so that platform-specific code (`pty_windows.go`, `secrets_windows.go`, etc.) is verified on its target platform. `npm audit` is recommended before releases but is not yet wired into CI.

## Security Measures

### BaseURL validation — SSRF prevention (G-SEC-01)
`services/ai_urlsec.go` validates every user-supplied AI BaseURL before it is stored or used:
- Scheme must be `http` or `https` (rejects `file:`, `data:`, `ftp:`, `gopher:`, `javascript:`, etc.).
- No embedded userinfo (`http://user:pass@host`) is allowed — a credential-leakage vector.
- Non-loopback hosts MUST use `https`. Loopback (`localhost`, `127.0.0.0/8`, `::1`, `*.localhost`) may use plain `http` to support local LLM servers (Ollama, LM Studio, llama.cpp).
- The AI HTTP client disables redirects (`CheckRedirect: noRedirectPolicy`) so a malicious provider cannot redirect the API key to a different host.

### Agent commands require manual approval (G-SEC-02)
Every non-empty agent/shell command requires explicit user approval before execution — there is **no auto-approve** path for `run`. The `Safe` risk level is reserved for the empty-command no-op; all real commands return at minimum `RiskElevated`. Dangerous-command patterns (e.g. `rm -rf`, deletes targeting root/home/wildcards) are flagged for extra scrutiny, but the primary protection is always mandatory manual approval.

**Write tools (prompt-5 Task E / BUG-M4):** `write` is also **never auto-approved**, regardless of `toolApprovalConfig.write`. Only safer tools such as `read` and `search` may use `auto-approve`. The Settings → Agent UI hides the auto-approve option for `run` and `write`. Without an open project workspace, `write`/`run` refuse to execute (no empty-root sandbox bypass).

### Untrusted workflows do not auto-execute (G-SEC-03)
Project-level workflows loaded from `.nknk/workflows/` are treated as untrusted because they ship with the repository and may be malicious. `RequiresConfirmation` is **forced to `true`** for project sources regardless of what the workflow file declares, so a cloned repo cannot bypass the confirmation gate by setting `requiresConfirmation: false`. Startup-trigger workflows in particular are never auto-executed on project load; the UI lists them as "Pending Confirmation" and the user must click "Run".

### Race detector + govulncheck in CI (G-SEC-04)
See the table above. `go test -race` catches data races in concurrent backend code; `govulncheck` catches known CVEs in `go.mod` transitively-resolved modules that `go vet` and `golangci-lint` do not detect.

### iframe sandbox uses `allow-scripts` only (G-SEC-05)
Extension webviews and plugin views render in an `<iframe sandbox="allow-scripts">`:
- **No `allow-same-origin`** — the iframe gets an opaque origin and cannot access the parent's DOM, `localStorage`, cookies, or `window.go` bindings.
- No `allow-forms`, `allow-top-navigation`, `allow-popups`, `allow-pointer-lock`.
- Every `postMessage` is validated by `event.source` equality (not `event.origin`, which is `"null"` for sandboxed iframes and can be spoofed).
- Every RPC from the iframe is permission-gated by the same `METHOD_PERMISSIONS` map as the Worker sandbox.

### Symlink path validation via EvalSymlinks (G-SEC-06)
`services/pathsec.go` centralizes path-traversal defense. `ValidatePathWithinRoot` resolves symlinks on **both** the target and the workspace root before checking the relative path, so a symlink inside the workspace that points outside is rejected. For not-yet-existing targets (e.g. a file about to be created), the parent directory's symlinks are resolved and the basename is re-joined. `IsRelativePathSafe` rejects `..`, absolute paths, Windows drive paths, UNC paths, and volume-relative forms (`C:foo`).

### API key encrypted, not returned to frontend (G-SEC-07)
`services/secrets*.go` encrypts the AI API key at rest:
- **Windows**: DPAPI (`CryptProtectData`), machine-bound — stored as a `"dpapi:"`-prefixed blob in `settings.json`.
- **macOS / Linux**: AES-256-GCM with a per-install 32-byte key file (`~/.config/gugacode/secret.key`, `0600`), or the platform keychain (Keychain / libsecret) via a `"keyring:"` marker.
- Legacy plaintext keys are auto-migrated to encrypted form on first `LoadSettings`.

`LoadSettings` clears `AIApiKey` to `""` before returning across the Wails binding, so the plaintext never lives in the frontend JS heap. The frontend receives only `AIApiKeyConfigured` (bool) and `AIApiKeyStorageMethod` (`"dpapi"`/`"aes"`/`"keyring"`/`"plain"`/`"none"`). The decrypted key is available to the backend only via `GetDecryptedAPIKey`. Unrelated saves that arrive with an empty key + `AIApiKeyConfigured=true` preserve the existing on-disk key so unrelated settings edits don't wipe the stored key.

### HTTP response body limited to 64 KB (G-SEC-08)
`services/ai_service.go` `parseAIError` reads error-response bodies with `io.LimitReader(resp.Body, 64*1024)` so a malicious or misconfigured provider cannot exhaust memory with a huge error payload.

### Atomic JSON writes (G-SEC-09)
`services/atomic_write.go` writes settings (and other JSON state) atomically: marshal → temp file in the same directory → `fsync` → `chmod 0600` → `rename`. A crash mid-write cannot leave a half-written `settings.json`. The settings file is `0600` because it holds an (encrypted) API key.

### CSP nonce uses crypto/rand, no fallback (G-SEC-10)
`main.go` `generateNonce` produces a fresh 16-byte hex nonce per HTML response using `crypto/rand.Read`. If `crypto/rand.Read` fails, the request is refused (HTTP 500) — there is **no fallback** to a predictable time-derived nonce. A predictable nonce would defeat CSP, so the page is not served rather than shipped with a weak nonce. Each response gets its own nonce; nonces are not reused. This lets `script-src` use `'nonce-<N>'` instead of `'unsafe-inline'`.

### Markdown links forced to `target=_blank rel=noopener` (G-SEC-11)
`frontend/src/lib/markdown.ts` registers a DOMPurify `afterSanitizeAttributes` hook that forces every `<a href>` to `target="_blank" rel="noopener noreferrer"`. Markdown is rendered with `marked`, syntax-highlighted with `highlight.js`, and sanitized with DOMPurify (allow-listed tags/attributes, `ALLOW_DATA_ATTR: false`) before insertion. Vue's template engine escapes all other user input by default.

### Extension security — SHA-256 verification, default disabled, permission classification (G-SEC-12)
`services/extension_security_service.go` + `services/extension_blacklist.go` implement VS Code-style extension security (a separate code path from the native plugin system):
- **Signature verification**: VSIX files are verified via SHA-256 hash against the marketplace-published hash. Unverified extensions cannot be enabled (`ErrNotVerified`).
- **Default disabled + pending review**: newly installed extensions start `Enabled=false, PendingReview=true`. The first enable attempt surfaces a popup listing the requested API permissions.
- **Permission classification**: each extension is classified `Trusted` / `Reviewed` / `Restricted` from its declared permissions. Enabling a `Restricted` extension requires explicit approval (`ErrRestrictedRequiresApproval`).
- **Blacklist**: known-malicious extension IDs (e.g. `anabarban.anabarban`, `esbenp.prettier-vscode-stolen`) are blocked from installation and enablement. The built-in list cannot be removed at runtime; users can add entries via `<configDir>/gugacode/extension-blacklist.json`.
- **API surface restriction**: the `vscode`-compatible API shim (`frontend/src/lib/extensionHost/apiSurface.ts`) is deny-by-default — only listed methods are exposed, and dangerous commands (`workbench.action.terminal.sendSequence`, `workbench.action.files.save`, `_workbench.*`) always require confirmation regardless of security level. Extensions never receive `appState` or `window.go` bindings directly.

See [`docs/extension-security.md`](docs/extension-security.md) for the full extension security model.

### Path Sandboxing
All file operations are sandboxed to the workspace root. `FileService.validatePath()` (and the shared `services/pathsec.go` helpers) prevent directory traversal attacks by checking that the resolved path is within the workspace root, with symlinks evaluated on both sides. Terminal sessions and agent CWDs validate their working directory similarly.

### Input Validation
- Project IDs and flat-namespace names are validated as safe relative paths (no separators, no `..`, no absolute paths) to prevent path traversal via filenames.
- AI API responses are checked for non-2xx status codes and parsed for structured error messages (body capped at 64 KB).
- HTTP clients disable redirects to prevent SSRF.

### XSS Prevention
- Markdown rendering uses DOMPurify to sanitize HTML before rendering, with links forced to `target="_blank" rel="noopener noreferrer"`.
- All user input displayed in the UI is escaped by Vue's template engine by default.

### API Key Storage
- API keys are encrypted at rest (DPAPI on Windows, AES-256-GCM / keychain elsewhere) and never transmitted to any server except the user-configured AI provider.
- The decrypted API key is never returned to the frontend across the Wails binding (G-SEC-07).
- API keys are not logged or included in error messages.

### Dependency Security
- `govulncheck ./services/... .` runs in CI on every push/PR (G-SEC-04).
- `npm audit` should be run in the `frontend/` directory before releases.
- `golangci-lint` and `go vet` run in CI to catch issues `govulncheck` does not cover.

## Security Headers

The Wails v3 webview's asset middleware injects the following headers on every response (`main.go`):
- `Content-Security-Policy` — `script-src 'nonce-<N>'` (per-response nonce from `crypto/rand`, no `'unsafe-inline'`); `connect-src` restricted to `'self'` because all AI/network calls are made from Go, not from the webview.
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Referrer-Policy: no-referrer`

The desktop app makes external network requests only for AI provider API calls (to the user-configured BaseURL, validated per G-SEC-01). Link clicks in the Help menu open in the external browser. No CSRF, ClickJacking, or CORS protections are needed beyond the headers above since the app runs in a desktop webview, not a browser tab.

## Disclosure Timeline

- **Day 0**: Vulnerability reported
- **Day 1-2**: Acknowledgment and initial assessment
- **Day 3-7**: Fix development and testing
- **Day 7-14**: Patch release (severity-dependent)
- **Day 30**: Public disclosure (if applicable)

## Contact

- Security email: security@gugacode.dev
- General issues: [GitHub Issues](https://github.com/gugacode/gugacode/issues)
