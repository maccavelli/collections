package handler

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"mcp-server-magictools/internal/db"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSyncHandlersMethods(t *testing.T) {
	ctx := context.Background()

	t.Run("SyncEcosystem", func(t *testing.T) {
		h, store, _, tmpDir := newTestHandler(t)
		defer os.RemoveAll(tmpDir)
		defer store.Close()

		req := &mcp.CallToolRequest{}
		_, err := h.SyncEcosystem(ctx, req)
		if err != nil {
			t.Errorf("SyncEcosystem failed: %v", err)
		}
	})

	t.Run("SyncServer", func(t *testing.T) {
		h, store, _, tmpDir := newTestHandler(t)
		defer os.RemoveAll(tmpDir)
		defer store.Close()

		args, _ := json.Marshal(map[string]string{"name": "test-server"})
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: args,
			},
		}
		_, err := h.SyncServer(ctx, req)
		if err == nil {
			t.Errorf("SyncServer unexpectedly succeeded with a non-MCP dummy command")
		}

		argsMT, _ := json.Marshal(map[string]string{"name": "magictools"})
		reqMT := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: argsMT,
			},
		}
		_, errMT := h.SyncServer(ctx, reqMT)
		if errMT != nil {
			t.Errorf("SyncServer magictools failed: %v", errMT)
		}
	})

	t.Run("SleepServers", func(t *testing.T) {
		h, store, _, tmpDir := newTestHandler(t)
		defer os.RemoveAll(tmpDir)
		defer store.Close()

		_, err := h.SleepServers(ctx, &mcp.CallToolRequest{})
		if err != nil {
			t.Errorf("SleepServers failed: %v", err)
		}
	})

	t.Run("WakeServers", func(t *testing.T) {
		h, store, _, tmpDir := newTestHandler(t)
		defer os.RemoveAll(tmpDir)
		defer store.Close()

		_, err := h.WakeServers(ctx, &mcp.CallToolRequest{})
		if err != nil {
			t.Errorf("WakeServers failed: %v", err)
		}
	})

	t.Run("ReloadServers", func(t *testing.T) {
		h, store, _, tmpDir := newTestHandler(t)
		defer os.RemoveAll(tmpDir)
		defer store.Close()

		t.Run("ReloadAll", func(t *testing.T) {
			_, err := h.ReloadServers(ctx, &mcp.CallToolRequest{})
			if err != nil {
				t.Errorf("ReloadServers (all) failed: %v", err)
			}
		})

		t.Run("ReloadSelective", func(t *testing.T) {
			args, _ := json.Marshal(map[string]string{"names": "test-server1 test-server2"})
			req := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Arguments: args,
				},
			}
			_, err := h.ReloadServers(ctx, req)
			if err != nil {
				t.Errorf("ReloadServers (selective) failed: %v", err)
			}
		})
	})
}

func TestOnServerPromotedDemoted(t *testing.T) {
	h, store, _, tmpDir := newTestHandler(t)
	defer os.RemoveAll(tmpDir)
	defer store.Close()

	// 1. Demoted (magictools gains ownership)
	h.OnServerDemoted("plugin-x")

	// 2. Promoted (IDE gains ownership)
	_ = store.SaveTool(&db.ToolRecord{
		URN:    "plugin-x:tool1",
		Server: "plugin-x",
	})

	h.OnServerPromoted("plugin-x")

	results, _ := store.SearchTools("tool1", "", "", 0.0)
	if len(results) != 0 {
		t.Errorf("expected 0 results after promotion, got %d", len(results))
	}
}
