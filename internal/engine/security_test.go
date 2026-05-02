package engine

import (
	"context"
	"github.com/tidwall/buntdb"
	"testing"
)

func TestAnalyzeThreatModel(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	// Case 1: Database and Network risks
	traceMap := map[string]any{
		"imports": []any{
			"database/sql",
			"net/http",
			"os/exec",
			"crypto",
		},
	}
	resp, err := e.AnalyzeThreatModel(ctx, "admin portal design", traceMap)
	if err != nil {
		t.Fatalf("AnalyzeThreatModel failed: %v", err)
	}

	if resp.Data.Metrics.Tampering == 0 {
		t.Error("expected tampering metrics to be > 0")
	}
	if resp.Data.Metrics.Spoofing == 0 {
		t.Error("expected spoofing metrics to be > 0")
	}
	if resp.Data.Metrics.ElevationOfPrivilege == 0 {
		t.Error("expected elevation of privilege metrics to be > 0")
	}

	// Case 2: Clean design
	resp2, err := e.AnalyzeThreatModel(ctx, "simple calculator", nil)
	if err != nil {
		t.Fatalf("AnalyzeThreatModel failed: %v", err)
	}
	if resp2.Data.Metrics.Tampering != 0 {
		t.Error("expected zero tampering for clean design")
	}
}

func TestExtractArchitectureTelemetry(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	resp, err := e.ExtractArchitectureTelemetry(ctx, "session-1", "/tmp/project", "test instructions")
	if err != nil {
		t.Fatalf("ExtractArchitectureTelemetry failed: %v", err)
	}
	if resp.Data.TraceData == "" {
		t.Error("expected non-empty trace data")
	}
}
