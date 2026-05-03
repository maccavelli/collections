// Package engine provides functionality for the engine subsystem.
package engine

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tidwall/buntdb"
	"mcp-server-brainstorm/internal/models"
)

// DefaultStandards contains the hardcoded Tier 3 fallback rules for the brainstorm
// toolsuite when running natively in standalone environments without the
// orchestrator's remote recall cluster access.
var DefaultStandards = map[string]string{
	"critique_design":         "Evaluate anti-patterns, edge cases, and failure modes across proposed architectural footprints. Enforce rigid boundaries.",
	"analyze_evolution":       "Calculate technical debt vectors, blast radius impacts, and forward-compatibility horizons for proposed systems.",
	"clarify_requirements":    "Isolate unstated assumptions, force ambiguity resolution, and map non-functional constraints securely before implementation.",
	"discovery_architectural": "Orient the intelligence space around baseline package patterns, Go idiomatic hierarchies, and domain-driven design contours.",
	"thesis_architect":        "Evaluate Go codebases across 6 dimensions: type safety & generics, modernization, modularization, efficiency, reliability, and maintainability. Advocate for maximum Go 1.26.1 adoption.",
	"antithesis_skeptic":      "Challenge proposed changes across the same 6 dimensions for unjustified complexity, runtime regression, abstraction overhead, cohesion loss, and stability risk.",
	"aporia_engine":           "Moderate thesis/antithesis contradictions. Detect Aporia points where both arguments are valid but mutually exclusive. Determine the safe path forward.",
}

// SyncWrapper creates a JSON shell for data caching guaranteeing soft invalidation.
type SyncWrapper struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Data      string    `json:"data"`
}

// EnsureRecallCache coordinates lazy-fetching of role-specific data from Recall,
// ensuring only one inflight request per session/role.
func (e *Engine) EnsureRecallCache(ctx context.Context, session *models.Session, role string, toolName string, arguments map[string]any) string {
	cacheKey := "recall_cache_" + role
	syncKey := "recall_sync_" + role

	var staleVal string
	var returnFresh bool

	if e.DB != nil {
		if err := e.DB.View(func(tx *buntdb.Tx) error {
			val, err := tx.Get(cacheKey)
			if err == nil && val != "" {
				var wrapper SyncWrapper
				if decodeErr := json.Unmarshal([]byte(val), &wrapper); decodeErr == nil {
					staleVal = wrapper.Data
					if time.Since(wrapper.UpdatedAt) < 1*time.Hour {
						returnFresh = true
					}
				} else {
					staleVal = val     // Legacy string fallback
					returnFresh = true // Treat legacy existing cache as 'needs revalidating? no, just stale so it falls through to update.
					returnFresh = false
				}
			}
			return err
		}); err != nil {
			slog.Debug("BuntDB view bypassed or errored", "role", role, "err", err)
		}
		if returnFresh {
			slog.Debug("Recall BuntDB cache hit (fresh)", "role", role, "size", len(staleVal))
			return staleVal
		}
	}

	e.mu.Lock()
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}

	if pending, ok := session.Metadata[syncKey].(*sync.WaitGroup); ok {
		e.mu.Unlock()
		pending.Wait()

		if e.DB != nil {
			var postWaitVal string
			_ = e.DB.View(func(tx *buntdb.Tx) error {
				val, err := tx.Get(cacheKey)
				if err == nil && val != "" {
					var wrapper SyncWrapper
					if err := json.Unmarshal([]byte(val), &wrapper); err == nil {
						postWaitVal = wrapper.Data
					} else {
						postWaitVal = val
					}
				}
				return nil
			})
			if postWaitVal != "" {
				slog.Debug("Recall BuntDB cache hit (after wait)", "role", role, "size", len(postWaitVal))
				return postWaitVal
			}
		}
		return staleVal // Fallback gracefully if Wait returned nothing new
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	session.Metadata[syncKey] = wg
	e.mu.Unlock()

	e.mu.RLock()
	client := e.ExternalClient
	e.mu.RUnlock()

	var globalRecall string
	if client != nil && client.RecallEnabled() {
		globalRecall = client.CallDatabaseTool(ctx, toolName, arguments)
	}

	var localStandards string
	if cacheDir, err := os.UserCacheDir(); err == nil {
		localPath := filepath.Join(cacheDir, "brainstorm", "standards", role+".md")
		if b, err := os.ReadFile(localPath); err == nil {
			localStandards = string(b)
		}
	}

	hardcodedFallback := DefaultStandards[role]

	var val string
	if localStandards != "" || globalRecall != "" || hardcodedFallback != "" {
		val = "<STANDARDS_HIERARCHY>\n"

		isOrchestrator := globalRecall != ""

		if isOrchestrator {
			if globalRecall != "" {
				val += "  <PRIORITY_1_GLOBAL_RECALL>\n" + globalRecall + "\n  </PRIORITY_1_GLOBAL_RECALL>\n"
			}
			if localStandards != "" {
				val += "  <PRIORITY_2_FILESYSTEM_STANDARDS>\n" + localStandards + "\n  </PRIORITY_2_FILESYSTEM_STANDARDS>\n"
			}
			if hardcodedFallback != "" {
				val += "  <PRIORITY_3_HARDCODED_HEURISTICS>\n" + hardcodedFallback + "\n  </PRIORITY_3_HARDCODED_HEURISTICS>\n"
			}
			val += "  <INSTRUCTION>\n"
			val += "    Apply highest priority rules first. Global recall constraints > Local filesystem standard overrides > hard-coded engine defaults.\n"
			val += "  </INSTRUCTION>\n"
		} else {
			if localStandards != "" {
				val += "  <PRIORITY_1_FILESYSTEM_STANDARDS>\n" + localStandards + "\n  </PRIORITY_1_FILESYSTEM_STANDARDS>\n"
			}
			if hardcodedFallback != "" {
				val += "  <PRIORITY_2_HARDCODED_HEURISTICS>\n" + hardcodedFallback + "\n  </PRIORITY_2_HARDCODED_HEURISTICS>\n"
			}
			val += "  <INSTRUCTION>\n"
			val += "    Apply highest priority rules first. Local filesystem standard overrides > hard-coded engine defaults.\n"
			val += "  </INSTRUCTION>\n"
		}
		val += "</STANDARDS_HIERARCHY>"
	}

	if val != "" {
		slog.Debug("Recall cache miss (fetched from remote)", "role", role, "size", len(val))
		if e.DB != nil {
			wrapper := SyncWrapper{
				UpdatedAt: time.Now(),
				Data:      val,
			}
			if b, err := json.Marshal(wrapper); err == nil {
				_ = e.DB.Update(func(tx *buntdb.Tx) error {
					_, _, err := tx.Set(cacheKey, string(b), nil) // Infinite TTL Edge Caching
					return err
				})
			}
		}
	} else if staleVal != "" {
		slog.Warn("mcp server dropped standard sync - deploying standalone using stale DB context", "role", role)
		val = staleVal
	}

	e.mu.Lock()
	delete(session.Metadata, syncKey)
	e.mu.Unlock()

	wg.Done()
	return val
}
