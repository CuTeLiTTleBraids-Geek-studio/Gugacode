package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TerminalSession represents a single PTY terminal session.
type TerminalSession struct {
	id         string
	conn       io.ReadWriteCloser
	outputBuf  *outputBuffer
	running    bool
	workingDir string
	shell      []string
}

// TerminalService manages multiple terminal sessions.
//
// N-94 / Proposal AB: session.running is protected by t.mu. All reads and
// writes of session.running happen while holding t.mu, eliminating the
// TOCTOU race where WriteSession/ResizeSession/IsSessionRunning observed
// a stale value after releasing the lock.
//
// N-95 / Proposal AC: ctx and cancel provide a cancellation mechanism for
// readLoop goroutines. Shutdown() cancels the context and closes all
// session conns, which unblocks any pending Read calls, then waits for
// all goroutines to exit via wg.
type TerminalService struct {
	mu       sync.Mutex
	sessions map[string]*TerminalSession
	rootDir  string
	app      *application.App
	// N-95: ctx is cancelled by Shutdown() to signal all readLoop goroutines
	// to exit. cancel is the function that cancels ctx.
	ctx    context.Context
	cancel context.CancelFunc
	// N-95: wg tracks all active readLoop goroutines so Shutdown() can wait
	// for them to exit before returning.
	wg sync.WaitGroup
}

func NewTerminalService() *TerminalService {
	ctx, cancel := context.WithCancel(context.Background())
	return &TerminalService{
		sessions: make(map[string]*TerminalSession),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// SetApp links the Wails app for event emission.
func (t *TerminalService) SetApp(app *application.App) {
	t.mu.Lock()
	t.app = app
	t.mu.Unlock()
}

// SetWorkspaceRoot sets the directory within which terminal sessions are allowed.
// Pass an empty string to disable sandboxing.
func (t *TerminalService) SetWorkspaceRoot(root string) error {
	if root == "" {
		t.mu.Lock()
		t.rootDir = ""
		t.mu.Unlock()
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
	t.mu.Lock()
	t.rootDir = abs
	t.mu.Unlock()
	return nil
}

// validateWorkingDir checks that the path is within the workspace root (if set).
//
// G-SEC-06: validation is delegated to ValidatePathWithinRoot, which
// resolves symlinks on both the target and the root before comparing.
// The previous lexical-only check (filepath.Abs + filepath.Rel) could
// be bypassed by a symlink inside the workspace pointing outside.
func (t *TerminalService) validateWorkingDir(workingDir string) error {
	if workingDir == "" {
		return nil
	}
	t.mu.Lock()
	root := t.rootDir
	t.mu.Unlock()
	if root == "" {
		return nil
	}
	_, err := ValidatePathWithinRoot(root, workingDir)
	return err
}

// StartSession creates and starts a new terminal session with the given ID.
// allowedShells is the whitelist of shell base names that can be used in
// StartSession (M-4). The check is on the base name of the shell path, so
// "/usr/bin/bash" and "bash" both match. On Windows, the comparison is
// case-insensitive (and the .exe suffix is stripped).
var allowedShells = map[string]bool{
	"bash":       true,
	"sh":         true,
	"zsh":        true,
	"powershell": true,
	"pwsh":       true,
	"cmd":        true,
}

// isAllowedShell returns true if the shell's base name (with .exe stripped
// on Windows) is in the allowedShells whitelist. This prevents the frontend
// from launching an arbitrary binary as a terminal shell (M-4).
func isAllowedShell(shell string) bool {
	base := filepath.Base(shell)
	// HIGH-01: lowercase before trimming .exe so "CMD.EXE" / "PowerShell.exe"
	// normalize correctly (previously TrimSuffix(".exe") missed uppercase).
	base = strings.ToLower(base)
	base = strings.TrimSuffix(base, ".exe")
	return allowedShells[base]
}

func (t *TerminalService) StartSession(id string, workingDir string, shell string) error {
	if id == "" {
		return fmt.Errorf("session ID cannot be empty")
	}
	if err := t.validateWorkingDir(workingDir); err != nil {
		slog.Warn("terminal: invalid working dir", "sessionId", id, "workingDir", workingDir, "err", err)
		return err
	}

	t.mu.Lock()
	if _, exists := t.sessions[id]; exists {
		t.mu.Unlock()
		return fmt.Errorf("session %s already exists", id)
	}
	t.mu.Unlock()

	if workingDir != "" {
		info, err := os.Stat(workingDir)
		if err != nil {
			slog.Warn("terminal: working dir stat failed", "sessionId", id, "workingDir", workingDir, "err", err)
			return fmt.Errorf("invalid working directory: %w", err)
		}
		if !info.IsDir() {
			slog.Warn("terminal: working dir not a directory", "sessionId", id, "workingDir", workingDir)
			return fmt.Errorf("working directory is not a directory: %s", workingDir)
		}
	}

	resolvedShell := defaultShell()
	if shell != "" {
		if !isAllowedShell(shell) {
			slog.Warn("terminal: rejected shell not in whitelist", "sessionId", id, "shell", shell)
			return fmt.Errorf("shell %q is not in the allowed list (M-4: bash/sh/zsh/powershell/pwsh/cmd)", shell)
		}
		resolvedShell = []string{shell}
	}

	conn, err := startPty(resolvedShell, workingDir)
	if err != nil {
		slog.Error("terminal: pty start failed", "sessionId", id, "shell", resolvedShell, "err", err)
		return err
	}

	session := &TerminalSession{
		id:         id,
		conn:       conn,
		outputBuf:  newOutputBuffer(),
		running:    true,
		workingDir: workingDir,
		shell:      resolvedShell,
	}

	t.mu.Lock()
	t.sessions[id] = session
	app := t.app
	t.mu.Unlock()

	// N-95: track the goroutine so Shutdown() can wait for it to exit.
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.readLoop(session, app)
	}()

	slog.Info("terminal: session started", "sessionId", id, "workingDir", workingDir, "shell", resolvedShell[0])
	return nil
}

// KillSession kills and removes a specific terminal session.
// N-94: session.running is set to false under t.mu to prevent TOCTOU races.
func (t *TerminalService) KillSession(id string) error {
	t.mu.Lock()
	session, exists := t.sessions[id]
	if !exists {
		t.mu.Unlock()
		return fmt.Errorf("session %s not found", id)
	}
	// N-94: set running = false while holding the lock so concurrent
	// WriteSession/ResizeSession callers see the updated state.
	session.running = false
	delete(t.sessions, id)
	t.mu.Unlock()

	session.conn.Close()

	slog.Info("terminal: session killed", "sessionId", id)
	return nil
}

// WriteSession writes input to a specific terminal session.
// N-94: checks session.running while holding t.mu to prevent TOCTOU.
// Note: input is string (not []byte) because Wails bindings encode []byte as
// base64, which breaks when the frontend sends raw keystroke strings.
func (t *TerminalService) WriteSession(id string, input string) error {
	t.mu.Lock()
	session, exists := t.sessions[id]
	if !exists || !session.running {
		t.mu.Unlock()
		return ErrTerminalNotRunning
	}
	conn := session.conn
	t.mu.Unlock()
	_, err := conn.Write([]byte(input))
	return err
}

// ResizeSession resizes a specific terminal session.
// N-94: checks session.running while holding t.mu to prevent TOCTOU.
func (t *TerminalService) ResizeSession(id string, cols int, rows int) error {
	t.mu.Lock()
	session, exists := t.sessions[id]
	if !exists || !session.running {
		t.mu.Unlock()
		return ErrTerminalNotRunning
	}
	conn := session.conn
	t.mu.Unlock()
	if r, ok := conn.(ptyResizer); ok {
		return r.Resize(cols, rows)
	}
	return nil
}

// IsSessionRunning checks if a specific session is running.
// N-94: reads session.running while holding t.mu to prevent TOCTOU.
func (t *TerminalService) IsSessionRunning(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	session, exists := t.sessions[id]
	return exists && session.running
}

// ListSessions returns the IDs of all active sessions.
func (t *TerminalService) ListSessions() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	ids := make([]string, 0, len(t.sessions))
	for id := range t.sessions {
		ids = append(ids, id)
	}
	return ids
}

// readLoop reads from the session's PTY and emits terminal:output events.
//
// N-94: sets session.running = false under t.mu when the session exits.
// N-65: deletes the session from t.sessions map when the session exits
// naturally (err != nil), preventing memory leaks from dead sessions.
// N-95: checks t.ctx.Done() between reads; Shutdown() cancels the context
// and closes all conns, which unblocks any pending Read.
func (t *TerminalService) readLoop(session *TerminalSession, app *application.App) {
	buf := make([]byte, 4096)
	for {
		// N-95: check for shutdown between reads. The blocking Read below
		// is unblocked by Shutdown() closing the conn.
		select {
		case <-t.ctx.Done():
			t.cleanupSession(session, app, t.ctx.Err())
			return
		default:
		}

		n, err := session.conn.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			session.outputBuf.Append(buf[:n])
			if app != nil {
				app.Event.Emit("terminal:output", map[string]string{
					"sessionId": session.id,
					"data":      data,
				})
			}
		}
		if err != nil {
			t.cleanupSession(session, app, err)
			return
		}
	}
}

// cleanupSession marks the session as not running, removes it from the
// sessions map (N-65), and emits the terminal:exited event.
// N-94: all mutations of session.running and t.sessions happen under t.mu.
func (t *TerminalService) cleanupSession(session *TerminalSession, app *application.App, err error) {
	t.mu.Lock()
	session.running = false
	// N-65: delete the session from the map so it doesn't leak. Only delete
	// if it still points to our session (KillSession may have already deleted it).
	if cur, ok := t.sessions[session.id]; ok && cur == session {
		delete(t.sessions, session.id)
	}
	t.mu.Unlock()

	slog.Info("terminal: session exited", "sessionId", session.id, "err", err)
	if app != nil {
		app.Event.Emit("terminal:output", map[string]string{
			"sessionId": session.id,
			"data":      "\r\n\x1b[90m[Process exited]\x1b[0m\r\n",
		})
		// N-47: emit a separate terminal:exited event so the frontend
		// can mark the session as not running immediately, without
		// relying on parsing the [Process exited] text marker. This
		// lets runCommandInSession return promptly instead of waiting
		// for the 5-minute timeout when the PTY dies mid-step.
		app.Event.Emit("terminal:exited", map[string]any{
			"sessionId": session.id,
			"err":       err.Error(),
		})
	}
}

// Shutdown cancels all readLoop goroutines and waits for them to exit
// (N-95 / Proposal AC). Should be called from application.OnShutdown.
// Safe to call multiple times.
func (t *TerminalService) Shutdown() {
	// Cancel the context to signal all readLoop goroutines to exit.
	t.cancel()

	// Close all session conns to unblock any pending Read calls.
	// Closing the conn causes Read to return an error, which triggers
	// cleanupSession and goroutine exit.
	t.mu.Lock()
	for _, session := range t.sessions {
		session.running = false
		session.conn.Close()
	}
	t.mu.Unlock()

	// Wait for all readLoop goroutines to exit.
	t.wg.Wait()
}

// --- Backward-compatible single-session API (uses "default" session) ---

func (t *TerminalService) Start(workingDir string) error {
	return t.StartSession("default", workingDir, "")
}

func (t *TerminalService) Write(input string) error {
	return t.WriteSession("default", input)
}

func (t *TerminalService) Resize(cols, rows int) error {
	return t.ResizeSession("default", cols, rows)
}

func (t *TerminalService) Kill() {
	_ = t.KillSession("default")
}

func (t *TerminalService) IsRunning() bool {
	return t.IsSessionRunning("default")
}

func (t *TerminalService) ReadOutput(timeout time.Duration) string {
	t.mu.Lock()
	session, exists := t.sessions["default"]
	t.mu.Unlock()
	if !exists {
		return ""
	}
	return session.outputBuf.Read(timeout)
}

type ptyResizer interface {
	Resize(cols, rows int) error
}

var ErrTerminalNotRunning = errTerminalNotRunning{}

type errTerminalNotRunning struct{}

func (errTerminalNotRunning) Error() string { return "terminal not running" }
