package execution

import (
	"context"
	"testing"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/state"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExecutionTools(t *testing.T) {
	store, _ := state.NewStore(t.TempDir())
	defer store.Close()

	eng, _ := engine.NewEngine(store, t.TempDir()+"/idx")
	close(eng.ReadyCh)
	Register(eng, nil)

	tool1 := &DecomposeTool{Engine: eng}
	if tool1.Name() != "magicskills_decompose_task" {
		t.Fatal("wrong name for decompose tool")
	}

	ctx := context.Background()
	_, _, err := tool1.Handle(ctx, &mcp.CallToolRequest{}, DecomposeInput{Prompt: ""})
	_ = err

	eng.Skills["test-skill"] = &models.Skill{Metadata: models.SkillMetadata{Name: "test-skill"}}
	_ = eng.Bleve.Index("test-skill", map[string]any{"name": "test-skill", "content": "hello world test followed by split"})

	_, out1, err := tool1.Handle(ctx, &mcp.CallToolRequest{}, DecomposeInput{Prompt: "hello world test followed by split"})
	if err != nil || out1 == nil {
		t.Fatal("expected no handle error for decompose task")
	}

	tool2 := &EfficacyTool{Engine: eng}
	if tool2.Name() != "magicskills_record_efficacy" {
		t.Fatal("wrong name for efficacy tool")
	}

	_, _, err = tool2.Handle(ctx, &mcp.CallToolRequest{}, EfficacyInput{SkillName: ""})
	_ = err

	_, out2, err := tool2.Handle(ctx, &mcp.CallToolRequest{}, EfficacyInput{SkillName: "test-skill", Success: true})
	if err != nil || out2 == nil {
		t.Fatal("expected no handle error for efficacy record")
	}
}
