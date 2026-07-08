package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
)

// keyFilePath returns the path to the per-install AES key file, stored in the
// XDG config directory. The key is a 32-byte random value generated on first
// use and persisted with 0600 permissions.
func keyFilePath() string {
	return filepath.Join(xdg.ConfigHome, "gugacode", "secret.key")
}

// loadOrCreateAESKey reads the per-install AES key, generating and persisting a
// new 32-byte key on first use. The key file is created with 0600 permissions
// so only the current user can read it.
func loadOrCreateAESKey() ([]byte, error) {
	p := keyFilePath()
	data, err := os.ReadFile(p)
	if err == nil {
		key, derr := hex.DecodeString(strings.TrimSpace(string(data)))
		if derr == nil && len(key) == 32 {
			return key, nil
		}
		// Corrupt or wrong-length key file — fall through to regenerate.
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("aes: failed to generate key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return nil, fmt.Errorf("aes: failed to create key dir: %w", err)
	}
	if err := os.WriteFile(p, []byte(hex.EncodeToString(key)), 0600); err != nil {
		return nil, fmt.Errorf("aes: failed to write key file: %w", err)
	}
	return key, nil
}

// aesEncrypt encrypts plaintext using AES-256-GCM with the per-install key.
// The nonce is prepended to the ciphertext, base64-encoded, and prefixed
// with the "aes:" marker. This is the fallback path when no platform-native
// keyring is available (or the keyring CLI is missing).
func aesEncrypt(plaintext string) (string, error) {
	bytes := []byte(plaintext)
	if len(bytes) == 0 {
		return "", nil
	}
	key, err := loadOrCreateAESKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes: NewCipher failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("aes: NewGCM failed: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("aes: nonce generation failed: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, bytes, nil)
	return secretPrefixAES + base64.StdEncoding.EncodeToString(sealed), nil
}

// aesDecrypt decrypts an "aes:"-prefixed value using AES-256-GCM.
func aesDecrypt(stored string) (string, error) {
	b64 := strings.TrimPrefix(stored, secretPrefixAES)
	sealed, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("aes: invalid base64: %w", err)
	}
	key, err := loadOrCreateAESKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("aes: NewCipher failed: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("aes: NewGCM failed: %w", err)
	}
	if len(sealed) < gcm.NonceSize() {
		return "", fmt.Errorf("aes: ciphertext too short")
	}
	nonce, ciphertext := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("aes: decryption failed: %w", err)
	}
	return string(plaintext), nil
}
