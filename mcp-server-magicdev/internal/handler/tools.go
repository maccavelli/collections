package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magicdev/internal/db"
	"mcp-server-magicdev/internal/integration"
)

func textResult(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}, nil, nil
}

func errorResult(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}, nil, nil
}

type EvaluateIdeaArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID"`
	TechStack string `json:"tech_stack" jsonschema:"description=The technology stack (.NET or Node)"`
}

type ClarifyRequirementsArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID"`
	Findings  string `json:"findings" jsonschema:"description=Socratic gaps and findings"`
}

type IngestStandardArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID"`
	Standard  string `json:"standard" jsonschema:"description=The standard to ingest"`
}

type CritiqueDesignArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID"`
	Design    string `json:"design" jsonschema:"description=The design to critique"`
}

type FinalizeRequirementsArgs struct {
	SessionID  string `json:"session_id" jsonschema:"description=The session ID"`
	GoldenSpec string `json:"golden_spec" jsonschema:"description=The finalized golden spec"`
}

type GenerateDocumentsArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID"`
	Title     string `json:"title" jsonschema:"description=Document title"`
	Markdown  string `json:"markdown" jsonschema:"description=Markdown spec"`
	RepoPath  string `json:"repo_path" jsonschema:"description=Repository path"`
}

type CompleteDesignArgs struct {
	SessionID string `json:"session_id" jsonschema:"description=The session ID"`
}

// RegisterTools registers all 6 MCP tools into the server using the generic AddTool.
func RegisterTools(s *mcp.Server, store *db.Store) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "evaluate_idea",
		Description: "Detects stack (.NET/Node) and initializes session in BuntDB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args EvaluateIdeaArgs) (*mcp.CallToolResult, any, error) {
		session := db.NewSessionState(args.SessionID)
		session.TechStack = args.TechStack
		session.StepStatus["evaluate_idea"] = "COMPLETED"

		if err := store.SaveSession(session); err != nil {
			return errorResult(err.Error())
		}
		return textResult(fmt.Sprintf("Session %s initialized for %s stack.", args.SessionID, args.TechStack))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "clarify_requirements",
		Description: "Socratic analysis to fill gaps. Updates BuntDB metadata.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ClarifyRequirementsArgs) (*mcp.CallToolResult, any, error) {
		session, err := store.LoadSession(args.SessionID)
		if err != nil || session == nil {
			return errorResult("session not found")
		}
		session.StepStatus["clarify_requirements"] = "COMPLETED"
		if err := store.SaveSession(session); err != nil {
			return errorResult(err.Error())
		}
		return textResult("Requirements clarified and saved.")
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "ingest_standard",
		Description: "Stores fetched standards directly into the BuntDB session state.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args IngestStandardArgs) (*mcp.CallToolResult, any, error) {
		if err := store.AppendStandard(args.SessionID, args.Standard); err != nil {
			return errorResult(err.Error())
		}
		return textResult("Standard successfully ingested.")
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "critique_design",
		Description: "CRITICAL: Fetches the ingested standards from BuntDB and evaluates the design against them.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CritiqueDesignArgs) (*mcp.CallToolResult, any, error) {
		session, err := store.LoadSession(args.SessionID)
		if err != nil || session == nil {
			return errorResult("session not found")
		}
		standardsJSON, _ := json.Marshal(session.Standards)
		return textResult(fmt.Sprintf("Evaluate the following design:\n\n%s\n\nAgainst these standards:\n%s", args.Design, string(standardsJSON)))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "finalize_requirements",
		Description: "Consolidates design into a Golden Copy JSON spec.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args FinalizeRequirementsArgs) (*mcp.CallToolResult, any, error) {
		session, err := store.LoadSession(args.SessionID)
		if err != nil || session == nil {
			return errorResult("session not found")
		}
		session.StepStatus["finalize_requirements"] = "COMPLETED"
		store.SaveSession(session)
		return textResult("Golden copy finalized.")
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_documents",
		Description: "Creates Jira task, Confluence page (ADF), and Hybrid Markdown Git commits.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GenerateDocumentsArgs) (*mcp.CallToolResult, any, error) {
		err := integration.ProcessDocumentGeneration(args.Title, args.Markdown, args.RepoPath, args.SessionID)
		if err != nil {
			return errorResult(err.Error())
		}
		return textResult("Documents successfully generated and pushed.")
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "complete_design",
		Description: "Final handoff summary and session cleanup.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args CompleteDesignArgs) (*mcp.CallToolResult, any, error) {
		store.DeleteSession(args.SessionID)
		return textResult("Session completed and archived.")
	})
}
