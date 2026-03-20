package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"mcp-server-magicskills/internal/engine"
	"mcp-server-magicskills/internal/models"
)

func TestHandleGetSection(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Description: "test skill"},
		Sections: map[string]string{
			"workflow": "1. Step one\n- Step two",
			"full":     "Full content",
		},
	}

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}
	
	// Test valid section
	args := map[string]interface{}{"name": "test", "section": "workflow"}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
	
	resp, err := h.HandleGetSection(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleGetSection failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Step one") {
		t.Errorf("Expected Step one in output, got: %s", text)
	}
}

func TestHandleMatchSkills(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test-skill"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test-skill", Description: "Searchable info"},
	}

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}
	
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"intent": "Searchable"},
		},
	}
	
	resp, err := h.HandleMatchSkills(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleMatchSkills failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "test-skill") {
		t.Errorf("Expected test-skill in search result, got: %s", text)
	}
}

func TestHandleSummarize(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test"},
		Sections: map[string]string{
			"magic directive": "Short instruction",
		},
	}

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}
	
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"name": "test"},
		},
	}
	
	resp, err := h.HandleSummarize(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleSummarize failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Short instruction") {
		t.Errorf("Expected summary in output, got: %s", text)
	}
}

func TestHandleGetLogs(t *testing.T) {
	lb := &LogBuffer{}
	_, _ = lb.Write([]byte("Line 1\nLine 2"))
	h := &MagicSkillsHandler{Logs: lb}
	
	req := mcp.CallToolRequest{}
	resp, err := h.HandleGetLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleGetLogs failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Line 1") || !strings.Contains(text, "Line 2") {
		t.Fatal("Missing log contents")
	}
}

func TestHandleBootstrapTask(t *testing.T) {
	eng := engine.NewEngine()
	eng.Skills["test"] = &models.Skill{
		Metadata: models.SkillMetadata{Name: "test", Description: "test skill"},
		Sections: map[string]string{
			"workflow": "1. Step one\n- Step two",
		},
	}

	h := &MagicSkillsHandler{Engine: eng, Logs: &LogBuffer{}}
	
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]interface{}{"name": "test"},
		},
	}
	
	resp, err := h.HandleBootstrapTask(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleBootstrapTask failed: %v", err)
	}
	text := resp.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "- [ ]") {
		t.Errorf("Expected checklist format - [ ], got: %s", text)
	}
}
