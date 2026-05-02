package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/logging"
	"mcp-server-magictools/internal/util"
	"mcp-server-magictools/internal/vector"
)

// SyncResult is undocumented but satisfies standard structural requirements.
type SyncResult struct {
	TotalPotential int64
	Connected      []string
	Failed         []string
}

// SyncNativeTools routes internal orchestrator tools (magictools) through the standard
// DB parse-and-save pipeline to acquire semantic hydrator metadata.
func (m *WarmRegistry) SyncNativeTools(ctx context.Context, tools *mcp.ListToolsResult) (int, error) {
	// Treat magictools as a synthetic sub-server for DB routing purposes
	sc := config.ServerConfig{
		Name: "magictools",
		// Provide a non-nil array to avoid panics when accessing array properties later
		DisabledTools: []string{},
	}

	// Create a synthetic SubServer wrapper to pass nil checks in parseAndSaveTools
	srv := &SubServer{
		Status: StatusReady,
	}

	indexed, err := m.parseAndSaveTools(sc, srv, tools)
	if err != nil {
		return 0, fmt.Errorf("failed to sync native tools: %w", err)
	}
	return indexed, nil
}

// SyncEcosystem is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) SyncEcosystem(ctx context.Context) (*SyncResult, error) {
	if m.Config.ConfigPath != "" {
		freshCfg, err := m.Config.Reload()
		if err == nil {
			m.Config.UpdateManagedServers(freshCfg.ManagedServers)
		}
	}

	managed := m.Config.GetManagedServers()
	result := &SyncResult{}
	var mu sync.Mutex

	// 🛡️ BACKGROUND ORPHAN SWEEP: Remove ghost servers/tools from BadgerDB that are no longer managed
	var activeNames []string
	for _, sc := range managed {
		activeNames = append(activeNames, sc.Name)
	}
	// 🛡️ EXEMPT NATIVE ORCHESTRATOR: 'magictools' itself is not listed in GetManagedServers()
	activeNames = append(activeNames, "magictools")

	go func(names []string) {
		if err := m.Store.PurgeOrphanedServers(names); err != nil {
			slog.Warn("sync: background sweep of orphaned servers failed", "error", err)
		}
	}(activeNames)

	// Seed the trigger DB with default keyword→server mappings for data-driven steering.
	m.Store.PopulateDefaultTriggers()

	// All servers sync concurrently — no ordering needed.

	maxConcurrency := min(runtime.NumCPU()*2, 10)
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, sc := range managed {
		wg.Add(1)
		go func(c context.Context, serverConfig config.ServerConfig) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-c.Done():
				return
			}

			timeoutCtx, cancel := context.WithTimeout(c, 60*time.Second)
			defer cancel()

			_, err := m.indexServer(timeoutCtx, serverConfig)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				m.logSyncError(serverConfig.Name, err)
				result.Failed = append(result.Failed, serverConfig.Name)
				return
			}

			result.Connected = append(result.Connected, serverConfig.Name)
		}(ctx, sc)
	}

	done := make(chan struct{})
	go func(c context.Context) {
		wg.Wait()
		close(done)
	}(ctx)

	select {
	case <-done:
		m.IsSynced.Store(true)
		result.TotalPotential = int64(len(result.Connected))
		return result, nil
	case <-ctx.Done():
		return result, ctx.Err()
	}
}

// SyncServer is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) SyncServer(ctx context.Context, name string) error {
	var sc *config.ServerConfig
	for _, c := range m.Config.GetManagedServers() {
		if c.Name == name {
			sc = &c
			break
		}
	}

	if sc == nil {
		return fmt.Errorf("server %s not found in managed config", name)
	}

	// Timeout ownership belongs to the caller (executeBootSequence: 60s).
	// No nested timeout here — stacking identical 60s contexts adds no
	// protection and misleads developers during timeout debugging.
	_, err := m.indexServer(ctx, *sc)
	return err
}

func (m *WarmRegistry) indexServer(ctx context.Context, sc config.ServerConfig) (int, error) {
	// Always sync to warm up the server on startup or manual sync.
	// We no longer skip if tools are in the store to ensure handshakes complete.

	srv, ok := m.GetServer(sc.Name)
	if !ok || srv.Session == nil {
		slog.Log(ctx, util.LevelTrace, "sync: initiating server connection", "server", sc.Name)
		timeoutDur := 10 * time.Second
		if sc.Name == "recall" {
			timeoutDur = 15 * time.Second // 🛡️ Recall on localhost should connect within 5s; 15s is generous
		}
		connectCtx, cancelConnect := context.WithTimeout(ctx, timeoutDur)
		err := m.Connect(connectCtx, sc.Name, sc.Command, sc.Args, sc.Env, sc.Hash())
		cancelConnect()
		if err != nil {
			return 0, fmt.Errorf("server %s connection failed: %w", sc.Name, err)
		}
		srv, _ = m.GetServer(sc.Name)
		slog.Log(ctx, util.LevelTrace, "sync: server connected", "server", sc.Name)
	} else {
		// Just ensure it's healthy if already connected
		s, sOk := m.GetServer(sc.Name)
		if sOk && s != nil {
			m.mu.Lock()
			s.DesiredState = StatusHealthy
			mailbox := s.Mailbox
			m.mu.Unlock()
			select {
			case mailbox <- cmdConnect:
			default:
			}
		}
	}

	// 🛡️ NIL GUARD + RACE FIX: Snapshot session under RLock to protect against
	// race where process crashes between Connect and ListTools.
	m.mu.RLock()
	if srv == nil || srv.Session == nil {
		m.mu.RUnlock()
		return 0, fmt.Errorf("server %s: session lost before tool sync (process may have crashed)", sc.Name)
	}
	session := srv.Session // snapshot under lock
	m.mu.RUnlock()

	m.mu.Lock()
	srv.Status = StatusReady
	m.mu.Unlock()

	slog.Log(ctx, util.LevelTrace, "sync: server marked ready, executing ListTools synchronously", "server", sc.Name)

	tools, err := session.ListTools(ctx, nil)
	if err != nil || tools == nil || len(tools.Tools) == 0 {
		errMsg := "empty result"
		if err != nil {
			errMsg = err.Error()
		}

		var raw []byte
		if srv.Filter != nil {
			raw = srv.Filter.GetLastFrame()
		}
		slog.Error("Sync Failed / tools/list error",
			"component", "lifecycle",
			"server_id", sc.Name,
			"error", errMsg,
			"raw_payload", string(raw))

		m.Logger.Log(logging.WARNING, sc.Name, fmt.Sprintf("Sync Failed: %v", errMsg))
		return 0, fmt.Errorf("tools/list failed for %s: %s", sc.Name, errMsg)
	}

	indexed, parseErr := m.parseAndSaveTools(sc, srv, tools)
	if parseErr != nil {
		slog.Error("Sync Failed / parse error", "server_id", sc.Name, "error", parseErr)
		return 0, parseErr
	}
	return indexed, nil
}

func (m *WarmRegistry) parseAndSaveTools(sc config.ServerConfig, srv *SubServer, tools *mcp.ListToolsResult) (int, error) {
	// 🛡️ SYNC VERIFICATION: Zero-Null Sanitization Check & Legacy Recovery
	var rawList map[string]any
	if srv.Filter != nil {
		if raw := srv.Filter.GetLastFrame(); len(raw) > 0 {
			if err := json.Unmarshal(raw, &rawList); err != nil {
				slog.Warn("sync: failed to unmarshal raw tool list", "component", "proxy", "server_id", sc.Name, "error", err)
			}
		}
	}

	for i, t := range tools.Tools {
		if t.InputSchema == nil && rawList != nil {
			if result, ok := rawList["result"].(map[string]any); ok {
				if rTools, ok := result["tools"].([]any); ok && i < len(rTools) {
					if rTool, ok := rTools[i].(map[string]any); ok {
						if params, ok := rTool["parameters"]; ok && params != nil {
							slog.Info("sync: recovered legacy 'parameters' field", "component", "proxy", "server_id", sc.Name, "tool", t.Name)
							t.InputSchema = params
						}
					}
				}
			}
		}

		if err := util.ValidateZeroNull(t); err != nil {
			var raw []byte
			if srv.Filter != nil {
				raw = srv.Filter.GetLastFrame()
			}
			slog.Warn("Sync Warning: Schema issues detected but will attempt repair",
				"server", sc.Name, "tool", t.Name, "error", err, "raw_json", string(raw))
			m.Logger.Log(logging.WARNING, sc.Name, fmt.Sprintf("Schema Warning: %v (Attempting Repair)", err))
		}
	}

	m.Logger.Log(logging.SYNC, sc.Name, fmt.Sprintf("%d Tools Indexed", len(tools.Tools)))

	disabled := make(map[string]bool, len(sc.DisabledTools))
	for _, t := range sc.DisabledTools {
		disabled[t] = true
	}

	// 🛡️ INTELLIGENCE GC: Prune missing / orphaned LLM states asynchronously.
	// This is maintenance work — safe to defer without blocking boot-critical sync.
	var validURNs []string
	for _, t := range tools.Tools {
		if !disabled[t.Name] {
			validURNs = append(validURNs, fmt.Sprintf("%s:%s", sc.Name, util.SanitizeToolSchema(t).Name))
		}
	}
	go func(server string, urns []string) {
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("sync: intelligence GC panicked", "server", server, "panic", r)
			}
		}()
		if err := m.Store.PruneOrphanedIntelligence(server, urns); err != nil {
			slog.Warn("sync: background sweep of orphaned intelligence failed", "server", server, "error", err)
		}
	}(sc.Name, validURNs)

	indexed := 0
	var batchRecords []*db.ToolRecord
	batchSchemas := make(map[string]map[string]any)

	for _, t := range tools.Tools {
		if disabled[t.Name] {
			continue
		}

		st := util.SanitizeToolSchema(t)
		h := m.hashSchema(st)
		record := &db.ToolRecord{
			URN:          fmt.Sprintf("%s:%s", sc.Name, st.Name),
			Name:         st.Name,
			Server:       sc.Name,
			Description:  st.Description,
			LiteSummary:  m.minifyDescription(st.Description),
			Intent:       m.extractIntent(st.Name, st.Description),
			SchemaHash:   h,
			LastSyncedAt: time.Now().Unix(),
			Category:     m.deriveCategory(sc.Name),
			IsNative:     sc.Name == "magictools",
		}

		// 🛡️ PIPELINE TAXONOMY: Hydrate Role and Phase for compose_pipeline DAG intelligence
		hydrateRoleAndPhase(record)

		batchRecords = append(batchRecords, record)
		batchSchemas[h] = m.toSchemaMap(st.InputSchema)
		indexed++
	}

	// 🛡️ PERF: Hash-gated sync — skip the expensive purge+save+hydrate cycle
	// if the tool list hasn't changed since the last successful sync.
	compositeHasher := sha256.New()
	for _, rec := range batchRecords {
		compositeHasher.Write([]byte(rec.SchemaHash))
	}
	compositeHash := fmt.Sprintf("%x", compositeHasher.Sum(nil))

	if storedHash := m.Store.GetServerSyncHash(sc.Name); storedHash == compositeHash && len(batchRecords) > 0 {
		slog.Info("sync: tool list unchanged, skipping purge+save", "server", sc.Name, "tools", len(batchRecords))
		return indexed, nil
	}

	if len(batchRecords) > 0 {
		// 🛡️ SAVE-FIRST: Upsert new tools immediately so they're available for
		// align_tools queries. BatchSaveTools uses Badger Set (upsert semantics)
		// and Bleve IndexBatch (upsert) so this is safe before purging stale keys.
		if err := m.Store.BatchSaveTools(batchRecords, batchSchemas); err != nil {
			slog.Warn("sync: failed to batch save tools", "server", sc.Name, "error", err)
			return 0, fmt.Errorf("failed to batch save tools: %w", err)
		}

		// Update stored sync hash for future comparisons
		m.Store.SaveServerSyncHash(sc.Name, compositeHash)

		// 🛡️ DELTA-PURGE: Background cleanup of stale tool keys that are no longer
		// in the current tool list. This avoids a zero-tools window that would occur
		// if we purged BEFORE saving. Only orphaned keys get removed.
		var currentURNs []string
		for _, rec := range batchRecords {
			currentURNs = append(currentURNs, rec.URN)
		}
		go func(server string, urns []string) {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("sync: stale tool purge panicked", "server", server, "panic", r)
				}
			}()
			if err := m.Store.PurgeStaleServerTools(server, urns); err != nil {
				slog.Warn("sync: background stale tool purge failed", "server", server, "error", err)
			}
		}(sc.Name, currentURNs)

		// 🧠 SEMANTIC HYDRATOR: Populate intelligence for un-hydrated tools.
		// When LLM is configured: mark as "pending" so the daemon will process via LLM.
		// When LLM is NOT configured: use deterministic static templates as fallback.
		// Wrapped in recover() so hydration failures never block sync.
		llmConfigured := m.Config.Intelligence.Provider != "" && m.Config.Intelligence.APIKey != ""

		go func(records []*db.ToolRecord, schemas map[string]map[string]any) {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("sync: semantic hydrator panicked (recovered)", "server", sc.Name, "error", r)
				}
			}()

			// 🛡️ HNSW DELEGATION: Vector embedding is now handled exclusively by
			// the hydrator daemon post-sweep to prevent thundering herd of LLM API
			// calls during sync. Previously, this fired HydrateToolGraph
			// inline, creating 14+ concurrent embedding requests that stalled boot.

			for _, rec := range records {
				existing, err := m.Store.GetIntelligence(rec.URN)

				if llmConfigured {
					// LLM path: Mark un-hydrated tools as "pending" for the daemon
					if err == nil && existing != nil && existing.AnalysisStatus == "hydrated" {
						if existing.SchemaHash == rec.SchemaHash {
							continue // Schema unchanged, preserve existing hydration
						}
						if existing.SchemaHash == "" {
							// 🛡️ LEGACY MIGRATION: First boot with SchemaHash feature.
							// Port the live hash into the intel record without re-queuing for LLM.
							existing.SchemaHash = rec.SchemaHash
							if sErr := m.Store.SaveIntelligence(rec.URN, existing); sErr != nil {
								slog.Warn("sync: failed to migrate legacy schema hash", "urn", rec.URN, "error", sErr)
							} else {
								slog.Debug("sync: migrated legacy schema hash", "urn", rec.URN)
							}
							continue
						}
						// Hash genuinely changed — fall through to re-mark as pending
					}
					intel := &db.ToolIntelligence{
						AnalysisStatus: "pending",
						Metrics: db.ToolMetrics{
							ProxyReliability: computeWBase(rec.Description, rec.Category, schemas[rec.SchemaHash], nil),
						},
					}
					if err := m.Store.SaveIntelligence(rec.URN, intel); err != nil {
						slog.Warn("sync: failed to mark tool as pending", "urn", rec.URN, "error", err)
					} else {
						m.Store.PendingHydrations.Add(1)
						slog.Debug("sync: marked tool for LLM hydration", "urn", rec.URN)
					}
				} else {
					// Fallback path: Use static template generation
					if err == nil && existing != nil && existing.AnalysisStatus == hydratorVersion {
						if existing.SchemaHash == rec.SchemaHash {
							continue // Schema unchanged, preserve existing static hydration
						}
						if existing.SchemaHash == "" {
							// Legacy migration: port hash without regenerating
							existing.SchemaHash = rec.SchemaHash
							_ = m.Store.SaveIntelligence(rec.URN, existing)
							continue
						}
						// Hash genuinely changed — fall through to regenerate
					}

					category := rec.Category
					schema := schemas[rec.SchemaHash]

					negTriggers := generateNegativeTriggers(rec.Name, category)

					intel := &db.ToolIntelligence{
						AnalysisStatus:   hydratorVersion,
						SyntheticIntents: generateSyntheticIntents(rec.Name, rec.Description, category),
						LexicalTokens:    generateLexicalTokens(rec.Name, schema),
						NegativeTriggers: negTriggers,
						Metrics: db.ToolMetrics{
							ProxyReliability: computeWBase(rec.Description, category, schema, negTriggers),
						},
					}

					if err := m.Store.SaveIntelligence(rec.URN, intel); err != nil {
						slog.Warn("sync: failed to save static intelligence", "urn", rec.URN, "error", err)
					} else {
						slog.Debug("sync: static hydrated tool intelligence", "urn", rec.URN, "wbase", intel.Metrics.ProxyReliability)
					}
				}
			}
		}(batchRecords, batchSchemas)
	}
	return indexed, nil
}

// HydrateToolGraph asynchronously embeds all sub-server tools into the HNSW vector
// database core to enable semantic search across the entire tool ecosystem.
func (m *WarmRegistry) HydrateToolGraph(records []*db.ToolRecord) {
	e := vector.GetEngine()
	if e == nil || !e.VectorEnabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	count := 0
	for _, rec := range records {
		if rec == nil {
			continue
		}
		if err := e.AddDocument(ctx, rec.URN, rec.Description); err != nil {
			slog.Warn("sync: background graph hydration failed", "urn", rec.URN, "error", err)
		} else {
			count++
		}
	}
	if count > 0 {
		slog.Info("sync: semantic mapped vectors natively hydrated", "tools", count)
	}
}
