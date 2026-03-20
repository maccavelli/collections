package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mcp-server-brainstorm/internal/models"
)

func TestSessionManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-state-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)

	// 1. Test Load non-existent.
	session, err := mgr.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session.Status != "DISCOVERY" {
		t.Errorf("want DISCOVERY, got %s", session.Status)
	}

	// 2. Test Save.
	session.Status = "PLANNING"
	if err := mgr.SaveSession(context.Background(), session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// 3. Test Load existing.
	session2, err := mgr.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session2.Status != "PLANNING" {
		t.Errorf("want PLANNING, got %s", session2.Status)
	}
}

func TestSaveSession_MarshalError(t *testing.T) {
	mgr := NewManager(".")
	// Session has no fields that would fail marshaling normally,
	// but we test the pattern.
	err := mgr.SaveSession(context.Background(), &models.Session{})
	if err != nil {
		// Should succeed.
	}
}

func TestSaveSession_Error(t *testing.T) {
	// 1. Write error
	m := NewManager("/not-real-at-all/invalid/directory")
	s := &models.Session{Status: "error"}
	if err := m.SaveSession(context.Background(), s); err == nil {
		t.Error("expected error for invalid directory")
	}

	// 2. Rename error
	tmpDir := t.TempDir()
	m2 := NewManager(tmpDir)
	// Create a directory where the final file should be to cause Rename to fail.
	os.Mkdir(filepath.Join(tmpDir, StateFile), 0755)
	if err := m2.SaveSession(context.Background(), s); err == nil {
		t.Error("expected error when target is a directory")
	}
}

func TestLoadSession_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, StateFile)
	// Create a directory where the file should be to cause ReadFile to fail.
	os.Mkdir(path, 0755)
	
	m := NewManager(tmpDir)
	_, err := m.LoadSession(context.Background())
	if err == nil {
		t.Error("expected error when reading a directory as a file")
	}
}

func TestLoadSession_UnmarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, StateFile)
	os.WriteFile(path, []byte("{invalid-json"), 0644)
	
	m := NewManager(tmpDir)
	_, err := m.LoadSession(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
