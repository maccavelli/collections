package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"mcp-server-magictools/internal/config"
)

// Embedder is the unified interface for all embedding providers.
// Implementations must be safe for concurrent use.
type Embedder interface {
	// Embed converts text into a dense float32 vector.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Provider returns the canonical provider name (gemini, openai, voyage).
	Provider() string
}

// NewEmbedderFromConfig constructs the appropriate Embedder based on config.
// Returns nil if no embedding provider is configured or discoverable.
func NewEmbedderFromConfig(cfg *config.Config) Embedder {
	provider := cfg.Intelligence.EmbeddingProvider
	model := cfg.Intelligence.EmbeddingModel
	apiKey := cfg.Intelligence.EmbeddingAPIKey
	dims := cfg.Intelligence.EmbeddingDimensionality

	if provider == "" || (apiKey == "" && provider != "ollama") {
		slog.Warn("embedding engine DISABLED: missing embedding_provider or embedding_api_key",
			"component", "vector")
		return nil
	}

	if dims <= 0 {
		dims = 1536
	}

	slog.Info("embedding engine initialized",
		"component", "vector",
		"provider", provider,
		"model", model,
		"dimensions", dims)

	switch provider {
	case "gemini":
		return &GeminiEmbedder{apiKey: apiKey, model: model, dims: dims}
	case "openai":
		return &OpenAIEmbedder{apiKey: apiKey, model: model, dims: dims}
	case "voyage":
		return &VoyageEmbedder{apiKey: apiKey, model: model}
	case "ollama":
		return &OllamaEmbedder{
			apiURL: cfg.Intelligence.EmbeddingAPIURL,
			model:  model,
			apiKey: apiKey,
		}
	default:
		slog.Warn("unknown embedding provider, vector engine disabled",
			"component", "vector", "provider", provider)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Gemini Embedder — uses REST (GenAI SDK to be added when go.mod dependency lands)
// ---------------------------------------------------------------------------

// GeminiEmbedder implements Embedder using the Gemini embedContent REST API.
// Supports task_type and output_dimensionality for gemini-embedding-2-preview+.
type GeminiEmbedder struct {
	apiKey string
	model  string
	dims   int
}

func (e *GeminiEmbedder) Provider() string { return "gemini" }

func (e *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	type part struct {
		Text string `json:"text"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type reqBody struct {
		Model                string  `json:"model"`
		Content              content `json:"content"`
		TaskType             string  `json:"taskType,omitempty"`
		OutputDimensionality int     `json:"outputDimensionality,omitempty"`
	}

	// Determine task type from context (set by caller via context value)
	taskType, _ := ctx.Value(ctxKeyTaskType).(string)

	body := reqBody{
		Model:   "models/" + e.model,
		Content: content{Parts: []part{{Text: text}}},
	}
	if taskType != "" {
		body.TaskType = taskType
	}
	if e.dims > 0 && strings.Contains(e.model, "embedding-2") {
		body.OutputDimensionality = e.dims
	}

	reqBytes, _ := json.Marshal(body)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s",
		e.model, e.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("gemini embed: request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gemini embedding error: status %d", resp.StatusCode)
	}

	var res struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("gemini embed: decode failed: %w", err)
	}
	return res.Embedding.Values, nil
}

// ---------------------------------------------------------------------------
// OpenAI Embedder — REST client for api.openai.com/v1/embeddings
// ---------------------------------------------------------------------------

// OpenAIEmbedder implements Embedder using the OpenAI Embeddings REST API.
type OpenAIEmbedder struct {
	apiKey string
	model  string
	dims   int
}

func (e *OpenAIEmbedder) Provider() string { return "openai" }

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	type reqBody struct {
		Input      string `json:"input"`
		Model      string `json:"model"`
		Dimensions int    `json:"dimensions,omitempty"`
	}
	body := reqBody{Input: text, Model: e.model}
	if e.dims > 0 && strings.Contains(e.model, "text-embedding-3") {
		body.Dimensions = e.dims
	}

	reqBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings",
		bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("openai embed: request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai embedding error: status %d", resp.StatusCode)
	}

	var res struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("openai embed: decode failed: %w", err)
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("openai returned empty embedding array")
	}
	return res.Data[0].Embedding, nil
}

// ---------------------------------------------------------------------------
// Voyage Embedder — REST client for api.voyageai.com/v1/embeddings
// ---------------------------------------------------------------------------

// VoyageEmbedder implements Embedder using the Voyage AI REST API.
type VoyageEmbedder struct {
	apiKey string
	model  string
}

func (e *VoyageEmbedder) Provider() string { return "voyage" }

func (e *VoyageEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	type reqBody struct {
		Input []string `json:"input"`
		Model string   `json:"model"`
	}
	body := reqBody{Input: []string{text}, Model: e.model}

	reqBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.voyageai.com/v1/embeddings",
		bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("voyage embed: request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("voyage embedding error: status %d", resp.StatusCode)
	}

	var res struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("voyage embed: decode failed: %w", err)
	}
	if len(res.Data) == 0 {
		return nil, fmt.Errorf("voyage returned empty embedding array")
	}
	return res.Data[0].Embedding, nil
}

// ---------------------------------------------------------------------------
// Ollama Embedder — REST client for local Ollama instances
// ---------------------------------------------------------------------------

// OllamaEmbedder implements Embedder using the Ollama /api/embed REST endpoint.
// Validated 384-dimension models:
//   - granite-embedding:30m  (default, 63MB, IBM Granite)
//   - snowflake-arctic-embed:33m (67MB, Snowflake Labs)
//   - snowflake-arctic-embed:22m (46MB, Snowflake Labs)
//   - all-minilm:33m (67MB, sentence-transformers)
//   - all-minilm:22m (46MB, sentence-transformers)
type OllamaEmbedder struct {
	apiURL string // e.g., http://localhost:11434
	model  string // e.g., granite-embedding:30m
	apiKey string // optional — most Ollama instances run without auth
}

func (e *OllamaEmbedder) Provider() string { return "ollama" }

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	type reqBody struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}
	body := reqBody{Model: e.model, Input: text}

	reqBytes, _ := json.Marshal(body)

	url := strings.TrimRight(e.apiURL, "/") + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama embed: request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama embedding error: status %d", resp.StatusCode)
	}

	var res struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("ollama embed: decode failed: %w", err)
	}
	if len(res.Embeddings) == 0 || len(res.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding array")
	}
	return res.Embeddings[0], nil
}

// ---------------------------------------------------------------------------
// Shared infrastructure
// ---------------------------------------------------------------------------

// ctxKey is an unexported type for context value keys to prevent collisions.
type ctxKey string

// ctxKeyTaskType is the context key for embedding task type (RETRIEVAL_DOCUMENT / RETRIEVAL_QUERY).
const ctxKeyTaskType ctxKey = "embedding_task_type"

// WithTaskType returns a child context carrying the specified embedding task type.
// Currently only used by Gemini; other providers silently ignore it.
func WithTaskType(ctx context.Context, taskType string) context.Context {
	return context.WithValue(ctx, ctxKeyTaskType, taskType)
}

// httpClient is a shared HTTP client with sane timeouts for all embedding REST calls.
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}
