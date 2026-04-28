package client

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DisconnectServer gracefully disconnects a single sub-server.
// If deleteFromMap is true, the server entry is removed from the registry.
func (m *WarmRegistry) DisconnectServer(name string, deleteFromMap bool) {
	m.mu.Lock()
	s, ok := m.Servers[name]
	if !ok {
		m.mu.Unlock()
		return
	}
	s.ReadyChan = nil
	s.Status = StatusDisconnected
	// Snapshot and nil-out session/process/cancel under lock to prevent
	// races with the spawnProcess Wait goroutine.
	session := s.Session
	process := s.Process
	cancelFunc := s.CancelFunc
	s.Session = nil
	s.Process = nil
	if deleteFromMap {
		delete(m.Servers, name)
	}
	m.mu.Unlock()

	if m.PIDDir != "" {
		if err := os.Remove(filepath.Join(m.PIDDir, name+".pid")); err != nil && !os.IsNotExist(err) {
			slog.Warn("lifecycle: failed to remove pid file on disconnect", "server", name, "error", err)
		}
	}

	m.closeSubServerSnapshot(session, process, name)

	if cancelFunc != nil {
		cancelFunc()
	}
	slog.Info("lifecycle: server offline", "name", name)
}

// DisconnectAll disconnects all registered sub-servers.
func (m *WarmRegistry) DisconnectAll() {
	var names []string
	m.mu.RLock()
	for name := range m.Servers {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		m.DisconnectServer(name, true)
	}
}

// closeSubServerSnapshot safely closes a session and kills a process using snapshotted values.
// This avoids race conditions with the spawnProcess Wait goroutine.
func (m *WarmRegistry) closeSubServerSnapshot(session *mcp.ClientSession, process *exec.Cmd, name string) {
	if session != nil {
		go func(sess *mcp.ClientSession, serverName string) {
			defer func() { recover() }() //nolint:errcheck // intentional panic guard during teardown
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			done := make(chan struct{})
			go func() {
				defer func() { recover() }() //nolint:errcheck // intentional panic guard during teardown
				if err := sess.Close(); err != nil {
					slog.Debug("lifecycle: session close error during teardown", "server", serverName, "error", err)
				}
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
				slog.Warn("lifecycle: session close timed out", "server", serverName)
			}
		}(session, name)
	}
	if process != nil {
		if err := killProcessGroup(process); err != nil {
			slog.Debug("lifecycle: kill process group failed during teardown", "error", err)
		}
	}
}
