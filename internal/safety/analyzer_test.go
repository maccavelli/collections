package safety

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// VulnerableCode is a test fixture for injection detection.
func VulnerableCode(db *sql.DB, user string) {
	_, _ = db.Query("SELECT * FROM users WHERE name = '" + user + "'")
	_, _ = db.Exec(fmt.Sprintf("DELETE FROM users WHERE id = %s", user))
}

func TestDetectInjections(t *testing.T) {
	result, err := DetectInjections(context.Background(), ".")
	if err != nil {
		t.Fatalf("failed to detect injections: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if len(result.Vulnerabilities) < 2 {
		t.Fatalf("expected at least 2 vulnerabilities, got %d", len(result.Vulnerabilities))
	}

	foundQuery := false
	foundExec := false
	for _, v := range result.Vulnerabilities {
		if strings.Contains(v.Reason, "Query") {
			foundQuery = true
		}
		if strings.Contains(v.Reason, "Exec") {
			foundExec = true
		}
	}

	if !foundQuery {
		t.Errorf("expected Query vulnerability not found: %v", result.Vulnerabilities)
	}
	if !foundExec {
		t.Errorf("expected Exec vulnerability not found: %v", result.Vulnerabilities)
	}
}
