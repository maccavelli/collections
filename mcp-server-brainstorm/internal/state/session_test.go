package state

import (
	"context"
	"testing"

	"mcp-server-brainstorm/internal/models"
)

func TestLoadAndSaveSession(t *testing.T) {
	mgr := NewManager("/tmp/test-project")
	ctx := context.Background()

	// First load creates a fresh session.
	session, err := mgr.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session.Status != "DISCOVERY" {
		t.Errorf("expected DISCOVERY, got %s", session.Status)
	}

	// Mutate and save.
	session.Status = "CLARIFICATION"
	session.Gaps = append(session.Gaps, models.Gap{
		Area: "TEST", Description: "test gap", Severity: "HIGH",
	})
	if err := mgr.SaveSession(ctx, session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	// Reload should return the same mutated session.
	session2, err := mgr.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if session2.Status != "CLARIFICATION" {
		t.Errorf("expected CLARIFICATION, got %s", session2.Status)
	}
	if len(session2.Gaps) != 1 {
		t.Errorf("expected 1 gap, got %d", len(session2.Gaps))
	}
}

func TestLoadSession_ContextCancelled(t *testing.T) {
	mgr := NewManager("/tmp/test-project")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.LoadSession(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestSaveSession_ContextCancelled(t *testing.T) {
	mgr := NewManager("/tmp/test-project")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.SaveSession(ctx, &models.Session{})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestMultipleProjectRoots(t *testing.T) {
	mgr1 := NewManager("/tmp/project-a")
	mgr2 := NewManager("/tmp/project-b")
	ctx := context.Background()

	s1, _ := mgr1.LoadSession(ctx)
	s1.Status = "PROJECT_A"
	_ = mgr1.SaveSession(ctx, s1)

	s2, _ := mgr2.LoadSession(ctx)
	if s2.Status != "DISCOVERY" {
		t.Errorf("project-b should have fresh session, got %s", s2.Status)
	}
}
