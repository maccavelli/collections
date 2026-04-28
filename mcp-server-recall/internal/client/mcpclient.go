package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient wraps the official go-sdk Streamable HTTP transport for connecting
// to the running mcp-server-recall instance. Used by the CLI harvest command
// to call harvest_standards on the live server, ensuring proper write-lock
// coordination at runtime without stopping the server.
type MCPClient struct {
	URL           string
	mu            sync.Mutex
	session       *mcp.ClientSession
	recallEnabled bool
	logger        *slog.Logger
}

// NewMCPClient initializes a new Streamable HTTP MCP client.
func NewMCPClient(url string) *MCPClient {
	return &MCPClient{
		URL:    url,
		logger: slog.Default(),
	}
}

// RecallEnabled safely returns true if the connection is fully established.
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

// Start connects to the running recall server using Streamable HTTP transport.
// SDK's Connect() handles the handshake synchronously — no startup delay needed.
func (c *MCPClient) Start(ctx context.Context) {
	client := mcp.NewClient(
		&mcp.Implementation{
			Name:    "mcp-server-recall-cli",
			Version: "1.0.0",
		},
		nil,
	)

	transport := &mcp.StreamableClientTransport{
		Endpoint: c.URL,
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		c.logger.Error("Failed to connect via Streamable HTTP", "url", c.URL, "err", err)
		c.setRecallEnabled(false)
		return
	}

	c.mu.Lock()
	c.session = session
	c.mu.Unlock()

	c.setRecallEnabled(true)
	c.logger.Info("Streamable HTTP connection established", "url", c.URL)

	// Block until connection closes, then disable the circuit breaker.
	if err := session.Wait(); err != nil {
		c.logger.Warn("Streamable HTTP session closed", "url", c.URL, "err", err)
	}
	c.setRecallEnabled(false)
}

// CallDatabaseTool executes a remote tool on the recall server and returns
// the structured or text result. Returns a descriptive error for every failure
// path so callers can distinguish server crashes from parsing issues.
// No timeout — harvest_standards can run indefinitely until context cancelled.
func (c *MCPClient) CallDatabaseTool(ctx context.Context, toolName string, arguments map[string]interface{}) (string, error) {
	if !c.RecallEnabled() {
		return "", fmt.Errorf("recall unavailable: circuit breaker active")
	}

	c.mu.Lock()
	session := c.session
	c.mu.Unlock()

	if session == nil {
		return "", fmt.Errorf("recall unavailable: session not initialized")
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		return "", fmt.Errorf("recall RPC failed: %w", err)
	}

	if result == nil {
		return "", fmt.Errorf("recall returned nil result")
	}

	// Surface server-side tool errors (IsError=true).
	if result.IsError {
		return "", fmt.Errorf("recall tool error: %s", extractErrorText(result))
	}

	// Priority: StructuredContent first (harvest_standards, metrics, etc.)
	if result.StructuredContent != nil {
		if sc, ok := result.StructuredContent.(map[string]interface{}); ok {
			if data, exists := sc["data"]; exists {
				j, jErr := json.Marshal(data)
				if jErr != nil {
					return "", fmt.Errorf("recall structured data marshal failed: %w", jErr)
				}
				c.logger.Info("API command executed", "target", "recall", "tool", toolName)
				return string(j), nil
			}
			j, jErr := json.Marshal(sc)
			if jErr != nil {
				return "", fmt.Errorf("recall structured content marshal failed: %w", jErr)
			}
			c.logger.Info("API command executed", "target", "recall", "tool", toolName)
			return string(j), nil
		}
	}

	// Fallback: extract text content if present.
	for _, content := range result.Content {
		if tc, ok := content.(*mcp.TextContent); ok && tc.Text != "" {
			c.logger.Info("API command executed", "target", "recall", "tool", toolName)
			return tc.Text, nil
		}
	}

	return "", fmt.Errorf("recall returned no parseable content (Content=%d, StructuredContent=%v)", len(result.Content), result.StructuredContent != nil)
}

// extractErrorText pulls the first non-empty text from an IsError result's Content slice.
func extractErrorText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			return tc.Text
		}
	}
	return "unknown error"
}
