# Security & Bug Fix Implementation Plan (Plan 7)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix known security issues and functional bugs to make gugacode production-ready.

**Architecture:** Go backend service fixes only — TDD approach (write failing test, fix, verify).

**Tech Stack:** Go 1.25, net/http, os

---

### Task 1: Fix output_buffer.go — Clear buffer after Read

**Bug:** `Read()` returns buffer contents but never clears them, causing stale data on subsequent reads.

**Files:**
- Modify: `services/output_buffer.go`
- Test: `services/output_buffer_test.go` (new)

- [x] **Step 1: Write failing test** — append data, read, then read again; second read must be empty
- [x] **Step 2: Fix** — add `o.buf.Reset()` after `o.buf.String()`
- [x] **Step 3: Verify** — `go test ./services/... -run TestOutputBuffer -v`

---

### Task 2: Fix ai_service.go — Add HTTP timeout

**Bug:** `Send()` and `SendStreamWithContext()` use `http.DefaultClient` which has no timeout, allowing hung requests to block forever.

**Files:**
- Modify: `services/ai_service.go`

- [x] **Step 1:** Add module-level `var aiHTTPClient = &http.Client{Timeout: 120 * time.Second}` for non-streaming
- [x] **Step 2:** Add `var aiStreamHTTPClient = &http.Client{}` (no total timeout for long streams; context handles cancellation) for streaming
- [x] **Step 3:** Replace `http.DefaultClient.Do(req)` with `aiHTTPClient.Do(req)` in `Send()` and `aiStreamHTTPClient.Do(req)` in `SendStreamWithContext()`
- [x] **Step 4:** Verify existing tests still pass

---

### Task 3: Fix ai_service.go — Validate HTTP status code

**Bug:** If the AI API returns 401/403/429/500, the code tries to decode the error body as a valid response, producing confusing errors.

**Files:**
- Modify: `services/ai_service.go`
- Test: `services/ai_service_test.go`

- [x] **Step 1: Write failing test** — server returns 401, expect descriptive error
- [x] **Step 2: Fix** — add status code check after `resp, err := client.Do(req)` in both `Send()` and `SendStreamWithContext()`
- [x] **Step 3: Verify**

---

### Task 4: Fix terminal_service.go — Validate workingDir

**Bug:** `Start(workingDir)` passes any string to the PTY without checking it exists, which can cause confusing errors or security issues.

**Files:**
- Modify: `services/terminal_service.go`
- Test: `services/terminal_service_test.go`

- [x] **Step 1: Write failing test** — `Start("/nonexistent/path")` should return error
- [x] **Step 2: Fix** — add `os.Stat()` check before `startPty()`
- [x] **Step 3: Verify**

---

### Task 5: Full Verification

- [x] `go vet . && go build . && go test ./services/... -v`
- [x] `cd frontend && npx vue-tsc --noEmit && npx vitest run`
