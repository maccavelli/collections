package server

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-server-recall/internal/util"
)

func add[In any](
	_ *MCPRecallServer,
	srv *mcp.Server,
	safeMap map[string]bool,
	name, desc string,
	handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error),
) {
	if safeMap != nil && !safeMap[name] {
		return
	}
	util.HardenedAddTool(srv, &mcp.Tool{Name: name, Description: desc}, handler)
}

func (rs *MCPRecallServer) registerAll(srv *mcp.Server, safeMap map[string]bool) {
	add(rs, srv, safeMap, "remember", "[DIRECTIVE: Memory Storage] Commits long-term agent context securely embedding semantic inline deduplication. Keywords: save, memorize, persistent, context, text, facts, history", rs.handleRemember)
	add(rs, srv, safeMap, "recall", "[DIRECTIVE: Memory Extraction] Extracts isolated unstructured conversational knowledge explicitly via unique key constraint. Keywords: fetch-memory, exact-key, retrieve-fact, discrete-pull", rs.handleRecall)
	add(rs, srv, safeMap, "recall_recent", "[DIRECTIVE: Context Synchronization] Restores immediately preceding state data natively restoring IDE restarts. Keywords: recent, latest, restart-recovery, timeline", rs.handleRecallRecent)
	add(rs, srv, safeMap, "get_metrics", "[DIRECTIVE: Hardware Diagnostics] Exports runtime footprint and storage telemetry limits. Keywords: ram, memory-size, footprint, cache-hits, system-stats", rs.handleGetMetrics)
	add(rs, srv, safeMap, "save_sessions", "[DIRECTIVE: Diagnostic Snapshot] Binds OpenTelemetry-compliant W3C tracing state to the ecosystem index securely. Keywords: session, trace, spans, states, active-context", rs.handleSaveSessions)
	add(rs, srv, safeMap, "forget", "[DIRECTIVE: Memory Eradication] Deletes obsolete conversational facts and unstructured knowledge snippets natively. Keywords: unlearn-memory, drop-fact, target-erase", rs.handleForget)
	add(rs, srv, safeMap, "reload_cache", "[DIRECTIVE: Storage Synchronization] Forces memory backend alignment bridging database indices ensuring zero-drift globally. Keywords: sync, repair, force-rebuild", rs.handleReloadCache)
	add(rs, srv, safeMap, "get_internal_logs", "[DIRECTIVE: Audit Streaming] Streams live fault vectors and daemon stdout directly bypassing limits. Keywords: debug, errors, stack-trace, logs, fault", rs.handleGetLogs)
	add(rs, srv, safeMap, "prune_records", "[DIRECTIVE: Database Maintenance] Executes garbage collection eliminating orphaned network nodes autonomously. Keywords: gc, prune, sweep, deduplication, dry-run", rs.handleContextVacuum)
	add(rs, srv, safeMap, "batch_remember", "[DIRECTIVE: Mass Memory Storage] Queues multiple context chunks atomically maximizing bandwidth efficiently. Keywords: bulk-upload, batch-save, array-commit", rs.handleBatchRemember)
	add(rs, srv, safeMap, "batch_recall", "[DIRECTIVE: Mass Memory Retrieval] Iterates an array of context items via unique ID map securely. Keywords: bulk-fetch, sync-array, mass-download", rs.handleBatchRecall)
	add(rs, srv, safeMap, "export_records", "[DIRECTIVE: Memory Backup] Dumps internal state strictly to explicit sandbox JSONL filesystem safely. Keywords: backup, disk-write, export", rs.handleExportMemories)
	add(rs, srv, safeMap, "import_records", "[DIRECTIVE: Mass Ingestion] Pushes physical disk JSONL arrays directly overriding active database mapping natively. Keywords: restore, payload, load-file", rs.handleImportMemories)
	add(rs, srv, safeMap, "ingest_files", "[DIRECTIVE: Code Harvester] Recursively indexes live workspace source code into dynamic arrays buffering duplicates natively. Keywords: parse-directory, scan-project, bleve-mapping, load-code", rs.handleIngestFiles)

	if safeMap == nil || safeMap["search"] {
		add(rs, srv, safeMap, "search", "[DIRECTIVE: Universal Discovery] Evaluates unstructured persistent memory context, architectural standards, and project code ASTs via hybrid arrays. Keywords: query, fuzzy, explore, find, codebase, RAG, history, vector", rs.handleUniversalSearch)
	}
	if safeMap == nil || safeMap["list"] {
		add(rs, srv, safeMap, "list", "[DIRECTIVE: Universal Enumeration] Generates structural hierarchical overviews mapping all packages, roots, memory keys, and sessions securely. Keywords: inventory, topology, root-folders, map-keys", rs.handleUniversalList)
	}
	if safeMap == nil || safeMap["get"] {
		add(rs, srv, safeMap, "get", "[DIRECTIVE: Universal Retrieval] Bypasses all specific domain boundaries fetching raw underlying literal paths or standard documents verbatim. Keywords: raw-text, exact-uri, absolute-pull, structural-download", rs.handleUniversalGet)
	}
	if safeMap != nil && safeMap["harvest"] {
		add(rs, srv, safeMap, "harvest", "Canonical AST parsing traversal engine. CLI Restricted operation to parse, scan, index, ingest, and structurally map physical file system directories, Go code projects, and active standards into the internal Bleve knowledge backend. DO NOT INVOKE AS AN AGENT.", rs.handleUniversalHarvest)
	}
	if safeMap == nil || safeMap["delete"] {
		add(rs, srv, safeMap, "delete", "[DIRECTIVE: Universal Eradication] Destroys absolute string indices wiping constraints and root definitions globally dropping safety checks native. Keywords: wipe-explicit, override-delete, target-destroy", rs.handleUniversalDelete)
	}
}

// registerTools registers all MCP tools on the primary server.
func (rs *MCPRecallServer) registerTools() {
	rs.registerAll(rs.mcpServer, nil)
	slog.Info("All hardened and featured tools registered")
}

// RegisterSafeTools assigns the safe, non-destructive queries and temporal scaffolding buckets to the SSE instance.
func (rs *MCPRecallServer) RegisterSafeTools(srv *mcp.Server) {
	safeConfig := rs.cfg.SafeTools()
	safeMap := make(map[string]bool, len(safeConfig))
	for _, name := range safeConfig {
		safeMap[name] = true
	}
	rs.registerAll(srv, safeMap)
	slog.Info("All safe tools are now registered to secondary server endpoint", "count", len(safeConfig))
}

// RegisterSafeToolsInternal assigns the full administrative tool suite to the internal CLI SSE instance.
func (rs *MCPRecallServer) RegisterSafeToolsInternal(srv *mcp.Server) {
	safeConfig := rs.cfg.SafeToolsInternal()
	safeMap := make(map[string]bool, len(safeConfig))
	for _, name := range safeConfig {
		safeMap[name] = true
	}
	rs.registerAll(srv, safeMap)
	slog.Info("All internal safe tools are now registered to local server endpoint", "count", len(safeConfig))
}
