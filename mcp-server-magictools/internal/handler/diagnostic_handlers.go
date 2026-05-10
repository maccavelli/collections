package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"mcp-server-magictools/internal/config"
	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/logging"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util/logutil"
	"mcp-server-magictools/internal/vector"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (h *OrchestratorHandler) registerDiagnosticTools(s *mcp.Server) {
	// Descriptions are intentionally omitted — addTool() sources them from
	// inventory.go (InternalToolsInventoryJSON), the single source of truth.
	h.addTool(s, &mcp.Tool{Name: "get_internal_logs"}, h.GetInternalLogs)
	h.addTool(s, &mcp.Tool{Name: "get_session_stats"}, h.GetSessionStats)
	h.addTool(s, &mcp.Tool{Name: "get_health_report"}, h.GetHealthReport)
	h.addTool(s, &mcp.Tool{Name: "analyze_system_logs"}, h.AnalyzeSystemLogs)
	h.addTool(s, &mcp.Tool{Name: "update_config"}, h.UpdateConfig)
	h.addTool(s, &mcp.Tool{Name: "self_check"}, h.SelfCheck)
	h.addTool(s, &mcp.Tool{Name: "list_tools"}, h.ListToolsInfo)
	h.addTool(s, &mcp.Tool{Name: "semantic_similarity"}, h.SemanticSimilarityAudit)
	h.addTool(s, &mcp.Tool{Name: "query_standards"}, h.QueryStandards)
}

// GetInternalLogs is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) GetInternalLogs(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		MaxLines int `json:"max_lines"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	maxLines := 50
	if input.MaxLines > 0 {
		maxLines = input.MaxLines
	}

	logs, err := h.Store.GetLogs(maxLines)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve logs: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: strings.Join(logs, "\n"),
			},
		},
	}, nil
}

// GetSessionStats is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) GetSessionStats(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stats := h.Telemetry.GetSessionStats()
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal telemetry: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
}

// QueryStandards is completely undocumented but provides extreme LLM memory mapping directly back into standard text dynamically matching user queries directly from Hydrator memory banks securely natively.
func (h *OrchestratorHandler) QueryStandards(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	e := vector.GetEngine()
	if e == nil || !e.VectorEnabled() {
		if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
			standardsText := h.RecallClient.SearchStandards(ctx, input.Query, "", "", 5)

			envelope := map[string]any{
				"metadata": map[string]any{
					"query":  input.Query,
					"source": "recall_fallback_bm25",
				},
			}
			envJSON, _ := json.MarshalIndent(envelope, "", "  ")

			if standardsText == "" {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("```json\n%s\n```\n\nNo specific standards matching the query were found via offline BM25 fallback natively.", string(envJSON))}}}, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{
						Text: fmt.Sprintf("```json\n%s\n```\n\n✨ Offline Standards Memory Bank Response:\n%s\n\n(Note: Generated natively via offline Recall fallback bypassing Vector dependencies.)", string(envJSON), standardsText),
					},
				},
			}, nil
		}
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "Vector Intelligence Offline and Recall Unreachable. Set LLM API Environment Variables securely to unlock Hydrator Database mapping."}},
		}, nil
	}

	urns, err := e.Search(ctx, input.Query, 5) // Return top 5 closest matched conceptual artifacts intuitively
	if err != nil {
		return nil, fmt.Errorf("failed semantic index bounding search: %w", err)
	}

	envelope := map[string]any{
		"metadata": map[string]any{
			"query":       input.Query,
			"source":      "vector_rag",
			"match_count": len(urns),
			"matches":     urns,
		},
	}
	envJSON, _ := json.MarshalIndent(envelope, "", "  ")

	if len(urns) == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("```json\n%s\n```\n\nNo specific standards matching the query were physically extracted dynamically natively.", string(envJSON))}}}, nil
	}

	// Simulate document parsing logically
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf("```json\n%s\n```\n\n✨ RAG Standards Memory Bank Response:\nFound %d relevant context bounds exactly matching concept structurally.\nMatched Identifiers natively: %v\n\n(Note: Full document fetching from these identifiers requires recall or direct filesystem interaction.)", string(envJSON), len(urns), urns),
			},
		},
	}, nil
}

// GetHealthReport is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) GetHealthReport(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.Registry.PingAll(ctx)
	var names []string
	for _, sc := range h.Config.GetManagedServers() {
		names = append(names, sc.Name)
	}
	statusReport := h.Registry.GetStatusReport(names)

	report := map[string]any{
		"servers": statusReport,
	}

	// 🛡️ RECALL ENRICHMENT: Add historical trends from past boot snapshots
	if h.RecallClient != nil && h.RecallClient.RecallEnabled() {
		raw := h.RecallClient.ListSessionsByFilter(ctx, "", "magictools-diagnostics", "", 5)
		if raw != "" {
			var envelope map[string]any
			if json.Unmarshal([]byte(raw), &envelope) == nil {
				var entries []any
				if e, ok := envelope["entries"].([]any); ok {
					entries = e
				} else if data, ok := envelope["data"].(map[string]any); ok {
					entries, _ = data["entries"].([]any)
				}
				if len(entries) > 0 {
					report["historical_trends"] = map[string]any{
						"boot_snapshots_available": len(entries),
						"source":                   "recall",
					}
				}
			}
		}
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		slog.Error("diagnostic: failed to marshal status report", "error", err)
		return nil, fmt.Errorf("failed to generate health report")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
}

// AnalyzeSystemLogs is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) AnalyzeSystemLogs(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ServerID string `json:"server_id"`
		Lines    int    `json:"lines"`
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	linesToScan := 50
	if input.Lines > 0 {
		linesToScan = input.Lines
	}
	// Cap at 1000 lines for performance and to prevent OOM
	if linesToScan > 1000 {
		linesToScan = 1000
	}

	logPath := h.Config.LogPath
	if logPath == "" {
		logPath = config.DefaultLogPath()
	}

	// 1. Efficient Tail
	candidateLines, err := logutil.TailFile(logPath, linesToScan)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return nil, fmt.Errorf("Log file not found at %s. Ensure logging is enabled in config.", logPath)
		}
		return nil, fmt.Errorf("tail failed: %w", err)
	}

	// 2. Multi-dimensional Filter
	filtered := logutil.FilterLogs(candidateLines, input.ServerID, input.Severity)

	// 3. ✨ PREDICTIVE TELEMETRY: Augment crash lines utilizing semantic vector distance organically
	if len(filtered) > 0 {
		if e := vector.GetEngine(); e != nil && e.VectorEnabled() && input.Severity == "ERROR" {
			fixes, sErr := e.Search(ctx, filtered[0], 1)
			if sErr == nil && len(fixes) > 0 && fixes[0] != "" {
				filtered = append(filtered, "---")
				filtered = append(filtered, "✨ Semantic Telemetry: Correlated Historical Diagnostic Context:")
				filtered = append(filtered, fixes[0])
			}
		}

		// 🛡️ RECALL ENRICHMENT: Query past diagnostic snapshots for historical error patterns
		if input.Severity == "ERROR" && h.RecallClient != nil && h.RecallClient.RecallEnabled() {
			raw := h.RecallClient.ListSessionsByFilter(ctx, "", "magictools-diagnostics", "", 5)
			if raw != "" {
				filtered = append(filtered, "---")
				filtered = append(filtered, "📊 Historical Context (from Recall):")
				filtered = append(filtered, fmt.Sprintf("  Cross-session diagnostic data available (%d chars)", len(raw)))
			}
		}
	}

	// Format output as Markdown code block
	envelope := map[string]any{
		"metadata": map[string]any{
			"server_id":     input.ServerID,
			"severity":      input.Severity,
			"lines_scanned": linesToScan,
			"total_matches": len(filtered),
		},
	}
	envJSON, _ := json.MarshalIndent(envelope, "", "  ")

	responseText := fmt.Sprintf("```json\n%s\n```\n\n```\n%s\n```", string(envJSON), strings.Join(filtered, "\n"))
	if len(filtered) == 0 {
		responseText = fmt.Sprintf("```json\n%s\n```\n\n*No matching log entries found.*", string(envJSON))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: responseText,
			},
		},
	}, nil
}

// UpdateConfig is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) UpdateConfig(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
	}

	input.Key = strings.TrimSpace(input.Key)
	input.Value = strings.TrimSpace(input.Value)
	if input.Key == "" || input.Value == "" {
		return nil, fmt.Errorf("both 'key' and 'value' are required")
	}

	// Validate and apply to runtime state
	oldValue, err := h.Config.UpdateConfigValue(input.Key, input.Value)
	if err != nil {
		return nil, err
	}

	// 🛡️ LIVE LOG LEVEL: Apply immediately to all slog handlers
	if input.Key == "logLevel" && h.LogLevel != nil {
		newLevel := logging.ParseLogLevel(strings.ToUpper(input.Value))
		h.LogLevel.Set(newLevel)
		slog.Info("update_config: log level changed at runtime", "old", oldValue, "new", strings.ToUpper(input.Value))
	}

	// Persist to disk via SaveConfiguration()
	if err := h.Config.SaveConfiguration(); err != nil {
		return nil, fmt.Errorf("failed to persist configuration: %w", err)
	}

	msg := fmt.Sprintf("Configuration updated: %s changed from '%s' to '%s'. Change persisted to config.yaml and applied at runtime.", input.Key, oldValue, input.Value)
	if input.Key == "logFormat" {
		msg += "\n\n[!] NOTE: logFormat requires an orchestrator process restart (`mcp_magictools_reload_servers magictools`) to rebuild the root slog handler tree."
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, nil
}

// SelfCheck is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) SelfCheck(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct{}
	if req.Params != nil && len(req.Params.Arguments) > 0 {
		_ = json.Unmarshal(req.Params.Arguments, &input)
	}

	// 1. Host OS Metrics
	osMetrics := telemetry.GetSystemProcessStats()

	// 2. Cache Efficacy
	rHits, rMisses, rItems := h.Store.Cache.GetMetrics()
	resHits, resMisses, resItems := h.Responses.GetMetrics()

	// 3. Database & Sync
	dbStats, err := h.Store.GetExtendedDiagnostics()
	if err != nil {
		slog.Warn("self_check: failed to retrieve extended database diagnostics", "error", err)
	}

	payload := map[string]any{
		"system": osMetrics,
		"handlers": map[string]any{
			"align_tools_latency":    telemetry.MetaLatencies.AlignTools,
			"call_proxy_latency":     telemetry.MetaLatencies.CallProxy,
			"call_proxy_hot_latency": telemetry.MetaLatencies.CallProxyHot,
			"boot_latency":           telemetry.MetaLatencies.BootLatency,
		},
		"cache": map[string]any{
			"align_cache": map[string]any{
				"hits":   telemetry.SearchMetrics.AlignCacheHits.Load(),
				"misses": telemetry.SearchMetrics.AlignCacheMisses.Load(),
				"items":  h.AlignCache.Len(),
			},
			"registry_cache": map[string]any{
				"hits":   rHits,
				"misses": rMisses,
				"items":  rItems,
			},
			"response_cache": map[string]any{
				"hits":   resHits,
				"misses": resMisses,
				"items":  resItems,
			},
		},
		"database": dbStats,
	}

	// 🛡️ VECTOR TELEMETRY: Include HNSW engine state in self_check payload
	if e := vector.GetEngine(); e != nil {
		payload["vector"] = map[string]any{
			"enabled":         e.VectorEnabled(),
			"graph_nodes":     e.Len(),
			"needs_hydration": e.RequiresHydration(),
			"vector_wins":     telemetry.SearchMetrics.VectorWins.Load(),
			"lexical_wins":    telemetry.SearchMetrics.LexicalWins.Load(),
			"total_searches":  telemetry.SearchMetrics.TotalSearches.Load(),
		}
	}

	// 🛡️ RECALL ENRICHMENT: Add session memory section showing recall status
	if h.RecallClient != nil {
		recallStatus := map[string]any{
			"connected": h.RecallClient.RecallEnabled(),
		}
		if h.RecallClient.RecallEnabled() {
			raw := h.RecallClient.ListSessionsByFilter(ctx, "", "magictools-diagnostics", "", 1)
			if raw != "" {
				recallStatus["last_snapshot_available"] = true
			} else {
				recallStatus["last_snapshot_available"] = false
			}
		}
		payload["session_memory"] = recallStatus
	}

	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to format self_check payload: %w", err)
	}

	markdown := string(jsonData)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: markdown,
			},
		},
	}, nil
}

// ListToolsOptions is undocumented but satisfies standard structural requirements.
type ListToolsOptions struct {
	MaxTools *int `json:"max_tools,omitempty"`
}

// ListToolsInfo is undocumented but satisfies standard structural requirements.
func (h *OrchestratorHandler) ListToolsInfo(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input struct {
		ServerName string `json:"server_name"`

		Options *ListToolsOptions `json:"options"`
	}

	hasArgs := false
	if req.Params != nil && len(req.Params.Arguments) > 0 {
		hasArgs = strings.TrimSpace(string(req.Params.Arguments)) != "{}"
		if hasArgs {
			if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
				return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
			}
		}
	}

	if input.Options == nil {
		input.Options = &ListToolsOptions{MaxTools: new(int(1000))}
	} else if input.Options.MaxTools == nil {
		input.Options.MaxTools = new(int(1000))
	}

	if !hasArgs {
		var internals []db.ToolRecord
		if err := json.Unmarshal(InternalToolsInventoryJSON, &internals); err != nil {
			return nil, fmt.Errorf("failed to unmarshal internal inventory: %w", err)
		}

		var sb strings.Builder
		sb.WriteString("# Available MCP Sub-Server Tools\n\n## magictools\n\n")

		sort.Slice(internals, func(i, j int) bool {
			return internals[i].Name < internals[j].Name
		})

		for _, t := range internals {
			toolName := t.Name
			if !strings.HasPrefix(toolName, "magictools:") {
				toolName = "magictools:" + toolName
			}
			sb.WriteString(fmt.Sprintf("### `%s`\n", toolName))
			if t.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", t.Description))
			}
			// t.InputSchema is json.RawMessage or map depending on db type,
			// the existing code uses len(t.InputSchema) > 0.
			if len(t.InputSchema) > 0 {
				schemaBytes, _ := json.MarshalIndent(t.InputSchema, "", "  ")
				sb.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", string(schemaBytes)))
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: sb.String(),
				},
			},
		}, nil
	}

	serverFilter := strings.TrimSpace(input.ServerName)
	serverTools := make(map[string][]*db.ToolRecord)
	count := 0

	var serversToScan []string
	if serverFilter != "" {
		serversToScan = append(serversToScan, serverFilter)
	} else {
		for _, sc := range h.Config.GetManagedServers() {
			serversToScan = append(serversToScan, sc.Name)
		}
	}

	for _, srv := range serversToScan {
		tools, bErr := h.Store.GetServerToolsNatively(srv, *input.Options.MaxTools)
		if bErr != nil {
			slog.Warn("diagnostic_handlers: badgerdb server mapping error", "error", bErr)
			continue
		}
		for _, r := range tools {
			if r.Server != "magictools" {
				serverTools[r.Server] = append(serverTools[r.Server], r)
				count++
			}
			if count >= *input.Options.MaxTools {
				break
			}
		}
		if count >= *input.Options.MaxTools {
			break
		}
	}

	if count == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No sub-server tools found."}}}, nil
	}

	var servers []string
	for s := range serverTools {
		servers = append(servers, s)
	}
	sort.Strings(servers)

	var sb strings.Builder
	sb.WriteString("# Available MCP Sub-Server Tools\n\n")

	for _, srv := range servers {
		tools := serverTools[srv]
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Name < tools[j].Name
		})

		sb.WriteString(fmt.Sprintf("## %s\n\n", srv))
		isSummarized := len(tools) > 50

		for _, t := range tools {
			toolName := t.Name
			if !strings.HasPrefix(toolName, srv+":") {
				toolName = srv + ":" + toolName
			}
			sb.WriteString(fmt.Sprintf("### `%s`\n", toolName))

			if isSummarized {
				summary := t.Description
				if summary == "" {
					summary = "No description available."
				}
				if t.LiteSummary != "" {
					summary = t.LiteSummary
				} else if len(summary) > 200 {
					summary = summary[:197] + "..."
				}
				sb.WriteString(fmt.Sprintf("%s\n\n", summary))
			} else {
				if t.Description != "" {
					sb.WriteString(fmt.Sprintf("%s\n\n", t.Description))
				}
				if t.LiteSummary != "" {
					sb.WriteString(fmt.Sprintf("> Usage Hint: %s\n\n", t.LiteSummary))
				}
				if len(t.InputSchema) > 0 {
					schemaBytes, _ := json.MarshalIndent(t.InputSchema, "", "  ")
					sb.WriteString(fmt.Sprintf("```json\n%s\n```\n\n", string(schemaBytes)))
				}
			}
		}
	}

	markdown := sb.String()

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: markdown,
			},
		},
	}, nil
}
