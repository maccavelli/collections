package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestAcquireLock(t *testing.T) {
	// Override lockDir to use a temp directory.
	origDir := os.Getenv("XDG_CACHE_HOME")
	tmpDir := t.TempDir()
	os.Setenv("XDG_CACHE_HOME", tmpDir)
	defer os.Setenv("XDG_CACHE_HOME", origDir)

	fl, err := AcquireLock()
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if fl == nil {
		t.Fatal("AcquireLock returned nil lock")
	}

	// Verify PID file was written.
	lockPath := filepath.Join(tmpDir, "mcp-server-magicdev", lockFileName)
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("Lock file contains non-numeric PID: %q", string(data))
	}
	if pid != os.Getpid() {
		t.Errorf("Expected PID %d in lock file, got %d", os.Getpid(), pid)
	}

	// Release the lock.
	if err := fl.Unlock(); err != nil {
		t.Errorf("Unlock failed: %v", err)
	}
}

func TestAcquireLock_SecondInstance(t *testing.T) {
	// Acquire first lock.
	origDir := os.Getenv("XDG_CACHE_HOME")
	tmpDir := t.TempDir()
	os.Setenv("XDG_CACHE_HOME", tmpDir)
	defer os.Setenv("XDG_CACHE_HOME", origDir)

	fl1, err := AcquireLock()
	if err != nil {
		t.Fatalf("First AcquireLock failed: %v", err)
	}
	defer fl1.Unlock()

	// Write a fake PID (non-existent process) to simulate a stale lock.
	lockPath := filepath.Join(tmpDir, "mcp-server-magicdev", lockFileName)
	os.WriteFile(lockPath, []byte("999999999"), 0644)

	// Release first lock to simulate the "old process dying".
	fl1.Unlock()

	// Second instance should acquire the lock.
	fl2, err := AcquireLock()
	if err != nil {
		t.Fatalf("Second AcquireLock failed: %v", err)
	}
	defer fl2.Unlock()
}

func TestReadPID(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Valid PID.
	os.WriteFile(lockPath, []byte("12345"), 0644)
	pid, err := readPID(lockPath)
	if err != nil {
		t.Fatalf("readPID failed: %v", err)
	}
	if pid != 12345 {
		t.Errorf("Expected PID 12345, got %d", pid)
	}

	// Empty file.
	os.WriteFile(lockPath, []byte(""), 0644)
	_, err = readPID(lockPath)
	if err == nil {
		t.Error("Expected error for empty lock file")
	}

	// Non-numeric content.
	os.WriteFile(lockPath, []byte("notapid"), 0644)
	_, err = readPID(lockPath)
	if err == nil {
		t.Error("Expected error for non-numeric PID")
	}

	// Missing file.
	_, err = readPID(filepath.Join(tmpDir, "nonexistent.lock"))
	if err == nil {
		t.Error("Expected error for missing lock file")
	}
}

func TestKillProcess_NonExistent(t *testing.T) {
	// Should not panic when killing a non-existent PID.
	killProcess(999999999)
}

func TestKillProcess_Self(t *testing.T) {
	// Should not kill ourselves (safety check).
	killProcess(os.Getpid())
	// If we got here, we didn't kill ourselves — test passes.
}

func TestWatchParent_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		WatchParent(ctx, cancel)
		close(done)
	}()

	// Cancel context immediately — watchdog should exit.
	cancel()

	select {
	case <-done:
		// Success — watchdog exited.
	case <-time.After(5 * time.Second):
		t.Fatal("WatchParent did not exit after context cancellation")
	}
}

func TestShutdownDeadline(t *testing.T) {
	// We can't fully test os.Exit, but we can verify the goroutine starts.
	// Just ensure it doesn't panic.
	ShutdownDeadline(1 * time.Hour) // Very long deadline — won't fire during test.
}

func TestIsParentAlive(t *testing.T) {
	// Our actual parent should be alive.
	ppid := os.Getppid()
	if ppid > 1 {
		if !isParentAlive(ppid) {
			t.Error("Expected current parent to be alive")
		}
	}

	// A non-existent PID should report as dead.
	if isParentAlive(999999999) {
		t.Error("Expected non-existent PID to report as dead")
	}
}
