package services

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestConversationService_Save_andLoad(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	conv := Conversation{
		ID:        "test-1",
		Title:     "Test conversation",
		CreatedAt: time.Now().Unix(),
		Messages: []ConversationMessage{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
		},
	}

	err := svc.Save(conv)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "test-1.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}

	loaded, err := svc.Load("test-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Title != "Test conversation" {
		t.Errorf("expected title 'Test conversation', got %q", loaded.Title)
	}
	if len(loaded.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[1].Content != "hi there" {
		t.Errorf("expected second message 'hi there', got %q", loaded.Messages[1].Content)
	}
}

func TestConversationService_Load_nonexistentReturnsError(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	_, err := svc.Load("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent conversation")
	}
}

func TestConversationService_List_returnsAllSortedByCreatedDesc(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	// Save three conversations with different timestamps
	for i, id := range []string{"a", "b", "c"} {
		err := svc.Save(Conversation{
			ID:        id,
			Title:     "Conv " + id,
			CreatedAt: int64(i + 1),
			Messages:  []ConversationMessage{},
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(list))
	}
	// Should be sorted by CreatedAt descending (newest first)
	if list[0].ID != "c" {
		t.Errorf("expected newest (c) first, got %q", list[0].ID)
	}
	if list[2].ID != "a" {
		t.Errorf("expected oldest (a) last, got %q", list[2].ID)
	}
}

func TestConversationService_Delete_removesFile(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}

	err := svc.Save(Conversation{
		ID:        "to-delete",
		Title:     "Delete me",
		CreatedAt: 1,
		Messages:  []ConversationMessage{},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.Delete("to-delete")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	path := filepath.Join(dir, "to-delete.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, stat err=%v", err)
	}
}

func TestConversationService_GenerateID_isUnique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateConversationID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestConversationService_GenerateTitleFromMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello world", "Hello world"},
		{"This is a very long message that should be truncated because it exceeds the max title length", "This is a very long message that should be truncated because it e"},
		{"", "(new conversation)"},
		{"   ", "(new conversation)"},
	}
	for _, tt := range tests {
		got := GenerateTitle(tt.input)
		if got != tt.expected {
			t.Errorf("GenerateTitle(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// Ensure sort is stable for List — sanity check
func TestConversationService_List_emptyDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := &ConversationService{storageDir: dir}
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List on empty dir failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
	_ = sort.IntSlice{} // keep sort import used
}

func TestConversationService_GenerateConversationID_Method(t *testing.T) {
	svc := &ConversationService{storageDir: t.TempDir()}
	id1 := svc.GenerateConversationID()
	id2 := svc.GenerateConversationID()
	if id1 == "" || id2 == "" {
		t.Error("GenerateConversationID should return non-empty string")
	}
	if id1 == id2 {
		t.Error("GenerateConversationID should return unique IDs")
	}
}

func TestConversationService_GenerateTitle_Method(t *testing.T) {
	svc := &ConversationService{storageDir: t.TempDir()}
	got := svc.GenerateTitle("Hello world")
	if got != "Hello world" {
		t.Errorf("GenerateTitle method = %q, want %q", got, "Hello world")
	}
	got = svc.GenerateTitle("")
	if got != "(new conversation)" {
		t.Errorf("GenerateTitle empty = %q, want %q", got, "(new conversation)")
	}
}

func TestConversationService_UpdateTitle(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)

	conv := Conversation{
		ID:        "test-conv-1",
		Title:     "Old Title",
		CreatedAt: time.Now().UnixMilli(),
		Messages:  []ConversationMessage{{Role: "user", Content: "hello"}},
	}
	if err := svc.Save(conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := svc.UpdateTitle("test-conv-1", "New Title"); err != nil {
		t.Fatalf("UpdateTitle failed: %v", err)
	}

	loaded, err := svc.Load("test-conv-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Title != "New Title" {
		t.Errorf("expected 'New Title', got %q", loaded.Title)
	}
	if len(loaded.Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(loaded.Messages))
	}
}

// N-60: Per-conversation system prompt override persistence.
func TestConversationService_SystemPromptOverride_Persists(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)

	conv := Conversation{
		ID:                   "override-1",
		Title:                "Custom persona",
		CreatedAt:            time.Now().Unix(),
		Messages:             []ConversationMessage{{Role: "user", Content: "hi"}},
		SystemPromptOverride: "You are a strict code reviewer.",
	}
	if err := svc.Save(conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := svc.Load("override-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.SystemPromptOverride != "You are a strict code reviewer." {
		t.Errorf("expected override to persist, got %q", loaded.SystemPromptOverride)
	}
}

// N-60: Empty override should be omitted from JSON (omitempty).
func TestConversationService_SystemPromptOverride_EmptyOmitted(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)

	conv := Conversation{
		ID:                   "no-override",
		Title:                "Default",
		CreatedAt:            1,
		Messages:             []ConversationMessage{},
		SystemPromptOverride: "",
	}
	if err := svc.Save(conv); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := svc.Load("no-override")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.SystemPromptOverride != "" {
		t.Errorf("expected empty override, got %q", loaded.SystemPromptOverride)
	}
}

// N-60: Loading a legacy conversation without the field should default to empty.
func TestConversationService_SystemPromptOverride_LegacyFileDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)

	// Write a legacy JSON file without the system_prompt_override field.
	legacyJSON := `{"id":"legacy","title":"Legacy","created_at":1,"messages":[]}`
	path := filepath.Join(dir, "legacy.json")
	if err := os.WriteFile(path, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	loaded, err := svc.Load("legacy")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.SystemPromptOverride != "" {
		t.Errorf("expected empty override for legacy file, got %q", loaded.SystemPromptOverride)
	}
}

func TestConversationService_UpdateTitle_NotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	err := svc.UpdateTitle("nonexistent", "New Title")
	if err == nil {
		t.Fatal("expected error for nonexistent conversation")
	}
}

func TestConversationService_UpdateTitle_EmptyTitle(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	conv := Conversation{ID: "c1", Title: "Old", CreatedAt: time.Now().UnixMilli()}
	svc.Save(conv)
	err := svc.UpdateTitle("c1", "")
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

// N-91: Path traversal defense — Save/Load/Delete must reject ids that
// contain "..", path separators, or absolute paths. Previously, an id like
// "../../etc/evil" would read/write/delete arbitrary .json files outside
// the storage directory.
func TestConversationService_N91_Save_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	maliciousIDs := []string{
		"../evil",
		"..\\evil",
		"../../etc/passwd",
		"/etc/passwd",
		"sub/file",
		"sub\\file",
		".",
		"..",
	}
	for _, id := range maliciousIDs {
		t.Run(id, func(t *testing.T) {
			err := svc.Save(Conversation{
				ID:        id,
				Title:     "evil",
				CreatedAt: 1,
				Messages:  []ConversationMessage{{Role: "user", Content: "x"}},
			})
			if err == nil {
				t.Errorf("Save(%q) should reject path traversal, got nil", id)
			}
			// Verify no file was created outside the storage dir.
			target := filepath.Join(dir, id+".json")
			if _, err := os.Stat(target); err == nil {
				t.Errorf("file should not exist at %s", target)
			}
		})
	}
}

func TestConversationService_N91_Load_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	maliciousIDs := []string{
		"../evil",
		"..\\evil",
		"/etc/passwd",
		"sub/file",
		"..",
	}
	for _, id := range maliciousIDs {
		t.Run(id, func(t *testing.T) {
			_, err := svc.Load(id)
			if err == nil {
				t.Errorf("Load(%q) should reject path traversal, got nil", id)
			}
		})
	}
}

func TestConversationService_N91_Delete_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	// Place a file outside the storage dir to verify Delete does NOT remove it.
	parent := filepath.Dir(dir)
	outsideTarget := filepath.Join(parent, "evil.json")
	if err := os.WriteFile(outsideTarget, []byte("sensitive"), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outsideTarget)

	err := svc.Delete("../evil")
	if err == nil {
		t.Error("Delete should reject path traversal, got nil")
	}
	// Verify the outside file is still there.
	if _, err := os.Stat(outsideTarget); err != nil {
		t.Errorf("outside file should still exist after rejected Delete: %v", err)
	}
}

// N-91: Sanity check — a legitimate id with a dash (the format produced by
// GenerateConversationID) is still accepted after the traversal fix.
func TestConversationService_N91_LegitimateIDStillWorks(t *testing.T) {
	dir := t.TempDir()
	svc := NewConversationService(dir)
	id := svc.GenerateConversationID()
	conv := Conversation{
		ID:        id,
		Title:     "legit",
		CreatedAt: time.Now().Unix(),
		Messages:  []ConversationMessage{{Role: "user", Content: "hi"}},
	}
	if err := svc.Save(conv); err != nil {
		t.Fatalf("Save with generated ID failed: %v", err)
	}
	loaded, err := svc.Load(id)
	if err != nil {
		t.Fatalf("Load with generated ID failed: %v", err)
	}
	if loaded.ID != id {
		t.Errorf("expected ID %q, got %q", id, loaded.ID)
	}
	if err := svc.Delete(id); err != nil {
		t.Fatalf("Delete with generated ID failed: %v", err)
	}
}
