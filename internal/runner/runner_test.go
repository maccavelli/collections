package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunner(t *testing.T) {
	tmp, _ := os.MkdirTemp("", "runner-test")
	defer os.RemoveAll(tmp)

	r := New(tmp)
	if r.Dir != tmp {
		t.Errorf("expected %s, got %s", tmp, r.Dir)
	}

	// Test RunGo (version)
	res, err := r.RunGo(context.Background(), "version")
	if err != nil {
		t.Fatalf("RunGo version failed: %v", err)
	}
	if !res.Success() {
		t.Errorf("expected version to succeed, stderr: %s", string(res.Stderr))
	}
	if len(res.Stdout) == 0 {
		t.Error("expected stdout for version")
	}

	// Test RunGo with empty args (bug prevention)
	res, err = r.RunGo(context.Background(), "version", "", " ")
	if err != nil {
		t.Fatalf("RunGo with empty args failed: %v", err)
	}
	if !res.Success() {
		t.Error("expected version with empty args to succeed")
	}

	// Test WriteFileAtomic
	path := "test.txt" // relative path
	data := []byte("hello")
	err = r.WriteFileAtomic(path, data)
	if err != nil {
		t.Fatalf("WriteFileAtomic failed: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(tmp, path))
	if string(got) != "hello" {
		t.Errorf("expected hello, got %s", string(got))
	}

	// Test result helper
	s := res.String()
	if len(s) == 0 {
		t.Error("expected non-empty string result")
	}
}

func TestRunner_ErrorHandling(t *testing.T) {
	tmp, _ := os.MkdirTemp("", "runner-err-test")
	defer os.RemoveAll(tmp)
	r := New(tmp)

	// Test invalid command
	res, err := r.RunGo(context.Background(), "no-such-cmd")
	if err == nil && res.Success() {
		t.Error("expected error for invalid command")
	}
}
