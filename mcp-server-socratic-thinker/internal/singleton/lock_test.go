package singleton

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockPath_Deterministic(t *testing.T) {
	t.Parallel()
	p1 := lockPath()
	p2 := lockPath()
	if p1 != p2 {
		t.Fatalf("lockPath() not deterministic: %q vs %q", p1, p2)
	}
	if !strings.HasSuffix(p1, ".sock") {
		t.Fatalf("expected .sock suffix, got %q", p1)
	}
	if !strings.Contains(filepath.Base(p1), "socratic-") {
		t.Fatalf("expected socratic- prefix in basename, got %q", filepath.Base(p1))
	}
}

func TestLockPath_UnderPathLimit(t *testing.T) {
	t.Parallel()
	p := lockPath()
	// sockaddr_un limit is typically 104 (macOS) or 108 (Linux) bytes.
	if len(p) >= 104 {
		t.Fatalf("lockPath length %d exceeds sockaddr_un limit (104): %q", len(p), p)
	}
}

func TestAcquireLock_SingleInstance(t *testing.T) {
	// Use a temp dir so we don't conflict with any running daemon.
	tmp := t.TempDir()
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	ln := AcquireLock()
	if ln == nil {
		t.Fatal("AcquireLock returned nil")
	}
	defer ln.Close()

	// The socket file should exist.
	sockPath := lockPath()
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		t.Fatalf("socket file not created at %q", sockPath)
	}
}

func TestAssassinate_StaleSocket(t *testing.T) {
	tmp := t.TempDir()
	// Create a stale socket file (no listener).
	stalePath := filepath.Join(tmp, "stale.sock")
	ln, err := net.Listen("unix", stalePath)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // Close immediately — socket file remains.

	// assassinate should return false (ECONNREFUSED).
	got := assassinate(stalePath)
	if got {
		t.Fatal("expected assassinate to return false for stale socket")
	}
}

func TestAssassinate_LiveProcess(t *testing.T) {
	tmp := t.TempDir()
	sockPath := filepath.Join(tmp, "live.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Simulate the assassination listener reading the payload.
	received := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 16)
		n, _ := conn.Read(buf)
		received <- string(buf[:n])
	}()

	got := assassinate(sockPath)
	if !got {
		t.Fatal("expected assassinate to return true for live socket")
	}

	select {
	case payload := <-received:
		if payload != diePayload {
			t.Fatalf("expected %q payload, got %q", diePayload, payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for assassination payload")
	}
}

func TestIsAddrInUse(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	sockPath := filepath.Join(tmp, "inuse.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Attempt to bind the same path — should get EADDRINUSE.
	_, err = net.Listen("unix", sockPath)
	if err == nil {
		t.Fatal("expected EADDRINUSE error, got nil")
	}
	if !isAddrInUse(err) {
		t.Fatalf("expected isAddrInUse to return true for %v", err)
	}
}
