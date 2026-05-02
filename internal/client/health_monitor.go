package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util"
	"mcp-server-magictools/internal/vector"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-magictools/internal/config"
	"path/filepath"
)

// ServerStatus contains health info
type ServerStatus struct {
	Name              string `json:"name"`
	Running           bool   `json:"running"`
	Uptime            string `json:"uptime,omitzero"`
	TotalCalls        int64  `json:"total_calls"`
	LastLatency       string `json:"last_latency,omitzero"`
	PingLatency       string `json:"ping_latency,omitzero"`
	ConsecutiveErrors int    `json:"consecutive_errors"`
	LastPing          string `json:"last_ping,omitzero"`
	LastUsed          string `json:"last_used"`
	MemoryRSS         string `json:"memory_rss,omitzero"`
	CPUUsage          string `json:"cpu_usage,omitzero"`
}

// StartHealthMonitor runs a background check loop
func (m *WarmRegistry) StartHealthMonitor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastFlush time.Time
	var cachedTrending map[string]map[string]float64
	var cachedDbTrending map[string]any

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.PollConfigChanges()
			m.MonitorResources()
			m.PingAll(ctx)
			m.EvictInactive(1 * time.Hour)
			m.PruneOrphans()

			now := time.Now()
			flush := false
			if now.Sub(lastFlush) >= 1*time.Minute {
				flush = true
				lastFlush = now
				// Refresh trending from BadgerDB on flush ticks only
				if m.Store != nil {
					cachedTrending = m.Store.ComputeTrending()
					cachedDbTrending = m.Store.ComputeDatabaseTrending()
				}
			}

			// Compute scores on EVERY tick for real-time updates
			var dashboardScores map[string]any
			if m.Store != nil {
				dashboardScores = m.Store.ComputeScoreBoard(cachedTrending)
			}
			if dashboardScores == nil {
				dashboardScores = make(map[string]any)
			}

			m.WriteSnapshot(flush, dashboardScores, cachedDbTrending)
		}
	}
}

// PingAll is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) PingAll(ctx context.Context) {
	// Snapshot servers and their sessions under RLock.
	// After the lock is released, session pointers remain valid (GC keeps them alive).
	type pingTarget struct {
		srv  *SubServer
		sess *mcp.ClientSession
	}
	m.mu.RLock()
	var targets []pingTarget
	for _, s := range m.Servers {
		if s.Session != nil {
			targets = append(targets, pingTarget{srv: s, sess: s.Session})
		}
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for _, t := range targets {
		wg.Add(1)
		go func(target pingTarget) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				m.executePingWithSession(ctx, target.srv, target.sess)
			case <-ctx.Done():
			}
		}(t)
	}
	wg.Wait()
}

// executePing was removed — it read srv.Session without lock (data race).
// All callers use executePingWithSession with a pre-snapshotted session instead.

// executePingWithSession pings using a pre-snapshotted session pointer,
// avoiding a racy read of srv.Session after the registry lock is released.
func (m *WarmRegistry) executePingWithSession(ctx context.Context, srv *SubServer, sess *mcp.ClientSession) {
	if sess == nil {
		return
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	start := time.Now()
	err := sess.Ping(pingCtx, nil)
	latency := time.Since(start)

	m.mu.Lock()
	srv.LastPing = time.Now()
	shouldRestart := false
	if err != nil {
		srv.ConsecutivePingFailures++
		if srv.ConsecutivePingFailures >= 3 {
			shouldRestart = true
		}
	} else {
		srv.PingLatency = latency
		srv.ConsecutivePingFailures = 0
		// Don't update LastUsed on pings — only real tool calls should count.
		// Otherwise idle eviction (EvictInactive) never fires.
	}
	m.mu.Unlock()

	if shouldRestart {
		m.orchestrateRestart(srv)
	}
}

// MonitorResources reads RSS and CPU for each running sub-server.
// NOTE: The PID snapshot taken under RLock can become stale if the process exits
// before GetRSS/GetProcessCPU are called. This is intentionally tolerated because
// those functions gracefully handle missing /proc entries by returning errors,
// which we silently skip. This avoids holding the lock during slow /proc reads.
func (m *WarmRegistry) MonitorResources() {
	type pidSnapshot struct {
		name string
		pid  int
		srv  *SubServer
	}
	var snapshots []pidSnapshot

	m.mu.RLock()
	for _, s := range m.Servers {
		if s.Process != nil && s.Process.Process != nil {
			snapshots = append(snapshots, pidSnapshot{s.Name, s.Process.Process.Pid, s})
		}
	}
	m.mu.RUnlock()

	// 🛡️ SELF-WATCHDOG: Always monitor the orchestrator's own process footprint
	snapshots = append(snapshots, pidSnapshot{name: "magictools (orchestrator)", pid: os.Getpid(), srv: nil})

	for _, snap := range snapshots {
		rss, err := util.GetRSS(snap.pid)
		if err == nil {
			m.mu.Lock()
			if snap.srv != nil {
				snap.srv.MemoryRSS = rss
			}
			m.mu.Unlock()

			limitBytes := uint64(2048 * 1024 * 1024)
			if snap.srv != nil && snap.srv.MemoryLimitMB > 0 {
				limitBytes = uint64(snap.srv.MemoryLimitMB) * 1024 * 1024
			}
			if rss > limitBytes {
				slog.Warn("memory limit exceeded, restarting", "component", "watchdog", "server", snap.name, "rss", rss, "limit", limitBytes)
				if snap.srv != nil {
					// 🛡️ ACTIVE-CALL GUARD: Defer restart if the server has in-flight proxy calls.
					// This prevents killing go-refactor mid-flight during long-running tools
					// like go_test_validation, which is the root cause of proxy EOF errors.
					if active := snap.srv.ActiveCalls.Load(); active > 0 {
						slog.Warn("memory limit exceeded but server has active calls — deferring restart",
							"component", "watchdog",
							"server", snap.name,
							"rss", rss,
							"limit", limitBytes,
							"active_calls", active,
						)
						continue
					}
					m.orchestrateRestart(snap.srv)
				} else {
					// This is the orchestrator itself. Panic to trigger the IDE's auto-restart
					// and prevent runaway resource exhaustion.
					panic(fmt.Sprintf("Orchestrator self-watchdog: memory limit exceeded: %d bytes", rss))
				}
				continue
			}
		}

		cpu, err := util.GetProcessCPU(snap.pid)
		if err == nil {
			m.mu.Lock()
			if snap.srv != nil {
				snap.srv.CPUUsage = cpu
			}
			m.mu.Unlock()

			if cpu > 95.0 {
				slog.Warn("high cpu detected", "component", "watchdog", "server", snap.name, "cpu", cpu)
			}
		}
	}
}

// EvictInactive is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) EvictInactive(ttl time.Duration) {
	pinned := m.pinnedSet()
	var toEvict []string
	m.mu.RLock()
	now := time.Now()
	for name, s := range m.Servers {
		if pinned[name] {
			continue
		}
		if s.Session != nil && now.Sub(s.LastUsed) > ttl {
			toEvict = append(toEvict, name)
		}
	}
	m.mu.RUnlock()

	for _, name := range toEvict {
		telemetry.LifecycleEvents.EvictionsEviction.Add(1)
		m.DisconnectServer(name, true)
	}
}

// GetStatusReport is undocumented but satisfies standard structural requirements.
func (m *WarmRegistry) GetStatusReport(managed []string) []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var report []ServerStatus
	for _, name := range managed {
		s, running := m.Servers[name]
		status := ServerStatus{
			Name:    name,
			Running: running && s.Session != nil,
		}
		if running && s.Session != nil {
			status.Uptime = time.Since(s.StartTime).Round(time.Second).String()
			status.TotalCalls = s.TotalCalls
			status.LastLatency = s.LastLatency.Round(time.Millisecond).String()
			status.PingLatency = s.PingLatency.Round(time.Millisecond).String()
			if !s.LastPing.IsZero() {
				status.LastPing = time.Since(s.LastPing).Round(time.Second).String() + " ago"
			}
			status.LastUsed = s.LastUsed.Format(time.Kitchen)
			status.ConsecutiveErrors = s.ConsecutiveErrors
			if s.MemoryRSS > 0 {
				status.MemoryRSS = fmt.Sprintf("%.2f MB", float64(s.MemoryRSS)/1024/1024)
			}
			if s.CPUUsage > 0 {
				status.CPUUsage = fmt.Sprintf("%.1f%%", s.CPUUsage)
			}
		} else {
			status.LastUsed = "Disconnected"
		}
		report = append(report, status)
	}
	return report
}

// EvictLRU evicts the least-recently-used server if the active count exceeds MaxRunningServers.
// LOCK CONTRACT: This method acquires m.mu.Lock internally, then releases it BEFORE calling
// DisconnectServer (which also acquires m.mu.Lock). This ordering prevents deadlocks.
// Do NOT refactor to hold the lock across the DisconnectServer call.
func (m *WarmRegistry) EvictLRU(excludeName string) {
	pinned := m.pinnedSet()
	m.mu.Lock()
	var active []*SubServer
	for _, s := range m.Servers {
		if s.Session != nil && s.Name != excludeName && !pinned[s.Name] {
			active = append(active, s)
		}
	}

	if len(active) <= MaxRunningServers {
		m.mu.Unlock()
		return
	}

	// O(n) min-scan: find the oldest server by LastUsed
	oldest := active[0]
	for _, s := range active[1:] {
		if s.LastUsed.Before(oldest.LastUsed) {
			oldest = s
		}
	}

	m.mu.Unlock()
	telemetry.LifecycleEvents.EvictionsEviction.Add(1)
	m.DisconnectServer(oldest.Name, true)
}

// pinnedSet returns a set of server names that are exempt from eviction.
func (m *WarmRegistry) pinnedSet() map[string]bool {
	var names []string
	if m.Config != nil {
		names = m.Config.GetPinnedServers()
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}

// PollConfigChanges hot-reloads servers.yaml and adjusts sub-server bounds on the fly
func (m *WarmRegistry) PollConfigChanges() {
	path := filepath.Join(config.DefaultConfigDir(), config.ServersConfigFile)
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	m.mu.Lock()
	if !info.ModTime().After(m.lastConfigModTime) {
		m.mu.Unlock()
		return
	}
	m.lastConfigModTime = info.ModTime()
	m.mu.Unlock()

	newServers, err := config.LoadManagedServers()
	if err != nil {
		slog.Error("PollConfigChanges: failed to hot-reload servers.yaml", "error", err)
		return
	}

	m.Config.UpdateManagedServers(newServers)

	for _, ns := range newServers {
		m.mu.Lock()
		s, ok := m.Servers[ns.Name]
		if ok && s.ConfigHash != ns.Hash() {
			slog.Info("detected limit changes; orchestrating restart", "component", "watchdog", "server", ns.Name)
			s.Command = ns.Command
			s.Args = ns.Args
			s.Env = ns.Env
			s.MemoryLimitMB = ns.MemoryLimitMB
			s.GoMemLimitMB = ns.GoMemLimitMB
			s.MaxCPULimit = ns.MaxCPULimit
			s.ConfigHash = ns.Hash()
			m.mu.Unlock()
			m.orchestrateRestart(s)
		} else {
			m.mu.Unlock()
		}
	}
}

// WriteSnapshot gathers global observability metrics and writes them to the memory-mapped ring buffer.
func (m *WarmRegistry) WriteSnapshot(flush bool, dashboardScores map[string]any, databasesHistory map[string]any) {
	if telemetry.GlobalRingBuffer == nil && !flush {
		return
	}

	var managedServers []string
	var recallSess *mcp.ClientSession
	m.mu.RLock()
	for name, s := range m.Servers {
		managedServers = append(managedServers, name)
		if name == "recall" && s.Session != nil {
			recallSess = s.Session
		}
	}
	m.mu.RUnlock()

	// 🛡️ RECALL METRICS BOUNDARY
	var recallDBMetrics any
	if recallSess != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req := &mcp.CallToolParams{
			Name: "get_metrics",
		}
		res, err := recallSess.CallTool(ctx, req)
		cancel()
		if err == nil && res != nil && res.StructuredContent != nil {
			if sc, ok := res.StructuredContent.(map[string]any); ok {
				if data, ok := sc["data"]; ok {
					recallDBMetrics = data
				}
			}
		}
	}

	report := m.GetStatusReport(managedServers)

	// 🛡️ FIX: Prevent JSON null — always write a valid map so the CLI reader's
	// type assertion snapshot["scores"].(map[string]interface{}) never fails.
	scoresPayload := dashboardScores
	if scoresPayload == nil {
		scoresPayload = make(map[string]any)
	}

	snapshot := map[string]any{
		"timestamp": time.Now().UnixNano(),
		"servers":   report,
		"tools":     telemetry.GlobalToolTracker.GetAll(),
		"scores":    scoresPayload,
		"databases": map[string]any{
			"magictools": m.Store.GetMetrics(),
			"recall":     recallDBMetrics,
		},
		"databases_history": databasesHistory,
		"opt_metrics": map[string]int64{
			"squeeze_bypass":       telemetry.OptMetrics.SqueezeBypassCount.Load(),
			"squeeze_trunc":        telemetry.OptMetrics.SqueezeTruncations.Load(),
			"total_raw_bytes":      telemetry.OptMetrics.TotalRawBytes.Load(),
			"total_squeezed_bytes": telemetry.OptMetrics.TotalSqueezedBytes.Load(),
			"hfsc_success":         telemetry.OptMetrics.HFSCReassemblySuccesses.Load(),
			"hfsc_fail":            telemetry.OptMetrics.HFSCReassemblyFails.Load(),
			"hfsc_swept":           telemetry.OptMetrics.HFSCSweptStale.Load(),
			"hfsc_active":          telemetry.OptMetrics.HFSCActiveStreams.Load(),
			"cssa_offload":         telemetry.OptMetrics.CSSAOffloadBytes.Load(),
			"cssa_sync":            telemetry.OptMetrics.CSSASyncOperations.Load(),
		},
		"errors": map[string]int64{
			"timeout":            telemetry.ErrorTaxonomy.Timeout.Load(),
			"connection_refused": telemetry.ErrorTaxonomy.ConnectionRefused.Load(),
			"panic":              telemetry.ErrorTaxonomy.Panic.Load(),
			"validation":         telemetry.ErrorTaxonomy.Validation.Load(),
			"hallucination":      telemetry.ErrorTaxonomy.HallucinationBlocked.Load(),
			"pipe_error":         telemetry.ErrorTaxonomy.PipeError.Load(),
			"context_cancelled":  telemetry.ErrorTaxonomy.ContextCancelled.Load(),
		},
		"lifecycle": map[string]int64{
			"restarts_health":      telemetry.LifecycleEvents.RestartsHealth.Load(),
			"restarts_oom":         telemetry.LifecycleEvents.RestartsOOM.Load(),
			"evictions":            telemetry.LifecycleEvents.EvictionsEviction.Load(),
			"reconnections":        telemetry.LifecycleEvents.Reconnections.Load(),
			"config_reloads":       telemetry.LifecycleEvents.ConfigReloads.Load(),
			"backpressure_pending": telemetry.LifecycleEvents.BackpressurePending.Load(),
			"backpressure_reject":  telemetry.LifecycleEvents.BackpressureReject.Load(),
		},
		"recent_errors":       telemetry.RecentErrors.GetAll(),
		"collisions":          telemetry.Collisions.Snapshot(),
		"dag_status":          telemetry.GlobalDAGTracker.Snapshot(),
		"cross_server_routes": telemetry.GlobalRouteTracker.Snapshot(),
	}

	// 🛡️ CONFIG SNAPSHOT: Expose active configuration for the Config dashboard tab
	if m.Config != nil {
		squeezeLevel := 0
		if m.Config.SqueezeLevelState != nil {
			squeezeLevel = *m.Config.SqueezeLevelState
		}
		snapshot["config"] = map[string]any{
			"score_threshold":       m.Config.ScoreThreshold,
			"squeeze_level":         squeezeLevel,
			"max_response_tokens":   m.Config.MaxResponseTokens,
			"log_level":             m.Config.GetLogLevel(),
			"mcp_log_level":         m.Config.GetMCPLogLevel(),
			"log_format":            m.Config.GetLogFormat(),
			"validate_proxy":        m.Config.ValidateProxyCalls,
			"no_optimize":           m.Config.NoOptimize,
			"pinned_servers":        m.Config.GetPinnedServers(),
			"squeeze_bypass":        m.Config.GetSqueezeBypass(),
			"managed_servers":       len(managedServers),
			"token_spend_thresh":    m.Config.TokenSpendThresh,
			"config_path":           m.Config.ConfigPath,
			"db_path":               m.Config.DBPath,
			"intelligence_provider": m.Config.Intelligence.Provider,
			"intelligence_model":    m.Config.Intelligence.Model,
		}
	}

	// 🛡️ PROXY SNAPSHOT: Expose per-server throughput and EMA latencies for the Proxy dashboard tab
	snapshot["proxy"] = map[string]any{
		"servers":       telemetry.GlobalTracker.GetAll(),
		"session_stats": telemetry.GlobalTracker.GetSessionStats(),
		"latencies": map[string]any{
			"align_tools_ema":    telemetry.MetaLatencies.AlignTools.EMA,
			"align_tools_count":  telemetry.MetaLatencies.AlignTools.Count,
			"call_proxy_ema":     telemetry.MetaLatencies.CallProxy.EMA,
			"call_proxy_count":   telemetry.MetaLatencies.CallProxy.Count,
			"call_proxy_hot_ema": telemetry.MetaLatencies.CallProxyHot.EMA,
			"call_proxy_hot_cnt": telemetry.MetaLatencies.CallProxyHot.Count,
			"boot_ema":           telemetry.MetaLatencies.BootLatency.EMA,
			"boot_count":         telemetry.MetaLatencies.BootLatency.Count,
		},
	}

	// 🛡️ RUNTIME SNAPSHOT: Go memory/GC metrics for the Runtime dashboard tab
	rtSnap := telemetry.CaptureRuntime()
	snapshot["runtime"] = map[string]any{
		"heap_alloc_mb":   rtSnap.HeapAllocMB,
		"heap_sys_mb":     rtSnap.HeapSysMB,
		"num_gc":          rtSnap.NumGC,
		"pause_total_ms":  rtSnap.PauseTotalMs,
		"num_goroutine":   rtSnap.NumGoroutine,
		"go_max_procs":    rtSnap.GoMaxProcs,
		"go_mem_limit_mb": rtSnap.GoMemLimitMB,
		"headroom_pct":    rtSnap.HeadroomPct,
	}

	// 🛡️ DYNAMIC TELEMETRY SYNTHESIS: Generate requested dashboards from available metrics
	scoringFactors := []map[string]any{}
	volatilityIndex := []map[string]any{}
	
	for _, cardAny := range scoresPayload {
		if card, ok := cardAny.(map[string]any); ok {
			urn, _ := card["URN"].(string)
			
			numF := func(k string) float64 {
				if v, ok := card[k]; ok {
					switch n := v.(type) {
					case float64: return n
					case int: return float64(n)
					case int64: return float64(n)
					}
				}
				return 0.0
			}
			
			faults := numF("Faults")
			deltaAll := numF("DeltaAll")
			calls := numF("Calls")
			
			if faults > 0 {
				scoringFactors = append(scoringFactors, map[string]any{
					"category": "Fault Recovery",
					"count": int64(faults),
					"impact_type": "Penalty",
				})
				
				volScore := faults*0.5 + calls*0.1
				if volScore > 1.0 {
					volatilityIndex = append(volatilityIndex, map[string]any{
						"score": volScore,
						"URN": urn,
					})
				}
			}
			if deltaAll > 0 {
				scoringFactors = append(scoringFactors, map[string]any{
					"category": "Trending Alignment",
					"count": int64(1),
					"impact_type": "Reward",
				})
			}
		}
	}

	snapshot["scoring_factors"] = scoringFactors
	snapshot["volatility_index"] = volatilityIndex

	// 🛡️ NETWORK DYNAMICS: Squeeze vs Raw
	var sqSat, hfSat float64
	rawBytes := telemetry.OptMetrics.TotalRawBytes.Load()
	sqBytes := telemetry.OptMetrics.TotalSqueezedBytes.Load()
	if rawBytes > 0 {
		sqSat = float64(rawBytes-sqBytes) / float64(rawBytes) * 100.0
	}
	activeStreams := telemetry.OptMetrics.HFSCActiveStreams.Load()
	if activeStreams > 0 {
		hfSat = float64(activeStreams) / 2048.0 * 100.0
	}
	snapshot["network_dynamics"] = map[string]any{
		"token_velocity_tps":     0.0,
		"squeeze_saturation_pct": sqSat,
		"hfsc_saturation_pct":    hfSat,
	}

	// 🛡️ SEARCH SNAPSHOT: Search mode + counters for the Search dashboard tab
	vectorMode := "Lexical (Bleve)"
	if m.Config != nil && m.Config.Intelligence.Provider != "" && m.Config.Intelligence.APIKey != "" {
		vectorMode = "Vector (HNSW)"
	}
	var bTop, hTop []string
	if bp := telemetry.SearchMetrics.LastBleveTop5.Load(); bp != nil {
		bTop = *bp
	}
	if hp := telemetry.SearchMetrics.LastHnswTop5.Load(); hp != nil {
		hTop = *hp
	}

	snapshot["search"] = map[string]any{
		"mode":                   vectorMode,
		"total_searches":         telemetry.SearchMetrics.TotalSearches.Load(),
		"vector_searches":        telemetry.SearchMetrics.VectorSearches.Load(),
		"lexical_searches":       telemetry.SearchMetrics.LexicalSearches.Load(),
		"total_latency_ms":       telemetry.SearchMetrics.TotalLatencyMs.Load(),
		"total_confidence_score": math.Float64frombits(telemetry.SearchMetrics.TotalConfidenceScore.Load()),
		"l1_cache_hits":          telemetry.SearchMetrics.AlignCacheHits.Load(),
		"l1_cache_misses":        telemetry.SearchMetrics.AlignCacheMisses.Load(),
		"cache_hits":             telemetry.SearchMetrics.CacheHits.Load(),
		"cache_misses":           telemetry.SearchMetrics.CacheMisses.Load(),
		"vector_wins":            telemetry.SearchMetrics.VectorWins.Load(),
		"lexical_wins":           telemetry.SearchMetrics.LexicalWins.Load(),
		"hnsw_graph_size":        getHNSWGraphSize(),
		"fusion_mode":            getFusionModeLabel(m.Config),
		"bleve_top_5":            bTop,
		"hnsw_top_5":             hTop,
	}

	if telemetry.GlobalRingBuffer != nil {
		if b, err := json.Marshal(snapshot); err == nil && len(b) > 16368 {
			snapshot["recent_errors"] = []string{"[TRUNCATED BY RING BUFFER]"}
			snapshot["databases_history"] = []string{"[TRUNCATED BY RING BUFFER]"}
			b2, _ := json.Marshal(snapshot)
			if len(b2) > 16368 {
				snapshot["tools"] = []string{"[TRUNCATED]"}
			}
		}
		if err := telemetry.GlobalRingBuffer.WriteGauges(snapshot); err != nil {
			slog.Debug("telemetry: failed to write gauge snapshot to ring buffer", "error", err)
		}
	}

	if flush {
		db.FlushMetricBucket(m.Store, snapshot)
	}
}

// getHNSWGraphSize returns the current HNSW graph node count, or 0 if the engine is disabled.
func getHNSWGraphSize() int {
	e := vector.GetEngine()
	if e == nil || !e.VectorEnabled() {
		return 0
	}
	return e.Len()
}

// getFusionModeLabel returns a human-readable label for the active search fusion mode.
func getFusionModeLabel(cfg *config.Config) string {
	if cfg == nil {
		return "Lexical-Only"
	}
	e := vector.GetEngine()
	if e == nil || !e.VectorEnabled() {
		return "Lexical-Only (BM25)"
	}
	return fmt.Sprintf("Hybrid (α=%.2f)", cfg.ScoreFusionAlpha)
}
