// Package secretbox encrypts small secrets at rest with AES-GCM.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// Encrypt seals plaintext using a key derived from keyMaterial (for example SESSION_SECRET).
// Ciphertext is nonce||ciphertext.
func Encrypt(keyMaterial string, plaintext []byte) ([]byte, error) {
	if keyMaterial == "" {
		return nil, errors.New("encryption key is empty")
	}

	block, err := aes.NewCipher(deriveKey(keyMaterial))
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt opens ciphertext produced by Encrypt.
func Decrypt(keyMaterial string, ciphertext []byte) ([]byte, error) {
	if keyMaterial == "" {
		return nil, errors.New("encryption key is empty")
	}

	block, err := aes.NewCipher(deriveKey(keyMaterial))
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, payload := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plain, nil
}

func deriveKey(keyMaterial string) []byte {
	sum := sha256.Sum256([]byte(keyMaterial))
	return sum[:]
}
