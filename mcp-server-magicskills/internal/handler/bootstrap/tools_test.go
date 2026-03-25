package bootstrap

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
)

func TestBootstrapTool_Handle(t *testing.T) {
	eng := engine.NewEngine()
	skill := &models.Skill{
		Metadata: models.SkillMetadata{
			Name: "test-skill",
		},
		Sections: map[string]string{
			"workflow": "- task 1\n- task 2",
		},
	}
	eng.Skills["test-skill"] = skill
	tool := &BootstrapTool{Engine: eng}

	ctx := context.Background()
	input := BootstrapInput{
		Name: "test-skill",
	}

	// Test Handle
	res, _, err := tool.Handle(ctx, &mcp.CallToolRequest{}, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}
	txt := res.Content[0].(*mcp.TextContent).Text
	if !res.IsError && (len(txt) == 0 || res.Content[0].(*mcp.TextContent).Text == "") {
		t.Error("expected non-empty checklist")
	}

	// Verify Name
	if tool.Name() != "magicskills_bootstrap" {
		t.Errorf("expected magicskills_bootstrap, got %s", tool.Name())
	}
}
