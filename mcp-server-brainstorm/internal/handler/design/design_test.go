package design

import (
	"context"
	"testing"

	"mcp-server-brainstorm/internal/state"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDesignHandlers_Empty(t *testing.T) {
	defer func() { recover() }()
	ctx := context.Background()
	mgr := state.NewManager("")

	t1 := &CritiqueDesignTool{Manager: mgr}
	t1.Handle(ctx, &mcp.CallToolRequest{}, DesignInput{})

	t2 := &AnalyzeEvolutionTool{Manager: mgr}
	t2.Handle(ctx, &mcp.CallToolRequest{}, EvolutionInput{})

	t3 := &ClarifyRequirementsTool{Manager: mgr}
	t3.Handle(ctx, &mcp.CallToolRequest{}, RequirementsInput{})

	t4 := &ArchitecturalDiagrammerTool{Manager: mgr}
	t4.Handle(ctx, &mcp.CallToolRequest{}, DiagrammerInput{})

	t5 := &ThesisArchitectTool{Manager: mgr}
	t5.Handle(ctx, &mcp.CallToolRequest{}, ThesisInput{})

	t6 := &AntithesisSkepticTool{Manager: mgr}
	t6.Handle(ctx, &mcp.CallToolRequest{}, AntithesisInput{})
}
