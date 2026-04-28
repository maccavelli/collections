package external

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient wraps the official go-sdk Streamable HTTP transport for connecting
// to external MCP servers (e.g., mcp-server-recall). It replaces the former
// custom SSE client with the standards-compliant transport from the MCP spec
// (2025-03-26), while preserving the RecallEnabled circuit breaker for
// graceful standalone-mode degradation.
type MCPClient struct {
	URL             string
	mu              sync.Mutex
	session         *mcp.ClientSession
	recallEnabled   bool
	logger          *slog.Logger
	telemetryShards [8]chan telemetryEvent
}

type telemetryEvent struct {
	sessionID string
	projectID string
	payload   interface{}
}

// NewMCPClient initializes a new Streamable HTTP MCP client.
func NewMCPClient(url string) *MCPClient {
	client := &MCPClient{
		URL:    url,
		logger: slog.Default(),
	}
	for i := 0; i < 8; i++ {
		client.telemetryShards[i] = make(chan telemetryEvent, 2048)
		go client.workerFlusher(i)
	}
	return client
}

// RecallEnabled safely returns true if the connection to Recall is fully established.
func (c *MCPClient) RecallEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.recallEnabled
}

func (c *MCPClient) setRecallEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recallEnabled = enabled
}

// Start connects to the remote MCP server using Streamable HTTP transport.
// It runs initialization (handshake + capability discovery) and sets the
// circuit breaker to enabled on success. Tracks A & C natively remove boot
// starvation and blocking telemetry pipelines.
func (c *MCPClient) Start(ctx context.Context) {
	// 🛡️ RECONNECTION LOOP: Ensure client reconnects if recall crashes
	for {
		if ctx.Err() != nil {
			return
		}

		connected := false
		backoff := 100 * time.Millisecond
		maxBackoff := 5 * time.Second

		// Rapid circuit breaker without arbitrary loop ceilings
		for {
			if ctx.Err() != nil {
				return
			}

			client := mcp.NewClient(
				&mcp.Implementation{
					Name:    "mcp-server-go-refactor",
					Version: "1.0.0",
				},
				nil,
			)

			transport := &mcp.StreamableClientTransport{
				Endpoint: c.URL,
			}

			session, err := client.Connect(ctx, transport, nil)
			if err != nil {
				c.logger.Error("[RECALL] connection attempt failed",
					"server", "go-refactor",
					"url", c.URL,
					"error", err,
					"retry_in", backoff,
				)
				c.setRecallEnabled(false)

				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff = backoff * 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
				continue
			}

			c.mu.Lock()
			c.session = session
			c.mu.Unlock()

			c.setRecallEnabled(true)
			connected = true
			c.logger.Info("Streamable HTTP connection established", "url", c.URL, "session_id", session.ID())

			// Setup channel to wait on session closure
			errCh := make(chan error, 1)
			go func() {
				errCh <- session.Wait()
			}()

			// Block until connection closes or context is cancelled
			select {
			case <-ctx.Done():
				c.logger.Info("Context cancelled, closing Streamable HTTP session", "url", c.URL)
				session.Close()
				c.setRecallEnabled(false)
				return
			case err := <-errCh:
				if err != nil {
					c.logger.Warn("Streamable HTTP session closed unexpectedly (crash detected)",
						"server", "go-refactor", "url", c.URL, "error", err)
				}
				c.setRecallEnabled(false)
			}
			break // break attempt loop to trigger reconnect
		}

		if !connected {
			c.setRecallEnabled(false)
			c.logger.Warn("Recall connection dropped, attempting to reconnect in 5s...")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// CallDatabaseTool generically executes remote database tools and marshals their
// structured data results. Returns empty string if recall is unavailable.
func (c *MCPClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]interface{}) string {
	if !c.RecallEnabled() {
		c.logger.Warn("recall_unreachable: circuit breaker is open, recall connection not established",
			"tool", toolName, "reason", "circuit_breaker_open")
		return ""
	}

	c.mu.Lock()
	session := c.session
	c.mu.Unlock()

	if session == nil {
		c.logger.Warn("recall_unreachable: session handle is nil despite circuit breaker being closed",
			"tool", toolName, "reason", "nil_session")
		return ""
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		c.logger.Warn("recall_unreachable: RPC call to recall failed",
			"tool", toolName, "error", err, "reason", "rpc_error")
		return ""
	}

	// Prefer structured content for machine-to-machine consumption (e.g., recall)
	if result.StructuredContent != nil {
		if sc, ok := result.StructuredContent.(map[string]interface{}); ok {
			if data, exists := sc["data"]; exists {
				j, _ := json.Marshal(data)
				return string(j)
			}
			j, _ := json.Marshal(sc)
			return string(j)
		}
	}

	// Fallback: extract text content if structured content is missing or invalid
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok && tc.Text != "" {
			c.logger.Info("API command text content fallback executed", "target", "recall", "tool", toolName)
			return tc.Text
		}
	}

	c.logger.Warn("recall_key_not_found: recall responded but returned no data for the requested key",
		"tool", toolName, "arguments", arguments, "reason", "empty_response")
	return ""
}

// SaveSession persists context state into the remote recall server.
// projectID is the project-scoped key (e.g., project root path) used for recall's compound key.
func (c *MCPClient) SaveSession(ctx context.Context, sessionID, projectID string, payload interface{}) error {
	shardIdx := 0
	if len(sessionID) > 0 {
		shardIdx = int(sessionID[0]) % 8
	}

	select {
	case c.telemetryShards[shardIdx] <- telemetryEvent{sessionID: sessionID, projectID: projectID, payload: payload}:
		return nil
	default:
		c.logger.Warn("Telemetry queue full, shedding payload", "session_id", sessionID, "project_id", projectID, "shard", shardIdx)
		return fmt.Errorf("telemetry queue full")
	}
}

func (c *MCPClient) workerFlusher(shard int) {
	// Worker runs infinitely until the process dies.
	// The native context from CallDatabaseTool is ephemeral per interaction.
	bgCtx := context.Background()
	for event := range c.telemetryShards[shard] {
		var payloadBytes []byte
		if event.payload != nil {
			payloadBytes, _ = json.Marshal(event.payload)
		}
		if len(payloadBytes) == 0 || string(payloadBytes) == "null" {
			payloadBytes = []byte("{}")
		}
		sID := event.sessionID
		if sID == "" {
			sID = fmt.Sprintf("fallback-%d", time.Now().UnixNano())
		}
		pID := event.projectID
		if pID == "" {
			pID = "global"
		}
		args := map[string]interface{}{
			"server_id":     "go-refactor",
			"project_id":    pID,
			"outcome":       "saved",
			"session_id":    sID,
			"model":         "native",
			"token_spend":   0,
			"trace_context": "async_push",
			"state_data":    string(payloadBytes),
		}

		retries := 15
		backoff := 100 * time.Millisecond

		for i := 0; i < retries; i++ {
			res := c.CallDatabaseTool(bgCtx, "save_sessions", args)
			if res != "" {
				break
			}
			if i == retries-1 {
				c.logger.Warn("telemetry_push_failed: recall save_sessions returned empty repeatedly, dropping payload",
					"server_id", "go-refactor",
					"session_id", event.sessionID,
					"shard", shard,
					"attempts", retries,
				)
			}
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 2*time.Second {
				backoff = 2 * time.Second
			}
		}
	}
}

// GetSession retrieves context state from the remote recall server.
// The sessionID corresponds to the CSSA pipeline correlation ID (typically the project root).
func (c *MCPClient) GetSession(ctx context.Context, sessionID string) (map[string]interface{}, error) {
	args := map[string]interface{}{
		"session_id": sessionID,
	}
	args["namespace"] = "sessions"
	res := c.CallDatabaseTool(ctx, "get", args)
	if res == "" {
		if !c.RecallEnabled() {
			return nil, fmt.Errorf("recall_unreachable: recall circuit breaker is open, cannot retrieve session '%s'", sessionID)
		}
		return nil, fmt.Errorf("recall_key_not_found: no session data found for key '%s' — verify the session_id matches an existing recall entry", sessionID)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(res), &data); err != nil {
		return nil, fmt.Errorf("recall_data_corrupt: session '%s' returned unparseable data: %w", sessionID, err)
	}
	return data, nil
}

// AggregateSessionFromRecall retrieves ALL session records for a given server_id
// and project_id from recall's list_sessions endpoint, then merges their state_data
// into a unified map. This is the correct aggregation path for generate_final_report
// which needs to combine data from all pipeline stages.
//
// Limits results to the 20 most recent entries and truncates per-stage content
// at 32KB to prevent orchestrator payload overflow (25MB limit).
func (c *MCPClient) AggregateSessionFromRecall(ctx context.Context, serverID, projectID string) (map[string]interface{}, error) {
	const maxEntries = 20       // Most recent N entries per server_id (brainstorm=9, go-refactor=15)
	const maxContentLen = 32768 // 32KB per stage content cap

	if !c.RecallEnabled() {
		return nil, fmt.Errorf("recall_unreachable: recall circuit breaker is open, cannot aggregate sessions")
	}

	args := map[string]interface{}{
		"server_id":        serverID,
		"project_id":       projectID,
		"limit":            maxEntries,
		"truncate_content": true,
	}
	args["namespace"] = "sessions"
	res := c.CallDatabaseTool(ctx, "list", args)
	if res == "" {
		return nil, fmt.Errorf("recall_key_not_found: no sessions found for server_id='%s' project_id='%s'", serverID, projectID)
	}

	// Parse the list_sessions response.
	// CallDatabaseTool may return either:
	//   (a) Unwrapped: {"count":N, "entries":[...]} — when StructuredContent has "data" key
	//   (b) Wrapped:   {"data":{"count":N, "entries":[...]}} — when TextContent fallback is used
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(res), &envelope); err != nil {
		return nil, fmt.Errorf("recall_data_corrupt: list_sessions returned unparseable data: %w", err)
	}

	// Handle both unwrapped (entries at top level) and wrapped (entries inside "data").
	var entries []interface{}
	if e, ok := envelope["entries"].([]interface{}); ok {
		// Case (a): CallDatabaseTool already unwrapped the "data" field
		entries = e
	} else if data, ok := envelope["data"].(map[string]interface{}); ok {
		// Case (b): full response envelope with "data" wrapper
		entries, _ = data["entries"].([]interface{})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("recall_key_not_found: no session entries for server_id='%s' project_id='%s'", serverID, projectID)
	}

	// Limit to most recent N entries (recall returns chronologically ordered).
	totalEntries := len(entries)
	if totalEntries > maxEntries {
		entries = entries[totalEntries-maxEntries:]
	}

	// Merge all stage state_data into a unified result map.
	merged := make(map[string]interface{})
	merged["_total_entries"] = totalEntries
	merged["_stage_count"] = len(entries)

	stages := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		e, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := e["key"].(string)
		record, _ := e["record"].(map[string]interface{})
		if record == nil {
			continue
		}
		content, _ := record["content"].(string)
		if content == "" {
			continue
		}

		// Parse the state_data JSON stored in the content field.
		var stageData map[string]interface{}
		if json.Unmarshal([]byte(content), &stageData) == nil {
			if len(content) > maxContentLen {
				delete(stageData, "files")
				delete(stageData, "diff")
				delete(stageData, "stdout")
				delete(stageData, "stderr")
				stageData["_pruned"] = true
			}
			stages = append(stages, stageData)
			// Merge top-level fields from each stage into the unified map.
			for k, v := range stageData {
				merged[k] = v
			}
		}

		c.logger.Debug("AggregateSessionFromRecall: merged stage", "key", key)
	}

	merged["_stages"] = stages
	c.logger.Info("AggregateSessionFromRecall success",
		"server_id", serverID,
		"project_id", projectID,
		"total_avail", totalEntries,
		"stages_merged", len(stages),
	)

	return merged, nil
}

// ---------------------------------------------------------------------------
// Standards Namespace Convenience API
// ---------------------------------------------------------------------------

// SearchStandards queries the recall standards namespace with multi-dimensional
// BM25 search. Filters are optional — pass empty string to skip.
// Returns empty string if recall is unavailable or no matches found.
func (c *MCPClient) SearchStandards(ctx context.Context, query, pkg, symbolType string, limit int) string {
	args := map[string]interface{}{"query": query}
	if limit > 0 {
		args["limit"] = limit
	}
	if pkg != "" {
		args["package"] = pkg
	}
	if symbolType != "" {
		args["symbol_type"] = symbolType
	}
	return c.CallDatabaseTool(ctx, "search_ecosystem", args)
}

// ---------------------------------------------------------------------------
// Sessions Namespace Convenience API
// ---------------------------------------------------------------------------

// ListSessionsByFilter retrieves sessions matching specified criteria with
// truncation enabled to prevent payload overflow. All filters are optional.
func (c *MCPClient) ListSessionsByFilter(ctx context.Context, projectID, serverID, outcome string, limit int) string {
	args := map[string]interface{}{"truncate_content": true}
	if limit > 0 {
		args["limit"] = limit
	}
	if projectID != "" {
		args["project_id"] = projectID
	}
	if serverID != "" {
		args["server_id"] = serverID
	}
	if outcome != "" {
		args["outcome"] = outcome
	}
	args["namespace"] = "sessions"
	return c.CallDatabaseTool(ctx, "list", args)
}
