package handler

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterPrompts registers the Master Prompt template that instructs the AI on the exact state machine sequence.
func RegisterPrompts(s *mcp.Server) {
	s.AddPrompt(&mcp.Prompt{
		Name:        "start-magicdev",
		Description: "Initializes the MagicDev pipeline for a new .NET or Node.js idea.",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "idea",
				Description: "The raw software idea or feature request",
				Required:    true,
			},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		idea := ""
		if req.Params != nil && req.Params.Arguments != nil {
			idea = req.Params.Arguments["idea"]
		}

		text := fmt.Sprintf(`You are the MagicDev Architect. I have an idea: "%s". 
Follow these strict steps using the magicdev-server:
1. Call 'evaluate_idea' with this text to initialize a session.
2. Call 'clarify_requirements' to perform Socratic gap analysis.
3. Call 'ingest_standards' to pull in architectural standards.
4. Call 'critique_design' as a vetting gate against the standards.
5. Call 'finalize_requirements' to generate the golden copy JSON spec.
6. Call 'blueprint_implementation' to map the design to technical patterns and estimate complexity.
7. Only after vetting and blueprinting is complete, call 'generate_documents' to sync with Jira, Confluence, and GitLab.
8. Wrap up with 'complete_design'.
Maintain the session_id across all calls.`, idea)

		return &mcp.GetPromptResult{
			Description: "MagicDev Architect Instructions",
			Messages: []*mcp.PromptMessage{
				{
					Role: mcp.Role("user"),
					Content: &mcp.TextContent{
						Text: text,
					},
				},
			},
		}, nil
	})
}
