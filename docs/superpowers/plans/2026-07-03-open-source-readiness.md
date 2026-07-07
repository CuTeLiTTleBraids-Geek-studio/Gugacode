# Open Source Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform gugacode from a working IDE prototype into a properly documented, licensed, and CI-tested open-source project ready for public release.

**Architecture:** Documentation and CI configuration only — no application code changes. All files live at the project root or in `.github/`.

**Tech Stack:** Markdown, YAML (GitHub Actions), MIT License

---

## File Structure

- **Create:** `LICENSE` — MIT license text
- **Create:** `CONTRIBUTING.md` — contribution guidelines
- **Create:** `CODE_OF_CONDUCT.md` — Contributor Covenant 2.1
- **Create:** `CHANGELOG.md` — version history (Keep a Changelog format)
- **Create:** `.github/workflows/ci.yml` — CI pipeline (Go build/test + frontend test)
- **Rewrite:** `README.md` — project introduction, features, install, usage
- **Modify:** `.gitignore` — add binary artifacts, config files

---

### Task 1: Create LICENSE (MIT)

**Files:**
- Create: `LICENSE`

- [ ] **Step 1: Create MIT license file**

```
MIT License

Copyright (c) 2026 gugacode contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Commit**

```bash
git add LICENSE
git commit -m "docs: add MIT license"
```

---

### Task 2: Rewrite README.md

**Files:**
- Rewrite: `README.md`

- [ ] **Step 1: Replace generic Wails README with project-specific README**

The new README must include: project name + tagline, feature list, screenshots placeholder, prerequisites, install/build/run instructions, project structure overview, AI configuration guide, testing instructions, license reference, contributing link.

See Task 2 implementation for the full content.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: rewrite README for gugacode open-source release"
```

---

### Task 3: Create CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`

- [ ] **Step 1: Create contribution guidelines**

Include: how to report bugs, how to request features, development setup, code style, testing requirements, PR process, commit message conventions.

- [ ] **Step 2: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: add contributing guidelines"
```

---

### Task 4: Create CODE_OF_CONDUCT.md

**Files:**
- Create: `CODE_OF_CONDUCT.md`

- [ ] **Step 1: Create Contributor Covenant 2.1 code of conduct**

- [ ] **Step 2: Commit**

```bash
git add CODE_OF_CONDUCT.md
git commit -m "docs: add code of conduct"
```

---

### Task 5: Create CHANGELOG.md

**Files:**
- Create: `CHANGELOG.md`

- [ ] **Step 1: Create changelog in Keep a Changelog format**

Document v0.1.0 with all features implemented across Plans 1-5.

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add changelog for v0.1.0"
```

---

### Task 6: Update .gitignore

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Add missing entries**

Add: binary artifacts (`*.exe`, `changeme.exe`, `gugacode.exe`), OS files (`.DS_Store`, `Thumbs.db`), IDE files (`.vscode/`, `.idea/`), env files (`.env`, `.env.local`).

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: update gitignore for open-source hygiene"
```

---

### Task 7: Create GitHub Actions CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create CI pipeline**

Workflow triggers on push/PR to main. Jobs:
- `go-test`: setup Go 1.25, run `go vet .`, `go build .`, `go test ./services/... -v`
- `frontend-test`: setup Node 20, cd frontend, npm ci, npx vue-tsc --noEmit, npx vitest run

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add GitHub Actions workflow for Go and frontend tests"
```

---

### Task 8: Verification

- [ ] **Step 1: Verify Go backend**

Run: `go vet . && go build . && go test ./services/... -v`
Expected: all pass

- [ ] **Step 2: Verify frontend**

Run: `cd frontend && npx vue-tsc --noEmit && npx vitest run`
Expected: all pass, 77 tests

- [ ] **Step 3: Verify documentation files exist**

Check: LICENSE, README.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md, CHANGELOG.md, .github/workflows/ci.yml all exist and are non-empty.
