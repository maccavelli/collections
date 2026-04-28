package dag

import (
	"math"
	"mcp-server-magictools/internal/db"
	"regexp"
	"strings"
)

// roleTagStripper removes [ROLE: X] prefixes from descriptions before BM25 tokenization.
var roleTagStripper = regexp.MustCompile(`(?i)\[ROLE:\s*\w+\]\s*`)

// Document represents an indexed tool.
type Document struct {
	URN    string
	Tokens []string
}

// BM25Scorer calculates semantic relevance using BM25.
type BM25Scorer struct {
	k1      float64
	b       float64
	docs    []Document
	avgdl   float64
	docFreq map[string]int
}

// NewBM25Scorer initializes the scoring matrix.
func NewBM25Scorer(tools []*db.ToolRecord) *BM25Scorer {
	scorer := &BM25Scorer{
		k1:      1.2,
		b:       0.75,
		docFreq: make(map[string]int),
	}

	tokenizer := regexp.MustCompile(`[a-zA-Z0-9]+`)
	stopWords := map[string]bool{
		"the": true, "and": true, "a": true, "to": true, "of": true,
		"in": true, "i": true, "is": true, "that": true, "it": true,
		"on": true, "you": true, "this": true, "for": true, "or": true, "with": true,
		"as": true, "by": true, "an": true, "be": true,
	}

	var totalLength int
	for _, t := range tools {
		if t == nil {
			continue
		}
		// Combine URN and Description for the document.
		// Strip [ROLE: X] prefix to prevent role tags from polluting BM25 term frequencies.
		desc := roleTagStripper.ReplaceAllString(t.Description, "")

		descTokens := tokenizer.FindAllString(strings.ToLower(desc), -1)
		nameTokens := tokenizer.FindAllString(strings.ToLower(t.Name), -1)
		urnTokens := tokenizer.FindAllString(strings.ToLower(t.URN), -1)

		var rawTokens []string
		for _, tok := range descTokens {
			if !stopWords[tok] {
				rawTokens = append(rawTokens, tok)
			}
		}
		for _, tok := range nameTokens {
			if !stopWords[tok] {
				rawTokens = append(rawTokens, tok, tok) // 2x multiplier
			}
		}
		for _, tok := range urnTokens {
			if !stopWords[tok] {
				rawTokens = append(rawTokens, tok, tok, tok) // 3x multiplier
			}
		}

		doc := Document{
			URN:    t.URN,
			Tokens: rawTokens,
		}
		scorer.docs = append(scorer.docs, doc)
		totalLength += len(rawTokens)

		// Calculate doc frequency
		seen := make(map[string]bool)
		for _, tok := range rawTokens {
			if !seen[tok] {
				scorer.docFreq[tok]++
				seen[tok] = true
			}
		}
	}

	if len(scorer.docs) > 0 {
		scorer.avgdl = float64(totalLength) / float64(len(scorer.docs))
	}

	return scorer
}

// Score calculates the BM25 score for each document against the query,
// then applies min-max normalization to compress results to [0,1].
func (s *BM25Scorer) Score(query string) map[string]float64 {
	scores := make(map[string]float64)
	queryTokens := strings.Fields(strings.ToLower(query))

	N := float64(len(s.docs))

	for _, doc := range s.docs {
		var docScore float64
		D := float64(len(doc.Tokens))

		termFreq := make(map[string]float64)
		for _, tok := range doc.Tokens {
			termFreq[tok]++
		}

		for _, qToken := range queryTokens {
			// Calculate IDF
			n := float64(s.docFreq[qToken])
			idf := math.Log(((N - n + 0.5) / (n + 0.5)) + 1.0)

			// Calculate TF component
			tf := termFreq[qToken]
			if tf == 0 {
				continue
			}

			tfNum := tf * (s.k1 + 1.0)
			tfDen := tf + s.k1*(1.0-s.b+s.b*(D/s.avgdl))

			docScore += idf * (tfNum / tfDen)
		}
		scores[doc.URN] = docScore
	}

	// Min-Max Normalization: Compress raw BM25 scores to [0,1]
	// This ensures mathematical consistency with the vector engine's cosine similarity range.
	var minScore, maxScore float64
	first := true
	for _, v := range scores {
		if first || v < minScore {
			minScore = v
		}
		if first || v > maxScore {
			maxScore = v
		}
		first = false
	}
	spread := maxScore - minScore
	if spread > 0 {
		for k, v := range scores {
			scores[k] = (v - minScore) / spread
		}
	} else if maxScore > 0 {
		// All scores identical and positive: normalize to 1.0
		for k := range scores {
			scores[k] = 1.0
		}
	}

	return scores
}
