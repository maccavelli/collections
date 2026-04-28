package state

import (
	"testing"
)

func TestBadgerFallbackMock(t *testing.T) {
	tmpDir := t.TempDir()

	s, err := NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed store creation: %v", err)
	}
	defer s.Close()

	err = s.RecordEfficacy("", "test_skill", true)
	if err != nil {
		t.Errorf("failed record: %v", err)
	}

	stats, err := s.GetEfficacy("", "test_skill")
	if err != nil {
		t.Errorf("failed get: %v", err)
	}
	if stats.Successes != 1 {
		t.Error("expected 1 success")
	}

	// Unrecorded read
	_, _ = s.GetEfficacy("", "unknown")

	h, m := s.GetMetrics()
	if h != 1 || m != 1 {
		t.Errorf("metrics mismatch H:%d M:%d", h, m)
	}
}
