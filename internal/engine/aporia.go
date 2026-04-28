package engine

import (
	"context"
	"fmt"

	"mcp-server-brainstorm/internal/models"
)

// ResolveSafePath cross-references thesis and antithesis pillar pairs
// to determine the safe path forward for each dimension.
func (e *Engine) ResolveSafePath(
	ctx context.Context,
	thesis models.ThesisDocument,
	counter models.CounterThesisReport,
) []models.AporiaResolution {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	thesisByName := make(map[string]models.DialecticPillar, len(thesis.Data.Pillars))
	for _, p := range thesis.Data.Pillars {
		thesisByName[p.Name] = p
	}

	skepticByName := make(map[string]models.DialecticPillar, len(counter.Pillars))
	for _, p := range counter.Pillars {
		skepticByName[p.Name] = p
	}

	resolutions := make([]models.AporiaResolution, 0, 6)

	for _, name := range pillarNames {
		tp := thesisByName[name]
		sp := skepticByName[name]

		resolution, contradiction, action := resolveConflict(tp.Score, sp.Score)

		resolutions = append(resolutions, models.AporiaResolution{
			Pillar:         name,
			ThesisScore:    tp.Score,
			SkepticScore:   sp.Score,
			Contradiction:  contradiction,
			Resolution:     resolution,
			SafePathAction: action,
		})
	}

	return resolutions
}

// resolveConflict determines the resolution for a single pillar pair.
func resolveConflict(thesisScore, skepticScore int) (resolution string, contradiction bool, action string) {
	switch {
	case thesisScore >= 7 && skepticScore >= 7:
		return "ADOPT", false,
			"Both thesis and antithesis agree: beneficial and safe to adopt"

	case thesisScore >= 7 && skepticScore >= 4 && skepticScore < 7:
		return "ADOPT_WITH_MITIGATION", false,
			"Beneficial but risks identified — adopt with skeptic's mitigations applied"

	case thesisScore >= 7 && skepticScore < 4:
		return "APORIA", true,
			"True paradox: technically valid improvement with equally valid risk concerns — requires human judgment"

	case thesisScore >= 4 && thesisScore < 7 && skepticScore >= 7:
		return "SKIP", false,
			"Low benefit with low risk of change — not worth the effort"

	case thesisScore >= 4 && thesisScore < 7 && skepticScore >= 4 && skepticScore < 7:
		return "REVIEW", false,
			"Ambiguous signal from both sides — needs more data or context"

	default:
		return "SKIP", false,
			"Thesis does not identify strong improvement opportunity"
	}
}

// ComputeSafePathVerdict derives the overall safe path verdict from resolutions.
func ComputeSafePathVerdict(resolutions []models.AporiaResolution) string {
	if len(resolutions) == 0 {
		return "REVIEW"
	}

	hasAporia := false
	hasReject := false

	for _, r := range resolutions {
		if r.Resolution == "APORIA" {
			hasAporia = true
		}
		if r.Contradiction {
			hasReject = true
		}
	}

	switch {
	case hasReject:
		return "REJECT"
	case hasAporia:
		return "REVIEW"
	default:
		return "APPROVE"
	}
}

// FormatSafePathNarrative builds a human-readable summary of all resolutions.
func FormatSafePathNarrative(resolutions []models.AporiaResolution) string {
	if len(resolutions) == 0 {
		return "No pillar data available for safe path analysis."
	}
	var b []byte
	b = append(b, "## Safe Path Analysis\n\n"...)
	for _, r := range resolutions {
		line := fmt.Sprintf("### %s [%s]\n- Thesis: %d/10 | Skeptic: %d/10\n- %s\n\n",
			r.Pillar, r.Resolution, r.ThesisScore, r.SkepticScore, r.SafePathAction)
		b = append(b, line...)
	}
	return string(b)
}
