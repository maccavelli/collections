package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"golang.org/x/sync/errgroup"
	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/llm"
	"mcp-server-magictools/internal/util"
	"mcp-server-magictools/internal/vector"
)

// LLMResponse is undocumented but satisfies standard structural requirements.
type LLMResponse struct {
	SyntheticIntents []string `json:"synthetic_intents"`
	LexicalTokens    []string `json:"lexical_tokens"`
	NegativeTriggers []string `json:"negative_triggers"`
}

// RunSweep executes the synchronous LLM hydration and vector embedding process.
// Bounded natively by maxPerSweep. Returns true if there are more tools left pending.
// It is intended to be invoked iteratively by the daemon continuation loop.
func RunSweep(ctx context.Context, store *db.Store, cfg *config.Config, providers map[string]llm.Provider) bool {
	if cfg.Intelligence.Provider == "" || cfg.Intelligence.APIKey == "" {
		slog.Warn("RunSweep aborted: LLM provider or API key missing", "component", "hydrator")
		return false
	}

	// 🛡️ LLM AVAILABILITY PROBE: Test connectivity before processing
	if err := probeLLMAvailability(ctx, cfg); err != nil {
		slog.Warn("llm provider is NOT REACHABLE. hydration sweep aborted.",
			"component", "hydrator",
			"provider", cfg.Intelligence.Provider,
			"error", err)
		return false
	}

	var targets []*db.ToolRecord
	var hnswBackfill []*db.ToolRecord

	// 🛡️ OPTIMISTIC SKIP: If no tools have been marked pending since last sweep, skip the full scan.
	// EXCEPTION: When the vector engine is online, always allow the scan if the HNSW graph
	// is missing tools. This catches the case where tools were hydrated in BadgerDB (intel
	// status = "hydrated") but never embedded in the HNSW graph (e.g., after expanding
	// universal hydration to all sub-servers).
	e := vector.GetEngine()
	vectorMissing := e != nil && e.VectorEnabled() && e.RequiresHydration()

	// 🛡️ HNSW DELTA DETECTION: Even if the graph isn't empty, it may be incomplete.
	// Compare HNSW graph size against the Bleve index tool count to detect missing tools.
	hnswIncomplete := false
	if !vectorMissing && e != nil && e.VectorEnabled() {
		if expectedCount, err := store.Index.DocCount(); err == nil && uint64(e.Len()) < expectedCount {
			hnswIncomplete = true
		}
	}

	if store.PendingHydrations.Load() == 0 && !vectorMissing && !hnswIncomplete {
		return false
	}

	// Scan dataset for missing or "pending" extraction
	if err := store.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			_ = item.Value(func(val []byte) error {
				var r db.ToolRecord
				if err := json.Unmarshal(val, &r); err == nil {
					// 🛡️ DYNAMIC OVERLAY: Extract intel natively without triggering a nested `store.DB.View` RWMutex deadlock.
					var intelPtr *db.ToolIntelligence
					var iErr error
					if intelItem, err := txn.Get([]byte("intel:" + r.URN)); err == nil {
						_ = intelItem.Value(func(v []byte) error {
							var i db.ToolIntelligence
							if unmarshErr := json.Unmarshal(v, &i); unmarshErr == nil {
								intelPtr = &i
							}
							return nil
						})
					} else {
						iErr = err // Track missing intel
					}

					if iErr != nil || intelPtr == nil || intelPtr.AnalysisStatus == "pending" || intelPtr.AnalysisStatus == "" || r.SchemaHash != intelPtr.SchemaHash {
						targets = append(targets, &r)
					} else {
						// 🛡️ HNSW BACKFILL: Even if fully hydrated natively in Badger, verify presence in HNSW graph.
						e := vector.GetEngine()
						if e != nil && e.VectorEnabled() && !e.HasDocument(r.URN) {
							hnswBackfill = append(hnswBackfill, &r)
						}
					}
				}
				return nil
			})
		}
		return nil
	}); err != nil {
		slog.Warn("database scan failed", "component", "hydrator", "error", err)
		return false
	}

	// Reset the pending counter after scanning — the sweep will handle whatever was found.
	// NOTE: Reset happens AFTER batch cap to preserve the remaining count for subsequent ticks.

	// 🛡️ HNSW MISSING HOOK: Instantly hydrate tools completely missing from the vector index
	if len(hnswBackfill) > 0 {
		slog.Info("hydrator: HNSW graph missing known tools, triggering native graph backfill", "count", len(hnswBackfill))
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("post-sweep HNSW backfill panicked (recovered)", "component", "hydrator", "panic", r)
				}
			}()
			hydrateVectorGraph(ctx, hnswBackfill)
		}()
	}

	if len(targets) == 0 {
		store.PendingHydrations.Store(0)
		return false
	}

	// 🛡️ BATCH CAP: Limit tools processed per sweep to ensure background fairness.
	const maxPerSweep = 5
	hasMore := len(targets) > maxPerSweep
	if hasMore {
		remaining := int64(len(targets) - maxPerSweep)
		slog.Info("sweep batch capped to prevent boot stall", "component", "hydrator",
			"total_pending", len(targets), "processing", maxPerSweep, "deferred", remaining)
		targets = targets[:maxPerSweep]
		// Preserve remaining count so subsequent ticks continue processing
		store.PendingHydrations.Store(remaining)
	} else {
		store.PendingHydrations.Store(0)
	}

	slog.Info("hydration sweep starting", "component", "hydrator",
		"tools", len(targets), "estimated_seconds", len(targets)*2)

	var succeeded, failed atomic.Int64
	sweepStart := time.Now()

	// 🛡️ BOUNDED CONCURRENCY: Process tools with errgroup (max 3 concurrent)
	const maxConcurrency = 3
	sem := make(chan struct{}, maxConcurrency)
	eg, egCtx := errgroup.WithContext(ctx)

	for i, tool := range targets {
		// Rate pacing: 1-second delay between launches to prevent API 429 throttling
		if i > 0 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(1 * time.Second):
			}
		}

		tool := tool // capture loop variable
		eg.Go(func() error {
			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-egCtx.Done():
				return nil
			}

			util.TraceFunc(egCtx, "event", "hydrator_start", "tool", tool.URN)

			// 🛡️ STATIC PROXY OVERRIDE: magictools tools bypass the semantic engine
			if tool.IsNative {
				slog.Info("applying static maximum trust score to native proxy tool", "component", "hydrator", "tool", tool.URN)

				var intents []string
				var tokens []string
				switch tool.Name {
				case "align_tools":
					intents = []string{"search tools", "find tool", "discover", "lookup URN"}
					tokens = []string{"discovery", "search", "directory", "URN"}
				case "call_proxy":
					intents = []string{"execute tool", "run", "invoke", "proxy call"}
					tokens = []string{"execute", "dispatch", "proxy", "run"}
				case "execute_pipeline":
					intents = []string{"DAG", "pipeline", "execution plan", "analysis graph"}
					tokens = []string{"DAG", "pipeline", "brainstorm", "go-refactor", "sequence"}
				case "sync_ecosystem":
					intents = []string{"synchronize", "refresh all", "update local cache", "resync"}
					tokens = []string{"sync", "ecosystem", "refresh", "index"}
				case "list_tools":
					intents = []string{"inventory", "available tools", "show tools", "enumerate"}
					tokens = []string{"list", "inventory", "sub-servers", "tools"}
				case "get_health_report":
					intents = []string{"health", "status", "alive", "ping", "availability"}
					tokens = []string{"health", "ping", "status", "state"}
				default:
					intents = []string{"manage orchestrator", "system tools", "execute", tool.Name}
					tokens = []string{"orchestrator", "admin", "magictools", tool.Name}
				}

				intel := &db.ToolIntelligence{
					SyntheticIntents: intents,
					LexicalTokens:    tokens,
					AnalysisStatus:   "hydrated",
					SchemaHash:       tool.SchemaHash,
				}
				intel.Metrics.ProxyReliability = 5.0 // Guarantee native tools mathematically outrank all sub-servers

				if err := store.SaveIntelligence(tool.URN, intel); err != nil {
					slog.Warn("failed to save static proxy state", "component", "hydrator", "error", err)
					updateToolStatus(store, tool, "failed")
					failed.Add(1)
				} else {
					slog.Debug("static proxy state committed", "component", "hydrator", "tool", tool.URN)
					succeeded.Add(1)
				}
				return nil
			}

			slog.Info("fetching semantic augmentation", "component", "hydrator", "tool", tool.URN)

			startTime := time.Now()
			toolCtx, toolCancel := context.WithTimeout(egCtx, time.Duration(cfg.Intelligence.TimeoutSeconds)*time.Second)
			result, err := applySemanticAugmentation(toolCtx, tool, cfg, providers)
			toolCancel()
			elapsed := time.Since(startTime)

			if err != nil {
				slog.Warn("remote augmentation failed", "component", "hydrator", "tool", tool.URN, "duration", elapsed.String(), "error", err)
				updateToolStatus(store, tool, "failed")
				failed.Add(1)
				return nil // don't abort the group on individual tool failure
			}

			slog.Debug("parsing completed successfully", "component", "hydrator", "tool", tool.URN, "duration", elapsed.String())

			// Inject analytical properties into isolated struct.
			// ProxyReliability is NOT set here — it is owned exclusively by
			// the deterministic computeWBase() baseline + dynamic delta from
			// UpdateToolMetrics. The LLM is used for semantic enrichment only.
			existingIntel, _ := store.GetIntelligence(tool.URN)
			var preservedReliability float64
			if existingIntel != nil && existingIntel.Metrics.ProxyReliability > 0 {
				preservedReliability = existingIntel.Metrics.ProxyReliability
			} else {
				preservedReliability = 1.0 // safe default for first-time hydration
			}

			intel := &db.ToolIntelligence{
				SyntheticIntents: result.SyntheticIntents,
				LexicalTokens:    result.LexicalTokens,
				NegativeTriggers: result.NegativeTriggers,
				AnalysisStatus:   "hydrated",
				SchemaHash:       tool.SchemaHash,
			}
			intel.Metrics.ProxyReliability = preservedReliability

			if err := store.SaveIntelligence(tool.URN, intel); err != nil {
				slog.Warn("failed to save updated tool state to badger", "component", "hydrator", "error", err)
				failed.Add(1)
			} else {
				slog.Debug("status transition committed", "component", "hydrator", "tool", tool.URN, "new_status", "hydrated")
				succeeded.Add(1)
			}
			return nil
		})
	}

	_ = eg.Wait() // individual errors are handled inside goroutines

	// 🧠 POST-SWEEP NORMALIZATION: Z-score normalize ProxyReliability values
	// to ensure relative differentiation even when raw scores cluster together.
	// Native tools are excluded — they retain their static 1.5 override.
	normalizeProxyScores(store)

	// 🧠 POST-SWEEP HNSW HYDRATION: Populate HNSW vector graph with successfully
	// hydrated tool descriptions. This runs AFTER all LLM API calls complete,
	// preventing the thundering herd that previously stalled boot/reload.
	// Wrapped in recover() so HNSW library panics (e.g. "node not added"
	// on duplicate keys) never kill the hydrator daemon goroutine.
	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("post-sweep HNSW hydration panicked (recovered)", "component", "hydrator", "panic", r)
			}
		}()
		hydrateVectorGraph(ctx, targets)
	}()

	slog.Info("sweep complete",
		"component", "hydrator",
		"total", len(targets),
		"succeeded", succeeded.Load(),
		"failed", failed.Load(),
		"duration", time.Since(sweepStart).String())

	return hasMore
}

func updateToolStatus(store *db.Store, tool *db.ToolRecord, status string) {
	intel, _ := store.GetIntelligence(tool.URN)
	if intel == nil {
		intel = &db.ToolIntelligence{}
	}
	intel.AnalysisStatus = status
	intel.SchemaHash = tool.SchemaHash
	if err := store.SaveIntelligence(tool.URN, intel); err != nil {
		slog.Warn("failed to save updated tool state to badger", "component", "hydrator", "error", err)
	} else {
		slog.Info("status transition committed", "component", "hydrator", "tool", tool.URN, "new_status", status)
	}
}

// hydrateVectorGraph populates the HNSW vector index with tool descriptions
// from all sub-servers. This runs sequentially after the LLM sweep completes
// to prevent thundering herd API calls.
func hydrateVectorGraph(ctx context.Context, tools []*db.ToolRecord) {
	e := vector.GetEngine()
	if e == nil || !e.VectorEnabled() {
		return
	}

	vecCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	count := 0
	for _, tool := range tools {
		if tool == nil {
			continue
		}

		// 🛡️ PANIC GUARD: The HNSW library panics with "node not added"
		// on duplicate key insertion. Catch per-tool to prevent one bad
		// node from aborting the entire vector hydration batch.
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("HNSW graph.Add panicked for tool", "component", "hydrator", "urn", tool.URN, "panic", r)
				}
			}()
			if err := e.AddDocument(vecCtx, tool.URN, tool.Description); err != nil {
				slog.Warn("post-sweep HNSW hydration failed", "component", "hydrator", "urn", tool.URN, "error", err)
			} else {
				count++
			}
		}()
	}
	if count > 0 {
		slog.Info("post-sweep HNSW vectors committed", "component", "hydrator", "tools", count)
	}
}

// normalizeProxyScores applies z-score normalization across all non-native tool
// ProxyReliability values after a hydration sweep. This ensures relative
// differentiation is preserved even when raw scores cluster together.
//
// Native tools (IsNative) are excluded from normalization to preserve their
// static 1.5 override. Requires ≥3 non-native tools with non-zero stdev.
//
// Formula: normalized = 0.8 + ((raw - mean) / stdev) × 0.25
// Clamped to [0.5, 1.3].
func normalizeProxyScores(store *db.Store) {
	type entry struct {
		urn         string
		reliability float64
	}
	var entries []entry

	// Scan dataset for all non-native tools
	_ = store.DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("tool:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			_ = item.Value(func(val []byte) error {
				var t db.ToolRecord
				if err := json.Unmarshal(val, &t); err == nil {
					if t.IsNative {
						return nil
					}
					// 🛡️ DYNAMIC OVERLAY: Extract natively inline avoiding nested RWMutex deadlocks.
					if intelItem, extractErr := txn.Get([]byte("intel:" + t.URN)); extractErr == nil {
						_ = intelItem.Value(func(iv []byte) error {
							var intel db.ToolIntelligence
							if unmarshalErr := json.Unmarshal(iv, &intel); unmarshalErr == nil {
								if intel.Metrics.ProxyReliability > 0 {
									entries = append(entries, entry{urn: t.URN, reliability: intel.Metrics.ProxyReliability})
								}
							}
							return nil
						})
					}
				}
				return nil
			})
		}
		return nil
	})

	if len(entries) < 3 {
		slog.Debug("normalization skipped: insufficient non-native tools", "component", "hydrator", "count", len(entries))
		return
	}

	// Compute mean
	var sum float64
	for _, e := range entries {
		sum += e.reliability
	}
	mean := sum / float64(len(entries))

	// Compute standard deviation
	var variance float64
	for _, e := range entries {
		d := e.reliability - mean
		variance += d * d
	}
	stdev := math.Sqrt(variance / float64(len(entries)))

	if stdev < 0.001 {
		slog.Warn("normalization skipped: near-zero standard deviation",
			"component", "hydrator",
			"mean", mean, "stdev", stdev, "count", len(entries))
		return
	}

	slog.Info("normalizing proxyreliability scores",
		"component", "hydrator",
		"tools", len(entries), "mean", fmt.Sprintf("%.4f", mean), "stdev", fmt.Sprintf("%.4f", stdev))

	var normalized int
	for _, e := range entries {
		z := (e.reliability - mean) / stdev
		newScore := 0.8 + (z * 0.25)

		// Clamp to [0.5, 1.3]
		if newScore < 0.5 {
			newScore = 0.5
		}
		if newScore > 1.3 {
			newScore = 1.3
		}

		intel, err := store.GetIntelligence(e.urn)
		if err != nil || intel == nil {
			continue
		}
		oldScore := intel.Metrics.ProxyReliability
		intel.Metrics.ProxyReliability = newScore
		if err := store.SaveIntelligence(e.urn, intel); err != nil {
			slog.Warn("failed to save normalized score", "component", "hydrator", "urn", e.urn, "error", err)
		} else {
			slog.Debug("score normalized", "component", "hydrator", "urn", e.urn,
				"old", fmt.Sprintf("%.4f", oldScore), "new", fmt.Sprintf("%.4f", newScore))
			normalized++
		}
	}

	slog.Info("normalization complete", "component", "hydrator", "normalized", normalized)
}

// probeLLMAvailability performs a lightweight connectivity check against the
// configured LLM provider. Returns nil if reachable, error otherwise.
func probeLLMAvailability(ctx context.Context, cfg *config.Config) error {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var p llm.Provider
	var err error
	switch cfg.Intelligence.Provider {
	case "gemini":
		p, err = llm.NewGemini(probeCtx, cfg.Intelligence.APIKey, cfg.Intelligence.Model)
	case "openai":
		p = llm.NewOpenAI(cfg.Intelligence.APIKey, cfg.Intelligence.Model)
	case "anthropic":
		p, err = llm.NewAnthropic(cfg.Intelligence.APIKey, cfg.Intelligence.Model)
	default:
		return fmt.Errorf("unknown provider: %s", cfg.Intelligence.Provider)
	}
	if err != nil {
		return fmt.Errorf("provider initialization failed: %w", err)
	}

	// Lightweight probe: request a single-token response
	_, err = p.Generate(probeCtx, "Respond with exactly: OK")
	if err != nil {
		return fmt.Errorf("LLM probe failed: %w", err)
	}
	return nil
}

// initProviders pre-initializes LLM providers for the model cascade.
// Providers are created once per sweep and reused across all tools.
func initProviders(ctx context.Context, cfg *config.Config) map[string]llm.Provider {
	models := append([]string{cfg.Intelligence.Model}, cfg.Intelligence.FallbackModels...)
	providers := make(map[string]llm.Provider, len(models))

	for _, model := range models {
		var p llm.Provider
		var err error
		switch cfg.Intelligence.Provider {
		case "gemini":
			p, err = llm.NewGemini(ctx, cfg.Intelligence.APIKey, model)
		case "openai":
			p = llm.NewOpenAI(cfg.Intelligence.APIKey, model)
		case "anthropic":
			p, err = llm.NewAnthropic(cfg.Intelligence.APIKey, model)
		default:
			slog.Warn("unknown provider", "component", "hydrator", "provider", cfg.Intelligence.Provider)
			continue
		}
		if err != nil {
			slog.Warn("model init failed", "component", "hydrator", "model", model, "error", err)
			continue
		}
		providers[model] = p
	}
	return providers
}

// LLMToolPayload is a trimmed struct sent to the LLM to reduce API token waste.
// Excludes internal fields like usage_count, last_synced_at, schema_hash.
type LLMToolPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Server      string `json:"server"`
}

func applySemanticAugmentation(ctx context.Context, tool *db.ToolRecord, cfg *config.Config, providers map[string]llm.Provider) (*LLMResponse, error) {
	prompt := `You are the Semantic Intelligence Engine for the MagicTools Orchestrator.
Task: Analyze the attached raw JSON tool schema and "hydrate" it into a searchable, weighted intent matrix.
Return ONLY valid JSON matching this schema:
{
  "synthetic_intents": ["phrase a user would say", ... 12 max],
  "lexical_tokens": ["technical_keyword", ... 8 max],
  "negative_triggers": ["phrase that sounds identical but is unrelated", ... 5 max]
}

Rules:
- synthetic_intents: natural language phrases a user would type to trigger this tool. Be specific and diverse.
- lexical_tokens: precise technical keywords unique to this tool's domain. Avoid generic terms.
- negative_triggers: phrases that SOUND related but should NOT trigger this tool. Focus on cross-category confusion.

Do not use markdown blocks, just return the raw JSON object string.`

	// 🛡️ TRIMMED PAYLOAD: Only send semantically relevant fields to the LLM
	payload := LLMToolPayload{
		Name:        tool.Name,
		Description: tool.Description,
		Category:    tool.Category,
		Server:      tool.Server,
	}
	schemaBytes, _ := json.Marshal(payload)
	fullPrompt := prompt + "\n\nSCHEMA:\n" + string(schemaBytes)

	// 🔄 MODEL CASCADE: Try primary model first, then fallbacks
	modelsToTry := append([]string{cfg.Intelligence.Model}, cfg.Intelligence.FallbackModels...)

	retryCount := cfg.Intelligence.RetryCount
	if retryCount <= 0 {
		retryCount = 2
	}
	retryDelay := time.Duration(cfg.Intelligence.RetryDelay) * time.Second
	if retryDelay <= 0 {
		retryDelay = 5 * time.Second
	}

	var lastErr error
	for _, model := range modelsToTry {
		p, ok := providers[model]
		if !ok {
			lastErr = fmt.Errorf("provider for model %s not initialized", model)
			continue
		}

		rawText, err := llm.GenerateWithRetry(ctx, p, fullPrompt, retryCount, retryDelay)
		if err != nil {
			slog.Warn("model generation failed, trying next", "component", "hydrator", "model", model, "error", err)
			lastErr = err
			continue
		}

		// Robust strip if the gateway returned markdown bounding box
		rawText = strings.TrimSpace(rawText)
		lower := strings.ToLower(rawText)
		if strings.HasPrefix(lower, "```json") {
			rawText = rawText[7:]
		} else if strings.HasPrefix(lower, "```") {
			rawText = rawText[3:]
		}
		rawText = strings.TrimSuffix(strings.TrimSpace(rawText), "```")
		rawText = strings.TrimSpace(rawText)

		var result LLMResponse
		if err := json.Unmarshal([]byte(rawText), &result); err != nil {
			slog.Warn("json parse failed, trying next model", "component", "hydrator", "model", model, "error", err, "raw", rawText)
			lastErr = err
			continue
		}

		slog.Debug("augmentation succeeded", "component", "hydrator", "model", model)
		return &result, nil
	}

	return nil, fmt.Errorf("all models failed for %s, last error: %w", cfg.Intelligence.Provider, lastErr)
}

// RecallMiner abstracts the recall client APIs needed by the intelligence
// package. This prevents a direct dependency on the external package.
type RecallMiner interface {
	RecallEnabled() bool
	AggregateSessionFromRecall(ctx context.Context, serverID, projectID string) (map[string]any, error)
	ListSessionsByFilter(ctx context.Context, projectID, serverID, outcome string, limit int) string
}

// MineRecallPatterns queries recall for historical session data and feeds
// empirically validated tool-to-intent mappings into the Ghost Index.
// This creates a feedback loop: successful pipeline executions stored in
// recall feed back into compose_pipeline's scoring, making future DAG
// compositions grounded in real usage patterns.
func MineRecallPatterns(ctx context.Context, rc RecallMiner, store *db.Store) {
	if rc == nil || !rc.RecallEnabled() || store == nil {
		return
	}

	servers := []string{"brainstorm", "go-refactor"}
	var totalIndexed int

	for _, serverID := range servers {
		sessionData, err := rc.AggregateSessionFromRecall(ctx, serverID, "global")
		if err != nil {
			slog.Log(ctx, util.LevelTrace, "recall_miner: no sessions for server",
				"server", serverID, "error", err)
			continue
		}

		entries, ok := sessionData["entries"].([]any)
		if !ok {
			// Try nested under stages
			if stages, ok := sessionData["stages"].([]any); ok {
				entries = stages
			}
		}
		if len(entries) == 0 {
			continue
		}

		// Extract tool URN sequences and intents from session entries.
		var dagURNs []string
		var intent string
		for _, entryRaw := range entries {
			entry, ok := entryRaw.(map[string]any)
			if !ok {
				continue
			}

			// Extract stage/tool name from tags or content
			var stageName string

			// Strategy 1: Content JSON has "stage" key
			if content, ok := entry["content"].(string); ok && content != "" {
				var contentObj map[string]any
				if json.Unmarshal([]byte(content), &contentObj) == nil {
					if s, ok := contentObj["stage"].(string); ok {
						stageName = s
					}
					if stageName == "execute_pipeline" {
						if i, ok := contentObj["intent"].(string); ok {
							intent = i
						}
					}
				}
			}

			// Strategy 2: Tags carry "trace:<tool_name>"
			if stageName == "" {
				if tags, ok := entry["tags"].([]any); ok {
					for _, tag := range tags {
						tagStr, _ := tag.(string)
						if after, ok0 := strings.CutPrefix(tagStr, "trace:"); ok0 {
							candidate := after
							if candidate != "auto_publish" && candidate != "async_push" {
								stageName = candidate
							}
						}
					}
				}
			}

			// Check for error outcomes — skip failed sessions
			if tags, ok := entry["tags"].([]any); ok {
				for _, tag := range tags {
					tagStr, _ := tag.(string)
					if tagStr == "outcome:error" || tagStr == "outcome:failed" {
						dagURNs = nil // Discard this session's data
						break
					}
				}
			}
			if dagURNs == nil {
				break // Session had a failure, skip entirely
			}

			if stageName != "" && stageName != "execute_pipeline" && stageName != "generate_audit_report" {
				dagURNs = append(dagURNs, stageName)
			}
		}

		// Only index if we have a valid DAG with at least 2 tools and an intent
		if len(dagURNs) >= 2 && intent != "" {
			if err := store.Index.IndexSyntheticIntent(intent, dagURNs); err != nil {
				slog.Warn("recall_miner: failed to index empirical intent",
					"intent", intent, "error", err)
			} else {
				totalIndexed++
			}
		}
	}

	if totalIndexed > 0 {
		slog.Info("recall_miner: empirical patterns indexed into Ghost Index",
			"count", totalIndexed)
	}
}

// CalibrateFromRecall mines recall session outcomes to empirically adjust
// ProxyReliability scores. Tools with high real-world success rates get
// boosted, while tools that consistently error get penalized.
// Final score = (z_normalized * 0.6) + (empirical_rate * 0.4)
func CalibrateFromRecall(ctx context.Context, rc RecallMiner, store *db.Store) {
	if rc == nil || !rc.RecallEnabled() || store == nil {
		return
	}

	// Query recall for recent session outcomes from both sub-servers
	type toolStats struct {
		success int
		total   int
	}
	stats := make(map[string]*toolStats)
	servers := []string{"brainstorm", "go-refactor"}

	for _, serverID := range servers {
		raw := rc.ListSessionsByFilter(ctx, "", serverID, "", 30)
		if raw == "" {
			continue
		}

		var envelope map[string]any
		if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
			continue
		}

		var entries []any
		if e, ok := envelope["entries"].([]any); ok {
			entries = e
		} else if data, ok := envelope["data"].(map[string]any); ok {
			entries, _ = data["entries"].([]any)
		}

		for _, entryRaw := range entries {
			entry, ok := entryRaw.(map[string]any)
			if !ok {
				continue
			}
			record, _ := entry["record"].(map[string]any)
			if record == nil {
				continue
			}

			// Extract tool URN from tags
			tags, _ := record["tags"].([]any)
			var toolURN string
			isSuccess := true
			for _, tag := range tags {
				tagStr, _ := tag.(string)
				if after, ok0 := strings.CutPrefix(tagStr, "trace:"); ok0 {
					candidate := after
					if candidate != "auto_publish" && candidate != "async_push" {
						toolURN = serverID + ":" + candidate
					}
				}
				if tagStr == "outcome:error" || tagStr == "outcome:failed" {
					isSuccess = false
				}
			}

			if toolURN == "" {
				continue
			}
			if _, ok := stats[toolURN]; !ok {
				stats[toolURN] = &toolStats{}
			}
			stats[toolURN].total++
			if isSuccess {
				stats[toolURN].success++
			}
		}
	}

	if len(stats) == 0 {
		return
	}

	// Apply empirical calibration
	var calibrated int
	for urn, s := range stats {
		if s.total < 3 {
			continue // Not enough data for meaningful calibration
		}
		empiricalRate := float64(s.success) / float64(s.total)

		intel, err := store.GetIntelligence(urn)
		if err != nil || intel == nil {
			continue
		}

		// Blend: final = (z_normalized * 0.6) + (empirical * 0.4)
		blended := (intel.Metrics.ProxyReliability * 0.6) + (empiricalRate * 0.4)

		// Clamp to [0.5, 1.3] to prevent extreme swings
		blended = math.Max(0.5, math.Min(1.3, blended))

		if blended != intel.Metrics.ProxyReliability {
			intel.Metrics.ProxyReliability = blended
			store.SaveIntelligence(urn, intel)
			calibrated++
		}
	}

	if calibrated > 0 {
		slog.Info("recall_calibration: ProxyReliability adjusted from empirical data",
			"tools_calibrated", calibrated, "total_tracked", len(stats))
	}
}
