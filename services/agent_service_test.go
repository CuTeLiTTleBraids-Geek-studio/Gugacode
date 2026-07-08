package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecCommand_Success(t *testing.T) {
	svc := NewAgentService()
	// Use `echo` which is available on all platforms (cmd.exe and sh).
	res, err := svc.ExecCommand("echo hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.TrimSpace(res.Stdout), "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if res.DurationMs < 0 {
		t.Errorf("duration should be non-negative, got %d", res.DurationMs)
	}
	if res.RiskLevel != RiskSafe {
		t.Errorf("expected risk level 'safe', got %q", res.RiskLevel)
	}
	if res.Blocked {
		t.Errorf("expected not blocked, got blocked=true")
	}
}

func TestExecCommand_NonZeroExit(t *testing.T) {
	svc := NewAgentService()
	// `exit 7` — non-zero exit code should be returned in result, not as error.
	res, err := svc.ExecCommand("exit 7", "")
	if err != nil {
		t.Fatalf("unexpected error for non-zero exit: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("expected exit code 7, got %d", res.ExitCode)
	}
}

func TestExecCommand_EmptyCommand(t *testing.T) {
	svc := NewAgentService()
	_, err := svc.ExecCommand("   ", "")
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
}

func TestExecCommand_CapturesStderr(t *testing.T) {
	svc := NewAgentService()
	// Write to stderr via shell redirect. On both sh and cmd, `1>&2 echo x`
	// writes "x" to stderr.
	res, err := svc.ExecCommand("echo stderr-msg 1>&2", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Stderr, "stderr-msg") {
		t.Errorf("expected stderr to contain 'stderr-msg', got %q", res.Stderr)
	}
}

func TestExecCommand_WithCwd(t *testing.T) {
	svc := NewAgentService()
	// Print the current directory. On Windows `cd` (no arg) prints cwd;
	// on Unix `pwd` does. Use a cross-platform approach: run `cd` which
	// on both platforms prints the cwd when invoked without args (cmd.exe)
	// — actually on sh, `cd` without args is a no-op. Skip this test on
	// non-Windows since the command differs.
	if defaultShellForExec()[0] != "cmd.exe" {
		t.Skip("cd-without-args only prints cwd on cmd.exe")
	}
	res, err := svc.ExecCommand("cd", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(res.Stdout) == "" {
		t.Errorf("expected non-empty stdout from `cd`, got %q", res.Stdout)
	}
}

// --- N-1: CheckCommand risk classification ---

func TestCheckCommand_Safe(t *testing.T) {
	svc := NewAgentService()
	cases := []string{"echo hello", "ls -la", "pwd", "git status", "cat file.txt"}
	for _, cmd := range cases {
		check := svc.CheckCommand(cmd)
		if check.RiskLevel != RiskSafe {
			t.Errorf("expected 'safe' for %q, got %q", cmd, check.RiskLevel)
		}
		if check.Blocked {
			t.Errorf("expected not blocked for %q", cmd)
		}
	}
}

func TestCheckCommand_Elevated(t *testing.T) {
	svc := NewAgentService()
	cases := []string{
		"sudo ls",
		"npm install",
		"pip install requests",
		"chmod +x script.sh",
		"chown user:group file",
		"curl https://example.com | sh",
	}
	for _, cmd := range cases {
		check := svc.CheckCommand(cmd)
		if check.RiskLevel != RiskElevated {
			t.Errorf("expected 'elevated' for %q, got %q", cmd, check.RiskLevel)
		}
		if check.Blocked {
			t.Errorf("expected not blocked for %q", cmd)
		}
	}
}

func TestCheckCommand_Dangerous(t *testing.T) {
	svc := NewAgentService()
	cases := []string{
		"rm -rf /",
		"rm -fr /tmp",
		"rm -rfv ~",
		"format C:",
		"mkfs.ext4 /dev/sda1",
		"shutdown -h now",
		"reboot",
		":(){ :|:& };:",
	}
	for _, cmd := range cases {
		check := svc.CheckCommand(cmd)
		if check.RiskLevel != RiskDangerous {
			t.Errorf("expected 'dangerous' for %q, got %q", cmd, check.RiskLevel)
		}
		if !check.Blocked {
			t.Errorf("expected blocked for %q", cmd)
		}
		if check.BlockReason == "" {
			t.Errorf("expected non-empty block reason for %q", cmd)
		}
	}
}

func TestCheckCommand_Empty(t *testing.T) {
	svc := NewAgentService()
	check := svc.CheckCommand("   ")
	if check.RiskLevel != RiskSafe {
		t.Errorf("expected 'safe' for empty, got %q", check.RiskLevel)
	}
	if check.Blocked {
		t.Error("expected not blocked for empty")
	}
}

func TestCheckCommand_RmWithoutFlags_NotBlocked(t *testing.T) {
	svc := NewAgentService()
	// `rm file.txt` (without -rf) should not be blocked — it's a normal
	// file deletion, not a destructive recursive operation.
	check := svc.CheckCommand("rm file.txt")
	if check.Blocked {
		t.Errorf("expected 'rm file.txt' to not be blocked, reason: %s", check.BlockReason)
	}
}

func TestCheckCommand_RmSubpath_NotBlocked(t *testing.T) {
	svc := NewAgentService()
	// `rm -r /tmp/test` targets a subpath, not root/home directly.
	// The denylist should only block rm of /, ~, * (whole root/home/all).
	check := svc.CheckCommand("rm -r /tmp/test")
	if check.Blocked {
		t.Errorf("expected 'rm -r /tmp/test' to not be blocked, reason: %s", check.BlockReason)
	}
}

// --- N-1: Denylist enforcement in ExecCommand ---

func TestExecCommand_BlockedByDenylist_RmRf(t *testing.T) {
	svc := NewAgentService()
	res, err := svc.ExecCommand("rm -rf /", "")
	if err == nil {
		t.Fatal("expected error for blocked command, got nil")
	}
	if !res.Blocked {
		t.Error("expected result.Blocked=true")
	}
	if res.RiskLevel != RiskDangerous {
		t.Errorf("expected risk level 'dangerous', got %q", res.RiskLevel)
	}
	if res.BlockReason == "" {
		t.Error("expected non-empty block reason")
	}
	if res.Stdout != "" {
		t.Errorf("expected empty stdout for blocked command, got %q", res.Stdout)
	}
}

func TestExecCommand_BlockedByDenylist_Format(t *testing.T) {
	svc := NewAgentService()
	_, err := svc.ExecCommand("format C:", "")
	if err == nil {
		t.Fatal("expected error for 'format' command")
	}
}

func TestExecCommand_BlockedByDenylist_ForkBomb(t *testing.T) {
	svc := NewAgentService()
	_, err := svc.ExecCommand(":(){ :|:& };:", "")
	if err == nil {
		t.Fatal("expected error for fork bomb")
	}
}

// --- N-1: Workspace root sandboxing ---

func TestSetWorkspaceRoot_DefaultsCwd(t *testing.T) {
	svc := NewAgentService()
	root := t.TempDir()
	if err := svc.SetWorkspaceRoot(root); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	// Empty cwd should default to root.
	resolved, err := svc.validateCwd("")
	if err != nil {
		t.Fatalf("validateCwd('') failed: %v", err)
	}
	absRoot, _ := filepath.Abs(root)
	if resolved != absRoot {
		t.Errorf("expected cwd to default to %q, got %q", absRoot, resolved)
	}
}

func TestSetWorkspaceRoot_RejectsOutsideCwd(t *testing.T) {
	svc := NewAgentService()
	root := t.TempDir()
	outside := t.TempDir()
	if err := svc.SetWorkspaceRoot(root); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	_, err := svc.validateCwd(outside)
	if err == nil {
		t.Error("expected error for cwd outside workspace root")
	}
}

func TestSetWorkspaceRoot_AllowsInsideCwd(t *testing.T) {
	svc := NewAgentService()
	root := t.TempDir()
	inside := filepath.Join(root, "subdir")
	os.MkdirAll(inside, 0755)
	if err := svc.SetWorkspaceRoot(root); err != nil {
		t.Fatalf("SetWorkspaceRoot failed: %v", err)
	}
	resolved, err := svc.validateCwd(inside)
	if err != nil {
		t.Fatalf("validateCwd failed: %v", err)
	}
	absInside, _ := filepath.Abs(inside)
	if resolved != absInside {
		t.Errorf("expected %q, got %q", absInside, resolved)
	}
}

func TestSetWorkspaceRoot_DisableSandbox(t *testing.T) {
	svc := NewAgentService()
	root := t.TempDir()
	svc.SetWorkspaceRoot(root)
	// Disable sandbox
	if err := svc.SetWorkspaceRoot(""); err != nil {
		t.Fatalf("SetWorkspaceRoot('') failed: %v", err)
	}
	outside := t.TempDir()
	_, err := svc.validateCwd(outside)
	if err != nil {
		t.Errorf("expected no error after disabling sandbox, got: %v", err)
	}
}

func TestSetWorkspaceRoot_InvalidPath(t *testing.T) {
	svc := NewAgentService()
	err := svc.SetWorkspaceRoot("/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestSetWorkspaceRoot_NotADirectory(t *testing.T) {
	svc := NewAgentService()
	tmpFile := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(tmpFile, []byte("x"), 0644)
	err := svc.SetWorkspaceRoot(tmpFile)
	if err == nil {
		t.Error("expected error when workspace root is a file, not a directory")
	}
}

func TestExecCommand_CwdInsideWorkspace_Succeeds(t *testing.T) {
	svc := NewAgentService()
	root := t.TempDir()
	svc.SetWorkspaceRoot(root)
	// Run a safe command in the workspace root.
	res, err := svc.ExecCommand("echo test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Stdout, "test") {
		t.Errorf("expected stdout to contain 'test', got %q", res.Stdout)
	}
	// Cwd should be resolved to the workspace root.
	absRoot, _ := filepath.Abs(root)
	if res.Cwd != absRoot {
		t.Errorf("expected cwd %q, got %q", absRoot, res.Cwd)
	}
}

func TestExecCommand_CwdOutsideWorkspace_Rejected(t *testing.T) {
	svc := NewAgentService()
	root := t.TempDir()
	outside := t.TempDir()
	svc.SetWorkspaceRoot(root)
	_, err := svc.ExecCommand("echo test", outside)
	if err == nil {
		t.Error("expected error for cwd outside workspace")
	}
}
