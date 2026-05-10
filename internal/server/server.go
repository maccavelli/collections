package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	"mcp-server-recall/internal/config"
	"mcp-server-recall/internal/memory"
	"mcp-server-recall/internal/telemetry"
)

const recallServerName = "mcp-server-recall"

// MCPRecallServer defines the server that implements the mcp-server-recall tools.
type MCPRecallServer struct {
	mcpServer       *mcp.Server
	store           *memory.MemoryStore
	logs            *LogBuffer
	cfg             *config.Config
	searchLatencyMS atomic.Int64 // Cumulative search latency
	searchCount     atomic.Int64 // Total search operations
	startTime       time.Time    // Application start time for uptime
}

// LogBuffer stores recent server logs in memory with ring-buffer semantics.
type LogBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

var secretRegex = regexp.MustCompile(`(?i)(token_|sk_|key_|secret_)[a-zA-Z0-9_-]+`)

func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	redacted := secretRegex.ReplaceAll(p, []byte("[REDACTED]"))
	_, err = lb.buf.Write(redacted)
	if err != nil {
		return len(p), err
	}
	n = len(p)

	if lb.buf.Len() > config.LogBufferLimit {
		data := lb.buf.Bytes()
		trimPoint := len(data) - config.LogTrimTarget
		if idx := bytes.IndexByte(data[trimPoint:], '\n'); idx >= 0 {
			trimPoint += idx + 1
		}
		newData := make([]byte, len(data)-trimPoint)
		copy(newData, data[trimPoint:])
		lb.buf.Reset()
		if _, err := lb.buf.Write(newData); err != nil {
			return 0, fmt.Errorf("trim buffer: %w", err)
		}
	}

	return n, nil
}

func (lb *LogBuffer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.buf.String()
}

// NewMCPRecallServer creates a new instance of the recall server.
func NewMCPRecallServer(cfg *config.Config, store *memory.MemoryStore, logs *LogBuffer, logger *slog.Logger) (*MCPRecallServer, error) {
	s := mcp.NewServer(
		&mcp.Implementation{
			Name:    cfg.Name(),
			Version: cfg.Version,
		},
		&mcp.ServerOptions{Logger: logger},
	)

	rs := &MCPRecallServer{
		mcpServer: s,
		store:     store,
		logs:      logs,
		cfg:       cfg,
		startTime: time.Now(),
	}

	rs.registerTools()

	telemetry.StartTelemetryLoop(cfg, store, logs.String)

	return rs, nil
}

// toolCatalog returns the central registry of all MCP tools.

func (rs *MCPRecallServer) handleContextVacuum(ctx context.Context, req *mcp.CallToolRequest, args ContextVacuumInput) (*mcp.CallToolResult, any, error) {

	if args.FlattenThreshold <= 0 {
		args.FlattenThreshold = 1000
	}
	if args.Namespace == "" {
		args.Namespace = "sessions"
	}
	if args.DedupThreshold <= 0 {
		args.DedupThreshold = 0.7
	}

	var reports []*memory.VacuumReport
	var sessionMutated int

	// Namespace dispatch.
	switch args.Namespace {
	case "sessions":
		if args.DaysOld <= 0 {
			args.DaysOld = rs.cfg.SessionPurgeDays()
		}
		mutated, err := rs.store.VacuumSessions(ctx, args.TargetOutcome, args.FlattenThreshold, args.DaysOld)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error performing session vacuum: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		sessionMutated = mutated

	case "memories":
		if args.DaysOld <= 0 {
			args.DaysOld = 30
		}
		report, err := rs.store.VacuumMemories(ctx, args.DaysOld, args.DedupThreshold, args.Category, args.ReportOnly)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error performing memory vacuum: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		reports = append(reports, report)

	case "standards":
		report, err := rs.store.VacuumStandards(ctx, args.ReportOnly)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error performing standards vacuum: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		reports = append(reports, report)

	case "all":
		if args.DaysOld <= 0 {
			args.DaysOld = 30
		}
		// Sessions
		mutated, err := rs.store.VacuumSessions(ctx, args.TargetOutcome, args.FlattenThreshold, args.DaysOld)
		if err != nil {
			slog.Warn("Session vacuum portion failed during full vacuum", "error", err)
		}
		sessionMutated = mutated

		// Memories
		memReport, err := rs.store.VacuumMemories(ctx, args.DaysOld, args.DedupThreshold, args.Category, args.ReportOnly)
		if err != nil {
			slog.Warn("Memory vacuum portion failed during full vacuum", "error", err)
		} else {
			reports = append(reports, memReport)
		}

		// Standards
		stdReport, err := rs.store.VacuumStandards(ctx, args.ReportOnly)
		if err != nil {
			slog.Warn("Standards vacuum portion failed during full vacuum", "error", err)
		} else {
			reports = append(reports, stdReport)
		}

	default:
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Invalid namespace: %q. Must be one of: sessions, memories, standards, all", args.Namespace)}},
			IsError: true,
		}, nil, nil
	}

	// Build structured result.
	result := map[string]any{
		"namespace": args.Namespace,
	}

	if args.Namespace == "sessions" || args.Namespace == "all" {
		result["sessions_pruned"] = sessionMutated
	}

	if len(reports) > 0 {
		var summaryParts []string
		for _, r := range reports {
			summaryParts = append(summaryParts, fmt.Sprintf("[%s] scanned=%d stale=%d duplicates=%d pruned=%d merged=%d",
				r.Namespace, r.TotalScanned, len(r.StaleEntries), len(r.DuplicateClusters), r.Pruned, r.Merged))
			result[r.Namespace+"_report"] = r
		}
		result["summary"] = strings.Join(summaryParts, " | ")
	} else if args.Namespace == "sessions" {
		result["summary"] = fmt.Sprintf("Context vacuum completed: %d '%s' records semantic-pruned and tombstoned (older than %d days). Defragmentation constraints evaluated against Threshold %d. ValueLog GC triggered.",
			sessionMutated, args.TargetOutcome, args.DaysOld, args.FlattenThreshold)
	}

	return &mcp.CallToolResult{
		StructuredContent: result,
	}, nil, nil
}

func (rs *MCPRecallServer) handleGetLogs(ctx context.Context, req *mcp.CallToolRequest, args GetLogsInput) (*mcp.CallToolResult, any, error) {

	if args.MaxLines <= 0 {
		args.MaxLines = config.DefaultLogLines
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: tailLines(rs.logs.String(), args.MaxLines)}},
	}, nil, nil
}

// tailLines returns the last n lines of s using a zero-allocation backward scan.
func tailLines(s string, n int) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) == 0 {
		return ""
	}
	count := 0
	i := len(s)
	for i > 0 {
		i--
		if s[i] == '\n' {
			count++
			if count == n {
				return s[i+1:]
			}
		}
	}
	return s
}

// handleConsolidate has been removed — dedup is now inline in remember/batch_remember.

func (rs *MCPRecallServer) handleRemember(ctx context.Context, req *mcp.CallToolRequest, args RememberInput) (*mcp.CallToolResult, any, error) {

	if len(args.Value) > 15000000 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: Payload value exceeds maximum length bounds (15MB limit) preventing Memory OOM."}},
			IsError: true,
		}, nil, nil
	}

	threshold := rs.cfg.DedupThreshold()
	if args.DedupThreshold > 0 {
		threshold = args.DedupThreshold
	}

	result, err := rs.store.Save(ctx, args.Title, args.Key, args.Value, args.Category, args.Tags, memory.DomainMemories, threshold)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	summary := fmt.Sprintf("Memory for '%s' %s.", result.Key, result.Action)
	data := map[string]any{
		"message":  summary,
		"action":   result.Action,
		"title":    args.Title,
		"key":      result.Key,
		"tags":     args.Tags,
		"category": args.Category,
	}
	if result.MergedKey != "" {
		data["merged_with"] = result.MergedKey
	}

	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data":    data,
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleSaveSessions(ctx context.Context, req *mcp.CallToolRequest, args SaveSessionsInput) (*mcp.CallToolResult, any, error) {

	if len(args.StateData) > 15000000 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: Session StateData exceeds maximum length bounds (15MB limit) preventing Memory OOM."}},
			IsError: true,
		}, nil, nil
	}

	tags := []string{"session"}
	if args.ProjectID != "" {
		tags = append(tags, fmt.Sprintf("project:%s", args.ProjectID))
	}
	if args.Outcome != "" {
		tags = append(tags, fmt.Sprintf("outcome:%s", args.Outcome))
	}
	if args.Model != "" {
		tags = append(tags, fmt.Sprintf("model:%s", args.Model))
	}
	if args.TraceContext != "" {
		tags = append(tags, fmt.Sprintf("trace:%s", args.TraceContext))
	}

	// UMFA / CSSA Transport Decoupling
	// Intercept the payload, explicitly tear off "report_fragment", dump to local artifacts, and delete from CSSA DB commit entirely.

	// We have a fragment. Slice it explicitly to disk.

	// Create standard normalized layout name: magictools_report_generated_generate_audit_report.md

	// Write fragment explicitly natively

	// Strip from badgerDB persistence map

	// Repackage metric JSON exclusively

	// Native 5-part matrix key for subset bound scanning
	key := fmt.Sprintf("%s:session:%s:%s:%s", args.ServerID, args.ProjectID, args.Outcome, args.SessionID)

	result, err := rs.store.Save(ctx, "Session State", key, args.StateData, args.ServerID, tags, memory.DomainSessions, 0.0)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	summary := fmt.Sprintf("Session trace state for '%s' saved persistently.", key)
	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]any{
				"key":        key,
				"server_id":  args.ServerID,
				"session_id": args.SessionID,
				"action":     result.Action,
			},
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleRecall(ctx context.Context, req *mcp.CallToolRequest, args RecallInput) (*mcp.CallToolResult, any, error) {

	rec, err := rs.store.Get(ctx, args.Key)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	// Domain isolation: recall is scoped to the memories namespace.
	if rec.Domain != memory.DomainMemories {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' belongs to the %s domain. Use 'get_sessions' or 'get_standards' instead.", args.Key, rec.Domain)}},
			IsError: true,
		}, nil, nil
	}

	summary := fmt.Sprintf("Retrieved memory: %s", args.Key)
	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data":    rec,
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleSearch(ctx context.Context, req *mcp.CallToolRequest, args SearchMemoriesInput) (*mcp.CallToolResult, any, error) {

	if args.Limit <= 0 {
		args.Limit = 20
	}

	start := time.Now()
	results, err := rs.store.Search(ctx, args.Query, args.Tag, args.Limit)
	elapsed := time.Since(start)

	// Update latency metrics
	rs.store.RecordSearchTelemetry(elapsed.Milliseconds())

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return rs.formatResults(fmt.Sprintf("Search Results for '%s'", args.Query), req, results)
}

func (rs *MCPRecallServer) handleSearchSessions(ctx context.Context, req *mcp.CallToolRequest, args SearchSessionsInput) (*mcp.CallToolResult, any, error) {
	if args.Limit <= 0 {
		args.Limit = 20
	}

	start := time.Now()
	results, err := rs.store.SearchSessions(ctx, args.Query, args.ProjectID, args.ServerID, args.Outcome, args.TraceContext, args.Limit)
	elapsed := time.Since(start)

	// Update latency metrics
	rs.store.RecordSearchTelemetry(elapsed.Milliseconds())

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return rs.formatResults(fmt.Sprintf("Session Search Results for '%s'", args.Query), req, results)
}

func (rs *MCPRecallServer) handleRecallRecent(ctx context.Context, req *mcp.CallToolRequest, args RecallRecentInput) (*mcp.CallToolResult, any, error) {

	if args.Count <= 0 {
		args.Count = 10
	}

	results, err := rs.store.GetRecent(ctx, args.Count)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return rs.formatResults("Recent Context Memories", req, results)
}

func (rs *MCPRecallServer) handleList(ctx context.Context, req *mcp.CallToolRequest, _ ListMemoriesInput) (*mcp.CallToolResult, any, error) {
	keys, err := rs.store.ListKeys(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return rs.formatResults("Knowledge Index", req, keys)
}

func (rs *MCPRecallServer) handleListSessions(ctx context.Context, req *mcp.CallToolRequest, args ListSessionsInput) (*mcp.CallToolResult, any, error) {

	if req.Params.Arguments != nil {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, nil, fmt.Errorf("invalid parameters: %w", err)
		}
	}

	// Default limit to 50 to prevent payload explosion.
	if args.Limit <= 0 {
		args.Limit = 50
	}

	// Dispatch structured query constraints to the DB layer instead of pulling purely domain objects
	sessions, err := rs.store.ListSessions(ctx, args.ProjectID, args.ServerID, args.Outcome, args.TraceContext)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	// Apply limit — keep the most recent entries (end of slice).
	if len(sessions) > args.Limit {
		sessions = sessions[len(sessions)-args.Limit:]
	}

	// Truncate content if requested to prevent payload overflow during aggregation.
	const maxContentLen = 32768 // 32KB
	if args.TruncateContent {
		for _, s := range sessions {
			if s.Record != nil && len(s.Record.Content) > maxContentLen {
				s.Record.Content = s.Record.Content[:maxContentLen]
				s.IsTruncated = true
			}
		}
	}

	return rs.formatResults("Analytic Session Dataset", req, sessions)
}

func (rs *MCPRecallServer) handleGetSessions(ctx context.Context, _ *mcp.CallToolRequest, args GetSessionsInput) (*mcp.CallToolResult, any, error) {

	// Direct key lookup takes precedence.
	if args.Key != "" {
		rec, err := rs.store.Get(ctx, args.Key)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Session not found: %v", err)}},
				IsError: true,
			}, nil, nil
		}
		if rec.Domain != memory.DomainSessions {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' is not a session record (domain: %s). Use 'recall' for memories.", args.Key, rec.Domain)}},
				IsError: true,
			}, nil, nil
		}
		return &mcp.CallToolResult{
			StructuredContent: map[string]any{
				"summary": fmt.Sprintf("Session '%s' retrieved.", args.Key),
				"data": map[string]any{
					"key":        args.Key,
					"server_id":  rec.Category,
					"content":    rec.Content,
					"tags":       rec.Tags,
					"created_at": rec.CreatedAt,
					"updated_at": rec.UpdatedAt,
				},
			},
		}, nil, nil
	}

	// Fallback: suffix-match scan using session_id across the sessions domain.
	if args.SessionID != "" {
		suffix := ":" + args.SessionID
		sessions, err := rs.store.ListSessions(ctx, "", "", "", "")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error scanning sessions: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		// Find the most recent session whose composite key ends with :session_id.
		var bestMatch *memory.SearchResult
		for _, s := range sessions {
			if strings.HasSuffix(s.Key, suffix) {
				if bestMatch == nil || s.Record.UpdatedAt.After(bestMatch.Record.UpdatedAt) {
					bestMatch = s
				}
			}
		}

		if bestMatch == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("No session found matching session_id '%s'.", args.SessionID)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			StructuredContent: map[string]any{
				"summary": fmt.Sprintf("Session '%s' retrieved via session_id match.", bestMatch.Key),
				"data": map[string]any{
					"key":        bestMatch.Key,
					"server_id":  bestMatch.Record.Category,
					"content":    bestMatch.Record.Content,
					"tags":       bestMatch.Record.Tags,
					"created_at": bestMatch.Record.CreatedAt,
					"updated_at": bestMatch.Record.UpdatedAt,
				},
			},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Either 'key' or 'session_id' must be provided."}},
		IsError: true,
	}, nil, nil
}

func (rs *MCPRecallServer) handleGetMetrics(ctx context.Context, _ *mcp.CallToolRequest, args GetMetricsInput) (*mcp.CallToolResult, any, error) {
	if os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true" {
		metrics := rs.store.GetMetrics()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "JSON-RPC Telemetry Payload yielded correctly."}},
			StructuredContent: map[string]any{
				"summary": "Shadow channel metric sync executed.",
				"data":    metrics,
			},
		}, nil, nil
	}

	// Database Entry Stats
	count, size, err := rs.store.GetStats()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error fetching DB stats: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	metrics := rs.store.GetMetrics()

	// Wait on SWR/BadgerDB Custom Counters
	cacheHits, cacheMisses, dbHits, dbMisses := rs.store.GetTelemetry()

	// Gather gopsutil metrics natively
	vMem, memErr := mem.VirtualMemory()
	if memErr != nil {
		slog.Warn("Failed to read virtual memory stats", "error", memErr)
	}
	cpuPct, cpuErr := cpu.Percent(0, false)
	if cpuErr != nil {
		slog.Warn("Failed to read CPU stats", "error", cpuErr)
	}
	hInfo, hostErr := host.Info()
	if hostErr != nil {
		slog.Warn("Failed to read host info", "error", hostErr)
	}

	cpuUsage := 0.0
	if len(cpuPct) > 0 {
		cpuUsage = cpuPct[0]
	}

	// Runtime and Application level
	appUptime := time.Since(rs.startTime)
	sysUptime := time.Duration(0)
	if hInfo != nil {
		sysUptime = time.Duration(hInfo.Uptime) * time.Second
	}

	avgLatency := float64(0)
	sCount := rs.searchCount.Load()
	if sCount > 0 {
		avgLatency = float64(rs.searchLatencyMS.Load()) / float64(sCount)
	}

	summary := fmt.Sprintf("System metrics retrieved. App Uptime: %s, CPU: %.2f%%, Mem Used: %.2f%%",
		appUptime.Round(time.Second), cpuUsage, vMem.UsedPercent)

	data := map[string]any{
		"system": map[string]any{
			"app_uptime":      appUptime.String(),
			"host_uptime_sec": sysUptime.Seconds(),
			"goroutines":      runtime.NumGoroutine(),
			"cpus_available":  runtime.NumCPU(),
			"cpu_usage_pct":   cpuUsage,
			"memory_total_mb": float64(vMem.Total) / 1024 / 1024,
			"memory_used_pct": vMem.UsedPercent,
			"memory_alloc_mb": float64(vMem.Used) / 1024 / 1024,
		},
		"storage": map[string]any{
			"db_entries":            count,
			"memories_count":        metrics.Memories,
			"sessions_count":        metrics.Sessions,
			"standards_count":       metrics.Standards,
			"projects_count":        metrics.Projects,
			"size_formatted":        fmt.Sprintf("%.2f KB", float64(size)/1024),
			"db_hits":               dbHits,
			"db_misses":             dbMisses,
			"cache_hits":            cacheHits,
			"cache_misses":          cacheMisses,
			"index_drift_alerts":    rs.store.DriftAlerts(),
			"avg_search_latency_ms": fmt.Sprintf("%.2fms", avgLatency),
		},
	}

	// Build readable text for proxy/Content consumers.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Recall Metrics\n\n%s\n\n", summary))
	sb.WriteString("## System\n")
	sb.WriteString(fmt.Sprintf("- App Uptime: %s\n", appUptime.Round(time.Second)))
	sb.WriteString(fmt.Sprintf("- Host Uptime: %.0fs\n", sysUptime.Seconds()))
	sb.WriteString(fmt.Sprintf("- Goroutines: %d\n", runtime.NumGoroutine()))
	sb.WriteString(fmt.Sprintf("- CPUs: %d\n", runtime.NumCPU()))
	sb.WriteString(fmt.Sprintf("- CPU Usage: %.2f%%\n", cpuUsage))
	sb.WriteString(fmt.Sprintf("- Memory Total: %.0f MB\n", float64(vMem.Total)/1024/1024))
	sb.WriteString(fmt.Sprintf("- Memory Used: %.2f%%\n", vMem.UsedPercent))
	sb.WriteString(fmt.Sprintf("- Memory Alloc: %.0f MB\n", float64(vMem.Used)/1024/1024))
	sb.WriteString("\n## Storage\n")
	sb.WriteString(fmt.Sprintf("- DB Entries: %d\n", count))
	sb.WriteString(fmt.Sprintf("- Memories: %d\n", metrics.Memories))
	sb.WriteString(fmt.Sprintf("- Sessions: %d\n", metrics.Sessions))
	sb.WriteString(fmt.Sprintf("- Standards: %d\n", metrics.Standards))
	sb.WriteString(fmt.Sprintf("- Projects: %d\n", metrics.Projects))
	sb.WriteString(fmt.Sprintf("- Size: %.2f KB\n", float64(size)/1024))
	sb.WriteString(fmt.Sprintf("- DB Hits: %d | Misses: %d\n", dbHits, dbMisses))
	sb.WriteString(fmt.Sprintf("- Cache Hits: %d | Misses: %d\n", cacheHits, cacheMisses))
	sb.WriteString(fmt.Sprintf("- Index Drift Alerts: %d\n", rs.store.DriftAlerts()))
	sb.WriteString(fmt.Sprintf("- Avg Search Latency: %.2fms\n", avgLatency))

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
		StructuredContent: map[string]any{
			"summary": summary,
			"data":    data,
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleForget(ctx context.Context, req *mcp.CallToolRequest, args ForgetInput) (*mcp.CallToolResult, any, error) {

	if err := rs.store.Delete(ctx, args.Key); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	summary := fmt.Sprintf("Memory for '%s' forgotten.", args.Key)
	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]any{
				"message": summary,
				"key":     args.Key,
			},
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleBatchRemember(ctx context.Context, req *mcp.CallToolRequest, args BatchRememberInput) (*mcp.CallToolResult, any, error) {

	if len(args.Entries) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: entries array is empty"}},
			IsError: true,
		}, nil, nil
	}

	// Validate each entry.
	for i, e := range args.Entries {
		if strings.TrimSpace(e.Key) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: entry[%d] has an empty key", i)}},
				IsError: true,
			}, nil, nil
		}
		if strings.TrimSpace(e.Value) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: entry[%d] (key=%q) has an empty value", i, e.Key)}},
				IsError: true,
			}, nil, nil
		}
		// 🛡️ Namespace enforcement: batch_remember is scoped exclusively to the memories domain.
		args.Entries[i].Domain = memory.DomainMemories
	}

	stored, batchErrors, err := rs.store.SaveBatch(ctx, args.Entries)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Batch save error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	summary := fmt.Sprintf("Batch save complete: %d stored, %d failed.", stored, len(batchErrors))
	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]any{
				"stored": stored,
				"failed": len(batchErrors),
				"errors": batchErrors,
			},
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleBatchRecall(ctx context.Context, req *mcp.CallToolRequest, args BatchRecallInput) (*mcp.CallToolResult, any, error) {

	if len(args.Keys) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: keys array is empty"}},
			IsError: true,
		}, nil, nil
	}

	found, missing, err := rs.store.GetBatch(ctx, args.Keys)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Batch recall error: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	// 🛡️ Namespace isolation: batch_recall only returns memories-domain records.
	for key, rec := range found {
		if rec.Domain != memory.DomainMemories {
			delete(found, key)
			missing = append(missing, key)
		}
	}

	summary := fmt.Sprintf("Batch recall complete: %d found, %d missing.", len(found), len(missing))
	return &mcp.CallToolResult{
		StructuredContent: map[string]any{
			"summary": summary,
			"data": map[string]any{
				"found":   len(found),
				"missing": missing,
				"entries": found,
			},
		},
	}, nil, nil
}

func (rs *MCPRecallServer) handleListCategories(ctx context.Context, req *mcp.CallToolRequest, _ ListCategoriesInput) (*mcp.CallToolResult, any, error) {

	categories, err := rs.store.ListCategories(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error fetching categories: %v", err)}},
			IsError: true,
		}, nil, nil
	}

	return rs.formatResults("Memory Categories", req, categories)
}

func (rs *MCPRecallServer) formatResults(title string, req *mcp.CallToolRequest, results any) (*mcp.CallToolResult, any, error) {
	var summary string

	var artifactPath string
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		var ap struct {
			ArtifactPath string `json:"artifact_path,omitempty"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &ap); err == nil {
			artifactPath = strings.TrimSpace(ap.ArtifactPath)
		}
	}

	// Try to determine size if it's a slice
	count := 0
	switch v := results.(type) {
	case []*memory.SearchResult:
		count = len(v)
	case []string:
		count = len(v)
	case []any:
	case map[string]int:
		count = len(v)
	}

	res := &mcp.CallToolResult{}

	if count == 0 {
		summary = fmt.Sprintf("%s: No results found.", title)
		res.StructuredContent = map[string]any{
			"summary": summary,
			"data": map[string]any{
				"message": "No matches found.",
			},
		}
	} else {
		summary = fmt.Sprintf("%s: Found %d entries.", title, count)
		res.StructuredContent = map[string]any{
			"summary": summary,
			"data": map[string]any{
				"title":   title,
				"count":   count,
				"entries": results,
			},
		}
	}

	if rawJSON, err := json.MarshalIndent(res.StructuredContent, "", "  "); err == nil {
		if artifactPath != "" {
			if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
				return nil, nil, fmt.Errorf("failed to create artifact directory: %w", err)
			}
			if err := os.WriteFile(artifactPath, rawJSON, 0o644); err != nil {
				return nil, nil, fmt.Errorf("failed to write artifact: %w", err)
			}
			res.Content = []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Artifact written to: %s", artifactPath)}}
			return res, nil, nil
		}
		res.Content = []mcp.Content{&mcp.TextContent{Text: string(rawJSON)}}
	}

	return res, nil, nil
}

// Serve initializes the transport and starts serving the MCP protocol.
func (rs *MCPRecallServer) Serve(ctx context.Context, stdout io.WriteCloser, reader io.ReadCloser) error {
	slog.Info("Serving MCP-Recall on pure IOTransport")
	t := &mcp.IOTransport{
		Reader: reader,
		Writer: stdout,
	}
	_, err := rs.mcpServer.Connect(ctx, t, nil)
	return err
}

func (rs *MCPRecallServer) handleExportMemories(ctx context.Context, req *mcp.CallToolRequest, args ExportMemoriesInput) (*mcp.CallToolResult, any, error) {

	fname := args.Filename
	if fname == "" {
		fname = fmt.Sprintf("recall_export_%s.jsonl", time.Now().Format("20060102_150405"))
	}
	fname = filepath.Base(fname)

	exportPath := filepath.Join(rs.cfg.ExportDir(), fname)

	count, err := rs.store.ExportJSONL(ctx, exportPath, "", nil)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to export: %v", err)}},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully exported %d records to %s", count, exportPath)}},
	}, nil, nil
}

func (rs *MCPRecallServer) handleImportMemories(ctx context.Context, req *mcp.CallToolRequest, args ImportMemoriesInput) (*mcp.CallToolResult, any, error) {

	if args.Filename == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: filename is strictly required for import."}},
		}, nil, nil
	}

	fname := filepath.Base(args.Filename)
	importPath := filepath.Join(rs.cfg.ExportDir(), fname)

	count, errList, err := rs.store.ImportJSONL(ctx, importPath, "")
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Catastrophic error during import: %v. %d records succeeded.", err, count)}},
		}, nil, nil
	}

	if len(errList) > 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Partially imported %d records but encountered %d errors (e.g. %v).", count, len(errList), errList[0])}},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully imported %d records from %s", count, importPath)}},
	}, nil, nil
}

func (rs *MCPRecallServer) handleReloadCache(ctx context.Context, _ *mcp.CallToolRequest, args ReloadCacheInput) (*mcp.CallToolResult, any, error) {
	if rs.store == nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: store not initialized"}},
		}, nil, nil
	}

	if err := rs.store.SyncSearchIndex(ctx); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to reload cache: %v", err)}},
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Search cache successfully re-synchronized with source of truth."}},
	}, nil, nil
}
