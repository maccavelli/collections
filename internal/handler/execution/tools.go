package execution

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/registry"
	"mcp-server-magicskills/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DecomposeTool implements magicskills_decompose_task.
type DecomposeTool struct {
	Engine *engine.Engine
}

func (t *DecomposeTool) Name() string { return "magicskills_decompose_task" }

// DecomposeInput defines the structural representation for the entity.
type DecomposeInput struct {
	util.UniversalBaseInput
	Prompt string `json:"prompt" jsonschema:"The complex task or high-level prompt to decompose into skills"`
}

func (t *DecomposeTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "Break a complex multi-step prompt into a sequence of skill-mapped steps. Analyzes the prompt, identifies sub-tasks, and matches each to the most relevant skill. Use this when a task involves multiple skills and you need an execution plan.",
	}, t.Handle)
}

func (t *DecomposeTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input DecomposeInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	if input.Prompt == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("missing 'prompt' argument"))
		return res, nil, nil
	}

	// Heuristic splitting by common conjunctions
	raw := strings.ToLower(input.Prompt)
	delimiters := []string{", ", " and ", " then ", " followed by "}
	parts := []string{raw}
	for _, numDelim := range delimiters {
		var newParts []string
		for _, part := range parts {
			split := strings.SplitSeq(part, numDelim)
			for s := range split {
				if strings.TrimSpace(s) != "" {
					newParts = append(newParts, strings.TrimSpace(s))
				}
			}
		}
		parts = newParts
	}

	var chain []map[string]any
	seen := make(map[string]bool)

	for _, subIntent := range parts {
		matches := t.Engine.MatchSkills(ctx, subIntent, "", "", 3)
		if len(matches) > 0 {
			best := matches[0] // take the top match for each sub-intent
			if !seen[best.Skill.Metadata.Name] {
				seen[best.Skill.Metadata.Name] = true
				chain = append(chain, map[string]any{
					"intent": subIntent,
					"skill":  best.Skill.Metadata.Name,
					"score":  best.Score,
				})
			}
		}
	}

	// Fallback to general intent matching if no logical sub-intents yielded distinct skills
	if len(chain) == 0 {
		matches := t.Engine.MatchSkills(ctx, input.Prompt, "", "", 3)
		for _, m := range matches {
			chain = append(chain, map[string]any{
				"intent": "general",
				"skill":  m.Skill.Metadata.Name,
				"score":  m.Score,
			})
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: fmt.Sprintf("Decomposed task into %d skill steps.", len(chain)),
		Data: map[string]any{
			"original_prompt": input.Prompt,
			"chain_of_skills": chain,
		},
	}, nil
}

// EfficacyTool implements magicskills_record_efficacy.
type EfficacyTool struct {
	Engine *engine.Engine
}

func (t *EfficacyTool) Name() string { return "magicskills_record_efficacy" }

// EfficacyInput defines the structural representation for the entity.
type EfficacyInput struct {
	util.UniversalBaseInput
	SkillName string `json:"skill_name" jsonschema:"The exact name of the skill evaluated"`
	Target    string `json:"target,omitempty" jsonschema:"Optional target workspace root to dynamically constrain stats bounds"`
	Success   bool   `json:"success" jsonschema:"True if the skill successfully accomplished the goal, false otherwise"`
}

func (t *EfficacyTool) Register(s *mcp.Server) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "Record whether a skill succeeded or failed after execution. Tracks success rates and reliability metrics per skill. Call this after using a skill to improve future match quality.",
	}, t.Handle)
}

func (t *EfficacyTool) Handle(ctx context.Context, request *mcp.CallToolRequest, input EfficacyInput) (*mcp.CallToolResult, any, error) {
	if err := t.Engine.WaitReady(ctx); err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("engine initialization aborted: %w", err))
		return res, nil, nil
	}
	if input.SkillName == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("missing 'skill_name' argument"))
		return res, nil, nil
	}

	err := t.Engine.Store.RecordEfficacy(input.Target, input.SkillName, input.Success)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to record efficacy: %w", err))
		return res, nil, nil
	}

	stats, err := t.Engine.Store.GetEfficacy(input.Target, input.SkillName)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("failed to retrieve efficacy stats: %w", err))
		return res, nil, nil
	}

	statusMsg := "recorded success"
	if !input.Success {
		statusMsg = "recorded failure"
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data"`
	}{
		Summary: fmt.Sprintf("Successfully %s for %s", statusMsg, input.SkillName),
		Data: map[string]any{
			"skill_name": input.SkillName,
			"success":    input.Success,
			"stats":      stats,
		},
	}, nil
}

// Register registers execution tools with the global registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&DecomposeTool{Engine: eng})
	registry.Global.Register(&EfficacyTool{Engine: eng})
}
