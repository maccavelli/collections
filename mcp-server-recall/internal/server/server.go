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
		Description: "Saves or updates information, architectural decisions, or context with optional tags.",
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
				}
			},
			"required": ["key", "value"]
		}`),
	}, rs.handleRemember)

	// 2. recall
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "recall",
		Description: "Retrieves a specific memory and its metadata.",
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
		Description: "Keyword or tag-based search across all memories.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "Keyword to look for in keys, content, and tags."
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
		Description: "Retrieves the most recently updated memories. Excellent for resuming context after an IDE restart.",
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
		Description: "Lists all stored memory keys with metadata summaries for knowledge discovery.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleList)

	// 6. memory_stats (NEW)
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "memory_status",
		Description: "Provides database usage statistics like total entry count and storage size.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleStats)

	// 7. forget
	rs.mcpServer.AddTool(&mcp.Tool{
		Name:        "forget",
		Description: "Permanently deletes a memory.",
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
		Description: "Permanently wipes all memories from the store. Use with extreme caution.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
	}, rs.handleClearAll)

	slog.Info("All hardened and featured tools registered")
}

func (rs *MCPRecallServer) handleRemember(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key   string   `json:"key"`
		Value string   `json:"value"`
		Tags  []string `json:"tags"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if err := rs.store.Save(ctx, args.Key, args.Value, args.Tags); err != nil {
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

	if len(keys) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Memory index is empty."}},
		}, nil
	}

	out := fmt.Sprintf("Knowledge Index (%d entries):\n", len(keys))
	for k, v := range keys {
		tagStr := ""
		if len(v.Tags) > 0 {
			tagStr = fmt.Sprintf(" (Tags: %v)", v.Tags)
		}
		out += fmt.Sprintf("- %s%s\n", k, tagStr)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: out}},
	}, nil
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

func (rs *MCPRecallServer) formatResults(title string, results map[string]*memory.Record) (*mcp.CallToolResult, error) {
	if len(results) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "No matches found."}},
		}, nil
	}

	out := title + ":\n"
	for k, v := range results {
		tagString := ""
		if len(v.Tags) > 0 {
			tagString = fmt.Sprintf(" [%v]", v.Tags)
		}
		summary := v.Content
		if len(summary) > 120 {
			summary = summary[:117] + "..."
		}
		out += fmt.Sprintf("- %s: %s%s (Updated: %s)\n", k, summary, tagString, v.UpdatedAt.Format(time.RFC3339))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: out}},
	}, nil
}

// Serve initializes the transport and starts serving the MCP protocol.
func (rs *MCPRecallServer) Serve(ctx context.Context) error {
	slog.Info("Serving MCP-Recall on stdio transport")
	t := &mcp.StdioTransport{}
	_, err := rs.mcpServer.Connect(ctx, t, nil)
	return err
}
