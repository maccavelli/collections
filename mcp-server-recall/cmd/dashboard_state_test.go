package cmd

import "testing"

func TestDashboardState(t *testing.T) {
	state := NewUIState()
	if state.GetActiveTab() != 1 {
		t.Errorf("expected 1, got %d", state.GetActiveTab())
	}

	state.SetActiveTab(2)
	if state.GetActiveTab() != 2 {
		t.Errorf("expected 2, got %d", state.GetActiveTab())
	}
}
