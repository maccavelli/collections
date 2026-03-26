package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/memory"
)

// MCPRecallServer defines the server that implements the mcp-server-recall tools.
type MCPRecallServer struct {
	mcpServer *mcp.Server
	store    *memory.MemoryStore
}

// NewMCPRecallServer creates a new instance of the recall server.
func NewMCPRecallServer(name, version string, store *memory.MemoryStore) (*MCPRecallServer, error) {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    name,
			Version: version,
		},
		nil,
	)

	rs := &MCPRecallServer{
		mcpServer: s,
		store:    store,
	}

	rs.registerTools()

	return rs, nil
}

// registerTools defines and registers all the MCP tools for this server.
func (rs *MCPRecallServer) registerTools() {
	// 1. remember
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "remember",
		Description: "PERSISTENCE MANDATE: Primary gateway for long-term knowledge retention. Call this to store architectural decisions, workflow patterns, or critical context with optional tags. Cascades to search_memories for verification.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"key": {
					"type": "string",
					"description": "Unique identifier (e.g., 'auth-strategy')."
				},
				"value": {
					"type": "string",
					"description": "Content to store."
				},
				"tags": {
					"type": "array",
					"items": { "type": "string" },
					"description": "Optional categories (e.g., 'architecture', 'context', 'lesson')."
				},
				"category": {
					"type": "string",
					"description": "Primary classification for the memory (e.g., 'workflow', 'architecture')."
				}
			},
			"required": ["key", "value"]
		}`),
	}, rs.handleRemember)

	// 2. recall
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "recall",
		Description: "PRECISION RETRIEVAL: Targeted extraction of a specific knowledge entry by its unique key. Call this when you have an exact reference from list_memories or search_memories to fetch full, unmodified details.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"key": {
					"type": "string",
					"description": "Key of the memory to retrieve."
				}
			},
			"required": ["key"]
		}`),
	}, rs.handleRecall)

	// 3. search_memories
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "search_memories",
		Description: "FUZZY DISCOVERY: AI-optimized keyword and tag-based search across all memories. Call this for knowledge exploration, semantic matching, or when exact keys are unknown. Results are ranked by relevance score.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "Fuzzy keyword to look for in keys, content, and tags. Partial matches and typos are supported."
				},
				"tag": {
					"type": "string",
					"description": "Optional filter for a specific tag."
				},
				"limit": {
					"type": "integer",
					"description": "Maximum number of results to return.",
					"default": 20
				}
			}
		}`),
	}, rs.handleSearch)

	// 4. recall_recent (NEW)
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "recall_recent",
		Description: "CHRONOLOGICAL AUDIT: Rapid context recovery tool. Call this immediately after an IDE restart or session interruption to synchronize with the most recent developments. Cascades to list_memories for broader history.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"count": {
					"type": "integer",
					"description": "Number of recent memories to retrieve.",
					"default": 10
				}
			}
		}`),
	}, rs.handleRecallRecent)

	// 5. list_memories
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "list_memories",
		Description: "CATALOG EXPLORATION: Comprehensive index of stored knowledge. Call this to map available keys and metadata summaries for initial system orientation and knowledge discovery. Cascades to recall for full details.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleList)

	// 6. memory_stats (NEW)
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_status",
		Description: "HEALTH DIAGNOSTIC: Infrastructure-level telemetry for the memory store. Call this to monitor entry counts and storage footprints before large-scale ingestion or batch operations.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleStats)

	// 7. forget
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "forget",
		Description: "HYGIENE MANDATE: Targeted pruning of obsolete or incorrect knowledge. Call this to remove specific entries that have been superseded or are no longer relevant to the current project lifecycle.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"key": {
					"type": "string",
					"description": "Key of the memory to delete."
				}
			},
			"required": ["key"]
		}`),
	}, rs.handleForget)

	// 8. clear_all (NEW)
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "clear_all_memories",
		Description: "SYSTEM PURGE: Catastrophic reset of the entire knowledge ecosystem. Call this ONLY when an absolute clean slate is required for a completely new environment. Use with extreme caution.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleClearAll)

	// 9. consolidate_memories (AUTO-CLUSTERING)
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "consolidate_memories",
		Description: "ENTROPY OPTIMIZATION: Intelligent de-duplication and merging engine. Call this to reduce noise and increase knowledge density by clustering semantically similar entries into unified, high-value records.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"similarity_threshold": {
					"type": "number",
					"description": "Threshold for redundancy clustering (0.0 to 1.0). Default 0.8.",
					"default": 0.8
				},
				"dry_run": {
					"type": "boolean",
					"description": "If true, only report what WOULD be merged without making changes.",
					"default": false
				}
			}
		}`),
	}, rs.handleConsolidate)

	// 10. list_categories (NEW)
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "list_categories",
		Description: "TAXONOMY AUDIT: Returns a unique list of all memory categories current stored in the database. Call this for organized discovery and filtered searches across the knowledge base.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleListCategories)

	slog.Info("All hardened and featured tools registered")
}

func (rs *MCPRecallServer) handleConsolidate(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Threshold float64 `json:"similarity_threshold"`
		DryRun    bool    `json:"dry_run"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Threshold <= 0 {
		args.Threshold = 0.8
	}

	count, merged, err := rs.store.Consolidate(ctx, args.Threshold, args.DryRun)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Consolidation error: %v", err)}},
			IsError: true,
		}, nil
	}

	status := "Consolidation Complete"
	if args.DryRun {
		status = "Consolidation Dry Run (No changes applied)"
	}

	msg := fmt.Sprintf("%s:\n- Clusters Merged: %d\n- Redundant Keys Removed: %d\n- Removed Keys: %v", 
		status, count, len(merged), merged)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, nil
}

func (rs *MCPRecallServer) handleRemember(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key      string   `json:"key"`
		Value    string   `json:"value"`
		Category string   `json:"category"`
		Tags     []string `json:"tags"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if err := rs.store.Save(ctx, args.Key, args.Value, args.Category, args.Tags); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Memory for '%s' saved with tags %v.", args.Key, args.Tags)}},
	}, nil
}

func (rs *MCPRecallServer) handleRecall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	rec, err := rs.store.Get(ctx, args.Key)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	out, _ := json.MarshalIndent(rec, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Memory Found:\nKey: %s\n%s", args.Key, string(out))}},
	}, nil
}

func (rs *MCPRecallServer) handleSearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Query string `json:"query"`
		Tag   string `json:"tag"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Limit <= 0 {
		args.Limit = 20
	}

	results, err := rs.store.Search(ctx, args.Query, args.Tag, args.Limit)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults("Search Results", results)
}

func (rs *MCPRecallServer) handleRecallRecent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Count <= 0 {
		args.Count = 10
	}

	results, err := rs.store.GetRecent(ctx, args.Count)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults("Recent Context Memories", results)
}

func (rs *MCPRecallServer) handleList(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	keys, err := rs.store.ListKeys(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults("Knowledge Index", keys)
}

func (rs *MCPRecallServer) handleStats(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	count, size, err := rs.store.GetStats()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Store Statistics:\n- Entry Count: %d\n- Estimated Size: %.2f KB", count, float64(size)/1024)}},
	}, nil
}

func (rs *MCPRecallServer) handleForget(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if err := rs.store.Delete(ctx, args.Key); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Memory for '%s' forgotten.", args.Key)}},
	}, nil
}

func (rs *MCPRecallServer) handleClearAll(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := rs.store.Clear(ctx); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error clearing store: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "All context memories have been permanently cleared."}},
	}, nil
}

func (rs *MCPRecallServer) handleListCategories(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	categories, err := rs.store.ListCategories(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error fetching categories: %v", err)}},
			IsError: true,
		}, nil
	}

	out, _ := json.MarshalIndent(categories, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Stored Categories:\n%s", string(out))}},
	}, nil
}

func (rs *MCPRecallServer) formatResults(title string, results []*memory.SearchResult) (*mcp.CallToolResult, error) {
	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No matches found."}},
		}, nil
	}

	// Prepare the hybrid JSON-Markdown response
	type Response struct {
		Title   string                 `json:"title"`
		Summary map[string]interface{} `json:"search_summary"`
		Results []*memory.SearchResult  `json:"results"`
	}

	maxScore := 0
	avgScore := 0
	
	// Concurrency: Use goroutines if the result set is particularly large, but for typically <100 entries, it's fast.
	for _, res := range results {
		if res.Score > maxScore {
			maxScore = res.Score
		}
		avgScore += res.Score

		// Token Economy: Implement automatic summarization/truncation
		// If content is large (>500 chars), provide a summary and flag for retrieval
		if res.Record != nil && len(res.Record.Content) > 500 {
			res.Summary = res.Record.Content[:497] + "..."
			res.IsTruncated = true
			res.Record.Content = "" 
		}
	}
	if len(results) > 0 {
		avgScore /= len(results)
	}

	resp := Response{
		Title: title,
		Summary: map[string]interface{}{
			"count":         len(results),
			"highest_relevance": maxScore,
			"average_relevance": avgScore,
			"timestamp":     time.Now().Format(time.RFC3339),
		},
		Results: results,
	}

	out, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search results: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(out)}},
	}, nil
}

// Serve initializes the transport and starts serving the MCP protocol.
func (rs *MCPRecallServer) Serve(ctx context.Context) error {
	slog.Info("Serving MCP-Recall on stdio transport")
	t := &mcp.StdioTransport{}
	_, err := rs.mcpServer.Connect(ctx, t, nil)
	return err
}
