package config

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	plaintext := "secret-token-123"
	
	encrypted, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}
	
	if encrypted == plaintext {
		t.Error("Encrypted text matches plaintext")
	}
	
	decrypted, err := Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}
	
	if decrypted != plaintext {
		t.Errorf("Expected decrypted %q, got %q", plaintext, decrypted)
	}
}

func TestDecryptInvalid(t *testing.T) {
	// Not base64
	_, err := Decrypt("invalid-base64-!@#")
	if err == nil {
		t.Error("Expected error decrypting invalid base64")
	}
	
	// Too short
	_, err = Decrypt("YQ==") // 'a' in base64
	if err == nil {
		t.Error("Expected error decrypting too short ciphertext")
	}
}
