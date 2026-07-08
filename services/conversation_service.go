package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ConversationMessage is a single message in a persisted conversation.
type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Conversation is a saved AI chat conversation.
type Conversation struct {
	ID        string               `json:"id"`
	Title     string               `json:"title"`
	CreatedAt int64                `json:"created_at"`
	Messages  []ConversationMessage `json:"messages"`
	// SystemPromptOverride (N-60): when non-empty, this conversation uses
	// a custom system prompt instead of the global appState.aiSystemPrompt.
	// This allows per-session persona customization (e.g. "strict code reviewer"
	// for one conversation, "creative brainstorm partner" for another).
	// Empty string means "use the global default".
	SystemPromptOverride string `json:"system_prompt_override,omitempty"`
}

// ConversationService persists AI conversations to disk as JSON files.
type ConversationService struct {
	storageDir string
}

// NewConversationService creates a ConversationService rooted at the given directory.
func NewConversationService(storageDir string) *ConversationService {
	return &ConversationService{storageDir: storageDir}
}

// defaultStorageDir returns the user config dir + "/gugacode/conversations".
func defaultStorageDir() string {
	home, err := os.UserConfigDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "gugacode", "conversations")
	}
	return filepath.Join(home, "gugacode", "conversations")
}

// ensureDir creates the storage directory if it doesn't exist.
func (s *ConversationService) ensureDir() error {
	dir := s.storageDir
	if dir == "" {
		dir = defaultStorageDir()
		s.storageDir = dir
	}
	return os.MkdirAll(dir, 0755)
}

// pathFor returns the absolute path for a conversation file after validating
// that id is a safe filename component. Returns an error if id is empty,
// contains path separators, parent traversal (".."), or is an absolute path.
//
// N-91: previously this used filepath.Join(s.storageDir, id+".json")
// without any sanitization, allowing id values like "../../etc/evil" to
// read/write/delete arbitrary .json files. We now use SafeNameJoin from
// pathsec.go which rejects path separators, parent traversal, and absolute
// paths via IsRelativePathSafe.
func (s *ConversationService) pathFor(id string) (string, error) {
	return SafeNameJoin(s.storageDir, id, ".json")
}

// Save writes a conversation to disk.
func (s *ConversationService) Save(conv Conversation) error {
	if conv.ID == "" {
		return errors.New("conversation ID is required")
	}
	// N-134: limit SystemPromptOverride to prevent excessive disk usage.
	const maxSystemPromptOverrideLen = 100_000
	if len(conv.SystemPromptOverride) > maxSystemPromptOverrideLen {
		return fmt.Errorf("system prompt override exceeds maximum length of %d characters", maxSystemPromptOverrideLen)
	}
	path, err := s.pathFor(conv.ID)
	if err != nil {
		return fmt.Errorf("invalid conversation ID: %w", err)
	}
	if err := s.ensureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads a conversation by ID.
func (s *ConversationService) Load(id string) (Conversation, error) {
	var conv Conversation
	path, err := s.pathFor(id)
	if err != nil {
		return conv, fmt.Errorf("invalid conversation ID: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return conv, err
	}
	if err := json.Unmarshal(data, &conv); err != nil {
		return conv, err
	}
	return conv, nil
}

// List returns all conversations sorted by CreatedAt descending (newest first).
func (s *ConversationService) List() ([]Conversation, error) {
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.storageDir)
	if err != nil {
		return nil, err
	}
	var convs []Conversation
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.storageDir, entry.Name()))
		if err != nil {
			continue
		}
		var conv Conversation
		if err := json.Unmarshal(data, &conv); err != nil {
			continue
		}
		convs = append(convs, conv)
	}
	sort.Slice(convs, func(i, j int) bool {
		return convs[i].CreatedAt > convs[j].CreatedAt
	})
	return convs, nil
}

// Delete removes a conversation file.
func (s *ConversationService) Delete(id string) error {
	path, err := s.pathFor(id)
	if err != nil {
		return fmt.Errorf("invalid conversation ID: %w", err)
	}
	return os.Remove(path)
}

// UpdateTitle renames an existing conversation. Returns an error if the
// conversation doesn't exist or the new title is empty.
func (s *ConversationService) UpdateTitle(id string, title string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title cannot be empty")
	}
	conv, err := s.Load(id)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}
	conv.Title = strings.TrimSpace(title)
	return s.Save(conv)
}

// GenerateConversationID returns a random 16-byte hex ID with a time-based prefix.
func (s *ConversationService) GenerateConversationID() string {
	return GenerateConversationID()
}

// GenerateTitle returns a title derived from the first user message.
// Truncates to 65 characters; returns "(new conversation)" for empty input.
func (s *ConversationService) GenerateTitle(firstMessage string) string {
	return GenerateTitle(firstMessage)
}

// GenerateConversationID returns a random 16-byte hex ID with a time-based prefix.
func GenerateConversationID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// GenerateTitle returns a title derived from the first user message.
// Truncates to 65 characters; returns "(new conversation)" for empty input.
func GenerateTitle(firstMessage string) string {
	s := strings.TrimSpace(firstMessage)
	if s == "" {
		return "(new conversation)"
	}
	const maxLen = 65
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
