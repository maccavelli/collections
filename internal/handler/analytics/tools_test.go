package analytics

import (
	"context"
	"os"
	"testing"

	"mcp-server-brainstorm/internal/engine"
	"mcp-server-brainstorm/internal/models"
	"mcp-server-brainstorm/internal/state"

	"github.com/tidwall/buntdb"
)

// seedAnalyticsSession creates a seeded manager+engine for testing handler paths past LoadSession.
func seedAnalyticsSession(t *testing.T) (string, *state.Manager, *engine.Engine) {
	t.Helper()
	tmp, err := os.MkdirTemp("", "brainstorm-analytics-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })

	db, err := buntdb.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	mgr := state.NewManager(tmp)
	eng := engine.NewEngine(tmp, db)

	_ = mgr.SaveSession(context.Background(), &models.Session{
		ProjectRoot: tmp,
		Metadata:    map[string]any{"scope": "test"},
	})
	return tmp, mgr, eng
}

func TestAnalytics_RegisterAll(t *testing.T) {
	_, mgr, eng := seedAnalyticsSession(t)
	Register(mgr, eng)
}
