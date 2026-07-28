package retrieval

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
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
	Search(ctx context.Context, query string, chunks []Chunk, topK int) []ScoredChunk
}

var _ Searcher = (*KeywordSearcher)(nil)

// KeywordSearcher implements Searcher using regex + keyword matching
// with TF-IDF-like scoring. This is the MVP implementation requiring
// zero external dependencies.
//
// When an InvertedIndex is pre-built (via SetIndex), Search uses the
// index path instead of O(N) full scan — reducing query time from
// O(N*T) to O(term_postings) for large chunk sets.
type KeywordSearcher struct {
	// CaseSensitive enables case-sensitive matching.
	CaseSensitive bool
	// MinScore filters out results below this threshold (default: 0.1).
	MinScore float64

	// index is an optional pre-built inverted index. When set via SetIndex,
	// Search uses the index path. Protected by atomic.Pointer for lock-free
	// concurrent reads during pointer swap.
	index atomic.Pointer[InvertedIndex]
}

// NewKeywordSearcher creates a KeywordSearcher with sensible defaults.
func NewKeywordSearcher() *KeywordSearcher {
	return &KeywordSearcher{
		CaseSensitive: false,
		MinScore:      0.1,
	}
}

// SetIndex sets or clears (nil) the inverted index. Call this after loading
// chunks to enable the index path. The index is used by all subsequent
// Search calls until replaced or cleared. Thread-safe via atomic.Pointer.
func (ks *KeywordSearcher) SetIndex(idx *InvertedIndex) {
	ks.index.Store(idx)
}

// --- InvertedIndex ---

// InvertedIndex is a pre-built term-to-chunk index for efficient keyword search.
// It eliminates the O(N) full scan of KeywordSearcher by looking up query terms'
// postings lists — only chunks matching at least one term are scored.
//
// Build with BuildInvertedIndex after loading chunks, then pass to
// KeywordSearcher via SetIndex. Rebuild when chunks change.
type InvertedIndex struct {
	// postings maps each term (lowercase) to a list of (chunk index, frequency).
	postings map[string][]postingEntry
	// chunks holds the original chunk slice (indexed by position).
	chunks []Chunk
	// idf precomputes inverse document frequency for each term.
	idf map[string]float64
	// totalChunks is N in the IDF formula.
	totalChunks int
	// minTokenLen is the minimum token length to index (default: 2).
	// Shorter tokens produce too many false positives in CJK text.
	minTokenLen int
}

// postingEntry records one chunk's match for a term.
type postingEntry struct {
	ChunkIdx int // index into InvertedIndex.chunks
	Count    int // term frequency in this chunk
}

// BuildInvertedIndex builds an inverted index from a set of chunks.
// Each chunk's content is tokenized; terms are deduplicated and their
// frequencies counted. Terms shorter than minTokenLen characters are
// skipped to avoid noise. Use the returned index with KeywordSearcher.SetIndex.
//
// Memory: for 1000 chunks × ~100 terms each = ~100K postings entries,
// roughly 2-4 MB (map overhead + struct slices). The chunks themselves
// are not copied — the index holds a reference to the original slice.
func BuildInvertedIndex(chunks []Chunk) *InvertedIndex {
	idx := &InvertedIndex{
		postings:    make(map[string][]postingEntry),
		chunks:      chunks,
		idf:         make(map[string]float64),
		totalChunks: len(chunks),
		minTokenLen: 2,
	}
	if len(chunks) == 0 {
		return idx
	}

	// Phase 1: build term→chunk frequency map.
	for ci, chunk := range chunks {
		tokens := indexTokens(chunk.Content)
		if len(tokens) == 0 {
			continue
		}
		seen := make(map[string]bool, len(tokens))
		tc := make(map[string]int, len(tokens))
		for _, tok := range tokens {
			if len(tok) < idx.minTokenLen {
				continue
			}
			seen[tok] = true
			tc[tok]++
		}
		for term := range seen {
			idx.postings[term] = append(idx.postings[term], postingEntry{ChunkIdx: ci, Count: tc[term]})
		}
	}

	// Phase 2: compute IDF for each term.
	for term, postings := range idx.postings {
		df := len(postings) // document frequency
		idx.idf[term] = math.Log2(1.0 + float64(idx.totalChunks)/float64(df))
	}

	return idx
}

// indexTokens tokenizes content for indexing. For CJK-dominant text, it
// extracts 2-grams; for ASCII-dominant text, it splits on whitespace/punctuation.
// 2-grams are sufficient for CJK partial matching — longer n-grams would index
// O(L²) substrings per token and add index bloat without recall benefit.
func indexTokens(content string) []string {
	content = strings.ToLower(content)
	re := regexp.MustCompile(`[^a-zA-Z0-9\p{Han}]+`)
	parts := re.Split(content, -1)

	var tokens []string
	seen := make(map[string]bool)

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// For CJK-dominant tokens, extract 2-grams.
		if isCJKToken(p) {
			runes := []rune(p)
			for i := 0; i+2 <= len(runes); i++ {
				gram := string(runes[i : i+2])
				if !seen[gram] {
					seen[gram] = true
					tokens = append(tokens, gram)
				}
			}
		} else if !seen[p] {
			seen[p] = true
			tokens = append(tokens, p)
		}
	}
	return tokens
}

// isCJKToken reports whether the string contains primarily CJK characters.
func isCJKToken(s string) bool {
	cjk := 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF || r >= 0x3400 && r <= 0x4DBF {
			cjk++
		}
	}
	return cjk > 0
}

// --- KeywordSearcher.Search with index support ---

// Search implements Searcher.Search. When an inverted index is set (via
// SetIndex), it uses the index path for O(term_postings) performance.
// Otherwise it falls back to the original O(N) full scan.
//
// When the index is set, the chunks parameter is ignored — the index
// contains its own chunk reference. Pass nil or a valid chunk slice;
// either way the index's chunks are used.
func (ks *KeywordSearcher) Search(ctx context.Context, query string, chunks []Chunk, topK int) []ScoredChunk {
	if topK <= 0 {
		topK = 5
	}

	terms := extractTerms(query)
	if len(terms) == 0 {
		return nil
	}

	// Index path: use inverted index for fast lookup.
	idx := ks.index.Load()
	if idx != nil {
		return ks.indexSearch(idx, terms, topK)
	}

	// Fallback: original O(N) full scan.
	return ks.fullScanSearch(terms, chunks, topK)
}

// indexSearch uses the inverted index for efficient keyword search.
func (ks *KeywordSearcher) indexSearch(idx *InvertedIndex, terms []string, topK int) []ScoredChunk {

	// Accumulate scores per chunk using postings lists.
	type acc struct {
		score float64
		nb    int // number of terms matched (for averaging)
	}
	accum := make(map[int]*acc)
	matchedTerms := make(map[int][]string)

	for _, term := range terms {
		lowerTerm := strings.ToLower(term)
		postings, ok := idx.postings[lowerTerm]
		if !ok {
			continue
		}
		idf := idx.idf[lowerTerm]
		for _, pe := range postings {
			a, exists := accum[pe.ChunkIdx]
			if !exists {
				a = &acc{}
				accum[pe.ChunkIdx] = a
			}
			// TF-IDF score: (1 + log2(tf)) * idf
			tf := 1.0 + math.Log2(float64(pe.Count+1))
			a.score += tf * idf
			a.nb++
			matchedTerms[pe.ChunkIdx] = append(matchedTerms[pe.ChunkIdx], term)
		}
	}

	if len(accum) == 0 {
		return nil
	}

	// Build results with position bonus.
	minScore := ks.MinScore
	if minScore <= 0 {
		minScore = 0.1
	}
	cse := ks.CaseSensitive

	var results []ScoredChunk
	for ci, a := range accum {
		if a.score < minScore {
			continue
		}
		chunk := idx.chunks[ci]

		// Position bonus: matches in the first 20% of content score higher.
		content := chunk.Content
		if !cse {
			content = strings.ToLower(content)
		}
		firstTerm := strings.ToLower(terms[0])
		posBonus := 1.0
		if firstIdx := strings.Index(content, firstTerm); firstIdx >= 0 && firstIdx < len(content)/5 {
			posBonus = 1.5
		}

		// Phrase bonus: multi-word terms score higher.
		phraseBonus := 1.0
		for _, t := range terms {
			if strings.Contains(t, " ") && strings.Contains(content, strings.ToLower(t)) {
				phraseBonus = 2.0
				break
			}
		}

		finalScore := a.score * posBonus * phraseBonus
		results = append(results, ScoredChunk{
			Chunk:   chunk,
			Score:   finalScore,
			Matches: matchedTerms[ci],
		})
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// fullScanSearch is the original O(N) full scan implementation.
func (ks *KeywordSearcher) fullScanSearch(terms []string, chunks []Chunk, topK int) []ScoredChunk {
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

	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
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
//
// Scoring formula (must match indexSearch for consistent ranking):
//
//	tf = 1 + log2(count + 1); score = Σ tf * idf * posBonus * phraseBonus
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

		// Term frequency score: log-scaled count (must match indexSearch).
		tf := 1.0 + math.Log2(float64(count+1))

		// Position bonus: matches in the first 20% of content are weighted higher.
		firstIdx := strings.Index(content, termContent)
		posBonus := 1.0
		if firstIdx >= 0 && firstIdx < len(content)/5 {
			posBonus = 1.5
		}

		// Exact phrase bonus (multi-word terms).
		phraseBonus := 1.0
		if strings.Contains(term, " ") {
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
	re := regexp.MustCompile(`[，。！？、；："“”'（）《》\[\]【】\s]+`)
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
