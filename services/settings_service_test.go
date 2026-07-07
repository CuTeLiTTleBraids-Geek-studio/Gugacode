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
	if loaded.AIApiKey != "sk-test-key" {
		t.Errorf("expected AIApiKey 'sk-test-key', got %q", loaded.AIApiKey)
	}
	if loaded.AIBaseURL != "https://api.openai.com" {
		t.Errorf("expected AIBaseURL 'https://api.openai.com', got %q", loaded.AIBaseURL)
	}
	if loaded.AIModel != "gpt-4o" {
		t.Errorf("expected AIModel 'gpt-4o', got %q", loaded.AIModel)
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
