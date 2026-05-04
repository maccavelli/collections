package server

import (
	"context"

	"fmt"
	"mcp-server-recall/internal/memory"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// handleIngestFiles proxies the user path down to the concurrent memory dispatcher.
func (rs *MCPRecallServer) handleIngestFiles(ctx context.Context, req *mcp.CallToolRequest, args IngestFilesInput) (*mcp.CallToolResult, any, error) {

	if args.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: path is required"}},
			IsError: true,
		}, nil, nil
	}

	storedCount, err := rs.store.ProcessPath(ctx, args.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error during ingest: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	summary := fmt.Sprintf("Ingestion Complete: Processed %s, generated %d memory clips.", args.Path, storedCount)
	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]any{
				"message": summary,
				"path":    args.Path,
				"clips":   storedCount,
			},
		},
	}, nil, nil
}

// handleDeleteMemories processes dual-mode deletions natively.
func (rs *MCPRecallServer) handleDeleteMemories(ctx context.Context, _ *mcp.CallToolRequest, args DeleteMemoriesInput) (*mcp.CallToolResult, any, error) {

	if !args.All && strings.TrimSpace(args.Key) == "" && strings.TrimSpace(args.Category) == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: must specify either 'key', 'category', or explicitly set 'all' to true"}},
			IsError: true,
		}, nil, nil
	}

	var summary string
	var err error

	if args.Category != "" {
		deletedCount, catErr := rs.store.DeleteByCategory(ctx, args.Category)
		if catErr != nil {
			err = catErr
		} else {
			summary = fmt.Sprintf("Deleted %d memories associated with category '%s'.", deletedCount, args.Category)
		}
	} else if args.Key != "" {
		if keyErr := rs.store.Delete(ctx, args.Key); keyErr != nil {
			err = keyErr
		} else {
			summary = fmt.Sprintf("Deleted memory '%s'.", args.Key)
		}
	} else if args.All {
		deletedCount, allErr := rs.store.DeleteDomain(ctx, memory.DomainMemories)
		if allErr != nil {
			err = allErr
		} else {
			summary = fmt.Sprintf("Deleted ALL %d memory records.", deletedCount)
		}
	}

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error deleting memories: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]string{
				"message": summary,
			},
		},
	}, nil, nil
}

// handleDeleteSessions processes session deletion locally or globally.
func (rs *MCPRecallServer) handleDeleteSessions(ctx context.Context, _ *mcp.CallToolRequest, args DeleteSessionsInput) (*mcp.CallToolResult, any, error) {

	if !args.All && strings.TrimSpace(args.Key) == "" && strings.TrimSpace(args.SessionID) == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: must specify either 'key', 'session_id', or explicitly set 'all' to true"}},
			IsError: true,
		}, nil, nil
	}

	var summary string
	var err error

	if args.All {
		deletedCount, allErr := rs.store.DeleteDomain(ctx, memory.DomainSessions)
		if allErr != nil {
			err = allErr
		} else {
			summary = fmt.Sprintf("Deleted ALL %d session records.", deletedCount)
		}
	} else {
		// NOTE: if key is specified, just use rs.store.Delete
		if args.Key != "" {
			if keyErr := rs.store.Delete(ctx, args.Key); keyErr != nil {
				err = keyErr
			} else {
				summary = fmt.Sprintf("Deleted session '%s'.", args.Key)
			}
		} else if args.SessionID != "" {
			// fallback if they only pass session_id
			if keyErr := rs.store.Delete(ctx, args.SessionID); keyErr != nil {
				err = keyErr
			} else {
				summary = fmt.Sprintf("Deleted session '%s'.", args.SessionID)
			}
		}
	}

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error deleting sessions: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]string{
				"message": summary,
			},
		},
	}, nil, nil
}
