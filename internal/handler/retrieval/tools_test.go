package retrieval

import (
	"mcp-server-magicskills/internal/state"

	"context"
	"testing"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetTool_Handle(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	skill := &models.Skill{
		Metadata: models.SkillMetadata{
			Name:    "test-skill",
			Version: "1.0.0",
		},
		Sections: map[string]string{
			"usage": "how to use it",
		},
	}
	eng.Skills["test-skill"] = skill
	tool := &GetTool{Engine: eng}

	ctx := context.Background()
	input := GetInput{
		Name:    "test-skill",
		Section: "usage",
	}

	// Test Handle
	_, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	// Test Skill Not Found
	notFoundInput := GetInput{Name: "non-existent"}
	res, _, _ := tool.Handle(ctx, &mcp.CallToolRequest{}, notFoundInput)
	if !res.IsError {
		t.Error("expected error for non-existent skill")
	}

	// Test Semver Compliance
	vInput := GetInput{Name: "test-skill", Version: "1.0.0"}
	res, _, _ = tool.Handle(ctx, &mcp.CallToolRequest{}, vInput)
	if res.IsError {
		t.Errorf("Handle failed for version 1.0.0: %v", res)
	}

	oldVInput := GetInput{Name: "test-skill", Version: "2.0.0"}
	res, _, _ = tool.Handle(ctx, &mcp.CallToolRequest{}, oldVInput)
	if !res.IsError {
		t.Error("expected error for version mismatch (requested 2.0.0, have 1.0.0)")
	}

	// Verify Name
	if tool.Name() != "magicskills_get" {
		t.Errorf("expected magicskills_get, got %s", tool.Name())
	}
}

func TestRegister(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	defer store.Close()
	Register(eng, nil)

	srv := mcp.NewServer(&mcp.Implementation{Name: "test"}, &mcp.ServerOptions{})
	tool := &GetTool{Engine: eng}
	tool.Register(srv)
}
