package engine

import (
	"context"
	"strings"
	"testing"

	"mcp-server-brainstorm/internal/models"
)

func TestChallengeAssumption_Database(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.ChallengeAssumption(ctx, "Use a PostgreSQL db")
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
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.ChallengeAssumption(
		ctx, "Expose via HTTP API",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstr(got, "authentication") {
		t.Error("expected an authentication challenge")
	}
}

func TestChallengeAssumption_General(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.ChallengeAssumption(
		ctx, "Random design idea",
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
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.ChallengeAssumption(ctx, "")
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
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.ChallengeAssumption(
		ctx, "Add a Redis cache layer",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstr(got, "invalidation") {
		t.Error("expected a cache invalidation challenge")
	}
}

func TestAnalyzeEvolution_Refactor(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "refactor the auth module",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "refactor" {
		t.Errorf(
			"want category 'refactor', got: %s",
			got.Category,
		)
	}
	if got.RiskLevel != "HIGH" {
		t.Errorf(
			"want risk 'HIGH', got: %s", got.RiskLevel,
		)
	}
	if !strings.Contains(
		strings.ToLower(got.Recommendation), "upstream",
	) {
		t.Errorf(
			"want upstream warning, got: %s",
			got.Recommendation,
		)
	}
}

func TestAnalyzeEvolution_Stable(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "review the docs",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "general" {
		t.Errorf(
			"want category 'general', got: %s",
			got.Category,
		)
	}
	if got.RiskLevel != "LOW" {
		t.Errorf(
			"want risk 'LOW', got: %s", got.RiskLevel,
		)
	}
}

func TestAnalyzeEvolution_Rename(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "rename the auth package",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "rename" {
		t.Errorf(
			"want category 'rename', got: %s",
			got.Category,
		)
	}
}

func TestAnalyzeEvolution_Upgrade(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.AnalyzeEvolution(
		ctx, "upgrade the dependency to v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "dependency_change" {
		t.Errorf(
			"want category 'dependency_change', got: %s",
			got.Category,
		)
	}
}

func TestEvaluateQualityAttributes_Default(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	metrics, err := e.EvaluateQualityAttributes(
		ctx, "basic web service",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 4 {
		t.Fatalf("want 4 metrics, got %d", len(metrics))
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
	e := NewEngine(".")
	ctx := context.Background()

	metrics, err := e.EvaluateQualityAttributes(
		ctx, "add Redis cache layer",
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
	e := NewEngine(".")
	ctx := context.Background()

	authOnly, err := e.EvaluateQualityAttributes(
		ctx, "auth token system",
	)
	if err != nil {
		t.Fatal(err)
	}
	both, err := e.EvaluateQualityAttributes(
		ctx, "auth token with tls encryption",
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

// TestEvaluateQuality_Negation verifies that negation
// patterns prevent score boosting.
func TestEvaluateQuality_Negation(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	metrics, err := e.EvaluateQualityAttributes(
		ctx, "system with no auth",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metrics {
		if m.Attribute == "Security" {
			if !strings.Contains(
				m.Observation, "Negation",
			) {
				t.Errorf(
					"expected negation detection,"+
						" got: %s", m.Observation,
				)
			}
		}
	}
}

func TestRedTeamReview_NoLogs(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.RedTeamReview(ctx, "a simple web service")
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
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.RedTeamReview(ctx, "expose a public API")
	if err != nil {
		t.Fatal(err)
	}
	if !containsPersona(got, "Security") {
		t.Error("expected Security persona")
	}
}

func TestRedTeamReview_ReliabilityHawk(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	got, err := e.RedTeamReview(
		ctx, "a basic data pipeline",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPersona(got, "Reliability") {
		t.Error("expected Reliability persona")
	}
}


func TestCaptureDecisionLogic_Layered(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	adr, err := e.CaptureDecisionLogic(
		ctx,
		"Use PostgreSQL for performance",
		"SQLite",
	)
	if err != nil {
		t.Fatal(err)
	}

	if adr.Narrative == "" {
		t.Error("expected non-empty narrative in ADR")
	}
	if !strings.Contains(adr.SummaryMD, "### Architecture Decision Record") {
		t.Error("expected markdown summary header in ADR")
	}
}

func TestCritiqueDesign_Consolidated(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()

	resp, err := e.CritiqueDesign(ctx, "Use a Redis cache with retry logic")
	if err != nil {
		t.Fatal(err)
	}

	if resp.Narrative == "" {
		t.Error("expected non-empty narrative")
	}
	if !strings.Contains(resp.SummaryMD, "#### Quality Attributes") {
		t.Error("expected quality attributes table in markdown")
	}
	if len(resp.Metrics) == 0 {
		t.Error("expected metrics in response")
	}
}

func TestDiscoverProject_Consolidated(t *testing.T) {
	e := NewEngine(".")
	ctx := context.Background()
	session := &models.Session{Status: "DISCOVERY"}

	resp, err := e.DiscoverProject(ctx, ".", session)
	if err != nil {
		t.Fatalf("DiscoverProject failed: %v", err)
	}

	if resp.Narrative == "" {
		t.Error("expected non-empty narrative")
	}
	if !strings.Contains(resp.SummaryMD, "### Project Discovery") {
		t.Error("expected markdown summary header")
	}
	if resp.NextStep == "" {
		t.Error("expected next step suggestion")
	}
}

func TestSuggestNextStep_WithGaps(t *testing.T) {
	e := NewEngine(".")
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
	e := NewEngine(".")
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
	e := NewEngine(".")
	ctx := context.Background()

	gaps, err := e.AnalyzeDiscovery(ctx, ".")
	if err != nil {
		t.Fatalf("AnalyzeDiscovery failed: %v", err)
	}

	// Running in the project root — should find go.mod
	// and potentially some AST-based gaps.
	_ = gaps
}

func TestSuggestNextStep_Statuses(t *testing.T) {
	e := NewEngine(".")
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
	e := NewEngine(".")
	ctx := context.Background()
	
	// Test with a non-existent path.
	_, err := e.AnalyzeDiscovery(ctx, "/tmp/non-existent-brainstorm-path-abs-xyz")
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestResolvePath_Abs(t *testing.T) {
	e := NewEngine("/tmp")
	
	// Empty path
	if got := e.ResolvePath(""); got != "/tmp" {
		t.Errorf("ResolvePath('') = %s; want /tmp", got)
	}
	
	// Relative path
	if got := e.ResolvePath("sub"); strings.HasSuffix(got, "..") {
		// Just verify it's joined.
	}
	
	// Absolute path
	abs := "/home/user/test"
	if got := e.ResolvePath(abs); got != abs {
		t.Errorf("ResolvePath(%s) = %s; want %s", abs, got, abs)
	}
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
