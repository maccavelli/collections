package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// consolidatedTools dynamically bundles polymorphic schemas over legacy discrete functions cleanly natively.
func (rs *MCPRecallServer) consolidatedTools() []toolDef {
	return []toolDef{
		{
			Name:        "search",
			Description: "[DIRECTIVE: Universal Discovery] Evaluates unstructured persistent memory context, architectural standards, and project code ASTs via hybrid arrays. Keywords: query, fuzzy, explore, find, codebase, RAG, history, vector",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"namespace":   { "type": "string", "enum": ["memories", "sessions", "standards", "projects", "ecosystem"], "description": "Routing constraint." },
					"query":       { "type": "string", "description": "Free-text search query mapping BM25." },
					"package":     { "type": "string", "description": "Scoping constraint for AST analysis." },
					"symbol_type": { "type": "string", "description": "Filter by func, struct, interface." },
					"interface":   { "type": "string", "description": "Implements interface restriction." },
					"receiver":    { "type": "string", "description": "Method receiver constraint." },
					"domain":      { "type": "string", "description": "Domain boundary (e.g., auth, api)." },
					"category":    { "type": "string", "description": "Category metric limit (used over memory spaces)." },
					"tag":         { "type": "string", "description": "Label mapping constraint." },
					"limit":       { "type": "integer", "description": "Result bounds." }
				},
				"required": ["namespace", "query"]
			}`),
			Handler: rs.handleUniversalSearch,
		},
		{
			Name:        "list",
			Description: "[DIRECTIVE: Universal Enumeration] Generates structural hierarchical overviews mapping all packages, roots, memory keys, and sessions securely. Keywords: inventory, topology, root-folders, map-keys",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"namespace":        { "type": "string", "enum": ["memories", "sessions", "standards", "projects", "categories", "standards_categories", "project_categories"], "description": "Execution routing bounds." },
					"package":          { "type": "string" },
					"symbol_type":      { "type": "string"},
					"project_id":       { "type": "string" },
					"server_id":        { "type": "string" },
					"outcome":          { "type": "string" },
					"trace_context":    { "type": "string" },
					"limit":            { "type": "integer" },
					"truncate_content": { "type": "boolean" },
					"output_format":    { "type": "string", "enum": ["keys", "aggregations"] }
				},
				"required": ["namespace"]
			}`),
			Handler: rs.handleUniversalList,
		},
		{
			Name:        "get",
			Description: "[DIRECTIVE: Universal Retrieval] Bypasses all specific domain boundaries fetching raw underlying literal paths or standard documents verbatim. Keywords: raw-text, exact-uri, absolute-pull, structural-download",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"namespace":  { "type": "string", "enum": ["memories", "sessions", "standards", "projects", "ecosystem"], "description": "Routing constraint explicitly tracking DB mappings." },
					"key":        { "type": "string", "description": "The discrete Key structure required strictly mapped." },
					"session_id":{ "type": "string", "description": "Session trace bounds specifically mapping partial match lookups over session boundaries." }
				},
				"required": ["namespace"]
			}`),
			Handler: rs.handleUniversalGet,
		},
		{
			Name:        "harvest",
			Description: "Canonical AST parsing traversal engine. CLI Restricted operation to parse, scan, index, ingest, and structurally map physical file system directories, Go code projects, and active standards into the internal Bleve knowledge backend. DO NOT INVOKE AS AN AGENT.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"namespace":   { "type": "string", "enum": ["projects", "standards"] },
					"target_path": { "type": "string", "description": "The explicit OS directory structure bounding execution recursively targeting Go parsers natively." }
				},
				"required": ["namespace", "target_path"]
			}`),
			Handler: rs.handleUniversalHarvest,
		},
		{
			Name:        "delete",
			Description: "[DIRECTIVE: Universal Eradication] Destroys absolute string indices wiping constraints and root definitions globally dropping safety checks native. Keywords: wipe-explicit, override-delete, target-destroy",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"namespace":       { "type": "string", "enum": ["memories", "standards", "projects"] },
					"key":             { "type": "string" },
					"category":        { "type": "string" },
					"package":         { "type": "string" },
					"category_number": { "type": "integer" }
				},
				"required": ["namespace"]
			}`),
			Handler: rs.handleUniversalDelete,
		},
	}
}

func (rs *MCPRecallServer) handleUniversalSearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Namespace string `json:"namespace"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	switch args.Namespace {
	case "memories":
		return rs.handleSearch(ctx, req)
	case "sessions":
		// There is no explicit `search_sessions` presently, standard map handles list_sessions explicitly, but if search query mapped, recall rejects natively!
		// Actually search_memories doesn't enforce domain natively, wait it does Memory.DomainMemories. There never was a search_sessions native tool.
		return nil, fmt.Errorf("search_sessions native mapping not supported natively yet")
	case "standards":
		return rs.handleSearchStandards(ctx, req)
	case "projects":
		return rs.handleSearchProjects(ctx, req)
	case "ecosystem":
		return rs.handleSearchEcosystem(ctx, req)
	default:
		return nil, fmt.Errorf("invalid namespace: %s", args.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalList(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Namespace    string `json:"namespace"`
		OutputFormat string `json:"output_format"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	switch args.Namespace {
	case "memories":
		if args.OutputFormat == "aggregations" {
			return rs.handleListCategories(ctx, req)
		}
		return rs.handleList(ctx, req)
	case "categories":
		return rs.handleListCategories(ctx, req)
	case "sessions":
		return rs.handleListSessions(ctx, req)
	case "standards", "standards_categories":
		return rs.handleListStandardsCategories(ctx, req)
	case "projects", "project_categories":
		return rs.handleListProjectCategories(ctx, req)
	default:
		return nil, fmt.Errorf("invalid namespace for list binding: %s", args.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalGet(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Namespace string `json:"namespace"`
		Key       string `json:"key"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)
	if args.Key == "" && args.Namespace != "sessions" {
		return nil, fmt.Errorf("key strictly required")
	}

	switch args.Namespace {
	case "memories":
		return rs.handleRecall(ctx, req)
	case "sessions":
		return rs.handleGetSessions(ctx, req)
	case "standards":
		return rs.handleGetStandard(ctx, req)
	case "projects":
		return rs.handleGetProject(ctx, req)
	case "ecosystem":
		return rs.handleGetEcosystem(ctx, req)
	default:
		return nil, fmt.Errorf("invalid namespace for get binding: %s", args.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalHarvest(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Namespace string `json:"namespace"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	switch args.Namespace {
	case "standards":
		return rs.handleHarvestStandards(ctx, req)
	case "projects":
		return rs.handleHarvestProjects(ctx, req)
	default:
		return nil, fmt.Errorf("invalid namespace for harvest binding: %s", args.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalDelete(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Namespace string `json:"namespace"`
	}
	_ = json.Unmarshal(req.Params.Arguments, &args)

	switch args.Namespace {
	case "memories":
		return rs.handleDeleteMemories(ctx, req)
	case "standards":
		return rs.handleDeleteStandards(ctx, req)
	case "projects":
		return rs.handleDeleteProjects(ctx, req)
	default:
		return nil, fmt.Errorf("invalid namespace for delete binding: %s", args.Namespace)
	}
}
