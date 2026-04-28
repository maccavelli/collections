package engine

import (
	"context"
	"fmt"
	"strings"

	"mcp-server-brainstorm/internal/models"
)

// AnalyzeThreatModel performs STRIDE analysis on the proposed architectural text.
func (e *Engine) AnalyzeThreatModel(ctx context.Context, featureDesign string, traceMap map[string]interface{}) (models.ThreatModelResponse, error) {
	resp := models.ThreatModelResponse{
		Summary: "Threat model analysis evaluated natively.",
	}
	resp.Data.Narrative = "Socratic engine mapped trust boundaries against physical dependencies via STRIDE heuristics."

	metrics := models.STRIDEMetrics{}
	var vulns []string
	var recs []string

	// Check empirical telemetry (Angle 3)
	if traceMap != nil {
		if imports, ok := traceMap["imports"].([]interface{}); ok {
			for _, imp := range imports {
				pkg, _ := imp.(string)

				if strings.Contains(pkg, "database/sql") || strings.Contains(pkg, "lib/pq") || strings.Contains(pkg, "gorm") {
					metrics.Tampering++
					metrics.InformationLeak++
					vulns = append(vulns, fmt.Sprintf("Data tampering & exfiltration risk associated to database bindings (%s)", pkg))
					recs = append(recs, fmt.Sprintf("Ensure strict RBAC and parameterized inputs over DB connections for %s", pkg))
				}

				if strings.Contains(pkg, "net/http") || strings.Contains(pkg, "grpc") || strings.Contains(pkg, "gin") {
					metrics.Spoofing++
					metrics.DenialOfService++
					metrics.InformationLeak++
					vulns = append(vulns, fmt.Sprintf("Spoofing and DoS vectors open on ingress/egress boundaries (%s)", pkg))
					recs = append(recs, fmt.Sprintf("Enforce strict mTLS and rate limits on HTTP/gRPC interfaces for %s", pkg))
				}

				if strings.Contains(pkg, "os/exec") {
					metrics.ElevationOfPrivilege++
					metrics.Tampering++
					vulns = append(vulns, fmt.Sprintf("Critical OS-level shell injection risk detected (%s)", pkg))
					recs = append(recs, fmt.Sprintf("Audit pathing and sanitize all inputs mapped to %s", pkg))
				}

				if strings.Contains(pkg, "crypto") {
					metrics.Repudiation++
					vulns = append(vulns, fmt.Sprintf("Repudiation issues or bad entropy could breach cryptography (%s)", pkg))
					recs = append(recs, fmt.Sprintf("Validate hardware entropy limits and ensure non-repudiation logging for %s", pkg))
				}
			}
		}
	}

	// Dynamic text fallback heuristic
	lower := strings.ToLower(featureDesign)
	if strings.Contains(lower, "admin") || strings.Contains(lower, "auth") {
		metrics.ElevationOfPrivilege++
		vulns = append(vulns, "Concept implies authenticated administrative actions.")
		recs = append(recs, "Always assume breach on admin control planes; enforce zero-trust bounds.")
	}

	if metrics.Spoofing == 0 && metrics.Tampering == 0 && metrics.ElevationOfPrivilege == 0 && metrics.InformationLeak == 0 && metrics.DenialOfService == 0 && metrics.Repudiation == 0 {
		resp.Data.Narrative = "No extreme STRIDE surface detected from AST imports."
		vulns = append(vulns, "None strictly detected via current AST/import footprint.")
		recs = append(recs, "Maintain standard defense-in-depth security posturing.")
	}

	resp.Data.Metrics = metrics
	resp.Data.Vulnerabilities = vulns
	resp.Data.Recommendations = recs

	return resp, nil
}

// ExtractArchitectureTelemetry isolates cross-session W3C traces and project structure for orchestration representation.
func (e *Engine) ExtractArchitectureTelemetry(ctx context.Context, sessionID, target, instructions string) (models.TelemetryResponse, error) {
	resp := models.TelemetryResponse{
		Summary: "Architecture telemetry extracted for orchestrator visualization.",
	}

	// Dynamically fetch robust structural data (e.g., from a prior deep structural analysis).
	tracePayload := e.LoadCrossSessionFromRecall(ctx, "go-refactor", e.ResolvePath(target))
	if tracePayload == "" {
		tracePayload = "{\"status\": \"no structural session trace found for target boundary\", \"target\": \"" + target + "\"}"
	}

	resp.Data.TraceData = tracePayload
	return resp, nil
}
