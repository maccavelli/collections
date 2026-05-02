package engine

import (
	"context"
	"github.com/tidwall/buntdb"
	"testing"
)

type mockRecallClient struct {
	recallEnabled bool
	callCount     int
}

func (m *mockRecallClient) RecallEnabled() bool {
	return m.recallEnabled
}

func (m *mockRecallClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]any) string {
	m.callCount++
	if toolName == "list" {
		return `{"entries": [{"key": "test", "record": {"content": "{\"status\": \"found\"}"}}]}`
	}
	return "ok"
}

func (m *mockRecallClient) AggregateSessionFromRecall(ctx context.Context, serverID, projectID string) (map[string]any, error) {
	return map[string]any{"status": "aggregated"}, nil
}

func (m *mockRecallClient) SaveSession(ctx context.Context, sessionID, projectID string, payload any) error {
	return nil
}

func TestPublishSessionToRecall(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)

	mock := &mockRecallClient{recallEnabled: true}
	e.ExternalClient = mock

	ctx := context.Background()
	e.PublishSessionToRecall(ctx, "session-1", "project-1", "success", "gpt-4", "trace", "fragment", nil)

	if mock.callCount == 0 {
		t.Error("expected CallDatabaseTool to be called")
	}
}

func TestLoadCrossSessionFromRecall(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)

	mock := &mockRecallClient{recallEnabled: true}
	e.ExternalClient = mock

	ctx := context.Background()
	res := e.LoadCrossSessionFromRecall(ctx, "peer", "project-1")

	if res == "" {
		t.Error("expected non-empty result")
	}
	if mock.callCount == 0 {
		t.Error("expected CallDatabaseTool to be called")
	}
}
