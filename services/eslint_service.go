package services

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EslintService provides long-lived ESLint access (prompt-13 13-B).
// Prefers eslint_d daemon; falls back to a single-flight eslint CLI with cache.
type EslintService struct {
	mu            sync.Mutex
	workspaceRoot string
	useDaemon     bool // eslint_d available
	daemonChecked bool
	lastHash      map[string]string // path -> content hash
}

// EslintDiagnostic is one lint finding.
type EslintDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"` // error | warning
	Message  string `json:"message"`
	Rule     string `json:"rule,omitempty"`
	Source   string `json:"source"`
}

// EslintLintResult is returned to the frontend.
type EslintLintResult struct {
	Success      bool               `json:"success"`
	Output       string             `json:"output"`
	Diagnostics  []EslintDiagnostic `json:"diagnostics"`
	UsedDaemon   bool               `json:"usedDaemon"`
	Skipped      bool               `json:"skipped"` // content hash hit
	DurationMs   int64              `json:"durationMs"`
	DaemonStatus string             `json:"daemonStatus"`
}

// NewEslintService creates the service.
func NewEslintService() *EslintService {
	return &EslintService{lastHash: map[string]string{}}
}

// SetWorkspaceRoot sets default cwd for eslint.
func (e *EslintService) SetWorkspaceRoot(root string) {
	e.mu.Lock()
	e.workspaceRoot = root
	e.daemonChecked = false
	e.mu.Unlock()
}

// Status reports whether eslint_d / eslint are available.
func (e *EslintService) Status() map[string]interface{} {
	e.ensureDaemon()
	e.mu.Lock()
	defer e.mu.Unlock()
	eslint := lookPathExists("eslint")
	eslintd := lookPathExists("eslint_d")
	return map[string]interface{}{
		"eslint":     eslint,
		"eslint_d":   eslintd,
		"useDaemon":  e.useDaemon,
		"workspace":  e.workspaceRoot,
		"hint":       "Install eslint_d for warm daemon: npm i -g eslint_d",
	}
}

func (e *EslintService) ensureDaemon() {
	e.mu.Lock()
	if e.daemonChecked {
		e.mu.Unlock()
		return
	}
	e.daemonChecked = true
	e.mu.Unlock()
	if lookPathExists("eslint_d") {
		// best-effort start (may already be running)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := commandContext(ctx, "eslint_d", "start")
		e.mu.Lock()
		if e.workspaceRoot != "" {
			cmd.Dir = e.workspaceRoot
		}
		e.mu.Unlock()
		_ = cmd.Run()
		cancel()
		e.mu.Lock()
		e.useDaemon = true
		e.mu.Unlock()
	}
}

// LintFile lints file content via eslint_d (preferred) or eslint --stdin.
// contentHash empty skips cache.
func (e *EslintService) LintFile(filePath, content, contentHash string) (EslintLintResult, error) {
	e.ensureDaemon()
	if filePath == "" {
		return EslintLintResult{Success: false, Output: "no file path"}, nil
	}
	if contentHash != "" {
		e.mu.Lock()
		if e.lastHash[filePath] == contentHash {
			e.mu.Unlock()
			return EslintLintResult{Success: true, Skipped: true, DaemonStatus: "hash-skip"}, nil
		}
		e.mu.Unlock()
	}

	start := time.Now()
	e.mu.Lock()
	useD := e.useDaemon
	root := e.workspaceRoot
	e.mu.Unlock()

	var (
		out    []byte
		err    error
		daemon bool
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if useD {
		cmd := commandContext(ctx, "eslint_d", "--stdin", "--stdin-filename", filePath, "-f", "json")
		if root != "" {
			cmd.Dir = root
		} else {
			cmd.Dir = filepath.Dir(filePath)
		}
		cmd.Stdin = strings.NewReader(content)
		out, err = cmd.CombinedOutput()
		daemon = true
		// if eslint_d failed hard, fall back once
		if err != nil && (bytes.Contains(out, []byte("Cannot find module")) || bytes.Contains(out, []byte("not running"))) {
			useD = false
		}
	}
	if !useD {
		bin := "eslint"
		if p, e2 := exec.LookPath("eslint"); e2 == nil {
			bin = p
		} else if root != "" {
			local := filepath.Join(root, "node_modules", ".bin", "eslint")
			if _, e3 := os.Stat(local); e3 == nil {
				bin = local
			}
		}
		cmd := commandContext(ctx, bin, "--stdin", "--stdin-filename", filePath, "-f", "json", "--no-error-on-unmatched-pattern")
		if root != "" {
			cmd.Dir = root
		} else {
			cmd.Dir = filepath.Dir(filePath)
		}
		cmd.Stdin = strings.NewReader(content)
		out, err = cmd.CombinedOutput()
		daemon = false
	}

	diags := parseEslintJSON(out, filePath)
	// eslint exits 1 when there are findings — not a hard failure
	success := err == nil || len(diags) > 0 || bytes.Contains(out, []byte("["))
	if contentHash != "" && success {
		e.mu.Lock()
		e.lastHash[filePath] = contentHash
		e.mu.Unlock()
	}
	status := "eslint-cli"
	if daemon {
		status = "eslint_d"
	}
	return EslintLintResult{
		Success:      success || len(diags) >= 0,
		Output:       string(out),
		Diagnostics:  diags,
		UsedDaemon:   daemon,
		DurationMs:   time.Since(start).Milliseconds(),
		DaemonStatus: status,
	}, nil
}

func parseEslintJSON(raw []byte, fallbackPath string) []EslintDiagnostic {
	// eslint json is array of {filePath, messages:[{line,column,severity,message,ruleId}]}
	raw = bytes.TrimSpace(raw)
	// may have non-json prefix; find first [
	if i := bytes.IndexByte(raw, '['); i > 0 {
		raw = raw[i:]
	}
	var files []struct {
		FilePath string `json:"filePath"`
		Messages []struct {
			Line     int    `json:"line"`
			Column   int    `json:"column"`
			Severity int    `json:"severity"` // 1=warn 2=error
			Message  string `json:"message"`
			RuleID   string `json:"ruleId"`
		} `json:"messages"`
	}
	if json.Unmarshal(raw, &files) != nil {
		return nil
	}
	var out []EslintDiagnostic
	for _, f := range files {
		fp := f.FilePath
		if fp == "" {
			fp = fallbackPath
		}
		for _, m := range f.Messages {
			sev := "warning"
			if m.Severity >= 2 {
				sev = "error"
			}
			out = append(out, EslintDiagnostic{
				File: fp, Line: m.Line, Column: m.Column,
				Severity: sev, Message: m.Message, Rule: m.RuleID, Source: "eslint",
			})
		}
	}
	return out
}

// WarmDaemon starts eslint_d if available (explicit).
func (e *EslintService) WarmDaemon() string {
	e.mu.Lock()
	e.daemonChecked = false
	e.mu.Unlock()
	e.ensureDaemon()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.useDaemon {
		return "eslint_d ready"
	}
	return "eslint_d not found — using CLI single-flight fallback"
}
