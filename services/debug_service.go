package services

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// DebugService provides a minimal Delve headless DAP session (prompt-10 10-G).
// MVP: launch package or test under `dlv --headless --api-version=2`, expose
// listen address for attach; Stop kills the process. Full in-IDE variable
// inspection remains future work — this is a demonstrable debug launch path.
type DebugService struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	addr    string
	mode    string // "package" | "test"
	started time.Time
}

// NewDebugService creates the debug service.
func NewDebugService() *DebugService {
	return &DebugService{}
}

// IsAvailable reports whether delve is on PATH.
func (d *DebugService) IsAvailable() bool {
	return lookPathExists("dlv")
}

// StatusMessage returns a user-facing status string.
func (d *DebugService) StatusMessage() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil {
		return fmt.Sprintf("Delve headless running on %s (%s)", d.addr, d.mode)
	}
	if d.IsAvailable() {
		return "Delve available — use Debug Package / Debug Test (headless DAP)"
	}
	return "Delve not installed (go install github.com/go-delve/delve/cmd/dlv@latest)"
}

// DebugSessionInfo is returned after a successful launch.
type DebugSessionInfo struct {
	Running bool   `json:"running"`
	Address string `json:"address"`
	Mode    string `json:"mode"`
	Message string `json:"message"`
}

// IsRunning reports whether a headless session is active.
func (d *DebugService) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil
}

// GetSession returns current session state.
func (d *DebugService) GetSession() DebugSessionInfo {
	d.mu.Lock()
	running := d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil
	addr, mode := d.addr, d.mode
	d.mu.Unlock()
	if running {
		return DebugSessionInfo{
			Running: true,
			Address: addr,
			Mode:    mode,
			Message: fmt.Sprintf("Delve headless running on %s (%s)", addr, mode),
		}
	}
	return DebugSessionInfo{Running: false, Message: d.StatusMessage()}
}

// LaunchPackage starts `dlv debug <dir> --headless --listen=<free> --api-version=2`.
// packageDir should be a directory containing Go sources (often workspace root).
func (d *DebugService) LaunchPackage(packageDir string) (DebugSessionInfo, error) {
	return d.launch(packageDir, "package", nil)
}

// LaunchTest starts `dlv test <dir> --headless ... -- -test.run <runRegex>`.
func (d *DebugService) LaunchTest(packageDir, runRegex string) (DebugSessionInfo, error) {
	extra := []string{}
	if runRegex != "" {
		extra = []string{"--", "-test.run", runRegex}
	}
	return d.launch(packageDir, "test", extra)
}

func (d *DebugService) launch(packageDir, mode string, testArgs []string) (DebugSessionInfo, error) {
	if !d.IsAvailable() {
		return DebugSessionInfo{}, fmt.Errorf("dlv not found on PATH")
	}
	d.mu.Lock()
	if d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil {
		d.mu.Unlock()
		return DebugSessionInfo{}, fmt.Errorf("debug session already running on %s", d.addr)
	}
	d.mu.Unlock()

	abs := packageDir
	if abs == "" {
		abs, _ = os.Getwd()
	}
	if a, err := filepath.Abs(abs); err == nil {
		abs = a
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return DebugSessionInfo{}, fmt.Errorf("package dir invalid: %s", packageDir)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return DebugSessionInfo{}, err
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	dlv, err := exec.LookPath("dlv")
	if err != nil {
		return DebugSessionInfo{}, err
	}

	args := []string{}
	if mode == "test" {
		args = append(args, "test", abs)
	} else {
		args = append(args, "debug", abs)
	}
	args = append(args,
		"--headless",
		"--listen="+addr,
		"--api-version=2",
		"--accept-multiclient",
	)
	args = append(args, testArgs...)

	cmd := exec.Command(dlv, args...)
	cmd.Dir = abs
	// Discard stdio to avoid pipe blocks; logs go to slog.
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return DebugSessionInfo{}, fmt.Errorf("start dlv: %w", err)
	}
	slog.Info("delve launched", "mode", mode, "addr", addr, "dir", abs, "pid", cmd.Process.Pid)

	d.mu.Lock()
	d.cmd = cmd
	d.addr = addr
	d.mode = mode
	d.started = time.Now()
	d.mu.Unlock()

	// Reap process when it exits.
	go func() {
		_ = cmd.Wait()
		d.mu.Lock()
		if d.cmd == cmd {
			d.cmd = nil
			d.addr = ""
			d.mode = ""
		}
		d.mu.Unlock()
		slog.Info("delve exited", "addr", addr)
	}()

	return DebugSessionInfo{
		Running: true,
		Address: addr,
		Mode:    mode,
		Message: fmt.Sprintf("Delve headless DAP listening on %s — attach with a DAP client", addr),
	}, nil
}

// Stop terminates the headless Delve session.
func (d *DebugService) Stop() error {
	d.mu.Lock()
	cmd := d.cmd
	d.cmd = nil
	d.addr = ""
	d.mode = ""
	d.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = cmd.Process.Kill()
	return nil
}
