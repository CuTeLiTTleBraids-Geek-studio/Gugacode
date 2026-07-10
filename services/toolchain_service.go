package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ToolchainService exposes Go/TS/JS toolchain commands (build, test, lint,
// format) for the command palette and editor context menu (G-FEAT-03).
//
// Commands run in the workspace root (or a file's directory) and their
// stdout/stderr is captured. Compiler/linter output is parsed into
// Diagnostic entries so the frontend can surface them in the Problems
// panel; the raw output is also returned for the Output panel.
//
// Tool resolution: the service checks the ToolPaths map (populated from
// Settings.ToolPaths) first, then falls back to PATH via exec.LookPath.
// When a tool is not installed, RunToolchainCommand returns a result with
// Success=false and an explanatory message that the frontend can show as a
// notification with the install command.
type ToolchainService struct {
	mu            sync.Mutex
	workspaceRoot string
	toolPaths     map[string]string
}

// NewToolchainService creates a ToolchainService with no workspace root and
// no tool path overrides. Use SetWorkspaceRoot and SetToolPaths to configure it.
func NewToolchainService() *ToolchainService {
	return &ToolchainService{
		toolPaths: map[string]string{},
	}
}

// SetWorkspaceRoot sets the directory toolchain commands run in (when no
// per-file directory is supplied). Pass an empty string to disable sandboxing.
// Mirrors the pattern used by FileService and AgentService.
func (s *ToolchainService) SetWorkspaceRoot(root string) error {
	if root == "" {
		s.mu.Lock()
		s.workspaceRoot = ""
		s.mu.Unlock()
		return nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory: %s", abs)
	}
	s.mu.Lock()
	s.workspaceRoot = abs
	s.mu.Unlock()
	return nil
}

// SetToolPaths replaces the tool path override map. Keys are tool names
// (e.g. "golangci-lint"), values are absolute (or PATH-resolved) executables.
// Loaded from Settings.ToolPaths by the frontend on startup / settings save.
func (s *ToolchainService) SetToolPaths(paths map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if paths == nil {
		s.toolPaths = map[string]string{}
		return
	}
	cp := make(map[string]string, len(paths))
	for k, v := range paths {
		cp[k] = v
	}
	s.toolPaths = cp
}

// ToolchainCommand describes a single toolchain action exposed to the UI.
type ToolchainCommand struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Language    string   `json:"language"` // "go", "typescript", "javascript", "general"
	Command     string   `json:"command"`  // e.g. "go build", "golangci-lint run"
	Args        []string `json:"args"`
	Description string   `json:"description"`
}

// ToolchainResult is the outcome of running a toolchain command.
type ToolchainResult struct {
	Success  bool                  `json:"success"`
	Output   string                `json:"output"`
	Errors   []ToolchainDiagnostic `json:"errors"`
	Duration int64                 `json:"durationMs"`
	// NotInstalled is true when the tool binary could not be found. The
	// frontend uses this to show an install-command notification instead of
	// a generic error.
	NotInstalled bool   `json:"notInstalled"`
	InstallCmd   string `json:"installCmd,omitempty"`
}

// ToolchainDiagnostic is a single parsed compiler/linter issue. It is
// distinct from the LSP Diagnostic type (which uses an integer severity)
// because toolchain output carries a file path and a string severity.
type ToolchainDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
	Severity string `json:"severity"` // "error", "warning", "info"
	Source   string `json:"source"`   // "go build", "eslint", etc.
}

// allToolchainCommands is the full catalog of supported commands. The
// language field groups them in the command palette.
var allToolchainCommands = []ToolchainCommand{
	// Go
	{ID: "go-build", Label: "Go: Build", Language: "go", Command: "go build", Args: []string{"./..."}, Description: "Compile all Go packages"},
	{ID: "go-test", Label: "Go: Test", Language: "go", Command: "go test", Args: []string{"./..."}, Description: "Run all Go tests"},
	{ID: "go-vet", Label: "Go: Vet", Language: "go", Command: "go vet", Args: []string{"./..."}, Description: "Run go vet on all packages"},
	{ID: "go-mod-tidy", Label: "Go: Mod Tidy", Language: "go", Command: "go mod tidy", Description: "Tidy go.mod and go.sum"},
	// prompt-8 Task 8-I: list vs file-scoped write; avoid dangerous whole-repo default write.
	{ID: "gofmt", Label: "Go: gofmt (list workspace)", Language: "go", Command: "gofmt", Args: []string{"-l", "."}, Description: "List files that need formatting"},
	{ID: "gofmt-file", Label: "Go: gofmt (current file)", Language: "go", Command: "gofmt", Args: []string{"-w"}, Description: "Format current .go file in place (pass file path)"},
	{ID: "goimports", Label: "Go: goimports (list workspace)", Language: "go", Command: "goimports", Args: []string{"-l", "."}, Description: "List files that need import formatting"},
	{ID: "goimports-file", Label: "Go: goimports (current file)", Language: "go", Command: "goimports", Args: []string{"-w"}, Description: "Organize imports for current .go file"},
	{ID: "go-test-pkg", Label: "Go: Test (current package)", Language: "go", Command: "go", Args: []string{"test", "."}, Description: "Run tests in the current package directory"},
	// prompt-9 9-C: Run Test at Cursor is a special command (see RunTestAtCursor).
	{ID: "go-test-cursor", Label: "Go: Test at Cursor", Language: "go", Command: "go", Args: []string{"test"}, Description: "Run the TestXxx under the cursor"},
	{ID: "golangci-lint", Label: "Go: golangci-lint", Language: "go", Command: "golangci-lint run", Description: "Run golangci-lint"},
	// TypeScript / JavaScript
	{ID: "tsc", Label: "TypeScript: Type Check", Language: "typescript", Command: "tsc", Args: []string{"--noEmit"}, Description: "Type-check with tsc --noEmit"},
	{ID: "eslint", Label: "ESLint: Check (workspace)", Language: "javascript", Command: "eslint", Args: []string{"."}, Description: "Lint workspace (no --fix by default)"},
	{ID: "eslint-file", Label: "ESLint: Fix (current file)", Language: "javascript", Command: "eslint", Args: []string{"--fix"}, Description: "eslint --fix for one file"},
	{ID: "prettier", Label: "Prettier: Check (workspace)", Language: "javascript", Command: "prettier", Args: []string{"--check", "."}, Description: "Check formatting (does not write)"},
	{ID: "prettier-file", Label: "Prettier: Write (current file)", Language: "javascript", Command: "prettier", Args: []string{"--write"}, Description: "Format current file with prettier"},
	{ID: "vitest", Label: "Vitest: Run (workspace)", Language: "typescript", Command: "vitest", Args: []string{"run"}, Description: "Run all vitest tests once"},
	{ID: "vitest-file", Label: "Vitest: Run (current file)", Language: "typescript", Command: "vitest", Args: []string{"run"}, Description: "Run vitest for one test file"},
	// prompt-9 9-H: Vitest at cursor uses RunTestAtCursor with language=typescript.
	{ID: "vitest-cursor", Label: "Vitest: Test at Cursor", Language: "typescript", Command: "vitest", Args: []string{"run"}, Description: "Run the it/test under the cursor"},
	// General
	{ID: "npm-scripts", Label: "Run: npm scripts", Language: "general", Command: "npm run", Description: "List runnable package.json scripts"},
	{ID: "make", Label: "Run: Make", Language: "general", Command: "make", Description: "Run Makefile default target"},
}

// installHints maps a tool name to the command a user can run to install it.
// Shown in the "tool not installed" notification.
var installHints = map[string]string{
	"golangci-lint": "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
	"goimports":     "go install golang.org/x/tools/cmd/goimports@latest",
	"tsc":           "npm i -g typescript",
	"eslint":        "npm i -g eslint",
	"prettier":      "npm i -g prettier",
	"vitest":        "npm i -g vitest",
}

// ListToolchainCommands returns the toolchain commands available in the
// current workspace. Go commands are offered when go.mod is present (or go
// is on PATH); TS/JS commands when package.json is present; general
// commands (make / npm) when their respective files exist. When no
// workspace root is set, the full catalog is returned so the palette stays
// populated (commands will report not-installed at run time if unusable).
func (s *ToolchainService) ListToolchainCommands() []ToolchainCommand {
	s.mu.Lock()
	root := s.workspaceRoot
	s.mu.Unlock()

	if root == "" {
		return append([]ToolchainCommand{}, allToolchainCommands...)
	}

	hasGoMod := fileExists(filepath.Join(root, "go.mod"))
	hasPkgJSON := fileExists(filepath.Join(root, "package.json"))
	hasMakefile := fileExists(filepath.Join(root, "Makefile")) || fileExists(filepath.Join(root, "makefile"))

	var out []ToolchainCommand
	for _, c := range allToolchainCommands {
		switch {
		case c.Language == "go" && !hasGoMod:
			continue
		case (c.Language == "typescript" || c.Language == "javascript") && !hasPkgJSON:
			continue
		case c.ID == "make" && !hasMakefile:
			continue
		case c.ID == "npm-scripts" && !hasPkgJSON:
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		// Fall back to the full catalog so the palette is never empty.
		return append([]ToolchainCommand{}, allToolchainCommands...)
	}
	return out
}

// DetectToolchains reports which toolchain binaries are available. The map
// keys are tool names; values are true when the binary resolves (either via
// the ToolPaths override or exec.LookPath on PATH).
func (s *ToolchainService) DetectToolchains() map[string]bool {
	tools := []string{"go", "gofmt", "goimports", "golangci-lint", "tsc", "eslint", "prettier", "vitest", "npm", "make"}
	out := make(map[string]bool, len(tools))
	for _, name := range tools {
		out[name] = s.resolveTool(name) != ""
	}
	return out
}

// RunToolchainCommand executes the command identified by cmdID. filePath,
// when non-empty, makes the command run in the file's directory instead of
// the workspace root (useful for linting a single file). The command's
// stdout and stderr are captured; compiler/linter output is parsed into
// Diagnostics.
func (s *ToolchainService) RunToolchainCommand(cmdID string, filePath string) (ToolchainResult, error) {
	var cmd ToolchainCommand
	found := false
	for _, c := range allToolchainCommands {
		if c.ID == cmdID {
			cmd = c
			found = true
			break
		}
	}
	if !found {
		return ToolchainResult{}, fmt.Errorf("unknown toolchain command: %s", cmdID)
	}

	// Resolve working directory: file's dir > workspace root > "".
	workDir := s.workDirForFile(filePath)

	// Split the Command field into [tool, ...baseArgs] and resolve the tool.
	tokens := strings.Fields(cmd.Command)
	if len(tokens) == 0 {
		return ToolchainResult{}, fmt.Errorf("empty command for %s", cmdID)
	}
	toolName := tokens[0]
	baseArgs := tokens[1:]

	resolved := s.resolveTool(toolName)
	if resolved == "" {
		return ToolchainResult{
			Success:      false,
			NotInstalled: true,
			InstallCmd:   installHints[toolName],
			Output:       fmt.Sprintf("%s is not installed or not on PATH", toolName),
		}, nil
	}

	args := append(append([]string{}, baseArgs...), cmd.Args...)
	// prompt-8 Task 8-I/J: file-scoped commands append the target path.
	if filePath != "" && strings.HasSuffix(cmdID, "-file") {
		absFile := filePath
		if a, err := filepath.Abs(filePath); err == nil {
			absFile = a
		}
		args = append(args, absFile)
	}

	// Execute with a generous timeout so a stuck linter cannot hang the UI.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	c := exec.CommandContext(ctx, resolved, args...)
	// Package-level go test uses file's directory as workDir.
	c.Dir = workDir
	// Inherit the environment so tools find GOPATH, NODE_PATH, etc.
	c.Env = os.Environ()

	start := time.Now()
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	runErr := c.Run()
	duration := time.Since(start).Milliseconds()

	combined := stdout.String()
	if stderr.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr.String()
	}

	// Parse diagnostics from the combined output.
	diags := parseDiagnostics(cmd, combined)

	success := runErr == nil
	result := ToolchainResult{
		Success:  success,
		Output:   combined,
		Errors:   diags,
		Duration: duration,
	}
	return result, nil
}

// RuntimeVersions holds tool versions for StatusBar (prompt-9 9-I).
type RuntimeVersions struct {
	GoVersion   string `json:"goVersion"`
	NodeVersion string `json:"nodeVersion"`
	GoplsVer    string `json:"goplsVersion"`
	HasGoWork   bool   `json:"hasGoWork"`
}

// DetectRuntimeVersions returns go/node/gopls version strings (prompt-9 9-I/N).
func (s *ToolchainService) DetectRuntimeVersions() RuntimeVersions {
	rv := RuntimeVersions{}
	if p, err := exec.LookPath("go"); err == nil {
		if out, err := exec.Command(p, "version").Output(); err == nil {
			// "go version go1.22.0 windows/amd64" → go1.22.0
			parts := strings.Fields(string(out))
			if len(parts) >= 3 {
				rv.GoVersion = parts[2]
			} else {
				rv.GoVersion = strings.TrimSpace(string(out))
			}
		}
	}
	if p, err := exec.LookPath("node"); err == nil {
		if out, err := exec.Command(p, "--version").Output(); err == nil {
			rv.NodeVersion = strings.TrimSpace(string(out))
		}
	}
	if p, err := exec.LookPath("gopls"); err == nil {
		if out, err := exec.Command(p, "version").Output(); err == nil {
			line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
			rv.GoplsVer = line
		}
	}
	s.mu.Lock()
	root := s.workspaceRoot
	s.mu.Unlock()
	if root != "" {
		if _, err := os.Stat(filepath.Join(root, "go.work")); err == nil {
			rv.HasGoWork = true
		}
	}
	return rv
}

// RunTestAtCursor runs the test under the given line (prompt-9 9-C / 9-H).
// language: "go" | "typescript" | "javascript"
// content: full file buffer used to discover TestXxx / it("/test(" names.
func (s *ToolchainService) RunTestAtCursor(language, filePath string, line int, content string) (ToolchainResult, error) {
	name := findTestNameAtLine(language, content, line)
	if name == "" {
		return ToolchainResult{
			Success: false,
			Output:  "no test found at cursor (expected func TestXxx or it/test(...))",
		}, nil
	}
	workDir := s.workDirForFile(filePath)
	var resolved string
	var args []string
	switch language {
	case "go":
		resolved = s.resolveTool("go")
		if resolved == "" {
			return ToolchainResult{Success: false, NotInstalled: true, InstallCmd: "install Go from https://go.dev", Output: "go not found"}, nil
		}
		// go test -run: TestXxx or TestXxx/sub (prompt-10 10-C)
		runPat := name
		if !strings.Contains(name, "/") {
			runPat = "^" + name + "$"
		}
		args = []string{"test", "-count=1", "-run", runPat, "."}
	case "typescript", "javascript":
		resolved = s.resolveTool("vitest")
		if resolved == "" {
			// try npx vitest
			if npx, err := exec.LookPath("npx"); err == nil {
				resolved = npx
				args = []string{"vitest", "run", "-t", name}
				if filePath != "" {
					args = append(args, filePath)
				}
			} else {
				return ToolchainResult{Success: false, NotInstalled: true, InstallCmd: "npm i -D vitest", Output: "vitest not found"}, nil
			}
		} else {
			args = []string{"run", "-t", name}
			if filePath != "" {
				args = append(args, filePath)
			}
		}
	default:
		return ToolchainResult{Success: false, Output: "unsupported language for test-at-cursor: " + language}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, resolved, args...)
	c.Dir = workDir
	c.Env = os.Environ()
	start := time.Now()
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	runErr := c.Run()
	combined := stdout.String()
	if stderr.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr.String()
	}
	cmdMeta := ToolchainCommand{ID: "test-cursor", Command: language + " test", Language: language}
	diags := parseDiagnostics(cmdMeta, combined)
	// Also parse go test FAIL lines generically
	diags = append(diags, parseGoTestFailures(combined)...)
	return ToolchainResult{
		Success:  runErr == nil,
		Output:   combined,
		Errors:   dedupeToolchainDiags(diags),
		Duration: time.Since(start).Milliseconds(),
	}, nil
}

// findTestNameAtLine finds the nearest test name at or above line (0-based).
// prompt-10 10-C: also recognizes Go t.Run subtests and vitest test.each / it.each.
func findTestNameAtLine(language, content string, line int) string {
	lines := strings.Split(content, "\n")
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	if line < 0 {
		return ""
	}
	goFuncRe := regexp.MustCompile(`^\s*func\s+(Test[A-Za-z0-9_]+)`)
	// t.Run("name", ...) or t.Run(`name`, ...)
	goRunRe := regexp.MustCompile(`\bt\.Run\(\s*["'\x60]([^"'\x60]+)["'\x60]`)
	// it/test/describe('name') and it.each/test.each(...)('name'
	jsRe := regexp.MustCompile(`^\s*(?:it|test|describe)(?:\.\w+)?\s*(?:\([^)]*\)\s*)?\(\s*['"\x60]([^'"\x60]+)['"\x60]`)
	jsEachRe := regexp.MustCompile(`(?:it|test)\.each\s*\([^)]*\)\s*\(\s*['"\x60]([^'"\x60]+)['"\x60]`)

	var parentGo string
	var subGo string
	for i := 0; i <= line && i < len(lines); i++ {
		l := lines[i]
		if language == "go" {
			if m := goFuncRe.FindStringSubmatch(l); m != nil {
				parentGo = m[1]
				subGo = ""
			}
			if m := goRunRe.FindStringSubmatch(l); m != nil {
				subGo = m[1]
			}
		}
	}
	if language == "go" {
		// Prefer innermost t.Run at or above cursor; go test -run Parent/Sub
		if parentGo != "" && subGo != "" {
			return parentGo + "/" + subGo
		}
		if parentGo != "" {
			return parentGo
		}
		// fallback scan upward for func only
		for i := line; i >= 0; i-- {
			if m := goFuncRe.FindStringSubmatch(lines[i]); m != nil {
				return m[1]
			}
		}
		return ""
	}

	for i := line; i >= 0; i-- {
		l := lines[i]
		if m := jsEachRe.FindStringSubmatch(l); m != nil {
			return m[1]
		}
		if m := jsRe.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// GoTestJSONEvent is one line of `go test -json` output (prompt-11 11-F).
type GoTestJSONEvent struct {
	Time    string  `json:"time,omitempty"`
	Action  string  `json:"action"` // run|pass|fail|skip|output|start
	Package string  `json:"package,omitempty"`
	Test    string  `json:"test,omitempty"`
	Output  string  `json:"output,omitempty"`
	Elapsed float64 `json:"elapsed,omitempty"`
}

// GoTestJSONResult aggregates structured test run status for the test explorer.
type GoTestJSONResult struct {
	Success bool              `json:"success"`
	Output  string            `json:"output"`
	Events  []GoTestJSONEvent `json:"events"`
	// StatusByTest maps "Package::TestName" or TestName → pass|fail|skip|run
	StatusByTest map[string]string `json:"statusByTest"`
	DurationMs   int64             `json:"durationMs"`
}

// RunGoTestsJSON runs `go test -json` in packageDir (prompt-11 11-F).
func (s *ToolchainService) RunGoTestsJSON(packageDir, runRegex string) (GoTestJSONResult, error) {
	dir := packageDir
	if dir == "" {
		s.mu.Lock()
		dir = s.workspaceRoot
		s.mu.Unlock()
	}
	if dir == "" {
		return GoTestJSONResult{Success: false, Output: "no package directory"}, nil
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	goBin := s.resolveTool("go")
	if goBin == "" {
		return GoTestJSONResult{Success: false, Output: "go not found"}, nil
	}
	args := []string{"test", "-json", "-count=1"}
	if runRegex != "" {
		args = append(args, "-run", runRegex)
	}
	args = append(args, ".")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	start := time.Now()
	out, err := cmd.CombinedOutput()
	events := parseGoTestJSONLines(string(out))
	status := map[string]string{}
	for _, e := range events {
		if e.Test == "" {
			continue
		}
		key := e.Test
		if e.Package != "" {
			key = e.Package + "::" + e.Test
		}
		switch e.Action {
		case "pass", "fail", "skip", "run":
			status[key] = e.Action
			status[e.Test] = e.Action
		}
	}
	return GoTestJSONResult{
		Success:      err == nil,
		Output:       string(out),
		Events:       events,
		StatusByTest: status,
		DurationMs:   time.Since(start).Milliseconds(),
	}, nil
}

func parseGoTestJSONLines(output string) []GoTestJSONEvent {
	var events []GoTestJSONEvent
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var e GoTestJSONEvent
		if json.Unmarshal([]byte(line), &e) == nil && e.Action != "" {
			events = append(events, e)
		}
	}
	return events
}

// parseGoTestFailures extracts file:line from go test failure output (9-C/9-J).
func parseGoTestFailures(output string) []ToolchainDiagnostic {
	// e.g.     main_test.go:12: expected ...
	re := regexp.MustCompile(`(?m)^\s*([\w./\\-]+\.go):(\d+):\s*(.+)$`)
	var out []ToolchainDiagnostic
	for _, m := range re.FindAllStringSubmatch(output, -1) {
		line, _ := strconv.Atoi(m[2])
		out = append(out, ToolchainDiagnostic{
			File:     m[1],
			Line:     line,
			Column:   1,
			Severity: "error",
			Message:  m[3],
			Source:   "go test",
		})
	}
	return out
}

func dedupeToolchainDiags(in []ToolchainDiagnostic) []ToolchainDiagnostic {
	seen := map[string]bool{}
	var out []ToolchainDiagnostic
	for _, d := range in {
		k := fmt.Sprintf("%s:%d:%s", d.File, d.Line, d.Message)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	return out
}

// workDirForFile returns the directory to run a command in for the given
// file path, falling back to the workspace root. Empty when neither is set.
func (s *ToolchainService) workDirForFile(filePath string) string {
	if filePath != "" {
		if abs, err := filepath.Abs(filePath); err == nil {
			return filepath.Dir(abs)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspaceRoot
}

// resolveTool returns the path to the named tool, checking the ToolPaths
// override first then PATH. Returns "" when not found.
//
// An explicit override is authoritative: when set, the tool is resolved
// solely through the override (absolute path must exist, or bare name must
// resolve on PATH). There is no silent fallback to PATH lookup for that
// tool, so users get deterministic control over which binary runs — a
// broken override surfaces as NotInstalled rather than unexpectedly
// executing a different binary found on PATH.
func (s *ToolchainService) resolveTool(name string) string {
	s.mu.Lock()
	override := s.toolPaths[name]
	s.mu.Unlock()
	if override != "" {
		if filepath.IsAbs(override) {
			if fileExists(override) {
				return override
			}
			return ""
		}
		if p, err := exec.LookPath(override); err == nil {
			return p
		}
		return ""
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ---------------------------------------------------------------------------
// Output parsers
// ---------------------------------------------------------------------------

// parseDiagnostics routes the command output to the appropriate parser based
// on the command id / tool. Unknown commands produce no diagnostics.
func parseDiagnostics(cmd ToolchainCommand, output string) []ToolchainDiagnostic {
	switch {
	case cmd.ID == "golangci-lint":
		return parseGolangciLint(output)
	case strings.HasPrefix(cmd.Command, "go build") || strings.HasPrefix(cmd.Command, "go vet"):
		return parseGoCompiler(output, cmd.Command)
	case strings.HasPrefix(cmd.Command, "go test"):
		return parseGoCompiler(output, cmd.Command)
	case strings.HasPrefix(cmd.Command, "tsc"):
		return parseTypeScript(output)
	case strings.HasPrefix(cmd.Command, "eslint"):
		return parseESLint(output)
	}
	return nil
}

// goCompilerRe matches `file.go:line:col: message` and `file.go:line: message`
// (the no-column form used by some compiler errors). The column group is
// optional: when absent, m[3] is "" and parseGoCompiler leaves Column as 0.
var goCompilerRe = regexp.MustCompile(`^(.+\.go):(\d+)(?::(\d+))?:\s*(.*)$`)

// parseGoCompiler parses Go compiler / go vet output:
//
//	main.go:10:3: undefined: foo
//	main.go:12: syntax error
func parseGoCompiler(output, source string) []ToolchainDiagnostic {
	var diags []ToolchainDiagnostic
	for _, line := range splitToolchainLines(output) {
		m := goCompilerRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		col := 0
		if m[3] != "" {
			fmt.Sscanf(m[3], "%d", &col)
		}
		var lineNo int
		fmt.Sscanf(m[2], "%d", &lineNo)
		severity := "error"
		if strings.Contains(m[4], "warning") || strings.HasPrefix(line, "warning:") {
			severity = "warning"
		}
		diags = append(diags, ToolchainDiagnostic{
			File:     m[1],
			Line:     lineNo,
			Column:   col,
			Message:  m[4],
			Severity: severity,
			Source:   source,
		})
	}
	return diags
}

// golangciLintRe matches `file.go:line:col: message (linter)`.
var golangciLintRe = regexp.MustCompile(`^(.+\.go):(\d+):(\d+):\s*(.+?)\s+\(([^)]+)\)\s*$`)

// parseGolangciLint parses golangci-lint stylish output:
//
//	main.go:10:3: unused variable `foo` (govet)
func parseGolangciLint(output string) []ToolchainDiagnostic {
	var diags []ToolchainDiagnostic
	for _, line := range splitToolchainLines(output) {
		m := golangciLintRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var lineNo, col int
		fmt.Sscanf(m[2], "%d", &lineNo)
		fmt.Sscanf(m[3], "%d", &col)
		diags = append(diags, ToolchainDiagnostic{
			File:     m[1],
			Line:     lineNo,
			Column:   col,
			Message:  m[4],
			Severity: "warning",
			Source:   "golangci-lint/" + m[5],
		})
	}
	return diags
}

// tsCompilerRe matches `file.ts(line,col): error TS1234: message`. Note:
// there is no colon between the filename and the opening paren.
var tsCompilerRe = regexp.MustCompile(`^(.+\.tsx?)\((\d+),(\d+)\):\s+(error|warning)\s+TS\d+:\s*(.*)$`)

// parseTypeScript parses tsc output:
//
//	src/index.ts(10,3): error TS2322: Type 'string' is not assignable to type 'number'.
func parseTypeScript(output string) []ToolchainDiagnostic {
	var diags []ToolchainDiagnostic
	for _, line := range splitToolchainLines(output) {
		m := tsCompilerRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var lineNo, col int
		fmt.Sscanf(m[2], "%d", &lineNo)
		fmt.Sscanf(m[3], "%d", &col)
		diags = append(diags, ToolchainDiagnostic{
			File:     m[1],
			Line:     lineNo,
			Column:   col,
			Message:  m[5],
			Severity: m[4],
			Source:   "tsc",
		})
	}
	return diags
}

// eslintRe matches `file:line:col: message rule`.
var eslintRe = regexp.MustCompile(`^(.+):(\d+):(\d+):\s+(.+?)\s+([\w-]+(?:/[a-z-]+)?)\s*$`)

// parseESLint parses eslint stylish output:
//
//	src/index.js:10:3: 'foo' is not defined  no-undef
func parseESLint(output string) []ToolchainDiagnostic {
	var diags []ToolchainDiagnostic
	for _, line := range splitToolchainLines(output) {
		m := eslintRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Skip lines where the "file" doesn't look like a source path
		// (avoids matching the summary line "✖ N problems (N errors, M warnings)").
		if !looksLikeSourceFile(m[1]) {
			continue
		}
		var lineNo, col int
		fmt.Sscanf(m[2], "%d", &lineNo)
		fmt.Sscanf(m[3], "%d", &col)
		diags = append(diags, ToolchainDiagnostic{
			File:     m[1],
			Line:     lineNo,
			Column:   col,
			Message:  m[4],
			Severity: "warning",
			Source:   "eslint/" + m[5],
		})
	}
	return diags
}

var sourceExtRe = regexp.MustCompile(`\.(js|mjs|cjs|jsx|ts|mts|cts|tsx|vue|svelte)$`)

func looksLikeSourceFile(s string) bool {
	return sourceExtRe.MatchString(s)
}

// splitToolchainLines splits on \n and \r\n, returning all lines. It is a
// local helper kept distinct from myers_diff.splitLines to avoid a package
// redeclaration conflict.
func splitToolchainLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		// Keep blank lines so indices stay stable if ever needed; the
		// parsers simply won't match them.
		out = append(out, l)
	}
	return out
}
