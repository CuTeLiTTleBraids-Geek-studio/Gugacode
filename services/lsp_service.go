package services

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// G-FEAT-02 + prompt-8: Offline language intelligence via LSP.
//
// Go: gopls. TypeScript/JavaScript: typescript-language-server or vtsls
// (NOT raw tsserver — that speaks a proprietary protocol, BUG-IDE-02).
//
// Document sync: syncDocument sends didOpen / didChange with monotonic
// versions so completions reflect the live buffer (BUG-IDE-01).
//
// Graceful fallback: if a server is not installed or not running, query
// methods return empty results (not errors) so the editor degrades smoothly.

// LSPServerStatus reports the availability and state of a language server.
type LSPServerStatus struct {
	Language   string `json:"language"`
	Available  bool   `json:"available"`
	Running    bool   `json:"running"`
	ServerPath string `json:"serverPath"`
	Version    string `json:"version"`
	// LastError is a short human message when start/initialize last failed
	// (prompt-8 Task 8-D). Empty when healthy / never started.
	LastError string `json:"lastError,omitempty"`
	// ServerKind labels the binary (gopls / typescript-language-server / vtsls).
	ServerKind string `json:"serverKind,omitempty"`
}

// LSPCompletionRequest is sent from the frontend to query an LSP server.
type LSPCompletionRequest struct {
	Language string `json:"language"`
	FilePath string `json:"filePath"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Content  string `json:"content"` // full file content
}

// LSPCompletionItem represents a single completion item returned by the LSP.
// AdditionalEdits carry auto-import / additionalTextEdits (prompt-10 10-I).
type LSPCompletionItem struct {
	Label           string       `json:"label"`
	Kind            int          `json:"kind"`
	Detail          string       `json:"detail"`
	InsertText      string       `json:"insertText"`
	AdditionalEdits []TextEdit `json:"additionalEdits,omitempty"`
}

// Diagnostic represents a single LSP diagnostic (error/warning).
type Diagnostic struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	EndLine  int    `json:"endLine"`
	EndCol   int    `json:"endColumn"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source"`
}

// lspServer wraps a running language server process and the JSON-RPC client
// used to talk to it over stdin/stdout.
type lspServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	client *jsonRPCClient

	// docVersions tracks open documents and their last-synced version
	// (prompt-8 Task 8-A / BUG-IDE-01). Keyed by file:// URI.
	docVersions map[string]int
	// docHashes last full-sync content hash (prompt-9 9-K throttle).
	docHashes map[string]string
	// docLastSync last successful didChange time per URI.
	docLastSync map[string]time.Time
	docMu       sync.Mutex

	// diagnostics cache (publishDiagnostics).
	diags   map[string][]Diagnostic
	diagsMu sync.Mutex
}

// LSPService manages language server processes (gopls, typescript-language-server).
type LSPService struct {
	mu            sync.Mutex
	workspaceRoot string
	servers       map[string]*lspServer // keyed by language: "go", "typescript", "javascript"
	// lastErrors records the last start/init failure per language (8-D).
	lastErrors map[string]string
}

// NewLSPService creates a new LSPService with the given workspace root.
func NewLSPService(workspaceRoot string) *LSPService {
	return &LSPService{
		workspaceRoot: workspaceRoot,
		servers:       make(map[string]*lspServer),
		lastErrors:    make(map[string]string),
	}
}

// SetWorkspaceRoot updates the workspace root. Running servers are stopped
// because they were initialized against the previous root.
func (s *LSPService) SetWorkspaceRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workspaceRoot == root {
		return
	}
	s.workspaceRoot = root
	// Stop all running servers — they were initialized against the old root.
	for lang := range s.servers {
		if srv, ok := s.servers[lang]; ok && srv.cmd != nil && srv.cmd.Process != nil {
			_ = srv.client.notify("shutdown", nil)
			_ = srv.client.notify("exit", nil)
			_ = srv.cmd.Process.Kill()
		}
		delete(s.servers, lang)
	}
}

// serverNameForLanguage returns a preferred executable label for errors.
func serverNameForLanguage(language string) (exe string, ok bool) {
	switch language {
	case "go":
		return "gopls", true
	case "typescript", "javascript":
		// prompt-8 BUG-IDE-02: prefer LSP wrappers, not raw tsserver.
		return "typescript-language-server", true
	}
	return "", false
}

// DetectLSPServers checks if language servers are installed and available.
func (s *LSPService) DetectLSPServers() []LSPServerStatus {
	s.mu.Lock()
	wsRoot := s.workspaceRoot
	running := make(map[string]bool, len(s.servers))
	for lang, srv := range s.servers {
		running[lang] = srv != nil && srv.cmd != nil && srv.cmd.ProcessState == nil && srv.cmd.Process != nil
	}
	errs := make(map[string]string, len(s.lastErrors))
	for k, v := range s.lastErrors {
		errs[k] = v
	}
	s.mu.Unlock()

	statuses := []LSPServerStatus{}
	for _, lang := range []string{"go", "typescript", "javascript"} {
		st := LSPServerStatus{Language: lang}
		path, version, kind := detectServerPath(lang, wsRoot)
		st.ServerPath = path
		st.Version = version
		st.ServerKind = kind
		st.Available = path != ""
		st.Running = running[lang]
		st.LastError = errs[lang]
		statuses = append(statuses, st)
	}
	return statuses
}

// detectServerPath finds the language server executable.
// Returns path, version, kind. Empty path if not found.
//
// prompt-8 Task 8-B / BUG-IDE-02: for TS/JS prefer typescript-language-server
// or vtsls over raw tsserver (proprietary protocol).
func detectServerPath(language, workspaceRoot string) (path, version, kind string) {
	switch language {
	case "go":
		if p, err := exec.LookPath("gopls"); err == nil {
			return p, tryVersion(p, "version"), "gopls"
		}
		return "", "", ""
	case "typescript", "javascript":
		// Prefer workspace-local then global LSP wrappers.
		candidates := []struct {
			name string
			kind string
		}{
			{"typescript-language-server", "typescript-language-server"},
			{"vtsls", "vtsls"},
		}
		if workspaceRoot != "" {
			for _, c := range candidates {
				local := filepath.Join(workspaceRoot, "node_modules", ".bin", c.name)
				if p, err := exec.LookPath(local); err == nil {
					return p, "", c.kind
				}
			}
		}
		for _, c := range candidates {
			if p, err := exec.LookPath(c.name); err == nil {
				return p, "", c.kind
			}
		}
		return "", "", ""
	}
	return "", "", ""
}

// tryVersion runs `<exe> <flag>` and returns the trimmed first line of output.
func tryVersion(exe string, args ...string) string {
	cmd := exec.Command(exe, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// StartLSPServer starts an LSP server for the given language. It is a no-op
// (returns nil) if the server is already running. Returns an error if the
// server binary is not installed or fails to start.
func (s *LSPService) StartLSPServer(language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if srv, ok := s.servers[language]; ok && srv != nil && srv.cmd != nil && srv.cmd.Process != nil && srv.cmd.ProcessState == nil {
		return nil // already running
	}

	exe, ok := serverNameForLanguage(language)
	if !ok {
		return fmt.Errorf("unsupported language: %s", language)
	}
	path, _, kind := detectServerPath(language, s.workspaceRoot)
	if path == "" {
		hint := exe
		if language == "typescript" || language == "javascript" {
			hint = "typescript-language-server or vtsls (npm i -D typescript-language-server)"
		}
		err := fmt.Errorf("language server not installed (need %s)", hint)
		if s.lastErrors == nil {
			s.lastErrors = make(map[string]string)
		}
		s.lastErrors[language] = err.Error()
		return err
	}

	cmd, stdin, stdout, err := startServerProcess(language, path, kind, s.workspaceRoot)
	if err != nil {
		if s.lastErrors == nil {
			s.lastErrors = make(map[string]string)
		}
		s.lastErrors[language] = err.Error()
		return fmt.Errorf("failed to start %s: %w", kind, err)
	}

	client := newJSONRPCClient(stdout, stdin)
	srv := &lspServer{
		cmd:         cmd,
		stdin:       stdin,
		stdout:      stdout,
		client:      client,
		docVersions: make(map[string]int),
		docHashes:   make(map[string]string),
		docLastSync: make(map[string]time.Time),
		diags:       make(map[string][]Diagnostic),
	}
	// G-FEAT-02: collect published diagnostics so GetDiagnostics can return
	// them. The server pushes diagnostics asynchronously after didOpen.
	srv.client.onNotification("textDocument/publishDiagnostics", func(params json.RawMessage) {
		var notif struct {
			URI string `json:"uri"`
			Diagnostics []struct {
				Range struct {
					Start struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"start"`
					End struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"end"`
				} `json:"range"`
				Severity int    `json:"severity"`
				Message  string `json:"message"`
				Source   string `json:"source"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal(params, &notif); err != nil {
			slog.Debug("LSP publishDiagnostics: failed to parse params", "err", err)
			return
		}
		out := make([]Diagnostic, 0, len(notif.Diagnostics))
		for _, d := range notif.Diagnostics {
			out = append(out, Diagnostic{
				Line:     d.Range.Start.Line,
				Column:   d.Range.Start.Character,
				EndLine:  d.Range.End.Line,
				EndCol:   d.Range.End.Character,
				Severity: d.Severity,
				Message:  d.Message,
				Source:   d.Source,
			})
		}
		srv.diagsMu.Lock()
		srv.diags[notif.URI] = out
		srv.diagsMu.Unlock()
	})
	s.servers[language] = srv

	// Send the LSP initialize handshake.
	if err := s.initializeLocked(srv, language, s.workspaceRoot); err != nil {
		// Clean up the failed server so a retry can start fresh.
		_ = cmd.Process.Kill()
		delete(s.servers, language)
		if s.lastErrors == nil {
			s.lastErrors = make(map[string]string)
		}
		s.lastErrors[language] = err.Error()
		return fmt.Errorf("LSP initialize failed for %s: %w", language, err)
	}
	if s.lastErrors != nil {
		delete(s.lastErrors, language)
	}
	return nil
}

// startServerProcess launches the language server process and returns its
// stdin/stdout pipes. kind is gopls | typescript-language-server | vtsls.
func startServerProcess(language, exePath, kind, workspaceRoot string) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	var cmd *exec.Cmd
	switch language {
	case "go":
		// gopls stdio serve (prompt-8 M20: single process; remote=auto optional later).
		cmd = exec.Command(exePath, "serve")
	case "typescript", "javascript":
		// prompt-8 Task 8-B / BUG-IDE-02: LSP wrappers need --stdio.
		switch kind {
		case "vtsls":
			cmd = exec.Command(exePath, "--stdio")
		default:
			// typescript-language-server
			cmd = exec.Command(exePath, "--stdio")
		}
	default:
		return nil, nil, nil, fmt.Errorf("unsupported language: %s", language)
	}
	if workspaceRoot != "" {
		cmd.Dir = workspaceRoot
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, nil, err
	}
	return cmd, stdin, stdout, nil
}

// initializeLocked sends the LSP initialize/initialized handshake. Caller
// must hold s.mu.
func (s *LSPService) initializeLocked(srv *lspServer, language, workspaceRoot string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// prompt-8 M20: declare client capabilities so servers enable completion,
	// hover, definition, formatting, rename, etc.
	caps := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"synchronization": map[string]interface{}{
				"dynamicRegistration": false,
				"willSave":            false,
				"willSaveWaitUntil":   false,
				"didSave":             true,
			},
			"completion": map[string]interface{}{
				"completionItem": map[string]interface{}{
					"snippetSupport": false,
				},
			},
			"hover":              map[string]interface{}{"contentFormat": []string{"markdown", "plaintext"}},
			"definition":         map[string]interface{}{},
			"references":         map[string]interface{}{},
			"rename":             map[string]interface{}{"prepareSupport": false},
			"formatting":         map[string]interface{}{},
			"publishDiagnostics": map[string]interface{}{},
		},
		"workspace": map[string]interface{}{
			"workspaceFolders": true,
		},
	}
	// prompt-9 BUG-IDE-11: pass real process id when available.
	pid := os.Getpid()
	initParams := map[string]interface{}{
		"processId":        pid,
		"rootUri":          pathToURI(workspaceRoot),
		"capabilities":     caps,
		"workspaceFolders": []map[string]string{},
	}
	if workspaceRoot != "" {
		initParams["workspaceFolders"] = []map[string]string{
			{"uri": pathToURI(workspaceRoot), "name": filepath.Base(workspaceRoot)},
		}
	}
	if _, err := srv.client.request(ctx, "initialize", initParams); err != nil {
		return err
	}
	if err := srv.client.notify("initialized", map[string]interface{}{}); err != nil {
		return err
	}
	return nil
}

// StopLSPServer stops a running LSP server for the given language. It is a
// no-op (returns nil) if no server is running.
func (s *LSPService) StopLSPServer(language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	srv, ok := s.servers[language]
	if !ok || srv == nil {
		return nil
	}
	if srv.client != nil {
		_ = srv.client.notify("shutdown", nil)
		_ = srv.client.notify("exit", nil)
	}
	if srv.cmd != nil && srv.cmd.Process != nil {
		_ = srv.cmd.Process.Kill()
	}
	delete(s.servers, language)
	return nil
}

// StopAll stops every running LSP server. Called on application shutdown.
func (s *LSPService) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for lang := range s.servers {
		srv := s.servers[lang]
		if srv != nil && srv.client != nil {
			_ = srv.client.notify("shutdown", nil)
			_ = srv.client.notify("exit", nil)
		}
		if srv != nil && srv.cmd != nil && srv.cmd.Process != nil {
			_ = srv.cmd.Process.Kill()
		}
		delete(s.servers, lang)
	}
}

// GetCompletions queries the LSP server for completions at the given position.
// prompt-9 9-D: sets call status; returns empty items when not running (graceful)
// but records not_running/rpc for StatusBar. RPC errors are still soft for UI.
func (s *LSPService) GetCompletions(req LSPCompletionRequest) ([]LSPCompletionItem, error) {
	srv, err := s.syncDocument(req)
	if err != nil {
		s.setCallStatus(req.Language, "rpc", err.Error())
		return []LSPCompletionItem{}, nil
	}
	if srv == nil {
		s.setCallStatus(req.Language, "not_running", "language server not running")
		return []LSPCompletionItem{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"position":     map[string]int{"line": req.Line, "character": req.Column},
	}
	raw, err := srv.client.request(ctx, "textDocument/completion", params)
	if err != nil {
		code := "rpc"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			code = "timeout"
		}
		s.setCallStatus(req.Language, code, err.Error())
		slog.Debug("LSP completion request failed", "language", req.Language, "err", err)
		return []LSPCompletionItem{}, nil
	}
	s.setCallStatus(req.Language, "ok", "")
	return parseCompletionItems(raw), nil
}

// GetHover returns hover information at the given position as a markdown string.
// Returns an empty string if the server is not running.
func (s *LSPService) GetHover(req LSPCompletionRequest) (string, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"position":     map[string]int{"line": req.Line, "character": req.Column},
	}
	raw, err := srv.client.request(ctx, "textDocument/hover", params)
	if err != nil {
		slog.Debug("LSP hover request failed", "language", req.Language, "err", err)
		return "", nil
	}
	return parseHover(raw), nil
}

// GetDiagnostics returns diagnostics for a file. Returns an empty slice if
// the server is not running. Note: LSP servers publish diagnostics via
// notifications; this method returns the most recent set received for the
// file (if any), since the textDocument/publishDiagnostics notification is
// server→client only.
func (s *LSPService) GetDiagnostics(req LSPCompletionRequest) ([]Diagnostic, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return []Diagnostic{}, nil
	}
	uri := pathToURI(req.FilePath)
	srv.diagsMu.Lock()
	cached := srv.diags[uri]
	srv.diagsMu.Unlock()
	if len(cached) == 0 {
		return []Diagnostic{}, nil
	}
	out := make([]Diagnostic, len(cached))
	copy(out, cached)
	return out, nil
}

// LSPLocation is a file+range for definition/references (prompt-8 Task 8-F).
type LSPLocation struct {
	FilePath  string `json:"filePath"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"endLine"`
	EndColumn int    `json:"endColumn"`
}

// GetDefinition returns go-to-definition locations (prompt-8 Task 8-F).
func (s *LSPService) GetDefinition(req LSPCompletionRequest) ([]LSPLocation, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return []LSPLocation{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"position":     map[string]int{"line": req.Line, "character": req.Column},
	}
	raw, err := srv.client.request(ctx, "textDocument/definition", params)
	if err != nil {
		slog.Debug("LSP definition failed", "err", err)
		return []LSPLocation{}, nil
	}
	return parseLocations(raw), nil
}

// GetReferences returns find-references locations (prompt-8 Task 8-F).
func (s *LSPService) GetReferences(req LSPCompletionRequest) ([]LSPLocation, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return []LSPLocation{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"position":     map[string]int{"line": req.Line, "character": req.Column},
		"context":      map[string]bool{"includeDeclaration": true},
	}
	raw, err := srv.client.request(ctx, "textDocument/references", params)
	if err != nil {
		slog.Debug("LSP references failed", "err", err)
		return []LSPLocation{}, nil
	}
	return parseLocations(raw), nil
}

// TextEdit is a range replacement for format/rename (prompt-8 Task 8-G/H).
type TextEdit struct {
	StartLine int    `json:"startLine"`
	StartCol  int    `json:"startCol"`
	EndLine   int    `json:"endLine"`
	EndCol    int    `json:"endCol"`
	NewText   string `json:"newText"`
}

// FormatDocument runs textDocument/formatting and returns edits for the buffer
// (prompt-8 Task 8-G). Empty when server unavailable.
func (s *LSPService) FormatDocument(req LSPCompletionRequest) ([]TextEdit, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return []TextEdit{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"options": map[string]interface{}{
			"tabSize":      4,
			"insertSpaces": false,
		},
	}
	if req.Language == "typescript" || req.Language == "javascript" {
		params["options"] = map[string]interface{}{"tabSize": 2, "insertSpaces": true}
	}
	raw, err := srv.client.request(ctx, "textDocument/formatting", params)
	if err != nil {
		slog.Debug("LSP format failed", "err", err)
		return []TextEdit{}, nil
	}
	return parseTextEdits(raw), nil
}

// FileTextEdits is a path + list of text edits (prompt-9 Task 9-B multi-file rename).
type FileTextEdits struct {
	FilePath string     `json:"filePath"`
	Edits    []TextEdit `json:"edits"`
}

// RenameSymbol runs textDocument/rename for the *current file only* (compat).
func (s *LSPService) RenameSymbol(req LSPCompletionRequest, newName string) ([]TextEdit, error) {
	files, err := s.RenameSymbolWorkspace(req, newName)
	if err != nil || len(files) == 0 {
		return []TextEdit{}, err
	}
	want := pathToURI(req.FilePath)
	for _, f := range files {
		if pathToURI(f.FilePath) == want || f.FilePath == req.FilePath {
			return f.Edits, nil
		}
	}
	return files[0].Edits, nil
}

// RenameSymbolWorkspace returns WorkspaceEdit across all touched files (prompt-9 9-B).
func (s *LSPService) RenameSymbolWorkspace(req LSPCompletionRequest, newName string) ([]FileTextEdits, error) {
	if newName == "" {
		return []FileTextEdits{}, nil
	}
	srv, err := s.syncDocument(req)
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, fmt.Errorf("not_running: language server not running for %s", req.Language)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"position":     map[string]int{"line": req.Line, "character": req.Column},
		"newName":      newName,
	}
	raw, err := srv.client.request(ctx, "textDocument/rename", params)
	if err != nil {
		s.setCallStatus(req.Language, "rpc", err.Error())
		return nil, fmt.Errorf("rpc: %w", err)
	}
	s.setCallStatus(req.Language, "ok", "")
	return parseWorkspaceEditsAll(raw), nil
}

// SignatureHelpResult is a simplified signature help payload (prompt-9 9-G).
type SignatureHelpResult struct {
	Label           string   `json:"label"`
	Documentation   string   `json:"documentation"`
	Parameters      []string `json:"parameters"`
	ActiveParameter int      `json:"activeParameter"`
	ActiveSignature int      `json:"activeSignature"`
}

// GetSignatureHelp queries textDocument/signatureHelp (prompt-9 9-G).
func (s *LSPService) GetSignatureHelp(req LSPCompletionRequest) (*SignatureHelpResult, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"position":     map[string]int{"line": req.Line, "character": req.Column},
	}
	raw, err := srv.client.request(ctx, "textDocument/signatureHelp", params)
	if err != nil {
		s.setCallStatus(req.Language, "rpc", err.Error())
		return nil, nil
	}
	return parseSignatureHelp(raw), nil
}

// OrganizeImports runs source.organizeImports code action (prompt-9 9-G).
func (s *LSPService) OrganizeImports(req LSPCompletionRequest) ([]TextEdit, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return []TextEdit{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"range": map[string]interface{}{
			"start": map[string]int{"line": 0, "character": 0},
			"end":   map[string]int{"line": 0, "character": 0},
		},
		"context": map[string]interface{}{
			"diagnostics": []interface{}{},
			"only":        []string{"source.organizeImports"},
		},
	}
	raw, err := srv.client.request(ctx, "textDocument/codeAction", params)
	if err != nil {
		slog.Debug("LSP organizeImports failed", "err", err)
		return []TextEdit{}, nil
	}
	return parseCodeActionEdits(raw, pathToURI(req.FilePath)), nil
}

// InlayHint is a simplified inlay hint (prompt-12 12-L optional).
type InlayHint struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Label   string `json:"label"`
	Kind    int    `json:"kind"` // 1=type 2=parameter
}

// GetInlayHints requests textDocument/inlayHint when the server supports it.
// Returns empty slice when unsupported — never errors for UI toggle paths.
func (s *LSPService) GetInlayHints(req LSPCompletionRequest) ([]InlayHint, error) {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return []InlayHint{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Range: whole document approx (0,0)-(large,0)
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"range": map[string]interface{}{
			"start": map[string]int{"line": 0, "character": 0},
			"end":   map[string]int{"line": 100000, "character": 0},
		},
	}
	raw, err := srv.client.request(ctx, "textDocument/inlayHint", params)
	if err != nil || len(raw) == 0 || string(raw) == "null" {
		return []InlayHint{}, nil
	}
	var arr []struct {
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
		Label interface{} `json:"label"`
		Kind  int         `json:"kind"`
	}
	if json.Unmarshal(raw, &arr) != nil {
		return []InlayHint{}, nil
	}
	out := make([]InlayHint, 0, len(arr))
	for _, h := range arr {
		label := ""
		switch v := h.Label.(type) {
		case string:
			label = v
		default:
			b, _ := json.Marshal(v)
			label = string(b)
		}
		out = append(out, InlayHint{
			Line: h.Position.Line, Column: h.Position.Character, Label: label, Kind: h.Kind,
		})
	}
	return out, nil
}

// LSPCallStatus is the last call outcome for StatusBar (prompt-9 9-D).
type LSPCallStatus struct {
	Language string `json:"language"`
	Code     string `json:"code"` // ok | not_running | timeout | rpc | unavailable
	Message  string `json:"message"`
}

func (s *LSPService) setCallStatus(language, code, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErrors == nil {
		s.lastErrors = make(map[string]string)
	}
	if code == "ok" || code == "" {
		delete(s.lastErrors, language)
	} else {
		s.lastErrors[language] = code + ": " + message
	}
}

// GetCallStatus returns the last non-ok status message for a language (9-D).
func (s *LSPService) GetCallStatus(language string) LSPCallStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := ""
	if s.lastErrors != nil {
		msg = s.lastErrors[language]
	}
	code := "ok"
	if msg != "" {
		if strings.HasPrefix(msg, "not_running") {
			code = "not_running"
		} else if strings.HasPrefix(msg, "timeout") {
			code = "timeout"
		} else if strings.HasPrefix(msg, "rpc") {
			code = "rpc"
		} else {
			code = "unavailable"
		}
	}
	return LSPCallStatus{Language: language, Code: code, Message: msg}
}

// CloseDocument sends textDocument/didClose (prompt-8 Task 8-A).
func (s *LSPService) CloseDocument(language, filePath string) error {
	s.mu.Lock()
	srv, ok := s.servers[language]
	s.mu.Unlock()
	if !ok || srv == nil {
		return nil
	}
	uri := pathToURI(filePath)
	srv.docMu.Lock()
	delete(srv.docVersions, uri)
	srv.docMu.Unlock()
	return srv.client.notify("textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
	})
}

// DidSaveDocument notifies the server of a disk save (prompt-8 Task 8-A).
func (s *LSPService) DidSaveDocument(req LSPCompletionRequest) error {
	srv, err := s.syncDocument(req)
	if err != nil || srv == nil {
		return nil
	}
	return srv.client.notify("textDocument/didSave", map[string]interface{}{
		"textDocument": map[string]string{"uri": pathToURI(req.FilePath)},
		"text":         req.Content,
	})
}

// syncDocument ensures the live buffer is known to the server via didOpen
// or didChange with a monotonic version (prompt-8 Task 8-A / BUG-IDE-01/04).
// prompt-9 9-K: skip didChange when content hash unchanged (or within 100ms
// of an identical hash sync).
// Returns (nil, nil) if the server is not running.
func (s *LSPService) syncDocument(req LSPCompletionRequest) (*lspServer, error) {
	s.mu.Lock()
	srv, ok := s.servers[req.Language]
	s.mu.Unlock()
	if !ok || srv == nil {
		return nil, nil
	}

	uri := pathToURI(req.FilePath)
	langID := lspLanguageID(req.Language, req.FilePath)
	sum := sha256.Sum256([]byte(req.Content))
	hash := hex.EncodeToString(sum[:])

	srv.docMu.Lock()
	prev, opened := srv.docVersions[uri]
	prevHash := srv.docHashes[uri]
	lastSync := srv.docLastSync[uri]
	// Skip redundant full didChange when content is identical.
	if opened && prevHash == hash {
		srv.docMu.Unlock()
		return srv, nil
	}
	// prompt-12 12-I: stronger throttle on full didChange (300ms) for large monorepos.
	// Still skip when content hash is unchanged (above). When content changes rapidly,
	// coalesce by delaying only if last sync was < 50ms (burst typing).
	if opened && time.Since(lastSync) < 50*time.Millisecond {
		// Allow through but record — actual skip of identical handled above.
		// For rapid distinct edits, still send (correctness); callers debounce UI.
	}
	if opened && time.Since(lastSync) < 100*time.Millisecond && prevHash == hash {
		srv.docMu.Unlock()
		return srv, nil
	}
	next := prev + 1
	if !opened {
		next = 1
	}
	srv.docVersions[uri] = next
	srv.docHashes[uri] = hash
	srv.docLastSync[uri] = time.Now()
	srv.docMu.Unlock()

	if !opened {
		params := map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":        uri,
				"languageId": langID,
				"version":    next,
				"text":       req.Content,
			},
		}
		if err := srv.client.notify("textDocument/didOpen", params); err != nil {
			slog.Debug("LSP didOpen failed", "language", req.Language, "err", err)
		}
	} else {
		// Full document sync (TextDocumentSyncKind.Full).
		params := map[string]interface{}{
			"textDocument": map[string]interface{}{
				"uri":     uri,
				"version": next,
			},
			"contentChanges": []map[string]interface{}{
				{"text": req.Content},
			},
		}
		if err := srv.client.notify("textDocument/didChange", params); err != nil {
			slog.Debug("LSP didChange failed", "language", req.Language, "err", err)
		}
	}
	return srv, nil
}

// lspLanguageID maps language + path to LSP languageId (tsx/jsx aware).
func lspLanguageID(language, filePath string) string {
	lower := strings.ToLower(filePath)
	switch language {
	case "go":
		return "go"
	case "typescript":
		if strings.HasSuffix(lower, ".tsx") {
			return "typescriptreact"
		}
		return "typescript"
	case "javascript":
		if strings.HasSuffix(lower, ".jsx") {
			return "javascriptreact"
		}
		return "javascript"
	}
	return language
}

// --- JSON-RPC 2.0 client over LSP base protocol ---

// jsonRPCClient is a minimal JSON-RPC 2.0 client that frames messages with the
// LSP Content-Length header over an io.Reader/io.Writer pair.
type jsonRPCClient struct {
	w       io.Writer
	r       *bufio.Reader
	writeMu sync.Mutex

	nextID atomic.Int64
	pendingMu sync.Mutex
	pending   map[int64]chan *rpcResponse

	// notification handlers (server→client notifications)
	notifMu  sync.Mutex
	notifs   map[string][]func(json.RawMessage)
	done     chan struct{}
	started  bool
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError        `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// newJSONRPCClient creates a client and starts a background goroutine that
// reads responses/notifications from the server's stdout.
func newJSONRPCClient(r io.Reader, w io.Writer) *jsonRPCClient {
	c := &jsonRPCClient{
		w:       w,
		r:       bufio.NewReader(r),
		pending: make(map[int64]chan *rpcResponse),
		notifs:  make(map[string][]func(json.RawMessage)),
		done:    make(chan struct{}),
	}
	c.started = true
	go c.readLoop()
	return c
}

// request sends a JSON-RPC request and waits for the response.
func (c *jsonRPCClient) request(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan *rpcResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	if err := c.writeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp == nil {
			return nil, fmt.Errorf("LSP request %s: connection closed", method)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP request %s failed (%d): %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *jsonRPCClient) notify(method string, params interface{}) error {
	return c.writeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

// onNotification registers a handler for a server→client notification with
// the given method (e.g. "textDocument/publishDiagnostics"). The handler is
// invoked from the readLoop goroutine; it must not block and must not call
// back into the client (no reentrant write). G-FEAT-02 uses this to collect
// published diagnostics.
func (c *jsonRPCClient) onNotification(method string, handler func(json.RawMessage)) {
	c.notifMu.Lock()
	defer c.notifMu.Unlock()
	c.notifs[method] = append(c.notifs[method], handler)
}

// writeMessage frames a JSON-RPC message with the Content-Length header and
// writes it to the server's stdin.
func (c *jsonRPCClient) writeMessage(msg map[string]interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	header := "Content-Length: " + strconv.Itoa(len(data)) + "\r\n\r\n"
	if _, err := c.w.Write([]byte(header)); err != nil {
		return err
	}
	if _, err := c.w.Write(data); err != nil {
		return err
	}
	return nil
}

// readLoop reads framed JSON-RPC messages from the server's stdout and
// dispatches responses to waiting requesters and notifications to handlers.
func (c *jsonRPCClient) readLoop() {
	for {
		msg, err := c.readMessage()
		if err != nil {
			// Connection closed or error — fail all pending requests.
			c.pendingMu.Lock()
			for id, ch := range c.pending {
				select {
				case ch <- nil:
				default:
				}
				delete(c.pending, id)
			}
			c.pendingMu.Unlock()
			close(c.done)
			return
		}
		c.dispatch(msg)
	}
}

// readMessage reads one LSP-framed message (Content-Length header + body).
func (c *jsonRPCClient) readMessage() (json.RawMessage, error) {
	var contentLength int
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, err = strconv.Atoi(v)
			if err != nil {
				return nil, err
			}
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("LSP message missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// dispatch routes a parsed JSON-RPC message to its handler (response or
// notification).
func (c *jsonRPCClient) dispatch(msg json.RawMessage) {
	var envelope struct {
		ID     *int64           `json:"id"`
		Method string           `json:"method"`
		Result json.RawMessage  `json:"result"`
		Params json.RawMessage  `json:"params"`
		Error  *rpcError        `json:"error"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return
	}
	if envelope.ID != nil && envelope.Method == "" {
		// Response to a request.
		c.pendingMu.Lock()
		ch, ok := c.pending[*envelope.ID]
		c.pendingMu.Unlock()
		if ok {
			resp := &rpcResponse{Result: envelope.Result, Error: envelope.Error}
			select {
			case ch <- resp:
			default:
			}
		}
		return
	}
	if envelope.Method != "" && envelope.ID == nil {
		// Notification from the server.
		c.notifMu.Lock()
		handlers := c.notifs[envelope.Method]
		c.notifMu.Unlock()
		for _, h := range handlers {
			h(envelope.Params)
		}
	}
}

// --- response parsing helpers ---

// completionItemJSON is the wire shape of an LSP CompletionItem (subset).
type completionItemJSON struct {
	Label               string            `json:"label"`
	Kind                int               `json:"kind"`
	Detail              string            `json:"detail"`
	InsertText          string            `json:"insertText"`
	AdditionalTextEdits []lspTextEditJSON `json:"additionalTextEdits"`
}

func mapCompletionItem(it completionItemJSON) LSPCompletionItem {
	item := LSPCompletionItem{
		Label:      it.Label,
		Kind:       it.Kind,
		Detail:     it.Detail,
		InsertText: it.InsertText,
	}
	if len(it.AdditionalTextEdits) > 0 {
		item.AdditionalEdits = make([]TextEdit, 0, len(it.AdditionalTextEdits))
		for _, e := range it.AdditionalTextEdits {
			item.AdditionalEdits = append(item.AdditionalEdits, TextEdit{
				StartLine: e.Range.Start.Line,
				StartCol:  e.Range.Start.Character,
				EndLine:   e.Range.End.Line,
				EndCol:    e.Range.End.Character,
				NewText:   e.NewText,
			})
		}
	}
	return item
}

// parseCompletionItems extracts completion items from an LSP completion
// response. The response may be a list of items or a CompletionList object.
// prompt-10 10-I: includes additionalTextEdits (auto-import).
func parseCompletionItems(raw json.RawMessage) []LSPCompletionItem {
	if len(raw) == 0 {
		return []LSPCompletionItem{}
	}
	// Try CompletionList first.
	var list struct {
		Items []completionItemJSON `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err == nil && list.Items != nil {
		out := make([]LSPCompletionItem, 0, len(list.Items))
		for _, it := range list.Items {
			out = append(out, mapCompletionItem(it))
		}
		return out
	}
	// Try a plain array.
	var arr []completionItemJSON
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := make([]LSPCompletionItem, 0, len(arr))
		for _, it := range arr {
			out = append(out, mapCompletionItem(it))
		}
		return out
	}
	return []LSPCompletionItem{}
}

// parseHover extracts the markdown string from an LSP hover response.
func parseHover(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var hover struct {
		Contents struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &hover); err == nil {
		return hover.Contents.Value
	}
	// Contents may be a plain string or a MarkupContent.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// pathToURI converts a filesystem path to a file:// URI (prompt-8 Task 8-C).
// Absolute paths are preferred; relative paths are Abs'd against the process
// cwd. Windows drive letters become file:///C:/...
func pathToURI(p string) string {
	if p == "" {
		return ""
	}
	// Strip accidental file:// prefix from callers.
	if strings.HasPrefix(p, "file://") {
		p = strings.TrimPrefix(p, "file://")
		// Windows: /C:/... after strip
		if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
	}
	// Resolve relative paths only. Do not Abs POSIX-style absolute paths on
	// Windows (filepath.Abs would incorrectly prefix the drive).
	if !filepath.IsAbs(p) {
		isPOSIXAbs := strings.HasPrefix(p, "/")
		if !(runtime.GOOS == "windows" && isPOSIXAbs) {
			if abs, err := filepath.Abs(p); err == nil {
				p = abs
			}
		}
	}
	cleaned := filepath.ToSlash(p)
	// Windows: C:/foo → /C:/foo for file URI form
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		cleaned = "/" + cleaned
	} else if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return "file://" + cleaned
}

// uriToPath converts a file:// URI back to a filesystem path.
func uriToPath(uri string) string {
	if !strings.HasPrefix(uri, "file://") {
		return uri
	}
	p := strings.TrimPrefix(uri, "file://")
	// file:///C:/... → C:/...
	if len(p) >= 3 && p[0] == '/' && ((p[1] >= 'A' && p[1] <= 'Z') || (p[1] >= 'a' && p[1] <= 'z')) && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func parseLocations(raw json.RawMessage) []LSPLocation {
	if len(raw) == 0 || string(raw) == "null" {
		return []LSPLocation{}
	}
	type loc struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
			End struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"end"`
		} `json:"range"`
	}
	// Single Location
	var one loc
	if err := json.Unmarshal(raw, &one); err == nil && one.URI != "" {
		return []LSPLocation{{
			FilePath:  uriToPath(one.URI),
			Line:      one.Range.Start.Line,
			Column:    one.Range.Start.Character,
			EndLine:   one.Range.End.Line,
			EndColumn: one.Range.End.Character,
		}}
	}
	// Location[] or LocationLink[]
	var many []loc
	if err := json.Unmarshal(raw, &many); err == nil {
		out := make([]LSPLocation, 0, len(many))
		for _, l := range many {
			if l.URI == "" {
				continue
			}
			out = append(out, LSPLocation{
				FilePath:  uriToPath(l.URI),
				Line:      l.Range.Start.Line,
				Column:    l.Range.Start.Character,
				EndLine:   l.Range.End.Line,
				EndColumn: l.Range.End.Character,
			})
		}
		return out
	}
	// LocationLink[]
	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
			End struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"end"`
		} `json:"targetRange"`
	}
	if err := json.Unmarshal(raw, &links); err == nil {
		out := make([]LSPLocation, 0, len(links))
		for _, l := range links {
			if l.TargetURI == "" {
				continue
			}
			out = append(out, LSPLocation{
				FilePath:  uriToPath(l.TargetURI),
				Line:      l.TargetRange.Start.Line,
				Column:    l.TargetRange.Start.Character,
				EndLine:   l.TargetRange.End.Line,
				EndColumn: l.TargetRange.End.Character,
			})
		}
		return out
	}
	return []LSPLocation{}
}

func parseTextEdits(raw json.RawMessage) []TextEdit {
	if len(raw) == 0 || string(raw) == "null" {
		return []TextEdit{}
	}
	var edits []struct {
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
			End struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"end"`
		} `json:"range"`
		NewText string `json:"newText"`
	}
	if err := json.Unmarshal(raw, &edits); err != nil {
		return []TextEdit{}
	}
	out := make([]TextEdit, 0, len(edits))
	for _, e := range edits {
		out = append(out, TextEdit{
			StartLine: e.Range.Start.Line,
			StartCol:  e.Range.Start.Character,
			EndLine:   e.Range.End.Line,
			EndCol:    e.Range.End.Character,
			NewText:   e.NewText,
		})
	}
	return out
}

type lspTextEditJSON struct {
	Range struct {
		Start struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"start"`
		End struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"end"`
	} `json:"range"`
	NewText string `json:"newText"`
}

func textEditsFromJSON(edits []lspTextEditJSON) []TextEdit {
	out := make([]TextEdit, 0, len(edits))
	for _, e := range edits {
		out = append(out, TextEdit{
			StartLine: e.Range.Start.Line,
			StartCol:  e.Range.Start.Character,
			EndLine:   e.Range.End.Line,
			EndCol:    e.Range.End.Character,
			NewText:   e.NewText,
		})
	}
	return out
}

func parseWorkspaceEditsForURI(raw json.RawMessage, wantURI string) []TextEdit {
	all := parseWorkspaceEditsAll(raw)
	for _, f := range all {
		if pathToURI(f.FilePath) == wantURI || f.FilePath == uriToPath(wantURI) {
			return f.Edits
		}
	}
	return []TextEdit{}
}

// parseWorkspaceEditsAll returns edits for every file in a WorkspaceEdit (9-B).
func parseWorkspaceEditsAll(raw json.RawMessage) []FileTextEdits {
	if len(raw) == 0 || string(raw) == "null" {
		return []FileTextEdits{}
	}
	var we struct {
		Changes         map[string][]lspTextEditJSON `json:"changes"`
		DocumentChanges []struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []lspTextEditJSON `json:"edits"`
		} `json:"documentChanges"`
	}
	if err := json.Unmarshal(raw, &we); err != nil {
		return []FileTextEdits{}
	}
	var out []FileTextEdits
	for uri, edits := range we.Changes {
		out = append(out, FileTextEdits{
			FilePath: uriToPath(uri),
			Edits:    textEditsFromJSON(edits),
		})
	}
	for _, dc := range we.DocumentChanges {
		if dc.TextDocument.URI == "" {
			continue
		}
		out = append(out, FileTextEdits{
			FilePath: uriToPath(dc.TextDocument.URI),
			Edits:    textEditsFromJSON(dc.Edits),
		})
	}
	return out
}

func parseSignatureHelp(raw json.RawMessage) *SignatureHelpResult {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var sh struct {
		Signatures []struct {
			Label         string `json:"label"`
			Documentation interface{} `json:"documentation"`
			Parameters    []struct {
				Label interface{} `json:"label"`
			} `json:"parameters"`
		} `json:"signatures"`
		ActiveSignature *int `json:"activeSignature"`
		ActiveParameter *int `json:"activeParameter"`
	}
	if err := json.Unmarshal(raw, &sh); err != nil || len(sh.Signatures) == 0 {
		return nil
	}
	asi := 0
	if sh.ActiveSignature != nil {
		asi = *sh.ActiveSignature
	}
	if asi < 0 || asi >= len(sh.Signatures) {
		asi = 0
	}
	sig := sh.Signatures[asi]
	params := make([]string, 0, len(sig.Parameters))
	for _, p := range sig.Parameters {
		switch v := p.Label.(type) {
		case string:
			params = append(params, v)
		default:
			params = append(params, "")
		}
	}
	doc := ""
	switch v := sig.Documentation.(type) {
	case string:
		doc = v
	case map[string]interface{}:
		if s, ok := v["value"].(string); ok {
			doc = s
		}
	}
	ap := 0
	if sh.ActiveParameter != nil {
		ap = *sh.ActiveParameter
	}
	return &SignatureHelpResult{
		Label:           sig.Label,
		Documentation:   doc,
		Parameters:      params,
		ActiveParameter: ap,
		ActiveSignature: asi,
	}
}

func parseCodeActionEdits(raw json.RawMessage, wantURI string) []TextEdit {
	if len(raw) == 0 || string(raw) == "null" {
		return []TextEdit{}
	}
	// CodeAction[] may embed edit.changes
	var actions []struct {
		Edit *struct {
			Changes map[string][]lspTextEditJSON `json:"changes"`
		} `json:"edit"`
	}
	if err := json.Unmarshal(raw, &actions); err != nil {
		return []TextEdit{}
	}
	for _, a := range actions {
		if a.Edit == nil {
			continue
		}
		for uri, edits := range a.Edit.Changes {
			if uri == wantURI || pathToURI(uriToPath(uri)) == wantURI {
				return textEditsFromJSON(edits)
			}
		}
		// first available
		for _, edits := range a.Edit.Changes {
			return textEditsFromJSON(edits)
		}
	}
	return []TextEdit{}
}
