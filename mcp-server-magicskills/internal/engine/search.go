package engine

import (
	"context"
	"fmt"
	"iter"
	"math"
	"slices"
	"time"

	"mcp-server-magicskills/internal/models"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
)

func (e *Engine) GetSkill(name string) (*models.Skill, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	slugName := slugify(name)
	s, ok := e.Skills[slugName]
	if ok {
		return s, true
	}
	// Fallback: search bleve for 1 result
	q := bleve.NewMatchQuery(name)
	req := bleve.NewSearchRequestOptions(q, 1, 0, false)
	res, err := e.Bleve.Search(req)
	if err == nil && len(res.Hits) > 0 {
		if best, ok := e.Skills[res.Hits[0].ID]; ok {
			return best, true
		}
	}
	return nil, false
}

func (e *Engine) SkillCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.Skills)
}

func (e *Engine) AllSkills() iter.Seq[*models.Skill] {
	return func(yield func(*models.Skill) bool) {
		e.mu.RLock()
		defer e.mu.RUnlock()
		for _, s := range e.Skills {
			if !yield(s) {
				return
			}
		}
	}
}

type ScoredSkill struct {
	Skill *models.Skill `json:"skill"`
	Score float64       `json:"score"`
}

// MatchSkills returns skills matched via Bleve, scored with Wilson confidence and recency boost.
func (e *Engine) MatchSkills(ctx context.Context, intent string, filterCategory string, targetWorkspace string, matchLimit int) []ScoredSkill {
	if intent == "" {
		return nil
	}

	cacheKey := fmt.Sprintf("%s:%s:%s", intent, filterCategory, targetWorkspace)
	if e.MatchCache != nil {
		if val, ok := e.MatchCache.Get(cacheKey); ok {
			return val
		}
	}

	if matchLimit <= 0 {
		matchLimit = 3
	}

	contentQ := bleve.NewMatchQuery(intent)
	contentQ.SetField("content")
	contentQ.SetBoost(1.0)

	descQ := bleve.NewMatchQuery(intent)
	descQ.SetField("description")
	descQ.SetBoost(2.0)

	nameQ := bleve.NewMatchQuery(intent)
	nameQ.SetField("name")
	nameQ.SetBoost(5.0)

	tagQ := bleve.NewMatchQuery(intent)
	tagQ.SetField("tags")
	tagQ.SetBoost(3.0)

	var q query.Query = bleve.NewDisjunctionQuery(contentQ, descQ, nameQ, tagQ)

	if filterCategory != "" {
		catQ := bleve.NewMatchQuery(filterCategory)
		catQ.SetField("context_domain")
		q = bleve.NewConjunctionQuery(q, catQ)
	}

	if targetWorkspace != "" {
		wsQ := bleve.NewMatchPhraseQuery(targetWorkspace)
		wsQ.SetField("workspace")
		q = bleve.NewConjunctionQuery(q, wsQ)
	}

	req := bleve.NewSearchRequestOptions(q, matchLimit*3, 0, false)
	res, err := e.Bleve.Search(req)
	if err != nil || len(res.Hits) == 0 {
		return nil
	}

	maxScore := res.Hits[0].Score
	if maxScore <= 0 {
		maxScore = 1.0
	}

	// Copy skill pointers under read lock, then release before efficacy lookups
	type hitInfo struct {
		skill *models.Skill
		id    string
		score float64
	}
	e.mu.RLock()
	hits := make([]hitInfo, 0, len(res.Hits))
	for _, h := range res.Hits {
		sk, ok := e.Skills[h.ID]
		if ok {
			hits = append(hits, hitInfo{skill: sk, id: h.ID, score: h.Score / maxScore})
		}
	}
	e.mu.RUnlock()

	// Score with Wilson confidence and recency boost (lock-free)
	now := time.Now()
	var matches []ScoredSkill
	for _, h := range hits {
		confidence := 0.5 // neutral prior for skills with no efficacy data
		recency := 1.0

		stats, err := e.Store.GetEfficacy(targetWorkspace, h.id)
		if err == nil {
			confidence = wilsonLowerBound(stats.Successes, stats.Failures)
			if !stats.LastSuccessAt.IsZero() {
				days := now.Sub(stats.LastSuccessAt).Hours() / 24
				recency = 1.0 + 0.2*math.Max(0, 1-days/30)
			}
		}

		matches = append(matches, ScoredSkill{
			Skill: h.skill,
			Score: h.score * confidence * recency,
		})
	}

	slices.SortFunc(matches, func(a, b ScoredSkill) int {
		if b.Score > a.Score {
			return 1
		}
		if b.Score < a.Score {
			return -1
		}
		return 0
	})

	limit := matchLimit
	if len(matches) < limit {
		limit = len(matches)
	}

	result := make([]ScoredSkill, limit)
	for i := 0; i < limit; i++ {
		result[i] = matches[i]
	}

	if e.MatchCache != nil {
		e.MatchCache.Add(cacheKey, result)
	}
	return result
}

// wilsonLowerBound calculates the lower bound of a Wilson score confidence interval.
// Uses z=1.96 (95% confidence). Returns 0.5 (neutral) when n=0.
func wilsonLowerBound(successes, failures int) float64 {
	n := float64(successes + failures)
	if n == 0 {
		return 0.5
	}
	const z = 1.96
	p := float64(successes) / n
	denominator := 1 + z*z/n
	centre := p + z*z/(2*n)
	spread := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n))
	return (centre - spread) / denominator
}

func (e *Engine) Summarize(ctx context.Context, name string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	slugName := slugify(name)
	s, ok := e.Skills[slugName]
	if !ok {
		return "", false
	}

	for _, key := range []string{"magic directive", "directive", "summary"} {
		if dir, ok := s.Sections[key]; ok {
			return dir, true
		}
	}

	full := s.Sections["full"]
	if len(full) > 300 {
		return full[:300] + "...", true
	}
	return full, true
}
