# Contributing to gugacode

Thank you for your interest in contributing to gugacode! This document describes how to set up your development environment and the conventions we follow.

## Reporting Issues

- **Bugs:** Open a GitHub issue with the label `bug`. Include OS, gugacode version, steps to reproduce, and expected vs actual behavior.
- **Feature requests:** Open a GitHub issue with the label `enhancement`. Describe the use case and proposed solution.
- **Security vulnerabilities:** Do NOT open a public issue. Email the maintainers privately.

## Development Setup

### Prerequisites

- **Go** 1.25 or later
- **Node.js** 20 or later (with npm)
- **Wails3 CLI** (optional, only needed for `wails3 dev` / `wails3 build`)

### Getting the Code

```bash
git clone https://github.com/<your-org>/gugacode.git
cd gugacode
```

### Installing Dependencies

```bash
# Go modules
go mod download

# Frontend dependencies
cd frontend
npm install
```

### Running in Development

If you have the Wails3 CLI installed:

```bash
wails3 dev -config ./build/config.yml -port 9245
```

If not, you can run the frontend and backend separately:

```bash
# Terminal 1 — frontend dev server
cd frontend
npm run dev

# Terminal 2 — Go backend
go run .
```

### Running Tests

```bash
# Go backend tests
go test ./services/... -v

# Frontend tests
cd frontend
npx vitest run

# Type checking
cd frontend
npx vue-tsc --noEmit
```

All tests must pass before a PR can be merged.

## Code Style

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) and `gofmt` / `goimports`.
- Run `go vet .` before committing — it must report no issues.
- Service methods receive a pointer receiver (`*ServiceName`).
- Exported identifiers have doc comments starting with the identifier name.
- Handle errors at boundaries; never swallow them silently.

### TypeScript / Vue

- Use `<script setup lang="ts">` for all new components.
- Follow the existing file structure: `stores/` for reactive state, `components/` for UI, `views/` for route-level pages, `composables/` for reusable logic, `lib/` for pure utilities.
- Prefer composition API over options API.
- Use Element Plus components where a matching component exists.
- Run `npx vue-tsc --noEmit` before committing — it must report no errors.

### CSS

- Use BEM-style class naming (`block__element--modifier`).
- Use CSS custom properties (`var(--color-...)`) from `assets/styles/main.css` — never hardcode colors.
- Scope styles with `<style scoped>` in `.vue` files.

## Commit Message Conventions

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**
- `feat` — new feature
- `fix` — bug fix
- `docs` — documentation only
- `style` — formatting, no code change
- `refactor` — code change that neither fixes a bug nor adds a feature
- `test` — adding or correcting tests
- `chore` — build process, auxiliary tools, dependencies
- `ci` — CI configuration changes

**Examples:**
```
feat(ai): add conversation history sidebar
fix(terminal): clear output buffer after read
docs: update README with AI configuration guide
test(editor): add saveFile unit tests
```

## Pull Request Process

1. Fork the repository and create a feature branch from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```
2. Make your changes. Keep commits focused — one logical change per commit.
3. Add or update tests for your changes. New code must have test coverage.
4. Run the full test suite (Go + frontend) and ensure everything passes.
5. Update the CHANGELOG.md `Unreleased` section with your changes.
6. Open a pull request. Reference any related issues (`Closes #123`).
7. Respond to review feedback and update your PR.

### PR Checklist

- [ ] Tests pass (`go test ./services/...` and `cd frontend && npx vitest run`)
- [ ] `go vet .` is clean
- [ ] `npx vue-tsc --noEmit` is clean
- [ ] CHANGELOG.md updated (if user-facing change)
- [ ] Commit messages follow Conventional Commits
- [ ] No new linting warnings introduced

## Project Structure

```
gugacode/
├── main.go                  # Go entry point, service registration
├── services/                # Go backend services (file, git, ai, terminal, ...)
├── frontend/
│   ├── src/
│   │   ├── api/             # Wails service bindings
│   │   ├── components/      # Vue UI components
│   │   ├── composables/     # Vue composables (useKeyboard, etc.)
│   │   ├── lib/             # Pure utility functions (markdown, language detection)
│   │   ├── router/          # Vue Router configuration
│   │   ├── stores/          # Reactive application state
│   │   ├── types/           # TypeScript type definitions
│   │   └── views/           # Route-level views
│   ├── bindings/            # Auto-generated Wails bindings
│   └── package.json
├── build/                   # Platform-specific build configs
└── docs/                    # Documentation and implementation plans
```

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
