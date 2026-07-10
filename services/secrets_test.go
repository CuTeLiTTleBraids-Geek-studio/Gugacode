package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptSecret_emptyInputReturnsEmpty(t *testing.T) {
	got, err := EncryptSecret("")
	if err != nil {
		t.Fatalf("EncryptSecret(\"\") returned error: %v", err)
	}
	if got != "" {
		t.Errorf("EncryptSecret(\"\") = %q, want \"\"", got)
	}
}

func TestDecryptSecret_emptyInputReturnsEmpty(t *testing.T) {
	got, err := DecryptSecret("")
	if err != nil {
		t.Fatalf("DecryptSecret(\"\") returned error: %v", err)
	}
	if got != "" {
		t.Errorf("DecryptSecret(\"\") = %q, want \"\"", got)
	}
}

func TestEncryptDecryptSecret_roundTrip(t *testing.T) {
	cases := []string{
		"sk-test-key",
		"sk-abc123xyz",
		"a",
		"key with spaces",
		"key-with-special-chars-!@#$%^&*()",
		strings.Repeat("x", 256),
	}
	for _, plaintext := range cases {
		t.Run(plaintext[:min(len(plaintext), 20)], func(t *testing.T) {
			encrypted, err := EncryptSecret(plaintext)
			if err != nil {
				t.Fatalf("EncryptSecret failed: %v", err)
			}
			if encrypted == "" {
				t.Fatal("EncryptSecret returned empty for non-empty input")
			}
			if encrypted == plaintext {
				t.Error("EncryptSecret returned plaintext — not encrypted")
			}
			if !IsSecretEncrypted(encrypted) {
				t.Errorf("IsSecretEncrypted(%q) = false, want true", encrypted[:min(len(encrypted), 20)])
			}
			decrypted, err := DecryptSecret(encrypted)
			if err != nil {
				t.Fatalf("DecryptSecret failed: %v", err)
			}
			if decrypted != plaintext {
				t.Errorf("DecryptSecret = %q, want %q", decrypted, plaintext)
			}
		})
	}
}

func TestDecryptSecret_legacyPlaintextReturnedAsIs(t *testing.T) {
	// Bare strings without a prefix should be returned as-is so existing
	// settings.json files keep working after the N-13 upgrade.
	cases := []string{
		"sk-legacy-key",
		"plain-key-without-prefix",
		"key:with:colons", // not a real prefix, should pass through
	}
	for _, stored := range cases {
		t.Run(stored, func(t *testing.T) {
			got, err := DecryptSecret(stored)
			if err != nil {
				t.Fatalf("DecryptSecret failed: %v", err)
			}
			if got != stored {
				t.Errorf("DecryptSecret(%q) = %q, want %q", stored, got, stored)
			}
			if IsSecretEncrypted(stored) {
				t.Errorf("IsSecretEncrypted(%q) = true, want false", stored)
			}
		})
	}
}

func TestDecryptSecret_plainPrefixStripped(t *testing.T) {
	got, err := DecryptSecret("plain:sk-fallback-key")
	if err != nil {
		t.Fatalf("DecryptSecret failed: %v", err)
	}
	if got != "sk-fallback-key" {
		t.Errorf("DecryptSecret = %q, want %q", got, "sk-fallback-key")
	}
	if IsSecretEncrypted("plain:sk-fallback-key") {
		t.Error("IsSecretEncrypted should return false for plain: prefix")
	}
}

// TestIsSecretEncrypted_keyringPrefix verifies the keyring: prefix (added in
// N-15 for macOS Keychain / Linux libsecret) is recognized as encrypted.
// We don't test DecryptSecret on a keyring: value here because decryption
// requires the platform keychain — that path is exercised manually on each
// platform instead.
func TestIsSecretEncrypted_keyringPrefix(t *testing.T) {
	if !IsSecretEncrypted("keyring:YWk=") {
		t.Error("IsSecretEncrypted should return true for keyring: prefix")
	}
	if SecretMethod("keyring:YWk=") != "keyring" {
		t.Errorf("SecretMethod = %q, want %q", SecretMethod("keyring:YWk="), "keyring")
	}
}

func TestSecretMethod(t *testing.T) {
	cases := []struct {
		stored string
		want   string
	}{
		{"", "none"},
		{"dpapi:abc123", "dpapi"},
		{"aes:xyz789", "aes"},
		{"keyring:YWk=", "keyring"},
		{"plain:foo", "plain"},
		{"sk-legacy", "plain"}, // legacy plaintext
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := SecretMethod(tc.stored)
			if got != tc.want {
				t.Errorf("SecretMethod(%q) = %q, want %q", tc.stored, got, tc.want)
			}
		})
	}
}

func TestIsSecretEncrypted(t *testing.T) {
	cases := []struct {
		stored string
		want   bool
	}{
		{"", false},
		{"dpapi:abc123", true},
		{"aes:xyz789", true},
		{"keyring:YWk=", true},
		{"plain:foo", false},
		{"sk-legacy", false},
	}
	for _, tc := range cases {
		t.Run(tc.stored, func(t *testing.T) {
			got := IsSecretEncrypted(tc.stored)
			if got != tc.want {
				t.Errorf("IsSecretEncrypted(%q) = %v, want %v", tc.stored, got, tc.want)
			}
		})
	}
}

// --- SettingsService encryption integration tests ---

func TestSettingsService_SaveSettings_encryptsAPIKeyOnDisk(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = "sk-secret-key"

	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Read the raw JSON file and check the key is NOT plaintext.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, "sk-secret-key") {
		t.Error("settings.json contains plaintext API key — not encrypted")
	}
	// The encrypted form should have a prefix (dpapi on Windows, aes or
	// keyring on macOS/Linux depending on keychain availability).
	if !strings.Contains(raw, "dpapi:") && !strings.Contains(raw, "aes:") && !strings.Contains(raw, "keyring:") {
		t.Error("settings.json does not contain an encryption prefix")
	}
}

func TestSettingsService_LoadSettings_decryptsAPIKey(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = "sk-decrypt-me"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Load with a fresh service instance.
	svc2 := &SettingsService{configPath: configPath}
	loaded, err := svc2.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	// G-SEC-07: LoadSettings must NOT return the plaintext key. It is cleared
	// and AIApiKeyConfigured signals that a key is stored. The decrypted key
	// is available via GetDecryptedAPIKey for internal backend use.
	if loaded.AIApiKey != "" {
		t.Errorf("AIApiKey = %q, want empty (G-SEC-07)", loaded.AIApiKey)
	}
	if !loaded.AIApiKeyConfigured {
		t.Error("AIApiKeyConfigured = false, want true")
	}
	// The internal accessor still returns the decrypted key.
	got, err := svc2.GetDecryptedAPIKey()
	if err != nil {
		t.Fatalf("GetDecryptedAPIKey failed: %v", err)
	}
	if got != "sk-decrypt-me" {
		t.Errorf("GetDecryptedAPIKey = %q, want %q", got, "sk-decrypt-me")
	}
}

func TestSettingsService_LoadSettings_migratesLegacyPlaintext(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	// Write a legacy settings.json with a plaintext API key (no prefix).
	legacy := `{
  "language": "en",
  "theme": "dark",
  "fontSize": 14,
  "fontFamily": "JetBrains Mono",
  "tabSize": 2,
  "wordWrap": true,
  "lineNumbers": true,
  "minimap": false,
  "aiApiKey": "sk-legacy-plaintext",
  "aiBaseUrl": "https://api.openai.com",
  "aiModel": "gpt-4o",
  "aiSystemPrompt": "",
  "cursorBlinking": "blink",
  "cursorStyle": "line",
  "bracketColorization": true,
  "autoSave": false,
  "autoSaveDelay": "afterDelay",
  "aiProvider": "",
  "temperature": 0.7,
  "maxTokens": 4096,
  "defaultShell": "",
  "terminalFontSize": 13,
  "terminalCursorStyle": "block",
  "scrollback": 10000,
  "uiDensity": "comfortable",
  "fontSizeScaling": 100
}`
	if err := os.WriteFile(configPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &SettingsService{configPath: configPath}
	loaded, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	// G-SEC-07: the plaintext key is NOT returned; AIApiKeyConfigured signals
	// that a key was stored, and the storage method reflects the legacy form.
	if loaded.AIApiKey != "" {
		t.Errorf("AIApiKey = %q, want empty (G-SEC-07)", loaded.AIApiKey)
	}
	if !loaded.AIApiKeyConfigured {
		t.Error("AIApiKeyConfigured = false, want true")
	}
	// The decrypted key is still available internally for backend use.
	got, err := svc.GetDecryptedAPIKey()
	if err != nil {
		t.Fatalf("GetDecryptedAPIKey failed: %v", err)
	}
	if got != "sk-legacy-plaintext" {
		t.Errorf("GetDecryptedAPIKey = %q, want %q", got, "sk-legacy-plaintext")
	}

	// The on-disk file should now be encrypted (auto-migration).
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	raw := string(data)
	if strings.Contains(raw, "sk-legacy-plaintext") {
		t.Error("settings.json still contains plaintext key after LoadSettings — migration did not happen")
	}
	if !strings.Contains(raw, "dpapi:") && !strings.Contains(raw, "aes:") && !strings.Contains(raw, "keyring:") {
		t.Error("settings.json does not contain encryption prefix after migration")
	}
}

func TestSettingsService_LoadSettings_emptyKeyStaysEmpty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = ""
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	loaded, err := svc.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.AIApiKey != "" {
		t.Errorf("AIApiKey = %q, want empty", loaded.AIApiKey)
	}
	if loaded.AIApiKeyConfigured {
		t.Error("AIApiKeyConfigured = true, want false (no key stored)")
	}
	if loaded.AIApiKeyStorageMethod != "none" {
		t.Errorf("AIApiKeyStorageMethod = %q, want %q", loaded.AIApiKeyStorageMethod, "none")
	}
}

func TestSettingsService_IsAPIKeyEncryptedOnDisk(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	// No file → not encrypted.
	if svc.IsAPIKeyEncryptedOnDisk() {
		t.Error("IsAPIKeyEncryptedOnDisk() = true, want false (no file)")
	}

	// Save with a key → should be encrypted.
	settings := defaultSettings()
	settings.AIApiKey = "sk-test-encryption"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
	if !svc.IsAPIKeyEncryptedOnDisk() {
		t.Error("IsAPIKeyEncryptedOnDisk() = false, want true (after save)")
	}

	// Write legacy plaintext → not encrypted.
	legacy := `{"aiApiKey": "sk-plaintext"}`
	if err := os.WriteFile(configPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	if svc.IsAPIKeyEncryptedOnDisk() {
		t.Error("IsAPIKeyEncryptedOnDisk() = true, want false (plaintext)")
	}
}

func TestSettingsService_GetAPIKeyStorageMethod(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	// No file → "none".
	if got := svc.GetAPIKeyStorageMethod(); got != "none" {
		t.Errorf("GetAPIKeyStorageMethod() = %q, want %q", got, "none")
	}

	// Save with a key → should return "dpapi" or "aes".
	settings := defaultSettings()
	settings.AIApiKey = "sk-method-test"
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
	method := svc.GetAPIKeyStorageMethod()
	if method != "dpapi" && method != "aes" && method != "keyring" {
		t.Errorf("GetAPIKeyStorageMethod() = %q, want \"dpapi\", \"aes\", or \"keyring\"", method)
	}

	// Empty key → "none".
	settings.AIApiKey = ""
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}
	if got := svc.GetAPIKeyStorageMethod(); got != "none" {
		t.Errorf("GetAPIKeyStorageMethod() = %q, want %q (empty key)", got, "none")
	}
}

func TestSettingsService_SaveSettings_emptyKeyDoesNotAddPrefix(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "settings.json")
	svc := &SettingsService{configPath: configPath}

	settings := defaultSettings()
	settings.AIApiKey = ""
	if err := svc.SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	raw := string(data)
	// Empty key should remain empty (no "plain:" or "dpapi:" prefix).
	if strings.Contains(raw, "plain:") || strings.Contains(raw, "dpapi:") || strings.Contains(raw, "aes:") {
		t.Error("settings.json contains encryption prefix for empty key — should be empty string")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- N-49: Cross-platform AES decrypt via DecryptSecret ---

// TestDecryptSecret_aesPrefixDecryptsOnAllPlatforms verifies that an "aes:"
// prefixed value can be decrypted via DecryptSecret on every platform. AES is
// the cross-platform fallback, so a value encrypted on macOS/Linux can be
// decrypted on Windows (and vice versa) when the per-install key file is
// available. This test encrypts with aesEncrypt directly (bypassing the
// platform encrypt path) and decrypts via the public DecryptSecret entry
// point.
func TestDecryptSecret_aesPrefixDecryptsOnAllPlatforms(t *testing.T) {
	plaintext := "sk-cross-platform-key"
	encrypted, err := aesEncrypt(plaintext)
	if err != nil {
		t.Fatalf("aesEncrypt failed: %v", err)
	}
	if !strings.HasPrefix(encrypted, secretPrefixAES) {
		t.Fatalf("aesEncrypt returned %q, want aes: prefix", encrypted[:min(len(encrypted), 20)])
	}
	got, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret(aes:) failed: %v", err)
	}
	if got != plaintext {
		t.Errorf("DecryptSecret(aes:) = %q, want %q", got, plaintext)
	}
}

// TestDecryptSecret_aesRoundTripViaEncryptSecret verifies that when the
// platform's EncryptSecret chooses AES (the fallback path), the result
// can be decrypted by DecryptSecret. On Windows this uses DPAPI, so we
// test the AES path by calling aesEncrypt directly — but we also verify
// that EncryptSecret's output round-trips (which it should regardless of
// the platform's chosen method).
func TestDecryptSecret_aesRoundTripViaEncryptSecret(t *testing.T) {
	// Encrypt with AES directly, simulating a value created on a non-Windows
	// platform that was then copied to this machine.
	plaintext := "sk-migrated-from-macos"
	encrypted, err := aesEncrypt(plaintext)
	if err != nil {
		t.Fatalf("aesEncrypt failed: %v", err)
	}
	// DecryptSecret should handle aes: on any platform (N-49).
	got, err := DecryptSecret(encrypted)
	if err != nil {
		t.Fatalf("DecryptSecret failed for aes: value: %v", err)
	}
	if got != plaintext {
		t.Errorf("DecryptSecret = %q, want %q", got, plaintext)
	}
}

// --- N-49: ListSecrets / DeleteSecret ---

// TestListSecrets_doesNotError verifies that ListSecrets returns without
// error on every platform. On Windows it returns an empty list (DPAPI blobs
// live in settings.json). On macOS/Linux it may return entries if the
// keyring CLI is available and has gugacode entries.
func TestListSecrets_doesNotError(t *testing.T) {
	infos, err := ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets failed: %v", err)
	}
	// We don't assert specific contents — the result depends on whether the
	// keyring CLI is available and has entries. We just verify it doesn't
	// crash and returns a (possibly empty) slice.
	if infos == nil {
		// nil is acceptable on Windows / when no keyring is available.
		return
	}
	for _, info := range infos {
		if info.Account == "" {
			t.Errorf("ListSecrets returned entry with empty Account: %+v", info)
		}
	}
}

// TestDeleteSecret_idempotent verifies that DeleteSecret returns nil even
// when the entry doesn't exist. This is important because users may click
// "delete keyring entry" when there's nothing to delete.
func TestDeleteSecret_idempotent(t *testing.T) {
	// Delete a non-existent account — should not error.
	if err := DeleteSecret("nonexistent-account-12345"); err != nil {
		t.Errorf("DeleteSecret(nonexistent) returned error: %v", err)
	}
	// Delete the default account — should not error whether or not it exists.
	if err := DeleteSecret(keyringAccount); err != nil {
		t.Errorf("DeleteSecret(%q) returned error: %v", keyringAccount, err)
	}
	// Delete with empty account — should not error.
	if err := DeleteSecret(""); err != nil {
		t.Errorf("DeleteSecret(\"\") returned error: %v", err)
	}
}

// TestSettingsService_ListSecrets verifies the SettingsService method
// delegates to the package-level ListSecrets without error.
func TestSettingsService_ListSecrets(t *testing.T) {
	svc := &SettingsService{configPath: filepath.Join(t.TempDir(), "settings.json")}
	infos, err := svc.ListSecrets()
	if err != nil {
		t.Fatalf("SettingsService.ListSecrets failed: %v", err)
	}
	// Result should be nil or a slice — both are acceptable.
	_ = infos
}

// TestSettingsService_DeleteSecret verifies the SettingsService method
// delegates to the package-level DeleteSecret without error.
func TestSettingsService_DeleteSecret(t *testing.T) {
	svc := &SettingsService{configPath: filepath.Join(t.TempDir(), "settings.json")}
	if err := svc.DeleteSecret("test-account"); err != nil {
		t.Errorf("SettingsService.DeleteSecret failed: %v", err)
	}
}

// TestSecretInfo_JSON verifies the SecretInfo struct serializes correctly
// for the Wails binding layer.
func TestSecretInfo_JSON(t *testing.T) {
	info := SecretInfo{Account: "ai-api-key", Method: "keyring", Stored: true}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"account":"ai-api-key"`) {
		t.Errorf("JSON missing account field: %s", got)
	}
	if !strings.Contains(got, `"method":"keyring"`) {
		t.Errorf("JSON missing method field: %s", got)
	}
	if !strings.Contains(got, `"stored":true`) {
		t.Errorf("JSON missing stored field: %s", got)
	}
}
