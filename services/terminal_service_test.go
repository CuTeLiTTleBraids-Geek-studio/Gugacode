package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// skipIfNoConsole skips tests that require a real PTY/ConPTY (N-6).
// On Windows, CreatePseudoConsole returns E_HANDLE (0x80070006) when
// stdout is redirected (e.g. by test runners or CI), because ConPTY
// needs a real console host. On Unix, PTY creation can also fail in
// headless environments. We detect this by checking (1) the CI env
// var and (2) whether stdout is a character device.
func skipIfNoConsole(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Skip("skipping PTY/ConPTY test in CI environment (N-6)")
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		t.Skip("skipping PTY/ConPTY test: cannot stat stdout (N-6)")
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		t.Skip("skipping PTY/ConPTY test: stdout is not a console (N-6)")
	}
}

func TestTerminalService_StartAndRead(t *testing.T) {
	skipIfNoConsole(t)
	ts := NewTerminalService()
	defer ts.Kill()

	if err := ts.Start(""); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if ts.IsRunning() != true {
		t.Error("expected IsRunning() to be true after Start")
	}

	// Send a command and wait for output
	ts.Write("echo hello_pty\n")

	// Poll for the expected output — PowerShell may emit its banner first
	// and the echo output arrives a bit later.
	deadline := time.Now().Add(5 * time.Second)
	var output string
	for time.Now().Before(deadline) {
		output += ts.ReadOutput(500 * time.Millisecond)
		if strings.Contains(output, "hello_pty") {
			break
		}
	}
	if !strings.Contains(output, "hello_pty") {
		t.Errorf("expected output to contain 'hello_pty', got: %q", output)
	}
}

func TestTerminalService_Kill(t *testing.T) {
	skipIfNoConsole(t)
	ts := NewTerminalService()
	if err := ts.Start(""); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	ts.Kill()

	if ts.IsRunning() != false {
		t.Error("expected IsRunning() to be false after Kill")
	}
}

func TestTerminalService_WriteWhenNotRunning(t *testing.T) {
	ts := NewTerminalService()
	err := ts.Write("test")
	if err == nil {
		t.Error("expected error when writing to non-running terminal")
	}
}

func TestTerminalService_Resize(t *testing.T) {
	skipIfNoConsole(t)
	ts := NewTerminalService()
	defer ts.Kill()

	if err := ts.Start(""); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	err := ts.Resize(80, 24)
	if err != nil {
		t.Errorf("Resize failed: %v", err)
	}
}

func TestTerminalService_ResizeWhenNotRunning(t *testing.T) {
	ts := NewTerminalService()
	err := ts.Resize(80, 24)
	if err == nil {
		t.Error("expected error when resizing non-running terminal")
	}
}

func TestTerminalService_StartWithInvalidWorkingDir(t *testing.T) {
	ts := NewTerminalService()
	defer ts.Kill()
	err := ts.Start("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for non-existent working directory")
	}
}

func TestTerminalService_StartWithFileAsWorkingDir(t *testing.T) {
	ts := NewTerminalService()
	defer ts.Kill()
	// Pass a file path instead of a directory — should fail.
	err := ts.Start("terminal_service.go")
	if err == nil {
		t.Error("expected error when working directory is a file")
	}
}

func TestTerminalService_ValidateWorkingDir_RejectsOutsideWorkspace(t *testing.T) {
	svc := NewTerminalService()
	svc.SetWorkspaceRoot(t.TempDir())
	err := svc.validateWorkingDir(t.TempDir())
	if err == nil {
		t.Fatal("expected error for workingDir outside workspace")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("error should mention 'outside', got: %v", err)
	}
}

func TestTerminalService_ValidateWorkingDir_AcceptsInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	subDir := filepath.Join(workspace, "subdir")
	os.MkdirAll(subDir, 0755)

	svc := NewTerminalService()
	svc.SetWorkspaceRoot(workspace)
	err := svc.validateWorkingDir(subDir)
	if err != nil {
		t.Fatalf("expected no error for path inside workspace, got: %v", err)
	}
}

func TestTerminalService_ValidateWorkingDir_NoRootAllowsAny(t *testing.T) {
	svc := NewTerminalService()
	// No workspace root set — any directory should be allowed
	err := svc.validateWorkingDir(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error when no workspace root set, got: %v", err)
	}
}

func TestTerminalService_ValidateWorkingDir_EmptyAllowed(t *testing.T) {
	svc := NewTerminalService()
	svc.SetWorkspaceRoot(t.TempDir())
	err := svc.validateWorkingDir("")
	if err != nil {
		t.Fatalf("expected no error for empty workingDir, got: %v", err)
	}
}

func TestTerminalService_MultiSession(t *testing.T) {
	skipIfNoConsole(t)
	ts := NewTerminalService()
	defer ts.KillSession("s1")
	defer ts.KillSession("s2")

	if err := ts.StartSession("s1", "", ""); err != nil {
		t.Fatalf("StartSession s1 failed: %v", err)
	}
	if err := ts.StartSession("s2", "", ""); err != nil {
		t.Fatalf("StartSession s2 failed: %v", err)
	}

	sessions := ts.ListSessions()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}

	if !ts.IsSessionRunning("s1") || !ts.IsSessionRunning("s2") {
		t.Error("expected both sessions to be running")
	}

	// Kill one session
	if err := ts.KillSession("s1"); err != nil {
		t.Fatalf("KillSession s1 failed: %v", err)
	}

	if ts.IsSessionRunning("s1") {
		t.Error("expected s1 to not be running after kill")
	}
	if !ts.IsSessionRunning("s2") {
		t.Error("expected s2 to still be running")
	}

	sessions = ts.ListSessions()
	if len(sessions) != 1 {
		t.Errorf("expected 1 session after kill, got %d", len(sessions))
	}
}

func TestTerminalService_DuplicateSession(t *testing.T) {
	skipIfNoConsole(t)
	ts := NewTerminalService()
	defer ts.KillSession("dup")

	if err := ts.StartSession("dup", "", ""); err != nil {
		t.Fatalf("first StartSession failed: %v", err)
	}
	err := ts.StartSession("dup", "", "")
	if err == nil {
		t.Error("expected error starting duplicate session")
	}
}

func TestTerminalService_EmptySessionID(t *testing.T) {
	ts := NewTerminalService()
	err := ts.StartSession("", "", "")
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

func TestTerminalService_WriteSessionWhenNotRunning(t *testing.T) {
	ts := NewTerminalService()
	err := ts.WriteSession("nonexistent", "test")
	if err == nil {
		t.Error("expected error when writing to non-existent session")
	}
}

func TestTerminalService_ResizeSessionWhenNotRunning(t *testing.T) {
	ts := NewTerminalService()
	err := ts.ResizeSession("nonexistent", 80, 24)
	if err == nil {
		t.Error("expected error when resizing non-existent session")
	}
}

func TestTerminalService_KillSessionNotFound(t *testing.T) {
	ts := NewTerminalService()
	err := ts.KillSession("nonexistent")
	if err == nil {
		t.Error("expected error when killing non-existent session")
	}
}
