package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"strings"
	"time"

	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/intelligence"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// registerProxyTools delegates registration to specialized tool logic.
func (h *OrchestratorHandler) registerProxyTools(s *mcp.Server) {
	// Descriptions sourced from inventory.go via addTool().
	h.addTool(s, &mcp.Tool{Name: "align_tools"}, h.AlignTools)
	h.addTool(s, &mcp.Tool{Name: "call_proxy"}, h.CallProxy)
}

func (h *OrchestratorHandler) triggerAutoHeal(ctx context.Context) {
	if telemetry.SyncOutOfSync.Load() {
		if telemetry.IsHealing.CompareAndSwap(false, true) {
			slog.Warn("gateway: auto-healing triggered for OUT_OF_SYNC database")
			go func(c context.Context) {
				defer telemetry.IsHealing.Store(false)
				if err := h.Store.ReindexAllTools(); err == nil {
					telemetry.SyncOutOfSync.Store(false)
				} else {
					slog.Error("gateway: auto-healing failed", "error", err)
				}
			}(ctx)
		}
	}
}

// truncateSchemaDescriptions recursively truncates string description fields to 120 chars.
func truncateSchemaDescriptions(node any) {
	switch v := node.(type) {
	case map[string]any:
		for key, val := range v {
			if key == "description" {
				if str, ok := val.(string); ok && len(str) > 120 {
					v[key] = str[:117] + "..."
				}
			} else {
				truncateSchemaDescriptions(val)
			}
		}
	case []any:
		for _, val := range v {
			truncateSchemaDescriptions(val)
		}
	}
}

// AlignTools is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) AlignTools(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	defer func(start time.Time) {
		telemetry.MetaLatencies.AlignTools.Record(time.Since(start).Milliseconds())
	}(time.Now())

	h.triggerAutoHeal(ctx)

	var args struct {
		Query      string `json:"query"`
		ServerName string `json:"server_name,omitempty"`
		Category   string `json:"category,omitempty"`
		FullSchema bool   `json:"full_schema,omitempty"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	if args.ServerName == "" && args.Query != "" {
		preferredServers := h.Store.AnalyzeIntent(ctx, args.Query)
		if len(preferredServers) > 0 {
			args.ServerName = preferredServers[0]
			slog.Log(ctx, util.LevelTrace, "[HANDLER] align_tools intent pre-filter", "injected_server", args.ServerName)
		}
	}

	slog.Log(ctx, util.LevelTrace, "[HANDLER] align_tools entry", "query", args.Query, "server_name", args.ServerName, "category", args.Category)

	var results []*db.ToolRecord
	var err error

	telemetry.SearchMetrics.TotalSearches.Add(1)

	// 🛡️ LRU FAST PATH
	cacheKey := fmt.Sprintf("%s|%s|%s", args.Query, args.Category, args.ServerName)
	if cached, ok := h.AlignCache.Get(cacheKey); ok {
		telemetry.SearchMetrics.AlignCacheHits.Add(1)
		results = cached
	} else {
		telemetry.SearchMetrics.AlignCacheMisses.Add(1)
		// 🛡️ URN PREFIX FAST PATH
		searchQuery := args.Query
		if !strings.Contains(searchQuery, ":") && args.ServerName != "" && searchQuery != "" {
			searchQuery = args.ServerName + ":" + searchQuery
		}
		if strings.Contains(searchQuery, ":") {
			if tr, hErr := h.Store.GetTool(searchQuery); hErr == nil && tr != nil {
				// If it's a perfect URN match, bypass search entirely natively.
				results = append(results, tr)
			}
		}

		// 🛡️ ORCHESTRATOR EXACT-MATCH FAST PATH
		h.toolsMu.RLock()
		var exactInternalMatch *db.ToolRecord
		for _, it := range h.InternalTools {
			if strings.EqualFold(it.Name, args.Query) || strings.EqualFold("magictools:"+it.Name, args.Query) {
				exactInternalMatch = &db.ToolRecord{
					URN:                    "magictools:" + it.Name,
					Name:                   it.Name,
					Server:                 "magictools",
					Category:               it.Category,
					HighlightedDescription: it.Description,
					IsNative:               true,
				}
				break
			}
		}
		h.toolsMu.RUnlock()

		if exactInternalMatch != nil {
			results = append(results, exactInternalMatch)
		}

		if len(results) == 0 {
			// Bleve Native string search is the primary search engine for align_tools.
			telemetry.SearchMetrics.LexicalSearches.Add(1)

			var threshold float64
			// 🛡️ ZERO THRESHOLD OVERRIDE: When query is empty (MatchAllQuery), we MUST
			// bypass the global score threshold because MatchAllQuery scores everything 1.0.
			// If threshold > 1.0, MatchAllQuery natively drops everything randomly!
			if args.Query == "" {
				threshold = 0.0
			} else {
				threshold = h.Config.ScoreThreshold
			}

			results, err = h.Store.SearchTools(ctx, args.Query, args.Category, args.ServerName, threshold, h.Config.ScoreFusionAlpha)

			// ── Option 5: Pseudo-Relevance Feedback (PRF) ──
			// If we got results, extract discriminative terms from top hits and
			// re-expand the query. Discard expansion if topic drift is detected.
			if err == nil && len(results) > 0 && args.Query != "" {
				prfTerms := intelligence.ExtractPRFTerms(results, args.Query, 5)
				if len(prfTerms) > 0 {
					expandedQuery := args.Query + " " + strings.Join(prfTerms, " ")
					expandedResults, expandErr := h.Store.SearchTools(ctx, expandedQuery, args.Category, args.ServerName, threshold, h.Config.ScoreFusionAlpha)
					if expandErr == nil && len(expandedResults) > 0 {
						// Topic drift detection: if overlap drops below 50%, discard expansion.
						overlap := intelligence.ComputeResultOverlap(results, expandedResults)
						if overlap >= 0.50 {
							results = expandedResults
							slog.Debug("align_tools: PRF expansion accepted", "terms", prfTerms, "overlap", overlap)
						} else {
							slog.Debug("align_tools: PRF expansion rejected (topic drift)", "overlap", overlap)
						}
					}
				}
			}
		}

		if err == nil || err.Error() == "No certain match" {
			h.AlignCache.Add(cacheKey, results)
		}
	}

	if err != nil && err.Error() != "No certain match" {
		return nil, err
	}

	h.toolsMu.RLock()
	var internalMatches []*db.ToolRecord
	for _, it := range h.InternalTools {
		// Filter by ServerName early boundary
		if args.ServerName != "" && !strings.EqualFold("magictools", args.ServerName) {
			continue
		}

		// Skip if exactly matched to prevent duplication
		alreadyIncluded := false
		for _, r := range results {
			if r.Name == it.Name && r.Server == "magictools" {
				alreadyIncluded = true
				break
			}
		}
		if alreadyIncluded {
			continue
		}

		if args.Query == "" || strings.Contains(strings.ToLower(it.Name), strings.ToLower(args.Query)) ||
			strings.Contains(strings.ToLower(it.Description), strings.ToLower(args.Query)) {
			if args.Category != "" && !strings.EqualFold(it.Category, args.Category) {
				continue
			}
			internalMatches = append(internalMatches, &db.ToolRecord{
				URN:                    "magictools:" + it.Name,
				Name:                   it.Name,
				Server:                 "magictools",
				Category:               it.Category,
				HighlightedDescription: it.Description,
				IsNative:               true,
			})
		}
	}
	h.toolsMu.RUnlock()

	results = append(results, internalMatches...)

	// ── Option 6: Contrastive Failure Proximity Filtering ──
	// Reorder results by applying failure anchor proximity penalties.
	// Tools with known failure patterns for this intent get demoted.
	if args.Query != "" && len(results) > 1 {
		type scored struct {
			tool    *db.ToolRecord
			penalty float64
		}
		var scoredResults []scored
		for _, r := range results {
			p := intelligence.CheckFailureProximity(ctx, h.Store, args.Query, r.URN)
			scoredResults = append(scoredResults, scored{tool: r, penalty: p})
		}
		// Sort: higher penalty (1.0 = no failure) first, lower penalty last.
		sort.SliceStable(scoredResults, func(i, j int) bool {
			return scoredResults[i].penalty > scoredResults[j].penalty
		})
		results = make([]*db.ToolRecord, len(scoredResults))
		for i, sr := range scoredResults {
			results[i] = sr.tool
		}
	}

	if len(results) == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No specific tool found."}}}, nil
	}

	var text strings.Builder
	lruUpdated := false

	envelope := struct {
		Metadata map[string]any      `json:"metadata"`
		Tools    []map[string]string `json:"tools"`
	}{
		Metadata: make(map[string]any),
		Tools:    make([]map[string]string, 0),
	}

	for i, r := range results {
		if i >= 5 {
			break
		}

		var schema map[string]any
		if r.IsNative {
			h.toolsMu.RLock()
			for _, it := range h.InternalTools {
				if it.Name == r.Name {
					schema = h.toSchemaMap(it.InputSchema)
				}
			}
			h.toolsMu.RUnlock()
		} else {
			var schemaErr error
			schema, schemaErr = h.Store.GetSchema(r.SchemaHash)
			if schemaErr != nil {
				slog.Warn("gateway: failed to retrieve schema", "hash", r.SchemaHash, "error", schemaErr)
			} else {
				// 🛡️ LRU BOUNDING: Add sub-server tools to the dynamic LRU cache for the System Prompt.
				// This ensures the agent has zero-friction access to the full JSON schema natively.
				t := &mcp.Tool{
					Name:        r.URN,
					Description: r.Description,
					InputSchema: schema,
				}
				if !strings.Contains(t.Name, ":") {
					t.Name = fmt.Sprintf("%s:%s", r.Server, r.Name)
				}
				t = util.SanitizeToolSchema(t)
				h.ActiveToolsLRU.Add(r.URN, t)
				lruUpdated = true
			}
		}

		schemaStatus := "Loaded directly into System Prompt"
		if args.FullSchema {
			schemaStatus = "Included in Description payload below"
		}

		envelope.Tools = append(envelope.Tools, map[string]string{
			"urn":           r.URN,
			"server":        r.Server,
			"category":      r.Category,
			"schema_status": schemaStatus,
		})

		// 🛡️ CHAT HISTORY OPTIMIZATION: Always output the full unminified Description so the agent
		// has immediate context, but OMIT the massive InputSchema JSON because it is being loaded
		// directly into the System Prompt via list_changed.
		if args.FullSchema {
			// If explicitly requested, we still dump the whole schema.
			schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
			text.WriteString(fmt.Sprintf("**[%s]**\nDescription: %s\nInputSchema:\n```json\n%s\n```\n\n",
				r.URN, r.Description, string(schemaJSON)))
		} else {
			text.WriteString(fmt.Sprintf("**[%s]**\nDescription: %s\n\n",
				r.URN, r.Description))
		}
	}

	envelope.Metadata["intent_match_count"] = len(results)
	envelope.Metadata["system_prompt_updated"] = lruUpdated

	// 🛡️ ZERO-LATENCY SYNC: If we added any tools to the LRU, notify the IDE immediately.
	// The IDE will call tools/list and pull the bounded LRU set + base tools.
	// We use AddTool/RemoveTools to trigger the list_changed notification natively via SDK.
	if lruUpdated && h.Server != nil {
		slog.Log(ctx, util.LevelTrace, "align_tools: triggering tools/list_changed notification")
		dummy := &mcp.Tool{
			Name:        "__magic_lru_sync__",
			Description: "Internal synchronization signal",
			InputSchema: map[string]any{"type": "object"},
		}
		h.Server.AddTool(dummy, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, nil
		})
		h.Server.RemoveTools("__magic_lru_sync__")
	}

	envJSON, _ := json.MarshalIndent(envelope, "", "  ")
	finalText := fmt.Sprintf("```json\n%s\n```\n\n### Descriptions\n\n%s", string(envJSON), text.String())

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: finalText}},
	}, nil
}

// CallProxy is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) CallProxy(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	callStart := time.Now()
	defer func() {
		telemetry.MetaLatencies.CallProxy.Record(time.Since(callStart).Milliseconds())
	}()

	h.triggerAutoHeal(ctx)

	ps := NewProxyService(h)
	var params struct {
		URN                string         `json:"urn"`
		Arguments          map[string]any `json:"arguments"`
		BypassMinification bool           `json:"bypass_minification"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &params); err != nil {
		slog.Error("gateway: validation pre-check failed", "tool", "call_proxy", "error", err, "raw", string(req.Params.Arguments))
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	server, name, urn, toolRecord, err := ps.ResolveURN(ctx, params.URN)
	if err != nil {
		return nil, err
	}
	slog.Log(ctx, util.LevelTrace, "gateway: call_proxy start", "server", server, "tool", name)

	if server != "magictools" {
		go func() {
			h.Store.UpdateToolUsage(urn)
		}()
	}

	if h.Config.ValidateProxyCalls {
		if err := ps.ValidateArguments(ctx, urn, toolRecord, params.Arguments); err != nil {
			slog.Warn("gateway: pre-flight firewall trapped hallucination", "tool", name, "error", err)
			return nil, err
		}
	} else {
		slog.Log(ctx, util.LevelTrace, "gateway: proxy validation disabled by config", "tool", name)
	}

	return h.executeProxyPipeline(ctx, ps, params.Arguments, req, params.BypassMinification, server, name, urn, toolRecord)
}

func (h *OrchestratorHandler) executeProxyPipeline(ctx context.Context, ps *ProxyService, arguments map[string]any, req *mcp.CallToolRequest, bypassMinification bool, server, name, urn string, toolRecord *db.ToolRecord) (*mcp.CallToolResult, error) {

	// 🛡️ LOOPBACK: Native tools execute in-process, bypassing the external stdio proxy pipeline.
	if server == "magictools" {
		// 🛡️ RECURSION GUARD: Prevent tools from dispatching to themselves via loopback.
		// This happens when the fuzzy search resolves a query to itself
		// causing infinite self-referential recursion.
		if name == "align_tools" || name == "call_proxy" {
			return nil, fmt.Errorf("recursion_guard: tool %q cannot self-dispatch via loopback — use the resolved sub-server tool directly", name)
		}
		if handler, ok := h.loopbackHandlers[name]; ok {
			slog.Info("gateway: loopback dispatch", "tool", name)

			// 🛡️ TELEMETRY: Mark the node as actively executing in the DAG.
			telemetry.GlobalDAGTracker.UpdateActiveNode(urn, 0, 0, 0, "MISS", "")

			// 🛡️ ENVELOPE STRIPPING: Remarshal unwrapped sub-arguments and shallow copy the master request.
			unwrappedBytes, _ := json.Marshal(arguments)
			nativeReq := *req
			nativeReq.Params.Name = name
			nativeReq.Params.Arguments = unwrappedBytes

			res, err := handler(ctx, &nativeReq)

			var resSize int64
			if res != nil {
				resSize = measureResponseSize(res)
			}

			if err == nil && (res == nil || !res.IsError) {
				telemetry.GlobalDAGTracker.UpdateActiveNode(urn, 0, resSize, resSize, "HIT", "")
				telemetry.GlobalDAGTracker.CompleteNode(urn, true)
			} else {
				telemetry.GlobalDAGTracker.RecordFault(urn, "halt", 1, 1, "")
				telemetry.GlobalDAGTracker.CompleteNode(urn, false)
			}

			return res, err
		}
		return nil, fmt.Errorf("unknown internal tool: %s", name)
	}

	if h.isSafeToCache(urn) {
		cacheKey := h.getCacheKey(urn, arguments)
		if cached, ok := h.Responses.Get(cacheKey); ok {
			h.Telemetry.AddLatency(server, 0)
			telemetry.GlobalDAGTracker.UpdateActiveNode(urn, 0, 0, 0, "HIT", cacheKey)
			telemetry.GlobalDAGTracker.CompleteNode(urn, true)
			return cached, nil
		}
	}

	bootLatency := ps.EnsureServerReady(ctx, server)
	h.Telemetry.AddLatency(server, bootLatency)
	if bootLatency > 0 {
		telemetry.MetaLatencies.BootLatency.Record(bootLatency)
	}
	hotStart := time.Now() // Mark the start of boot-excluded hot path

	corrID := telemetry.NewCorrelationID()
	ctx = telemetry.WithCorrelationID(ctx, corrID)

	// 🛡️ CSSA TRACE: Correlation ID is propagated via context (telemetry.WithCorrelationID)
	// and logged in dispatch/complete messages. Do NOT inject into arguments — sub-servers
	// with strict schema validation (additionalProperties: false) will reject it.

	slog.Info("tool dispatch", "component", "backplane", "server", server, "tool", name, "urn", urn, "corr_id", corrID)

	parentID := telemetry.GetActiveCascadeParent()
	sourceServer := telemetry.GetActiveCascadeSource()
	if telemetry.GlobalRingBuffer != nil {
		spanJSON := fmt.Sprintf(`{"type":"SPAN_START","trace_id":%q,"parent_id":%q,"server":%q,"tool":%q,"start_time":%d}`, corrID, parentID, server, name, time.Now().UnixMilli())
		telemetry.GlobalRingBuffer.WriteRecord([]byte(spanJSON))
	}
	telemetry.RecordActiveDispatch(server, corrID)
	defer telemetry.ClearActiveDispatch(server)

	timeout := 120 * time.Second
	if toolRecord != nil && toolRecord.TimeoutSecs > 0 {
		timeout = time.Duration(toolRecord.TimeoutSecs) * time.Second
	}

	// 🛡️ DYNAMIC TOKEN QUOTA MIDDLEWARE (GOVERNOR)
	if h.Config.TokenSpendThresh > 0 && telemetry.GetTotalTokens() >= int64(h.Config.TokenSpendThresh) {
		slog.Warn("gateway: circuit breaker triggered - global token budget exceeded", "threshold", h.Config.TokenSpendThresh)
		return &mcp.CallToolResult{
			IsError: true,
			Content: append([]mcp.Content{}, &mcp.TextContent{Text: fmt.Sprintf("🛡️ ORCHESTRATOR MUZZLE EXCEPTION: Global session LLM token boundary heavily exceeded (%d / %d). Terminating runaway pipeline natively to prevent runaway budget burns.", telemetry.GetTotalTokens(), h.Config.TokenSpendThresh)}),
		}, nil
	}

	// 🛡️ TELEMETRY: Mark the node as actively executing in the DAG.
	telemetry.GlobalDAGTracker.UpdateActiveNode(urn, 0, 0, 0, "MISS", "")

	res, err := ps.ExecuteProxy(ctx, server, name, arguments, timeout)
	toolLatency := time.Since(hotStart).Milliseconds()
	if telemetry.GlobalRingBuffer != nil {
		endJSON := fmt.Sprintf(`{"type":"SPAN_END","trace_id":%q,"latency_ms":%d}`, corrID, toolLatency)
		telemetry.GlobalRingBuffer.WriteRecord([]byte(endJSON))
	}
	if err != nil {
		telemetry.ErrorTaxonomy.Classify(err)
		telemetry.RecentErrors.Record(server, corrID, err.Error())
		telemetry.GlobalToolTracker.Record(urn, toolLatency, true)
		telemetry.GlobalRouteTracker.RecordRoute(sourceServer, server, true)
		slog.Error("tool complete", "component", "backplane", "server", server, "tool", name, "urn", urn, "latency_ms", toolLatency, "status", "error", "error", err, "corr_id", corrID)
		telemetry.GlobalDAGTracker.RecordFault(urn, "halt", 1, 1, "")
		telemetry.GlobalDAGTracker.CompleteNode(urn, false)
		return nil, fmt.Errorf("invoke proxy error (%s): %w", urn, err)
	}
	slog.Info("tool complete", "component", "backplane", "server", server, "tool", name, "urn", urn, "latency_ms", toolLatency, "status", "ok", "corr_id", corrID)
	telemetry.GlobalToolTracker.Record(urn, toolLatency, false)
	telemetry.GlobalRouteTracker.RecordRoute(sourceServer, server, false)

	// 🛡️ TIER-2 HFSC: Detect extreme stream handshake from sub-servers.
	if tier2Res := h.interceptTier2HFSC(ctx, res, server); tier2Res != nil {
		return tier2Res, nil
	}

	// 🛡️ SQUEEZE BYPASS EVALUATION (ELEVATED)
	// Must execute before Tier-1 logic to ensure magicskills context is preserved natively.
	if !bypassMinification {
		bypassTargets := h.Config.GetSqueezeBypass()
		for _, target := range bypassTargets {
			if isTargetMatched(urn, target) {
				bypassMinification = true
				break
			}
		}

		if bypassMinification {
			telemetry.OptMetrics.SqueezeBypassCount.Add(1)
			slog.Debug("gateway: squeeze bypass activated", "urn", urn, "bypass_list", bypassTargets)
		} else {
			slog.Log(ctx, util.LevelTrace, "gateway: not in squeeze bypass list", "urn", urn)
		}
	}

	// 🛡️ TIER-1 HFSC (Orchestrator Native Extraction - NOW CONDITIONAL)
	// Automatically extract massive payloads to CSSA natively ONLY IF targeted.
	// Otherwise, falls through and restores default Tier-1 Squeeze Minifier.
	isRingBufferTarget := false
	for _, target := range h.Config.GetRingBufferTargets() {
		if isTargetMatched(urn, target) {
			isRingBufferTarget = true
			break
		}
	}

	if isRingBufferTarget && !bypassMinification && (len(res.Content) > 0 || res.StructuredContent != nil) {
		if tier1Res := h.interceptTier1Native(res, server, name); tier1Res != nil {
			return tier1Res, nil
		}
	}

	// Intercept and strip Orchestrator Signal Telemetry
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			var payload map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &payload); err == nil {
				// 🛡️ TOKEN AGGREGATOR: Scrape native token consumption footprints
				if spendRaw, hasSpend := payload["token_spend"]; hasSpend {
					var spend int
					switch v := spendRaw.(type) {
					case float64:
						spend = int(v)
					case int:
						spend = v
					}
					if spend > 0 {
						telemetry.AddTokens(spend)
					}
				}

				if sigR, hasSig := payload["__orchestrator_signal"]; hasSig {
					if sig, ok := sigR.(map[string]any); ok {
						success, _ := sig["success"].(bool)
						confidence, _ := sig["confidence"].(float64)
						go func(u string, s bool, conf float64) {
							if err := h.Store.UpdateToolMetrics(u, s, conf); err != nil {
								slog.Warn("gateway: failed to update tool metrics", "urn", u, "error", err)
							}
						}(urn, success, confidence)
					}
					delete(payload, "__orchestrator_signal")
					if stripped, err := json.Marshal(payload); err == nil {
						tc.Text = string(stripped)
					}
				}
			}
		}
	}

	// Phase 1: Soft failure inspection — detect empty/suspicious responses
	if diag := ps.InspectResponse(ctx, res, server, name); diag != nil && diag.Detected {
		if res.Meta == nil {
			res.Meta = make(map[string]any)
		}
		res.Meta["soft_failure"] = diag
		slog.Warn("gateway: soft failure detected", "server", server, "tool", name, "reason", diag.Reason)
		h.Telemetry.RecordSoftFailure(server)
	}

	// Measure raw size attributes for telemetry
	sentSize := int64(len(req.Params.Arguments))
	rawSize := measureResponseSize(res)

	if util.IsInternal(ctx) {
		slog.Debug("gateway: routing internal JSON-RPC response", "server", server, "tool", name)
		h.Telemetry.AddBytes(server, sentSize, rawSize, rawSize)
		return res, nil
	}

	truncateLimit := 2000
	if !bypassMinification {
		// 2. Dynamic JIT Squeeze Scaling
		if tuningRaw, ok := arguments["__orchestrator_squeeze_tuning"]; ok {
			if tuning, isMap := tuningRaw.(map[string]any); isMap {
				if limitRaw, exists := tuning["truncate_limit"]; exists {
					if limitFloat, isNum := limitRaw.(float64); isNum {
						truncateLimit = int(limitFloat)
					}
				}
			}
		}
	}

	var postSize int64
	if !bypassMinification {
		// MinifyResponse calls AddBytes internally with pre/post sizes
		res = ps.MinifyResponse(ctx, res, server, name, sentSize, truncateLimit)
		// If MinifyResponse exited early (no StructuredContent), it didn't
		// call AddBytes. Ensure we still track bytes for text-only responses.
		if res.StructuredContent == nil && rawSize > 0 {
			for _, c := range res.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					postSize += int64(len(tc.Text))
				}
			}
			// Only add if MinifyResponse didn't already (it sets StructuredContent=nil
			// after processing, so we check if raw content was originally text-only
			// by seeing if rawSize came from Content, not StructuredContent).
			if postSize == rawSize {
				h.Telemetry.AddBytes(server, sentSize, rawSize, rawSize)
			}
		} else {
			// postSize from minified content
			for _, c := range res.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					postSize += int64(len(tc.Text))
				}
			}
		}
	} else {
		if res.Meta == nil {
			res.Meta = make(map[string]any)
		}
		res.Meta["bypass_minification"] = true
		h.Telemetry.AddBytes(server, sentSize, rawSize, rawSize)
		postSize = rawSize

		if res.StructuredContent != nil {
			if rawJSON, marshalErr := json.MarshalIndent(res.StructuredContent, "", "  "); marshalErr == nil {
				md := fmt.Sprintf("```json\n%s\n```", string(rawJSON))
				res.Content = append([]mcp.Content{&mcp.TextContent{Text: md}}, res.Content...)
			} else {
				slog.Warn("gateway: failed to marshal structured content bypass", "error", marshalErr)
			}
		}
	}

	if !bypassMinification {
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok && len(tc.Text) > 24000 {
				tc.Text = util.CenterTruncate(tc.Text, 24000)
			}
		}
	}

	// Phase 3: Diagnostic enrichment — always-on pipeline metrics
	if res.Meta == nil {
		res.Meta = make(map[string]any)
	}
	squeezeRatio := float64(1)
	if rawSize > 0 {
		squeezeRatio = float64(postSize) / float64(rawSize)
	}
	res.Meta["_diagnostics"] = map[string]any{
		"raw_bytes":     rawSize,
		"post_bytes":    postSize,
		"squeeze_ratio": squeezeRatio,
		"minified":      !bypassMinification,
	}

	// 🛡️ O(1) Fast Byte Scanning to detect structural mutation mandates
	if !bypassMinification && rawSize > 0 {
		mutationTriggered := false
		if res.StructuredContent != nil {
			if structBytes, err := json.Marshal(res.StructuredContent); err == nil {
				if bytes.Contains(structBytes, []byte(`"mutation_required":true`)) || bytes.Contains(structBytes, []byte(`"mutation_required": true`)) {
					mutationTriggered = true
				}
			}
		}
		if !mutationTriggered {
			for _, c := range res.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					if strings.Contains(tc.Text, `"mutation_required":true`) || strings.Contains(tc.Text, `"mutation_required": true`) {
						mutationTriggered = true
						break
					}
				}
			}
		}

		if mutationTriggered {
			depth := telemetry.GlobalDAGTracker.IncrementMutationDepth()
			if depth <= 3 {
				slog.Warn("gateway: mid-pipeline mutation mandate detected natively. Synthesizing structural evolution.", "depth", depth)
				socraticNodes := []string{
					"brainstorm:brainstorm_complexity_forecaster",
					"brainstorm:antithesis_skeptic",
					"brainstorm:architectural_diagrammer",
					"go-refactor:generate_implementation_plan",
				}
				telemetry.GlobalDAGTracker.SpliceNodes(urn, socraticNodes)

				snapshot := telemetry.GlobalDAGTracker.Snapshot()
				if sid, ok := snapshot["session_id"].(string); ok && sid != "" {
					go func(sessionID string, state map[string]any) {
						bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						args := map[string]any{
							"domain": "session.meta",
							"key":    sessionID,
							"value":  state,
						}
						_, err := ps.ExecuteProxy(bgCtx, "recall", "upsert_session", args, 10*time.Second)
						if err != nil {
							slog.Error("gateway: failed to async commit DAG mutation to CSSA backplane", "error", err)
						}
					}(sid, snapshot)
				}
			} else {
				slog.Error("gateway: Topological Evolution bound exceeded (max 3), breaking infinite DAG loop")
			}
		}
	}

	// 🛡️ TELEMETRY: Update active node with final payload metrics and mark as DONE.
	telemetry.GlobalDAGTracker.UpdateActiveNode(urn, telemetry.GetTotalTokens(), rawSize, postSize, "MISS", "")
	telemetry.GlobalDAGTracker.CompleteNode(urn, true)

	telemetry.MetaLatencies.CallProxyHot.Record(time.Since(hotStart).Milliseconds())
	return res, nil
}

func transformToHybrid(rawJSON []byte, tokenLimit int) string {
	var m map[string]any
	if err := json.Unmarshal(rawJSON, &m); err != nil {
		return "## Tool Result\n- **Error**: Failed to decode sub-server JSON response."
	}

	var sb strings.Builder
	var headers []string
	summaryKeys := []string{"status", "count", "error", "message", "summary", "result_count", "outcome", "success"}
	for _, key := range summaryKeys {
		if v, ok := getIgnoreCase(m, key); ok {
			label := strings.ToUpper(key[:1]) + key[1:]
			if key == "result_count" {
				label = "Count"
			}
			headers = append(headers, fmt.Sprintf("- **%s**: %v", label, v))
		}
	}

	if len(headers) > 0 {
		sb.WriteString("### Summary\n")
		sb.WriteString(strings.Join(headers, "\n") + "\n\n")
	} else {
		sb.WriteString("## Tool Result\n")
	}

	var metadata []string
	metaKeywords := []string{"id", "timestamp", "version", "urn", "type", "created", "modified", "author", "hash", "uuid"}
	var toDelete []string
	for k, v := range m {
		kLower := strings.ToLower(k)
		isMeta := false
		for _, kw := range metaKeywords {
			if strings.Contains(kLower, kw) {
				isMeta = true
				break
			}
		}
		if isMeta {
			label := cases.Title(language.English).String(strings.ReplaceAll(k, "_", " "))
			metadata = append(metadata, fmt.Sprintf("- **%s**: %v", label, v))
			toDelete = append(toDelete, k)
		}
	}
	for _, k := range toDelete {
		delete(m, k)
	}

	if len(metadata) > 0 {
		sb.WriteString("#### Metadata\n")
		sb.WriteString(strings.Join(metadata, "\n") + "\n\n")
	}

	if len(m) > 0 {
		maxTokens := 1000
		if tokenLimit > 0 && tokenLimit < 1000 {
			maxTokens = tokenLimit
		}
		charLimit := maxTokens * 4
		dataJSON, dataErr := json.MarshalIndent(m, "", "  ")
		if dataErr != nil {
			slog.Warn("gateway: transformToHybrid failed to marshal subset", "error", dataErr)
			dataJSON = fmt.Appendf(nil, `{"error": "failed to encode: %v"}`, dataErr)
		}

		if len(dataJSON) > charLimit {
			sb.WriteString("> [!IMPORTANT]\n")
			sb.WriteString("> [Data too large; use get_raw(id=...) to fetch full content from bastion]\n")
		} else {
			sb.WriteString("```json:data\n")
			sb.WriteString(string(dataJSON))
			sb.WriteString("\n```")
		}
	}

	return sb.String()
}

func getIgnoreCase(m map[string]any, target string) (any, bool) {
	targetLower := strings.ToLower(target)
	for k, v := range m {
		if strings.ToLower(k) == targetLower {
			delete(m, k)
			return v, true
		}
	}
	return nil, false
}

func rawURN(urn string) string {
	urn = strings.TrimPrefix(urn, "urn:")
	urn = strings.TrimPrefix(urn, "tool:")
	return urn
}

// measureResponseSize calculates the total byte size of a CallToolResult
// by summing text content and marshalled structured content.
func measureResponseSize(res *mcp.CallToolResult) int64 {
	var size int64
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			size += int64(len(tc.Text))
		}
	}
	if res.StructuredContent != nil {
		if b, err := json.Marshal(res.StructuredContent); err == nil {
			size += int64(len(b))
		}
	}
	return size
}

func summarize(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 10 {
		return strings.Join(lines[:10], "\n") + "\n... (more lines available in full resource)"
	}
	return text
}

func (h *OrchestratorHandler) isPreferred(ctx context.Context, urn, query string) bool {
	preferredServers := h.Store.AnalyzeIntent(ctx, query)
	if len(preferredServers) == 0 {
		return false
	}

	u := strings.ToLower(urn)
	for _, server := range preferredServers {
		if strings.HasPrefix(u, strings.ToLower(server)+":") {
			return true
		}
	}
	return false
}

// isTargetMatched determines if the routing 'target' (e.g. "magicskills" or "magicskills:magicskills_match")
// correctly intercepts the specific server/tool pair (URN format).
func isTargetMatched(urn string, target string) bool {
	if !strings.Contains(target, ":") {
		// Broad server match
		return strings.HasPrefix(urn, target+":")
	}
	// Specific tool match
	return urn == target
}

// interceptTier1Native natively scans vanilla JSON-RPC results for massive payload boundaries
// (>8KB strings). If tripped, it strips the text from RAM, streams it directly
// to the CSSA GlobalRingBuffer natively, and returns an ultra-compact pointer to the LLM agent without ever touching the Squeezer minifier.
func (h *OrchestratorHandler) interceptTier1Native(res *mcp.CallToolResult, _, _ string) *mcp.CallToolResult {
	if res == nil {
		return nil
	}

	// Structural Payload Check
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err == nil && len(b) > 24000 {
			if telemetry.GlobalRingBuffer != nil {
				// Native payload serialization strictly memory mapped to GlobalRingBuffer
				telemetry.GlobalRingBuffer.WriteRecord(b)
			}

			// Hybrid JSON markdown_payload extraction: if the StructuredContent
			// contains a pre-rendered GFM markdown payload, surface it directly
			// as TextContent for model-agnostic consumption.
			if mdPayload := extractMarkdownPayload(b); mdPayload != "" {
				res.StructuredContent = nil
				// Surface the GFM markdown directly as TextContent
				res.Content = []mcp.Content{
					&mcp.TextContent{Text: mdPayload},
				}
				if res.Meta == nil {
					res.Meta = make(map[string]any)
				}
				res.Meta["tier1_extracted"] = true
				res.Meta["hybrid_extracted"] = true
				return res
			}

			res.StructuredContent = nil
			res.Content = []mcp.Content{
				&mcp.TextContent{
					Text: "High-Fidelity structural JSON payload natively extracted over high-speed telemetry pipe into CSSA GlobalRingBuffer.\n\n> This payload exceeds backplane context visibility. Verify with specific read tools.",
				},
			}

			if res.Meta == nil {
				res.Meta = make(map[string]any)
			}
			res.Meta["tier1_extracted"] = true
			return res
		}
	}

	for i, content := range res.Content {
		tc, ok := content.(*mcp.TextContent)
		if !ok || len(tc.Text) <= 24000 {
			continue // Below threshold or not text
		}

		// Large payload detected! Intercept directly over native RingBuffer stream pipeline natively.
		if telemetry.GlobalRingBuffer != nil {
			telemetry.GlobalRingBuffer.WriteRecord([]byte(tc.Text))
		}

		// Mutate the original text to a pure LLM artifact reference
		// using Absolute URI standard
		res.Content[i] = &mcp.TextContent{
			Text: "High-Fidelity payload natively extracted over high-speed telemetry pipe into CSSA GlobalRingBuffer.\n\n> This payload exceeds backplane context visibility. Verify with specific read tools.",
		}

		if res.Meta == nil {
			res.Meta = make(map[string]any)
		}
		res.Meta["tier1_extracted"] = true
		// Fast exit upon first large extraction match
		return res
	}

	// Nothing intercepted or disk write failed implicitly
	return nil
}

// interceptTier2HFSC manages the extreme payload continuous base64 log streaming boundaries.
// It traps heavily specialized tool results holding the `hfsc_stream` Meta boolean, completely
// freezing the JSON-RPC proxy response loop indefinitely until the Sub-Server manually
// pushes the `HFSC_FINALIZE` log instruction closing the pipeline channel via `doneCh`.
func (h *OrchestratorHandler) interceptTier2HFSC(ctx context.Context, res *mcp.CallToolResult, server string) *mcp.CallToolResult {
	// Check Meta["hfsc_stream"] — guarantees explicit sub-server intervention requested
	if res.Meta == nil {
		return nil
	}
	enabled, ok := res.Meta["hfsc_stream"].(bool)
	if !ok || !enabled {
		return nil
	}

	sessionID, _ := res.Meta["session_id"].(string)
	filename, _ := res.Meta["filename"].(string)
	projectID, _ := res.Meta["project_id"].(string)
	model, _ := res.Meta["model"].(string)

	if sessionID == "" {
		slog.Warn("tier2: extreme stream handshake completely malformed", "server", server, "meta", res.Meta)
		return nil
	}

	slog.Info("tier2: extreme stream handshake intercepted — locking proxy thread",
		"server", server,
		"session_id", sessionID,
		"filename", filename,
	)

	// Register the Tier 2 stream trap and open immediate Host file descriptor pipe
	doneCh, err := h.Registry.HFSC.Register(sessionID, filename, projectID, model, server)
	if err != nil {
		slog.Error("tier2: cataclysmic stream pipe initialization failure", "server", server, "session", sessionID, "error", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("HFSC Fatal File Pipe Error: %v", err)}},
			IsError: true,
		}
	}

	// 🛡️ TRAP: Complete suspension of process pipe proxy until remote host Log Stream issues FINALIZE
	const tier2Timeout = 6 * time.Minute
	select {
	case <-doneCh:
		// Artifact has formally been materialized internally to disk!
	case <-time.After(tier2Timeout):
		slog.Error("tier2: cataclysmic timeout waiting for FINALIZE lock release", "session", sessionID, "server", server)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("HFSC Pipeline Timeout: %s extreme payload log stream stalled.", filename)}},
			IsError: true,
		}
	case <-ctx.Done():
		slog.Warn("tier2: connection violently severed by proxy client context expiration", "session_id", sessionID)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "HFSC Pipeline Terminated: Client context cancelled connection mid-stream."}},
			IsError: true,
		}
	}

	finalSafeFilename := fmt.Sprintf("%s_%s", sessionID, filepath.Base(filename))
	artifactPath := filepath.Join(h.Registry.HFSC.ArtifactDir(), finalSafeFilename)

	slog.Info("tier2: trap released, returning absolute artifact path natively",
		"session_id", sessionID,
		"server", server,
		"artifact", artifactPath,
	)

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{
			Text: fmt.Sprintf("Extreme Payload Stream Successfully Synchronized.\n\n[Registry Volume Mount](file://%s)", artifactPath),
		}},
		Meta: map[string]any{
			"hfsc_delivered": true,
			"session_id":     sessionID,
			"filename":       filename,
			"server":         server,
			"artifact_path":  artifactPath,
		},
	}
}

// extractMarkdownPayload checks if raw JSON bytes contain a top-level
// "markdown_payload" string field. If present, it returns the value.
// This enables the HFSC proxy to surface pre-rendered GFM markdown from
// Hybrid JSON envelopes (used by generate_final_report in brainstorm and
// go-refactor) directly as TextContent to the AI agent, bypassing the
// generic transformToHybrid squeeze path.
func extractMarkdownPayload(raw []byte) string {
	var envelope struct {
		MarkdownPayload string `json:"markdown_payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ""
	}
	return envelope.MarkdownPayload
}
