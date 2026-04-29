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
	"mcp-server-recall/internal/util"
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

// toolDef describes a single MCP tool registration.
type toolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Handler     func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// registerTools registers all MCP tools on the primary server.
func (rs *MCPRecallServer) registerTools() {
	for _, td := range rs.toolCatalog() {
		if td.Name == "harvest" {
			continue
		}
		rs.mcpServer.AddTool(&mcp.Tool{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: td.InputSchema,
		}, util.SafeToolHandler(td.Handler))
	}
	slog.Info("All hardened and featured tools registered")
}

// RegisterSafeTools assigns the safe, non-destructive queries and temporal scaffolding buckets to the SSE instance.
func (rs *MCPRecallServer) RegisterSafeTools(srv *mcp.Server) {
	safeConfig := rs.cfg.SafeTools()
	safeMap := make(map[string]bool, len(safeConfig))
	for _, name := range safeConfig {
		safeMap[name] = true
	}

	catalog := rs.toolCatalog()

	for _, td := range catalog {
		if !safeMap[td.Name] {
			continue
		}
		srv.AddTool(&mcp.Tool{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: td.InputSchema,
		}, util.SafeToolHandler(td.Handler))
	}
	slog.Info("All safe tools are now registered to secondary server endpoint", "count", len(safeConfig))
}

// toolCatalog returns the central registry of all MCP tools.
func (rs *MCPRecallServer) toolCatalog() []toolDef {
	var tools []toolDef
	tools = append(tools, rs.buildRememberTool(), rs.buildRecallTool(), rs.buildRecallRecentTool(), rs.buildGetMetricsTool(), rs.buildSaveSessionsTool())
	tools = append(tools, rs.adminTools()...)
	tools = append(tools, rs.batchTools()...)
	tools = append(tools, rs.consolidatedTools()...)

	return tools
}

func (rs *MCPRecallServer) buildRememberTool() toolDef {
	return toolDef{
		Name:        "remember",
		Description: "[DIRECTIVE: Memory Storage] Commits long-term agent context securely embedding semantic inline deduplication. Keywords: save, memorize, persistent, context, text, facts, history",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"title": { "type": "string", "description": "Optional explicit title for maximum Search Relevance bounding." },
				"key": { "type": "string", "description": "Unique identifier (e.g., 'auth-strategy')." },
				"value": { "type": "string", "description": "Content to store." },
				"tags": { "type": "array", "items": { "type": "string" }, "description": "Optional categories." },
				"category": { "type": "string", "description": "Primary classification." },
				"dedup_threshold": { "type": "number", "description": "Jaccard similarity threshold for inline dedup (0.0-1.0). Default from config. Set 0 to disable.", "default": 0.8 }
			},
			"required": ["key", "value"]
		}`),
		Handler: rs.handleRemember,
	}
}

func (rs *MCPRecallServer) buildSaveSessionsTool() toolDef {
	return toolDef{
		Name:        "save_sessions",
		Description: "[DIRECTIVE: Diagnostic Snapshot] Binds OpenTelemetry-compliant W3C tracing state to the ecosystem index securely. Keywords: session, trace, spans, states, active-context",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"server_id": { "type": "string", "description": "The ID of the server saving the session." },
				"project_id": { "type": "string", "description": "The absolute path or UUID representing the target project." },
				"outcome": { "type": "string", "description": "The resolution of the session (e.g. 'approved', 'rejected', 'pending')." },
				"session_id": { "type": "string", "description": "Unique session trace identifier (e.g. timestamp or UUID) to prevent overwrites." },
				"model": { "type": "string", "description": "Optional telemetry: model version utilized." },
				"token_spend": { "type": "integer", "description": "Optional telemetry: gross token utilization count." },
				"trace_context": { "type": "string", "description": "Optional telemetry: parent step ID or multi-agent chain correlation ID." },
				"state_data": { "type": "string", "description": "JSON string or text containing session state." }
			},
			"required": ["server_id", "project_id", "outcome", "session_id", "state_data"]
		}`),
		Handler: rs.handleSaveSessions,
	}
}

func (rs *MCPRecallServer) buildListSessionsTool() toolDef {
	return toolDef{
		Name:        "list_sessions",
		Description: "Returns a list of cross-server session records matching specified filters. [Domain: Sessions] Supports analytic aggregation across 1:N data arrays.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"project_id": { "type": "string", "description": "Filter by target project." },
				"server_id": { "type": "string", "description": "Filter by saving server." },
				"outcome": { "type": "string", "description": "Filter by resolution outcome." },
				"trace_context": { "type": "string", "description": "Filter by explicit analytical thread." },
				"limit": { "type": "integer", "description": "Maximum number of results to return. Defaults to 50 if not specified. Use to prevent payload overflow." },
				"truncate_content": { "type": "boolean", "description": "If true, truncates each record's content to 32KB. Use for aggregation queries where full content is not needed." }
			}
		}`),
		Handler: rs.handleListSessions,
	}
}

func (rs *MCPRecallServer) buildGetSessionsTool() toolDef {
	return toolDef{
		Name:        "get_sessions",
		Description: "Fetches a specific cross-server pipeline trace. Accepts either a composite key for direct lookup, or a session_id for suffix-match scanning. [Domain: Sessions]",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"key": { "type": "string", "description": "Direct composite key lookup (e.g. 'gorefactor:session:global:saved:12345'). Takes precedence over session_id if both are provided." },
				"session_id": { "type": "string", "description": "The session trace identifier. The handler will scan the sessions domain for any key whose final segment matches this value." }
			}
		}`),
		Handler: rs.handleGetSessions,
	}
}

func (rs *MCPRecallServer) buildRecallTool() toolDef {
	return toolDef{
		Name:        "recall",
		Description: "[DIRECTIVE: Memory Extraction] Extracts isolated unstructured conversational knowledge explicitly via unique key constraint. Keywords: fetch-memory, exact-key, retrieve-fact, discrete-pull",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": { "key": { "type": "string", "description": "Key of the memory to retrieve." } },
			"required": ["key"]
		}`),
		Handler: rs.handleRecall,
	}
}

func (rs *MCPRecallServer) buildSearchMemoriesTool() toolDef {
	return toolDef{
		Name:        "search_memories",
		Description: "Performs full-text, vector search, fuzzy key query, and database lookup across all stored memories. [Domain: Memories] Use this for knowledge exploration or when exact keys are unknown. Results are ranked by relevance.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": { "type": "string", "description": "Fuzzy keyword to look for." },
				"tag": { "type": "string", "description": "Optional filter for a specific tag." },
				"limit": { "type": "integer", "description": "Maximum number of results to return.", "default": 20 }
			}
		}`),
		Handler: rs.handleSearch,
	}
}

func (rs *MCPRecallServer) buildRecallRecentTool() toolDef {
	return toolDef{
		Name:        "recall_recent",
		Description: "[DIRECTIVE: Context Synchronization] Restores immediately preceding state data natively restoring IDE restarts. Keywords: recent, latest, restart-recovery, timeline",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": { "count": { "type": "integer", "description": "Number of recent memories to retrieve.", "default": 10 } }
		}`),
		Handler: rs.handleRecallRecent,
	}
}

func (rs *MCPRecallServer) buildListMemoriesTool() toolDef {
	return toolDef{
		Name:        "list_memories",
		Description: "Lists all stored knowledge keys and metadata summaries. [Domain: Memories] Use this for initial system orientation to discover available memory keys.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
		Handler: rs.handleList,
	}
}

func (rs *MCPRecallServer) buildGetMetricsTool() toolDef {
	return toolDef{
		Name:        "get_metrics",
		Description: "[DIRECTIVE: Hardware Diagnostics] Exports runtime footprint and storage telemetry limits. Keywords: ram, memory-size, footprint, cache-hits, system-stats",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {}
		}`),
		Handler: rs.handleGetMetrics,
	}
}

func (rs *MCPRecallServer) adminTools() []toolDef {
	return []toolDef{
		// 7. forget
		{
			Name:        "forget",
			Description: "[DIRECTIVE: Memory Eradication] Deletes obsolete conversational facts and unstructured knowledge snippets natively. Keywords: unlearn-memory, drop-fact, target-erase",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"key": {
						"type": "string",
						"description": "Key of the memory to delete."
					}
				},
				"required": ["key"]
			}`),
			Handler: rs.handleForget,
		},

		// 9. reload_cache
		{
			Name:        "reload_cache",
			Description: "[DIRECTIVE: Storage Synchronization] Forces memory backend alignment bridging database indices ensuring zero-drift globally. Keywords: sync, repair, force-rebuild",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
			Handler: rs.handleReloadCache,
		},
		// 10. (consolidate_memories removed — dedup is now inline in remember/batch_remember)
		// 11. get_internal_logs
		{
			Name:        "get_internal_logs",
			Description: "[DIRECTIVE: Audit Streaming] Streams live fault vectors and daemon stdout directly bypassing limits. Keywords: debug, errors, stack-trace, logs, fault",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"max_lines": {
						"type": "integer",
						"description": "Max log lines to return (default 25)."
					}
				}
			}`),
			Handler: rs.handleGetLogs,
		},

		{
			Name:        "context_vacuum",
			Description: "[DIRECTIVE: Database Maintenance] Executes garbage collection eliminating orphaned network nodes autonomously. Keywords: gc, prune, sweep, deduplication, dry-run",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"namespace": {
						"type": "string",
						"enum": ["sessions", "memories", "standards", "all"],
						"description": "Data domain to vacuum. Default: sessions.",
						"default": "sessions"
					},
					"target_outcome": {
						"type": "string",
						"description": "Sessions namespace: outcome signature to evacuate (e.g. 'rejected', 'abandoned'). Ignored for other namespaces."
					},
					"days_old": {
						"type": "integer",
						"description": "Age limit in days for entries to be flagged/pruned. Defaults to configured sessionpurgedays for sessions, 30 for memories."
					},
					"dedup_threshold": {
						"type": "number",
						"description": "Memories namespace: Jaccard similarity threshold (0.0-1.0) for near-duplicate detection. Default: 0.7.",
						"default": 0.7
					},
					"category": {
						"type": "string",
						"description": "Memories/Standards: scope vacuum to a specific category."
					},
					"flatten_threshold": {
						"type": "integer",
						"description": "Optional threshold of mutated keys before triggering an I/O expensive LSM defragmentation (Flatten). Default is 1000."
					},
					"report_only": {
						"type": "boolean",
						"description": "If true, return analysis without mutating data. Default: false.",
						"default": false
					}
				}
			}`),
			Handler: rs.handleContextVacuum,
		},
	}
}

func (rs *MCPRecallServer) batchTools() []toolDef {
	return []toolDef{
		// 13. batch_remember
		{
			Name:        "batch_remember",
			Description: "[DIRECTIVE: Mass Memory Storage] Queues multiple context chunks atomically maximizing bandwidth efficiently. Keywords: bulk-upload, batch-save, array-commit",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"entries": {
						"type": "array",
						"items": {
							"type": "object",
							"properties": {
								"title":    { "type": "string", "description": "Optional explicit title." },
								"key":      { "type": "string", "description": "Unique identifier for the entry." },
								"value":    { "type": "string", "description": "Content to store." },
								"tags":     { "type": "array", "items": { "type": "string" }, "description": "Optional tags." },
								"category": { "type": "string", "description": "Optional category." }
							},
							"required": ["key", "value"]
						},
						"description": "Array of entries to store (max 100).",
						"maxItems": 100
					}
				},
				"required": ["entries"]
			}`),
			Handler: rs.handleBatchRemember,
		},
		// 14. batch_recall
		{
			Name:        "batch_recall",
			Description: "[DIRECTIVE: Mass Memory Retrieval] Iterates an array of context items via unique ID map securely. Keywords: bulk-fetch, sync-array, mass-download",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"keys": {
						"type": "array",
						"items": { "type": "string" },
						"description": "Array of keys to retrieve (max 100).",
						"maxItems": 100
					}
				},
				"required": ["keys"]
			}`),
			Handler: rs.handleBatchRecall,
		},
		// 15. export_memories
		{
			Name:        "export_memories",
			Description: "[DIRECTIVE: Memory Backup] Dumps internal state strictly to explicit sandbox JSONL filesystem safely. Keywords: backup, disk-write, export",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filename": { "type": "string", "description": "Optional name. Defaults to generated timestamp." }
				}
			}`),
			Handler: rs.handleExportMemories,
		},
		// 16. import_memories
		{
			Name:        "import_memories",
			Description: "[DIRECTIVE: Mass Ingestion] Pushes physical disk JSONL arrays directly overriding active database mapping natively. Keywords: restore, payload, load-file",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filename": { "type": "string", "description": "Required filename to import from within the sandbox." }
				},
				"required": ["filename"]
			}`),
			Handler: rs.handleImportMemories,
		},
		{
			Name:        "ingest_files",
			Description: "[DIRECTIVE: Code Harvester] Recursively indexes live workspace source code into dynamic arrays buffering duplicates natively. Keywords: parse-directory, scan-project, bleve-mapping, load-code",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": { "type": "string", "description": "Absolute path to a single file or directory." }
				},
				"required": ["path"]
			}`),
			Handler: rs.handleIngestFiles,
		},
	}
}

func (rs *MCPRecallServer) handleContextVacuum(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Namespace        string  `json:"namespace"`
		TargetOutcome    string  `json:"target_outcome"`
		FlattenThreshold int     `json:"flatten_threshold"`
		DaysOld          int     `json:"days_old"`
		DedupThreshold   float64 `json:"dedup_threshold"`
		Category         string  `json:"category"`
		ReportOnly       bool    `json:"report_only"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}
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
			}, nil
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
			}, nil
		}
		reports = append(reports, report)

	case "standards":
		report, err := rs.store.VacuumStandards(ctx, args.ReportOnly)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error performing standards vacuum: %v", err)}},
				IsError: true,
			}, nil
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
		}, nil
	}

	// Build structured result.
	result := map[string]interface{}{
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
	}, nil
}

func (rs *MCPRecallServer) handleGetLogs(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		MaxLines int `json:"max_lines"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}
	if args.MaxLines <= 0 {
		args.MaxLines = config.DefaultLogLines
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: tailLines(rs.logs.String(), args.MaxLines)}},
	}, nil
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

func (rs *MCPRecallServer) handleRemember(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Title          string   `json:"title"`
		Key            string   `json:"key"`
		Value          string   `json:"value"`
		Category       string   `json:"category"`
		Tags           []string `json:"tags"`
		DedupThreshold *float64 `json:"dedup_threshold,omitempty"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if len(args.Value) > 15000000 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: Payload value exceeds maximum length bounds (15MB limit) preventing Memory OOM."}},
			IsError: true,
		}, nil
	}

	threshold := rs.cfg.DedupThreshold()
	if args.DedupThreshold != nil {
		threshold = *args.DedupThreshold
	}

	result, err := rs.store.Save(ctx, args.Title, args.Key, args.Value, args.Category, args.Tags, memory.DomainMemories, threshold)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Memory for '%s' %s.", result.Key, result.Action)
	data := map[string]interface{}{
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
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data":    data,
		},
	}, nil
}

func (rs *MCPRecallServer) handleSaveSessions(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ServerID     string `json:"server_id"`
		ProjectID    string `json:"project_id"`
		Outcome      string `json:"outcome"`
		SessionID    string `json:"session_id"`
		Model        string `json:"model,omitempty"`
		TokenSpend   int    `json:"token_spend,omitempty"`
		TraceContext string `json:"trace_context,omitempty"`
		StateData    string `json:"state_data"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if len(args.StateData) > 15000000 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: Session StateData exceeds maximum length bounds (15MB limit) preventing Memory OOM."}},
			IsError: true,
		}, nil
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
	var metadata map[string]any
	if err := json.Unmarshal([]byte(args.StateData), &metadata); err == nil {
		if fragmentVal, ok := metadata["report_fragment"]; ok {
			if fragStr, isStr := fragmentVal.(string); isStr && fragStr != "" {
				// We have a fragment. Slice it explicitly to disk.
				homeDir, _ := os.UserHomeDir()
				fragDir := filepath.Join(homeDir, ".gemini", "antigravity", "brain", args.SessionID, "fragments")
				_ = os.MkdirAll(fragDir, 0755)

				// Create standard normalized layout name: magictools_report_generated_generate_audit_report.md
				safeOutcome := strings.ReplaceAll(args.Outcome, "/", "_")
				safeTrace := strings.ReplaceAll(args.TraceContext, "/", "_")
				fragFile := filepath.Join(fragDir, fmt.Sprintf("%s_%s_%s.md", args.ServerID, safeOutcome, safeTrace))

				// Write fragment explicitly natively
				_ = os.WriteFile(fragFile, []byte(fragStr), 0644)

				// Strip from badgerDB persistence map
				delete(metadata, "report_fragment")

				// Repackage metric JSON exclusively
				if cleanedData, mErr := json.Marshal(metadata); mErr == nil {
					args.StateData = string(cleanedData)
				}
			}
		}
	}

	// Native 5-part matrix key for subset bound scanning
	key := fmt.Sprintf("%s:session:%s:%s:%s", args.ServerID, args.ProjectID, args.Outcome, args.SessionID)

	result, err := rs.store.Save(ctx, "Session State", key, args.StateData, args.ServerID, tags, memory.DomainSessions, 0.0)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Session trace state for '%s' saved persistently.", key)
	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"key":        key,
				"server_id":  args.ServerID,
				"session_id": args.SessionID,
				"action":     result.Action,
			},
		},
	}, nil
}

func (rs *MCPRecallServer) handleRecall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	rec, err := rs.store.Get(ctx, args.Key)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	// Domain isolation: recall is scoped to the memories namespace.
	if rec.Domain != memory.DomainMemories {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' belongs to the %s domain. Use 'get_sessions' or 'get_standards' instead.", args.Key, rec.Domain)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Retrieved memory: %s", args.Key)
	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data":    rec,
		},
	}, nil
}

func (rs *MCPRecallServer) handleSearch(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Query string `json:"query"`
		Tag   string `json:"tag"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Limit <= 0 {
		args.Limit = 20
	}

	start := time.Now()
	results, err := rs.store.Search(ctx, args.Query, args.Tag, args.Limit)
	elapsed := time.Since(start)

	// Update latency metrics
	rs.searchLatencyMS.Add(elapsed.Milliseconds())
	rs.searchCount.Add(1)

	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults(fmt.Sprintf("Search Results for '%s'", args.Query), req, results)
}

func (rs *MCPRecallServer) handleRecallRecent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if args.Count <= 0 {
		args.Count = 10
	}

	results, err := rs.store.GetRecent(ctx, args.Count)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults("Recent Context Memories", req, results)
}

func (rs *MCPRecallServer) handleList(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	keys, err := rs.store.ListKeys(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults("Knowledge Index", req, keys)
}

func (rs *MCPRecallServer) handleListSessions(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		ProjectID       string `json:"project_id,omitempty"`
		ServerID        string `json:"server_id,omitempty"`
		Outcome         string `json:"outcome,omitempty"`
		TraceContext    string `json:"trace_context,omitempty"`
		Limit           int    `json:"limit,omitempty"`
		TruncateContent bool   `json:"truncate_content,omitempty"`
	}
	if req.Params.Arguments != nil {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid parameters: %w", err)
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
		}, nil
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

func (rs *MCPRecallServer) handleGetSessions(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key       string `json:"key,omitempty"`
		SessionID string `json:"session_id,omitempty"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	// Direct key lookup takes precedence.
	if args.Key != "" {
		rec, err := rs.store.Get(ctx, args.Key)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Session not found: %v", err)}},
				IsError: true,
			}, nil
		}
		if rec.Domain != memory.DomainSessions {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Key '%s' is not a session record (domain: %s). Use 'recall' for memories.", args.Key, rec.Domain)}},
				IsError: true,
			}, nil
		}
		return &mcp.CallToolResult{
			StructuredContent: map[string]interface{}{
				"summary": fmt.Sprintf("Session '%s' retrieved.", args.Key),
				"data": map[string]interface{}{
					"key":        args.Key,
					"server_id":  rec.Category,
					"content":    rec.Content,
					"tags":       rec.Tags,
					"created_at": rec.CreatedAt,
					"updated_at": rec.UpdatedAt,
				},
			},
		}, nil
	}

	// Fallback: suffix-match scan using session_id across the sessions domain.
	if args.SessionID != "" {
		suffix := ":" + args.SessionID
		sessions, err := rs.store.ListSessions(ctx, "", "", "", "")
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error scanning sessions: %v", err)}},
				IsError: true,
			}, nil
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
			}, nil
		}

		return &mcp.CallToolResult{
			StructuredContent: map[string]interface{}{
				"summary": fmt.Sprintf("Session '%s' retrieved via session_id match.", bestMatch.Key),
				"data": map[string]interface{}{
					"key":        bestMatch.Key,
					"server_id":  bestMatch.Record.Category,
					"content":    bestMatch.Record.Content,
					"tags":       bestMatch.Record.Tags,
					"created_at": bestMatch.Record.CreatedAt,
					"updated_at": bestMatch.Record.UpdatedAt,
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Either 'key' or 'session_id' must be provided."}},
		IsError: true,
	}, nil
}

func (rs *MCPRecallServer) handleGetMetrics(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if os.Getenv("MCP_ORCHESTRATOR_OWNED") == "true" {
		metrics := rs.store.GetMetrics()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "JSON-RPC Telemetry Payload yielded correctly."}},
			StructuredContent: map[string]interface{}{
				"summary": "Shadow channel metric sync executed.",
				"data":    metrics,
			},
		}, nil
	}

	// Database Entry Stats
	count, size, err := rs.store.GetStats()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error fetching DB stats: %v", err)}},
			IsError: true,
		}, nil
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

	data := map[string]interface{}{
		"system": map[string]interface{}{
			"app_uptime":      appUptime.String(),
			"host_uptime_sec": sysUptime.Seconds(),
			"goroutines":      runtime.NumGoroutine(),
			"cpus_available":  runtime.NumCPU(),
			"cpu_usage_pct":   cpuUsage,
			"memory_total_mb": float64(vMem.Total) / 1024 / 1024,
			"memory_used_pct": vMem.UsedPercent,
			"memory_alloc_mb": float64(vMem.Used) / 1024 / 1024,
		},
		"storage": map[string]interface{}{
			"db_entries":            count,
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
	sb.WriteString(fmt.Sprintf("- Standards: %d\n", metrics.Standards))
	sb.WriteString(fmt.Sprintf("- Projects: %d\n", metrics.Projects))
	sb.WriteString(fmt.Sprintf("- Size: %.2f KB\n", float64(size)/1024))
	sb.WriteString(fmt.Sprintf("- DB Hits: %d | Misses: %d\n", dbHits, dbMisses))
	sb.WriteString(fmt.Sprintf("- Cache Hits: %d | Misses: %d\n", cacheHits, cacheMisses))
	sb.WriteString(fmt.Sprintf("- Index Drift Alerts: %d\n", rs.store.DriftAlerts()))
	sb.WriteString(fmt.Sprintf("- Avg Search Latency: %.2fms\n", avgLatency))

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: sb.String()}},
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data":    data,
		},
	}, nil
}

func (rs *MCPRecallServer) handleForget(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if err := rs.store.Delete(ctx, args.Key); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Memory for '%s' forgotten.", args.Key)
	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"message": summary,
				"key":     args.Key,
			},
		},
	}, nil
}

func (rs *MCPRecallServer) handleBatchRemember(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Entries []memory.BatchEntry `json:"entries"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if len(args.Entries) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: entries array is empty"}},
			IsError: true,
		}, nil
	}

	// Validate each entry.
	for i, e := range args.Entries {
		if strings.TrimSpace(e.Key) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: entry[%d] has an empty key", i)}},
				IsError: true,
			}, nil
		}
		if strings.TrimSpace(e.Value) == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error: entry[%d] (key=%q) has an empty value", i, e.Key)}},
				IsError: true,
			}, nil
		}
	}

	stored, batchErrors, err := rs.store.SaveBatch(ctx, args.Entries)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Batch save error: %v", err)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Batch save complete: %d stored, %d failed.", stored, len(batchErrors))
	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"stored": stored,
				"failed": len(batchErrors),
				"errors": batchErrors,
			},
		},
	}, nil
}

func (rs *MCPRecallServer) handleBatchRecall(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid parameters: %w", err)
	}

	if len(args.Keys) == 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: keys array is empty"}},
			IsError: true,
		}, nil
	}

	found, missing, err := rs.store.GetBatch(ctx, args.Keys)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Batch recall error: %v", err)}},
			IsError: true,
		}, nil
	}

	summary := fmt.Sprintf("Batch recall complete: %d found, %d missing.", len(found), len(missing))
	return &mcp.CallToolResult{
		StructuredContent: map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"found":   len(found),
				"missing": missing,
				"entries": found,
			},
		},
	}, nil
}

func (rs *MCPRecallServer) handleListCategories(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	categories, err := rs.store.ListCategories(ctx)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Error fetching categories: %v", err)}},
			IsError: true,
		}, nil
	}

	return rs.formatResults("Memory Categories", req, categories)
}

func (rs *MCPRecallServer) formatResults(title string, req *mcp.CallToolRequest, results any) (*mcp.CallToolResult, error) {
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
	case []interface{}:
	case map[string]int:
		count = len(v)
	}

	res := &mcp.CallToolResult{}

	if count == 0 {
		summary = fmt.Sprintf("%s: No results found.", title)
		res.StructuredContent = map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"message": "No matches found.",
			},
		}
	} else {
		summary = fmt.Sprintf("%s: Found %d entries.", title, count)
		res.StructuredContent = map[string]interface{}{
			"summary": summary,
			"data": map[string]interface{}{
				"title":   title,
				"count":   count,
				"entries": results,
			},
		}
	}

	if rawJSON, err := json.MarshalIndent(res.StructuredContent, "", "  "); err == nil {
		if artifactPath != "" {
			if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
				return nil, fmt.Errorf("failed to create artifact directory: %w", err)
			}
			if err := os.WriteFile(artifactPath, rawJSON, 0o644); err != nil {
				return nil, fmt.Errorf("failed to write artifact: %w", err)
			}
			res.Content = []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Artifact written to: %s", artifactPath)}}
			return res, nil
		}
		res.Content = []mcp.Content{&mcp.TextContent{Text: string(rawJSON)}}
	}

	return res, nil
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

func (rs *MCPRecallServer) handleExportMemories(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid json arguments: %w", err)
	}

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
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully exported %d records to %s", count, exportPath)}},
	}, nil
}

func (rs *MCPRecallServer) handleImportMemories(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args struct {
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("invalid json arguments: %w", err)
	}

	if args.Filename == "" {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: filename is strictly required for import."}},
		}, nil
	}

	fname := filepath.Base(args.Filename)
	importPath := filepath.Join(rs.cfg.ExportDir(), fname)

	count, errList, err := rs.store.ImportJSONL(ctx, importPath, "")
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Catastrophic error during import: %v. %d records succeeded.", err, count)}},
		}, nil
	}

	if len(errList) > 0 {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Partially imported %d records but encountered %d errors (e.g. %v).", count, len(errList), errList[0])}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Successfully imported %d records from %s", count, importPath)}},
	}, nil
}

func (rs *MCPRecallServer) handleReloadCache(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if rs.store == nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: "Error: store not initialized"}},
		}, nil
	}

	if err := rs.store.SyncSearchIndex(ctx); err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Failed to reload cache: %v", err)}},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Search cache successfully re-synchronized with source of truth."}},
	}, nil
}
