package engine

import (
	"context"
	"time"
)

// StartEfficacyWatcher routinely audits skill BadgerDB efficacy records.
func (e *Engine) StartEfficacyWatcher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial audit immediately
	e.auditEfficacy(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.auditEfficacy(ctx)
		}
	}
}

func (e *Engine) auditEfficacy(ctx context.Context) {
	e.mu.RLock()
	slugs := make([]string, 0, len(e.Skills))
	for slug := range e.Skills {
		slugs = append(slugs, slug)
	}
	e.mu.RUnlock()

	var broken []string
	for _, slug := range slugs {
		if ctx.Err() != nil {
			return
		}
		stats, err := e.Store.GetEfficacy("", slug)
		if err == nil && stats.Failures >= 3 {
			broken = append(broken, slug)
		}
	}

	e.mu.Lock()
	e.BrokenSkills = broken
	e.mu.Unlock()
}

// GetBrokenSkills returns a thread-safe copy of degraded skills
func (e *Engine) GetBrokenSkills() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	broken := make([]string, len(e.BrokenSkills))
	copy(broken, e.BrokenSkills)
	return broken
}
