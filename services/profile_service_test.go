package services

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestProfileService creates a ProfileService rooted at a temp dir.
// The caller is responsible for cleanup (t.TempDir() handles it).
func newTestProfileService(t *testing.T) *ProfileService {
	t.Helper()
	return NewProfileService(t.TempDir())
}

// writeLegacySettings writes a settings.json at the legacy location
// (rootDir/settings.json) with the given content. Used to test
// migration behavior.
func writeLegacySettings(t *testing.T, s *ProfileService, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.legacyPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProfileService_EnsureProfilesDir_CreatesDefault(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.ensureProfilesDir(); err != nil {
		t.Fatal(err)
	}
	// Default profile directory should exist.
	defaultDir := filepath.Join(s.profilesDir, "default")
	if _, err := os.Stat(defaultDir); os.IsNotExist(err) {
		t.Fatal("default profile directory was not created")
	}
	// Default settings.json should exist.
	settingsPath := filepath.Join(defaultDir, "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("default profile settings.json was not created")
	}
	// State file should exist with "default" as active.
	if got := s.loadState(); got != "default" {
		t.Fatalf("expected active profile 'default', got %q", got)
	}
}

func TestProfileService_EnsureProfilesDir_MigratesLegacy(t *testing.T) {
	s := newTestProfileService(t)
	// Write a legacy settings.json before any profile structure exists.
	legacyContent := `{"language":"zh","theme":"light","fontSize":16}`
	writeLegacySettings(t, s, legacyContent)

	if err := s.ensureProfilesDir(); err != nil {
		t.Fatal(err)
	}
	// The default profile's settings.json should contain the legacy content.
	defaultSettingsPath := filepath.Join(s.profilesDir, "default", "settings.json")
	data, err := os.ReadFile(defaultSettingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != legacyContent {
		t.Fatalf("expected migrated content %q, got %q", legacyContent, string(data))
	}
}

func TestProfileService_ListProfiles_Empty(t *testing.T) {
	s := newTestProfileService(t)
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile (default), got %d", len(profiles))
	}
	if profiles[0].Name != "default" {
		t.Fatalf("expected 'default', got %q", profiles[0].Name)
	}
	if !profiles[0].Active {
		t.Fatal("expected default profile to be active")
	}
}

func TestProfileService_ListProfiles_SortedWithDefaultFirst(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("zebra", false); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfile("alpha", false); err != nil {
		t.Fatal(err)
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}
	// "default" should always sort first.
	if profiles[0].Name != "default" {
		t.Fatalf("expected 'default' first, got %q", profiles[0].Name)
	}
	// Then alphabetical.
	if profiles[1].Name != "alpha" {
		t.Fatalf("expected 'alpha' second, got %q", profiles[1].Name)
	}
	if profiles[2].Name != "zebra" {
		t.Fatalf("expected 'zebra' third, got %q", profiles[2].Name)
	}
}

func TestProfileService_GetActiveProfile_Default(t *testing.T) {
	s := newTestProfileService(t)
	active, err := s.GetActiveProfile()
	if err != nil {
		t.Fatal(err)
	}
	if active != "default" {
		t.Fatalf("expected 'default', got %q", active)
	}
}

func TestProfileService_SetActiveProfile_Switches(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveProfile("work"); err != nil {
		t.Fatal(err)
	}
	active, _ := s.GetActiveProfile()
	if active != "work" {
		t.Fatalf("expected 'work', got %q", active)
	}
	// ListProfiles should flag 'work' as active.
	profiles, _ := s.ListProfiles()
	for _, p := range profiles {
		if p.Name == "work" && !p.Active {
			t.Fatal("expected 'work' to be flagged active")
		}
		if p.Name == "default" && p.Active {
			t.Fatal("expected 'default' to NOT be flagged active")
		}
	}
}

func TestProfileService_SetActiveProfile_NotFound(t *testing.T) {
	s := newTestProfileService(t)
	err := s.SetActiveProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
}

func TestProfileService_SetActiveProfile_InvalidName(t *testing.T) {
	s := newTestProfileService(t)
	err := s.SetActiveProfile("UPPERCASE")
	if err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestProfileService_SetActiveProfile_OnSwitchCallback(t *testing.T) {
	s := newTestProfileService(t)
	var calledWithPath string
	s.SetOnSwitch(func(p string) { calledWithPath = p })

	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveProfile("work"); err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(s.profilesDir, "work", "settings.json")
	if calledWithPath != expected {
		t.Fatalf("expected callback path %q, got %q", expected, calledWithPath)
	}
}

func TestProfileService_CreateProfile_Defaults(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("test", false); err != nil {
		t.Fatal(err)
	}
	// The new profile should have a settings.json with default content.
	data, err := os.ReadFile(filepath.Join(s.profilesDir, "test", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty default settings")
	}
}

func TestProfileService_CreateProfile_FromCurrent(t *testing.T) {
	s := newTestProfileService(t)
	// Write custom content to the default profile's settings.json.
	customContent := `{"language":"ja","theme":"dark","fontSize":12}`
	defaultPath := filepath.Join(s.profilesDir, "default", "settings.json")
	if err := s.ensureProfilesDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultPath, []byte(customContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create "work" from current (default) — should copy custom content.
	if err := s.CreateProfile("work", true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.profilesDir, "work", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != customContent {
		t.Fatalf("expected copied content %q, got %q", customContent, string(data))
	}
}

func TestProfileService_CreateProfile_AlreadyExists(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	err := s.CreateProfile("work", false)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestProfileService_CreateProfile_ReservedDefault(t *testing.T) {
	s := newTestProfileService(t)
	err := s.CreateProfile("default", false)
	if err == nil {
		t.Fatal("expected error when creating 'default'")
	}
}

func TestProfileService_CreateProfile_InvalidName(t *testing.T) {
	s := newTestProfileService(t)
	cases := []string{"UPPER", "with space", "under_score", "dot.name", ""}
	for _, name := range cases {
		if err := s.CreateProfile(name, false); err == nil {
			t.Fatalf("expected error for invalid name %q", name)
		}
	}
}

func TestProfileService_DeleteProfile_Success(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProfile("work"); err != nil {
		t.Fatal(err)
	}
	// Profile directory should be gone.
	if _, err := os.Stat(filepath.Join(s.profilesDir, "work")); err == nil {
		t.Fatal("expected profile directory to be deleted")
	}
}

func TestProfileService_DeleteProfile_DefaultProtected(t *testing.T) {
	s := newTestProfileService(t)
	err := s.DeleteProfile("default")
	if err == nil {
		t.Fatal("expected error when deleting default profile")
	}
}

func TestProfileService_DeleteProfile_ActiveProtected(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveProfile("work"); err != nil {
		t.Fatal(err)
	}
	err := s.DeleteProfile("work")
	if err == nil {
		t.Fatal("expected error when deleting active profile")
	}
}

func TestProfileService_DeleteProfile_NotFound(t *testing.T) {
	s := newTestProfileService(t)
	err := s.DeleteProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
}

func TestProfileService_RenameProfile_Success(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameProfile("work", "office"); err != nil {
		t.Fatal(err)
	}
	// Old directory gone, new directory exists.
	if _, err := os.Stat(filepath.Join(s.profilesDir, "work")); !os.IsNotExist(err) {
		t.Fatal("expected old profile directory to be gone")
	}
	if _, err := os.Stat(filepath.Join(s.profilesDir, "office")); os.IsNotExist(err) {
		t.Fatal("expected new profile directory to exist")
	}
}

func TestProfileService_RenameProfile_DefaultProtected(t *testing.T) {
	s := newTestProfileService(t)
	err := s.RenameProfile("default", "other")
	if err == nil {
		t.Fatal("expected error when renaming default profile")
	}
}

func TestProfileService_RenameProfile_TargetExists(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProfile("office", false); err != nil {
		t.Fatal(err)
	}
	err := s.RenameProfile("work", "office")
	if err == nil {
		t.Fatal("expected error when target name exists")
	}
}

func TestProfileService_RenameProfile_ActiveUpdated(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveProfile("work"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameProfile("work", "office"); err != nil {
		t.Fatal(err)
	}
	active, _ := s.GetActiveProfile()
	if active != "office" {
		t.Fatalf("expected active profile 'office', got %q", active)
	}
}

func TestProfileService_SetProfileDescription(t *testing.T) {
	s := newTestProfileService(t)
	if err := s.SetProfileDescription("default", "My main profile"); err != nil {
		t.Fatal(err)
	}
	profiles, _ := s.ListProfiles()
	for _, p := range profiles {
		if p.Name == "default" {
			if p.Description != "My main profile" {
				t.Fatalf("expected description 'My main profile', got %q", p.Description)
			}
			return
		}
	}
	t.Fatal("default profile not found")
}

func TestProfileService_ExportProfile(t *testing.T) {
	s := newTestProfileService(t)
	// Customize default settings.
	customContent := `{"language":"zh","theme":"light"}`
	if err := s.ensureProfilesDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(s.profilesDir, "default", "settings.json"),
		[]byte(customContent), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	export, err := s.ExportProfile("default")
	if err != nil {
		t.Fatal(err)
	}
	if export.Name != "default" {
		t.Fatalf("expected name 'default', got %q", export.Name)
	}
	if string(export.Settings) != customContent {
		t.Fatalf("expected settings %q, got %q", customContent, string(export.Settings))
	}
	if export.ExportedAt == 0 {
		t.Fatal("expected non-zero ExportedAt")
	}
}

func TestProfileService_ExportProfile_NotFound(t *testing.T) {
	s := newTestProfileService(t)
	_, err := s.ExportProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
}

func TestProfileService_ImportProfile_Success(t *testing.T) {
	s := newTestProfileService(t)
	export := ProfileExport{
		Name:     "imported",
		Settings: []byte(`{"language":"ja","theme":"dark"}`),
	}
	name, err := s.ImportProfile(export)
	if err != nil {
		t.Fatal(err)
	}
	if name != "imported" {
		t.Fatalf("expected name 'imported', got %q", name)
	}
	// Settings should be written to the new profile's settings.json.
	data, err := os.ReadFile(filepath.Join(s.profilesDir, "imported", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"language":"ja","theme":"dark"}` {
		t.Fatalf("unexpected settings content: %q", string(data))
	}
}

func TestProfileService_ImportProfile_NameCollision(t *testing.T) {
	s := newTestProfileService(t)
	// Pre-create "imported".
	if err := s.CreateProfile("imported", false); err != nil {
		t.Fatal(err)
	}
	// Import with name "imported" — should auto-rename to "imported-2".
	export := ProfileExport{
		Name:     "imported",
		Settings: []byte(`{}`),
	}
	name, err := s.ImportProfile(export)
	if err != nil {
		t.Fatal(err)
	}
	if name != "imported-2" {
		t.Fatalf("expected 'imported-2', got %q", name)
	}
}

func TestProfileService_ImportProfile_DefaultNameRejected(t *testing.T) {
	s := newTestProfileService(t)
	export := ProfileExport{
		Name:     "default",
		Settings: []byte(`{}`),
	}
	name, err := s.ImportProfile(export)
	if err != nil {
		t.Fatal(err)
	}
	// Should fall back to "imported".
	if name != "imported" {
		t.Fatalf("expected 'imported', got %q", name)
	}
}

func TestProfileService_ActiveSettingsPath(t *testing.T) {
	s := newTestProfileService(t)
	path, err := s.ActiveSettingsPath()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(s.profilesDir, "default", "settings.json")
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
	// After switching, the path should change.
	if err := s.CreateProfile("work", false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetActiveProfile("work"); err != nil {
		t.Fatal(err)
	}
	path, _ = s.ActiveSettingsPath()
	expectedWork := filepath.Join(s.profilesDir, "work", "settings.json")
	if path != expectedWork {
		t.Fatalf("expected %q, got %q", expectedWork, path)
	}
}

func TestProfileService_NoConfigDir(t *testing.T) {
	s := NewProfileService("")
	_, err := s.ListProfiles()
	if err == nil {
		t.Fatal("expected error when config dir is empty")
	}
}
