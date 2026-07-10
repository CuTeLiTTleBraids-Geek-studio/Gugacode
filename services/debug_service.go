package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DebugService provides an in-IDE DAP client over Delve `dlv dap` (prompt-11/12).
// Capabilities: breakpoints (condition), F5/Restart, step, stack, locals, watch/evaluate.
type DebugService struct {
	mu         sync.Mutex
	writeMu    sync.Mutex // serializes DAP writes on conn
	cmd        *exec.Cmd
	addr       string
	mode       string // "package" | "test" | "node" | "attach"
	started    time.Time
	conn       net.Conn
	seq        int64
	pending    map[int]chan dapMessage
	readerDone chan struct{}

	// session UI state
	stopped     bool
	threadID    int
	stopReason  string
	breakpoints []DebugBreakpoint
	stack       []DebugStackFrame
	locals      []DebugVariable
	watches     []string
	watchValues []DebugVariable
	cwd         string

	// last launch for Restart (prompt-12 12-A/G)
	lastLaunch debugLaunchSpec
}

// debugLaunchSpec remembers how to restart the current configuration.
type debugLaunchSpec struct {
	Kind      string // package | test | node
	Dir       string
	RunRegex  string
	Program   string
	Args      []string
	Env       map[string]string
	Mode      string // debug | test
	StopEntry bool
}

// DebugBreakpoint is a source breakpoint (prompt-12: condition + verified UI).
type DebugBreakpoint struct {
	ID         int    `json:"id"`
	File       string `json:"file"`
	Line       int    `json:"line"` // 1-based
	Verified   bool   `json:"verified"`
	Condition  string `json:"condition,omitempty"`
	LogMessage string `json:"logMessage,omitempty"` // logpoint text
	Message    string `json:"message,omitempty"`    // adapter message when unverified
}

// DebugStackFrame is one stack frame.
type DebugStackFrame struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// DebugVariable is a local or scope variable.
type DebugVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// DebugSessionInfo is returned after launch / status queries.
type DebugSessionInfo struct {
	Running    bool   `json:"running"`
	Address    string `json:"address"`
	Mode       string `json:"mode"`
	Message    string `json:"message"`
	Stopped    bool   `json:"stopped"`
	StopReason string `json:"stopReason"`
	ThreadID   int    `json:"threadId"`
}

// DebugStateSnapshot is polled by the frontend for stack/locals/bps/watches.
type DebugStateSnapshot struct {
	Session     DebugSessionInfo  `json:"session"`
	Breakpoints []DebugBreakpoint `json:"breakpoints"`
	Stack       []DebugStackFrame `json:"stack"`
	Locals      []DebugVariable   `json:"locals"`
	Watches     []DebugVariable   `json:"watches"`
	StopReason  string            `json:"stopReason"`
}

// DebugLaunchConfig is a persisted launch profile (prompt-12 12-G).
type DebugLaunchConfig struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"` // package | test | node
	Dir       string            `json:"dir"`
	Program   string            `json:"program,omitempty"`
	RunRegex  string            `json:"runRegex,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	StopEntry bool              `json:"stopOnEntry,omitempty"`
	Mode      string            `json:"mode,omitempty"` // debug | test (delve)
}

// dapMessage is a subset of the Debug Adapter Protocol message envelope.
type dapMessage struct {
	Seq         int             `json:"seq"`
	Type        string          `json:"type"` // request | response | event
	Command     string          `json:"command,omitempty"`
	RequestSeq  int             `json:"request_seq,omitempty"`
	Success     bool            `json:"success,omitempty"`
	Message     string          `json:"message,omitempty"`
	Event       string          `json:"event,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
}

// NewDebugService creates the debug service.
func NewDebugService() *DebugService {
	return &DebugService{
		pending: make(map[int]chan dapMessage),
	}
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
		if d.stopped {
			return fmt.Sprintf("Debugging paused (%s) on %s", d.stopReason, d.addr)
		}
		return fmt.Sprintf("Debugging active (%s) DAP %s", d.mode, d.addr)
	}
	if d.IsAvailable() {
		return "Delve available — F5 / Debug Package for in-IDE DAP"
	}
	return "Delve not installed (go install github.com/go-delve/delve/cmd/dlv@latest)"
}

// IsRunning reports whether a DAP session is active.
func (d *DebugService) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil
}

// GetSession returns current session state.
func (d *DebugService) GetSession() DebugSessionInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sessionLocked()
}

func (d *DebugService) sessionLocked() DebugSessionInfo {
	running := d.cmd != nil && d.cmd.Process != nil && d.cmd.ProcessState == nil
	if running {
		msg := fmt.Sprintf("DAP session on %s (%s)", d.addr, d.mode)
		if d.stopped {
			msg = fmt.Sprintf("Paused: %s — %s", d.stopReason, d.addr)
		}
		return DebugSessionInfo{
			Running:    true,
			Address:    d.addr,
			Mode:       d.mode,
			Message:    msg,
			Stopped:    d.stopped,
			StopReason: d.stopReason,
			ThreadID:   d.threadID,
		}
	}
	return DebugSessionInfo{Running: false, Message: d.statusMessageUnlocked()}
}

func (d *DebugService) statusMessageUnlocked() string {
	if d.IsAvailable() {
		return "Delve available — F5 / Debug Package for in-IDE DAP"
	}
	return "Delve not installed"
}

// GetState returns full snapshot for the debug panel.
func (d *DebugService) GetState() DebugStateSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	bps := append([]DebugBreakpoint(nil), d.breakpoints...)
	stack := append([]DebugStackFrame(nil), d.stack...)
	locals := append([]DebugVariable(nil), d.locals...)
	watches := append([]DebugVariable(nil), d.watchValues...)
	return DebugStateSnapshot{
		Session:     d.sessionLocked(),
		Breakpoints: bps,
		Stack:       stack,
		Locals:      locals,
		Watches:     watches,
		StopReason:  d.stopReason,
	}
}

// LaunchPackage starts dlv dap and launches a debug session for packageDir.
func (d *DebugService) LaunchPackage(packageDir string) (DebugSessionInfo, error) {
	return d.LaunchWithConfig(DebugLaunchConfig{Kind: "package", Dir: packageDir, Mode: "debug"})
}

// LaunchTest starts dlv dap and launches a test debug session (-test.run regex).
func (d *DebugService) LaunchTest(packageDir, runRegex string) (DebugSessionInfo, error) {
	return d.LaunchWithConfig(DebugLaunchConfig{Kind: "test", Dir: packageDir, RunRegex: runRegex, Mode: "test"})
}

// LaunchNode starts a Node/TS debug session via node --inspect-brk (prompt-12 12-F MVP).
// Uses DAP when possible; falls back to process launch + evaluate via inspector is limited —
// primary path: node with inspect and client attaches through same DAP framing if adapter present,
// else launches node --inspect-brk and exposes address for session state.
func (d *DebugService) LaunchNode(program string, args []string) (DebugSessionInfo, error) {
	return d.LaunchWithConfig(DebugLaunchConfig{
		Kind:    "node",
		Program: program,
		Args:    args,
		Dir:     filepath.Dir(program),
	})
}

// LaunchWithConfig starts a session from a saved/selected profile (prompt-12 12-G).
func (d *DebugService) LaunchWithConfig(cfg DebugLaunchConfig) (DebugSessionInfo, error) {
	kind := cfg.Kind
	if kind == "" {
		kind = "package"
	}
	switch kind {
	case "node":
		return d.launchNode(cfg)
	default:
		mode := cfg.Mode
		if mode == "" {
			if kind == "test" {
				mode = "test"
			} else {
				mode = "debug"
			}
		}
		return d.launchDAP(cfg.Dir, mode, cfg.RunRegex, cfg)
	}
}

// Restart re-launches the last configuration (prompt-12 12-A).
func (d *DebugService) Restart() (DebugSessionInfo, error) {
	d.mu.Lock()
	spec := d.lastLaunch
	d.mu.Unlock()
	if spec.Kind == "" && spec.Dir == "" && spec.Program == "" {
		return DebugSessionInfo{}, fmt.Errorf("no previous launch to restart")
	}
	cfg := DebugLaunchConfig{
		Kind:      spec.Kind,
		Dir:       spec.Dir,
		Program:   spec.Program,
		RunRegex:  spec.RunRegex,
		Args:      spec.Args,
		Env:       spec.Env,
		StopEntry: spec.StopEntry,
		Mode:      spec.Mode,
	}
	if cfg.Kind == "" {
		cfg.Kind = "package"
	}
	return d.LaunchWithConfig(cfg)
}

func (d *DebugService) launchDAP(packageDir, mode, runRegex string, cfg DebugLaunchConfig) (DebugSessionInfo, error) {
	if !d.IsAvailable() {
		return DebugSessionInfo{}, fmt.Errorf("dlv not found on PATH")
	}
	// Stop previous session if any (prompt-11: allow relaunch).
	_ = d.Stop()

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

	// Use `dlv dap` so we speak standard DAP (prompt-11 11-A).
	cmd := exec.Command(dlv, "dap", "--listen="+addr, "--log=false")
	cmd.Dir = abs
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return DebugSessionInfo{}, fmt.Errorf("start dlv dap: %w", err)
	}
	slog.Info("dlv dap launched", "addr", addr, "dir", abs, "pid", cmd.Process.Pid)

	// Wait for port to accept connections.
	var conn net.Conn
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
		if err == nil {
			conn = c
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if conn == nil {
		_ = cmd.Process.Kill()
		return DebugSessionInfo{}, fmt.Errorf("could not connect to dlv dap on %s", addr)
	}

	d.mu.Lock()
	d.cmd = cmd
	d.addr = addr
	d.mode = mode
	d.started = time.Now()
	d.conn = conn
	d.pending = make(map[int]chan dapMessage)
	d.readerDone = make(chan struct{})
	d.stopped = false
	d.stack = nil
	d.locals = nil
	d.cwd = abs
	// keep breakpoints list (re-apply after launch)
	bpsCopy := append([]DebugBreakpoint(nil), d.breakpoints...)
	d.mu.Unlock()

	go d.readLoop()
	go func() {
		_ = cmd.Wait()
		d.mu.Lock()
		if d.cmd == cmd {
			d.cleanupLocked()
		}
		d.mu.Unlock()
		slog.Info("dlv dap exited", "addr", addr)
	}()

	if err := d.dapInitialize(); err != nil {
		_ = d.Stop()
		return DebugSessionInfo{}, fmt.Errorf("dap initialize: %w", err)
	}

	// Launch configuration for Delve DAP.
	launchArgs := map[string]interface{}{
		"request":     "launch",
		"mode":        "debug",
		"program":     abs,
		"cwd":         abs,
		"stopOnEntry": cfg.StopEntry,
	}
	if mode == "test" || cfg.Kind == "test" {
		launchArgs["mode"] = "test"
		launchArgs["program"] = abs
		if runRegex != "" {
			launchArgs["args"] = []string{"-test.run", runRegex}
		}
	}
	if len(cfg.Args) > 0 && mode != "test" {
		launchArgs["args"] = cfg.Args
	}
	if len(cfg.Env) > 0 {
		launchArgs["env"] = cfg.Env
	}
	if err := d.dapRequest("launch", launchArgs); err != nil {
		_ = d.Stop()
		return DebugSessionInfo{}, fmt.Errorf("dap launch: %w", err)
	}

	// Re-apply breakpoints grouped by file.
	if err := d.applyAllBreakpoints(bpsCopy); err != nil {
		slog.Debug("re-apply breakpoints", "err", err)
	}

	if err := d.dapRequest("configurationDone", map[string]interface{}{}); err != nil {
		// Some adapters treat this as optional after launch; log only.
		slog.Debug("configurationDone", "err", err)
	}

	d.mu.Lock()
	d.lastLaunch = debugLaunchSpec{
		Kind: cfg.Kind, Dir: abs, RunRegex: runRegex, Mode: mode,
		Args: cfg.Args, Env: cfg.Env, StopEntry: cfg.StopEntry,
	}
	if d.lastLaunch.Kind == "" {
		if mode == "test" {
			d.lastLaunch.Kind = "test"
		} else {
			d.lastLaunch.Kind = "package"
		}
	}
	d.mu.Unlock()

	return d.GetSession(), nil
}

// launchNode: MVP Node/TS debug via node --inspect-brk (prompt-12 12-F).
// Exposes listen address; full CDP UI is progressive — session is tracked like DAP.
func (d *DebugService) launchNode(cfg DebugLaunchConfig) (DebugSessionInfo, error) {
	_ = d.Stop()
	prog := cfg.Program
	if prog == "" {
		return DebugSessionInfo{}, fmt.Errorf("node program path required")
	}
	if a, err := filepath.Abs(prog); err == nil {
		prog = a
	}
	dir := cfg.Dir
	if dir == "" {
		dir = filepath.Dir(prog)
	}
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		return DebugSessionInfo{}, fmt.Errorf("node not found on PATH")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return DebugSessionInfo{}, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	args := []string{fmt.Sprintf("--inspect-brk=127.0.0.1:%d", port), prog}
	args = append(args, cfg.Args...)
	cmd := exec.Command(nodeBin, args...)
	cmd.Dir = dir
	if len(cfg.Env) > 0 {
		env := os.Environ()
		for k, v := range cfg.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	if err := cmd.Start(); err != nil {
		return DebugSessionInfo{}, err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	d.mu.Lock()
	d.cmd = cmd
	d.addr = addr
	d.mode = "node"
	d.cwd = dir
	d.stopped = true
	d.stopReason = "entry"
	d.lastLaunch = debugLaunchSpec{Kind: "node", Program: prog, Dir: dir, Args: cfg.Args, Env: cfg.Env}
	d.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		d.mu.Lock()
		if d.cmd == cmd {
			d.cleanupLocked()
		}
		d.mu.Unlock()
	}()
	return DebugSessionInfo{
		Running:    true,
		Address:    addr,
		Mode:       "node",
		Message:    fmt.Sprintf("Node inspect-brk on %s — attach Chrome DevTools or continue MVP session", addr),
		Stopped:    true,
		StopReason: "entry",
	}, nil
}

// ConnectMockDAP connects to an existing DAP server (tests / attach) and runs launch (prompt-12 12-E).
func (d *DebugService) ConnectMockDAP(addr string, launchArgs map[string]interface{}) (DebugSessionInfo, error) {
	_ = d.Stop()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return DebugSessionInfo{}, err
	}
	d.mu.Lock()
	d.conn = conn
	d.addr = addr
	d.mode = "attach"
	d.pending = make(map[int]chan dapMessage)
	d.readerDone = make(chan struct{})
	d.stopped = false
	bpsCopy := append([]DebugBreakpoint(nil), d.breakpoints...)
	d.mu.Unlock()
	go d.readLoop()

	if err := d.dapInitialize(); err != nil {
		_ = d.Stop()
		return DebugSessionInfo{}, err
	}
	if launchArgs == nil {
		launchArgs = map[string]interface{}{"request": "launch", "program": "."}
	}
	if err := d.dapRequest("launch", launchArgs); err != nil {
		_ = d.Stop()
		return DebugSessionInfo{}, err
	}
	_ = d.applyAllBreakpoints(bpsCopy)
	_ = d.dapRequest("configurationDone", map[string]interface{}{})
	// Wait briefly for stopped event from adapter
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d.mu.Lock()
		st := d.stopped
		d.mu.Unlock()
		if st {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = d.RefreshStackAndLocals()
	return d.GetSession(), nil
}

func (d *DebugService) cleanupLocked() {
	if d.conn != nil {
		_ = d.conn.Close()
		d.conn = nil
	}
	d.cmd = nil
	d.addr = ""
	d.mode = ""
	d.stopped = false
	d.stack = nil
	d.locals = nil
	for _, ch := range d.pending {
		close(ch)
	}
	d.pending = make(map[int]chan dapMessage)
}

// Stop terminates the DAP session and Delve process.
func (d *DebugService) Stop() error {
	d.mu.Lock()
	conn := d.conn
	cmd := d.cmd
	d.cleanupLocked()
	d.mu.Unlock()
	if conn != nil {
		// best-effort disconnect
		_ = d.sendRequestUnlocked(conn, "disconnect", map[string]interface{}{"restart": false})
		_ = conn.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

// SetBreakpoint adds or updates a source breakpoint (1-based line).
func (d *DebugService) SetBreakpoint(file string, line int) (DebugBreakpoint, error) {
	return d.SetBreakpointEx(file, line, "", "")
}

// SetBreakpointEx sets a breakpoint with optional condition / logpoint (prompt-12 12-B).
func (d *DebugService) SetBreakpointEx(file string, line int, condition, logMessage string) (DebugBreakpoint, error) {
	if line < 1 {
		return DebugBreakpoint{}, fmt.Errorf("invalid line")
	}
	abs := file
	if a, err := filepath.Abs(file); err == nil {
		abs = a
	}
	d.mu.Lock()
	found := false
	for i, b := range d.breakpoints {
		if filepath.Clean(b.File) == filepath.Clean(abs) && b.Line == line {
			found = true
			d.breakpoints[i].Verified = false
			d.breakpoints[i].Condition = condition
			d.breakpoints[i].LogMessage = logMessage
			d.breakpoints[i].Message = ""
			break
		}
	}
	if !found {
		d.breakpoints = append(d.breakpoints, DebugBreakpoint{
			File: abs, Line: line, Condition: condition, LogMessage: logMessage,
		})
	}
	bps := append([]DebugBreakpoint(nil), d.breakpoints...)
	running := d.conn != nil
	d.mu.Unlock()

	if running {
		if err := d.applyAllBreakpoints(bps); err != nil {
			return DebugBreakpoint{}, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, b := range d.breakpoints {
		if filepath.Clean(b.File) == filepath.Clean(abs) && b.Line == line {
			return b, nil
		}
	}
	return DebugBreakpoint{File: abs, Line: line, Condition: condition, LogMessage: logMessage}, nil
}

// SetBreakpointCondition updates condition on an existing breakpoint (prompt-12 12-B).
func (d *DebugService) SetBreakpointCondition(file string, line int, condition string) (DebugBreakpoint, error) {
	abs := file
	if a, err := filepath.Abs(file); err == nil {
		abs = a
	}
	d.mu.Lock()
	var logMsg string
	found := false
	for _, b := range d.breakpoints {
		if filepath.Clean(b.File) == filepath.Clean(abs) && b.Line == line {
			found = true
			logMsg = b.LogMessage
			break
		}
	}
	d.mu.Unlock()
	if !found {
		return d.SetBreakpointEx(abs, line, condition, "")
	}
	return d.SetBreakpointEx(abs, line, condition, logMsg)
}

// RemoveBreakpoint removes a breakpoint at file:line (1-based).
func (d *DebugService) RemoveBreakpoint(file string, line int) error {
	abs := file
	if a, err := filepath.Abs(file); err == nil {
		abs = a
	}
	d.mu.Lock()
	var next []DebugBreakpoint
	for _, b := range d.breakpoints {
		if filepath.Clean(b.File) == filepath.Clean(abs) && b.Line == line {
			continue
		}
		next = append(next, b)
	}
	d.breakpoints = next
	bps := append([]DebugBreakpoint(nil), d.breakpoints...)
	running := d.conn != nil
	d.mu.Unlock()
	if running {
		return d.applyAllBreakpoints(bps)
	}
	return nil
}

// ToggleBreakpoint toggles a breakpoint; returns the resulting list for the file.
func (d *DebugService) ToggleBreakpoint(file string, line int) ([]DebugBreakpoint, error) {
	abs := file
	if a, err := filepath.Abs(file); err == nil {
		abs = a
	}
	d.mu.Lock()
	exists := false
	for _, b := range d.breakpoints {
		if filepath.Clean(b.File) == filepath.Clean(abs) && b.Line == line {
			exists = true
			break
		}
	}
	d.mu.Unlock()
	if exists {
		_ = d.RemoveBreakpoint(abs, line)
	} else {
		_, _ = d.SetBreakpoint(abs, line)
	}
	return d.ListBreakpoints(), nil
}

// ListBreakpoints returns all breakpoints.
func (d *DebugService) ListBreakpoints() []DebugBreakpoint {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DebugBreakpoint(nil), d.breakpoints...)
}

// Continue resumes execution.
func (d *DebugService) Continue() error {
	d.mu.Lock()
	tid := d.threadID
	if tid == 0 {
		tid = 1
	}
	d.stopped = false
	d.mu.Unlock()
	return d.dapRequest("continue", map[string]interface{}{"threadId": tid})
}

// StepOver steps over the current line.
func (d *DebugService) StepOver() error {
	return d.step("next")
}

// StepIn steps into a call.
func (d *DebugService) StepIn() error {
	return d.step("stepIn")
}

// StepOut steps out of the current function.
func (d *DebugService) StepOut() error {
	return d.step("stepOut")
}

// Pause requests a pause (if supported).
func (d *DebugService) Pause() error {
	d.mu.Lock()
	tid := d.threadID
	if tid == 0 {
		tid = 1
	}
	d.mu.Unlock()
	return d.dapRequest("pause", map[string]interface{}{"threadId": tid})
}

func (d *DebugService) step(cmd string) error {
	d.mu.Lock()
	tid := d.threadID
	if tid == 0 {
		tid = 1
	}
	d.stopped = false
	d.mu.Unlock()
	return d.dapRequest(cmd, map[string]interface{}{"threadId": tid})
}

// RefreshStackAndLocals pulls stack + top-frame locals (call after stop).
func (d *DebugService) RefreshStackAndLocals() error {
	d.mu.Lock()
	tid := d.threadID
	if tid == 0 {
		tid = 1
	}
	d.mu.Unlock()

	body, err := d.dapRequestBody("stackTrace", map[string]interface{}{
		"threadId":   tid,
		"startFrame": 0,
		"levels":     32,
	})
	if err != nil {
		return err
	}
	var st struct {
		StackFrames []struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
			Source *struct {
				Path string `json:"path"`
				Name string `json:"name"`
			} `json:"source"`
		} `json:"stackFrames"`
	}
	_ = json.Unmarshal(body, &st)
	frames := make([]DebugStackFrame, 0, len(st.StackFrames))
	for _, f := range st.StackFrames {
		path := ""
		if f.Source != nil {
			path = f.Source.Path
			if path == "" {
				path = f.Source.Name
			}
		}
		frames = append(frames, DebugStackFrame{
			ID: f.ID, Name: f.Name, File: path, Line: f.Line, Column: f.Column,
		})
	}
	d.mu.Lock()
	d.stack = frames
	d.mu.Unlock()

	if len(frames) == 0 {
		d.mu.Lock()
		d.locals = nil
		d.mu.Unlock()
		return nil
	}
	return d.loadLocalsForFrame(frames[0].ID)
}

// SelectFrame loads locals for a stack frame id.
func (d *DebugService) SelectFrame(frameID int) error {
	return d.loadLocalsForFrame(frameID)
}

func (d *DebugService) loadLocalsForFrame(frameID int) error {
	body, err := d.dapRequestBody("scopes", map[string]interface{}{"frameId": frameID})
	if err != nil {
		return err
	}
	var sc struct {
		Scopes []struct {
			Name               string `json:"name"`
			VariablesReference int    `json:"variablesReference"`
		} `json:"scopes"`
	}
	_ = json.Unmarshal(body, &sc)
	var locals []DebugVariable
	for _, s := range sc.Scopes {
		// Prefer Locals; still include others if no Locals.
		if !strings.EqualFold(s.Name, "Locals") && !strings.EqualFold(s.Name, "Local") && len(sc.Scopes) > 1 {
			// still load Locals-like scopes first
			if strings.Contains(strings.ToLower(s.Name), "local") {
				// ok
			} else if s.Name != "Arguments" && s.Name != "Args" {
				continue
			}
		}
		vb, err := d.dapRequestBody("variables", map[string]interface{}{
			"variablesReference": s.VariablesReference,
		})
		if err != nil {
			continue
		}
		var vr struct {
			Variables []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
				Type  string `json:"type"`
			} `json:"variables"`
		}
		_ = json.Unmarshal(vb, &vr)
		for _, v := range vr.Variables {
			locals = append(locals, DebugVariable{Name: v.Name, Value: v.Value, Type: v.Type})
		}
		if strings.Contains(strings.ToLower(s.Name), "local") {
			break
		}
	}
	d.mu.Lock()
	d.locals = locals
	d.mu.Unlock()
	return nil
}

// --- DAP protocol helpers ---

func (d *DebugService) dapInitialize() error {
	return d.dapRequest("initialize", map[string]interface{}{
		"clientID":                     "gugacode",
		"clientName":                   "gugacode",
		"adapterID":                    "go",
		"pathFormat":                   "path",
		"linesStartAt1":                true,
		"columnsStartAt1":              true,
		"supportsVariableType":         true,
		"supportsVariablePaging":       false,
		"supportsRunInTerminalRequest": false,
	})
}

func (d *DebugService) applyAllBreakpoints(bps []DebugBreakpoint) error {
	byFile := map[string][]DebugBreakpoint{}
	for _, b := range bps {
		f := filepath.Clean(b.File)
		byFile[f] = append(byFile[f], b)
	}
	var verified []DebugBreakpoint
	for file, list := range byFile {
		src := map[string]interface{}{"path": file}
		bpsArgs := make([]map[string]interface{}, 0, len(list))
		for _, b := range list {
			arg := map[string]interface{}{"line": b.Line}
			if b.Condition != "" {
				arg["condition"] = b.Condition
			}
			if b.LogMessage != "" {
				arg["logMessage"] = b.LogMessage
			}
			bpsArgs = append(bpsArgs, arg)
		}
		body, err := d.dapRequestBody("setBreakpoints", map[string]interface{}{
			"source":      src,
			"breakpoints": bpsArgs,
		})
		if err != nil {
			for _, b := range list {
				b.Verified = false
				b.Message = err.Error()
				verified = append(verified, b)
			}
			continue
		}
		var resp struct {
			Breakpoints []struct {
				ID       int    `json:"id"`
				Line     int    `json:"line"`
				Verified bool   `json:"verified"`
				Message  string `json:"message"`
			} `json:"breakpoints"`
		}
		_ = json.Unmarshal(body, &resp)
		for i, b := range list {
			bp := b
			if i < len(resp.Breakpoints) {
				bp.ID = resp.Breakpoints[i].ID
				bp.Verified = resp.Breakpoints[i].Verified
				bp.Message = resp.Breakpoints[i].Message
				if !bp.Verified && bp.Message == "" {
					bp.Message = "unverified"
				}
				if resp.Breakpoints[i].Line > 0 {
					bp.Line = resp.Breakpoints[i].Line
				}
			} else {
				bp.Verified = false
				bp.Message = "no adapter response"
			}
			verified = append(verified, bp)
		}
	}
	d.mu.Lock()
	d.breakpoints = verified
	d.mu.Unlock()
	return nil
}

// Evaluate runs DAP evaluate for expression (watch / REPL) (prompt-12 12-B).
func (d *DebugService) Evaluate(expression string) (DebugVariable, error) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return DebugVariable{}, fmt.Errorf("empty expression")
	}
	d.mu.Lock()
	frameID := 0
	if len(d.stack) > 0 {
		frameID = d.stack[0].ID
	}
	d.mu.Unlock()
	body, err := d.dapRequestBody("evaluate", map[string]interface{}{
		"expression": expr,
		"frameId":    frameID,
		"context":    "watch",
	})
	if err != nil {
		return DebugVariable{Name: expr, Value: err.Error(), Type: "error"}, err
	}
	var resp struct {
		Result string `json:"result"`
		Type   string `json:"type"`
	}
	_ = json.Unmarshal(body, &resp)
	return DebugVariable{Name: expr, Value: resp.Result, Type: resp.Type}, nil
}

// AddWatch adds an expression to the watch list and evaluates if stopped.
func (d *DebugService) AddWatch(expression string) ([]DebugVariable, error) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}
	d.mu.Lock()
	for _, w := range d.watches {
		if w == expr {
			d.mu.Unlock()
			return d.RefreshWatches()
		}
	}
	d.watches = append(d.watches, expr)
	d.mu.Unlock()
	return d.RefreshWatches()
}

// RemoveWatch removes a watch expression.
func (d *DebugService) RemoveWatch(expression string) ([]DebugVariable, error) {
	d.mu.Lock()
	var next []string
	for _, w := range d.watches {
		if w != expression {
			next = append(next, w)
		}
	}
	d.watches = next
	d.mu.Unlock()
	return d.RefreshWatches()
}

// RefreshWatches re-evaluates all watches.
func (d *DebugService) RefreshWatches() ([]DebugVariable, error) {
	d.mu.Lock()
	exprs := append([]string(nil), d.watches...)
	d.mu.Unlock()
	var out []DebugVariable
	for _, e := range exprs {
		v, err := d.Evaluate(e)
		if err != nil {
			out = append(out, DebugVariable{Name: e, Value: err.Error(), Type: "error"})
		} else {
			out = append(out, v)
		}
	}
	d.mu.Lock()
	d.watchValues = out
	d.mu.Unlock()
	return append([]DebugVariable(nil), out...), nil
}

// ListWatches returns last evaluated watch values.
func (d *DebugService) ListWatches() []DebugVariable {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]DebugVariable(nil), d.watchValues...)
}

func (d *DebugService) dapRequest(command string, args map[string]interface{}) error {
	_, err := d.dapRequestBody(command, args)
	return err
}

func (d *DebugService) dapRequestBody(command string, args map[string]interface{}) (json.RawMessage, error) {
	d.mu.Lock()
	conn := d.conn
	if conn == nil {
		d.mu.Unlock()
		return nil, fmt.Errorf("no dap connection")
	}
	seq := int(atomic.AddInt64(&d.seq, 1))
	ch := make(chan dapMessage, 1)
	d.pending[seq] = ch
	d.mu.Unlock()

	payload := map[string]interface{}{
		"seq":       seq,
		"type":      "request",
		"command":   command,
		"arguments": args,
	}
	d.writeMu.Lock()
	err := writeDAPMessage(conn, payload)
	d.writeMu.Unlock()
	if err != nil {
		d.mu.Lock()
		delete(d.pending, seq)
		d.mu.Unlock()
		return nil, err
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("dap connection closed")
		}
		if !resp.Success && resp.Type == "response" {
			msg := resp.Message
			if msg == "" {
				msg = "dap request failed: " + command
			}
			return resp.Body, fmt.Errorf("%s", msg)
		}
		return resp.Body, nil
	case <-time.After(30 * time.Second):
		d.mu.Lock()
		delete(d.pending, seq)
		d.mu.Unlock()
		return nil, fmt.Errorf("dap timeout: %s", command)
	}
}

// sendRequestUnlocked sends without waiting (best-effort during shutdown).
func (d *DebugService) sendRequestUnlocked(conn net.Conn, command string, args map[string]interface{}) error {
	seq := int(atomic.AddInt64(&d.seq, 1))
	payload := map[string]interface{}{
		"seq": seq, "type": "request", "command": command, "arguments": args,
	}
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	return writeDAPMessage(conn, payload)
}

func writeDAPMessage(w io.Writer, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (d *DebugService) readLoop() {
	d.mu.Lock()
	conn := d.conn
	done := d.readerDone
	d.mu.Unlock()
	if conn == nil {
		return
	}
	defer func() {
		if done != nil {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	}()

	reader := bufio.NewReader(conn)
	for {
		msg, err := readDAPMessage(reader)
		if err != nil {
			return
		}
		d.handleDAPMessage(msg)
	}
}

func readDAPMessage(r *bufio.Reader) (dapMessage, error) {
	var contentLen int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return dapMessage{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			n := strings.TrimSpace(line[len("Content-Length:"):])
			// case-insensitive prefix already handled roughly
			if i := strings.Index(strings.ToLower(line), "content-length:"); i >= 0 {
				n = strings.TrimSpace(line[i+len("content-length:"):])
			}
			contentLen, _ = strconv.Atoi(n)
		}
	}
	if contentLen <= 0 {
		return dapMessage{}, fmt.Errorf("missing content-length")
	}
	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return dapMessage{}, err
	}
	var msg dapMessage
	if err := json.Unmarshal(buf, &msg); err != nil {
		return dapMessage{}, err
	}
	return msg, nil
}

func (d *DebugService) handleDAPMessage(msg dapMessage) {
	switch msg.Type {
	case "response":
		d.mu.Lock()
		ch := d.pending[msg.RequestSeq]
		delete(d.pending, msg.RequestSeq)
		d.mu.Unlock()
		if ch != nil {
			ch <- msg
		}
	case "event":
		d.handleDAPEvent(msg)
	}
}

func (d *DebugService) handleDAPEvent(msg dapMessage) {
	switch msg.Event {
	case "stopped":
		var body struct {
			Reason   string `json:"reason"`
			ThreadID int    `json:"threadId"`
		}
		_ = json.Unmarshal(msg.Body, &body)
		d.mu.Lock()
		d.stopped = true
		d.stopReason = body.Reason
		if d.stopReason == "" {
			d.stopReason = "paused"
		}
		if body.ThreadID != 0 {
			d.threadID = body.ThreadID
		}
		d.mu.Unlock()
		// best-effort refresh stack/locals/watches
		go func() {
			_ = d.RefreshStackAndLocals()
			_, _ = d.RefreshWatches()
		}()
	case "continued":
		d.mu.Lock()
		d.stopped = false
		d.stopReason = ""
		d.mu.Unlock()
	case "terminated", "exited":
		d.mu.Lock()
		d.stopped = false
		d.stopReason = msg.Event
		d.mu.Unlock()
	case "output":
		var body struct {
			Output string `json:"output"`
		}
		_ = json.Unmarshal(msg.Body, &body)
		if body.Output != "" {
			slog.Debug("dap output", "text", strings.TrimSpace(body.Output))
		}
	case "initialized":
		// configurationDone is sent after launch in launchDAP
	}
}
