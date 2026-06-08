package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext using AES-256-GCM.
// key must be exactly 32 bytes.
// Returns base64-encoded ciphertext (nonce prepended).
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("encrypt: key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt: new gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt: read nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded ciphertext produced by Encrypt.
func Decrypt(key []byte, encoded string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("decrypt: key must be 32 bytes, got %d", len(key))
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decrypt: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("decrypt: new cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decrypt: new gcm: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("decrypt: ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: open: %w", err)
	}

	return string(plaintext), nil
}
