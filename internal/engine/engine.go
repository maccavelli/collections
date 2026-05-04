// Package engine provides the central shared state for the
// go-refactor MCP server, including connections to external
// services such as the Recall knowledge graph.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mcp-server-go-refactor/internal/external"
	"mcp-server-go-refactor/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tidwall/buntdb"
)

// SyncWrapper creates a JSON shell for data caching guaranteeing soft invalidation.
type SyncWrapper struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Data      string    `json:"data"`
}

// Session holds workflow state for an end-to-end refactoring
// tool flow, keyed by project root.
type Session struct {
	ProjectRoot string            `json:"project_root"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	Artifacts   map[string]string `json:"artifacts,omitempty"`
}

// Engine holds shared state accessible to all handler packages.
type Engine struct {
	mu             sync.RWMutex
	ExternalClient *external.MCPClient
	mcpSession     *mcp.ServerSession
	sessions       map[string]*Session
	DB             *buntdb.DB
}

// NewEngine creates a new Engine instance.
func NewEngine(db *buntdb.DB) *Engine {
	return &Engine{
		sessions: make(map[string]*Session),
		DB:       db,
	}
}

// DBEntries returns the count of persistent entries in BuntDB.
func (e *Engine) DBEntries() int {
	if e == nil || e.DB == nil {
		return 0
	}
	var n int
	_ = e.DB.View(func(tx *buntdb.Tx) error {
		n, _ = tx.Len()
		return nil
	})
	return n
}

// SetExternalClient injects the MCPClient for cross-server API tools (e.g. Recall).
func (e *Engine) SetExternalClient(c *external.MCPClient) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ExternalClient = c
}

// SetSession performs the SetSession operation.
func (e *Engine) SetSession(s *mcp.ServerSession) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mcpSession = s
}

// Session performs the Session operation.
func (e *Engine) Session() *mcp.ServerSession {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.mcpSession
}

// search helper removed; EnsureRecallCache uses CallDatabaseTool directly

// LoadSession retrieves the workflow session for a project root.
// If no session exists, a fresh one is created in-memory.
func (e *Engine) LoadSession(_ context.Context, projectRoot string) *Session {
	e.mu.RLock()
	if s, ok := e.sessions[projectRoot]; ok {
		e.mu.RUnlock()
		return s
	}
	e.mu.RUnlock()

	s := &Session{
		ProjectRoot: projectRoot,
		Status:      "IDLE",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    make(map[string]any),
	}

	e.mu.Lock()
	e.sessions[projectRoot] = s
	e.mu.Unlock()

	return s
}

// SaveSession stores the workflow session in-memory and publishes to recall
// for cross-server consumption if the external client is available.
func (e *Engine) SaveSession(session *Session) {
	session.UpdatedAt = time.Now()
	e.mu.Lock()
	e.sessions[session.ProjectRoot] = session
	e.mu.Unlock()

	// Mirror brainstorm's CSSA pattern: publish every session save to recall
	// so cross-server report aggregation can find the data.
	if session.ProjectRoot != "" && len(session.Metadata) > 0 {
		outcome := strings.ToLower(session.Status)
		if outcome == "" {
			outcome = "analyzed"
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			e.PublishSessionToRecall(ctx, "", session.ProjectRoot, outcome, "native", "auto_publish", "", session.Metadata)
		}()
	}
}

// EnsureRecallCache coordinates lazy-fetching of role-specific data from Recall,
// ensuring only one inflight request per session/role.
func (e *Engine) EnsureRecallCache(ctx context.Context, session *Session, role string, toolName string, arguments map[string]any) string {
	cacheKey := "recall_cache_" + role
	syncKey := "recall_sync_" + role

	var staleVal string
	var returnFresh bool

	if e.DB != nil {
		if err := e.DB.View(func(tx *buntdb.Tx) error {
			val, err := tx.Get(cacheKey)
			if err == nil && val != "" {
				var wrapper SyncWrapper
				if decodeErr := json.Unmarshal([]byte(val), &wrapper); decodeErr == nil {
					staleVal = wrapper.Data
					if time.Since(wrapper.UpdatedAt) < 1*time.Hour {
						returnFresh = true
					}
				} else {
					staleVal = val     // Legacy string fallback
					returnFresh = true // Treat legacy existing cache as 'needs revalidating? no, just stale so it falls through to update.
					returnFresh = false
				}
			}
			return err
		}); err != nil {
			slog.Debug("BuntDB view bypassed or errored", "role", role, "err", err)
		}
		if returnFresh {
			slog.Debug("Recall BuntDB cache hit (fresh)", "role", role, "size", len(staleVal))
			return staleVal
		}
	}

	e.mu.Lock()
	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}

	if pending, ok := session.Metadata[syncKey].(*sync.WaitGroup); ok {
		e.mu.Unlock()
		pending.Wait()

		if e.DB != nil {
			var postWaitVal string
			_ = e.DB.View(func(tx *buntdb.Tx) error {
				val, err := tx.Get(cacheKey)
				if err == nil && val != "" {
					var wrapper SyncWrapper
					if err := json.Unmarshal([]byte(val), &wrapper); err == nil {
						postWaitVal = wrapper.Data
					} else {
						postWaitVal = val
					}
				}
				return nil
			})
			if postWaitVal != "" {
				slog.Debug("Recall BuntDB cache hit (after wait)", "role", role, "size", len(postWaitVal))
				return postWaitVal
			}
		}
		return staleVal // Fallback gracefully if Wait returned nothing new
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	session.Metadata[syncKey] = wg
	e.mu.Unlock()

	e.mu.RLock()
	client := e.ExternalClient
	e.mu.RUnlock()

	var localStandards string
	if cacheDir, err := os.UserCacheDir(); err == nil {
		localPath := filepath.Join(cacheDir, "go-refactor", "standards", role+".md")
		if b, err := os.ReadFile(localPath); err == nil {
			localStandards = string(b)
		}
	}

	var globalRecall string
	if client != nil && client.RecallEnabled() {
		globalRecall = client.CallDatabaseTool(ctx, toolName, arguments)
	}

	hardcodedFallback := DefaultStandards[role]

	var val string
	if localStandards != "" || globalRecall != "" || hardcodedFallback != "" {
		val = "<STANDARDS_HIERARCHY>\n"

		isOrchestrator := globalRecall != ""

		if isOrchestrator {
			if globalRecall != "" {
				val += "  <PRIORITY_1_GLOBAL_RECALL>\n" + globalRecall + "\n  </PRIORITY_1_GLOBAL_RECALL>\n"
			}
			if localStandards != "" {
				val += "  <PRIORITY_2_FILESYSTEM_STANDARDS>\n" + localStandards + "\n  </PRIORITY_2_FILESYSTEM_STANDARDS>\n"
			}
			if hardcodedFallback != "" {
				val += "  <PRIORITY_3_HARDCODED_HEURISTICS>\n" + hardcodedFallback + "\n  </PRIORITY_3_HARDCODED_HEURISTICS>\n"
			}
			val += "  <INSTRUCTION>\n"
			val += "    Apply highest priority rules first. Global recall constraints > Local filesystem standard overrides > hard-coded engine defaults.\n"
			val += "  </INSTRUCTION>\n"
		} else {
			if localStandards != "" {
				val += "  <PRIORITY_1_FILESYSTEM_STANDARDS>\n" + localStandards + "\n  </PRIORITY_1_FILESYSTEM_STANDARDS>\n"
			}
			if hardcodedFallback != "" {
				val += "  <PRIORITY_2_HARDCODED_HEURISTICS>\n" + hardcodedFallback + "\n  </PRIORITY_2_HARDCODED_HEURISTICS>\n"
			}
			val += "  <INSTRUCTION>\n"
			val += "    Apply highest priority rules first. Local filesystem standard overrides > hard-coded engine defaults.\n"
			val += "  </INSTRUCTION>\n"
		}
		val += "</STANDARDS_HIERARCHY>"
	}

	if val != "" {
		slog.Debug("Recall cache miss (fetched from remote)", "role", role, "size", len(val))
		if e.DB != nil {
			wrapper := SyncWrapper{
				UpdatedAt: time.Now(),
				Data:      val,
			}
			if b, err := json.Marshal(wrapper); err == nil {
				_ = e.DB.Update(func(tx *buntdb.Tx) error {
					_, _, err := tx.Set(cacheKey, string(b), nil) // Infinite TTL Edge Caching
					return err
				})
			}
		}
	} else if staleVal != "" {
		slog.Warn("mcp server dropped standard sync - deploying standalone using stale DB context", "role", role)
		val = staleVal
	}

	e.mu.Lock()
	delete(session.Metadata, syncKey)
	e.mu.Unlock()

	wg.Done()
	return val
}

// CSSATelemetry defines the strict data transfer object for recall cross-server bounds
type CSSATelemetry struct {
	ReportFragment string `json:"report_fragment,omitempty"`
	TraceData      any    `json:"trace_data,omitempty"`
}

// PublishSessionToRecall pushes the current session state to recall for cross-server consumption.
// Submits analytical trace data enabling W3C telemetry processing globally.
// The session_id is a raw nonce; recall's save_sessions handler constructs the
// full compound key: {server_id}:session:{project_id}:{outcome}:{session_id}.
func (e *Engine) PublishSessionToRecall(ctx context.Context, sessionID, projectID, outcome, model, traceContext, reportFragment string, data any) {
	e.mu.RLock()
	client := e.ExternalClient
	e.mu.RUnlock()

	if client == nil || !client.RecallEnabled() {
		slog.Debug("PublishSessionToRecall skipped: recall not available")
		return
	}

	telemetryDto := CSSATelemetry{
		ReportFragment: reportFragment,
		TraceData:      data,
	}

	payload, err := json.Marshal(telemetryDto)
	if err != nil {
		slog.Warn("PublishSessionToRecall marshal error", "project", projectID, "err", err)
		return
	}

	// Pass raw nonce as session_id — recall's save_sessions builds the compound key.
	nonce := sessionID
	if nonce == "" {
		nonce = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	args := map[string]any{
		"server_id":     "go-refactor",
		"project_id":    projectID,
		"outcome":       outcome,
		"session_id":    nonce,
		"model":         model,
		"token_spend":   0, // Hook point for future LLM engine extension
		"trace_context": traceContext,
		"state_data":    string(payload),
	}

	result := client.CallDatabaseTool(ctx, "save_sessions", args)
	if result != "" {
		slog.Info("PublishSessionToRecall success", "project", projectID, "outcome", outcome, "nonce", nonce, "size", len(payload))
	}
}

// LoadCrossSessionFromRecall queries recall for historic session data published by a peer server (e.g., brainstorm).
// It retrieves the entire trace dataset for the project, enabling 1:N analytical pattern discovery.
func (e *Engine) LoadCrossSessionFromRecall(ctx context.Context, peerServer, projectID string) string {
	e.mu.RLock()
	client := e.ExternalClient
	e.mu.RUnlock()

	if client == nil || !client.RecallEnabled() {
		slog.Debug("LoadCrossSessionFromRecall skipped: recall not available")
		return ""
	}

	result := client.CallDatabaseTool(ctx, "list", map[string]any{"namespace": "sessions",
		"server_id":        peerServer,
		"project_id":       projectID,
		"limit":            1,
		"truncate_content": true,
	})
	if result != "" && result != "No records found matching the specified parameters." && !strings.Contains(result, "Error") {
		slog.Info("LoadCrossSessionFromRecall hit", "peer", peerServer, "project", projectID, "size", len(result))
		return result
	}
	return ""
}

// WrapOutput standardizes payload responses for orchestration, injecting HFSC AST Hash and offline status.
func (e *Engine) WrapOutput(target, sessionID string, data map[string]any) models.UniversalPipelineOutput {
	// Import dynamic check here to avoid circular dependencies if config wraps engine
	isOrch := strings.TrimSpace(sessionID) != ""

	return models.UniversalPipelineOutput{
		ASTHash:           ComputeASTHash(target),
		TelemetryDisabled: !isOrch,
		Data:              data,
	}
}

// ComputeASTHash calculates a deterministic SHA-256 footprint of the AST boundary (target workspace logic).
func ComputeASTHash(target string) string {
	var hashBuilder strings.Builder
	_ = filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if b, err := os.ReadFile(path); err == nil {
			hashBuilder.Write(b)
		}
		return nil
	})
	if hashBuilder.Len() == 0 {
		return "empty_ast"
	}
	// We do inline sha256 to avoid import modification failures
	h := sha256.Sum256([]byte(hashBuilder.String()))
	return fmt.Sprintf("%x", h)
}
