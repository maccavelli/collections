package cmd

import (
	"errors"
	"testing"
	"time"

	"mcp-server-magicdev/internal/db"
)

func TestFormatBytes(t *testing.T) {
	if formatBytes(500) != "500 B" {
		t.Errorf("Expected 500 B, got %s", formatBytes(500))
	}
	if formatBytes(1500) != "1.5 KB" {
		t.Errorf("Expected 1.5 KB, got %s", formatBytes(1500))
	}
}

func TestExtractJSONValue(t *testing.T) {
	line := `{"step": "test_step", "sha256": "1234567890abcdef1234567890abcdef"}`
	step := extractJSONValue(line, "step")
	if step != " \"test_step\"" {
		t.Errorf("Expected \"test_step\", got %s", step)
	}

	sha := extractJSONValue(line, "sha256")
	if sha != " \"1234567890abcdef1234567890abcdef\"" {
		t.Errorf("Expected \"1234567890abcdef1234567890abcdef\", got %s", sha)
	}
}

func TestRenderViews(t *testing.T) {
	m := model{
		coldState: metricsMsg{
			DBSize: 2048,
			Keys: 5,
			SessionCount: 2,
			BaselineCount: 3,
			ChaosCount: 1,
			Sessions: []db.SessionState{
				{SessionID: "1234567890", CurrentStep: "step1", StepTimings: map[string]db.StepTiming{"step1": {StartedAt: time.Now().Add(-5 * time.Minute).Format(time.RFC3339)}}},
			},
		},
		hotState: udpMetricsMsg{
			NumCPU: 4,
			MemAlloc: 1024,
			NumGoroutine: 10,
			Uptime: "5s",
		},
	}

	overview := renderOverview(m)
	if overview == "" {
		t.Errorf("Overview is empty")
	}

	sessions := renderSessions(m)
	if sessions == "" {
		t.Errorf("Sessions is empty")
	}

	bucketData := renderBucketData(m)
	if bucketData == "" {
		t.Errorf("BucketData is empty")
	}

	config := renderConfig(m)
	if config == "" {
		t.Errorf("Config is empty")
	}
	
	// Just test View runs
	m.View()
}

func TestDashboardModelInitAndUpdate(t *testing.T) {
	m := model{}
	cmd := m.Init()
	if cmd != nil {
		t.Errorf("Expected Init to return nil, got %v", cmd)
	}

	// Just exercise Update
	updatedModel, cmd := m.Update(nil)
	if cmd != nil {
		t.Errorf("Expected Update with nil to return nil cmd")
	}
	_ = updatedModel
}

func TestReadDashboardSnapshotMissingDB(t *testing.T) {
	msg := ReadDashboardSnapshot("non-existent-db-path.db")
	if msg.DBSize != 0 {
		t.Errorf("Expected DBSize 0 for missing DB, got %d", msg.DBSize)
	}
}

func TestReadDashboardSnapshotSuccess(t *testing.T) {
	msg := ReadDashboardSnapshot("")

	// Just test that the msg is created correctly without panic.
	if msg.DBSize != 0 {
		t.Errorf("Expected DBSize 0 for empty DB, got %d", msg.DBSize)
	}
}

func TestModelUpdate(t *testing.T) {
	m := model{
		hotState: udpMetricsMsg{NumCPU: 4},
	}

	// Test udpMetricsMsg
	m3, _ := m.Update(udpMetricsMsg{NumCPU: 8})
	if m3.(model).hotState.NumCPU != 8 {
		t.Errorf("Expected NumCPU to be updated to 8")
	}
}

func TestIsClosedErr(t *testing.T) {
	if isClosedErr(nil) {
		t.Error("expected false for nil")
	}
	if isClosedErr(errors.New("random error")) {
		t.Error("expected false for random error")
	}
	if !isClosedErr(errors.New("use of closed network connection")) {
		t.Error("expected true for 'use of closed'")
	}
}

func TestModelUpdate_UDPMetrics_SetsConnected(t *testing.T) {
	m := model{}
	m3, _ := m.Update(udpMetricsMsg{NumCPU: 8})
	updated := m3.(model)
	if !updated.hotConnected {
		t.Error("expected hotConnected=true after udpMetricsMsg")
	}
	if updated.hotLastUpdate.IsZero() {
		t.Error("expected hotLastUpdate to be set")
	}
}

func TestModelUpdate_ReconnectMsg(t *testing.T) {
	m := model{}
	m3, _ := m.Update(reconnectMsg{port: 49802})
	updated := m3.(model)
	if updated.boundPort != 49802 {
		t.Errorf("expected boundPort=49802, got %d", updated.boundPort)
	}
}
