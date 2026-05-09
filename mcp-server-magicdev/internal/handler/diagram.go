// Package handler provides functionality for the handler subsystem.
package handler

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"mcp-server-magicdev/internal/db"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// idSafe sanitizes a string for use as a D2 node identifier.
// Strips non-alphanumeric characters and lowercases the result.
var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]`)

func idSafe(s string) string {
	return strings.ToLower(nonAlphaNum.ReplaceAllString(s, ""))
}

// d2Label wraps a label in quotes, escaping interior quotes for D2 syntax safety.
func d2Label(s string) string {
	return strings.ReplaceAll(s, `"`, `'`)
}

// GenerateD2Diagram translates a Blueprint and SessionState into a D2 architectural diagram source.
func GenerateD2Diagram(session *db.SessionState, bp *db.Blueprint) string {
	if bp == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("direction: right\n\n")

	// --- Session Context ---
	hasContext := false
	if session != nil && (session.BusinessCase != "" || session.TargetEnvironment != "" || session.TechStack != "" || session.RiskLevel != "" || len(session.ComplianceRequirements) > 0) {
		hasContext = true
		sb.WriteString("context: \"Session Context\" {\n")
		sb.WriteString("  style.fill: \"#f5f5f5\"\n")
		sb.WriteString("  style.stroke: \"#cccccc\"\n")
		sb.WriteString("  style.border-radius: 8\n")
		if session.BusinessCase != "" {
			sb.WriteString(fmt.Sprintf("  business_case: \"Business Case: %s\"\n", d2Label(session.BusinessCase)))
		}
		if session.TargetEnvironment != "" {
			sb.WriteString(fmt.Sprintf("  environment: \"Environment: %s\"\n", d2Label(session.TargetEnvironment)))
		}
		if session.TechStack != "" {
			sb.WriteString(fmt.Sprintf("  tech_stack: \"Tech Stack: %s\"\n", d2Label(session.TechStack)))
		}
		if session.RiskLevel != "" {
			sb.WriteString(fmt.Sprintf("  risk_level: \"Risk Level: %s\"\n", d2Label(session.RiskLevel)))
		}
		if len(session.ComplianceRequirements) > 0 {
			comp := strings.Join(session.ComplianceRequirements, ", ")
			sb.WriteString(fmt.Sprintf("  compliance: \"Compliance: %s\"\n", d2Label(comp)))
		}
		sb.WriteString("}\n\n")
	}

	// --- Core Architecture (Module Hierarchy) ---
	hasModules := session != nil && session.DesignProposal != nil && len(session.DesignProposal.ProposedModules) > 0

	// Stack-aware fill color
	archFill := "#e8f8e8" // Node.js green default
	if session != nil && strings.Contains(strings.ToLower(session.TechStack), ".net") {
		archFill = "#e8e8f8" // .NET blue
	}

	if hasModules {
		sb.WriteString("architecture: \"Core Architecture\" {\n")
		sb.WriteString(fmt.Sprintf("  style.fill: \"%s\"\n", archFill))
		sb.WriteString("  style.border-radius: 8\n")

		// Define module nodes
		for _, mod := range session.DesignProposal.ProposedModules {
			modID := idSafe(mod.Name)
			sb.WriteString(fmt.Sprintf("  %s: \"%s\" {\n", modID, d2Label(mod.Name)))
			sb.WriteString("    shape: package\n")
			if mod.Purpose != "" {
				sb.WriteString(fmt.Sprintf("    tooltip: \"%s\"\n", d2Label(mod.Purpose)))
			}
			sb.WriteString("  }\n")
		}

		// Define dependency edges between modules
		for _, mod := range session.DesignProposal.ProposedModules {
			modID := idSafe(mod.Name)
			for _, depName := range mod.Dependencies {
				// Verify target module exists
				for _, target := range session.DesignProposal.ProposedModules {
					if target.Name == depName {
						targetID := idSafe(target.Name)
						sb.WriteString(fmt.Sprintf("  %s -> %s: \"depends on\"\n", modID, targetID))
						break
					}
				}
			}
		}

		sb.WriteString("}\n\n")
	} else {
		sb.WriteString("architecture: \"Application Core\" {\n")
		sb.WriteString(fmt.Sprintf("  style.fill: \"%s\"\n", archFill))
		sb.WriteString("  style.border-radius: 8\n")
		sb.WriteString("  shape: package\n")
		sb.WriteString("}\n\n")
	}

	// --- API / MCP Interfaces ---
	if len(bp.MCPTools) > 0 || len(bp.MCPResources) > 0 || len(bp.MCPPrompts) > 0 {
		sb.WriteString("interfaces: \"MCP Interfaces\" {\n")
		sb.WriteString("  style.fill: \"#fff8e8\"\n")
		sb.WriteString("  style.border-radius: 8\n")

		for i, tool := range bp.MCPTools {
			nodeID := fmt.Sprintf("tool%d", i)
			sb.WriteString(fmt.Sprintf("  %s: \"Tool: %s\" {\n    shape: hexagon\n  }\n", nodeID, d2Label(tool.Name)))
		}
		for i, res := range bp.MCPResources {
			nodeID := fmt.Sprintf("res%d", i)
			sb.WriteString(fmt.Sprintf("  %s: \"Resource: %s\" {\n    shape: stored_data\n  }\n", nodeID, d2Label(res.Name)))
		}
		for i, prompt := range bp.MCPPrompts {
			nodeID := fmt.Sprintf("prompt%d", i)
			sb.WriteString(fmt.Sprintf("  %s: \"Prompt: %s\" {\n    shape: page\n  }\n", nodeID, d2Label(prompt.Name)))
		}

		sb.WriteString("}\n\n")
		sb.WriteString("architecture -> interfaces: \"exposes\"\n\n")
	} else if len(bp.APIContracts) > 0 {
		sb.WriteString("api: \"API Layer\" {\n")
		sb.WriteString("  style.fill: \"#fff8e8\"\n")
		sb.WriteString("  style.border-radius: 8\n")

		for i, endpoint := range bp.APIContracts {
			nodeID := fmt.Sprintf("endpoint%d", i)
			sb.WriteString(fmt.Sprintf("  %s: \"%s %s\" {\n    shape: hexagon\n  }\n", nodeID, endpoint.Method, d2Label(endpoint.Path)))
		}

		sb.WriteString("}\n\n")
		sb.WriteString("architecture -> api: \"exposes\"\n\n")
	}

	// --- Data Model (ERD with sql_table shapes) ---
	if len(bp.DataModel) > 0 {
		sb.WriteString("data: \"Data Model\" {\n")
		sb.WriteString("  style.fill: \"#f8f0ff\"\n")
		sb.WriteString("  style.border-radius: 8\n")

		for _, entity := range bp.DataModel {
			entID := idSafe(entity.Name)
			sb.WriteString(fmt.Sprintf("  %s: \"%s\" {\n", entID, d2Label(entity.Name)))
			sb.WriteString("    shape: sql_table\n")
			for _, field := range entity.Fields {
				constraint := ""
				if strings.Contains(strings.ToLower(field.Name), "id") && field.Required {
					constraint = " {constraint: primary_key}"
				}
				// Quote field type to handle special characters like [] in array types
				sb.WriteString(fmt.Sprintf("    %s: \"%s\"%s\n", idSafe(field.Name), d2Label(field.Type), constraint))
			}
			sb.WriteString("  }\n")
		}

		// Relationship edges
		for _, entity := range bp.DataModel {
			entID := idSafe(entity.Name)
			for _, rel := range entity.Relationships {
				// Parse "User hasMany Posts" style relationships
				parts := strings.Fields(rel)
				if len(parts) >= 3 {
					targetID := idSafe(parts[0])
					relType := strings.Join(parts[1:], " ")
					sb.WriteString(fmt.Sprintf("  %s -> %s: \"%s\"\n", entID, targetID, d2Label(relType)))
				} else if len(parts) >= 1 {
					targetID := idSafe(parts[0])
					sb.WriteString(fmt.Sprintf("  %s -> %s\n", entID, targetID))
				}
			}
		}

		sb.WriteString("}\n\n")
		sb.WriteString("architecture -> data: \"persists\"\n\n")
	}

	// --- External Dependencies ---
	if len(bp.DependencyManifest) > 0 {
		sb.WriteString("deps: \"External Dependencies\" {\n")
		sb.WriteString("  style.fill: \"#e8f4fd\"\n")
		sb.WriteString("  style.border-radius: 8\n")

		for i, dep := range bp.DependencyManifest {
			depID := fmt.Sprintf("dep%d", i)
			// Sanitize package names containing @ and / for D2 label safety
			name := d2Label(dep.Name)
			version := d2Label(dep.Version)
			label := fmt.Sprintf("%s %s", name, version)
			if dep.Ecosystem != "" {
				label += fmt.Sprintf(" (%s)", dep.Ecosystem)
			}
			sb.WriteString(fmt.Sprintf("  %s: \"%s\" {\n    shape: hexagon\n  }\n", depID, label))
		}

		sb.WriteString("}\n\n")
		sb.WriteString("architecture -> deps: \"imports\"\n\n")
	}

	// --- Cross-layer edges ---
	if hasContext {
		sb.WriteString("context -> architecture: {\n  style.stroke-dash: 5\n}\n")
	}

	return sb.String()
}

// RenderD2ToSVG compiles a D2 source string into an SVG using the dagre layout engine.
// Returns the SVG as a string. If compilation or rendering fails, an error is returned
// but the original D2 source remains available for manual inspection.
func RenderD2ToSVG(d2Source string) (string, error) {
	if d2Source == "" {
		return "", nil
	}

	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return "", fmt.Errorf("d2 ruler init: %w", err)
	}

	// Inject our stderr-bound JSON logger into the context so D2's internal
	// lib/log package uses it instead of its default PrettyHandler, which
	// writes ANSI escape codes to stdout via fmt.Println — fatally corrupting
	// the MCP JSON-RPC stdio transport.
	ctx := d2log.With(context.Background(), slog.Default())

	// d2lib.Compile needs a LayoutResolver to resolve the "dagre" engine name
	compileOpts := &d2lib.CompileOptions{
		Ruler: ruler,
		LayoutResolver: func(engine string) (d2graph.LayoutGraph, error) {
			return func(ctx context.Context, g *d2graph.Graph) error {
				return d2dagrelayout.Layout(ctx, g, nil)
			}, nil
		},
	}
	renderOpts := &d2svg.RenderOpts{}

	diagram, _, err := d2lib.Compile(ctx, d2Source, compileOpts, renderOpts)
	if err != nil {
		return "", fmt.Errorf("d2 compile: %w", err)
	}

	out, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return "", fmt.Errorf("d2 render: %w", err)
	}

	slog.Debug("[diagram] D2 SVG rendered successfully", "size_bytes", len(out))
	return string(out), nil
}
