package services

import (
	"strings"
)

// Secret method prefixes identify how a stored secret was encrypted.
//   - "dpapi:"    — Windows DPAPI (CryptProtectData), machine-bound
//   - "aes:"      — AES-256-GCM with a per-install key file (non-Windows fallback)
//   - "keyring:"  — macOS Keychain / Linux libsecret via CLI wrapper (N-15)
//   - "plain:"    — explicit plaintext marker (only used when encryption is unavailable)
//
// Bare strings (no prefix) are treated as legacy plaintext for backward
// compatibility with settings.json files written before N-13.
const (
	secretPrefixDPAPI   = "dpapi:"
	secretPrefixAES     = "aes:"
	secretPrefixKeyring = "keyring:"
	secretPrefixPlain   = "plain:"
)

// EncryptSecret encrypts a plaintext secret using the platform's preferred
// method. On Windows it uses DPAPI; on macOS/Linux it tries the native
// keychain (Keychain / libsecret) and falls back to AES-256-GCM with a
// per-install key file when the keyring CLI is unavailable. An empty input
// returns an empty string (no prefix), so empty fields stay empty.
func EncryptSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	return platformEncryptSecret(plaintext)
}

// DecryptSecret decrypts a value produced by EncryptSecret. It handles:
//   - "dpapi:", "aes:", and "keyring:" prefixed values (decrypted via the
//     matching platform path)
//   - "plain:" prefixed values (stripped)
//   - bare strings (returned as-is, for backward compatibility)
//   - empty string (returned as-is)
func DecryptSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if strings.HasPrefix(stored, secretPrefixPlain) {
		return strings.TrimPrefix(stored, secretPrefixPlain), nil
	}
	if strings.HasPrefix(stored, secretPrefixDPAPI) ||
		strings.HasPrefix(stored, secretPrefixAES) ||
		strings.HasPrefix(stored, secretPrefixKeyring) {
		return platformDecryptSecret(stored)
	}
	// Legacy plaintext — return as-is so existing settings keep working.
	return stored, nil
}

// IsSecretEncrypted returns true when the stored value carries an encryption
// prefix ("dpapi:", "aes:", or "keyring:"). Bare strings and "plain:" return
// false.
func IsSecretEncrypted(stored string) bool {
	return strings.HasPrefix(stored, secretPrefixDPAPI) ||
		strings.HasPrefix(stored, secretPrefixAES) ||
		strings.HasPrefix(stored, secretPrefixKeyring)
}

// SecretMethod returns a human-readable label for the encryption method used
// by the stored value: "dpapi", "aes", "keyring", "plain", or "none" (for
// empty/legacy).
func SecretMethod(stored string) string {
	switch {
	case stored == "":
		return "none"
	case strings.HasPrefix(stored, secretPrefixDPAPI):
		return "dpapi"
	case strings.HasPrefix(stored, secretPrefixAES):
		return "aes"
	case strings.HasPrefix(stored, secretPrefixKeyring):
		return "keyring"
	case strings.HasPrefix(stored, secretPrefixPlain):
		return "plain"
	default:
		return "plain"
	}
}

// SecretInfo describes a secret entry discovered in the platform keyring
// (macOS Keychain / Linux libsecret). Used by ListSecrets so the settings UI
// can show users what's stored and let them delete orphans (N-49).
type SecretInfo struct {
	Account string `json:"account"` // keyring account/label
	Method  string `json:"method"`  // "dpapi", "aes", "keyring", "plain", "none"
	Stored  bool   `json:"stored"`  // whether an entry exists in the keyring
}

// ListSecrets returns information about secrets stored in the platform
// keyring (macOS Keychain / Linux libsecret). On Windows, where DPAPI blobs
// live inside settings.json rather than a separate keyring, it returns an
// empty list. This is used by the settings UI to show users what's in their
// keyring so they can clean up orphan entries left behind when AIApiKey was
// cleared (N-49).
func ListSecrets() ([]SecretInfo, error) {
	return platformListSecrets()
}

// DeleteSecret removes the secret with the given account from the platform
// keyring. On Windows this is a no-op (DPAPI secrets are in settings.json).
// Returns nil if the entry didn't exist (idempotent). This lets users clean
// up orphan keyring entries that remain after clearing AIApiKey in
// settings.json (N-49).
func DeleteSecret(account string) error {
	return platformDeleteSecret(account)
}
