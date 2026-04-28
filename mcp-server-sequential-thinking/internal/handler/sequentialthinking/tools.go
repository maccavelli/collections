package sequentialthinking

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-sequential-thinking/internal/engine"
	"mcp-server-sequential-thinking/internal/registry"
	"mcp-server-sequential-thinking/internal/util"
)

// SequentialThinkingTool wraps the Sequential Thinking Engine as an MCP tool.
type SequentialThinkingTool struct {
	Engine *engine.Engine
}

// Name returns the MCP tool identifier.
func (t *SequentialThinkingTool) Name() string {
	return "sequentialthinking"
}

// Register exposes the tool metadata and registers the handler.
func (t *SequentialThinkingTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: `[DIRECTIVE: Deep Cognitive Processing] A profound cognitive processing lattice designed to map intricate paradoxes and abstract reasoning grids. Instead of merely executing, this engine dwells within the architecture, intentionally neutralizing cognitive drift by enforcing rigorous, expansive self-reflection over isolated neural logic loops. Engage this hyper-thread when you need to gaze deeply into a structural problem, unravel complex logic topologies, or truly ruminate on high-level architectural conceptualization before committing to terminal edits... Keywords: analyze, evaluate, sequential, reason, think, consider, reflect, deliberate, ponder, contemplate, ruminate, cognitive, conceptualize, logic`,
	}, t.Handle)
}

// Handle processes incoming requests and executes thought generation.
func (t *SequentialThinkingTool) Handle(ctx context.Context, req *mcp.CallToolRequest, input engine.ThoughtData) (*mcp.CallToolResult, any, error) {
	resp, err := t.Engine.ProcessThought(input)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(err)
		return res, nil, nil
	}
	return &mcp.CallToolResult{}, resp, nil
}

// Register adds the sequential thinking tool to the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&SequentialThinkingTool{Engine: eng})
}
