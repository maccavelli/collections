package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVaultLifecycle(t *testing.T) {
	// Isolate the test environment
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Ensure vaultKey is nil to start
	vaultKey = nil

	// 1. Test Init (should generate new key)
	if err := Init(); err != nil {
		t.Fatalf("Init failed on fresh setup: %v", err)
	}
	if vaultKey == nil {
		t.Fatal("vaultKey is nil after successful Init")
	}

	// Verify file was created
	keyFile := filepath.Join(tempHome, ".config", "mcp-server-magicdev", "vault.key")
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Fatalf("Vault key file was not created at %s", keyFile)
	}

	// 2. Test Encrypt/Decrypt
	plaintext := "my_secret_token"
	ciphertext, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if ciphertext == "" {
		t.Fatal("Ciphertext is empty")
	}

	decrypted, err := Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("Expected decrypted %q, got %q", plaintext, decrypted)
	}

	// 3. Test Init with existing key
	oldKey := vaultKey
	vaultKey = nil // force reload
	if err := Init(); err != nil {
		t.Fatalf("Init failed on reload: %v", err)
	}
	
	if string(vaultKey) != string(oldKey) {
		t.Errorf("Reloaded key does not match original key")
	}

	// 4. Test Decrypt with malformed ciphertext
	_, err = Decrypt("invalid_base64_!@#")
	if err == nil {
		t.Error("Expected error decrypting invalid base64")
	}

	// 5. Test Encrypt when uninitialized
	vaultKey = nil
	os.Remove(keyFile) // delete key file so it generates a new one
	_, err = Encrypt("test2")
	if err != nil {
		t.Fatalf("Encrypt failed when uninitialized: %v", err)
	}
	
	// 6. Test Decrypt when uninitialized
	vaultKey = nil
	_, err = Decrypt(ciphertext) 
	// This will actually fail because the key is regenerated and won't match the old ciphertext!
	if err == nil {
		t.Error("Expected error decrypting old ciphertext with new regenerated key")
	}
}
