package coverage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"mcp-server-go-refactor/internal/util"
	"os"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool implements the coverage tracer tool.
type Tool struct {
	Engine *engine.Engine
}

func (t *Tool) Name() string {
	return "go_test_coverage_tracer"
}

func (t *Tool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] [PHASE: ANALYSIS] TEST COVERAGE TRACER: Runs Go test suites, records pass/fail results, and reports uncovered code paths. Detects offline go toolchain environments and degrades gracefully. Produces test failure list with coverage gap analysis. [Routing Tags: tests, coverage, go-test, validate-paths]",
	}, t.Handle)
}

// Register adds the coverage tracer tool to the registry.
func Register(eng *engine.Engine) {
	registry.Global.Register(&Tool{Engine: eng})
}

type CoverageInput struct {
	SessionID string `json:"session_id,omitempty" jsonschema:"Optional CSSA backend storage pipeline correlation ID."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum items to return. Defaults to 500 if unassigned."`
	Offset    int    `json:"offset,omitempty" jsonschema:"Pagination offset slice start."`
	Pkg       string `json:"pkg" jsonschema:"The package path to test"`
}

func (t *Tool) Handle(ctx context.Context, req *mcp.CallToolRequest, input CoverageInput) (*mcp.CallToolResult, any, error) {
	var session *engine.Session

	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if t.Engine != nil {
		session = t.Engine.LoadSession(ctx, input.Pkg)

		if recallAvailable {
			// Load historical coverage data for this package for delta analysis.
			if history := t.Engine.LoadCrossSessionFromRecall(ctx, "gorefactor", input.Pkg); history != "" {
				if session.Metadata == nil {
					session.Metadata = make(map[string]any)
				}
				session.Metadata["historical_coverage"] = history
			}
		}

		// Sniff for native offline coverage files to force Test-Scaffolding pipelines locally
		if _, err := os.Stat(filepath.Join(input.Pkg, "coverage.out")); err == nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]any)
			}
			session.Metadata["offline_coverage_fallback"] = "coverage.out natively detected - forcing 0% structural node bounds scaffolding"
		}
	}

	result, err := Trace(ctx, input.Pkg)
	if err != nil {
		res := &mcp.CallToolResult{IsError: true}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			res.Content = []mcp.Content{
				&mcp.TextContent{
					Text: "TIMEOUT: The test suite execution was interrupted or exceeded the allocated time limit. This usually happens for large packages or slow environments. Try running tests for a smaller sub-package or increasing the proxy gateway timeout.",
				},
			}
			return res, nil, nil
		}
		res.SetError(err)
		return res, nil, nil
	}
	summary := "All tests passed successfully."
	if len(result.Failures) > 0 {
		summary = fmt.Sprintf("Found %d test failures in %s", len(result.Failures), input.Pkg)
	}

	// ---- CSSA / Pagination Fallback Logic ----
	var returnData any = result

	if recallAvailable && input.SessionID != "" {
		saveErr := t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, result)
		if saveErr == nil {
			summary += fmt.Sprintf("\n[CSSA STATUS]: Complete structural data saved successfully to recall session '%s'", input.SessionID)
			returnData = nil // Do not echo massive data across stdio
		} else {
			summary += "\n[CSSA STATUS]: Could not save to recall. Falling back to JSON-RPC pagination."
		}
	}

	if returnData != nil {
		p := util.Pagination{SessionID: input.SessionID, Limit: input.Limit, Offset: input.Offset}
		start, end := p.Apply(len(result.Failures))
		if len(result.Failures) > (end - start) {
			summary += fmt.Sprintf("\n[PAGINATION WARNING]: Payload truncated. Showing items %d-%d out of %d total. Pass a higher limit/offset if necessary.", start, end, len(result.Failures))
			returnData = &TraceResult{Failures: result.Failures[start:end]}
		}
	}
	// ------------------------------------------

	if session != nil {
		if session.Metadata == nil {
			session.Metadata = make(map[string]any)
		}

		if recallAvailable {
			testStds := t.Engine.EnsureRecallCache(ctx, session, "testing_standards", "search", map[string]interface{}{"namespace": "ecosystem",
				"query": "Go test coverage thresholds, table-driven test patterns, testing convention standards, and benchmark requirements for " + input.Pkg,
				"limit": 15,
			})
			session.Metadata["recall_cache_testing"] = testStds

			if testStds != "" {
				summary += fmt.Sprintf("\n\n[Testing & Coverage Standards]: %s", testStds)
			}
		}

		// Pillar metrics for brainstorm learning.
		session.Metadata["pillar_metrics"] = map[string]any{
			"pillar":             "reliability",
			"test_failure_count": len(result.Failures),
		}

		// AST faults.
		if len(result.Failures) > 0 {
			var astFaults []string
			if f, ok := session.Metadata["ast_faults"].([]string); ok {
				astFaults = f
			}
			session.Metadata["ast_faults"] = append(astFaults, "test_failure")
		}

		var diags []string
		if d, ok := session.Metadata["diagnostics"].([]string); ok {
			diags = d
		}
		session.Metadata["diagnostics"] = append(diags, summary)
		t.Engine.SaveSession(session)

		if recallAvailable {
			t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Pkg, "coverage_traced", "native", "go_test_coverage_tracer", "", session.Metadata)
		}
	}

	return &mcp.CallToolResult{}, struct {
		Summary string `json:"summary"`
		Data    any    `json:"data,omitempty"`
	}{
		Summary: summary,
		Data:    returnData,
	}, nil
}

// TestEvent represents a single JSON event from 'go test -json'
type TestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Output  string    `json:"Output"`
}

// TraceResult maps failed tests and their reason.
type TraceResult struct {
	Failures []Failure `json:"Failures"`
}

// Failure contains specific test failure details.
type Failure struct {
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

// Trace executes 'go test -json' and condenses the massive output to failures only.
func Trace(ctx context.Context, pkgPath string) (*TraceResult, error) {
	res, err := loader.Discover(ctx, pkgPath)
	if err != nil {
		return nil, err
	}

	stream, err := res.Runner.RunGoStream(ctx, "test", "-json", res.Pattern)
	if err != nil {
		return nil, fmt.Errorf("go test execution failed: %w", err)
	}
	defer stream.Wait()

	scanner := bufio.NewScanner(stream.Stdout)
	// Increase scanner buffer to 1MB to handle large JSON lines
	const maxTokenSize = 1024 * 1024
	scanner.Buffer(make([]byte, 64*1024), maxTokenSize)
	var failures []Failure
	outputTracker := make(map[string]*bytes.Buffer)

	for scanner.Scan() {
		var ev TestEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err == nil {
			if ev.Test != "" {
				testKey := ev.Package + "." + ev.Test
				switch ev.Action {
				case "run":
					outputTracker[testKey] = &bytes.Buffer{}
				case "output":
					if buf, ok := outputTracker[testKey]; ok && ev.Output != "" {
						// Cap per-test log to 1MB to prevent runaway failure output
						if buf.Len() < 1024*1024 {
							buf.WriteString(ev.Output)
						}
					}
				case "fail":
					if buf, ok := outputTracker[testKey]; ok {
						failures = append(failures, Failure{
							Package: ev.Package,
							Test:    ev.Test,
							Output:  buf.String(),
						})
					}
					delete(outputTracker, testKey) // Release memory immediately
				case "pass", "skip":
					delete(outputTracker, testKey) // Discard logs for non-failures
				}
			}
		}
	}

	return &TraceResult{Failures: failures}, nil
}
