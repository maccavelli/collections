package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mcp-server-brainstorm/internal/models"
)

// CaptureDecisionLogic generates an Architecture Decision
// Record from the provided decision context and
// alternatives.
func (e *Engine) CaptureDecisionLogic(
	ctx context.Context,
	decision string,
	alternatives string,
) (models.ADR, error) {
	select {
	case <-ctx.Done():
		return models.ADR{}, ctx.Err()
	default:
	}
	// Derive a title from the first 80 chars.
	title := decision
	if len(title) > 80 {
		title = title[:80] + "..."
	}

	// Generate consequences from the decision text.
	lower := strings.ToLower(decision)
	consequences := "Requires consistent" +
		" implementation across the team."
	if strings.Contains(lower, "performance") {
		consequences = "May add complexity but" +
			" improves runtime performance."
	}
	if strings.Contains(lower, "simplicity") ||
		strings.Contains(lower, "simple") {
		consequences = "Reduces maintenance cost" +
			" but may limit future extensibility."
	}

	res := models.ADR{
		ID: fmt.Sprintf(
			"ADR-%d", time.Now().Unix(),
		),
		Title:              title,
		Date:               time.Now(),
		Status:             "PROPOSED",
		Context:            decision,
		Decision:           decision,
		RejectedAlternates: alternatives,
		Consequences:       consequences,
	}

	res.Narrative = fmt.Sprintf(
		"Captured architectural decision: %s", title,
	)

	var sb strings.Builder
	sb.WriteString("### Architecture Decision Record\n\n")
	sb.WriteString(fmt.Sprintf("- **Title**: %s\n", title))
	sb.WriteString(fmt.Sprintf("- **Status**: %s\n", res.Status))
	sb.WriteString(fmt.Sprintf("- **Consequences**: %s\n", consequences))
	res.SummaryMD = sb.String()

	return res, nil
}
