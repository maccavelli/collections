package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"mcp-server-magicskills/internal/models"
	"mcp-server-magicskills/internal/state"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/blevesearch/bleve/v2/analysis"
	"github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/registry"
)

func init() {
	registry.RegisterAnalyzer("edgeNgramAnalyzer", func(config map[string]interface{}, cache *registry.Cache) (analysis.Analyzer, error) {
		tokenizer, err := cache.TokenizerNamed(unicode.Name)
		if err != nil {
			return nil, err
		}
		lcFilter, err := cache.TokenFilterNamed(lowercase.Name)
		if err != nil {
			return nil, err
		}
		ngramFilter := edgengram.NewEdgeNgramFilter(edgengram.FRONT, 2, 25)
		return &analysis.DefaultAnalyzer{
			Tokenizer:    tokenizer,
			TokenFilters: []analysis.TokenFilter{lcFilter, ngramFilter},
		}, nil
	})
}

const (
	CurrentSchemaVersion = "2.0"
)

var (
	builderPool = sync.Pool{
		New: func() any {
			b := new(strings.Builder)
			b.Grow(1024)
			return b
		},
	}
)

type Engine struct {
	mu           sync.RWMutex
	Skills       map[string]*models.Skill
	PathToName   map[string]string
	Bleve        bleve.Index
	Store        *state.Store
	BrokenSkills []string
	MatchCache   *lru.Cache[string, []ScoredSkill]
	ReadyCh      chan struct{}
}

func slugify(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return slug
}

func buildMapping() mapping.IndexMapping {
	textField := bleve.NewTextFieldMapping()
	textField.Store = false
	textField.Index = true
	textField.Analyzer = "edgeNgramAnalyzer"

	keywordField := bleve.NewKeywordFieldMapping()
	keywordField.Store = false
	keywordField.Index = true

	docMapping := bleve.NewDocumentMapping()
	docMapping.AddFieldMappingsAt("name", textField)
	docMapping.AddFieldMappingsAt("description", textField)
	docMapping.AddFieldMappingsAt("context_domain", keywordField)
	docMapping.AddFieldMappingsAt("tags", keywordField)
	docMapping.AddFieldMappingsAt("content", textField)
	docMapping.AddFieldMappingsAt("digest", textField)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.DefaultMapping = docMapping
	indexMapping.DefaultMapping.Dynamic = false

	return indexMapping
}

func NewEngine(store *state.Store, indexPath string) (*Engine, error) {
	// 🛡️ PERF: Strict in-memory volatile execution natively bypassing disk SSD thrashing
	idx, err := bleve.NewMemOnly(buildMapping())
	if err != nil {
		return nil, fmt.Errorf("failed to init bleve index: %w", err)
	}

	// Initialize LRU
	cache, _ := lru.New[string, []ScoredSkill](128)

	return &Engine{
		Skills:       make(map[string]*models.Skill),
		PathToName:   make(map[string]string),
		Store:        store,
		Bleve:        idx,
		BrokenSkills: make([]string, 0),
		MatchCache:   cache,
		ReadyCh:      make(chan struct{}),
	}, nil
}

// WaitReady blocks the context until the semantic engine has loaded completely.
func (e *Engine) WaitReady(ctx context.Context) error {
	if e == nil {
		return nil
	}
	select {
	case <-e.ReadyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CountSkills returns the total number of skills stored securely.
func (e *Engine) CountSkills() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.Skills)
}

// CountBleveDocs returns the exact number of indexed elements in the Bleve engine.
func (e *Engine) CountBleveDocs() uint64 {
	if e.Bleve == nil {
		return 0
	}
	count, err := e.Bleve.DocCount()
	if err != nil {
		return 0
	}
	return count
}
