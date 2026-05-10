package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mcp-server-magictools/internal/db"
	"mcp-server-magictools/internal/telemetry"
	"mcp-server-magictools/internal/util"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// ProxyService handles URN resolution, lazy activation, proxy execution,
// and response minification. Extracted from registerProxyGateway to reduce
// cognitive complexity.
type ProxyService struct {
	Handler *OrchestratorHandler
}

// NewProxyService creates a ProxyService backed by the given handler.
func NewProxyService(h *OrchestratorHandler) *ProxyService {
	return &ProxyService{Handler: h}
}

// AutoCoerceArguments rapidly injects missing properties with zero-values or defaults natively
// utilizing the pre-computed ZeroValues profile directly from the cached ToolRecord.
// This executes in O(1) latency cleanly offloading formatting burdens from the LLM agent.
func (ps *ProxyService) AutoCoerceArguments(record *db.ToolRecord, args map[string]any) {
	if record == nil {
		return
	}

	// 1. ZeroValues injection for completely missing fields
	if record.ZeroValues != nil {
		for key, zeroVal := range record.ZeroValues {
			if _, exists := args[key]; !exists {
				args[key] = zeroVal
			}
		}
	}

	// 2. Dynamic Type Coercion based on JSON schema types
	if record.InputSchema != nil {
		if props, ok := record.InputSchema["properties"].(map[string]any); ok {
			for key, val := range args {
				if propDef, exists := props[key].(map[string]any); exists {
					if typ, hasTyp := propDef["type"].(string); hasTyp {
						switch typ {
						case "integer":
							switch v := val.(type) {
							case float64:
								args[key] = int(v)
							case string:
								var i int
								if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
									args[key] = i
								}
							}
						case "number":
							if str, ok := val.(string); ok {
								var f float64
								if _, err := fmt.Sscanf(str, "%f", &f); err == nil {
									args[key] = f
								}
							}
						case "string":
							switch v := val.(type) {
							case float64, int, bool:
								args[key] = fmt.Sprintf("%v", v)
							case string:
								// 🛡️ ENUM BOUNDS SNAPPING: Automatically "snap" minor hallucinations
								if enum, ok := propDef["enum"].([]any); ok && len(enum) > 0 {
									snapped := ps.snapToEnum(v, enum)
									if snapped != v {
										slog.Info("gateway: enum snapped", "field", key, "original", v, "snapped", snapped)
										args[key] = snapped
									}
								}
							}
						case "array":
							if val == nil {
								args[key] = []any{}
							}
						}
					}
				}
			}
		}
	}
}

// ValidateArguments asserts schema constraints on incoming payloads natively.
// When record is non-nil, it skips the redundant GetTool DB lookup.
// Compiled schemas are cached by SchemaHash for reuse across calls seamlessly.
func (ps *ProxyService) ValidateArguments(ctx context.Context, urn string, record *db.ToolRecord, args map[string]any) error {
	if record == nil {
		var err error
		record, err = ps.Handler.Store.GetTool(urn)
		if err != nil {
			return nil
		}
	}
	hash := record.SchemaHash
	if hash == "" {
		return nil
	}

	// 🛡️ PERF: Check compiled schema cache first natively
	if cached, ok := ps.Handler.schemaCache.Load(hash); ok {
		sch := cached.(*jsonschema.Schema)
		if err := sch.Validate(args); err != nil {
			// AutoCoerce recovery path using memory-safe ZeroValues bypass natively
			ps.AutoCoerceArguments(record, args)
			if errRetry := sch.Validate(args); errRetry == nil {
				slog.Info("gateway: auto-coerced missing schema properties natively from Record Profile", "urn", urn)
				return nil
			}
			return ps.formatValidationError(err, hash)
		}
		return nil
	}

	// Cache miss: fetch, compile, and store organically
	schema, err := ps.Handler.Store.GetSchema(hash)
	if err != nil || len(schema) == 0 {
		return nil
	}
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		slog.Warn("gateway: failed to marshal schema for compilation locally", "error", err)
		return nil
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", bytes.NewReader(schemaBytes)); err != nil {
		return nil
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return nil
	}
	ps.Handler.schemaCache.Store(hash, sch)

	if err := sch.Validate(args); err != nil {
		// Native O(1) auto-coerce fallback bypassing reflection
		ps.AutoCoerceArguments(record, args)
		if errRetry := sch.Validate(args); errRetry == nil {
			slog.Info("gateway: auto-coerced missing schema properties natively on cache miss", "urn", urn)
			return nil
		}
		return ps.formatValidationError(err, hash)
	}
	return nil
}

func (ps *ProxyService) formatValidationError(baseErr error, hash string) error {
	schemaMap, err := ps.Handler.Store.GetSchema(hash)
	if err != nil || len(schemaMap) == 0 {
		return fmt.Errorf("[VALIDATION_ERROR]: Arguments do not match schema constraints: %v", baseErr)
	}
	schemaBytes, _ := json.MarshalIndent(schemaMap, "", "  ")
	return fmt.Errorf("[VALIDATION_ERROR]: Arguments do not match schema constraints: %v\n\nExpected Schema:\n```json\n%s\n```", baseErr, string(schemaBytes))
}

// ResolveURN parses and validates a tool URN, performing auto-resolution
// if the initial URN doesn't match a known tool.
// Returns the canonical server name, tool name, resolved URN, and the ToolRecord (if found).
func (ps *ProxyService) ResolveURN(ctx context.Context, inputURN string) (server, tool, resolvedURN string, record *db.ToolRecord, err error) {
	urn := rawURN(inputURN)
	parts := strings.Split(urn, ":")
	if len(parts) < 2 {
		return "", "", "", nil, fmt.Errorf("invalid URN")
	}

	if len(parts) == 3 {
		server, tool = parts[0], parts[2]
		urn = fmt.Sprintf("%s:%s", server, tool)
	} else {
		server, tool = parts[0], parts[1]
	}

	// Internal tools don't need DB validation
	if server == "magictools" {
		return server, tool, urn, nil, nil
	}

	// Pre-call validation: ensure the tool exists before attempting proxy
	record, getErr := ps.Handler.Store.GetTool(urn)
	if getErr != nil {
		// Auto-resolution: search explicitly by name globally
		suggestions, searchErr := ps.Handler.Store.SearchTools(ctx, tool, "", "", 0.0, ps.Handler.Config.ScoreFusionAlpha, db.DomainSystem)
		if searchErr != nil && searchErr.Error() != "No certain match" {
			slog.Log(ctx, util.LevelTrace, "gateway: auto-resolve search logic partial error", "error", searchErr)
		}
		if len(suggestions) > 0 {
			for _, sug := range suggestions {
				if strings.EqualFold(sug.Name, tool) {
					slog.Info("gateway: auto-resolved tool URN mismatch", "original", inputURN, "resolved", sug.URN)
					return sug.Server, tool, sug.URN, sug, nil
				}
			}
			// Attempt server-specific suggestion
			serverSpecific, serverErr := ps.Handler.Store.SearchTools(ctx, tool, "", server, 0.0, ps.Handler.Config.ScoreFusionAlpha, db.DomainSystem)
			if serverErr == nil && len(serverSpecific) > 0 {
				return "", "", "", nil, fmt.Errorf("tool URN %q not found. Did you mean %q?", inputURN, serverSpecific[0].URN)
			}
			return "", "", "", nil, fmt.Errorf("tool URN %q not found. Did you mean %q?", inputURN, suggestions[0].URN)
		}
		return "", "", "", nil, fmt.Errorf("tool URN %q not found. Call align_tools to discover available capabilities.", inputURN)
	}

	return server, tool, urn, record, nil
}

// EnsureServerReady performs lazy activation of a sub-server if it's not currently running.
func (ps *ProxyService) EnsureServerReady(ctx context.Context, server string) (bootLatencyMs int64) {
	startBoot := time.Now()
	if _, ok := ps.Handler.Registry.GetServerSession(server); !ok {
		for _, sc := range ps.Handler.Config.GetManagedServers() {
			if sc.Name == server {
				if err := ps.Handler.Registry.Connect(ctx, sc.Name, sc.Command, sc.Args, sc.Env, sc.Hash()); err != nil {
					slog.Error("gateway: lazy activation failed", "server", sc.Name, "error", err)
					ps.Handler.Telemetry.RecordFault(server)
				}
				break
			}
		}
	}
	return time.Since(startBoot).Milliseconds()
}

// ExecuteProxy calls a tool on a sub-server via the registry proxy.
func (ps *ProxyService) ExecuteProxy(ctx context.Context, server, tool string, arguments map[string]any, timeout time.Duration) (*mcp.CallToolResult, error) {
	res, err := ps.Handler.Registry.CallProxy(ctx, server, tool, arguments, timeout)
	if err != nil {
		slog.Log(ctx, util.LevelTrace, "gateway: call_proxy failure", "server", server, "tool", tool, "error", err)
		return nil, err
	}
	slog.Log(ctx, util.LevelTrace, "gateway: call_proxy success", "server", server, "tool", tool)
	return res, nil
}

// MinifyResponse applies the data hardening pipeline to a proxy result:
// squeeze nulls, truncate large strings, transform to hybrid markdown,
// and attach the raw-data retrieval footer.
func (ps *ProxyService) MinifyResponse(ctx context.Context, res *mcp.CallToolResult, server, tool string, sentSize int64, truncateLimit int) *mcp.CallToolResult {
	if res.IsError || res.StructuredContent == nil {
		return res
	}

	// A. Cache the RAW full version before any reduction
	callID := fmt.Sprintf("%s-full-%d", tool, time.Now().UnixNano())
	rawBytes, rawErr := json.Marshal(res.StructuredContent)
	if rawErr != nil {
		slog.Warn("gateway: failed to serialize raw response caching payload", "error", rawErr)
	}
	preLen := int64(len(rawBytes))
	go func(c context.Context, id string, data []byte) {
		if c.Err() != nil {
			return // Context cancelled; store may be closing
		}
		telemetry.OptMetrics.CSSAOffloadBytes.Add(int64(len(data)))
		if err := ps.Handler.Store.SaveRawResource(id, data); err != nil {
			slog.Warn("gateway: failed to session-cache raw response", "call_id", id, "error", err)
		}
	}(ctx, callID, rawBytes)

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("gateway: pre-transform raw output", "server", server, "tool", tool, "size", len(rawBytes), "content", string(rawBytes))
	}

	// Hybrid JSON markdown_payload fast-path: if the StructuredContent contains
	// a pre-rendered GFM markdown payload, surface it directly as TextContent.
	// This bypasses transformToHybrid which would strip the markdown into a generic
	// JSON code block, making reports unreadable to the AI agent.
	if mdPayload := extractMarkdownPayload(rawBytes); mdPayload != "" {
		slog.Info("gateway: hybrid markdown_payload fast-path activated",
			"server", server, "tool", tool, "md_size", len(mdPayload))

		postLen := int64(len(mdPayload))
		ps.Handler.Telemetry.AddBytes(server, sentSize, preLen, postLen)

		md := mdPayload + fmt.Sprintf("\n\n[Full raw output available: mcp://magictools/raw/%s]", callID)
		res.Content = append([]mcp.Content{&mcp.TextContent{Text: md}}, res.Content...)
		res.StructuredContent = nil
		return res
	}

	// Phase 2: Small-response fast-path — skip squeeze/truncate for payloads < 1KB
	if preLen < 1024 {
		slog.Log(ctx, util.LevelTrace, "gateway: fast-path (small response)", "server", server, "tool", tool, "size", preLen)

		minifiedData, miniErr := json.MarshalIndent(res.StructuredContent, "", "  ")
		if miniErr != nil {
			slog.Warn("gateway: failed to marshal for hybrid minification", "error", miniErr)
		}
		md := transformToHybrid(minifiedData, ps.Handler.Config.MaxResponseTokens)

		postLen := int64(len(md))
		ps.Handler.Telemetry.AddBytes(server, sentSize, preLen, postLen)

		md += fmt.Sprintf("\n\n[Full raw output available: mcp://magictools/raw/%s]", callID)
		res.Content = append([]mcp.Content{&mcp.TextContent{Text: md}}, res.Content...)
		res.StructuredContent = nil
		return res
	}

	// B. Single-pass squeeze + truncate: remove nulls/empty arrays AND center-truncate large strings (>2000 chars)
	telemetry.OptMetrics.SqueezeTruncations.Add(1)
	res.StructuredContent = util.SqueezeAndTruncate(res.StructuredContent, truncateLimit)

	processedBytes, procErr := json.Marshal(res.StructuredContent)
	if procErr == nil {
		postLen := int64(len(processedBytes))
		telemetry.OptMetrics.TotalRawBytes.Add(preLen)
		telemetry.OptMetrics.TotalSqueezedBytes.Add(postLen)
		if slog.Default().Enabled(ctx, slog.LevelDebug) {
			slog.Debug("gateway: post-squeeze-truncate output", "server", server, "tool", tool, "pre_size", preLen, "post_size", postLen, "content", string(processedBytes))
		}
	}

	// D. Transform to Hybrid Markdown
	minifiedData, miniErr := json.MarshalIndent(res.StructuredContent, "", "  ")
	if miniErr != nil {
		slog.Warn("gateway: failed to marshal structured content to markdown", "error", miniErr)
	}
	md := transformToHybrid(minifiedData, ps.Handler.Config.MaxResponseTokens)

	postLen := int64(len(md))
	ps.Handler.Telemetry.AddBytes(server, sentSize, preLen, postLen)

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("gateway: post-minifier markdown", "server", server, "tool", tool, "size", len(md), "content", md)
	}

	// Append the retrieval footer for cached raw data
	md += fmt.Sprintf("\n\n[Full raw output available: mcp://magictools/raw/%s]", callID)

	res.Content = append([]mcp.Content{&mcp.TextContent{Text: md}}, res.Content...)
	res.StructuredContent = nil

	return res
}

// SoftFailureDiagnostic describes a tool response that succeeded at the RPC
// level but returned suspiciously empty or zero-result data.
type SoftFailureDiagnostic struct {
	Detected   bool   `json:"detected"`
	Reason     string `json:"reason"`
	Server     string `json:"server"`
	Tool       string `json:"tool"`
	Suggestion string `json:"suggestion"`
}

// InspectResponse examines a successful proxy result for soft-failure patterns.
// It returns nil when the response looks normal (fast exit for the happy path).
func (ps *ProxyService) InspectResponse(ctx context.Context, res *mcp.CallToolResult, server, tool string) *SoftFailureDiagnostic {
	if res == nil || res.IsError {
		return nil
	}

	// Inspect StructuredContent (JSON payload) when present
	if res.StructuredContent != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return nil
		}
		if diag := inspectJSON(raw, server, tool); diag != nil {
			return diag
		}
	}

	// Inspect text content as a fallback (some servers return JSON in text)
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok || len(tc.Text) == 0 {
			continue
		}
		// Only attempt JSON parse if it looks like JSON
		trimmed := strings.TrimSpace(tc.Text)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			if diag := inspectJSON([]byte(trimmed), server, tool); diag != nil {
				return diag
			}
		}
	}

	return nil
}

// inspectJSON checks a raw JSON payload for soft-failure indicators.
func inspectJSON(raw []byte, server, tool string) *SoftFailureDiagnostic {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	// Check nested "data" wrapper (common pattern: {data: {results: [], metadata: {total_count: 0}}})
	if nested, ok := payload["data"].(map[string]any); ok {
		if diag := checkEmptyResults(nested, server, tool); diag != nil {
			return diag
		}
	}

	return checkEmptyResults(payload, server, tool)
}

// checkEmptyResults inspects a JSON object for empty-result patterns.
func checkEmptyResults(m map[string]any, server, tool string) *SoftFailureDiagnostic {
	// Pattern 1: "results" key is an empty array
	if results, ok := m["results"]; ok {
		if arr, isArr := results.([]any); isArr && len(arr) == 0 {
			return &SoftFailureDiagnostic{
				Detected:   true,
				Reason:     "results array is empty",
				Server:     server,
				Tool:       tool,
				Suggestion: "The tool returned successfully but with zero results. Consider retrying with different parameters or checking the sub-server logs.",
			}
		}
	}

	// Pattern 2: Count-like keys equal to zero
	countKeys := []string{"total_count", "result_count", "count", "totalCount", "resultCount"}
	for _, key := range countKeys {
		if v, ok := m[key]; ok {
			if isZeroNumeric(v) {
				return &SoftFailureDiagnostic{
					Detected:   true,
					Reason:     fmt.Sprintf("%s is 0", key),
					Server:     server,
					Tool:       tool,
					Suggestion: "The tool reported zero matching entries. Verify the query parameters or check sub-server connectivity.",
				}
			}
		}

		// Also check inside "metadata" wrapper
		if meta, ok := m["metadata"].(map[string]any); ok {
			if v, ok := meta[key]; ok {
				if isZeroNumeric(v) {
					return &SoftFailureDiagnostic{
						Detected:   true,
						Reason:     fmt.Sprintf("metadata.%s is 0", key),
						Server:     server,
						Tool:       tool,
						Suggestion: "The tool reported zero matching entries. Verify the query parameters or check sub-server connectivity.",
					}
				}
			}
		}
	}

	return nil
}

// isZeroNumeric checks whether a JSON-decoded value is numerically zero.
func isZeroNumeric(v any) bool {
	switch n := v.(type) {
	case float64:
		return n == 0
	case int:
		return n == 0
	case int64:
		return n == 0
	}
	return false
}

// snapToEnum uses Levenshtein distance to automatically "snap" minor enum hallucinations
// to valid schema bounds. This minimizes orchestrator retries for trivial mismatches.
func (ps *ProxyService) snapToEnum(val string, enum []any) string {
	if len(enum) == 0 {
		return val
	}
	minDist := 100
	bestMatch := val
	lowerVal := strings.ToLower(val)

	for _, e := range enum {
		if s, ok := e.(string); ok {
			if strings.EqualFold(s, val) {
				return s // Perfect match (case-insensitive)
			}
			dist := util.LevenshteinDistance(lowerVal, strings.ToLower(s))
			if dist < minDist {
				minDist = dist
				bestMatch = s
			}
		}
	}

	// Only snap if the distance is small (heuristic: distance <= 2 or < 30% of target length)
	if minDist <= 2 || float64(minDist) < float64(len(bestMatch))*0.3 {
		return bestMatch
	}
	return val
}

// repairJSONHeuristic attempts structural repairs on malformed JSON strings
// before the final validation pass. Handle Markdown escapes and missing braces.
func (ps *ProxyService) repairJSONHeuristic(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input
	}

	// 1. Handle Markdown code block escapes
	if after, ok := strings.CutPrefix(trimmed, "```json"); ok {
		trimmed = after
		trimmed = strings.TrimSuffix(trimmed, "```")
	} else if after, ok := strings.CutPrefix(trimmed, "```"); ok {
		trimmed = after
		trimmed = strings.TrimSuffix(trimmed, "```")
	}
	trimmed = strings.TrimSpace(trimmed)

	// 2. Repair missing closing braces
	opens := strings.Count(trimmed, "{")
	closes := strings.Count(trimmed, "}")
	if opens > closes {
		trimmed += strings.Repeat("}", opens-closes)
	}

	return trimmed
}
