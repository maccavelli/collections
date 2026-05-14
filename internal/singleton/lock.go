// Package singleton enforces exactly one instance of the socratic-thinker
// daemon per workspace using a Unix Domain Socket (UDS) lock with an
// assassination protocol for immediate takeover of zombie processes.
package singleton

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// diePayload is the byte sequence a new instance sends to assassinate an
// old instance occupying the lock.
const diePayload = "DIE"

// AcquireLock enforces the singleton contract. It returns a net.Listener
// that the caller must keep alive for the duration of the process. If
// another instance already holds the lock, this function will assassinate
// it and take over. This function blocks until the lock is acquired.
func AcquireLock() net.Listener {
	sockPath := lockPath()
	slog.Info("singleton: acquiring lock", "socket", sockPath)

	for {
		ln, err := net.Listen("unix", sockPath)
		if err == nil {
			// Lock acquired — start the assassination listener.
			go listenForAssassination(ln)
			slog.Info("singleton: lock acquired")
			return ln
		}

		// EADDRINUSE — another instance is alive (or a stale socket exists).
		if isAddrInUse(err) {
			if assassinate(sockPath) {
				// Killed the old instance; brief pause for kernel cleanup.
				time.Sleep(50 * time.Millisecond)
				continue
			}
			// Connection refused — stale socket from a crashed process.
			slog.Warn("singleton: removing stale socket", "path", sockPath)
			_ = os.Remove(sockPath)
			time.Sleep(20 * time.Millisecond)
			continue
		}

		// Unexpected error — fatal.
		slog.Error("singleton: failed to bind lock socket", "error", err)
		os.Exit(1)
	}
}

// assassinate dials an existing lock socket and sends the DIE payload.
// Returns true if a live process was contacted (it will terminate itself),
// false if the socket is stale (ECONNREFUSED).
func assassinate(sockPath string) bool {
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		// ECONNREFUSED or ENOENT → stale socket.
		return false
	}
	defer conn.Close()

	slog.Warn("singleton: assassinating previous instance")
	_, _ = conn.Write([]byte(diePayload))
	return true
}

// listenForAssassination runs in an isolated goroutine, completely decoupled
// from the MCP server context. On receiving a valid DIE payload it
// immediately calls os.Exit(0) to guarantee sub-millisecond yielding of
// the lock, regardless of any blocked LLM operations on the main thread.
func listenForAssassination(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener was closed (process shutting down normally).
			return
		}

		buf := make([]byte, 16)
		n, _ := conn.Read(buf)
		conn.Close()

		if string(buf[:n]) == diePayload {
			slog.Warn("singleton: received assassination signal — terminating immediately")
			os.Exit(0)
		}
	}
}

// lockPath computes a deterministic, workspace-scoped socket path.
// Uses os.UserCacheDir() to avoid /tmp cleaner cron jobs.
// The hash is truncated to 12 hex chars (48 bits of entropy) to stay
// well within the 104-byte sockaddr_un path limit.
func lockPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("singleton: cannot determine working directory", "error", err)
		os.Exit(1)
	}

	cwd, _ = filepath.Abs(cwd)

	hash := sha256.Sum256([]byte(cwd))
	short := fmt.Sprintf("%x", hash[:6]) // 12 hex chars

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Fallback: use /tmp on systems without XDG_CACHE_HOME.
		if runtime.GOOS == "linux" {
			cacheDir = "/tmp"
		} else {
			slog.Error("singleton: no cache dir available", "error", err)
			os.Exit(1)
		}
	}

	return filepath.Join(cacheDir, fmt.Sprintf("socratic-%s.sock", short))
}

// isAddrInUse checks if an error is the EADDRINUSE syscall error.
func isAddrInUse(err error) bool {
	var sysErr *os.SyscallError
	if errors.As(err, &sysErr) {
		var errno syscall.Errno
		if errors.As(sysErr.Err, &errno) {
			return errno == syscall.EADDRINUSE
		}
	}
	return false
}
