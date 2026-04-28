package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magictools/internal/db"
)

func TestSemanticSimilarity(t *testing.T) {
	// Zero-Flake Synctest Bubble
	// 1. Setup real BadgerDB and Bleve index in TempDir
	tmpDir := t.TempDir()
	store, err := db.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer store.Close()

	h := &OrchestratorHandler{
		Store:         store,
		InternalTools: make([]*InternalTool, 0),
	}

	// 2. Inject two identical dummy tools
	schemaMap := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filename": map[string]any{"type": "string"},
			"lines":    map[string]any{"type": "integer"},
		},
	}

	tool1 := &db.ToolRecord{
		URN:         "test_tool_1",
		Name:        "test_tool_1",
		Server:      "dummy",
		Description: "This unique string creates a strong tf-idf cluster for file reading",
		Category:    "filesystem",
		InputSchema: schemaMap,
	}

	tool2 := &db.ToolRecord{
		URN:         "test_tool_2",
		Name:        "test_tool_2",
		Server:      "dummy",
		Description: "This unique string creates a strong tf-idf cluster for file reading",
		Category:    "filesystem",
		InputSchema: schemaMap,
	}

	tool3 := &db.ToolRecord{
		URN:         "test_tool_3_excluded",
		Name:        "test_tool_3_excluded",
		Server:      "rogue_server",
		Description: "This unique string creates a strong tf-idf cluster for file reading",
		Category:    "filesystem",
		InputSchema: schemaMap,
	}

	if err := store.SaveTool(tool1); err != nil {
		t.Fatalf("Failed to save tool1: %v", err)
	}
	if err := store.SaveTool(tool2); err != nil {
		t.Fatalf("Failed to save tool2: %v", err)
	}
	if err := store.SaveTool(tool3); err != nil {
		t.Fatalf("Failed to save tool3: %v", err)
	}

	// Give Bleve Mem index a moment to flush asynchronously
	time.Sleep(100 * time.Millisecond)

	// 3. Call SemanticSimilarityAudit with scoped restrictions
	req := &mcp.CallToolRequest{}
	_ = json.Unmarshal([]byte(`{"params":{"arguments":{"servers":"dummy"}}}`), req)
	res, err := h.SemanticSimilarityAudit(context.Background(), req)
	if err != nil {
		t.Fatalf("SemanticSimilarityAudit failed: %v", err)
	}

	if len(res.Content) == 0 {
		t.Fatal("Expected content in response")
	}

	text := res.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "test_tool_1") || !strings.Contains(text, "test_tool_2") {
		t.Errorf("Expected response to mention both tools, got:\n%s", text)
	}

	if strings.Contains(text, "test_tool_3_excluded") {
		t.Errorf("Expected response to completely ignore out-of-scope tool3, but found it!")
	}

	if !strings.Contains(text, "Redundant Duplicate") {
		t.Errorf("Expected categorization 'Redundant Duplicate' for perfect match, got:\n%s", text)
	}
}
