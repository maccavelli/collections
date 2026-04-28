package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcp-server-brainstorm/internal/models"

	"github.com/tidwall/buntdb"
)

func TestChallengeAssumption_Database(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.ChallengeAssumption(ctx, "Use a PostgreSQL db", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("want >=2 challenges, got %d", len(got))
	}
	if !containsSubstr(got, "retry strategy") {
		t.Error("expected a retry strategy challenge")
	}
}

func TestChallengeAssumption_API(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.ChallengeAssumption(
		ctx, "Expose via HTTP API", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstr(got, "authentication") {
		t.Error("expected an authentication challenge")
	}
}

func TestChallengeAssumption_General(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.ChallengeAssumption(
		ctx, "Random design idea", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one challenge")
	}
	if !containsSubstr(got, "latency") {
		t.Error(
			"expected a latency challenge for" +
				" generic input",
		)
	}
}

func TestChallengeAssumption_Empty(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.ChallengeAssumption(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal(
			"empty input should still produce" +
				" general challenges",
		)
	}
}

func TestChallengeAssumption_Cache(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.ChallengeAssumption(
		ctx, "Add a Redis cache layer", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstr(got, "invalidation") {
		t.Error("expected a cache invalidation challenge")
	}
}

func TestAnalyzeEvolution_Refactor(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "refactor the auth module", "", map[string]interface{}{
			"total_nodes":     600.0,
			"total_functions": 25.0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.Category != "large_scale_refactor" {
		t.Errorf(
			"want category 'large_scale_refactor', got: %s",
			got.Data.Category,
		)
	}
	if got.Data.RiskLevel != "HIGH" {
		t.Errorf(
			"want risk 'HIGH', got: %s", got.Data.RiskLevel,
		)
	}
	if !strings.Contains(
		strings.ToLower(got.Data.Recommendation), "smaller",
	) {
		t.Errorf(
			"want smaller merge logic warning, got: %s",
			got.Data.Recommendation,
		)
	}
}

func TestAnalyzeEvolution_Stable(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "review the docs", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.Category != "general" {
		t.Errorf(
			"want category 'general', got: %s",
			got.Data.Category,
		)
	}
	if got.Data.RiskLevel != "LOW" {
		t.Errorf(
			"want risk 'LOW', got: %s", got.Data.RiskLevel,
		)
	}
}

func TestAnalyzeEvolution_Displacement(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "split the auth package", "", map[string]interface{}{
			"total_nodes": 150.0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.Category != "structural_displacement" {
		t.Errorf(
			"want category 'structural_displacement', got: %s",
			got.Data.Category,
		)
	}
	if got.Data.RiskLevel != "MEDIUM" {
		t.Errorf(
			"want risk 'MEDIUM', got: %s", got.Data.RiskLevel,
		)
	}
}

func TestEvaluateQualityAttributes_Default(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	metrics, err := e.EvaluateQualityAttributes(
		ctx, "basic web service", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 5 {
		t.Fatalf("want 5 metrics, got %d", len(metrics))
	}

	for _, m := range metrics {
		if m.Attribute == "" {
			t.Error("empty attribute name")
		}
		if m.Score < 1 || m.Score > 10 {
			t.Errorf(
				"score out of range for %s: %d",
				m.Attribute, m.Score,
			)
		}
	}
}

func TestEvaluateQualityAttributes_WithCache(
	t *testing.T,
) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	metrics, err := e.EvaluateQualityAttributes(
		ctx, "add Redis cache layer", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metrics {
		if m.Attribute == "Scalability" && m.Score < 7 {
			t.Errorf(
				"cache input should boost scalability,"+
					" got score %d", m.Score,
			)
		}
	}
}

// TestEvaluateQuality_AdditiveScoring verifies that
// mentioning multiple keywords produces a higher score
// than either keyword alone.
func TestEvaluateQuality_AdditiveScoring(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	authOnly, err := e.EvaluateQualityAttributes(
		ctx, "auth token system", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	both, err := e.EvaluateQualityAttributes(
		ctx, "auth token with tls encryption", nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	var authScore, bothScore int
	for _, m := range authOnly {
		if m.Attribute == "Security" {
			authScore = m.Score
		}
	}
	for _, m := range both {
		if m.Attribute == "Security" {
			bothScore = m.Score
		}
	}

	if bothScore <= authScore {
		t.Errorf(
			"both auth+encrypt (%d) should score"+
				" higher than auth-only (%d)",
			bothScore, authScore,
		)
	}
}

// TestEvaluateQuality_EmpiricalAST verifies that AST footprints
// properly override string scanning bonuses (Angle 6).
func TestEvaluateQuality_EmpiricalAST(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	// Provide an AST import array natively mapped to crypto edges
	metrics, err := e.EvaluateQualityAttributes(
		ctx, "general logic", map[string]interface{}{
			"imports": []interface{}{
				"crypto/sha256",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range metrics {
		if m.Attribute == "Security" {
			if !strings.Contains(
				m.Observation, "AST:",
			) {
				t.Errorf(
					"expected empirical AST execution path,"+
						" got: %s", m.Observation,
				)
			}
		}
	}
}

func TestRedTeamReview_NoLogs(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.RedTeamReview(ctx, "a simple web service", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one challenge")
	}
	if !containsPersona(got, "Maintenance") {
		t.Error("expected Maintenance persona")
	}
}

func TestRedTeamReview_WithAPI(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.RedTeamReview(ctx, "expose a public API", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPersona(got, "Security") {
		t.Error("expected Security persona")
	}
}

func TestRedTeamReview_ReliabilityHawk(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	got, err := e.RedTeamReview(
		ctx, "a basic data pipeline", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPersona(got, "Reliability") {
		t.Error("expected Reliability persona")
	}
}

func TestCaptureDecisionLogic_Layered(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	adr, err := e.CaptureDecisionLogic(
		ctx,
		"Use PostgreSQL for performance",
		"SQLite", "",
	)
	if err != nil {
		t.Fatal(err)
	}

	if adr.Data.Narrative == "" {
		t.Error("expected non-empty narrative in ADR")
	}
	if !strings.Contains(adr.Summary, "ADR Drafted") {
		t.Error("expected summary to contain draft status")
	}
}

func TestCritiqueDesign_Consolidated(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	// Simple query that might hit cache or DB.
	resp, err := e.CritiqueDesign(ctx, "Use a Redis cache with retry logic", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Data.Narrative == "" {
		t.Error("expected non-empty narrative")
	}
	if len(resp.Data.Metrics) == 0 {
		t.Error("expected metrics in response")
	}
}

func TestDiscoverProject_Consolidated(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()
	session := &models.Session{Status: "DISCOVERY"}

	resp, err := e.DiscoverProject(ctx, ".", session)
	if err != nil {
		t.Fatalf("DiscoverProject failed: %v", err)
	}

	if resp.Data.Narrative == "" {
		t.Error("expected non-empty narrative")
	}
	if resp.Data.NextStep == "" {
		t.Error("expected next step suggestion")
	}
}

func TestSuggestNextStep_WithGaps(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	session := &models.Session{
		Status: "DISCOVERY",
		Gaps: []models.Gap{
			{
				Area:        "CONTEXT",
				Description: "No README found.",
				Severity:    "CRITICAL",
			},
		},
	}

	got, err := e.SuggestNextStep(ctx, session, "")
	if err != nil {
		t.Fatalf("SuggestNextStep failed: %v", err)
	}
	if !strings.Contains(got, "CONTEXT") {
		t.Errorf("want gap reference, got: %s", got)
	}
}

func TestSuggestNextStep_Discovery(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	session := &models.Session{
		Status: "DISCOVERY",
		Gaps:   []models.Gap{},
	}

	got, err := e.SuggestNextStep(ctx, session, "")
	if err != nil {
		t.Fatalf("SuggestNextStep failed: %v", err)
	}
	if !strings.Contains(got, "Purpose") {
		t.Errorf("want Purpose prompt, got: %s", got)
	}
}

func TestAnalyzeDiscovery(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	session := &models.Session{Metadata: make(map[string]any)}
	gaps, err := e.AnalyzeDiscovery(ctx, ".", session)
	if err != nil {
		t.Fatalf("AnalyzeDiscovery failed: %v", err)
	}

	// Running in the project root — should find go.mod
	// and potentially some AST-based gaps.
	_ = gaps
}

func TestSuggestNextStep_Statuses(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	tests := []struct {
		status   string
		contains string
	}{
		{"CLARIFICATION", "Constraints"},
		{"UNKNOWN", "Iterative"},
	}

	for _, tc := range tests {
		session := &models.Session{Status: tc.status}
		got, err := e.SuggestNextStep(ctx, session, "")
		if err != nil {
			t.Errorf("status %s: SuggestNextStep failed: %v", tc.status, err)
			continue
		}
		if !strings.Contains(got, tc.contains) {
			t.Errorf("status %s: want %s, got %s", tc.status, tc.contains, got)
		}
	}
}

func TestAnalyzeDiscovery_DepthAndSkip(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()
	e := NewEngine(".", db)
	ctx := context.Background()

	// Test with a non-existent path.
	session := &models.Session{Metadata: make(map[string]any)}
	_, err := e.AnalyzeDiscovery(ctx, "/tmp/non-existent-brainstorm-path-abs-xyz", session)
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestResolvePath_Abs(t *testing.T) {
	db, _ := buntdb.Open(":memory:")
	defer db.Close()

	// Create a temp project with go.mod
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0644)
	os.MkdirAll(filepath.Join(tmp, "internal", "handler"), 0755)

	e := NewEngine(tmp, db)

	// Empty path returns project root
	if got := e.ResolvePath(""); got != tmp {
		t.Errorf("ResolvePath('') = %s; want %s", got, tmp)
	}

	// Subdirectory resolves to module root via sentinel walk-up
	sub := filepath.Join(tmp, "internal", "handler")
	if got := e.ResolvePath(sub); got != tmp {
		t.Errorf("ResolvePath(%s) = %s; want %s (sentinel walk-up)", sub, got, tmp)
	}
}

func TestFindProjectRoot(t *testing.T) {
	t.Run("go.mod sentinel", func(t *testing.T) {
		tmp := t.TempDir()
		os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0644)
		os.MkdirAll(filepath.Join(tmp, "a", "b", "c"), 0755)

		root, kind := findProjectRoot(filepath.Join(tmp, "a", "b", "c"))
		if root != tmp {
			t.Errorf("want root %s, got %s", tmp, root)
		}
		if kind != "module" {
			t.Errorf("want kind 'module', got %s", kind)
		}
	})

	t.Run(".git sentinel", func(t *testing.T) {
		tmp := t.TempDir()
		os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
		os.MkdirAll(filepath.Join(tmp, "src", "pkg"), 0755)

		root, kind := findProjectRoot(filepath.Join(tmp, "src", "pkg"))
		if root != tmp {
			t.Errorf("want root %s, got %s", tmp, root)
		}
		if kind != "vcs" {
			t.Errorf("want kind 'vcs', got %s", kind)
		}
	})

	t.Run("go.mod takes priority over .git", func(t *testing.T) {
		tmp := t.TempDir()
		os.MkdirAll(filepath.Join(tmp, ".git"), 0755)
		modDir := filepath.Join(tmp, "services", "api")
		os.MkdirAll(filepath.Join(modDir, "internal"), 0755)
		os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module api\n"), 0644)

		root, kind := findProjectRoot(filepath.Join(modDir, "internal"))
		if root != modDir {
			t.Errorf("want root %s (go.mod), got %s", modDir, root)
		}
		if kind != "module" {
			t.Errorf("want kind 'module', got %s", kind)
		}
	})

	t.Run("fallback returns input", func(t *testing.T) {
		tmp := t.TempDir()
		deep := filepath.Join(tmp, "no", "sentinel", "here")
		os.MkdirAll(deep, 0755)

		// Note: t.TempDir() is inside /tmp which may have .git above it.
		// The fallback case is hard to test in real filesystems, but we
		// can at least verify the function doesn't panic.
		root, _ := findProjectRoot(deep)
		if root == "" {
			t.Error("root should never be empty")
		}
	})
}

// containsSubstr checks if any string in the slice
// contains the given substring (case-insensitive).
func containsSubstr(
	items []string, substr string,
) bool {
	lower := strings.ToLower(substr)
	for _, item := range items {
		if strings.Contains(
			strings.ToLower(item), lower,
		) {
			return true
		}
	}
	return false
}

// containsPersona checks if any RedTeamChallenge in the
// slice has the given persona (case-insensitive).
func containsPersona(
	items []models.RedTeamChallenge, persona string,
) bool {
	lower := strings.ToLower(persona)
	for _, item := range items {
		if strings.Contains(
			strings.ToLower(item.Persona), lower,
		) {
			return true
		}
	}
	return false
}
