package db

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"sync/atomic"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/token/camelcase"
	"github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/registry"
	"github.com/blevesearch/bleve/v2/search/query"
)

const camelSplitAnalyzerName = "camel_split"
const edgeNgramAnalyzerName = "name_edge_ngram"

var roleTagStripperIndex = regexp.MustCompile(`(?i)\[.*?\]\s*`)

func init() {
	// Register a compound analyzer: unicode tokenizer → camelCase split → lowercase.
	// This enables "sequentialthinking" to match searches for "sequential" or "thinking".
	registry.RegisterAnalyzer(camelSplitAnalyzerName, func(config map[string]any, cache *registry.Cache) (analysis.Analyzer, error) {
		tokenizer, err := cache.TokenizerNamed(unicode.Name)
		if err != nil {
			return nil, err
		}
		camelFilter, err := cache.TokenFilterNamed(camelcase.Name)
		if err != nil {
			return nil, err
		}
		lcFilter, err := cache.TokenFilterNamed(lowercase.Name)
		if err != nil {
			return nil, err
		}
		return &analysis.DefaultAnalyzer{
			Tokenizer:    tokenizer,
			TokenFilters: []analysis.TokenFilter{camelFilter, lcFilter},
		}, nil
	})

	registry.RegisterAnalyzer(edgeNgramAnalyzerName, func(config map[string]any, cache *registry.Cache) (analysis.Analyzer, error) {
		tokenizer, err := cache.TokenizerNamed(unicode.Name)
		if err != nil {
			return nil, err
		}
		camelFilter, err := cache.TokenFilterNamed(camelcase.Name)
		if err != nil {
			return nil, err
		}
		lcFilter, err := cache.TokenFilterNamed(lowercase.Name)
		if err != nil {
			return nil, err
		}
		ngramFilter := edgengram.NewEdgeNgramFilter(edgengram.FRONT, 3, 14)
		return &analysis.DefaultAnalyzer{
			Tokenizer:    tokenizer,
			TokenFilters: []analysis.TokenFilter{camelFilter, lcFilter, ngramFilter},
		}, nil
	})
}

// BleveToolDocument is an adapter struct that provides proper JSON tags for Bleve
// indexing. ToolRecord's intelligence fields (SyntheticIntents, LexicalTokens, etc.)
// use json:"-" tags to exclude them from BadgerDB serialization, which also makes them
// invisible to Bleve's reflection-based field discovery. This adapter bridges the gap.
type BleveToolDocument struct {
	URN              string   `json:"urn"`
	Name             string   `json:"name"`
	Server           string   `json:"server"`
	Description      string   `json:"description"`
	Category         string   `json:"category"`
	Intent           string   `json:"intent"`
	SyntheticIntents string   `json:"synthetic_intents"`
	LexicalTokens    []string `json:"lexical_tokens"`
	UsageCount       int64    `json:"usage_count"`
	ProxyReliability float64  `json:"proxy_reliability"`
	InputSchema      string   `json:"input_schema"`
}

// Type natively binds this schema directly to the specific "tool" document mapping in Bleve
func (d BleveToolDocument) Type() string {
	return "tool"
}

// ToBleveDoc converts a ToolRecord (with overlaid intelligence) to a BleveToolDocument.
func ToBleveDoc(r *ToolRecord) BleveToolDocument {
	rawSchema, _ := json.Marshal(r.InputSchema)
	cleanDesc := roleTagStripperIndex.ReplaceAllString(r.Description, "")
	return BleveToolDocument{
		URN:              r.URN,
		Name:             r.Name,
		Server:           r.Server,
		Description:      cleanDesc,
		Category:         r.Category,
		Intent:           r.Intent,
		SyntheticIntents: strings.Join(r.SyntheticIntents, " "),
		LexicalTokens:    r.LexicalTokens,
		UsageCount:       r.UsageCount,
		ProxyReliability: r.Metrics.ProxyReliability,
		InputSchema:      string(rawSchema),
	}
}

// SyntheticIntent represents a successfully validated Prompt-to-DAG vector pairing organically tracked locally.
type SyntheticIntent struct {
	ID        string   `json:"id"`
	Prompt    string   `json:"prompt"`
	DAG       []string `json:"dag"`
	Timestamp int64    `json:"timestamp"` // Unix timestamp for confidence decay
}

func (s SyntheticIntent) Type() string {
	return "intent_cache"
}

// SearchIndex manages the Bleve index
type SearchIndex struct {
	index atomic.Value // holds bleve.Index
}

// NewSearchIndex creates an in-memory fast-path index shadowing BadgerDB.
func NewSearchIndex(dbPath string) (*SearchIndex, error) {
	// Define Mapping
	indexMapping := bleve.NewIndexMapping()

	// Map Tool Fields
	toolMapping := bleve.NewDocumentMapping()

	// - URN: Keyword (Exact match, no tokenization)
	urnFieldMapping := bleve.NewKeywordFieldMapping()
	toolMapping.AddFieldMappingsAt("urn", urnFieldMapping)

	// - Name: Text with edge ngram analyzer for partial matches
	nameFieldMapping := bleve.NewTextFieldMapping()
	nameFieldMapping.Analyzer = edgeNgramAnalyzerName
	toolMapping.AddFieldMappingsAt("name", nameFieldMapping)

	// - Server: Keyword (Exact match for telemetry filtering)
	serverFieldMapping := bleve.NewKeywordFieldMapping()
	toolMapping.AddFieldMappingsAt("server", serverFieldMapping)

	// - Description: Text (BM25 Full-text)
	descFieldMapping := bleve.NewTextFieldMapping()
	toolMapping.AddFieldMappingsAt("description", descFieldMapping)

	// - Category: Edge N-gram text analyzer for partial concept matches
	catFieldMapping := bleve.NewTextFieldMapping()
	catFieldMapping.Analyzer = edgeNgramAnalyzerName
	toolMapping.AddFieldMappingsAt("category", catFieldMapping)

	// - InputSchema: Edge N-gram text analyzer for granular parameter matching natively
	schemaFieldMapping := bleve.NewTextFieldMapping()
	schemaFieldMapping.Analyzer = edgeNgramAnalyzerName
	toolMapping.AddFieldMappingsAt("input_schema", schemaFieldMapping)

	// - Intent: Text field for conceptual matching (semantic substitute)
	intentFieldMapping := bleve.NewTextFieldMapping()
	intentFieldMapping.Analyzer = "en"
	toolMapping.AddFieldMappingsAt("intent", intentFieldMapping)

	// - SyntheticIntents: Text field for LLM generated intents
	syntheticIntentsFieldMapping := bleve.NewTextFieldMapping()
	syntheticIntentsFieldMapping.Analyzer = "en"
	toolMapping.AddFieldMappingsAt("synthetic_intents", syntheticIntentsFieldMapping)

	// - LexicalTokens: Keyword array for targeted terminology
	lexicalTokensFieldMapping := bleve.NewKeywordFieldMapping()
	toolMapping.AddFieldMappingsAt("lexical_tokens", lexicalTokensFieldMapping)

	// - UsageCount: Numeric for scoring boost
	usageFieldMapping := bleve.NewNumericFieldMapping()
	toolMapping.AddFieldMappingsAt("usage_count", usageFieldMapping)

	// - ProxyReliability: Numeric for empirical trust boost natively mapping Handler telemetry to Index Math
	reliabilityFieldMapping := bleve.NewNumericFieldMapping()
	toolMapping.AddFieldMappingsAt("proxy_reliability", reliabilityFieldMapping)

	indexMapping.AddDocumentMapping("tool", toolMapping)

	// 🛡️ INTENT CACHE MAPPING
	intentCacheMapping := bleve.NewDocumentMapping()
	promptFieldMapping := bleve.NewTextFieldMapping()
	promptFieldMapping.Analyzer = "en"
	intentCacheMapping.AddFieldMappingsAt("prompt", promptFieldMapping)

	dagFieldMapping := bleve.NewKeywordFieldMapping()
	intentCacheMapping.AddFieldMappingsAt("dag", dagFieldMapping)

	indexMapping.AddDocumentMapping("intent_cache", intentCacheMapping)

	// Create MemOnly index natively (volatily shadows persistent BadgerDB purely locally)
	index, err := bleve.NewMemOnly(indexMapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory bleve index: %w", err)
	}

	si := &SearchIndex{}
	si.index.Store(index)
	return si, nil
}

// Search performs a multi-stage ranked search with usage-weighted scoring.
func (si *SearchIndex) Search(queryStr string, category string, serverConstraint string, domain SearchDomain) (*bleve.SearchResult, error) {
	// 1. Exact Match (TermQuery on Keyword fields) - High Boost
	exactUrnQuery := bleve.NewTermQuery(queryStr)
	exactUrnQuery.SetField("urn")
	exactUrnQuery.SetBoost(100.0)

	exactNameQuery := bleve.NewTermQuery(queryStr)
	exactNameQuery.SetField("name")
	exactNameQuery.SetBoost(50.0)

	// 2. Fuzzy/Match Search (BM25) — Dynamic fuzziness scaling for short tokens
	matchQuery := bleve.NewBooleanQuery()
	tokens := strings.FieldsFunc(queryStr, func(c rune) bool {
		return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'))
	})

	if len(tokens) == 0 {
		mq := bleve.NewMatchQuery(queryStr)
		mq.SetFuzziness(1)
		matchQuery.AddShould(mq)
	} else {
		for _, token := range tokens {
			mq := bleve.NewMatchQuery(token)
			if len(token) <= 3 {
				mq.SetFuzziness(0)
			} else {
				mq.SetFuzziness(2)
			}
			matchQuery.AddShould(mq)
		}
	}
	matchQuery.SetBoost(1.0)

	// 2.5 Explicit Description Match Query
	descMatchQuery := bleve.NewMatchQuery(queryStr)
	descMatchQuery.SetField("description")
	descMatchQuery.SetBoost(1.5)

	// 3. Intent Search (boosted — enriched by synonym expansion)
	intentQuery := bleve.NewMatchQuery(queryStr)
	intentQuery.SetField("intent")
	intentQuery.SetBoost(3.0) // Increased from 2.0 to reflect richer intent data

	// 4. Name Search with camelCase splitting
	nameMatchQuery := bleve.NewMatchQuery(queryStr)
	nameMatchQuery.SetField("name")
	nameMatchQuery.SetFuzziness(1)
	nameMatchQuery.SetBoost(4.0)

	// 6. PrefixQuery (Option 2)
	parts := strings.Fields(queryStr)
	var prefixQuery query.Query
	if len(parts) > 0 {
		pr := bleve.NewPrefixQuery(strings.ToLower(parts[0]))
		pr.SetField("name")
		pr.SetBoost(2.0)
		prefixQuery = pr
	} else {
		pr := bleve.NewMatchNoneQuery()
		prefixQuery = pr
	}

	// 7. WildcardQuery across space-separated terms
	wildtext := "*" + strings.ToLower(strings.Join(parts, "*")) + "*"
	wildcardQuery := bleve.NewWildcardQuery(wildtext)
	wildcardQuery.SetField("name")
	wildcardQuery.SetBoost(2.0)

	// 8. Synthetic Intent Search (boosted heavily from NLP expansion)
	synthIntentQuery := bleve.NewMatchQuery(queryStr)
	synthIntentQuery.SetField("synthetic_intents")
	synthIntentQuery.SetBoost(5.0)

	// 9. Lexical Tokens (Term)
	lexicalQuery := bleve.NewTermQuery(queryStr)
	lexicalQuery.SetField("lexical_tokens")
	lexicalQuery.SetBoost(3.0)

	var finalQuery query.Query
	if queryStr == "" {
		// 🛡️ MATCH ALL: Emits all structural docs natively
		finalQuery = bleve.NewMatchAllQuery()
	} else {
		// Combine into Disjunction (Matches any of these)
		finalQuery = bleve.NewDisjunctionQuery(exactUrnQuery, exactNameQuery, matchQuery, descMatchQuery, intentQuery, nameMatchQuery, prefixQuery, wildcardQuery, synthIntentQuery, lexicalQuery)
	}

	// 🚀 Server Boost: Add a Should query to boost the "magictools" server organically
	magictoolsBoost := bleve.NewTermQuery("magictools")
	magictoolsBoost.SetField("server")
	magictoolsBoost.SetBoost(1.25)

	// 🚀 Proxy Reliability Skew: Boost results mathematically functionally robust
	highTrustMin := 0.90
	highTrustQuery := bleve.NewNumericRangeQuery(&highTrustMin, nil)
	highTrustQuery.SetField("proxy_reliability")
	highTrustQuery.SetBoost(2.5)

	// 🚀 Empirical Usage Tracking Skew: Value experience organically
	highUsageMin := 50.0
	highUsageQuery := bleve.NewNumericRangeQuery(&highUsageMin, nil)
	highUsageQuery.SetField("usage_count")
	highUsageQuery.SetBoost(2.0)

	boostWrapper := bleve.NewBooleanQuery()
	boostWrapper.AddMust(finalQuery)
	boostWrapper.AddShould(magictoolsBoost)
	boostWrapper.AddShould(highTrustQuery)
	boostWrapper.AddShould(highUsageQuery)
	finalQuery = boostWrapper

	// 🛡️ DOMAIN-AWARE SHARDING: Enforce strict visibility boundaries
	switch domain {
	case DomainUserLand:
		// Mask brainstorm and go-refactor unless explicitly targeted by URN/Name
		if serverConstraint == "" && !strings.Contains(queryStr, "brainstorm") && !strings.Contains(queryStr, "go-refactor") {
			bq := bleve.NewBooleanQuery()
			bq.AddMust(finalQuery)

			q1 := bleve.NewTermQuery("brainstorm")
			q1.SetField("server")
			q2 := bleve.NewTermQuery("go-refactor")
			q2.SetField("server")

			bq.AddMustNot(q1, q2)
			finalQuery = bq
		}
	case DomainPipelineOrchestration:
		// Restrict ONLY to brainstorm, go-refactor, and magictools synthesizers
		bq := bleve.NewBooleanQuery()
		bq.AddMust(finalQuery)

		q1 := bleve.NewTermQuery("brainstorm")
		q1.SetField("server")
		q2 := bleve.NewTermQuery("go-refactor")
		q2.SetField("server")
		q3 := bleve.NewTermQuery("magictools")
		q3.SetField("server")

		dis := bleve.NewDisjunctionQuery(q1, q2, q3)
		bq.AddMust(dis)
		finalQuery = bq
	case DomainSystem:
		// No sharding, show all tools natively
	}

	// 5. HIERARCHICAL ROUTING: Strict Intent Sharding natively replacing generic confidence gaps
	if category != "" {
		// Enforce strict category prefix bounds natively isolating cross-contamination functionally
		catQuery := bleve.NewTermQuery(category)
		catQuery.SetField("category")

		// Conjunction mathematically isolates searches securely returning only correctly sharded results
		finalQuery = bleve.NewConjunctionQuery(finalQuery, catQuery)
	}

	if serverConstraint != "" {
		// Enforce pure node orchestration functionally
		serverQuery := bleve.NewTermQuery(serverConstraint)
		serverQuery.SetField("server")

		finalQuery = bleve.NewConjunctionQuery(finalQuery, serverQuery)
	}

	searchRequest := bleve.NewSearchRequest(finalQuery)
	searchRequest.Size = 10
	searchRequest.Fields = []string{"urn", "name", "server", "description"}
	highlight := bleve.NewHighlightWithStyle("html")
	highlight.AddField("description")
	searchRequest.Highlight = highlight

	// Density Facet Tracking maps cross-server tool balancing recursively tracking "hot" boundaries.
	catFacet := bleve.NewFacetRequest("category", 10)
	searchRequest.AddFacet("category", catFacet)

	results, err := si.index.Load().(bleve.Index).Search(searchRequest)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// IndexSyntheticIntent maps a successful Socratic interaction vector dynamically into the secondary GhostIndex
func (si *SearchIndex) IndexSyntheticIntent(prompt string, dag []string) error {
	id := fmt.Sprintf("intent:%x", sha256.Sum256([]byte(prompt)))
	doc := SyntheticIntent{
		ID:        id,
		Prompt:    prompt,
		DAG:       dag,
		Timestamp: time.Now().Unix(),
	}
	return si.index.Load().(bleve.Index).Index(id, doc)
}

// GhostResult holds a URN and its associated timestamp for decay computation.
type GhostResult struct {
	URN       string
	Timestamp int64
}

// SearchSyntheticIntents performs a localized semantic probe across past successful orchestrations extracting pure DAG URIs dynamically.
func (si *SearchIndex) SearchSyntheticIntents(prompt string) ([]GhostResult, error) {
	idx, ok := si.index.Load().(bleve.Index)
	if !ok || idx == nil {
		return nil, fmt.Errorf("search index not initialized")
	}

	query := bleve.NewMatchQuery(prompt)
	query.SetField("prompt")

	req := bleve.NewSearchRequest(query)
	req.Size = 1 // Only load highest confidence Ghost DAG natively
	req.Fields = []string{"dag", "timestamp"}

	res, err := idx.Search(req)
	if err != nil || len(res.Hits) == 0 {
		return nil, err
	}

	hit := res.Hits[0]

	// Extract timestamp for decay
	var ts int64
	if tsRaw, ok := hit.Fields["timestamp"].(float64); ok {
		ts = int64(tsRaw)
	}

	// Extract string slices gracefully natively
	var results []GhostResult
	if dagRaw, ok := hit.Fields["dag"].([]any); ok {
		for _, u := range dagRaw {
			results = append(results, GhostResult{URN: fmt.Sprintf("%v", u), Timestamp: ts})
		}
	} else if dagSingle, ok := hit.Fields["dag"].(string); ok {
		results = append(results, GhostResult{URN: dagSingle, Timestamp: ts})
	}

	return results, nil
}

// GetToolsByServer natively interrogates the search index extracting raw URIs for a specific server instance.
func (si *SearchIndex) GetToolsByServer(serverName string, limit int) ([]string, error) {
	idx, ok := si.index.Load().(bleve.Index)
	if !ok || idx == nil {
		return nil, fmt.Errorf("search index not initialized")
	}

	query := bleve.NewMatchQuery(serverName)
	query.SetField("server")

	req := bleve.NewSearchRequest(query)
	req.Size = limit
	req.Fields = []string{"urn"}

	res, err := idx.Search(req)
	if err != nil {
		return nil, err
	}

	var urns []string
	for _, hit := range res.Hits {
		if urn, ok := hit.Fields["urn"].(string); ok {
			urns = append(urns, urn)
		} else {
			urns = append(urns, hit.ID)
		}
	}
	return urns, nil
}

// IsEmpty returns true if the index has no documents
func (si *SearchIndex) IsEmpty() bool {
	idx, ok := si.index.Load().(bleve.Index)
	if !ok || idx == nil {
		return true
	}
	count, err := idx.DocCount()
	return err != nil || count == 0
}

// DocCount returns the number of active documents inside the semantic inference index natively
func (si *SearchIndex) DocCount() (uint64, error) {
	idx, ok := si.index.Load().(bleve.Index)
	if !ok || idx == nil {
		return 0, fmt.Errorf("search index not initialized")
	}
	return idx.DocCount()
}

// IndexRecord adds or updates a record in the index using the Bleve adapter.
func (si *SearchIndex) IndexRecord(doc BleveToolDocument) error {
	return si.index.Load().(bleve.Index).Index(doc.URN, doc)
}

// IndexBatch adds or updates a slice of Bleve documents in the index efficiently via a single transaction.
func (si *SearchIndex) IndexBatch(docs []BleveToolDocument) error {
	idx, ok := si.index.Load().(bleve.Index)
	if !ok || idx == nil {
		return fmt.Errorf("search index not initialized")
	}

	batch := idx.NewBatch()
	for _, doc := range docs {
		if err := batch.Index(doc.URN, doc); err != nil {
			return err
		}
	}
	return idx.Batch(batch)
}

// DeleteRecord removes a record from the index
func (si *SearchIndex) DeleteRecord(urn string) error {
	return si.index.Load().(bleve.Index).Delete(urn)
}

// SwapIndex safely hot-swaps the underlying bleve.Index pointer and closes the old one
func (si *SearchIndex) SwapIndex(newIndex bleve.Index) error {
	oldIndex := si.index.Load().(bleve.Index)
	si.index.Store(newIndex)
	if oldIndex != nil {
		return oldIndex.Close()
	}
	return nil
}

// Close closes the index
func (si *SearchIndex) Close() error {
	idx := si.index.Load().(bleve.Index)
	if idx != nil {
		return idx.Close()
	}
	return nil
}
