package handler

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSyncEcosystem(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ctx := context.Background()

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "sync_ecosystem",
			Arguments: json.RawMessage(`{}`),
		},
	}

	res, err := h.SyncEcosystem(ctx, req)
	if err != nil {
		t.Fatalf("SyncEcosystem failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected results, got none")
	}
}

func TestSyncServer(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	ctx := context.Background()

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "sync_server",
			Arguments: json.RawMessage(`{"name": "magictools"}`),
		},
	}

	res, err := h.SyncServer(ctx, req)
	if err != nil {
		t.Fatalf("SyncServer failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected results, got none")
	}
}

func TestSleepServers(t *testing.T) {
	h, _, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "sleep_servers",
			Arguments: json.RawMessage(`{}`),
		},
	}

	res, err := h.SleepServers(ctx, req)
	if err != nil {
		t.Fatalf("SleepServers failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected results, got none")
	}
}

func TestWakeServers(t *testing.T) {
	h, _, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "wake_servers",
			Arguments: json.RawMessage(`{}`),
		},
	}

	res, err := h.WakeServers(ctx, req)
	if err != nil {
		t.Fatalf("WakeServers failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected results, got none")
	}
}

func TestReloadServers(t *testing.T) {
	h, _, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// 1. Test full reload
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "reload_servers",
			Arguments: json.RawMessage(`{}`),
		},
	}

	res, err := h.ReloadServers(ctx, req)
	if err != nil {
		t.Fatalf("ReloadServers failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatalf("expected results, got none")
	}

	// 2. Test selective reload (empty list is fine)
	req = &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "reload_servers",
			Arguments: json.RawMessage(`{"names": "test-server"}`),
		},
	}

	res, err = h.ReloadServers(ctx, req)
	if err != nil {
		t.Fatalf("ReloadServers failed: %v", err)
	}
}

func TestServerEvents(t *testing.T) {
	h, _, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)

	h.OnServerPromoted("test-server")
	h.OnServerDemoted("test-server")
	h.OnServerUpdated("test-server")
}
