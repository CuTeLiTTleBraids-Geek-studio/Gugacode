//go:build linux

package services

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// libsecretAvailable reports whether the `secret-tool` CLI is on PATH.
// `secret-tool` is part of the libsecret-tools package on Debian/Ubuntu and
// libsecret on Fedora. It speaks the Secret Service API over D-Bus, so any
// compatible backend (GNOME Keyring, KWallet via the ssue proxy, KeePassXC)
// will work.
func libsecretAvailable() bool {
	_, err := exec.LookPath("secret-tool")
	return err == nil
}

// libsecretStore stores the plaintext in the user's secret service via
// `secret-tool store`. The secret is read from stdin; the attributes
// (gugacode:service, gugacode:account) are used to look it up later.
// Returns the prefixed marker ("keyring:" + base64-label) on success.
func libsecretStore(plaintext string) (string, error) {
	cmd := exec.Command("secret-tool", "store",
		"--label", keyringServiceName+"/"+keyringAccount,
		"gugacode:service", keyringServiceName,
		"gugacode:account", keyringAccount,
	)
	cmd.Stdin = strings.NewReader(plaintext)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("libsecret: store failed: %w", err)
	}
	marker := base64.StdEncoding.EncodeToString([]byte(keyringAccount))
	return secretPrefixKeyring + marker, nil
}

// libsecretLoad retrieves the plaintext from the user's secret service via
// `secret-tool lookup`. The secret is written to stdout.
func libsecretLoad(markerB64 string) (string, error) {
	cmd := exec.Command("secret-tool", "lookup",
		"gugacode:service", keyringServiceName,
		"gugacode:account", keyringAccount,
	)
	var stdout strings.Builder
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("libsecret: lookup failed: %w", err)
	}
	out := strings.TrimRight(stdout.String(), "\n")
	if out == "" {
		return "", fmt.Errorf("libsecret: no secret found for service=%q account=%q",
			keyringServiceName, keyringAccount)
	}
	return out, nil
}

// platformEncryptSecret tries libsecret first, falling back to the per-install
// AES key file when `secret-tool` is unavailable or the secret service is not
// running (common in headless / CI environments). This keeps secrets usable
// across desktop and server Linux.
func platformEncryptSecret(plaintext string) (string, error) {
	if libsecretAvailable() {
		stored, err := libsecretStore(plaintext)
		if err == nil {
			return stored, nil
		}
		// Fall through to AES on error.
	}
	return aesEncrypt(plaintext)
}

// platformDecryptSecret dispatches based on the prefix. "keyring:" values
// are loaded from libsecret (with a CLI availability check — N-49); "aes:"
// values use the cross-platform AES fallback; foreign "dpapi:" values
// (created on Windows) return an error since Linux cannot invoke DPAPI.
func platformDecryptSecret(stored string) (string, error) {
	if strings.HasPrefix(stored, secretPrefixKeyring) {
		// N-49: Check CLI availability before calling libsecret. In headless
		// / CI environments `secret-tool` may be missing or the secret
		// service not running. Returning an error lets the caller prompt
		// the user to start the service or re-enter the API key.
		if !libsecretAvailable() {
			return "", fmt.Errorf("keyring: `secret-tool` CLI is unavailable or the secret service is not running; please start the secret service or re-enter the API key")
		}
		markerB64 := strings.TrimPrefix(stored, secretPrefixKeyring)
		return libsecretLoad(markerB64)
	}
	if strings.HasPrefix(stored, secretPrefixAES) {
		return aesDecrypt(stored)
	}
	if strings.HasPrefix(stored, secretPrefixDPAPI) {
		// N-49: Foreign DPAPI value — Windows-only, cannot decrypt on Linux.
		return "", fmt.Errorf("dpapi: secret was stored on Windows and cannot be accessed on Linux; please re-enter the API key")
	}
	// Unrecognized — return as-is (legacy plaintext).
	return stored, nil
}

// platformListSecrets checks libsecret for gugacode entries. Returns at
// most one entry (the fixed-account AI API key) when `secret-tool` is
// available and the entry exists. Used by the settings UI to show users
// what's in their secret service so they can clean up orphans (N-49).
func platformListSecrets() ([]SecretInfo, error) {
	if !libsecretAvailable() {
		return nil, nil
	}
	// `secret-tool lookup` exits 0 and prints the secret when found.
	err := exec.Command("secret-tool", "lookup",
		"gugacode:service", keyringServiceName,
		"gugacode:account", keyringAccount,
	).Run()
	if err == nil {
		return []SecretInfo{{Account: keyringAccount, Method: "keyring", Stored: true}}, nil
	}
	return nil, nil
}

// platformDeleteSecret removes the secret with the given account from the
// secret service via `secret-tool clear`. Idempotent — returns nil even if
// the entry doesn't exist.
func platformDeleteSecret(account string) error {
	if !libsecretAvailable() {
		return nil
	}
	// `secret-tool clear` returns non-zero when no entry matches; ignore
	// that error so the call is idempotent.
	_ = exec.Command("secret-tool", "clear",
		"gugacode:service", keyringServiceName,
		"gugacode:account", account,
	).Run()
	return nil
}
