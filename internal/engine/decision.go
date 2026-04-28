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
	standards string,
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

	// Fallback heuristic generation.
	lower := strings.ToLower(decision)
	consequences := "Requires consistent implementation across the team."
	if strings.Contains(lower, "performance") {
		consequences = "May add complexity but improves runtime performance."
	}
	if strings.Contains(lower, "simplicity") || strings.Contains(lower, "simple") {
		consequences = "Reduces maintenance cost but may limit future extensibility."
	}

	if standards != "" {
		consequences = fmt.Sprintf("%s\nMust strictly align with enterprise standards.", consequences)
	}

	id := fmt.Sprintf("ADR-%d", time.Now().Unix())
	res := models.ADR{
		Summary: fmt.Sprintf("ADR Drafted: %s", title),
		Data: struct {
			ID                 string    `json:"id"`
			Title              string    `json:"title"`
			Date               time.Time `json:"date"`
			Status             string    `json:"status"`
			Decision           string    `json:"decision"`
			RejectedAlternates string    `json:"rejected_alternates"`
			Consequences       string    `json:"consequences"`
			Narrative          string    `json:"narrative,omitempty"`
		}{
			ID:                 id,
			Title:              title,
			Date:               time.Now(),
			Status:             "PROPOSED",
			Decision:           decision,
			RejectedAlternates: alternatives,
			Consequences:       consequences,
			Narrative:          fmt.Sprintf("Captured architectural decision: %s", title),
		},
	}

	return res, nil
}
