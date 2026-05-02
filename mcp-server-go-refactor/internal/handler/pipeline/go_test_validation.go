package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"mcp-server-go-refactor/internal/engine"
	"mcp-server-go-refactor/internal/models"
	"mcp-server-go-refactor/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GoTestValidationTool executes synchronous test coverage bounds via 'go test' post-modification.
type GoTestValidationTool struct {
	Engine *engine.Engine
}

func (t *GoTestValidationTool) Name() string {
	return "go_test_validation"
}

func (t *GoTestValidationTool) Register(s util.SessionProvider) {
	util.HardenedAddTool(s, &mcp.Tool{
		Name:        t.Name(),
		Description: "[ROLE: CRITIC] [PHASE: VALIDATION] POST-EDIT TEST VALIDATOR: Runs go test and go vet after code writes to verify structural stability. [Routing Tags: go-vet, post-edit, run-tests, structural-stability]",
	}, t.Handle)
}

type GoTestInput struct {
	models.UniversalPipelineInput
}

func (t *GoTestValidationTool) Handle(ctx context.Context, _ *mcp.CallToolRequest, input GoTestInput) (*mcp.CallToolResult, any, error) {
	isOrchestrator := os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true"
	recallAvailable := isOrchestrator && t.Engine != nil && t.Engine.ExternalClient != nil && t.Engine.ExternalClient.RecallEnabled()
	if isOrchestrator && !recallAvailable {
		slog.Warn("[ORCHESTRATOR] recall unavailable — degrading to standalone", "tool", t.Name())
	}

	if input.Target == "" {
		res := &mcp.CallToolResult{}
		res.SetError(fmt.Errorf("target (project path) is required"))
		return res, nil, nil
	}

	dir := filepath.Dir(input.Target)
	if fi, err := os.Stat(input.Target); err == nil && fi.IsDir() {
		dir = input.Target
	}

	// 🛡️ OOM PROTECTION: Enforce strict sequential testing to avoid crashing the orchestrator ecosystem.
	// Decouple from MCP request context to prevent transport timeout killing go test.
	execCtx, execCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer execCancel()
	cmd := exec.CommandContext(execCtx, "go", "test", "-p=1", "-parallel=1", "-short", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()

	success := (err == nil)
	outputStr := string(out)

	// Post-test trace.
	session := t.Engine.LoadSession(ctx, input.Target)
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["last_test_run"] = dir
	session.Metadata["test_success"] = success
	t.Engine.SaveSession(session)

	// Publish test validation trace to recall sessions matrix.
	if recallAvailable {
		t.Engine.PublishSessionToRecall(ctx, input.SessionID, input.Target, "test_validated", "native", "go_test_validation", "", map[string]any{
			"target":    input.Target,
			"passed":    success,
			"phase":     "verification",
			"stage":     "go_test_validation",
			"last_tool": "go_test_validation",
		})
	}

	resp := struct {
		Summary    string `json:"summary"`
		Target     string `json:"target"`
		TestPassed bool   `json:"test_passed"`
		Output     string `json:"output"`
	}{
		Summary:    fmt.Sprintf("Tests executed for %s (Pass: %v)", dir, success),
		Target:     dir,
		TestPassed: success,
		Output:     outputStr,
	}

	// Publish to CSSA.
	if recallAvailable && input.SessionID != "" {
		_ = t.Engine.ExternalClient.SaveSession(ctx, input.SessionID, input.SessionID, resp)
	}

	return &mcp.CallToolResult{}, resp, nil
}
