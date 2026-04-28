package client

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDisconnectServer(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "magictools-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	m := &WarmRegistry{
		Servers: make(map[string]*SubServer),
		PIDDir:  tempDir,
	}

	name := "test-server"
	m.Servers[name] = &SubServer{
		Name:      name,
		Status:    StatusReady,
		ReadyChan: make(chan struct{}),
	}

	// Create a dummy PID file
	pidFile := filepath.Join(tempDir, name+".pid")
	if err := os.WriteFile(pidFile, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test disconnect without delete
	m.DisconnectServer(name, false)

	if m.Servers[name].Status != StatusDisconnected {
		t.Errorf("expected status %v, got %v", StatusDisconnected, m.Servers[name].Status)
	}
	if m.Servers[name].ReadyChan != nil {
		t.Error("expected ReadyChan to be nil")
	}

	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed")
	}

	// Test disconnect with delete
	m.DisconnectServer(name, true)
	if _, ok := m.Servers[name]; ok {
		t.Error("expected server to be deleted from map")
	}
}

func TestDisconnectAll(t *testing.T) {
	m := &WarmRegistry{
		Servers: make(map[string]*SubServer),
	}

	m.Servers["s1"] = &SubServer{Name: "s1", Status: StatusReady}
	m.Servers["s2"] = &SubServer{Name: "s2", Status: StatusReady}

	m.DisconnectAll()

	if len(m.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(m.Servers))
	}
}

func TestEvictInactive(t *testing.T) {
	m := &WarmRegistry{
		Servers: make(map[string]*SubServer),
	}

	now := time.Now()
	m.Servers["active"] = &SubServer{
		Name:     "active",
		LastUsed: now,
		Session:  &mcp.ClientSession{},
	}
	m.Servers["inactive"] = &SubServer{
		Name:     "inactive",
		LastUsed: now.Add(-2 * time.Hour),
		Session:  &mcp.ClientSession{},
	}

	m.EvictInactive(1 * time.Hour)

	if _, ok := m.Servers["inactive"]; ok {
		t.Error("expected inactive server to be evicted")
	}
	if _, ok := m.Servers["active"]; !ok {
		t.Error("expected active server to remain")
	}
}

func TestEvictLRU(t *testing.T) {
	m := &WarmRegistry{
		Servers: make(map[string]*SubServer),
	}

	// MaxRunningServers is 50. To trigger eviction, we need len(active) > 50.
	// We'll add 52 servers total.
	for i := 0; i <= MaxRunningServers+1; i++ {
		name := fmt.Sprintf("server-%d", i)
		m.Servers[name] = &SubServer{
			Name:     name,
			LastUsed: time.Now().Add(time.Duration(i) * time.Second),
			Session:  &mcp.ClientSession{},
		}
	}

	// active will contain server-0 to server-50 (51 servers)
	m.EvictLRU("server-51")

	if _, ok := m.Servers["server-0"]; ok {
		t.Error("expected oldest server-0 to be evicted")
	}
	if _, ok := m.Servers["server-1"]; !ok {
		t.Error("expected server-1 to remain")
	}
}

func TestCloseSubServerSnapshotTimeout(t *testing.T) {
	m := &WarmRegistry{}
	// Mock a session that never closes (actually harder with the real SDK,
	// but we can at least test that the function doesn't block infinitely)
	m.closeSubServerSnapshot(nil, nil, "timeout-test")
}

func TestDisconnectServerNilFields(t *testing.T) {
	m := &WarmRegistry{
		Servers: make(map[string]*SubServer),
	}
	m.Servers["nil-fields"] = &SubServer{
		Name: "nil-fields",
		// Session and Process are nil
	}
	// Should not panic
	m.DisconnectServer("nil-fields", true)
}
