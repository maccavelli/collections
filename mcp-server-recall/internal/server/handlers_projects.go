package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/memory"
)

// projectsTools returns the tool catalog for projects-domain retrieval.
func (rs *MCPRecallServer) projectsTools() []toolDef {
	return []toolDef{
		{
			Name:        "list_project_categories",
			Description: "Returns a hierarchical, package-grouped overview of all harvested project data. Shows packages, symbol counts by type, individual symbol names, package doc availability, and API drift checksums. Scoped exclusively to the projects domain.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"package": { "type": "string", "description": "Optional package path prefix to filter results." },
					"symbol_type": { "type": "string", "description": "Optional filter by symbol type: func, struct, interface." }
				}
			}`),
			Handler: rs.handleListProjectCategories,
		},
		{
			Name:        "search_projects",
			Description: "Multi-dimensional search across harvested project code. Supports free-text queries with structured filters for package, symbol_type, interface, receiver, and domain. Returns structured symbol metadata. Completely isolated from standards and general memories.",
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
			Handler: rs.handleSearchProjects,
		},
		{
			Name:        "get_project",
			Description: "Fetches a single project entry by its exact key with fully parsed metadata. Scoped exclusively to the projects namespace. Use keys from list_project_categories or search_projects for precision lookups.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"key": { "type": "string", "description": "Exact key of the project record to retrieve (e.g. 'pkg:mcp-server-magicskills/internal/server:MCPServer')." }
				},
				"required": ["key"]
			}`),
			Handler: rs.handleGetProject,
		},
		{
			Name:        "delete_projects",
			Description: "Deletes projects by category name (e.g. HarvestedCode), exact package name, or category number (matching list_project_categories output). Deleting by category wipes all packages within it; deleting by package name targets that package.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"category": { "type": "string", "description": "Delete all projects matching this category name (e.g. HarvestedCode, PackageDoc)." },
					"package": { "type": "string", "description": "Delete all projects under this specific package path prefix." },
					"category_number": { "type": "integer", "description": "Delete package corresponding to this 1-based index from the alphabetically sorted packages list." }
				}
			}`),
			Handler: rs.handleDeleteProjects,
		},
	}
}

func (rs *MCPRecallServer) handleListProjectCategories(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Package    string `json:"package"`
		SymbolType string `json:"symbol_type"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	packages, err := rs.store.ListDomainOverview(ctx, memory.DomainProjects, args.Package)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	// Post-filter by symbol type if requested
	if args.SymbolType != "" {
		for pkgPath, pkg := range packages {
			var filtered []memory.StandardsSymbolSummary
			for _, sym := range pkg.Symbols {
				if sym.SymbolType == args.SymbolType {
					filtered = append(filtered, sym)
				}
			}
			if len(filtered) == 0 {
				delete(packages, pkgPath)
			} else {
				pkg.Symbols = filtered
				pkg.TotalSymbols = len(filtered)
			}
		}
	}

	// Build summary stats
	totalPkgs := len(packages)
	totalSyms := 0
	totalDocs := 0
	for _, pkg := range packages {
		totalSyms += pkg.TotalSymbols
		if pkg.HasPackageDoc {
			totalDocs++
		}
	}

	// Build numbered listing
	var pkgNames []string
	for p := range packages {
		pkgNames = append(pkgNames, p)
	}
	sort.Strings(pkgNames)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Projects overview: %d packages, %d symbols, %d package docs.\n\n", totalPkgs, totalSyms, totalDocs))
	for i, name := range pkgNames {
		pkg := packages[name]
		docFlag := ""
		if pkg.HasPackageDoc {
			docFlag = " [doc]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%d symbols)%s\n", i+1, name, pkg.TotalSymbols, docFlag))
		for _, sym := range pkg.Symbols {
			sb.WriteString(fmt.Sprintf("   - [%s] %s\n", sym.SymbolType, sym.Name))
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil
}

func (rs *MCPRecallServer) handleSearchProjects(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	results, err := rs.store.SearchDomain(ctx, memory.DomainProjects, args.Query, args.Package, args.SymbolType, args.Interface, args.Receiver, args.Domain, args.Limit)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
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
	sb.WriteString(fmt.Sprintf("Projects search for '%s': %d results.\n", args.Query, len(results)))
	if len(filtersApplied) > 0 {
		sb.WriteString("Filters: ")
		for k, v := range filtersApplied {
			sb.WriteString(fmt.Sprintf("%s=%s ", k, v))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	for _, r := range results {
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

func (rs *MCPRecallServer) handleGetProject(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	rec, err := rs.store.Get(ctx, args.Key)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Project record not found: %v", err)}},
			IsError: true,
		}, nil
	}

	// Verify this is a projects record
	if rec.Domain != memory.DomainProjects {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' is not a projects record (domain: %s). Use 'get_standards' for standards or 'recall' for memories.", args.Key, rec.Domain)}},
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
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to marshal project record: %v", marshalErr)}},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil
}

func (rs *MCPRecallServer) handleDeleteProjects(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Category       string `json:"category"`
		Package        string `json:"package"`
		CategoryNumber int    `json:"category_number"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Category == "" && args.Package == "" && args.CategoryNumber <= 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: must specify either category, package, or category_number"}},
			IsError: true,
		}, nil
	}

	pkgFilter := args.Package

	if args.CategoryNumber > 0 {
		packagesMap, err := rs.store.ListDomainOverview(ctx, memory.DomainProjects, "")
		if err != nil {
			return nil, fmt.Errorf("failed to list projects overview: %w", err)
		}
		var pkgNames []string
		for p := range packagesMap {
			pkgNames = append(pkgNames, p)
		}
		sort.Strings(pkgNames)

		if args.CategoryNumber > len(pkgNames) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: category_number %d is out of bounds (max %d)", args.CategoryNumber, len(pkgNames))}},
				IsError: true,
			}, nil
		}
		pkgFilter = pkgNames[args.CategoryNumber-1]
	}

	deletedCount, err := rs.store.DeleteProjects(ctx, args.Category, pkgFilter)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error deleting projects: %v", err)}},
			IsError: true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Deleted %d project records.", deletedCount))
	if args.Category != "" {
		sb.WriteString(fmt.Sprintf(" Category: %s.", args.Category))
	}
	if pkgFilter != "" {
		sb.WriteString(fmt.Sprintf(" Package: %s.", pkgFilter))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil
}
