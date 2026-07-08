package services

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
)

// Settings holds all persisted application settings.
//
// The AIApiKey field is special (N-13): on disk it is stored encrypted with a
// "dpapi:" (Windows) or "aes:" (other platforms) prefix. In memory (after
// LoadSettings) it holds the plaintext key. SaveSettings always re-encrypts
// before writing. Legacy plaintext keys (no prefix) are auto-migrated to
// encrypted form on the first LoadSettings.
//
// CustomShortcuts (N-8) maps a shortcut label (e.g. "Save File") to a
// user-defined key combination that overrides the default binding. The map
// may be nil when no customizations have been made.
type Settings struct {
	Language    string `json:"language"`
	Theme       string `json:"theme"`
	FontSize    int    `json:"fontSize"`
	FontFamily  string `json:"fontFamily"`
	TabSize     int    `json:"tabSize"`
	WordWrap    bool   `json:"wordWrap"`
	LineNumbers bool   `json:"lineNumbers"`
	Minimap     bool   `json:"minimap"`
	AIApiKey       string `json:"aiApiKey"`
	AIBaseURL      string `json:"aiBaseUrl"`
	AIModel        string `json:"aiModel"`
	AISystemPrompt string `json:"aiSystemPrompt"`
	// Plan 54: optional overrides for the other three built-in prompts.
	// When non-empty, the AIService returns these instead of the built-in
	// const. Empty string means "use the built-in".
	AIAgentSystemPrompt         string `json:"aiAgentSystemPrompt,omitempty"`
	AIConversationTitlePrompt   string `json:"aiConversationTitlePrompt,omitempty"`
	AIInlineCompletionPrompt    string `json:"aiInlineCompletionPrompt,omitempty"`
	CursorBlinking          string  `json:"cursorBlinking"`
	CursorStyle             string  `json:"cursorStyle"`
	BracketColorization     bool    `json:"bracketColorization"`
	AutoSave                bool    `json:"autoSave"`
	AutoSaveDelay           string  `json:"autoSaveDelay"`
	AIProvider              string  `json:"aiProvider"`
	Temperature             float64 `json:"temperature"`
	MaxTokens               int     `json:"maxTokens"`
	DefaultShell            string  `json:"defaultShell"`
	TerminalFontSize        int     `json:"terminalFontSize"`
	TerminalCursorStyle     string  `json:"terminalCursorStyle"`
	Scrollback              int     `json:"scrollback"`
	UIDensity               string  `json:"uiDensity"`
	FontSizeScaling         int     `json:"fontSizeScaling"`
	InlineCompletionEnabled bool    `json:"inlineCompletionEnabled"`
	CustomShortcuts         map[string]ShortcutKeys `json:"customShortcuts,omitempty"`
	// N-20: layout state. AiChatPosition omitempty is safe (empty defaults to
	// "right" on the frontend). ActivityBarVisible must NOT use omitempty —
	// otherwise "false" (the zero value) would be dropped and reload as "true".
	AiChatPosition     string `json:"aiChatPosition,omitempty"`
	ActivityBarVisible bool   `json:"activityBarVisible"`
	// Plan 47: per-tool-kind approval policy. Keys are tool kinds
	// ("read"/"write"/"run"/"search"/custom), values are policy strings
	// ("always-ask"/"auto-approve"/"never-approve"). Missing keys default
	// to "always-ask" on the frontend. omitempty is safe — an empty map
	// is equivalent to all-default.
	ToolApprovalConfig map[string]string `json:"toolApprovalConfig,omitempty"`
	// Plan 48: accent theme key. Can be a built-in ("blue", "teal", ...)
	// or "custom". Empty defaults to "blue" on the frontend.
	AccentTheme string `json:"accentTheme,omitempty"`
	// Plan 48: custom accent theme definition. Only set when AccentTheme
	// === "custom". Pointer so nil is distinct from a zero-value struct.
	CustomAccent *CustomAccentTheme `json:"customAccent,omitempty"`
	// N-29: plugin sandbox mode. When true, plugins run in isolated Web
	// Workers with no DOM access. Defaults to true (v2 behavior). Users
	// can disable it for compatibility with v1 main-thread plugins.
	// omitempty is NOT used — false must round-trip correctly.
	EnablePluginSandbox bool `json:"enablePluginSandbox"`
	// Multi-provider AI configs (CC Switch-style). AIProviderConfigs holds
	// an unordered list of named configurations (each with its own provider
	// / apiKey / baseUrl / model / temperature / maxTokens / systemPrompt).
	// ActiveAIConfigID points at the currently active config's ID. The
	// legacy single-config fields (AIApiKey/AIBaseURL/AIModel/AIProvider/
	// Temperature/MaxTokens/AISystemPrompt) are kept as a mirror of the
	// active config so existing AI call paths work unchanged; switching
	// the active config syncs these fields.
	AIProviderConfigs  []AIProviderConfig `json:"aiProviderConfigs,omitempty"`
	ActiveAIConfigID   string             `json:"activeAIConfigId,omitempty"`
}

// AIProviderConfig is a single named AI provider configuration. Users can
// save any number of these and switch between them from the chat panel or
// settings page (similar to CC Switch). The Protocol field controls which
// HTTP API shape the backend uses: "openai" (default, /v1/chat/completions
// + Bearer) or "anthropic" (/v1/messages + x-api-key + anthropic-version).
type AIProviderConfig struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Provider     string  `json:"provider"`
	Protocol     string  `json:"protocol,omitempty"` // "openai" | "anthropic", default "openai"
	APIKey       string  `json:"apiKey"`
	BaseURL      string  `json:"baseUrl"`
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature,omitempty"`
	MaxTokens    int     `json:"maxTokens,omitempty"`
	SystemPrompt string  `json:"systemPrompt,omitempty"`
}

// CustomAccentTheme is a user-defined accent theme (Plan 48). The base Color
// is used to derive accent CSS tokens and register a Monaco theme. Optional
// token overrides take precedence over derived values.
type CustomAccentTheme struct {
	Name               string `json:"name"`
	Color              string `json:"color"`
	Primary            string `json:"primary,omitempty"`
	PrimaryHover       string `json:"primaryHover,omitempty"`
	PrimaryLight       string `json:"primaryLight,omitempty"`
	PrimaryContainer   string `json:"primaryContainer,omitempty"`
	OnPrimary          string `json:"onPrimary,omitempty"`
	OnPrimaryContainer string `json:"onPrimaryContainer,omitempty"`
}

// ShortcutKeys is a persisted key combination for a custom shortcut (N-8).
type ShortcutKeys struct {
	Key   string `json:"key"`
	Ctrl  bool   `json:"ctrl"`
	Shift bool   `json:"shift"`
	Alt   bool   `json:"alt"`
}

// SettingsService loads and saves settings as JSON in the config directory.
//
// Profile-aware (Plan 50): the configPath points at the active profile's
// settings.json. ProfileService.SetActiveProfile calls SetConfigPath to
// redirect this service to the new profile's settings file.
//
// N-76: pathMu protects configPath from concurrent access. Without it, a
// profile switch (SetConfigPath) racing with an in-flight SaveSettings
// could write the old profile's settings to the new profile's path,
// corrupting the new profile. All public methods that read or write
// configPath hold pathMu for the duration of the operation so the path
// cannot change mid-operation.
type SettingsService struct {
	configPath string
	pathMu     sync.RWMutex
}

// NewSettingsService creates a SettingsService using the XDG config path.
func NewSettingsService() *SettingsService {
	return &SettingsService{
		configPath: filepath.Join(xdg.ConfigHome, "gugacode", "settings.json"),
	}
}

// NewSettingsServiceWithPath creates a SettingsService that reads and
// writes settings at the given absolute path. Used by main.go when the
// ProfileService has determined the active profile's settings path.
func NewSettingsServiceWithPath(path string) *SettingsService {
	return &SettingsService{configPath: path}
}

// SetConfigPath redirects the service to read/write settings at the
// given path. Called by ProfileService (via the onSwitch callback) when
// the active profile changes. The next LoadSettings/SaveSettings call
// uses the new path.
//
// N-76: takes the write lock so it doesn't race with an in-flight
// Load/Save. If a Load/Save is in progress, SetConfigPath waits for it
// to finish before swapping the path, preventing cross-profile writes.
func (s *SettingsService) SetConfigPath(path string) {
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	s.configPath = path
}

// LoadSettings reads settings from disk, falling back to defaults if the file
// is missing or corrupt. The API key is decrypted in-memory. If the on-disk
// key is legacy plaintext (no encryption prefix), it is auto-migrated to
// encrypted form and re-saved (N-13).
//
// N-76: holds the read lock for the entire operation so a concurrent
// SetConfigPath cannot swap the path mid-load (which would read from the
// new profile's file using the old profile's expectations, or vice versa).
func (s *SettingsService) LoadSettings() (Settings, error) {
	s.pathMu.RLock()
	defer s.pathMu.RUnlock()
	settings := defaultSettings()
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		// N-109: the previous implementation silently returned defaults
		// with nil error, so a corrupt settings file invisibly reset the
		// user's preferences. We still return defaults (the app must be
		// able to launch with a corrupt file), but now log the parse
		// error so it's visible in the log file and on stderr — making
		// the silent reset diagnosable instead of invisible.
		slog.Warn("settings file is corrupt, falling back to defaults",
			"path", s.configPath, "err", err)
		return defaultSettings(), nil
	}
	// Decrypt the API key (handles legacy plaintext, dpapi:, aes:, plain:).
	rawKey := settings.AIApiKey
	decrypted, derr := DecryptSecret(rawKey)
	if derr != nil {
		// Decryption failed — clear the key to avoid exposing ciphertext
		// and return defaults for safety.
		settings.AIApiKey = ""
		return settings, nil
	}
	settings.AIApiKey = decrypted
	// Auto-migrate legacy plaintext to encrypted form (N-13). Best-effort:
	// errors are ignored so load still succeeds even if migration fails.
	// saveSettingsLocked is used (not SaveSettings) because we already hold
	// the read lock — Go's sync.RWMutex is NOT reentrant, so calling
	// SaveSettings (which tries to RLock again) would deadlock.
	if rawKey != "" && !IsSecretEncrypted(rawKey) {
		_ = s.saveSettingsLocked(settings)
	}
	return settings, nil
}

// SaveSettings writes settings to disk as pretty-printed JSON. The API key is
// encrypted before writing (N-13). If encryption fails, the key is saved with
// an explicit "plain:" prefix so settings can still be persisted.
//
// N-76: holds the read lock so a concurrent SetConfigPath cannot swap the
// path mid-save (which would write the old profile's data to the new
// profile's file).
func (s *SettingsService) SaveSettings(settings Settings) error {
	s.pathMu.RLock()
	defer s.pathMu.RUnlock()
	return s.saveSettingsLocked(settings)
}

// saveSettingsLocked encrypts the API key and writes to disk. Caller MUST
// hold s.pathMu (read or write). Used internally by LoadSettings (which
// already holds the lock) and SaveSettings.
func (s *SettingsService) saveSettingsLocked(settings Settings) error {
	// Make a shallow copy so we don't mutate the caller's struct.
	copy := settings
	encrypted, err := EncryptSecret(copy.AIApiKey)
	if err != nil {
		// Encryption failed — fall back to explicit plaintext marker so
		// settings can still be saved. The marker makes it clear the key
		// is not encrypted at rest.
		if copy.AIApiKey != "" {
			copy.AIApiKey = secretPrefixPlain + copy.AIApiKey
		}
	} else {
		copy.AIApiKey = encrypted
	}
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(copy, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath, data, 0644)
}

// saveSettingsInternal is kept for backward compatibility with callers that
// don't hold the lock. It acquires the read lock and delegates to
// saveSettingsLocked. Prefer SaveSettings for new code.
func (s *SettingsService) saveSettingsInternal(settings Settings) error {
	s.pathMu.RLock()
	defer s.pathMu.RUnlock()
	return s.saveSettingsLocked(settings)
}

// IsAPIKeyEncryptedOnDisk reads the raw settings file and returns true if the
// stored API key carries an encryption prefix ("dpapi:" or "aes:"). Returns
// false if the key is plaintext, empty, or the file is missing/corrupt.
//
// N-76: holds the read lock so configPath cannot change mid-read.
func (s *SettingsService) IsAPIKeyEncryptedOnDisk() bool {
	s.pathMu.RLock()
	defer s.pathMu.RUnlock()
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return false
	}
	var raw struct {
		AIApiKey string `json:"aiApiKey"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	return IsSecretEncrypted(raw.AIApiKey)
}

// GetAPIKeyStorageMethod returns a human-readable label for how the API key is
// stored on disk: "dpapi", "aes", "plain", or "none" (when empty or missing).
//
// N-76: holds the read lock so configPath cannot change mid-read.
func (s *SettingsService) GetAPIKeyStorageMethod() string {
	s.pathMu.RLock()
	defer s.pathMu.RUnlock()
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return "none"
	}
	var raw struct {
		AIApiKey string `json:"aiApiKey"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "none"
	}
	return SecretMethod(raw.AIApiKey)
}

// ListSecrets returns information about secrets stored in the platform
// keyring (N-49). On Windows this returns an empty list (DPAPI blobs live in
// settings.json). On macOS/Linux it queries the Keychain / libsecret for
// gugacode entries, allowing the settings UI to show users what's stored
// and help them clean up orphan entries.
func (s *SettingsService) ListSecrets() ([]SecretInfo, error) {
	return ListSecrets()
}

// DeleteSecret removes the secret with the given account from the platform
// keyring (N-49). On Windows this is a no-op. On macOS/Linux it deletes the
// Keychain / libsecret entry. Idempotent — returns nil if the entry doesn't
// exist. Used by the settings UI's "clear keyring" action.
func (s *SettingsService) DeleteSecret(account string) error {
	return DeleteSecret(account)
}

func defaultSettings() Settings {
	return Settings{
		Language:    "en",
		Theme:       "dark",
		FontSize:    14,
		FontFamily:  "JetBrains Mono",
		TabSize:     2,
		WordWrap:    true,
		LineNumbers: true,
		Minimap:     false,
		AIApiKey:       "",
		AIBaseURL:      "https://api.openai.com",
		AIModel:        "gpt-4o",
		AISystemPrompt: "",
		CursorBlinking:      "blink",
		CursorStyle:         "line",
		BracketColorization: true,
		AutoSave:            false,
		AutoSaveDelay:       "afterDelay",
		AIProvider:          "",
		Temperature:         0.7,
		MaxTokens:           4096,
		DefaultShell:        "",
		TerminalFontSize:    13,
		TerminalCursorStyle: "block",
		Scrollback:          10000,
		UIDensity:               "comfortable",
		FontSizeScaling:         100,
		InlineCompletionEnabled: true,
		AiChatPosition:          "right",
		ActivityBarVisible:      true,
		// N-29: sandbox enabled by default (v2 behavior).
		EnablePluginSandbox: true,
	}
}
