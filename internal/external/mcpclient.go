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
// to external MCP servers (e.g., mcp-server-recall). It replicates the
// Brainstorm reference architecture to maintain ecosystem consistency.
type MCPClient struct {
	URL             string
	mu              sync.Mutex
	session         *mcp.ClientSession
	recallEnabled   bool
	workersStarted  bool
	logger          *slog.Logger
	telemetryShards [8]chan telemetryEvent
}

type telemetryEvent struct {
	sessionID string
	projectID string
	payload   any
}

// NewMCPClient initializes a new Streamable HTTP MCP client.
func NewMCPClient(url string) *MCPClient {
	client := &MCPClient{
		URL:    url,
		logger: slog.Default(),
	}
	for i := range 8 {
		client.telemetryShards[i] = make(chan telemetryEvent, 2048)
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
// Starvation and blocking telemetry pipelines.
func (c *MCPClient) Start(ctx context.Context) {
	c.mu.Lock()
	if !c.workersStarted {
		c.workersStarted = true
		for i := range 8 {
			go c.workerFlusher(ctx, i)
		}
	}
	c.mu.Unlock()

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
					Name:    "mcp-server-magicskills",
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
					"server", "magicskills",
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
				backoff = min(backoff*2, maxBackoff)
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
						"server", "magicskills", "url", c.URL, "error", err)
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

// CallDatabaseTool generically executes remote database tools on the Recall server.
func (c *MCPClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]any) string {
	if !c.RecallEnabled() {
		return ""
	}

	c.mu.Lock()
	session := c.session
	c.mu.Unlock()

	if session == nil {
		return ""
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		c.logger.Warn("Recall RPC failed", "tool", toolName, "error", err)
		return ""
	}

	if result.StructuredContent != nil {
		if sc, ok := result.StructuredContent.(map[string]any); ok {
			if data, exists := sc["data"]; exists {
				j, _ := json.Marshal(data)
				return string(j)
			}
			j, _ := json.Marshal(sc)
			return string(j)
		}
	}

	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok && tc.Text != "" {
			return tc.Text
		}
	}

	return ""
}

// SaveSession persists context state into the remote recall server.
func (c *MCPClient) SaveSession(ctx context.Context, sessionID, projectID string, payload any) error {
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

func (c *MCPClient) workerFlusher(ctx context.Context, shard int) {
	// Worker runs strictly bound to the remote execution Context.
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-c.telemetryShards[shard]:
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
			args := map[string]any{
				"server_id":     "magicskills",
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

			for i := range retries {
				res := c.CallDatabaseTool(ctx, "save_sessions", args)
				if res != "" {
					break
				}
				if i == retries-1 {
					c.logger.Warn("telemetry_push_failed: recall save_sessions returned empty repeatedly, dropping payload",
						"server_id", "magicskills",
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
		} // end select
	}
}

// ---------------------------------------------------------------------------
// Sessions Namespace Convenience API
// ---------------------------------------------------------------------------

// SearchSessions queries the recall sessions namespace with multi-dimensional
// BM25/Jaccard search. Filters are optional — pass empty string to skip.
// Returns empty string if recall is unavailable or no matches found.
func (c *MCPClient) SearchSessions(ctx context.Context, query, projectID, serverID, outcome, traceContext string, limit int) string {
	args := map[string]any{"query": query}
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
	if traceContext != "" {
		args["trace_context"] = traceContext
	}
	args["namespace"] = "sessions"
	return c.CallDatabaseTool(ctx, "search", args)
}

