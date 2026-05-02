package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"
)

func TestASTProbeTool_Handle(t *testing.T) {
	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	tool := &ASTProbeTool{Manager: mgr, Engine: eng}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	// Create a temp file to probe
	tmpFile := filepath.Join(t.TempDir(), "test.go")
	os.WriteFile(tmpFile, []byte("package main\nfunc main() {}"), 0644)

	input := ASTProbeInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			SessionID: "test-session",
			Target:    tmpFile,
		},
	}

	_, result, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	payload := result.(map[string]any)
	if payload["package"] != "main" {
		t.Errorf("expected package main, got %v", payload["package"])
	}

	// Error case: Parse failure
	os.WriteFile(tmpFile, []byte("invalid go code"), 0644)
	resErr, _, _ := tool.Handle(ctx, req, input)
	if !resErr.IsError {
		t.Error("expected error for invalid AST parse")
	}

	// Error case: Missing target
	input.Target = ""
	resErr2, _, _ := tool.Handle(ctx, req, input)
	if !resErr2.IsError {
		t.Error("expected error for missing target")
	}
}

func TestComplexityForecasterTool_Handle(t *testing.T) {
	mgr := state.NewManager(".")
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	eng := engine.NewEngine(".", db)
	tool := &ComplexityForecasterTool{Manager: mgr, Engine: eng}

	ctx := context.Background()
	req := &mcp.CallToolRequest{}

	input := ComplexityForecasterInput{
		UniversalPipelineInput: models.UniversalPipelineInput{
			SessionID: "test-session",
			Context:   "x + y",
		},
	}

	_, result, err := tool.Handle(ctx, req, input)
	if err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	payload := result.(map[string]any)
	if payload["verdict"] != "APPROVED_PREDICTION" {
		t.Errorf("expected APPROVED_PREDICTION, got %v", payload["verdict"])
	}
	if payload["valid_expression"] != true {
		t.Error("expected valid_expression to be true")
	}

	// Error case: Empty context
	input.Context = ""
	resErr, _, _ := tool.Handle(ctx, req, input)
	if !resErr.IsError {
		t.Error("expected error for empty context")
	}
}
