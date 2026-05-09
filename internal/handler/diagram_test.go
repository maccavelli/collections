package handler

import (
	"strings"
	"testing"

	"mcp-server-magicdev/internal/db"
)

func TestGenerateD2Diagram(t *testing.T) {
	session := &db.SessionState{
		TechStack:         "Node",
		BusinessCase:      "Improve developer productivity",
		TargetEnvironment: "containerized",
		DesignProposal: &db.DesignProposal{
			ProposedModules: []db.ModuleSpec{
				{Name: "Auth Service", Purpose: "Handles authentication", Responsibilities: []string{"login", "register"}, Dependencies: []string{"Data Layer"}},
				{Name: "Data Layer", Purpose: "Database access"},
			},
		},
	}

	bp := &db.Blueprint{
		DependencyManifest: []db.Dependency{
			{Name: "express", Version: "4.18", Ecosystem: "npm"},
		},
		DataModel: []db.Entity{
			{
				Name: "User",
				Fields: []db.EntityField{
					{Name: "id", Type: "uuid", Required: true},
					{Name: "email", Type: "string"},
				},
			},
		},
		MCPTools: []db.MCPTool{
			{Name: "search", Description: "Search memories"},
		},
	}

	out := GenerateD2Diagram(session, bp)

	checks := []struct {
		label   string
		content string
	}{
		{"direction", "direction: right"},
		{"session context", "Session Context"},
		{"business case", "Business Case:"},
		{"architecture container", "Core Architecture"},
		{"module shape", "shape: package"},
		{"module dependency edge", "depends on"},
		{"mcp interface", "MCP Interfaces"},
		{"tool shape", "shape: hexagon"},
		{"data model", "Data Model"},
		{"sql table shape", "shape: sql_table"},
		{"dependency container", "External Dependencies"},
		{"express dep", "express 4.18"},
		{"cross-layer edge", "stroke-dash"},
	}

	for _, c := range checks {
		if !strings.Contains(out, c.content) {
			t.Errorf("Missing %s: expected %q in output", c.label, c.content)
		}
	}
}

func TestGenerateD2Diagram_DotNetStyling(t *testing.T) {
	session := &db.SessionState{
		TechStack: ".NET",
		DesignProposal: &db.DesignProposal{
			ProposedModules: []db.ModuleSpec{
				{Name: "Controller", Purpose: "HTTP handlers"},
			},
		},
	}
	bp := &db.Blueprint{}

	out := GenerateD2Diagram(session, bp)
	if !strings.Contains(out, "#e8e8f8") {
		t.Error("Expected .NET blue fill color for architecture container")
	}
}

func TestGenerateD2Diagram_NilInputs(t *testing.T) {
	if GenerateD2Diagram(nil, nil) != "" {
		t.Error("Expected empty string for nil inputs")
	}
	if GenerateD2Diagram(&db.SessionState{}, nil) != "" {
		t.Error("Expected empty string for nil blueprint")
	}
}

func TestRenderD2ToSVG(t *testing.T) {
	d2Source := `direction: right
a: "Hello" {
  shape: package
}
b: "World" {
  shape: hexagon
}
a -> b: "connects"
`
	svg, err := RenderD2ToSVG(d2Source)
	if err != nil {
		t.Fatalf("RenderD2ToSVG failed: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Error("Expected SVG output to contain <svg tag")
	}
	if !strings.Contains(svg, "Hello") {
		t.Error("Expected SVG output to contain node label 'Hello'")
	}
}

func TestRenderD2ToSVG_EmptyInput(t *testing.T) {
	svg, err := RenderD2ToSVG("")
	if err != nil {
		t.Fatalf("Expected no error for empty input, got: %v", err)
	}
	if svg != "" {
		t.Error("Expected empty SVG for empty input")
	}
}

func TestRenderD2ToSVG_InvalidInput(t *testing.T) {
	_, err := RenderD2ToSVG("{{{{ invalid d2 syntax }}}}")
	if err == nil {
		t.Error("Expected error for invalid D2 syntax, got nil")
	}
}

func TestGenerateD2Diagram_APIContracts(t *testing.T) {
	bp := &db.Blueprint{
		APIContracts: []db.APIEndpoint{
			{Method: "POST", Path: "/api/auth/login"},
			{Method: "GET", Path: "/api/users"},
		},
	}

	out := GenerateD2Diagram(nil, bp)
	if !strings.Contains(out, "API Layer") {
		t.Error("Expected API Layer container")
	}
	if !strings.Contains(out, "POST /api/auth/login") {
		t.Error("Expected POST endpoint in diagram")
	}
}

func TestGenerateD2Diagram_ArrayTypesAndScopedPackages(t *testing.T) {
	bp := &db.Blueprint{
		DataModel: []db.Entity{
			{
				Name: "AuditReport",
				Fields: []db.EntityField{
					{Name: "id", Type: "string", Required: true},
					{Name: "findings", Type: "AuditFinding[]"},
				},
				Relationships: []string{"AuditFinding has many AuditReport"},
			},
		},
		DependencyManifest: []db.Dependency{
			{Name: "@modelcontextprotocol/sdk", Version: "^1.12.0", Ecosystem: "npm"},
			{Name: "@typescript-eslint/parser", Version: "^8.0.0", Ecosystem: "npm"},
		},
	}

	out := GenerateD2Diagram(nil, bp)

	// Verify it generates valid D2 source
	if out == "" {
		t.Fatal("Expected non-empty D2 output")
	}

	// Verify array type is properly quoted
	if !strings.Contains(out, `"AuditFinding[]"`) {
		t.Error("Expected quoted AuditFinding[] type in sql_table")
	}

	// Verify scoped package names don't break labels
	if !strings.Contains(out, "@modelcontextprotocol/sdk") {
		t.Error("Expected scoped package name in dependency label")
	}

	// The real validation: it must compile to SVG without error
	svg, err := RenderD2ToSVG(out)
	if err != nil {
		t.Fatalf("D2 source with array types/scoped packages failed to render: %v\nD2 source:\n%s", err, out)
	}
	if !strings.Contains(svg, "<svg") {
		t.Error("Expected valid SVG output")
	}
}
