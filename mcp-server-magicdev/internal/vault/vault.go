// Package vault provides functionality for the vault subsystem.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var vaultKey []byte

// Init loads the master key from ~/.config/mcp-server-magicdev/vault.key
// If the key does not exist, it securely generates a new 32-byte key.
func Init() error {
	if vaultKey != nil {
		return nil // already initialized
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home dir for vault: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "mcp-server-magicdev")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("could not create config dir: %w", err)
	}

	keyFile := filepath.Join(configDir, "vault.key")

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		// Generate 32 bytes of secure random entropy
		newKey := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
			return fmt.Errorf("could not generate random key: %w", err)
		}

		encodedKey := base64.StdEncoding.EncodeToString(newKey)
		// 0600 ensures only the owner can read/write the file
		if err := os.WriteFile(keyFile, []byte(encodedKey), 0600); err != nil {
			return fmt.Errorf("could not write vault key: %w", err)
		}
		vaultKey = newKey
		return nil
	}

	// Read existing key
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("could not read vault key: %w", err)
	}

	decodedKey, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return fmt.Errorf("could not decode vault key: %w", err)
	}

	if len(decodedKey) != 32 {
		return errors.New("vault key must be exactly 32 bytes (AES-256)")
	}

	vaultKey = decodedKey
	return nil
}

// Encrypt encrypts a plaintext string using AES-256-GCM
func Encrypt(plaintext string) (string, error) {
	if vaultKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}

	block, err := aes.NewCipher(vaultKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a ciphertext string using AES-256-GCM
func Decrypt(ciphertextStr string) (string, error) {
	if vaultKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextStr)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(vaultKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("malformed ciphertext")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
