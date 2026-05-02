package server

import (
	"context"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UniversalSearchInput defines parameters for multi-domain search.
type UniversalSearchInput struct {
	Namespace  string `json:"namespace" jsonschema:"Target domain. One of: memories, sessions, standards, projects, ecosystem."`
	Query      string `json:"query" jsonschema:"Free-text search query mapping BM25."`
	Package    string `json:"package,omitempty" jsonschema:"Scoping constraint for AST analysis."`
	SymbolType string `json:"symbol_type,omitempty" jsonschema:"Filter by func, struct, interface."`
	Interface  string `json:"interface,omitempty" jsonschema:"Implements interface restriction."`
	Receiver   string `json:"receiver,omitempty" jsonschema:"Method receiver constraint."`
	Domain     string `json:"domain,omitempty" jsonschema:"Domain boundary (e.g., auth, api)."`
	Category   string `json:"category,omitempty" jsonschema:"Category metric limit (used over memory spaces)."`
	Tag          string `json:"tag,omitempty" jsonschema:"Label mapping constraint."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Result bounds."`
	ProjectID    string `json:"project_id,omitempty"`
	ServerID     string `json:"server_id,omitempty"`
	Outcome      string `json:"outcome,omitempty"`
	TraceContext string `json:"trace_context,omitempty"`
}

// UniversalListInput defines parameters for multi-domain enumeration.
type UniversalListInput struct {
	Namespace       string `json:"namespace" jsonschema:"Target domain. One of: memories, sessions, standards, projects, categories, standards_categories, project_categories."`
	Package         string `json:"package,omitempty"`
	SymbolType      string `json:"symbol_type,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
	ServerID        string `json:"server_id,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	TraceContext    string `json:"trace_context,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	TruncateContent bool   `json:"truncate_content,omitempty"`
	OutputFormat    string `json:"output_format,omitempty" jsonschema:"Output format. One of: keys, aggregations."`
}

// UniversalGetInput defines parameters for exact key retrieval across domains.
type UniversalGetInput struct {
	Namespace string `json:"namespace" jsonschema:"Target domain. One of: memories, sessions, standards, projects, ecosystem."`
	Key       string `json:"key,omitempty" jsonschema:"The discrete key to retrieve."`
	SessionID string `json:"session_id,omitempty" jsonschema:"Session trace ID for partial match lookups over session boundaries."`
}

// UniversalHarvestInput defines parameters for AST extraction.
type UniversalHarvestInput struct {
	Namespace  string `json:"namespace" jsonschema:"Target domain. One of: projects, standards."`
	TargetPath string `json:"target_path" jsonschema:"Absolute OS directory path to recursively harvest Go source from."`
}

// UniversalDeleteInput defines parameters for explicit node destruction.
type UniversalDeleteInput struct {
	Namespace      string `json:"namespace" jsonschema:"Target domain. One of: memories, standards, projects."`
	Key            string `json:"key,omitempty"`
	Category       string `json:"category,omitempty"`
	Package        string `json:"package,omitempty"`
	CategoryNumber int    `json:"category_number,omitempty"`
	All            bool   `json:"all,omitempty" jsonschema:"Set to true to explicitly confirm deletion of ALL records in the specified namespace or category."`
}


func (rs *MCPRecallServer) handleUniversalSearch(ctx context.Context, req *mcp.CallToolRequest, input UniversalSearchInput) (*mcp.CallToolResult, any, error) {
	switch input.Namespace {
	case "memories":
		return rs.handleSearch(ctx, req, SearchMemoriesInput{
			Query: input.Query,
			Tag:   input.Tag,
			Limit: input.Limit,
		})
	case "sessions":
		return rs.handleSearchSessions(ctx, req, SearchSessionsInput{
			Query:        input.Query,
			ProjectID:    input.ProjectID,
			ServerID:     input.ServerID,
			Outcome:      input.Outcome,
			TraceContext: input.TraceContext,
			Limit:        input.Limit,
		})
	case "standards":
		return rs.handleSearchStandards(ctx, req, SearchStandardsInput{
			Query: input.Query, Package: input.Package, SymbolType: input.SymbolType, Interface: input.Interface, Receiver: input.Receiver, Domain: input.Domain, Limit: input.Limit,
		})
	case "projects":
		return rs.handleSearchProjects(ctx, req, SearchProjectsInput{
			Query: input.Query, Package: input.Package, SymbolType: input.SymbolType, Interface: input.Interface, Receiver: input.Receiver, Domain: input.Domain, Limit: input.Limit,
		})
	case "ecosystem":
		return rs.handleSearchEcosystem(ctx, req, SearchEcosystemInput{
			Query: input.Query, Package: input.Package, SymbolType: input.SymbolType, Interface: input.Interface, Receiver: input.Receiver, Domain: input.Domain, Limit: input.Limit,
		})
	default:
		return nil, nil, fmt.Errorf("invalid namespace: %s", input.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalList(ctx context.Context, req *mcp.CallToolRequest, input UniversalListInput) (*mcp.CallToolResult, any, error) {
	switch input.Namespace {
	case "memories":
		if input.OutputFormat == "aggregations" {
			return rs.handleListCategories(ctx, req, ListCategoriesInput{})
		}
		return rs.handleList(ctx, req, ListMemoriesInput{})
	case "categories":
		return rs.handleListCategories(ctx, req, ListCategoriesInput{})
	case "sessions":
		return rs.handleListSessions(ctx, req, ListSessionsInput{
			ProjectID:       input.ProjectID,
			ServerID:        input.ServerID,
			Outcome:         input.Outcome,
			TraceContext:    input.TraceContext,
			Limit:           input.Limit,
			TruncateContent: input.TruncateContent,
		})
	case "standards", "standards_categories":
		return rs.handleListStandardsCategories(ctx, req, ListStandardsCategoriesInput{
			Package: input.Package, SymbolType: input.SymbolType,
		})
	case "projects", "project_categories":
		return rs.handleListProjectCategories(ctx, req, ListProjectCategoriesInput{
			Package: input.Package, SymbolType: input.SymbolType,
		})
	default:
		return nil, nil, fmt.Errorf("invalid namespace for list binding: %s", input.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalGet(ctx context.Context, req *mcp.CallToolRequest, input UniversalGetInput) (*mcp.CallToolResult, any, error) {
	if input.Key == "" && input.Namespace != "sessions" {
		return nil, nil, fmt.Errorf("key strictly required")
	}

	switch input.Namespace {
	case "memories":
		return rs.handleRecall(ctx, req, RecallInput{Key: input.Key})
	case "sessions":
		return rs.handleGetSessions(ctx, req, GetSessionsInput{Key: input.Key, SessionID: input.SessionID})
	case "standards":
		return rs.handleGetStandard(ctx, req, GetStandardInput{Key: input.Key})
	case "projects":
		return rs.handleGetProject(ctx, req, GetProjectInput{Key: input.Key})
	case "ecosystem":
		return rs.handleGetEcosystem(ctx, req, GetEcosystemInput{Key: input.Key})
	default:
		return nil, nil, fmt.Errorf("invalid namespace for get binding: %s", input.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalHarvest(ctx context.Context, req *mcp.CallToolRequest, input UniversalHarvestInput) (*mcp.CallToolResult, any, error) {
	switch input.Namespace {
	case "standards":
		return rs.handleHarvestStandards(ctx, req, HarvestStandardsInput{TargetPath: input.TargetPath})
	case "projects":
		return rs.handleHarvestProjects(ctx, req, HarvestProjectsInput{TargetPath: input.TargetPath})
	default:
		return nil, nil, fmt.Errorf("invalid namespace for harvest binding: %s", input.Namespace)
	}
}

func (rs *MCPRecallServer) handleUniversalDelete(ctx context.Context, req *mcp.CallToolRequest, input UniversalDeleteInput) (*mcp.CallToolResult, any, error) {
	switch input.Namespace {
	case "memories":
		return rs.handleDeleteMemories(ctx, req, DeleteMemoriesInput{Key: input.Key, Category: input.Category, All: input.All})
	case "standards":
		return rs.handleDeleteStandards(ctx, req, DeleteStandardsInput{Category: input.Category, Package: input.Package, CategoryNumber: input.CategoryNumber, All: input.All})
	case "projects":
		return rs.handleDeleteProjects(ctx, req, DeleteProjectsInput{Category: input.Category, Package: input.Package, CategoryNumber: input.CategoryNumber, All: input.All})
	case "sessions":
		return rs.handleDeleteSessions(ctx, req, DeleteSessionsInput{Key: input.Key, SessionID: input.Key, All: input.All})
	default:
		return nil, nil, fmt.Errorf("invalid namespace for delete binding: %s", input.Namespace)
	}
}


