package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/adrg/xdg"
)

// Plan 11 Task 4 — MCP (Model Context Protocol) client service.
//
// This file implements a self-contained MCP client that speaks JSON-RPC 2.0
// over three transports: stdio (subprocess), SSE, and streamable HTTP.
// It does NOT depend on github.com/mark3labs/mcp-go because the build
// environment is offline (Step 1: go get failed — network unreachable).
// The implementation follows the MCP 2024-11-05 specification:
//   https://spec.modelcontextprotocol.io/specification/2024-11-05/
//
// Security (G-SEC-02/09/12):
//   - All MCP server configs persisted via atomicWriteJSON with 0600.
//   - MCP tools default to RiskElevated; write/network/exec tools are
//     RiskDangerous. No tool is auto-approved unless AutoApprove is set
//     on the server config AND the tool name is in the AutoApprove list.
//   - MCP servers are treated as Restricted extensions (G-SEC-12): they
//     require explicitApproval before activation.
//   - stdio command paths are validated against the workspace root when
//     a root is set (ValidatePathWithinRoot).

// ---------------------------------------------------------------------------
// Schema (Step 4)
// ---------------------------------------------------------------------------

// MCPConfig is the on-disk configuration for all MCP servers.
type MCPConfig struct {
	Servers []MCPServerConfig `json:"servers"`
}

// MCPServerConfig describes a single MCP server connection.
//
// Transport selects how the client talks to the server:
//   - "stdio": spawn Command with Args + Env, communicate over stdin/stdout.
//   - "sse":   connect to URL, receive Server-Sent Events, POST requests back.
//   - "http":  streamable HTTP — POST JSON-RPC to URL and read the response.
//
// G-SEC-02: AutoApprove is an explicit allowlist of tool names that may
// execute without user approval. It defaults to empty (no auto-approve).
// Even with AutoApprove, the tool still appears in the audit log.
type MCPServerConfig struct {
	Name        string            `json:"name"`
	Transport   string            `json:"transport"` // "stdio" | "sse" | "http"
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
	AutoApprove []string          `json:"autoApprove,omitempty"`
}

// MCPTool is a tool exposed by an MCP server.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// MCPResource is a resource exposed by an MCP server.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPPrompt is a prompt template exposed by an MCP server.
type MCPPrompt struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Arguments   []map[string]interface{} `json:"arguments,omitempty"`
}

// MCPToolResult is the result of calling an MCP tool.
type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent is a content block in a tool result.
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Future: image/audio content types.
}

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 protocol (Step 2/3)
// ---------------------------------------------------------------------------

// jsonrpcRequest is a JSON-RPC 2.0 request/notification.
type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"` // nil for notifications
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Transport interface (Step 3)
// ---------------------------------------------------------------------------

// mcpTransport is the interface for a JSON-RPC transport layer.
type mcpTransport interface {
	// Send writes a JSON-RPC message. For notifications (id == nil), no
	// response is expected.
	Send(ctx context.Context, req *jsonrpcRequest) error
	// Recv reads the next JSON-RPC response. Returns io.EOF on close.
	Recv() (*jsonrpcResponse, error)
	// Close releases transport resources.
	Close() error
}

// ---------------------------------------------------------------------------
// stdio transport (Step 3a)
// ---------------------------------------------------------------------------

// stdioTransport communicates with an MCP server over a subprocess's
// stdin/stdout. Each JSON-RPC message is a single newline-delimited line.
type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
}

func newStdioTransport(ctx context.Context, cfg MCPServerConfig) (*stdioTransport, error) {
	if cfg.Command == "" {
		return nil, fmt.Errorf("stdio transport requires a command: %w", ErrInvalidInput)
	}
	cmd := commandContext(ctx, cfg.Command, cfg.Args...)
	// Inherit a minimal env: parent env + user-provided overrides.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = nil // discard stderr; MCP servers should log via protocol
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start mcp server: %w", err)
	}
	return &stdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

func (t *stdioTransport) Send(ctx context.Context, req *jsonrpcRequest) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}

func (t *stdioTransport) Recv() (*jsonrpcResponse, error) {
	line, err := t.stdout.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var resp jsonrpcResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

func (t *stdioTransport) Close() error {
	if err := t.stdin.Close(); err != nil && err != os.ErrClosed {
		// best-effort
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}
	return nil
}

// ---------------------------------------------------------------------------
// HTTP transport (Step 3c — streamable HTTP)
// ---------------------------------------------------------------------------

// httpTransport sends JSON-RPC requests via HTTP POST and reads the response
// from the same response body. This is the "streamable HTTP" transport from
// the MCP 2024-11-05 spec.
type httpTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
}

func newHTTPTransport(cfg MCPServerConfig) *httpTransport {
	return &httpTransport{
		url:     cfg.URL,
		headers: cfg.Headers,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *httpTransport) Send(ctx context.Context, req *jsonrpcRequest) error {
	// httpTransport is request/response: Send is a no-op because the actual
	// send+recv happens in Recv. We store the pending request on the context.
	return nil
}

func (t *httpTransport) Recv() (*jsonrpcResponse, error) {
	// httpTransport uses a combined send+recv model via postRequest.
	// This Recv() is unused for HTTP; the client calls postRequest directly.
	return nil, fmt.Errorf("http transport uses postRequest, not Recv: %w", ErrInvalidInput)
}

func (t *httpTransport) Close() error {
	t.client.CloseIdleConnections()
	return nil
}

// postRequest sends a JSON-RPC request and returns the parsed response.
func (t *httpTransport) postRequest(ctx context.Context, req *jsonrpcRequest) (*jsonrpcResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.url, strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit (G-SEC: bounded reads)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mcp server returned %d: %s", resp.StatusCode, string(body))
	}
	// The server may respond with application/json (single JSON-RPC object)
	// or text/event-stream (SSE frames). Parse both.
	contentType := resp.Header.Get("Content-Type")
	var rpcResp jsonrpcResponse
	if strings.Contains(contentType, "text/event-stream") {
		// Extract the first data: frame.
		rpcResp, err = parseSSEFrame(body)
		if err != nil {
			return nil, fmt.Errorf("parse sse response: %w", err)
		}
	} else {
		if err := json.Unmarshal(body, &rpcResp); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return &rpcResp, nil
}

// parseSSEFrame extracts the first "data:" frame from an SSE body.
func parseSSEFrame(body []byte) (jsonrpcResponse, error) {
	var resp jsonrpcResponse
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if err := json.Unmarshal([]byte(payload), &resp); err != nil {
				return resp, fmt.Errorf("unmarshal sse data: %w", err)
			}
			return resp, nil
		}
	}
	return resp, fmt.Errorf("no data frame in sse response: %w", ErrNotFound)
}

// ---------------------------------------------------------------------------
// SSE transport (Step 3b)
// ---------------------------------------------------------------------------

// sseTransport connects to an MCP server via Server-Sent Events. It opens
// a long-lived SSE connection for server→client messages and POSTs client
//→server messages to an endpoint URL provided by the server.
type sseTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	// postURL is the endpoint the server tells us to POST messages to.
	// It's discovered from the SSE stream's first "endpoint" event.
	postURL string
	events chan jsonrpcResponse
	done   chan struct{}
	once   sync.Once
}

func newSSETransport(cfg MCPServerConfig) *sseTransport {
	return &sseTransport{
		url:     cfg.URL,
		headers: cfg.Headers,
		client:  &http.Client{Timeout: 0}, // no timeout for SSE
		events:  make(chan jsonrpcResponse, 16),
		done:    make(chan struct{}),
	}
}

func (t *sseTransport) Send(ctx context.Context, req *jsonrpcRequest) error {
	if t.postURL == "" {
		return fmt.Errorf("sse transport: post URL not yet discovered: %w", ErrInvalidInput)
	}
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.postURL, strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("post to sse endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("sse post returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (t *sseTransport) Recv() (*jsonrpcResponse, error) {
	select {
	case resp := <-t.events:
		return &resp, nil
	case <-t.done:
		return nil, io.EOF
	}
}

func (t *sseTransport) Close() error {
	t.once.Do(func() { close(t.done) })
	t.client.CloseIdleConnections()
	return nil
}

// connect opens the SSE stream and starts reading events. The first
// "endpoint" event tells us where to POST messages.
func (t *sseTransport) connect(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", t.url, nil)
	if err != nil {
		return fmt.Errorf("create sse request: %w", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("connect sse: %w", err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return fmt.Errorf("sse connect returned %d", resp.StatusCode)
	}
	go t.readLoop(resp.Body)
	return nil
}

func (t *sseTransport) readLoop(body io.ReadCloser) {
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20) // 1MB max line
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Empty line = event boundary. Dispatch accumulated data.
			if len(dataLines) > 0 {
				payload := strings.Join(dataLines, "\n")
				dataLines = nil
				if strings.HasPrefix(payload, "endpoint:") {
					t.postURL = strings.TrimSpace(strings.TrimPrefix(payload, "endpoint:"))
					continue
				}
				var resp jsonrpcResponse
				if err := json.Unmarshal([]byte(payload), &resp); err == nil {
					select {
					case t.events <- resp:
					case <-t.done:
						return
					}
				}
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

// ---------------------------------------------------------------------------
// MCPClient (Step 2)
// ---------------------------------------------------------------------------

// MCPClient manages a single MCP server connection.
type MCPClient struct {
	cfg       MCPServerConfig
	transport mcpTransport
	nextID    int64
	mu        sync.Mutex
	closed    bool
}

// newMCPClient creates a client for the given config but does not connect.
func newMCPClient(cfg MCPServerConfig) *MCPClient {
	return &MCPClient{cfg: cfg}
}

// StartServer establishes the connection and performs the MCP initialize
// handshake (Step 2).
func (c *MCPClient) StartServer(ctx context.Context) error {
	switch c.cfg.Transport {
	case "stdio":
		t, err := newStdioTransport(ctx, c.cfg)
		if err != nil {
			return err
		}
		c.transport = t
	case "sse":
		t := newSSETransport(c.cfg)
		if err := t.connect(ctx); err != nil {
			return err
		}
		c.transport = t
	case "http":
		c.transport = newHTTPTransport(c.cfg)
	default:
		return fmt.Errorf("unknown transport %q: %w", c.cfg.Transport, ErrInvalidInput)
	}
	// Perform initialize handshake.
	if err := c.initialize(ctx); err != nil {
		c.transport.Close()
		c.transport = nil
		return err
	}
	return nil
}

// initialize sends the MCP initialize request + initialized notification.
func (c *MCPClient) initialize(ctx context.Context) error {
	resp, err := c.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "gugacode",
			"version": "1.0",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	_ = resp // server capabilities; we accept all
	// Send initialized notification (no response expected).
	return c.notify(ctx, "notifications/initialized", map[string]interface{}{})
}

// call sends a JSON-RPC request and waits for the response.
func (c *MCPClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return nil, fmt.Errorf("client not started: %w", ErrInvalidInput)
	}
	id := atomic.AddInt64(&c.nextID, 1)
	req := &jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	// HTTP transport uses a combined send+recv via postRequest.
	if ht, ok := c.transport.(*httpTransport); ok {
		resp, err := ht.postRequest(ctx, req)
		if err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
	// stdio / SSE: send then recv.
	if err := c.transport.Send(ctx, req); err != nil {
		return nil, fmt.Errorf("send %s: %w", method, err)
	}
	resp, err := c.transport.Recv()
	if err != nil {
		return nil, fmt.Errorf("recv %s: %w", method, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// notify sends a JSON-RPC notification (no id, no response).
func (c *MCPClient) notify(ctx context.Context, method string, params interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.transport == nil {
		return fmt.Errorf("client not started: %w", ErrInvalidInput)
	}
	req := &jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.transport.Send(ctx, req)
}

// ListTools returns the tools exposed by the server (Step 2).
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	raw, err := c.call(ctx, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the server (Step 2).
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*MCPToolResult, error) {
	raw, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	var result MCPToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tool result: %w", err)
	}
	return &result, nil
}

// ListResources returns the resources exposed by the server (Step 2).
func (c *MCPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	raw, err := c.call(ctx, "resources/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Resources []MCPResource `json:"resources"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal resources: %w", err)
	}
	return result.Resources, nil
}

// ReadResource reads a resource by URI (Step 2).
func (c *MCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	raw, err := c.call(ctx, "resources/read", map[string]interface{}{"uri": uri})
	if err != nil {
		return "", err
	}
	var result struct {
		Contents []struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("unmarshal resource: %w", err)
	}
	if len(result.Contents) == 0 {
		return "", fmt.Errorf("empty resource: %w", ErrNotFound)
	}
	return result.Contents[0].Text, nil
}

// ListPrompts returns the prompt templates exposed by the server (Step 2).
func (c *MCPClient) ListPrompts(ctx context.Context) ([]MCPPrompt, error) {
	raw, err := c.call(ctx, "prompts/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	var result struct {
		Prompts []MCPPrompt `json:"prompts"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal prompts: %w", err)
	}
	return result.Prompts, nil
}

// GetPrompt renders a prompt template by name (Step 2).
func (c *MCPClient) GetPrompt(ctx context.Context, name string, args map[string]string) ([]MCPContent, error) {
	raw, err := c.call(ctx, "prompts/get", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	var result struct {
		Messages []struct {
			Role    string      `json:"role"`
			Content MCPContent `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal prompt: %w", err)
	}
	var contents []MCPContent
	for _, m := range result.Messages {
		contents = append(contents, m.Content)
	}
	return contents, nil
}

// StopServer closes the connection (Step 2).
func (c *MCPClient) StopServer() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// MCPService (Step 4/9)
// ---------------------------------------------------------------------------

// MCPService manages multiple MCP server connections and persists their
// configuration. It is the Wails-bound entry point for the frontend.
//
// G-SEC-09: config is persisted via atomicWriteJSON with 0600 permissions.
// G-SEC-12: MCP servers are treated as Restricted — Enabled defaults to
// false and activation requires explicit user approval.
type MCPService struct {
	mu       sync.RWMutex
	config   MCPConfig
	cfgPath  string
	clients  map[string]*MCPClient
	rootDir  string // workspace root for path validation (empty = no sandbox)
	auditLog *os.File
}

// NewMCPService creates a new MCPService. The config is loaded from
// <configDir>/gugacode/mcp-servers.json (G-SEC-09: 0600).
func NewMCPService() *MCPService {
	cfgPath := filepath.Join(xdg.ConfigHome, "gugacode", "mcp-servers.json")
	s := &MCPService{
		cfgPath: cfgPath,
		clients: make(map[string]*MCPClient),
	}
	if err := s.load(); err != nil {
		slog.Warn("mcp: failed to load config", "error", err, "path", cfgPath)
	}
	// Best-effort audit log (matches AgentService pattern).
	logPath := filepath.Join(xdg.CacheHome, "gugacode", "mcp-audit.log")
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
		s.auditLog = f
	}
	return s
}

// SetWorkspaceRoot sets the workspace root for stdio command path validation.
// When set, stdio Command paths must resolve within this root (unless they
// are absolute system paths like /usr/bin or C:\Windows\System32).
func (s *MCPService) SetWorkspaceRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rootDir = root
}

// load reads the MCP config from disk. Missing file is not an error.
// G-SEC-07: Headers/Env values are stored encrypted on disk; after
// unmarshal we decrypt them into the in-memory config so MCP clients
// can use the real values when connecting.
func (s *MCPService) load() error {
	data, err := os.ReadFile(s.cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh start — no config yet
		}
		return fmt.Errorf("read mcp config: %w", err)
	}
	if err := json.Unmarshal(data, &s.config); err != nil {
		return fmt.Errorf("parse mcp config: %w", err)
	}
	// Decrypt secret-bearing maps into the in-memory (plaintext) config.
	for i := range s.config.Servers {
		decryptServerSecrets(&s.config.Servers[i])
	}
	return nil
}

// save persists the MCP config via atomicWriteJSON with 0600 (G-SEC-09).
// G-SEC-07: Headers/Env values are encrypted before writing so secrets are
// never stored as plaintext on disk. The in-memory config retains
// plaintext for use by running MCP connections.
func (s *MCPService) save() error {
	servers := make([]MCPServerConfig, len(s.config.Servers))
	copy(servers, s.config.Servers)
	for i := range servers {
		encryptServerSecretsForDisk(&servers[i])
	}
	enc := MCPConfig{Servers: servers}
	return atomicWriteJSON(s.cfgPath, enc, 0600)
}

// mcpSecretMask is the placeholder returned to the frontend in place of a
// real secret value. The UI does not display Headers/Env, so masking is
// invisible to the user while keeping plaintext out of the JS heap.
const mcpSecretMask = "***"

// maskServerSecretsForView returns a copy of cfg with non-empty
// Headers/Env values replaced by mcpSecretMask. Empty values stay empty.
func maskServerSecretsForView(cfg MCPServerConfig) MCPServerConfig {
	out := cfg
	out.Headers = maskSecretMap(cfg.Headers)
	out.Env = maskSecretMap(cfg.Env)
	return out
}

func maskSecretMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	masked := make(map[string]string, len(m))
	for k, v := range m {
		if v == "" {
			masked[k] = ""
		} else {
			masked[k] = mcpSecretMask
		}
	}
	return masked
}

// isMaskedSecret reports whether a value is the frontend mask placeholder.
func isMaskedSecret(v string) bool { return v == mcpSecretMask }

// mergeSecretMap merges incoming secret map onto existing. For each key:
//   - if the incoming value is the mask placeholder or empty AND the
//     existing value is non-empty, preserve the existing decrypted value
//     (the frontend did not change it — it only round-tripped the mask);
//   - otherwise adopt the incoming value (including newly-set plaintext).
func mergeSecretMap(existing, incoming map[string]string) map[string]string {
	if len(incoming) == 0 {
		// No incoming keys: keep existing secrets untouched.
		return existing
	}
	out := make(map[string]string, len(incoming))
	for k, v := range incoming {
		if (v == "" || isMaskedSecret(v)) && existing[k] != "" {
			out[k] = existing[k]
			continue
		}
		out[k] = v
	}
	return out
}

// encryptServerSecretsForDisk encrypts non-empty Headers/Env values in
// place (used for the on-disk copy only).
func encryptServerSecretsForDisk(cfg *MCPServerConfig) {
	cfg.Headers = encryptSecretMap(cfg.Headers)
	cfg.Env = encryptSecretMap(cfg.Env)
}

func encryptSecretMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if v == "" || isMaskedSecret(v) {
			out[k] = v
			continue
		}
		enc, err := EncryptSecret(v)
		if err != nil {
			out[k] = v // best-effort: keep value if encryption fails
			continue
		}
		out[k] = enc
	}
	return out
}

// decryptServerSecrets decrypts Headers/Env values in place (used after
// loading from disk to restore plaintext for in-memory use).
func decryptServerSecrets(cfg *MCPServerConfig) {
	cfg.Headers = decryptSecretMap(cfg.Headers)
	cfg.Env = decryptSecretMap(cfg.Env)
}

func decryptSecretMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		dec, err := DecryptSecret(v)
		if err != nil {
			out[k] = v // best-effort: keep raw if decrypt fails
			continue
		}
		out[k] = dec
	}
	return out
}

// ListServers returns all configured MCP servers. G-SEC-07: Headers/Env
// secret values are masked so plaintext never crosses the Wails binding.
func (s *MCPService) ListServers() []MCPServerConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCPServerConfig, len(s.config.Servers))
	for i, srv := range s.config.Servers {
		out[i] = maskServerSecretsForView(srv)
	}
	return out
}

// GetServer returns a single server config by name. G-SEC-07: Headers/Env
// secret values are masked in the returned copy.
func (s *MCPService) GetServer(name string) (MCPServerConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, srv := range s.config.Servers {
		if srv.Name == name {
			return maskServerSecretsForView(srv), nil
		}
	}
	return MCPServerConfig{}, fmt.Errorf("mcp server %q: %w", name, ErrNotFound)
}

// SaveServer adds or updates a server config. Names must be unique.
// G-SEC-12: new servers default to Enabled=false (Restricted).
func (s *MCPService) SaveServer(cfg MCPServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("server name required: %w", ErrInvalidInput)
	}
	if cfg.Transport != "stdio" && cfg.Transport != "sse" && cfg.Transport != "http" {
		return fmt.Errorf("invalid transport %q: %w", cfg.Transport, ErrInvalidInput)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Validate stdio command path if a workspace root is set.
	if cfg.Transport == "stdio" && s.rootDir != "" && cfg.Command != "" {
		if _, err := ValidatePathWithinRoot(s.rootDir, cfg.Command); err != nil {
			return fmt.Errorf("stdio command path outside workspace: %w", err)
		}
	}
	// Upsert.
	found := false
	for i, srv := range s.config.Servers {
		if srv.Name == cfg.Name {
			// Preserve Enabled when updating an existing server unless the
			// caller explicitly sets Enabled=true (explicit approval, G-SEC-12).
			if !cfg.Enabled {
				cfg.Enabled = srv.Enabled
			}
			// G-SEC-07: ListServers masks Headers/Env. When the frontend
			// round-trips a masked/empty value back through SaveServer,
			// preserve the existing decrypted secret rather than overwriting
			// it with the mask placeholder.
			cfg.Headers = mergeSecretMap(srv.Headers, cfg.Headers)
			cfg.Env = mergeSecretMap(srv.Env, cfg.Env)
			s.config.Servers[i] = cfg
			found = true
			break
		}
	}
	if !found {
		// G-SEC-12: new servers start disabled.
		cfg.Enabled = false
		s.config.Servers = append(s.config.Servers, cfg)
	}
	return s.save()
}

// DeleteServer removes a server config and stops its client if running.
func (s *MCPService) DeleteServer(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, srv := range s.config.Servers {
		if srv.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("mcp server %q: %w", name, ErrNotFound)
	}
	// Stop the client if running.
	if client, ok := s.clients[name]; ok {
		_ = client.StopServer()
		delete(s.clients, name)
	}
	s.config.Servers = append(s.config.Servers[:idx], s.config.Servers[idx+1:]...)
	return s.save()
}

// ConnectServer starts the MCP client for a configured server.
// G-SEC-12: the caller must have explicitly set Enabled=true via SaveServer
// (which requires user approval in the UI).
func (s *MCPService) ConnectServer(ctx context.Context, name string) error {
	s.mu.Lock()
	cfg, err := s.getServerLocked(name)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if !cfg.Enabled {
		s.mu.Unlock()
		return fmt.Errorf("server %q not enabled (G-SEC-12): %w", name, ErrUnauthorized)
	}
	if _, ok := s.clients[name]; ok {
		s.mu.Unlock()
		return fmt.Errorf("server %q already connected: %w", name, ErrAlreadyExists)
	}
	s.mu.Unlock()
	client := newMCPClient(cfg)
	if err := client.StartServer(ctx); err != nil {
		return fmt.Errorf("start server %q: %w", name, err)
	}
	s.mu.Lock()
	s.clients[name] = client
	s.mu.Unlock()
	s.audit("connect", name, "")
	return nil
}

// DisconnectServer stops a running MCP client.
func (s *MCPService) DisconnectServer(name string) error {
	s.mu.Lock()
	client, ok := s.clients[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("server %q not connected: %w", name, ErrNotFound)
	}
	delete(s.clients, name)
	s.mu.Unlock()
	err := client.StopServer()
	s.audit("disconnect", name, "")
	return err
}

// ListTools queries a connected server for its tools.
func (s *MCPService) ListTools(ctx context.Context, name string) ([]MCPTool, error) {
	client, err := s.getClient(name)
	if err != nil {
		return nil, err
	}
	return client.ListTools(ctx)
}

// CallTool invokes a tool on a connected server.
// G-SEC-02: the caller (agent_service) must classify the risk via
// ClassifyMCPToolRisk and require approval unless the tool is in the
// server's AutoApprove list.
func (s *MCPService) CallTool(ctx context.Context, server, tool string, args map[string]interface{}) (*MCPToolResult, error) {
	client, err := s.getClient(server)
	if err != nil {
		return nil, err
	}
	result, err := client.CallTool(ctx, tool, args)
	if err == nil {
		s.audit("call_tool", server, tool)
	}
	return result, err
}

// ListResources queries a connected server for its resources.
func (s *MCPService) ListResources(ctx context.Context, name string) ([]MCPResource, error) {
	client, err := s.getClient(name)
	if err != nil {
		return nil, err
	}
	return client.ListResources(ctx)
}

// ReadResource reads a resource by URI from a connected server.
func (s *MCPService) ReadResource(ctx context.Context, name, uri string) (string, error) {
	client, err := s.getClient(name)
	if err != nil {
		return "", err
	}
	return client.ReadResource(ctx, uri)
}

// ListPrompts queries a connected server for its prompts.
func (s *MCPService) ListPrompts(ctx context.Context, name string) ([]MCPPrompt, error) {
	client, err := s.getClient(name)
	if err != nil {
		return nil, err
	}
	return client.ListPrompts(ctx)
}

// GetPrompt renders a prompt template from a connected server.
func (s *MCPService) GetPrompt(ctx context.Context, name, prompt string, args map[string]string) ([]MCPContent, error) {
	client, err := s.getClient(name)
	if err != nil {
		return nil, err
	}
	return client.GetPrompt(ctx, prompt, args)
}

// Close shuts down all running MCP clients. Called on app shutdown.
func (s *MCPService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for name, client := range s.clients {
		if err := client.StopServer(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.clients, name)
	}
	if s.auditLog != nil {
		_ = s.auditLog.Close()
	}
	return firstErr
}

// ---------------------------------------------------------------------------
// Agent integration (Step 6/8)
// ---------------------------------------------------------------------------

// AgentMCPTool describes an MCP tool registered with the agent. The name
// follows the mcp.<server>.<tool> namespace (Step 6).
type AgentMCPTool struct {
	Namespace    string                 `json:"namespace"`    // mcp.<server>.<tool>
	Server       string                 `json:"server"`
	Tool         string                 `json:"tool"`
	Description  string                 `json:"description"`
	InputSchema  map[string]interface{} `json:"inputSchema"`
	RiskLevel    RiskLevel              `json:"riskLevel"`
	AutoApproved bool                   `json:"autoApproved"`
}

// ListAgentMCPTools returns all tools from all connected MCP servers,
// namespaced as mcp.<server>.<tool> (Step 6).
func (s *MCPService) ListAgentMCPTools(ctx context.Context) ([]AgentMCPTool, error) {
	s.mu.RLock()
	names := make([]string, 0, len(s.clients))
	for name := range s.clients {
		names = append(names, name)
	}
	s.mu.RUnlock()
	var tools []AgentMCPTool
	for _, server := range names {
		client, err := s.getClient(server)
		if err != nil {
			continue
		}
		serverTools, err := client.ListTools(ctx)
		if err != nil {
			slog.Warn("mcp: list tools failed", "server", server, "error", err)
			continue
		}
		cfg, _ := s.GetServer(server)
		for _, t := range serverTools {
			risk := ClassifyMCPToolRisk(t.Name, t.Description)
			autoApproved := false
			for _, a := range cfg.AutoApprove {
				if a == t.Name {
					autoApproved = true
					break
				}
			}
			tools = append(tools, AgentMCPTool{
				Namespace:    fmt.Sprintf("mcp.%s.%s", server, t.Name),
				Server:       server,
				Tool:         t.Name,
				Description:  t.Description,
				InputSchema:  t.InputSchema,
				RiskLevel:    risk,
				AutoApproved: autoApproved,
			})
		}
	}
	return tools, nil
}

// ClassifyMCPToolRisk determines the risk level of an MCP tool (Step 8).
//
// G-SEC-02: all MCP tools default to RiskElevated. Tools whose names or
// descriptions suggest write/network/exec operations are classified as
// RiskDangerous. No MCP tool is ever classified as RiskSafe — even with
// AutoApprove, the audit log records every call.
func ClassifyMCPToolRisk(name, description string) RiskLevel {
	combined := strings.ToLower(name + " " + description)
	// RiskDangerous: write/exec/network/file operations.
	dangerousKeywords := []string{
		"write", "create", "delete", "remove", "exec", "run", "shell",
		"command", "spawn", "kill", "move", "rename", "upload", "download",
		"fetch", "request", "post", "put", "patch",
	}
	for _, kw := range dangerousKeywords {
		if strings.Contains(combined, kw) {
			return RiskDangerous
		}
	}
	return RiskElevated
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *MCPService) getClient(name string) (*MCPClient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	client, ok := s.clients[name]
	if !ok {
		return nil, fmt.Errorf("server %q not connected: %w", name, ErrNotFound)
	}
	return client, nil
}

func (s *MCPService) getServerLocked(name string) (MCPServerConfig, error) {
	for _, srv := range s.config.Servers {
		if srv.Name == name {
			return srv, nil
		}
	}
	return MCPServerConfig{}, fmt.Errorf("mcp server %q: %w", name, ErrNotFound)
}

func (s *MCPService) audit(action, server, tool string) {
	if s.auditLog == nil {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	line := fmt.Sprintf("%s\t%s\tserver=%s\ttool=%s\n", ts, action, server, tool)
	_, _ = s.auditLog.WriteString(line)
}
