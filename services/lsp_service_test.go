package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLSPService_DetectLSPServers_ReturnsAllLanguages verifies that
// DetectLSPServers returns a status entry for every supported language
// (go, typescript, javascript), regardless of whether the servers are
// installed. Available/Running reflect the actual environment.
func TestLSPService_DetectLSPServers_ReturnsAllLanguages(t *testing.T) {
	svc := NewLSPService("")
	statuses := svc.DetectLSPServers()

	if len(statuses) != 3 {
		t.Fatalf("expected 3 status entries (go/typescript/javascript), got %d", len(statuses))
	}

	seen := map[string]bool{}
	for _, st := range statuses {
		seen[st.Language] = true
		switch st.Language {
		case "go", "typescript", "javascript":
		default:
			t.Errorf("unexpected language %q in status", st.Language)
		}
	}
	for _, lang := range []string{"go", "typescript", "javascript"} {
		if !seen[lang] {
			t.Errorf("missing status for language %q", lang)
		}
	}

	// None should be running since we never started any server.
	for _, st := range statuses {
		if st.Running {
			t.Errorf("language %q should not be Running on a fresh service", st.Language)
		}
	}
}

// TestLSPService_DetectLSPServers_GoReflectsGoplsAvailability checks that the
// "go" status's Available flag matches whether gopls is on PATH. This test
// skips gracefully if gopls is not installed.
func TestLSPService_DetectLSPServers_GoReflectsGoplsAvailability(t *testing.T) {
	svc := NewLSPService("")
	statuses := svc.DetectLSPServers()

	var goStatus LSPServerStatus
	for _, st := range statuses {
		if st.Language == "go" {
			goStatus = st
		}
	}

	_, err := exec.LookPath("gopls")
	goplsInstalled := err == nil

	if goStatus.Available != goplsInstalled {
		t.Errorf("go Available=%v but goplsInstalled=%v", goStatus.Available, goplsInstalled)
	}
	if goplsInstalled && goStatus.ServerPath == "" {
		t.Errorf("gopls is installed but ServerPath is empty")
	}
}

// TestLSPService_GetCompletions_EmptyWhenNotRunning verifies the graceful
// fallback: GetCompletions returns an empty (non-nil) slice and no error when
// no LSP server is running for the requested language.
func TestLSPService_GetCompletions_EmptyWhenNotRunning(t *testing.T) {
	svc := NewLSPService("")
	items, err := svc.GetCompletions(LSPCompletionRequest{
		Language: "go",
		FilePath: "/tmp/main.go",
		Line:     0,
		Column:   0,
		Content:  "package main\n",
	})
	if err != nil {
		t.Fatalf("expected no error when server not running, got: %v", err)
	}
	if items == nil {
		t.Fatal("expected non-nil items slice when server not running")
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items when server not running, got %d", len(items))
	}
}

// TestLSPService_GetHover_EmptyWhenNotRunning verifies GetHover returns an
// empty string and no error when no server is running.
func TestLSPService_GetHover_EmptyWhenNotRunning(t *testing.T) {
	svc := NewLSPService("")
	hover, err := svc.GetHover(LSPCompletionRequest{
		Language: "typescript",
		FilePath: "/tmp/a.ts",
		Line:     0,
		Column:   0,
		Content:  "const x = 1;\n",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if hover != "" {
		t.Errorf("expected empty hover, got %q", hover)
	}
}

// TestLSPService_GetDiagnostics_EmptyWhenNotRunning verifies GetDiagnostics
// returns an empty slice and no error when no server is running.
func TestLSPService_GetDiagnostics_EmptyWhenNotRunning(t *testing.T) {
	svc := NewLSPService("")
	diags, err := svc.GetDiagnostics(LSPCompletionRequest{
		Language: "javascript",
		FilePath: "/tmp/a.js",
		Line:     0,
		Column:   0,
		Content:  "const x = 1;\n",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if diags == nil {
		t.Fatal("expected non-nil diagnostics slice")
	}
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
}

// TestLSPService_StartLSPServer_ErrorWhenNotInstalled verifies that
// StartLSPServer returns an error (not a panic) when the language server is
// not installed. Skips if gopls happens to be installed.
func TestLSPService_StartLSPServer_ErrorWhenNotInstalled(t *testing.T) {
	// Pick a language whose server is guaranteed not installed by using an
	// unsupported language name.
	svc := NewLSPService("")
	err := svc.StartLSPServer("cobol")
	if err == nil {
		t.Fatal("expected error for unsupported language, got nil")
	}
}

// TestLSPService_StopLSPServer_NoopWhenNotRunning verifies StopLSPServer is a
// no-op (returns nil) when no server is running.
func TestLSPService_StopLSPServer_NoopWhenNotRunning(t *testing.T) {
	svc := NewLSPService("")
	if err := svc.StopLSPServer("go"); err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
	// Stopping again should still be a no-op.
	if err := svc.StopLSPServer("go"); err != nil {
		t.Errorf("expected nil error on second stop, got: %v", err)
	}
}

// TestLSPService_SetWorkspaceRoot_NoopOnSameRoot verifies that setting the
// same workspace root does nothing (and doesn't panic on empty servers map).
func TestLSPService_SetWorkspaceRoot_NoopOnSameRoot(t *testing.T) {
	svc := NewLSPService("/tmp")
	svc.SetWorkspaceRoot("/tmp") // same root — should be a no-op
	svc.SetWorkspaceRoot("/other") // different root — stops nothing (no servers)
}

// TestLSP_pathToURI verifies file:// URI construction (prompt-8 Task 8-C).
func TestLSP_pathToURI(t *testing.T) {
	if got := pathToURI(""); got != "" {
		t.Errorf("empty → %q", got)
	}
	// POSIX absolute (preserved even on Windows).
	if got := pathToURI("/home/user/main.go"); got != "file:///home/user/main.go" {
		t.Errorf("posix abs: got %q", got)
	}
	// Windows drive path.
	if got := pathToURI(`C:\Users\main.go`); got != "file:///C:/Users/main.go" {
		t.Errorf("windows drive: got %q", got)
	}
	// Absolute path round-trip via Abs for a real temp file path.
	dir := t.TempDir()
	p := filepath.Join(dir, "main.go")
	got := pathToURI(p)
	if !strings.HasPrefix(got, "file://") {
		t.Fatalf("want file:// prefix, got %q", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("uri missing basename: %q", got)
	}
}

// TestLSP_syncDocument_DidOpenThenDidChange uses an in-process mock LSP server
// (prompt-8 Task 8-E) to assert didOpen on first sync and didChange on second.
func TestLSP_syncDocument_DidOpenThenDidChange(t *testing.T) {
	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()

	var mu sync.Mutex
	var methods []string
	var versions []int
	var texts []string

	// Fake LSP server: respond to initialize; record notifications.
	go func() {
		defer serverW.Close()
		r := bufio.NewReader(serverR)
		for {
			// Read Content-Length framed message
			var contentLength int
			for {
				line, err := r.ReadString('\n')
				if err != nil {
					return
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				if strings.HasPrefix(line, "Content-Length:") {
					v := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
					contentLength, _ = strconv.Atoi(v)
				}
			}
			if contentLength <= 0 {
				return
			}
			body := make([]byte, contentLength)
			if _, err := io.ReadFull(r, body); err != nil {
				return
			}
			var msg map[string]interface{}
			if json.Unmarshal(body, &msg) != nil {
				continue
			}
			method, _ := msg["method"].(string)
			mu.Lock()
			methods = append(methods, method)
			if method == "textDocument/didOpen" || method == "textDocument/didChange" {
				params, _ := msg["params"].(map[string]interface{})
				if method == "textDocument/didOpen" {
					td, _ := params["textDocument"].(map[string]interface{})
					if v, ok := td["version"].(float64); ok {
						versions = append(versions, int(v))
					}
					if txt, ok := td["text"].(string); ok {
						texts = append(texts, txt)
					}
				} else {
					td, _ := params["textDocument"].(map[string]interface{})
					if v, ok := td["version"].(float64); ok {
						versions = append(versions, int(v))
					}
					if ch, ok := params["contentChanges"].([]interface{}); ok && len(ch) > 0 {
						if m, ok := ch[0].(map[string]interface{}); ok {
							if txt, ok := m["text"].(string); ok {
								texts = append(texts, txt)
							}
						}
					}
				}
			}
			mu.Unlock()

			// Respond to requests (initialize / completion).
			if id, ok := msg["id"]; ok && method != "" {
				var resultObj interface{}
				switch method {
				case "initialize":
					_ = json.Unmarshal([]byte(`{"capabilities":{}}`), &resultObj)
				case "textDocument/completion":
					_ = json.Unmarshal([]byte(`{"items":[{"label":"Hello","kind":1,"insertText":"Hello"}]}`), &resultObj)
				default:
					resultObj = map[string]interface{}{}
				}
				resp, _ := json.Marshal(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      id,
					"result":  resultObj,
				})
				header := "Content-Length: " + strconv.Itoa(len(resp)) + "\r\n\r\n"
				_, _ = serverW.Write([]byte(header))
				_, _ = serverW.Write(resp)
			}
		}
	}()

	client := newJSONRPCClient(clientR, clientW)
	srv := &lspServer{
		client:      client,
		docVersions: make(map[string]int),
		docHashes:   make(map[string]string),
		docLastSync: make(map[string]time.Time),
		diags:       make(map[string][]Diagnostic),
	}
	svc := NewLSPService("/tmp/ws")
	svc.mu.Lock()
	svc.servers["go"] = srv
	svc.mu.Unlock()

	// Initialize handshake
	if err := svc.initializeLocked(srv, "go", "/tmp/ws"); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	req1 := LSPCompletionRequest{
		Language: "go",
		FilePath: "/tmp/ws/main.go",
		Line:     0,
		Column:   0,
		Content:  "package main\n",
	}
	if _, err := svc.syncDocument(req1); err != nil {
		t.Fatalf("sync1: %v", err)
	}
	req2 := req1
	req2.Content = "package main\nfunc Hello() {}\n"
	if _, err := svc.syncDocument(req2); err != nil {
		t.Fatalf("sync2: %v", err)
	}

	// Allow async reads
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(versions)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	gotMethods := append([]string{}, methods...)
	gotVersions := append([]int{}, versions...)
	gotTexts := append([]string{}, texts...)
	mu.Unlock()
	if len(gotVersions) < 2 {
		t.Fatalf("expected didOpen+didChange versions, got methods=%v versions=%v", gotMethods, gotVersions)
	}
	if gotVersions[0] != 1 || gotVersions[1] != 2 {
		t.Errorf("versions = %v, want [1,2,...]", gotVersions)
	}
	if len(gotTexts) < 2 || gotTexts[1] != req2.Content {
		t.Errorf("didChange text = %q, want updated content", gotTexts)
	}
	// Completion should still work after sync (must not hold mu).
	items, err := svc.GetCompletions(req2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != "Hello" {
		t.Errorf("completions = %+v", items)
	}
	_ = clientW.Close()
	_ = serverR.Close()
}

// TestLSP_parseCompletionItems_ParsesList verifies that parseCompletionItems
// handles a CompletionList-shaped response.
func TestLSP_parseCompletionItems_ParsesList(t *testing.T) {
	raw := json.RawMessage(`{
		"items": [
			{"label": "fmt", "kind": 9, "detail": "package", "insertText": "fmt"},
			{"label": "Println", "kind": 3, "detail": "func()", "insertText": "Println"}
		]
	}`)
	items := parseCompletionItems(raw)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Label != "fmt" || items[0].Kind != 9 {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[1].Label != "Println" || items[1].InsertText != "Println" {
		t.Errorf("unexpected second item: %+v", items[1])
	}
}

// TestLSP_parseCompletionItems_ParsesArray verifies that parseCompletionItems
// handles a plain JSON array response (some servers return this).
func TestLSP_parseCompletionItems_ParsesArray(t *testing.T) {
	raw := json.RawMessage(`[
		{"label": "foo", "kind": 2, "detail": "var", "insertText": "foo"}
	]`)
	items := parseCompletionItems(raw)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Label != "foo" {
		t.Errorf("unexpected item label: %q", items[0].Label)
	}
}

// TestLSP_parseCompletionItems_EmptyOnNil verifies that an empty/nil raw
// payload yields an empty (non-nil) slice.
func TestLSP_parseCompletionItems_EmptyOnNil(t *testing.T) {
	items := parseCompletionItems(nil)
	if items == nil || len(items) != 0 {
		t.Errorf("expected empty non-nil slice, got %v (len=%d)", items, len(items))
	}
	items = parseCompletionItems(json.RawMessage(``))
	if items == nil || len(items) != 0 {
		t.Errorf("expected empty non-nil slice for empty input, got %v", items)
	}
}

// TestLSP_parseCompletionItems_AdditionalTextEdits covers auto-import (prompt-10 10-I).
func TestLSP_parseCompletionItems_AdditionalTextEdits(t *testing.T) {
	raw := json.RawMessage(`{
		"items": [{
			"label": "join",
			"kind": 3,
			"detail": "func join",
			"insertText": "join",
			"additionalTextEdits": [{
				"range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 0}},
				"newText": "import { join } from 'path'\n"
			}]
		}]
	}`)
	items := parseCompletionItems(raw)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if len(items[0].AdditionalEdits) != 1 {
		t.Fatalf("expected 1 additional edit, got %d", len(items[0].AdditionalEdits))
	}
	if items[0].AdditionalEdits[0].NewText == "" {
		t.Error("expected non-empty newText for auto-import")
	}
	if items[0].AdditionalEdits[0].StartLine != 0 {
		t.Errorf("startLine=%d", items[0].AdditionalEdits[0].StartLine)
	}
}

// TestLSP_parseHover_ParsesMarkupContent verifies hover parsing.
func TestLSP_parseHover_ParsesMarkupContent(t *testing.T) {
	raw := json.RawMessage(`{"contents":{"kind":"markdown","value":"# fmt\n"}}`)
	hover := parseHover(raw)
	if !strings.Contains(hover, "fmt") {
		t.Errorf("expected hover to contain 'fmt', got %q", hover)
	}
}

// TestLSP_parseHover_EmptyOnNil verifies empty hover on nil input.
func TestLSP_parseHover_EmptyOnNil(t *testing.T) {
	if got := parseHover(nil); got != "" {
		t.Errorf("expected empty hover, got %q", got)
	}
}

// TestLSP_jsonRPCClient_writeMessage_FramesWithContentLength verifies that
// writeMessage produces a valid LSP base-protocol frame with a Content-Length
// header followed by the JSON body. No real LSP server is needed — we write
// into a bytes.Buffer and inspect the output.
func TestLSP_jsonRPCClient_writeMessage_FramesWithContentLength(t *testing.T) {
	var buf bytes.Buffer
	// Build a client whose reader is a closed reader so the readLoop exits
	// immediately; we only exercise writeMessage here.
	c := &jsonRPCClient{
		w:       &buf,
		r:       bufio.NewReader(strings.NewReader("")),
		pending: make(map[int64]chan *rpcResponse),
		notifs:  make(map[string][]func(json.RawMessage)),
		done:    make(chan struct{}),
	}
	if err := c.writeMessage(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "test/method",
		"params":  map[string]string{"hello": "world"},
	}); err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "Content-Length:") {
		t.Errorf("expected output to start with Content-Length header, got: %q", out)
	}
	if !strings.Contains(out, "\r\n\r\n") {
		t.Errorf("expected header/body separator, got: %q", out)
	}
	// The body after the separator must be valid JSON with the method.
	idx := strings.Index(out, "\r\n\r\n")
	body := out[idx+4:]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("body is not valid JSON: %v (body=%q)", err, body)
	}
	if parsed["method"] != "test/method" {
		t.Errorf("expected method 'test/method', got %v", parsed["method"])
	}
}

// TestLSP_jsonRPCClient_readMessage_ParsesFrame verifies the readMessage
// parser can decode a Content-Length-framed message from a reader.
func TestLSP_jsonRPCClient_readMessage_ParsesFrame(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"foo","params":{}}`
	frame := "Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	c := &jsonRPCClient{
		r:       bufio.NewReader(strings.NewReader(frame)),
		pending: make(map[int64]chan *rpcResponse),
		notifs:  make(map[string][]func(json.RawMessage)),
		done:    make(chan struct{}),
	}
	msg, err := c.readMessage()
	if err != nil {
		t.Fatalf("readMessage failed: %v", err)
	}
	if !strings.Contains(string(msg), `"method":"foo"`) {
		t.Errorf("unexpected message body: %s", string(msg))
	}
}
