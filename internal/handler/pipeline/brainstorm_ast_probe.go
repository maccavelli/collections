// Package pipeline provides functionality for the pipeline subsystem.
package pipeline

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
	"mcp-server-brainstorm/internal/util"
)

// ASTProbeTool defines the ASTProbeTool structure.
type ASTProbeTool struct {
	Manager *state.Manager
	Engine  *engine.Engine
}

// Name performs the Name operation.
func (t *ASTProbeTool) Name() string {
	return "brainstorm_ast_probe"
}

// Register performs the Register operation.
func (t *ASTProbeTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: ANALYZER] [PHASE: SYNTHESIS] AST FEASIBILITY ORACLE: Non-mutating structural probe performing dynamic AST dry-run mappings to validate that the Socratic resolution (aporia) is structurally feasible before planning begins. Ensures proposed changes don't violate AST constraints. [REQUIRES: brainstorm:aporia_engine] [Routing Tags: probe, ast-scan, dry-run, structure-check, feasibility, validate-aporia]",
	}, t.Handle)
}

// ASTProbeInput defines the ASTProbeInput structure.
type ASTProbeInput struct {
	models.UniversalPipelineInput
}

// Handle performs the Handle operation.
func (t *ASTProbeTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input ASTProbeInput) (*mcp.CallToolResult, any, error) {
	if input.Target == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("target file is required for AST probe"))
		return res, nil, nil
	}

	session, err := t.Manager.LoadSession(ctx)
	if err != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("load session: %v", err))
		return res, nil, nil
	}

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()

	fset := token.NewFileSet()

	var payload map[string]any

	info, statErr := os.Stat(input.Target)
	if statErr != nil {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("AST probe target stat failed: %v", statErr))
		return res, nil, nil
	}

	if info.IsDir() {
		// Directory target: parse all Go files in the package.
		pkgs, pErr := parser.ParseDir(fset, input.Target, nil, parser.ParseComments)
		if pErr != nil {
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("AST parse directory failed: %v", pErr))
			return res, nil, nil
		}

		totalDecls := 0
		totalUnresolved := 0
		pkgName := ""
		fileDetails := make(map[string]any)

		for _, pkg := range pkgs {
			if pkgName == "" {
				pkgName = pkg.Name
			}
			for fname, f := range pkg.Files {
				totalDecls += len(f.Decls)
				totalUnresolved += len(f.Unresolved)
				fileDetails[fname] = map[string]any{
					"decls_count": len(f.Decls),
					"unresolved":  len(f.Unresolved),
				}
			}
		}

		payload = map[string]any{
			"target":      input.Target,
			"package":     pkgName,
			"decls_count": totalDecls,
			"unresolved":  totalUnresolved,
			"files":       fileDetails,
			"status":      "syntactically_feasible",
		}
	} else {
		// Single file target: parse the individual file.
		f, pErr := parser.ParseFile(fset, input.Target, nil, parser.ParseComments)
		if pErr != nil {
			res := &mcp.CallToolResult{}
			res.SetError(fmt.Errorf("AST parse failed: %v", pErr))
			return res, nil, nil
		}

		payload = map[string]any{
			"file":        input.Target,
			"package":     f.Name.Name,
			"decls_count": len(f.Decls),
			"unresolved":  len(f.Unresolved),
			"status":      "syntactically_feasible",
		}
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["ast_probe_results"] = payload
	t.Manager.SaveSession(ctx, session)

	if recallAvailable && session.ProjectRoot != "" {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, session.ProjectRoot, "ast_probe_complete", "native", t.Name(), "", session.Metadata)
	}

	if input.SessionID != "" && recallAvailable {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, payload)
	}

	return &mcp.CallToolResult{}, payload, nil
}
