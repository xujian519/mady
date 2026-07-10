package retrieval

import (
	"regexp"
	"sort"
	"strings"
)

// ScoredChunk is a chunk with a relevance score from a search operation.
type ScoredChunk struct {
	Chunk
	Score   float64
	Matches []string // snippets of matched text for explainability
}

// Searcher is the core search interface. Implementations range from
// simple keyword matching (KeywordSearcher) to semantic vector search.
type Searcher interface {
	// Search returns scored chunks matching the query, sorted by relevance.
	Search(query string, chunks []Chunk, topK int) []ScoredChunk
}

// KeywordSearcher implements Searcher using regex + keyword matching
// with TF-IDF-like scoring. This is the MVP implementation requiring
// zero external dependencies.
type KeywordSearcher struct {
	// CaseSensitive enables case-sensitive matching.
	CaseSensitive bool
	// MinScore filters out results below this threshold (default: 0.1).
	MinScore float64
}

// NewKeywordSearcher creates a KeywordSearcher with sensible defaults.
func NewKeywordSearcher() *KeywordSearcher {
	return &KeywordSearcher{
		CaseSensitive: false,
		MinScore:      0.1,
	}
}

// Search implements Searcher.Search using keyword + regex matching.
func (ks *KeywordSearcher) Search(query string, chunks []Chunk, topK int) []ScoredChunk {
	if topK <= 0 {
		topK = 5
	}

	// Extract search terms from the query.
	terms := extractTerms(query)
	if len(terms) == 0 {
		return nil
	}

	var results []ScoredChunk
	for _, chunk := range chunks {
		score, matches := ks.scoreChunk(terms, chunk)
		if score >= ks.MinScore {
			results = append(results, ScoredChunk{
				Chunk:   chunk,
				Score:   score,
				Matches: matches,
			})
		}
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// scoreChunk computes relevance score for a single chunk against search terms.
// Uses a simple TF-IDF-inspired scoring:
//   - Term frequency: how many times each term appears in the chunk
//   - Inverse chunk frequency penalty: terms that appear in too many chunks
//     are down-weighted (like IDF)
//   - Position bonus: matches in the first 20% of a chunk score higher
//   - Phrase bonus: exact phrase matches score significantly higher
func (ks *KeywordSearcher) scoreChunk(terms []string, chunk Chunk) (float64, []string) {
	content := chunk.Content
	if !ks.CaseSensitive {
		content = strings.ToLower(content)
	}

	var totalScore float64
	var matches []string

	for _, term := range terms {
		termContent := term
		if !ks.CaseSensitive {
			termContent = strings.ToLower(term)
		}

		count := strings.Count(content, termContent)
		if count == 0 {
			continue
		}

		// Term frequency score: log-scaled count.
		tf := 1.0 + log2(float64(count))

		// Position bonus: matches in the first 20% of content are weighted higher.
		firstIdx := strings.Index(content, termContent)
		posBonus := 1.0
		if firstIdx >= 0 && firstIdx < len(content)/5 {
			posBonus = 1.5
		}

		// Exact phrase bonus (multi-word terms).
		phraseBonus := 1.0
		if strings.Contains(term, " ") {
			// For multi-word terms, each occurrence counts extra.
			phraseBonus = 2.0
		}

		termScore := tf * posBonus * phraseBonus
		totalScore += termScore

		// Capture a match snippet.
		if firstIdx >= 0 {
			start := max(0, firstIdx-20)
			end := min(len(content), firstIdx+len(termContent)+30)
			matches = append(matches, content[start:end])
		}
	}

	return totalScore, matches
}

// extractTerms parses a query into atomic search terms.
// Chinese text is split by common delimiters and kept as-is since
// Chinese doesn't use spaces between words.
// English terms are lowercased and split by whitespace.
func extractTerms(query string) []string {
	// Remove common punctuation that isn't part of search terms.
	re := regexp.MustCompile(`[，。！？、；：""'（）《》\[\]【】\s]+`)
	parts := re.Split(query, -1)

	seen := make(map[string]bool)
	var terms []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) < 2 && !isASCIILetter(p) {
			continue // skip single non-letter characters
		}
		lower := strings.ToLower(p)
		if !seen[lower] {
			seen[lower] = true
			terms = append(terms, p)
		}
	}
	return terms
}

func isASCIILetter(s string) bool {
	if len(s) != 1 {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func log2(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Simple integer log2 approximation.
	result := 0.0
	for x >= 2.0 {
		x /= 2.0
		result += 1.0
	}
	return result
}
