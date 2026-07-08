//go:build windows

package services

import (
	"encoding/base64"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	crypt32DLL          = syscall.NewLazyDLL("crypt32.dll")
	kernel32DLL         = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtect    = crypt32DLL.NewProc("CryptProtectData")
	procCryptUnprotect  = crypt32DLL.NewProc("CryptUnprotectData")
	procLocalFree       = kernel32DLL.NewProc("LocalFree")
)

// dataBlob maps to the Windows DATA_BLOB struct used by DPAPI.
type dataBlob struct {
	cbData uint32
	pbData *byte
}

// platformEncryptSecret encrypts plaintext using Windows DPAPI
// (CryptProtectData). The result is base64-encoded and prefixed with "dpapi:".
// DPAPI encryption is machine-bound: the ciphertext can only be decrypted on
// the same Windows user account on the same machine.
func platformEncryptSecret(plaintext string) (string, error) {
	bytes := []byte(plaintext)
	if len(bytes) == 0 {
		return "", nil
	}
	blobIn := dataBlob{
		cbData: uint32(len(bytes)),
		pbData: &bytes[0],
	}
	var blobOut dataBlob
	r, _, err := procCryptProtect.Call(
		uintptr(unsafe.Pointer(&blobIn)),
		0, // description (optional)
		0, // reserved
		0, // prompt
		0, // prompt struct
		0, // flags
		uintptr(unsafe.Pointer(&blobOut)),
	)
	if r == 0 {
		return "", fmt.Errorf("dpapi: CryptProtectData failed: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(blobOut.pbData)))

	out := make([]byte, blobOut.cbData)
	copy(out, (*[1 << 30]byte)(unsafe.Pointer(blobOut.pbData))[:blobOut.cbData])
	return secretPrefixDPAPI + base64.StdEncoding.EncodeToString(out), nil
}

// platformDecryptSecret decrypts a prefixed value using Windows DPAPI
// (CryptUnprotectData) for "dpapi:" values, or AES-256-GCM for "aes:" values
// (N-49: AES is cross-platform, so a value encrypted on macOS/Linux can be
// decrypted on Windows if the per-install AES key file was copied along with
// settings.json). "keyring:" values are markers pointing to macOS Keychain
// or Linux libsecret entries, which Windows cannot access — an error is
// returned so the caller can prompt the user to re-enter the key.
func platformDecryptSecret(stored string) (string, error) {
	if strings.HasPrefix(stored, secretPrefixAES) {
		// N-49: AES is cross-platform — try to decrypt values created on
		// macOS/Linux. This succeeds when the per-install AES key file
		// (~/.config/gugacode/secret.key) was migrated alongside
		// settings.json.
		return aesDecrypt(stored)
	}
	if strings.HasPrefix(stored, secretPrefixKeyring) {
		// N-49: "keyring:" is a marker for a macOS Keychain / Linux libsecret
		// entry. Windows cannot access these — return an error so the caller
		// can prompt the user to re-enter the API key.
		return "", fmt.Errorf("keyring: secret was stored in macOS Keychain or Linux libsecret and cannot be accessed on Windows; please re-enter the API key")
	}
	if !strings.HasPrefix(stored, secretPrefixDPAPI) {
		// Not a recognized encrypted prefix — return as-is (legacy plaintext
		// or foreign format the caller can surface).
		return stored, nil
	}
	b64 := strings.TrimPrefix(stored, secretPrefixDPAPI)
	encrypted, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("dpapi: invalid base64: %w", err)
	}
	if len(encrypted) == 0 {
		return "", nil
	}
	blobIn := dataBlob{
		cbData: uint32(len(encrypted)),
		pbData: &encrypted[0],
	}
	var blobOut dataBlob
	r, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&blobIn)),
		0, // description out (optional)
		0, // reserved
		0, // prompt
		0, // prompt struct
		0, // flags
		uintptr(unsafe.Pointer(&blobOut)),
	)
	if r == 0 {
		return "", fmt.Errorf("dpapi: CryptUnprotectData failed: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(blobOut.pbData)))

	out := make([]byte, blobOut.cbData)
	copy(out, (*[1 << 30]byte)(unsafe.Pointer(blobOut.pbData))[:blobOut.cbData])
	return string(out), nil
}

// platformListSecrets returns an empty list on Windows. DPAPI-encrypted
// secrets live as blobs inside settings.json, not in a separate keyring, so
// there are no orphan entries to discover. The settings UI uses
// GetAPIKeyStorageMethod() to inspect the settings.json entry instead.
func platformListSecrets() ([]SecretInfo, error) {
	return nil, nil
}

// platformDeleteSecret is a no-op on Windows. DPAPI secrets are removed by
// clearing the AIApiKey field in settings.json (SaveSettings with an empty
// key). There is no separate keyring entry to delete.
func platformDeleteSecret(account string) error {
	return nil
}
