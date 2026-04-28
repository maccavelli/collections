package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// handleIngestFiles proxies the user path down to the concurrent memory dispatcher.
func (rs *MCPRecallServer) handleIngestFiles(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Path == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: path is required"}},
			IsError: true,
		}, nil
	}

	storedCount, err := rs.store.ProcessPath(ctx, args.Path)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error during ingest: %v", err)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Ingestion Complete: Processed %s, generated %d memory clips.", args.Path, storedCount)
	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"message": summary,
				"path":    args.Path,
				"clips":   storedCount,
			},
		},
	}, nil
}

// handleDeleteMemories processes dual-mode deletions natively.
func (rs *MCPRecallServer) handleDeleteMemories(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key      string `json:"key"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if strings.TrimSpace(args.Key) == "" && strings.TrimSpace(args.Category) == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: must specify either 'key' or 'category'"}},
			IsError: true,
		}, nil
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
	}

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error deleting memories: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data": map[string]string{
				"message": summary,
			},
		},
	}, nil
}
