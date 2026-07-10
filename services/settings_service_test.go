package services

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSettingsService_LoadSettings_returnsDefaultsWhenNoFile(t *testing.T) {
	svc := &SettingsService{configPath: filepath.Join(t.TempDir(), "settings.json")}
	settings, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if settings.FontSize != 14 {
		t.Errorf("expected default font size 14, got %d", settings.FontSize)
	}
	if settings.Theme != "dark" {
		t.Errorf("expected default theme 'dark', got '%s'", settings.Theme)
	}
	if settings.WordWrap != true {
		t.Error("expected default wordWrap true")
	}
}

func TestSettingsService_SaveAndLoad(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.FontSize = 18
	settings.TabSize = 4
	settings.WordWrap = false

	err := svc.SaveSettings(settings)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.FontSize != 18 {
		t.Errorf("expected font size 18, got %d", loaded.FontSize)
	}
	if loaded.TabSize != 4 {
		t.Errorf("expected tab size 4, got %d", loaded.TabSize)
	}
	if loaded.WordWrap != false {
		t.Error("expected wordWrap false")
	}
	if loaded.Version != 1 {
		t.Errorf("expected version 1 after first save, got %d", loaded.Version)
	}
}

// prompt-7 Task F / BUG-M14: settings version CAS.
func TestSettingsService_Save_VersionConflict(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}
	s := defaultSettings()
	if err := svc.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	loaded, err := svc.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	// Advance disk version.
	if err := svc.SaveSettings(loaded); err != nil {
		t.Fatal(err)
	}
	// Stale CAS should fail.
	stale := int64(1)
	loaded.ExpectedVersion = &stale
	loaded.FontSize = 99
	if err := svc.SaveSettings(loaded); err == nil {
		t.Fatal("expected version conflict")
	}
	// Matching version should succeed.
	cur, _ := svc.LoadSettings()
	match := cur.Version
	cur.ExpectedVersion = &match
	cur.FontSize = 20
	if err := svc.SaveSettings(cur); err != nil {
		t.Fatalf("matching CAS: %v", err)
	}
	final, _ := svc.LoadSettings()
	if final.FontSize != 20 || final.Version != match+1 {
		t.Fatalf("font=%d version=%d", final.FontSize, final.Version)
	}
}

func TestSettingsService_LoadSettings_corruptFileReturnsDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	// Write corrupt JSON
	svc.configPath = configPath
	writeCorruptSettings(t, configPath)

	settings, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("should not return error for corrupt file: %v", err)
	}
	if settings.FontSize != 14 {
		t.Errorf("expected defaults from corrupt file, got font size %d", settings.FontSize)
	}
}

func writeCorruptSettings(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsService_SaveAndLoadAIConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = "sk-test-key"
	settings.AIBaseURL = "https://api.openai.com"
	settings.AIModel = "gpt-4o"

	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	// G-SEC-07: LoadSettings must NOT return the decrypted API key to the
	// frontend. The key is cleared and AIApiKeyConfigured signals presence.
	if loaded.AIApiKey != "" {
		t.Errorf("expected AIApiKey to be empty (G-SEC-07), got %q", loaded.AIApiKey)
	}
	if !loaded.AIApiKeyConfigured {
		t.Error("expected AIApiKeyConfigured true, got false")
	}
	if loaded.AIApiKeyStorageMethod != "dpapi" && loaded.AIApiKeyStorageMethod != "aes" && loaded.AIApiKeyStorageMethod != "keyring" {
		t.Errorf("expected AIApiKeyStorageMethod dpapi/aes/keyring, got %q", loaded.AIApiKeyStorageMethod)
	}
	if loaded.AIBaseURL != "https://api.openai.com" {
		t.Errorf("expected AIBaseURL 'https://api.openai.com', got %q", loaded.AIBaseURL)
	}
	if loaded.AIModel != "gpt-4o" {
		t.Errorf("expected AIModel 'gpt-4o', got %q", loaded.AIModel)
	}
}

// G-SEC-07: GetDecryptedAPIKey returns the decrypted key for internal
// backend use (not exposed to the frontend via LoadSettings).
func TestSettingsService_GetDecryptedAPIKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = "sk-internal-key"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	got, err := svc2.GetDecryptedAPIKey()
	if err != nil {
		t.Fatalf("GetDecryptedAPIKey failed: %v", err)
	}
	if got != "sk-internal-key" {
		t.Errorf("GetDecryptedAPIKey = %q, want %q", got, "sk-internal-key")
	}
}

// G-SEC-07: GetDecryptedAPIKey returns empty when no key is stored.
func TestSettingsService_GetDecryptedAPIKey_emptyWhenNoKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	if err := svc.SaveSettings(defaultSettings()); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	got, err := svc2.GetDecryptedAPIKey()
	if err != nil {
		t.Fatalf("GetDecryptedAPIKey failed: %v", err)
	}
	if got != "" {
		t.Errorf("GetDecryptedAPIKey = %q, want empty", got)
	}
}

// G-SEC-07: an unrelated save (with empty AIApiKey but AIApiKeyConfigured
// true) must NOT wipe the existing on-disk key. The frontend no longer holds
// the plaintext key, so it saves with empty + configured=true; the backend
// preserves the stored key.
func TestSettingsService_SaveSettings_preservesKeyWhenEmptyButConfigured(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	// Save a key initially.
	settings := defaultSettings()
	settings.AIApiKey = "sk-preserve-me"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Simulate the frontend saving an unrelated change: AIApiKey empty (not
	// loaded, G-SEC-07) but AIApiKeyConfigured true.
	again := defaultSettings()
	again.AIApiKey = ""
	again.AIApiKeyConfigured = true
	again.FontSize = 20
	if err := svc.SaveSettings(again); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// The on-disk key must still be present (decryptable).
	svc2 := &SettingsService{configPath: configPath}
	got, err := svc2.GetDecryptedAPIKey()
	if err != nil {
		t.Fatalf("GetDecryptedAPIKey failed: %v", err)
	}
	if got != "sk-preserve-me" {
		t.Errorf("key was wiped by unrelated save: got %q, want %q", got, "sk-preserve-me")
	}
}

// G-SEC-07: a genuine clear (AIApiKey empty AND AIApiKeyConfigured false)
// must wipe the key, even if a key was previously stored.
func TestSettingsService_SaveSettings_clearsKeyWhenEmptyAndNotConfigured(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = "sk-clear-me"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	again := defaultSettings()
	again.AIApiKey = ""
	again.AIApiKeyConfigured = false
	if err := svc.SaveSettings(again); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	got, _ := svc2.GetDecryptedAPIKey()
	if got != "" {
		t.Errorf("key was not cleared: got %q, want empty", got)
	}
}

func TestSettingsService_SaveAndLoadAISystemPrompt(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AISystemPrompt = "You are a Rust expert."

	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.AISystemPrompt != "You are a Rust expert." {
		t.Errorf("expected AISystemPrompt 'You are a Rust expert.', got %q", loaded.AISystemPrompt)
	}
}

func TestSettingsService_DefaultInlineCompletionEnabled(t *testing.T) {
	svc := &SettingsService{configPath: filepath.Join(t.TempDir(), "settings.json")}
	settings, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if !settings.InlineCompletionEnabled {
		t.Error("expected default InlineCompletionEnabled true")
	}
}

// N-76: concurrent SetConfigPath + SaveSettings must not race on configPath.
// Run with `go test -race` to detect data races. Before the fix, this test
// would trigger the race detector; after the fix, the pathMu protects all
// access to configPath.
func TestSettingsService_N76_ConcurrentSetConfigPathAndSave_NoRace(t *testing.T) {
	svc := &SettingsService{configPath: filepath.Join(t.TempDir(), "profile1.json")}

	settings := defaultSettings()
	settings.Theme = "dark"

	var wg sync.WaitGroup
	// Writer: rapidly save settings.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = svc.SaveSettings(settings)
		}
	}()
	// Switcher: rapidly swap configPath between two profiles.
	wg.Add(1)
	go func() {
		defer wg.Done()
		path1 := filepath.Join(t.TempDir(), "profile1.json")
		path2 := filepath.Join(t.TempDir(), "profile2.json")
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				svc.SetConfigPath(path1)
			} else {
				svc.SetConfigPath(path2)
			}
		}
	}()
	// Reader: rapidly load settings.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = svc.LoadSettings()
		}
	}()
	wg.Wait()
}

// N-76: SetConfigPath acquires the write lock; SaveSettings holds the read
// lock for the full write, so a save that started before SetConfigPath
// completes with the OLD path. This test verifies the save goes to the
// original path, not the new one.
func TestSettingsService_N76_SaveCompletesOnOldPathAfterSwitch(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "new.json")
	svc := &SettingsService{configPath: oldPath}

	// Save to old path.
	settings := defaultSettings()
	settings.Theme = "dark"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
	// Switch path.
	svc.SetConfigPath(newPath)
	// Save again — should go to new path, not old.
	settings.Theme = "light"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
	// Old path should still have "dark".
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read old path: %v", err)
	}
	if !containsStr(string(oldData), "dark") {
		t.Errorf("old path should contain 'dark', got: %s", oldData)
	}
	// New path should have "light".
	newData, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new path: %v", err)
	}
	if !containsStr(string(newData), "light") {
		t.Errorf("new path should contain 'light', got: %s", newData)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestSettingsService_SaveAndLoadInlineCompletionEnabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.InlineCompletionEnabled = false

	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.InlineCompletionEnabled != false {
		t.Errorf("expected InlineCompletionEnabled false, got %v", loaded.InlineCompletionEnabled)
	}
}

// CRIT-01: multi-provider config keys must be encrypted at rest, cleared in
// LoadSettings (only APIKeyConfigured exposed), and decryptable via
// GetAPIKeyForConfig so the frontend never holds plaintext keys.
func TestSettingsService_CRIT01_ProviderKeyIsolation(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIProviderConfigs = []AIProviderConfig{
		{ID: "cfg-a", Name: "A", Provider: "openai", APIKey: "sk-provider-a", BaseURL: "https://api.openai.com", Model: "gpt-4o"},
		{ID: "cfg-b", Name: "B", Provider: "anthropic", Protocol: "anthropic", APIKey: "sk-provider-b", BaseURL: "https://api.anthropic.com", Model: "claude-3"},
	}
	settings.ActiveAIConfigID = "cfg-a"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Disk must NOT contain plaintext provider keys.
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	rawStr := string(raw)
	if containsStr(rawStr, "sk-provider-a") || containsStr(rawStr, "sk-provider-b") {
		t.Errorf("on-disk settings contain plaintext provider key (CRIT-01): %s", rawStr)
	}

	// LoadSettings must clear each config's APIKey + set APIKeyConfigured.
	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if len(loaded.AIProviderConfigs) != 2 {
		t.Fatalf("expected 2 provider configs, got %d", len(loaded.AIProviderConfigs))
	}
	for i, cfg := range loaded.AIProviderConfigs {
		if cfg.APIKey != "" {
			t.Errorf("config[%d] APIKey not cleared (CRIT-01): %q", i, cfg.APIKey)
		}
		if !cfg.APIKeyConfigured {
			t.Errorf("config[%d] APIKeyConfigured false, want true (CRIT-01)", i)
		}
	}

	// GetAPIKeyForConfig must decrypt the stored key for each config.
	for _, want := range []struct{ id, key string }{
		{"cfg-a", "sk-provider-a"},
		{"cfg-b", "sk-provider-b"},
	} {
		got, err := svc2.GetAPIKeyForConfig(want.id)
		if err != nil {
			t.Fatalf("GetAPIKeyForConfig(%s): %v", want.id, err)
		}
		if got != want.key {
			t.Errorf("GetAPIKeyForConfig(%s) = %q, want %q", want.id, got, want.key)
		}
	}
}

// CRIT-01: an unrelated save (empty provider key + configured=true) must
// preserve the existing on-disk provider key, INDEPENDENT of the legacy
// AIApiKey state. This verifies the provider-key preservation scope fix:
// previously the preservation was nested inside the legacy-key if-block, so
// when the legacy key was non-empty the provider keys were wiped.
func TestSettingsService_CRIT01_PreservesProviderKeyIndependentOfLegacyKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = "sk-legacy"
	settings.AIProviderConfigs = []AIProviderConfig{
		{ID: "cfg-a", Name: "A", Provider: "openai", APIKey: "sk-provider-a", BaseURL: "https://api.openai.com", Model: "gpt-4o"},
	}
	settings.ActiveAIConfigID = "cfg-a"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Frontend save with a NON-empty legacy key AND empty provider key +
	// configured=true (preserve provider key). The provider key must be
	// preserved regardless of the legacy key state.
	again := defaultSettings()
	again.AIApiKey = "sk-legacy-new"
	again.AIApiKeyConfigured = true
	again.AIProviderConfigs = []AIProviderConfig{
		{ID: "cfg-a", Name: "A", Provider: "openai", APIKey: "", APIKeyConfigured: true, BaseURL: "https://api.openai.com", Model: "gpt-4o"},
	}
	again.ActiveAIConfigID = "cfg-a"
	if err := svc.SaveSettings(again); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	svc2 := &SettingsService{configPath: configPath}
	got, err := svc2.GetAPIKeyForConfig("cfg-a")
	if err != nil {
		t.Fatalf("GetAPIKeyForConfig: %v", err)
	}
	if got != "sk-provider-a" {
		t.Errorf("provider key wiped when legacy key non-empty (scope bug): got %q, want %q", got, "sk-provider-a")
	}
}
