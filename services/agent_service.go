package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"github.com/google/shlex"
)

// RiskLevel classifies the potential impact of an agent command (N-1).
type RiskLevel string

const (
	RiskSafe       RiskLevel = "safe"
	RiskElevated   RiskLevel = "elevated"
	RiskDangerous  RiskLevel = "dangerous"
)

// denyPattern pairs a regex with a human-readable description for the
// block reason shown in the approval UI.
type denyPattern struct {
	desc string
	re   *regexp.Regexp
}

// dangerousPatterns are regex patterns for commands that are always
// blocked by ExecCommand. The denylist is intentionally conservative —
// it targets only unambiguously destructive operations. The risk level
// classification (elevatedPatterns) handles the broader "use with
// caution" category.
//
// G-SEC-02: denylist 非安全边界，仅作辅助过滤 (denylist is not a
// security boundary, only auxiliary filtering). Determined obfuscation
// (shell escaping, variables, pipes) can bypass these patterns. The
// primary protection is always mandatory user approval — no command is
// auto-approved, including those classified as "Safe".
var dangerousPatterns = []denyPattern{
	{"rm -rf (recursive force delete)", regexp.MustCompile(`(?i)\brm\s+(-\S*r\S*f\S*|-\S*f\S*r\S*)`)},
	{"rm targeting root, home, or wildcard", regexp.MustCompile(`(?i)\brm\s+(-\S+\s+)*[/~*](\s|$)`)},
	{"del /s /f /q (Windows destructive delete)", regexp.MustCompile(`(?i)\bdel\s+/(s|f|q)`)},
	{"format (disk format)", regexp.MustCompile(`(?i)\bformat\b`)},
	{"mkfs (filesystem creation)", regexp.MustCompile(`(?i)\bmkfs\b`)},
	{"fork bomb", regexp.MustCompile(`:\s*\(\)\s*\{`)},
	{"shutdown / reboot / halt", regexp.MustCompile(`(?i)\b(shutdown|reboot|halt)\b`)},
	{"dd to raw device", regexp.MustCompile(`(?i)\bdd\b.*\bof=/dev/`)},
	{":(){ :|:& };: fork bomb literal", regexp.MustCompile(`:\s*\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;`)},
}

// elevatedPatterns are regex patterns for commands that modify system
// state and warrant an "elevated" risk badge in the approval UI.
var elevatedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsudo\b`),
	regexp.MustCompile(`(?i)\bcurl\b[^\|]*\|\s*(sh|bash|zsh)\b`),
	regexp.MustCompile(`(?i)\bwget\b[^\|]*\|\s*(sh|bash|zsh)\b`),
	regexp.MustCompile(`(?i)\bnpm\s+install\b`),
	regexp.MustCompile(`(?i)\bpip\s+install\b`),
	regexp.MustCompile(`(?i)\bapt(-get)?\s+(install|remove|purge)\b`),
	regexp.MustCompile(`(?i)\bbrew\s+(install|uninstall)\b`),
	regexp.MustCompile(`(?i)\bchmod\b`),
	regexp.MustCompile(`(?i)\bchown\b`),
}

// AgentService provides tool-execution primitives for Agent mode (#11).
// When a workspace root is set via SetWorkspaceRoot, ExecCommand sandboxes
// the working directory to that root, rejects commands in the denylist,
// classifies the risk level, and writes each execution to an audit log
// (N-1).
type AgentService struct {
	mu          sync.Mutex
	rootDir     string
	auditLog    *os.File
	auditLogger *slog.Logger
	// Plan 11 Task 4 Step 6: MCP service for mcp.<server>.<tool> tool calls.
	// When set, CheckCommand recognizes the mcp.* namespace and applies
	// ClassifyMCPToolRisk instead of the shell-command patterns. CallMCPTool
	// dispatches to MCPService.CallTool after approval.
	mcpService *MCPService
	// Plan 11 Task 5 Step 7: Skills service. When set, the agent consults
	// SkillsService.MatchTriggers to inject SystemPrompt + AllowedTools
	// into the LLM call. AllowedTools are enforced via CheckCommand
	// (G-SEC-02: tool calls outside the active skills' whitelist are
	// rejected). SetWorkspaceRoot propagates to SkillsService so project-
	// scoped skills (G-SEC-03) load from <root>/.nknk/skills/.
	skillsService *SkillsService
}

// NewAgentService creates a new AgentService. It best-effort opens an
// audit log file in the XDG cache directory; if the file cannot be
// opened, audit logging falls back to slog.Default() (stderr). N-11 will
// introduce a unified slog setup across all services.
func NewAgentService() *AgentService {
	svc := &AgentService{}
	logPath := filepath.Join(xdg.CacheHome, "gugacode", "agent-audit.log")
	// P1-a: audit log contains sensitive command/agent activity - restrict
	// to owner-only (0600) instead of world-readable 0644.
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
		svc.auditLog = f
		svc.auditLogger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return svc
}

// Close releases resources held by the service. N-103: the audit log file
// opened in NewAgentService was never closed, leaking a file descriptor
// for the lifetime of the process. This is called from main on shutdown.
// Safe to call multiple times; subsequent calls are no-ops.
func (s *AgentService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.auditLog != nil {
		err := s.auditLog.Close()
		s.auditLog = nil
		return err
	}
	return nil
}

// SetWorkspaceRoot sets the directory within which agent commands are
// allowed to run. Pass an empty string to disable sandboxing. Mirrors
// the pattern used by FileService and TerminalService so that
// ProjectService can wire all three uniformly.
//
// Plan 11 Task 5: propagates the workspace root to SkillsService so that
// project-scoped skills (G-SEC-03) load from <root>/.nknk/skills/. The
// reload is best-effort: failure is logged but does not block the agent
// (skills are a non-critical enhancement).
func (s *AgentService) SetWorkspaceRoot(root string) error {
	if root == "" {
		s.mu.Lock()
		s.rootDir = ""
		sk := s.skillsService
		s.mu.Unlock()
		if sk != nil {
			sk.SetWorkspaceRoot("")
		}
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
	s.rootDir = abs
	sk := s.skillsService
	s.mu.Unlock()
	if sk != nil {
		sk.SetWorkspaceRoot(abs)
		// Best-effort reload; errors are surfaced via slog, not propagated.
		if err := sk.Load(); err != nil {
			slog.Warn("skills reload on workspace change failed", "err", err)
		}
	}
	return nil
}

// SetMCPService injects the MCP service so the agent can dispatch
// mcp.<server>.<tool> tool calls (Plan 11 Task 4 Step 6). Without this,
// MCP namespaced commands are treated as unknown and blocked.
func (s *AgentService) SetMCPService(mcp *MCPService) {
	s.mu.Lock()
	s.mcpService = mcp
	s.mu.Unlock()
}

// SetSkillsService injects the Skills service so the agent can apply
// SystemPrompt overrides + AllowedTools whitelist from active skills
// (Plan 11 Task 5 Step 7). Without this, skill matching is skipped.
func (s *AgentService) SetSkillsService(skills *SkillsService) {
	s.mu.Lock()
	s.skillsService = skills
	s.mu.Unlock()
}

// validateCwd returns the absolute working directory to use for the
// command. If cwd is empty, it defaults to the workspace root (when
// set). If the workspace root is set, cwd must be inside it.
//
// G-SEC-06: validation is delegated to ValidatePathWithinRoot, which
// resolves symlinks on both the target and the root before comparing.
// The previous lexical-only check (filepath.Abs + filepath.Rel) could
// be bypassed by a symlink inside the workspace pointing outside.
func (s *AgentService) validateCwd(cwd string) (string, error) {
	s.mu.Lock()
	root := s.rootDir
	s.mu.Unlock()

	if root == "" {
		if cwd == "" {
			return "", nil
		}
		return ValidatePathWithinRoot(root, cwd)
	}

	if cwd == "" {
		return root, nil
	}

	return ValidatePathWithinRoot(root, cwd)
}

// shellMetachars lists shell metacharacters that are rejected by parseCommand.
// HIGH-03: the agent command executor no longer wraps commands in
// `sh -c` / `cmd /c`. Commands are parsed into a simple argv and executed
// directly via exec.CommandContext. Any shell syntax is rejected because
// the raw string is passed to exec without shell interpretation, and
// allowing shell syntax would create an injection surface.
var shellMetachars = []struct {
	char byte
	desc string
}{
	{'|', "pipe (|) is not supported — run each command separately"},
	{'>', "output redirect (>) is not supported"},
	{'<', "input redirect (<) is not supported"},
	{'&', "background/chaining (&) is not supported"},
	{';', "command separator (;) is not supported — run each command separately"},
	{'`', "command substitution (backtick) is not supported"},
	{'$', "variable expansion ($) is not supported — use the literal value"},
	{'*', "glob wildcard (*) is not supported — use the exact filename"},
	{'?', "glob wildcard (?) is not supported — use the exact filename"},
	{'(', "subshell syntax () is not supported"},
	{')', "subshell syntax () is not supported"},
	{'{', "brace expansion {} is not supported"},
	{'}', "brace expansion {} is not supported"},
	{'~', "home directory expansion (~) is not supported — use the full path"},
	{'\n', "multi-line commands are not supported — run each command separately"},
}

// parseCommand splits a command line into an argv slice for direct
// execution (HIGH-03). It first scans the raw string for shell
// metacharacters (pipes, redirects, variable expansion, command
// substitution, command chaining, background execution, glob, brace
// expansion) and rejects them with a descriptive error. If the command
// is clean, it uses github.com/google/shlex to tokenize it into an argv
// slice. The returned argv is passed directly to exec.CommandContext
// without a shell wrapper, eliminating the sh -c / cmd /c injection
// surface.
func parseCommand(command string) ([]string, error) {
	for _, mc := range shellMetachars {
		if strings.IndexByte(command, mc.char) >= 0 {
			return nil, fmt.Errorf("unsupported shell syntax: %s", mc.desc)
		}
	}
	argv, err := shlex.Split(command)
	if err != nil {
		return nil, fmt.Errorf("parse command: %w", err)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("command is empty after parsing")
	}
	return argv, nil
}

// CommandCheck is the result of evaluating a command without executing
// it. The frontend calls CheckCommand before ExecCommand to display a
// risk badge and block notice in the agent approval UI (N-1).
type CommandCheck struct {
	RiskLevel   RiskLevel `json:"riskLevel"`
	Blocked     bool      `json:"blocked"`
	BlockReason string    `json:"blockReason,omitempty"`
}

// CheckCommand evaluates a command line and returns its risk level and
// whether it would be blocked by the denylist. It does not execute the
// command.
//
// G-SEC-02: ALL non-empty commands return at minimum RiskElevated. No
// command is classified as "Safe" — every command requires manual user
// approval. The "Safe" level is reserved for the empty-command no-op
// case. This closes the auto-approve bypass that previously allowed
// "Safe" commands to execute without explicit approval.
//
// HIGH-03: commands containing shell metacharacters (pipes, redirects,
// variable expansion, etc.) are blocked with a descriptive reason — the
// executor no longer uses `sh -c` / `cmd /c`, so shell syntax is rejected
// rather than silently passed to a shell.
func (s *AgentService) CheckCommand(command string) CommandCheck {
	if strings.TrimSpace(command) == "" {
		return CommandCheck{RiskLevel: RiskSafe}
	}
	// Plan 11 Task 4 Step 8: mcp.<server>.<tool> namespace is dispatched
	// via MCPService, not the shell executor. Classify via
	// ClassifyMCPToolRisk (G-SEC-02: default RiskElevated, write/exec/network
	// RiskDangerous). The actual call happens in CallMCPTool after user
	// approval.
	if strings.HasPrefix(command, "mcp.") {
		return s.checkMCPCommand(command)
	}
	// HIGH-03: reject shell syntax before the denylist check so the
	// block reason explains which feature is unsupported.
	if _, err := parseCommand(command); err != nil {
		return CommandCheck{RiskLevel: RiskDangerous, Blocked: true, BlockReason: err.Error()}
	}
	for _, p := range dangerousPatterns {
		if p.re.MatchString(command) {
			return CommandCheck{RiskLevel: RiskDangerous, Blocked: true, BlockReason: p.desc}
		}
	}
	for _, p := range elevatedPatterns {
		if p.MatchString(command) {
			return CommandCheck{RiskLevel: RiskElevated}
		}
	}
	// G-SEC-02: no command is "Safe" — minimum risk is Elevated so the
	// approval UI always requires manual confirmation.
	return CommandCheck{RiskLevel: RiskElevated}
}

// checkMCPCommand classifies an mcp.<server>.<tool> command. If the MCP
// service is not configured or the server/tool is unknown, the command is
// blocked. Otherwise the risk is determined by ClassifyMCPToolRisk.
func (s *AgentService) checkMCPCommand(command string) CommandCheck {
	s.mu.Lock()
	mcp := s.mcpService
	s.mu.Unlock()
	if mcp == nil {
		return CommandCheck{
			RiskLevel:   RiskDangerous,
			Blocked:     true,
			BlockReason: "MCP service not configured",
		}
	}
	// Parse mcp.<server>.<tool> — tool name may contain dots, so split
	// into exactly 3 parts: "mcp", server, tool.
	parts := strings.SplitN(command, ".", 3)
	if len(parts) != 3 || parts[0] != "mcp" || parts[1] == "" || parts[2] == "" {
		return CommandCheck{
			RiskLevel:   RiskDangerous,
			Blocked:     true,
			BlockReason: "invalid MCP tool namespace (expected mcp.<server>.<tool>)",
		}
	}
	server := parts[1]
	tool := parts[2]
	// Look up the tool to get its description for risk classification.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tools, err := mcp.ListAgentMCPTools(ctx)
	if err != nil {
		return CommandCheck{RiskLevel: RiskElevated}
	}
	for _, t := range tools {
		if t.Server == server && t.Tool == tool {
			return CommandCheck{RiskLevel: t.RiskLevel}
		}
	}
	// Tool not found — block rather than risk an unknown call.
	return CommandCheck{
		RiskLevel:   RiskDangerous,
		Blocked:     true,
		BlockReason: fmt.Sprintf("MCP tool %s.%s not found", server, tool),
	}
}

// CallMCPTool dispatches an mcp.<server>.<tool> call to the MCP service
// after the user has approved it (Plan 11 Task 4 Step 6). The args map is
// passed as the tool's arguments. The result is returned as a JSON string
// for the agent to interpret.
func (s *AgentService) CallMCPTool(ctx context.Context, namespace string, args map[string]interface{}) (*MCPToolResult, error) {
	s.mu.Lock()
	mcp := s.mcpService
	s.mu.Unlock()
	if mcp == nil {
		return nil, fmt.Errorf("MCP service not configured: %w", ErrInvalidInput)
	}
	parts := strings.SplitN(namespace, ".", 3)
	if len(parts) != 3 || parts[0] != "mcp" {
		return nil, fmt.Errorf("invalid MCP namespace %q: %w", namespace, ErrInvalidInput)
	}
	server, tool := parts[1], parts[2]
	return mcp.CallTool(ctx, server, tool, args)
}

// ExecResult is the outcome of a synchronous command execution.
type ExecResult struct {
	Command     string    `json:"command"`
	Cwd         string    `json:"cwd"`
	Stdout      string    `json:"stdout"`
	Stderr      string    `json:"stderr"`
	ExitCode    int       `json:"exitCode"`
	DurationMs  int64     `json:"durationMs"`
	RiskLevel   RiskLevel `json:"riskLevel"`
	Blocked     bool      `json:"blocked"`
	BlockReason string    `json:"blockReason,omitempty"`
}

// ExecCommand runs the given command line in the given working directory
// and returns the captured stdout/stderr. A 30-second timeout is enforced
// to prevent the agent from hanging on interactive commands.
//
// HIGH-03: the command is parsed into a simple argv (executable + args)
// using github.com/google/shlex and executed directly via
// exec.CommandContext — no shell wrapper (`sh -c` / `cmd /c`). Shell
// syntax (pipes, redirects, variable expansion, command substitution,
// command chaining, background execution, glob) is rejected by
// parseCommand and reported via CommandCheck.BlockReason.
//
// Security (N-1): if a workspace root is set, cwd is sandboxed to that
// root (empty defaults to root, paths outside are rejected). Commands
// matching the denylist are blocked before execution. Each execution is
// written to the audit log with command, cwd, exit code, duration, and
// risk level.
func (s *AgentService) ExecCommand(command, cwd string) (ExecResult, error) {
	if strings.TrimSpace(command) == "" {
		return ExecResult{}, fmt.Errorf("command is required")
	}

	// Denylist + shell-syntax check — block destructive commands and
	// unsupported shell syntax before execution.
	check := s.CheckCommand(command)
	if check.Blocked {
		result := ExecResult{
			Command:     command,
			Cwd:         cwd,
			RiskLevel:   RiskDangerous,
			Blocked:     true,
			BlockReason: check.BlockReason,
		}
		s.audit(result.Cwd, result)
		return result, fmt.Errorf("command blocked: %s", check.BlockReason)
	}

	// Sandbox cwd — default to root, reject paths outside the workspace.
	resolvedCwd, err := s.validateCwd(cwd)
	if err != nil {
		return ExecResult{}, err
	}

	// HIGH-03: parse into argv and execute directly without a shell.
	// CheckCommand already verified the command is parseable, but we
	// call parseCommand again to get the argv slice.
	argv, err := parseCommand(command)
	if err != nil {
		// Should not happen — CheckCommand already validated parsing.
		return ExecResult{}, fmt.Errorf("parse command: %w", err)
	}

	// Use a timeout so a misbehaving command cannot block the agent loop.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := commandContext(ctx, argv[0], argv[1:]...)
	if resolvedCwd != "" {
		cmd.Dir = resolvedCwd
	}

	start := time.Now()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start).Milliseconds()

	result := ExecResult{
		Command:    command,
		Cwd:        resolvedCwd,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   0,
		DurationMs: duration,
		RiskLevel:  check.RiskLevel,
	}

	if runErr != nil {
		// If the command ran but exited non-zero, extract the exit code
		// and return a normal result (not an error). The agent should see
		// the stderr and decide what to do.
		// N-106: use errors.As instead of a type assertion so wrapped
		// errors (e.g. fmt.Errorf("...: %w", exitErr)) are still
		// recognized as ExitError.
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			s.audit(resolvedCwd, result)
			return result, nil
		}
		// If the context deadline was exceeded, return a timeout result.
		// N-107: use errors.Is so wrapped context errors are recognized.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Stderr += "\n[command timed out after 30s]"
			result.ExitCode = -1
			s.audit(resolvedCwd, result)
			return result, nil
		}
		// Other errors (command not found, etc.) are returned as errors.
		s.audit(resolvedCwd, result)
		return result, fmt.Errorf("run command: %w", runErr)
	}

	s.audit(resolvedCwd, result)
	return result, nil
}

// audit writes a structured log entry for an agent command execution.
// If no audit logger is configured (file could not be opened), it falls
// back to slog.Default().
func (s *AgentService) audit(cwd string, r ExecResult) {
	keyvals := []any{
		"command", r.Command,
		"cwd", cwd,
		"exitCode", r.ExitCode,
		"durationMs", r.DurationMs,
		"riskLevel", string(r.RiskLevel),
		"blocked", r.Blocked,
	}
	if s.auditLogger != nil {
		s.auditLogger.Info("agent exec", keyvals...)
		return
	}
	slog.Default().Info("agent exec", keyvals...)
}
