package coverage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mcp-server-go-refactor/internal/loader"
	"mcp-server-go-refactor/internal/registry"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// Tool implements the coverage tracer tool.
type Tool struct{}

// Register adds the coverage tracer tool to the registry.
func Register() {
	registry.Global.Register(&Tool{})
}

func (t *Tool) Metadata() mcp.Tool {
	return mcp.NewTool("go_test_coverage_tracer",
		mcp.WithDescription("Executes a package-level test suite and intelligently filters the output to surface only actionable failures and their corresponding logs. This tool drastically reduces noise during TDD cycles by focusing the developer's attention on breaking changes."),
		mcp.WithString("pkg", mcp.Description("The package path to test"), mcp.Required()),
	)
}

func (t *Tool) Handle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pkg := request.GetString("pkg", "")
	result, err := Trace(ctx, pkg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("%+v", result)), nil
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

	out, err := res.Runner.RunGo(ctx, "test", "-json", res.Pattern)
	if err != nil && len(out.Stdout) == 0 {
		return nil, fmt.Errorf("go test execution failed: %w: %s", err, string(out.Stderr))
	}
	// We check for out.Err if we have no stdout, but if we have stdout, we proceed
	// to parse JSON even if tests failed (exit code 1).

	scanner := bufio.NewScanner(bytes.NewReader(out.Stdout))
	var failures []Failure
	outputTracker := make(map[string]*bytes.Buffer)

	for scanner.Scan() {
		var ev TestEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err == nil {
			testKey := ev.Package + "." + ev.Test
			if ev.Test != "" {
				if _, ok := outputTracker[testKey]; !ok {
					outputTracker[testKey] = &bytes.Buffer{}
				}
				if ev.Output != "" {
					outputTracker[testKey].WriteString(ev.Output)
				}

				if ev.Action == "fail" {
					failures = append(failures, Failure{
						Package: ev.Package,
						Test:    ev.Test,
						Output:  outputTracker[testKey].String(),
					})
				}
			}
		}
	}

	return &TraceResult{Failures: failures}, nil
}
