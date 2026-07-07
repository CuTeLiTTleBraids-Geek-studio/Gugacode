//go:build darwin

package services

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// macosKeychainAvailable reports whether the `security` CLI is on PATH.
// Used to decide whether to try the Keychain path or fall straight through
// to the AES fallback.
func macosKeychainAvailable() bool {
	_, err := exec.LookPath("security")
	return err == nil
}

// keychainStore stores the plaintext in the macOS Keychain via the `security`
// CLI. Returns the prefixed marker ("keyring:" + base64-label) on success.
// The actual plaintext is NOT embedded in the returned value — only a marker
// indicating "look this up in the Keychain" is stored. This keeps the value
// in settings.json opaque even if the file is leaked.
func keychainStore(plaintext string) (string, error) {
	// `security add-generic-password` refuses to overwrite an existing entry
	// by default. Delete any existing entry first (ignore "not found" errors).
	_ = exec.Command("security", "delete-generic-password",
		"-s", keyringServiceName,
		"-a", keyringAccount,
	).Run()
	// Store the password via stdin (-w with no argument reads from stdin).
	cmd := exec.Command("security", "add-generic-password",
		"-s", keyringServiceName,
		"-a", keyringAccount,
		"-w", // read password from stdin
		"-U", // update if exists (redundant with the delete above, but safe)
	)
	cmd.Stdin = strings.NewReader(plaintext)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("keychain: add-generic-password failed: %w", err)
	}
	// Store a marker rather than the secret itself. The marker is just the
	// account name base64-encoded so it's a stable opaque token.
	marker := base64.StdEncoding.EncodeToString([]byte(keyringAccount))
	return secretPrefixKeyring + marker, nil
}

// keychainLoad retrieves the plaintext from the macOS Keychain via the
// `security find-generic-password -g` CLI. The -g flag writes the password
// to stderr (with a "password: " prefix) and the metadata to stdout.
func keychainLoad(markerB64 string) (string, error) {
	// We don't decode the marker — it's just the account name, which is
	// fixed for v1. In the future, the marker could encode a different
	// account name for multi-secret support.
	cmd := exec.Command("security", "find-generic-password",
		"-s", keyringServiceName,
		"-a", keyringAccount,
		"-g", // print the password to stderr
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("keychain: find-generic-password failed: %w", err)
	}
	// Output format: "password: <the-password>\n"
	out := strings.TrimSpace(stderr.String())
	const prefix = "password: "
	if !strings.HasPrefix(out, prefix) {
		return "", fmt.Errorf("keychain: unexpected output format %q", out)
	}
	return strings.TrimPrefix(out, prefix), nil
}

// platformEncryptSecret tries the macOS Keychain first, falling back to the
// per-install AES key file when the `security` CLI is unavailable or the
// Keychain rejects the store (e.g. user denies access). This keeps secrets
// usable in headless / CI environments where the Keychain is locked.
func platformEncryptSecret(plaintext string) (string, error) {
	if macosKeychainAvailable() {
		stored, err := keychainStore(plaintext)
		if err == nil {
			return stored, nil
		}
		// Fall through to AES on error.
	}
	return aesEncrypt(plaintext)
}

// platformDecryptSecret dispatches based on the prefix. "keyring:" values
// are loaded from the Keychain (with a CLI availability check — N-49);
// "aes:" values use the cross-platform AES fallback; foreign "dpapi:" values
// (created on Windows) return an error since macOS cannot invoke DPAPI.
func platformDecryptSecret(stored string) (string, error) {
	if strings.HasPrefix(stored, secretPrefixKeyring) {
		// N-49: Check CLI availability before calling the Keychain. In
		// headless / CI environments the `security` CLI may be missing or
		// the Keychain locked. Returning an error lets the caller prompt
		// the user to unlock the Keychain or re-enter the API key, instead
		// of failing with an opaque exec error.
		if !macosKeychainAvailable() {
			return "", fmt.Errorf("keyring: macOS `security` CLI is unavailable or the Keychain is locked; please unlock the Keychain or re-enter the API key")
		}
		markerB64 := strings.TrimPrefix(stored, secretPrefixKeyring)
		return keychainLoad(markerB64)
	}
	if strings.HasPrefix(stored, secretPrefixAES) {
		return aesDecrypt(stored)
	}
	if strings.HasPrefix(stored, secretPrefixDPAPI) {
		// N-49: Foreign DPAPI value — Windows-only, cannot decrypt on macOS.
		return "", fmt.Errorf("dpapi: secret was stored on Windows and cannot be accessed on macOS; please re-enter the API key")
	}
	// Unrecognized — return as-is (legacy plaintext).
	return stored, nil
}

// platformListSecrets checks the macOS Keychain for gugacode entries.
// Returns at most one entry (the fixed-account AI API key) when the
// `security` CLI is available and the entry exists. Used by the settings UI
// to show users what's in their Keychain so they can clean up orphans (N-49).
func platformListSecrets() ([]SecretInfo, error) {
	if !macosKeychainAvailable() {
		return nil, nil
	}
	// `security find-generic-password` exits 0 when the entry exists.
	err := exec.Command("security", "find-generic-password",
		"-s", keyringServiceName,
		"-a", keyringAccount,
	).Run()
	if err == nil {
		return []SecretInfo{{Account: keyringAccount, Method: "keyring", Stored: true}}, nil
	}
	return nil, nil
}

// platformDeleteSecret removes the secret with the given account from the
// macOS Keychain. Idempotent — returns nil if the entry doesn't exist.
func platformDeleteSecret(account string) error {
	if !macosKeychainAvailable() {
		return nil
	}
	// `security delete-generic-password` returns non-zero when the entry is
	// not found; ignore that error so the call is idempotent.
	_ = exec.Command("security", "delete-generic-password",
		"-s", keyringServiceName,
		"-a", account,
	).Run()
	return nil
}
