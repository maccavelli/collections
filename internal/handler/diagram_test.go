package handler

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestGenerateMermaidDiagram(t *testing.T) {
	session := &db.SessionState{
		DesignProposal: &db.DesignProposal{
			ProposedModules: []db.ModuleSpec{
				{Name: "Core Server", Dependencies: []string{"Database"}},
				{Name: "Database", Dependencies: []string{}},
			},
		},
	}
	bp := &db.Blueprint{
		MCPTools: []db.MCPTool{
			{Name: "run_audit"},
		},
		MCPResources: []db.MCPResource{
			{Name: "logs"},
		},
	}

	out := GenerateMermaidDiagram(session, bp)
	if !strings.Contains(out, "```mermaid") {
		t.Errorf("Missing mermaid fence")
	}
	if !strings.Contains(out, "Core Server") {
		t.Errorf("Missing Core Server module")
	}
	if !strings.Contains(out, "Database") {
		t.Errorf("Missing Database module")
	}
	if !strings.Contains(out, "run_audit") {
		t.Errorf("Missing run_audit tool")
	}
	if !strings.Contains(out, "logs") {
		t.Errorf("Missing logs resource")
	}

	if GenerateMermaidDiagram(nil, nil) != "" {
		t.Errorf("Expected empty string for nil blueprint")
	}
}
