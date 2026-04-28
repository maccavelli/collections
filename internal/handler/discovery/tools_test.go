package discovery

import (
	"mcp-server-magicskills/internal/state"

	"context"
	"testing"

	"mcp-server-magicskills/internal/engine"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestListTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	tool := &ListTool{Engine: eng}

	ctx := context.Background()
	_, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, struct{}{})
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if tool.Name() != "magicskills_list" {
		t.Errorf("expected magicskills_list, got %s", tool.Name())
	}
}

func TestMatchTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	tool := &MatchTool{Engine: eng}

	ctx := context.Background()
	input := MatchInput{Intent: "refactor go"}
	_, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if tool.Name() != "magicskills_match" {
		t.Errorf("expected magicskills_match, got %s", tool.Name())
	}
}

func TestRegister(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	Register(eng, nil)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})
	list := &ListTool{Engine: eng}
	list.Register(srv)

	match := &MatchTool{Engine: eng}
	match.Register(srv)
}
