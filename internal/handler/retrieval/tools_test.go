package retrieval

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
)

func TestGetTool_Handle(t *testing.T) {
	eng := engine.NewEngine()
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

	// Verify Name
	if tool.Name() != "magicskills_get" {
		t.Errorf("expected magicskills_get, got %s", tool.Name())
	}
}
