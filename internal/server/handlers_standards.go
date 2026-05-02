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

// standardsTools returns the tool catalog for standards-domain retrieval.

func (rs *MCPRecallServer) handleListStandardsCategories(ctx context.Context, _ *mcp.CallToolRequest, args ListStandardsCategoriesInput) (*mcp.CallToolResult, any, error) {

	packages, err := rs.store.ListStandardsOverview(ctx, args.Package)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
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

	// Build numbered listing for category_number reference
	var pkgNames []string
	for p := range packages {
		pkgNames = append(pkgNames, p)
	}
	sort.Strings(pkgNames)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Standards overview: %d packages, %d symbols, %d package docs.\n\n", totalPkgs, totalSyms, totalDocs))
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
	}, nil, nil
}

func (rs *MCPRecallServer) handleSearchStandards(ctx context.Context, req *mcp.CallToolRequest, args SearchStandardsInput) (*mcp.CallToolResult, any, error) {

	if args.Limit <= 0 {
		args.Limit = 20
	}

	results, err := rs.store.SearchStandards(ctx, args.Query, args.Package, args.SymbolType, args.Interface, args.Receiver, args.Domain, args.Limit)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
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
	sb.WriteString(fmt.Sprintf("Standards search for '%s': %d results.\n", args.Query, len(results)))
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
	}, nil, nil
}

func (rs *MCPRecallServer) handleGetStandard(ctx context.Context, _ *mcp.CallToolRequest, args GetStandardInput) (*mcp.CallToolResult, any, error) {

	rec, err := rs.store.Get(ctx, args.Key)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Standard not found: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	// Verify this is a standards record
	if !memory.HarvestedCategories[rec.Category] {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' is not a standards record (category: %s). Use 'recall' for memories.", args.Key, rec.Category)}},
			IsError: true,
		}, nil, nil
	}

	data, marshalErr := json.MarshalIndent(map[string]any{
		"key":         args.Key,
		"title":       rec.Title,
		"category":    rec.Category,
		"tags":        rec.Tags,
		"content":     rec.Content,
		"source_path": rec.SourcePath,
		"source_hash": rec.SourceHash,
		"created_at":  rec.CreatedAt,
		"updated_at":  rec.UpdatedAt,
	}, "", "  ")
	if marshalErr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to marshal standard: %v", marshalErr)}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

func (rs *MCPRecallServer) handleDeleteStandards(ctx context.Context, _ *mcp.CallToolRequest, args DeleteStandardsInput) (*mcp.CallToolResult, any, error) {

	if !args.All && args.Category == "" && args.Package == "" && args.CategoryNumber <= 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: must specify category, package, category_number, or explicitly set 'all' to true"}},
			IsError: true,
		}, nil, nil
	}

	pkgFilter := args.Package

	if args.CategoryNumber > 0 {
		packagesMap, err := rs.store.ListStandardsOverview(ctx, "")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list standards: %w", err)
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
			}, nil, nil
		}
		pkgFilter = pkgNames[args.CategoryNumber-1]
	}

	deletedCount, err := rs.store.DeleteStandards(ctx, args.Category, pkgFilter)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error deleting standards: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Deleted %d standard records.", deletedCount))
	if args.Category != "" {
		sb.WriteString(fmt.Sprintf(" Category: %s.", args.Category))
	}
	if pkgFilter != "" {
		sb.WriteString(fmt.Sprintf(" Package: %s.", pkgFilter))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
	}, nil, nil
}
