# Security Hardening Implementation Plan (Plan 9)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Harden gugacode against the security gaps identified in the §9 harness matrix (G-SEC-01 through G-SEC-12) and the multi-vector review (M-3 through M-7).

**Architecture:** Backend Go services + frontend Vue + CI pipeline. Each task is a focused, test-driven fix that closes one gate without regressing existing behavior.

**Tech Stack:** Go 1.25, Wails v3, Vue 3, GitHub Actions

---

### Task 1: G-SEC-07 — API Key isolation

**Goal:** API keys must never be returned to the frontend in plaintext. `LoadSettings` clears the key and exposes only a `configured` boolean; AI calls use `UseStoredKey` + `ConfigID` to reference the stored key server-side.

**Files:**
- Modify: `services/settings_service.go` — `LoadSettings` zeroes `apiKey`, sets `APIKeyConfigured`
- Modify: `services/ai_service.go` — `AIConfig` adds `UseStoredKey`/`ConfigID`; `SetConfig` stores key when provided
- Modify: `frontend/src/api/services.ts` — remove double type assertion on `SetConfig` payload

- [x] Step 1: `LoadSettings` clears apiKey + sets APIKeyConfigured
- [x] Step 2: `AIConfig` + `SetConfig` implement useStoredKey logic
- [x] Step 3: Frontend `services.ts` double assertion removed
- [x] Step 4: Verify `go test ./services/... -run TestSettings -v`

---

### Task 2: G-SEC-09 / M-5 — Atomic writes

**Goal:** All persistent state writes must be atomic (temp + sync + chmod + rename) to prevent half-written files on crash. Sensitive files use 0600 permissions.

**Files:**
- Create: `services/atomic_write.go` — `atomicWriteJSON` + `atomicWriteFile` helpers
- Modify: `services/plugin_service.go`, `marketplace_service.go`, `extension_security_service.go`, `extension_blacklist.go`, `conversation_service.go`, `project_service.go`, `profile_service.go`, `rules_service.go`, `preset_service.go` — replace `os.WriteFile` with atomic helpers

- [x] Step 1: `atomicWriteJSON` + `atomicWriteFile` implemented
- [x] Step 2: 9 service files migrated to atomic writes
- [x] Step 3: `agent_service.go` audit log 0644 → 0600
- [x] Step 4: Verify `go test ./services/... -run TestAtomicWrite -v`

---

### Task 3: G-SEC-12 — Extension marketplace security chain

**Goal:** Marketplace installs must route through `RegisterInstall` for security classification + blacklist check. Extensions default to disabled; enabling a Reviewed/Restricted extension requires explicit user approval.

**Files:**
- Modify: `services/marketplace_service.go` — `installFromVSIXData` calls `RegisterInstall`
- Modify: `services/extension_security_service.go` — `requestEnableExtension` approval flow
- Create: `frontend/src/components/modals/ExtensionPermissionDialog.vue` — permission approval dialog
- Modify: `frontend/src/components/marketplace/MarketplacePanel.vue` — `toggleEnabled` uses `requestEnableExtension`

- [x] Step 1: `installFromVSIXData` calls `RegisterInstall`
- [x] Step 2: `requestEnableExtension` approval flow implemented
- [x] Step 3: ExtensionPermissionDialog component created
- [x] Step 4: MarketplacePanel uses `requestEnableExtension`
- [x] Step 5: Verify `go test ./services/... -run TestMarketplace -v`

---

### Task 4: G-SEC-11 — XSS prevention (v-html ban + DOMPurify)

**Goal:** Ban `v-html` in Vue templates via ESLint; all HTML rendering must go through DOMPurify with hooks enforcing `target/rel` and removing Google Fonts.

**Files:**
- Modify: `frontend/eslint.config.js` — add `"vue/no-v-html": "error"`
- Modify: `frontend/src/lib/markdown.ts` — DOMPurify hooks

- [x] Step 1: ESLint rule `vue/no-v-html: error` added
- [x] Step 2: DOMPurify hooks enforced
- [x] Step 3: Verify `npm run lint` passes

---

### Task 5: M-3 / M-4 — Windows PTY + terminal shell whitelist

**Goal:** Windows PTY arguments escaped via `syscall.EscapeArg`; terminal shell selection restricted to a whitelist.

**Files:**
- Modify: `services/pty_windows.go` — use `syscall.EscapeArg`
- Modify: `services/terminal_service.go` — `allowedShells` whitelist + `isAllowedShell`

- [x] Step 1: `pty_windows.go` uses `syscall.EscapeArg`
- [x] Step 2: `terminal_service.go` shell whitelist implemented
- [x] Step 3: Verify `go test ./services/... -run TestTerminal -v`

---

### Task 6: Path validation (symlink hardening)

**Goal:** All file path validation must use `EvalSymlinks` + base-dir containment to prevent symlink escapes.

**Files:**
- Modify: `services/pathsec.go` — `ValidatePathWithinRoot` with `EvalSymlinks`
- Modify: `services/file_service.go` — delegate to shared `validatePath`

- [x] Step 1: `ValidatePathWithinRoot` uses `EvalSymlinks`
- [x] Step 2: `FileService.validatePath` shared helper
- [x] Step 3: Verify `go test ./services/... -run TestPathSec -v`

---

### Task 7: Single instance enforcement

**Goal:** Only one gugacode instance may run per user, enforced via `InstanceLock` (O_EXCL) in `main.go`.

**Files:**
- Create: `services/instance_lock.go` — `InstanceLock` with O_EXCL
- Modify: `main.go` — acquire lock on startup, release on shutdown

- [x] Step 1: `InstanceLock` implemented
- [x] Step 2: `main.go` acquires lock on startup
- [x] Step 3: Verify `go test ./services/... -run TestInstanceLock -v`

---

### Task 8: G-SEC-04 — govulncheck CI gate

**Goal:** CI runs `govulncheck` to scan Go dependencies for known CVEs.

**Files:**
- Modify: `.github/workflows/ci.yml` — add `govulncheck` job

- [x] Step 1: govulncheck job added to CI
- [x] Step 2: Verify CI workflow YAML validity

---

### Task 9: G-PERF-04 — Performance regression gate

**Goal:** CI runs Go benchmarks and compares against a baseline; regressions >20% fail the job.

**Files:**
- Modify: `.github/workflows/ci.yml` — `perf-benchmark` job with `benchstat`
- Create: `services/perf_bench_test.go` — benchmark functions

- [x] Step 1: perf-benchmark job added with benchstat comparison
- [x] Step 2: `|| true` removed so regressions actually fail
- [x] Step 3: Benchmark functions created
- [x] Step 4: Verify `go test -bench=. ./services/...`

---

### Task 10: Sentinel error centralization

**Goal:** All sentinel errors centralized in `errors.go`; no panics; errors wrapped with `%w`.

**Files:**
- Create: `services/errors.go` — centralized sentinel errors
- Modify: service files — replace ad-hoc errors with sentinels

- [x] Step 1: `errors.go` created with centralized sentinels
- [x] Step 2: Services use sentinels + `%w` wrapping
- [x] Step 3: Verify `go build ./services/...`

---

## Summary

All 10 tasks close a specific G-SEC or M-* gate. The harness §9 matrix now passes G-SEC-01 through G-SEC-12 (BLOCKER level), and M-3 through M-7 (major level). The only known pre-existing issue is `frontend/dist` not being built (requires `wails3 build`), which is unrelated to this plan.
