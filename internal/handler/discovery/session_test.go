package discovery

import (
	"context"
	"os"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

func TestDiscoverProjectTool_WithSession(t *testing.T) {
	tmp, err := os.MkdirTemp("", "brainstorm-discovery-session-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	// Write a minimal go.mod to anchor the project root
	_ = os.WriteFile(tmp+"/go.mod", []byte("module testproject\ngo 1.21\n"), 0644)

	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	mgr := state.NewManager(tmp)
	eng := engine.NewEngine(tmp, db)

	// Pre-seed session to pass the LoadSession gate
	_ = mgr.SaveSession(context.Background(), &models.Session{
		ProjectRoot: tmp,
		Metadata:    map[string]any{"scope": "full-stack project analysis"},
	})

	tool := &DiscoverProjectTool{Manager: mgr, Engine: eng}
	input := DiscoverInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			Target: tmp,
		},
	}
	res, payload, err := tool.Handle(context.Background(), &mcp.CallToolRequest{}, input)
	_ = err
	_ = payload
	_ = res
}

func TestDiscoverProjectTool_EmptyTarget(t *testing.T) {
	tmp, _ := os.MkdirTemp("", "brainstorm-discovery-empty-*")
	defer os.RemoveAll(tmp)
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	mgr := state.NewManager(tmp)
	eng := engine.NewEngine(tmp, db)

	_ = mgr.SaveSession(context.Background(), &models.Session{
		ProjectRoot: tmp,
		Metadata:    make(map[string]any),
	})

	tool := &DiscoverProjectTool{Manager: mgr, Engine: eng}
	res, _, _ := tool.Handle(context.Background(), &mcp.CallToolRequest{}, DiscoverInput{})
	_ = res
}
