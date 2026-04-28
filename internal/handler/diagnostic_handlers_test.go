package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcp-server-magictools/internal/client"
	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAnalyzeSystemLogs(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "log-test")
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "magictools_debug.log")
	logContent := `{"level":"INFO","msg":"starting up"}
{"level":"ERROR","server":"github","msg":"connection failed"}
{"level":"INFO","server":"ddg-search","msg":"searching"}
{"level":"WARN","server":"github","msg":"retry 1"}
{"level":"ERROR","server":"ddg-search","msg":"timeout"}
`
	err := os.WriteFile(logPath, []byte(logContent), 0644)
	if err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	h := &OrchestratorHandler{
		Config: &config.Config{LogPath: logPath},
	}

	ctx := context.Background()

	t.Run("TailAll", func(t *testing.T) {
		req := &mcp.CallToolRequest{}
		req.Params = &mcp.CallToolParamsRaw{
			Name:      "analyze_system_logs",
			Arguments: json.RawMessage(`{"lines": 10}`),
		}

		result, err := h.AnalyzeSystemLogs(ctx, req)
		if err != nil {
			t.Fatalf("tool call failed: %v", err)
		}

		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "starting up") || !strings.Contains(text, "timeout") {
			t.Errorf("expected all logs, got: %s", text)
		}
		if !strings.HasPrefix(text, "```") || !strings.HasSuffix(text, "```") {
			t.Errorf("expected markdown code block, got: %s", text)
		}
	})

	t.Run("FilterServer", func(t *testing.T) {
		req := &mcp.CallToolRequest{}
		req.Params = &mcp.CallToolParamsRaw{
			Name:      "analyze_system_logs",
			Arguments: json.RawMessage(`{"server_id": "github", "lines": 10}`),
		}

		result, err := h.AnalyzeSystemLogs(ctx, req)
		if err != nil {
			t.Fatalf("tool call failed: %v", err)
		}

		text := result.Content[0].(*mcp.TextContent).Text
		if strings.Contains(text, "ddg-search") {
			t.Errorf("did not expect ddg-search logs, got: %s", text)
		}
		if !strings.Contains(text, "github") {
			t.Errorf("expected github logs, got: %s", text)
		}
	})

	t.Run("FilterSeverity", func(t *testing.T) {
		req := &mcp.CallToolRequest{}
		req.Params = &mcp.CallToolParamsRaw{
			Name:      "analyze_system_logs",
			Arguments: json.RawMessage(`{"severity": "ERROR", "lines": 10}`),
		}

		result, err := h.AnalyzeSystemLogs(ctx, req)
		if err != nil {
			t.Fatalf("tool call failed: %v", err)
		}

		text := result.Content[0].(*mcp.TextContent).Text
		if strings.Contains(text, "INFO") || strings.Contains(text, "WARN") {
			t.Errorf("expected only ERROR logs, got: %s", text)
		}
		if !strings.Contains(text, "ERROR") {
			t.Errorf("expected ERROR logs, got: %s", text)
		}
	})

	t.Run("FileNotFound", func(t *testing.T) {
		hErr := &OrchestratorHandler{
			Config: &config.Config{LogPath: filepath.Join(tmpDir, "nonexistent.log")},
		}
		req := &mcp.CallToolRequest{}
		req.Params = &mcp.CallToolParamsRaw{
			Name:      "analyze_system_logs",
			Arguments: json.RawMessage(`{"lines": 10}`),
		}

		_, err := hErr.AnalyzeSystemLogs(ctx, req)
		if err == nil {
			t.Fatal("expected error for non-existent file, got nil")
		}
		expectedMsg := "Log file not found at " + filepath.Join(tmpDir, "nonexistent.log") + ". Ensure logging is enabled in config."
		if err.Error() != expectedMsg {
			t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
		}
	})
}

func TestDiagnosticToolsMethods(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "diag-test")
	defer os.RemoveAll(tmpDir)

	store, _ := db.NewStore(filepath.Join(tmpDir, "db"))
	defer store.Close()
	cfg := &config.Config{LogPath: filepath.Join(tmpDir, "magictools_debug.log")}
	reg := client.NewWarmRegistry(filepath.Join(tmpDir, "pids"), store, cfg)
	h := NewHandler(store, reg, cfg)

	ctx := context.Background()

	t.Run("GetInternalLogs", func(t *testing.T) {
		_, _ = h.GetInternalLogs(ctx, &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{"max_lines": 5}`),
			},
		})
	})

	t.Run("GetSessionStats", func(t *testing.T) {
		_, _ = h.GetSessionStats(ctx, &mcp.CallToolRequest{})
	})

	t.Run("GetHealthReport", func(t *testing.T) {
		_, _ = h.GetHealthReport(ctx, &mcp.CallToolRequest{})
	})

	t.Run("UpdateConfig", func(t *testing.T) {
		_, _ = h.UpdateConfig(ctx, &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Arguments: json.RawMessage(`{"key": "logLevel", "value": "DEBUG"}`),
			},
		})
	})

	t.Run("SelfCheck", func(t *testing.T) {
		_, _ = h.SelfCheck(ctx, &mcp.CallToolRequest{})
	})

	t.Run("ListToolsInfo", func(t *testing.T) {
		result, err := h.ListToolsInfo(ctx, &mcp.CallToolRequest{})
		if err != nil {
			t.Fatalf("tool call failed: %v", err)
		}
		if len(result.Content) == 0 {
			t.Fatal("expected content")
		}
	})
}

func TestGetHealthReportNoRecall(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "diag-health-test")
	defer os.RemoveAll(tmpDir)

	store, _ := db.NewStore(filepath.Join(tmpDir, "db"))
	defer store.Close()
	cfg := &config.Config{LogPath: filepath.Join(tmpDir, "magictools_debug.log")}
	reg := client.NewWarmRegistry(filepath.Join(tmpDir, "pids"), store, cfg)
	h := NewHandler(store, reg, cfg)
	// RecallClient is nil by default — should produce output without historical_trends

	ctx := context.Background()
	result, err := h.GetHealthReport(ctx, &mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("GetHealthReport failed: %v", err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if strings.Contains(text, "historical_trends") {
		t.Error("did not expect historical_trends when RecallClient is nil")
	}
}
