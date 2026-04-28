package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/memory"
)

// ecosystemTools returns the tool catalog for aggregated ecosystem-domain retrieval (standards + projects).
func (rs *MCPRecallServer) ecosystemTools() []toolDef {
	return []toolDef{
		{
			Name:        "search_ecosystem",
			Description: "Multi-dimensional aggregated search across harvested standards AND project code. Supports free-text queries with structured filters for package, symbol_type, interface, receiver, and domain. Use this tool for comprehensive RAG intelligence.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query":       { "type": "string", "description": "Free-text search query (BM25 + fuzzy ranking)." },
					"package":     { "type": "string", "description": "Scope search to a specific package path prefix." },
					"symbol_type": { "type": "string", "description": "Filter by symbol type: func, struct, interface, type." },
					"interface":   { "type": "string", "description": "Find symbols implementing a specific interface." },
					"receiver":    { "type": "string", "description": "Find methods bound to a specific receiver type." },
					"domain":      { "type": "string", "description": "Filter by semantic domain: auth, database, api, middleware." },
					"limit":       { "type": "integer", "description": "Max results to return (default: 20)." }
				},
				"required": ["query"]
			}`),
			Handler: rs.handleSearchEcosystem,
		},
		{
			Name:        "get_ecosystem",
			Description: "Fetches a single ecosystem entry by its exact key. Wraps standard Key lookup logic.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"key": { "type": "string", "description": "Exact key of the record to retrieve." }
				},
				"required": ["key"]
			}`),
			Handler: rs.handleGetEcosystem,
		},
	}
}

func (rs *MCPRecallServer) handleSearchEcosystem(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Query      string `json:"query"`
		Package    string `json:"package"`
		SymbolType string `json:"symbol_type"`
		Interface  string `json:"interface"`
		Receiver   string `json:"receiver"`
		Domain     string `json:"domain"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Limit <= 0 {
		args.Limit = 20
	}

	var standardsResults []*memory.SearchResult
	var projectsResults []*memory.SearchResult
	var standardsErr, projectsErr error
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		standardsResults, standardsErr = rs.store.SearchDomain(ctx, memory.DomainStandards, args.Query, args.Package, args.SymbolType, args.Interface, args.Receiver, args.Domain, args.Limit)
	}()

	go func() {
		defer wg.Done()
		projectsResults, projectsErr = rs.store.SearchDomain(ctx, memory.DomainProjects, args.Query, args.Package, args.SymbolType, args.Interface, args.Receiver, args.Domain, args.Limit)
	}()

	wg.Wait()

	if standardsErr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error querying standards: %v", standardsErr)}},
			IsError: true,
		}, nil
	}
	if projectsErr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error querying projects: %v", projectsErr)}},
			IsError: true,
		}, nil
	}

	// Merge exactly as Bleve scored them natively initially, and then sort globally.
	var combined []*memory.SearchResult
	combined = append(combined, standardsResults...)
	combined = append(combined, projectsResults...)

	sort.SliceStable(combined, func(i, j int) bool {
		return combined[i].Score > combined[j].Score
	})

	if len(combined) > args.Limit {
		combined = combined[:args.Limit]
	}

	// Build filter summary for context
	filtersApplied := map[string]string{}
	if args.Package != "" {
		filtersApplied["package"] = args.Package
	}
	if args.SymbolType != "" {
		filtersApplied["symbol_type"] = args.SymbolType
	}
	if args.Interface != "" {
		filtersApplied["interface"] = args.Interface
	}
	if args.Receiver != "" {
		filtersApplied["receiver"] = args.Receiver
	}
	if args.Domain != "" {
		filtersApplied["domain"] = args.Domain
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Ecosystem search for '%s': %d results.\n", args.Query, len(combined)))
	if len(filtersApplied) > 0 {
		sb.WriteString("Filters: ")
		for k, v := range filtersApplied {
			sb.WriteString(fmt.Sprintf("%s=%s ", k, v))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	for _, r := range combined {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n  Key: %s\n", r.Record.Category, r.Key, r.Key))
		if r.Summary != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", r.Summary))
		}
		if len(r.Snippets) > 0 {
			for _, snip := range r.Snippets {
				sb.WriteString(fmt.Sprintf("    ... %s ...\n", strings.TrimSpace(snip)))
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil
}

func (rs *MCPRecallServer) handleGetEcosystem(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	rec, err := rs.store.Get(ctx, args.Key)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Ecosystem record not found: %v", err)}},
			IsError: true,
		}, nil
	}

	if rec.Domain != memory.DomainStandards && rec.Domain != memory.DomainProjects {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' is not in the ecosystem domains (domain: %s).", args.Key, rec.Domain)}},
			IsError: true,
		}, nil
	}

	data, marshalErr := json.MarshalIndent(map[string]interface{}{
		"key":         args.Key,
		"title":       rec.Title,
		"category":    rec.Category,
		"domain":      rec.Domain,
		"tags":        rec.Tags,
		"content":     rec.Content,
		"source_path": rec.SourcePath,
		"source_hash": rec.SourceHash,
		"created_at":  rec.CreatedAt,
		"updated_at":  rec.UpdatedAt,
	}, "", "  ")
	if marshalErr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to marshal record: %v", marshalErr)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil
}
