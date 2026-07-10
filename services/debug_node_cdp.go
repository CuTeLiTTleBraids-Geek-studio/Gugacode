package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// nodeCDPClient speaks Chrome DevTools Protocol over WebSocket (prompt-13 13-A).
// Powers Node/TS debugging inside the same Debug panel as Delve DAP.
type nodeCDPClient struct {
	mu       sync.Mutex
	conn     *websocket.Conn
	seq      int64
	pending  map[int64]chan cdpResponse
	closed   bool
	onPaused func(reason string, frames []DebugStackFrame, locals []DebugVariable)
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// connectNodeCDP waits for inspector HTTP, then opens the CDP websocket.
func connectNodeCDP(hostPort string, timeout time.Duration) (*nodeCDPClient, error) {
	deadline := time.Now().Add(timeout)
	var wsURL string
	for time.Now().Before(deadline) {
		u := "http://" + hostPort + "/json/list"
		resp, err := http.Get(u) //nolint:gosec // local inspector only
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			var list []struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
				Type                 string `json:"type"`
			}
			if json.Unmarshal(body, &list) == nil {
				for _, e := range list {
					if e.WebSocketDebuggerURL != "" {
						wsURL = e.WebSocketDebuggerURL
						break
					}
				}
			}
			if wsURL != "" {
				break
			}
		}
		// also try /json
		resp2, err2 := http.Get("http://" + hostPort + "/json") //nolint:gosec
		if err2 == nil {
			body, _ := io.ReadAll(resp2.Body)
			_ = resp2.Body.Close()
			var list []struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			if json.Unmarshal(body, &list) == nil && len(list) > 0 {
				wsURL = list[0].WebSocketDebuggerURL
			}
		}
		if wsURL != "" {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if wsURL == "" {
		return nil, fmt.Errorf("node inspector websocket not ready on %s", hostPort)
	}
	// normalize ws url host if needed
	if strings.HasPrefix(wsURL, "ws://") {
		// ok
	} else if u, err := url.Parse(wsURL); err == nil && u.Scheme == "" {
		wsURL = "ws://" + hostPort + wsURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cdp dial: %w", err)
	}
	c := &nodeCDPClient{
		conn:    conn,
		pending: make(map[int64]chan cdpResponse),
	}
	go c.readLoop()
	if err := c.call("Debugger.enable", map[string]interface{}{}); err != nil {
		_ = c.Close()
		return nil, err
	}
	_ = c.call("Runtime.enable", map[string]interface{}{})
	return c, nil
}

func (c *nodeCDPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[int64]chan cdpResponse)
	if c.conn != nil {
		return c.conn.Close(websocket.StatusNormalClosure, "")
	}
	return nil
}

func (c *nodeCDPClient) call(method string, params map[string]interface{}) error {
	_, err := c.callResult(method, params)
	return err
}

func (c *nodeCDPClient) callResult(method string, params map[string]interface{}) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.seq, 1)
	ch := make(chan cdpResponse, 1)
	c.mu.Lock()
	if c.closed || c.conn == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp closed")
	}
	c.pending[id] = ch
	conn := c.conn
	c.mu.Unlock()

	msg := map[string]interface{}{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	data, _ := json.Marshal(msg)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("cdp closed")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("%s", resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(10 * time.Second):
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp timeout: %s", method)
	}
}

func (c *nodeCDPClient) readLoop() {
	for {
		c.mu.Lock()
		conn := c.conn
		closed := c.closed
		c.mu.Unlock()
		if closed || conn == nil {
			return
		}
		ctx := context.Background()
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var resp cdpResponse
		if json.Unmarshal(data, &resp) != nil {
			continue
		}
		if resp.Method != "" {
			c.handleEvent(resp)
			continue
		}
		c.mu.Lock()
		ch := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (c *nodeCDPClient) handleEvent(ev cdpResponse) {
	if ev.Method != "Debugger.paused" {
		if ev.Method == "Debugger.resumed" {
			// ignore
		}
		return
	}
	var params struct {
		Reason string `json:"reason"`
		CallFrames []struct {
			CallFrameID string `json:"callFrameId"`
			FunctionName string `json:"functionName"`
			URL         string `json:"url"`
			Location    struct {
				ScriptID     string `json:"scriptId"`
				LineNumber   int    `json:"lineNumber"` // 0-based
				ColumnNumber int    `json:"columnNumber"`
			} `json:"location"`
			ScopeChain []struct {
				Type string `json:"type"`
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			} `json:"scopeChain"`
		} `json:"callFrames"`
	}
	if json.Unmarshal(ev.Params, &params) != nil {
		return
	}
	frames := make([]DebugStackFrame, 0, len(params.CallFrames))
	var locals []DebugVariable
	for i, f := range params.CallFrames {
		path := f.URL
		if strings.HasPrefix(path, "file:///") {
			path = strings.TrimPrefix(path, "file:///")
			if len(path) >= 2 && path[1] == ':' {
				// windows file:///C:/...
			} else if strings.HasPrefix(path, "/") {
				// unix
			}
		} else if strings.HasPrefix(path, "file://") {
			path = strings.TrimPrefix(path, "file://")
		}
		name := f.FunctionName
		if name == "" {
			name = "(anonymous)"
		}
		frames = append(frames, DebugStackFrame{
			ID: i + 1, Name: name, File: path,
			Line: f.Location.LineNumber + 1, Column: f.Location.ColumnNumber + 1,
		})
		if i == 0 {
			// fetch local scope properties
			for _, sc := range f.ScopeChain {
				if sc.Type == "local" || sc.Type == "closure" {
					if sc.Object.ObjectID != "" {
						locals = c.getProperties(sc.Object.ObjectID)
					}
					if sc.Type == "local" {
						break
					}
				}
			}
		}
	}
	c.mu.Lock()
	cb := c.onPaused
	c.mu.Unlock()
	if cb != nil {
		cb(params.Reason, frames, locals)
	}
}

func (c *nodeCDPClient) getProperties(objectID string) []DebugVariable {
	raw, err := c.callResult("Runtime.getProperties", map[string]interface{}{
		"objectId":               objectID,
		"ownProperties":          true,
		"accessorPropertiesOnly": false,
	})
	if err != nil {
		return nil
	}
	var res struct {
		Result []struct {
			Name  string `json:"name"`
			Value *struct {
				Type        string `json:"type"`
				Value       interface{} `json:"value"`
				Description string `json:"description"`
			} `json:"value"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return nil
	}
	var out []DebugVariable
	for _, p := range res.Result {
		if strings.HasPrefix(p.Name, "__") {
			continue
		}
		val := ""
		typ := ""
		if p.Value != nil {
			typ = p.Value.Type
			if p.Value.Description != "" {
				val = p.Value.Description
			} else {
				b, _ := json.Marshal(p.Value.Value)
				val = string(b)
			}
		}
		out = append(out, DebugVariable{Name: p.Name, Value: val, Type: typ})
		if len(out) >= 40 {
			break
		}
	}
	return out
}

func (c *nodeCDPClient) Resume() error {
	return c.call("Debugger.resume", map[string]interface{}{})
}

func (c *nodeCDPClient) StepOver() error {
	return c.call("Debugger.stepOver", map[string]interface{}{})
}

func (c *nodeCDPClient) StepInto() error {
	return c.call("Debugger.stepInto", map[string]interface{}{})
}

func (c *nodeCDPClient) StepOut() error {
	return c.call("Debugger.stepOut", map[string]interface{}{})
}

func (c *nodeCDPClient) Pause() error {
	return c.call("Debugger.pause", map[string]interface{}{})
}

// SetBreakpointByURL sets a breakpoint; line is 1-based.
func (c *nodeCDPClient) SetBreakpointByURL(file string, line int, condition string) (id string, verified bool, message string, err error) {
	// CDP uses 0-based lines
	urlPattern := file
	// also try file:// form
	params := map[string]interface{}{
		"lineNumber": line - 1,
		"url":        fileToFileURL(file),
	}
	if condition != "" {
		params["condition"] = condition
	}
	raw, err := c.callResult("Debugger.setBreakpointByUrl", params)
	if err != nil {
		// fallback urlRegex
		params2 := map[string]interface{}{
			"lineNumber": line - 1,
			"urlRegex":   ".*" + regexpQuoteMeta(filepathBase(file)) + ".*",
		}
		if condition != "" {
			params2["condition"] = condition
		}
		raw, err = c.callResult("Debugger.setBreakpointByUrl", params2)
		if err != nil {
			return "", false, err.Error(), err
		}
	}
	var res struct {
		BreakpointID string `json:"breakpointId"`
		Locations    []struct {
			LineNumber int `json:"lineNumber"`
		} `json:"locations"`
	}
	_ = json.Unmarshal(raw, &res)
	verified = len(res.Locations) > 0
	if !verified {
		message = "unverified (no matching script location yet)"
	}
	_ = urlPattern
	return res.BreakpointID, verified, message, nil
}

func (c *nodeCDPClient) Evaluate(expr string) (DebugVariable, error) {
	raw, err := c.callResult("Runtime.evaluate", map[string]interface{}{
		"expression":    expr,
		"returnByValue": true,
		"awaitPromise":  false,
	})
	if err != nil {
		return DebugVariable{Name: expr, Value: err.Error(), Type: "error"}, err
	}
	var res struct {
		Result struct {
			Type        string      `json:"type"`
			Value       interface{} `json:"value"`
			Description string      `json:"description"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text string `json:"text"`
		} `json:"exceptionDetails"`
	}
	if json.Unmarshal(raw, &res) != nil {
		return DebugVariable{Name: expr, Value: string(raw), Type: "raw"}, nil
	}
	if res.ExceptionDetails != nil {
		msg := res.ExceptionDetails.Text
		return DebugVariable{Name: expr, Value: msg, Type: "error"}, fmt.Errorf("%s", msg)
	}
	val := res.Result.Description
	if val == "" {
		b, _ := json.Marshal(res.Result.Value)
		val = string(b)
	}
	return DebugVariable{Name: expr, Value: val, Type: res.Result.Type}, nil
}

func fileToFileURL(path string) string {
	p := strings.ReplaceAll(path, "\\", "/")
	if len(p) >= 2 && p[1] == ':' {
		return "file:///" + p
	}
	if strings.HasPrefix(p, "/") {
		return "file://" + p
	}
	return "file:///" + p
}

func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return p
	}
	return p[i+1:]
}

func regexpQuoteMeta(s string) string {
	// minimal escape for file name in regex
	repl := []string{`.`, `\`, `+`, `*`, `?`, `(`, `)`, `[`, `]`, `{`, `}`, `^`, `$`, `|`}
	out := s
	for _, c := range repl {
		out = strings.ReplaceAll(out, c, `\`+c)
	}
	return out
}

// AttachDelve attaches to an existing headless/dlv dap listen address (prompt-13 13-E).
func (d *DebugService) AttachDelve(addr string) (DebugSessionInfo, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return DebugSessionInfo{}, fmt.Errorf("address required (host:port)")
	}
	if !strings.Contains(addr, ":") {
		addr = "127.0.0.1:" + addr
	}
	return d.ConnectMockDAP(addr, map[string]interface{}{
		"request": "attach",
		// delve dap often uses launch; for attach-style headless JSON-RPC this is best-effort
		"mode": "debug",
	})
}

// ProbeDelveTCP checks if a TCP port accepts connections (remote/container probe).
func (d *DebugService) ProbeDelveTCP(addr string) map[string]interface{} {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return map[string]interface{}{"ok": false, "message": "empty address"}
	}
	if !strings.Contains(addr, ":") {
		addr = "127.0.0.1:" + addr
	}
	conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
	if err != nil {
		return map[string]interface{}{"ok": false, "message": err.Error(), "address": addr}
	}
	_ = conn.Close()
	return map[string]interface{}{"ok": true, "message": "port open — use Attach Delve", "address": addr}
}
