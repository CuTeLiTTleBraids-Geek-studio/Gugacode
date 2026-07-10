package services

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Plan 11 Task 4 Step 11 — MCP service tests.
//
// These tests cover:
//   - ClassifyMCPToolRisk risk classification (Step 8 / G-SEC-02).
//   - MCPConfig persistence: SaveServer/DeleteServer/ListServers/GetServer (Step 9).
//   - G-SEC-12: new servers default to Enabled=false.
//   - Invalid transport rejection.
//   - atomicWriteJSON 0600 permissions (Step 9).
//   - Agent CheckCommand for mcp.* namespace (Step 6/8).
//   - CallMCPTool namespace parsing (Step 6).
//
// Integration tests (actually connecting to an MCP server over stdio/SSE/HTTP)
// are not included here because they require a live MCP server process, which
// is not available in the test environment. The transport implementations are
// covered by the compile-time interface check + manual testing.

// ---------------------------------------------------------------------------
// ClassifyMCPToolRisk (Step 8 / G-SEC-02)
// ---------------------------------------------------------------------------

func TestClassifyMCPToolRisk_DefaultElevated(t *testing.T) {
	// A tool with a benign name/description defaults to RiskElevated.
	got := ClassifyMCPToolRisk("search", "Search the documentation")
	if got != RiskElevated {
		t.Errorf("expected RiskElevated for benign tool, got %s", got)
	}
}

func TestClassifyMCPToolRisk_DangerousWrite(t *testing.T) {
	cases := []struct {
		name string
		desc string
	}{
		{"write_file", "Write content to a file"},
		{"delete_file", "Delete a file from disk"},
		{"run_command", "Execute a shell command"},
		{"fetch_url", "Fetch a URL from the network"},
		{"create_dir", "Create a directory"},
		{"exec_script", "Run a script"},
		{"upload_file", "Upload a file to a server"},
		{"download_file", "Download a file"},
	}
	for _, c := range cases {
		got := ClassifyMCPToolRisk(c.name, c.desc)
		if got != RiskDangerous {
			t.Errorf("expected RiskDangerous for %q (%s), got %s", c.name, c.desc, got)
		}
	}
}

// ---------------------------------------------------------------------------
// MCPConfig persistence (Step 9)
// ---------------------------------------------------------------------------

// newTestMCPService creates an MCPService with its config in a temp dir.
func newTestMCPService(t *testing.T) *MCPService {
	t.Helper()
	dir := t.TempDir()
	s := &MCPService{
		cfgPath: filepath.Join(dir, "mcp-servers.json"),
		clients: make(map[string]*MCPClient),
	}
	return s
}

func TestMCPService_SaveServer_NewDefaultsDisabled(t *testing.T) {
	s := newTestMCPService(t)
	// G-SEC-12: new servers must default to Enabled=false even if the caller
	// sets Enabled=true — activation requires explicit re-save after review.
	err := s.SaveServer(MCPServerConfig{
		Name:      "test-server",
		Transport: "stdio",
		Command:   "echo",
		Enabled:   true, // should be ignored for new servers
	})
	if err != nil {
		t.Fatalf("SaveServer: %v", err)
	}
	got, err := s.GetServer("test-server")
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if got.Enabled {
		t.Error("G-SEC-12: new server should default to Enabled=false")
	}
}

func TestMCPService_SaveServer_InvalidTransport(t *testing.T) {
	s := newTestMCPService(t)
	err := s.SaveServer(MCPServerConfig{
		Name:      "bad",
		Transport: "ftp", // unsupported
	})
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
}

func TestMCPService_SaveServer_EmptyName(t *testing.T) {
	s := newTestMCPService(t)
	err := s.SaveServer(MCPServerConfig{
		Name:      "",
		Transport: "stdio",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestMCPService_SaveServer_UpdatePreservesEnabled(t *testing.T) {
	s := newTestMCPService(t)
	// Create a new server (defaults to disabled).
	if err := s.SaveServer(MCPServerConfig{
		Name: "srv", Transport: "stdio", Command: "echo",
	}); err != nil {
		t.Fatal(err)
	}
	// Manually enable it (simulating user approval).
	s.mu.Lock()
	s.config.Servers[0].Enabled = true
	s.mu.Unlock()
	// Update the server without setting Enabled — should preserve the
	// existing Enabled=true.
	if err := s.SaveServer(MCPServerConfig{
		Name: "srv", Transport: "stdio", Command: "echo", Args: []string{"hi"},
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetServer("srv")
	if !got.Enabled {
		t.Error("update should preserve Enabled=true")
	}
	if len(got.Args) != 1 || got.Args[0] != "hi" {
		t.Errorf("update should set Args, got %v", got.Args)
	}
}

func TestMCPService_DeleteServer(t *testing.T) {
	s := newTestMCPService(t)
	s.SaveServer(MCPServerConfig{Name: "a", Transport: "stdio", Command: "echo"})
	s.SaveServer(MCPServerConfig{Name: "b", Transport: "http", URL: "http://localhost"})
	if len(s.ListServers()) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(s.ListServers()))
	}
	if err := s.DeleteServer("a"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	servers := s.ListServers()
	if len(servers) != 1 || servers[0].Name != "b" {
		t.Errorf("expected only b remaining, got %v", servers)
	}
}

func TestMCPService_DeleteServer_NotFound(t *testing.T) {
	s := newTestMCPService(t)
	err := s.DeleteServer("nope")
	if err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestMCPService_GetServer_NotFound(t *testing.T) {
	s := newTestMCPService(t)
	_, err := s.GetServer("nope")
	if err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestMCPService_ListServers_ReturnsCopy(t *testing.T) {
	s := newTestMCPService(t)
	s.SaveServer(MCPServerConfig{Name: "a", Transport: "stdio", Command: "echo"})
	list := s.ListServers()
	list[0].Name = "mutated"
	// Original should be unchanged.
	got, _ := s.GetServer("a")
	if got.Name != "a" {
		t.Error("ListServers should return a copy, not a reference")
	}
}

// ---------------------------------------------------------------------------
// Persistence: 0600 permissions (Step 9 / G-SEC-09)
// ---------------------------------------------------------------------------

func TestMCPService_PersistConfig_0600Permissions(t *testing.T) {
	// Windows does not honor Unix permission bits: os.Chmod only toggles
	// the read-only attribute, and os.Stat reports 0666 for writable
	// files. The 0600 contract (atomicWriteJSON perm) is enforced by the
	// shared helper and is therefore unverifiable on Windows. See
	// atomic_write_test.go for the same platform skip.
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not enforced on Windows")
	}
	s := newTestMCPService(t)
	if err := s.SaveServer(MCPServerConfig{
		Name: "srv", Transport: "stdio", Command: "echo",
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.cfgPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	// G-SEC-09: config file must be 0600 (owner read/write only).
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %o", info.Mode().Perm())
	}
}

func TestMCPService_LoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp-servers.json")
	// Save with one service, load with another.
	s1 := &MCPService{cfgPath: cfgPath, clients: make(map[string]*MCPClient)}
	s1.SaveServer(MCPServerConfig{
		Name: "persisted", Transport: "http", URL: "http://localhost:8080",
		Headers: map[string]string{"Authorization": "Bearer secret"},
	})
	s1.SaveServer(MCPServerConfig{
		Name: "stdio-srv", Transport: "stdio", Command: "/usr/bin/python3",
		Args: []string{"-m", "mcp_server"},
	})
	// New service loading from the same path.
	s2 := &MCPService{cfgPath: cfgPath, clients: make(map[string]*MCPClient)}
	if err := s2.load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	servers := s2.ListServers()
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	// Verify the persisted config survived the round-trip.
	httpSrv, _ := s2.GetServer("persisted")
	if httpSrv.URL != "http://localhost:8080" {
		t.Errorf("URL mismatch: %s", httpSrv.URL)
	}
	// G-SEC-07: the public GetServer masks secret-bearing Header values.
	if httpSrv.Headers["Authorization"] != mcpSecretMask {
		t.Errorf("expected masked header, got %q", httpSrv.Headers["Authorization"])
	}
	// The internal (in-memory) config retains the decrypted plaintext for
	// use by running MCP connections.
	httpInternal, _ := s2.getServerLocked("persisted")
	if httpInternal.Headers["Authorization"] != "Bearer secret" {
		t.Errorf("internal header not preserved: %q", httpInternal.Headers["Authorization"])
	}
	// Re-saving a masked round-trip must preserve the existing secret.
	if err := s2.SaveServer(httpSrv); err != nil {
		t.Fatalf("re-save masked: %v", err)
	}
	httpInternal2, _ := s2.getServerLocked("persisted")
	if httpInternal2.Headers["Authorization"] != "Bearer secret" {
		t.Errorf("secret lost after masked round-trip: %q", httpInternal2.Headers["Authorization"])
	}
	stdioSrv, _ := s2.GetServer("stdio-srv")
	if stdioSrv.Command != "/usr/bin/python3" {
		t.Errorf("Command mismatch: %s", stdioSrv.Command)
	}
	if len(stdioSrv.Args) != 2 || stdioSrv.Args[0] != "-m" {
		t.Errorf("Args not preserved: %v", stdioSrv.Args)
	}
}

// TestMCPService_SecretsEncryptedOnDisk verifies G-SEC-07: Header/Env
// secrets are encrypted on disk (not stored as plaintext).
func TestMCPService_SecretsEncryptedOnDisk(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp-enc.json")
	svc := &MCPService{cfgPath: cfgPath, clients: make(map[string]*MCPClient)}
	if err := svc.SaveServer(MCPServerConfig{
		Name: "s", Transport: "http", URL: "http://x",
		Headers: map[string]string{"Authorization": "Bearer topsecret"},
		Env:     map[string]string{"API_KEY": "sk-abc"},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Read the raw file and ensure the plaintext secrets do NOT appear.
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(raw), "topsecret") || strings.Contains(string(raw), "sk-abc") {
		t.Errorf("plaintext secret found on disk:\n%s", raw)
	}
	// But the in-memory config holds plaintext for connections.
	internal, _ := svc.getServerLocked("s")
	if internal.Headers["Authorization"] != "Bearer topsecret" || internal.Env["API_KEY"] != "sk-abc" {
		t.Errorf("in-memory plaintext lost: %+v", internal)
	}
}

// ---------------------------------------------------------------------------
// Agent integration: CheckCommand for mcp.* (Step 6/8)
// ---------------------------------------------------------------------------

func TestAgentService_CheckCommand_MCPNamespace_NoService(t *testing.T) {
	// An AgentService without MCPService should block mcp.* commands.
	agent := NewAgentService()
	check := agent.CheckCommand("mcp.server.tool")
	if !check.Blocked {
		t.Error("mcp.* command should be blocked when MCPService not set")
	}
	if check.RiskLevel != RiskDangerous {
		t.Errorf("expected RiskDangerous, got %s", check.RiskLevel)
	}
}

func TestAgentService_CheckCommand_MCPNamespace_InvalidFormat(t *testing.T) {
	agent := NewAgentService()
	agent.SetMCPService(newTestMCPService(t))
	// Too few parts.
	check := agent.CheckCommand("mcp.server")
	if !check.Blocked {
		t.Error("invalid mcp namespace should be blocked")
	}
	// Missing tool.
	check = agent.CheckCommand("mcp..")
	if !check.Blocked {
		t.Error("empty server/tool should be blocked")
	}
}

func TestAgentService_CheckCommand_MCPNamespace_UnknownTool(t *testing.T) {
	agent := NewAgentService()
	agent.SetMCPService(newTestMCPService(t))
	// No servers connected → tool not found → blocked.
	check := agent.CheckCommand("mcp.unknownsrv.unknowntool")
	if !check.Blocked {
		t.Error("unknown MCP tool should be blocked")
	}
}

func TestAgentService_CheckCommand_NonMCPStillWorks(t *testing.T) {
	agent := NewAgentService()
	agent.SetMCPService(newTestMCPService(t))
	// Regular shell commands should still be classified normally.
	check := agent.CheckCommand("ls -la")
	if check.Blocked {
		t.Error("ls should not be blocked")
	}
	if check.RiskLevel != RiskElevated {
		t.Errorf("expected RiskElevated for ls, got %s", check.RiskLevel)
	}
}

func TestAgentService_CallMCPTool_NoService(t *testing.T) {
	agent := NewAgentService()
	_, err := agent.CallMCPTool(context.Background(), "mcp.srv.tool", nil)
	if err == nil {
		t.Error("expected error when MCPService not configured")
	}
}

func TestAgentService_CallMCPTool_InvalidNamespace(t *testing.T) {
	agent := NewAgentService()
	agent.SetMCPService(newTestMCPService(t))
	_, err := agent.CallMCPTool(context.Background(), "not-mcp", nil)
	if err == nil {
		t.Error("expected error for non-mcp namespace")
	}
	_, err = agent.CallMCPTool(context.Background(), "mcp.only-two", nil)
	if err == nil {
		t.Error("expected error for 2-part namespace")
	}
}

// ---------------------------------------------------------------------------
// Transport config validation (Step 3)
// ---------------------------------------------------------------------------

func TestMCPService_SaveServer_AllTransports(t *testing.T) {
	s := newTestMCPService(t)
	// stdio
	if err := s.SaveServer(MCPServerConfig{
		Name: "stdio-srv", Transport: "stdio", Command: "node",
		Args: []string{"server.js"}, Env: map[string]string{"NODE_ENV": "production"},
	}); err != nil {
		t.Fatalf("stdio: %v", err)
	}
	// SSE
	if err := s.SaveServer(MCPServerConfig{
		Name: "sse-srv", Transport: "sse", URL: "http://localhost:3001/sse",
	}); err != nil {
		t.Fatalf("sse: %v", err)
	}
	// HTTP
	if err := s.SaveServer(MCPServerConfig{
		Name: "http-srv", Transport: "http", URL: "http://localhost:3002/mcp",
		Headers: map[string]string{"X-API-Key": "test"},
	}); err != nil {
		t.Fatalf("http: %v", err)
	}
	servers := s.ListServers()
	if len(servers) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(servers))
	}
}

func TestMCPServerConfig_AutoApprove_DefaultsEmpty(t *testing.T) {
	s := newTestMCPService(t)
	s.SaveServer(MCPServerConfig{
		Name: "srv", Transport: "stdio", Command: "echo",
	})
	got, _ := s.GetServer("srv")
	if len(got.AutoApprove) != 0 {
		// G-SEC-02: AutoApprove defaults to empty (no auto-approve).
		t.Errorf("AutoApprove should default to empty, got %v", got.AutoApprove)
	}
}

// ---------------------------------------------------------------------------
// ConnectServer requires Enabled=true (G-SEC-12)
// ---------------------------------------------------------------------------

func TestMCPService_ConnectServer_RequiresEnabled(t *testing.T) {
	s := newTestMCPService(t)
	s.SaveServer(MCPServerConfig{
		Name: "srv", Transport: "stdio", Command: "echo",
	})
	// New servers default to Enabled=false → ConnectServer should refuse.
	err := s.ConnectServer(context.Background(), "srv")
	if err == nil {
		t.Error("ConnectServer should refuse disabled server (G-SEC-12)")
	}
}

func TestMCPService_DisconnectServer_NotConnected(t *testing.T) {
	s := newTestMCPService(t)
	err := s.DisconnectServer("nope")
	if err == nil {
		t.Error("expected error for disconnecting non-connected server")
	}
}

// ---------------------------------------------------------------------------
// MCPClient lifecycle (unit tests without real server)
// ---------------------------------------------------------------------------

func TestMCPClient_StopServer_Idempotent(t *testing.T) {
	c := newMCPClient(MCPServerConfig{Name: "test", Transport: "stdio"})
	// StopServer on an unstarted client should be a safe no-op.
	if err := c.StopServer(); err != nil {
		t.Errorf("StopServer on unstarted client: %v", err)
	}
	// Double-stop should also be safe.
	if err := c.StopServer(); err != nil {
		t.Errorf("double StopServer: %v", err)
	}
}

func TestMCPClient_Call_NotStarted(t *testing.T) {
	c := newMCPClient(MCPServerConfig{Name: "test", Transport: "stdio"})
	_, err := c.call(context.Background(), "tools/list", nil)
	if err == nil {
		t.Error("call on unstarted client should fail")
	}
}
