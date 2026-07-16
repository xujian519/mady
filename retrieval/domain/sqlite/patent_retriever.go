// Package sqlite provides the first concrete implementation of
// domain.DomainRetriever, backed by the local knowledge.db SQLite store.
//
// It lives in a subpackage of retrieval/domain so the parent package keeps
// its current dependency boundary: retrieval/domain depends only on the
// knowledge abstraction (and retrieval), never on knowledge/sqlite. This
// sqlite subpackage is the composition seam that binds the concrete store.
package sqlite

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/xujian519/mady/knowledge/sqlite"
	"github.com/xujian519/mady/retrieval"
	"github.com/xujian519/mady/retrieval/domain"
)

// SourceNamePatent is the SourceName reported by PatentDomainRetriever.
const SourceNamePatent = "本地专利知识库(knowledge.db)"

// PatentDomainRetriever implements domain.DomainRetriever over a local
// knowledge.db store (FTS full-text search). It is the concrete retriever
// backing the retrieve_prior_art disclosure node and any patent-domain
// retrieval that operates on the already-indexed xiaonuo corpus.
//
// Search is keyword/phrase based via SQLite FTS5 (trigram tokenizer); no
// embedder is required, so it works in FTS-only configurations. Scores are
// bm25-derived and normalized to (0,1] so downstream rerankers/RRF fusion
// can treat them uniformly with vector scores.
type PatentDomainRetriever struct {
	store *sqlite.SQLiteStore
}

// compile-time interface satisfaction check.
var _ domain.DomainRetriever = (*PatentDomainRetriever)(nil)

// NewPatentDomainRetriever binds a knowledge.db store. Returns nil if store
// is nil so callers can let a nil retriever disable the patent lane.
func NewPatentDomainRetriever(store *sqlite.SQLiteStore) *PatentDomainRetriever {
	if store == nil {
		return nil
	}
	return &PatentDomainRetriever{store: store}
}

// SourceName returns the human-readable data source identifier.
func (r *PatentDomainRetriever) SourceName() string {
	return SourceNamePatent
}

// Search queries the patent knowledge corpus via FTS5 and maps hits to
// DomainDocuments. query.Text is the primary query; each Keyword is issued
// as an additional query and results are merged + deduplicated by chunk ID.
//
// FTS5 (via FTSSearch) wraps each query as a phrase match, so multi-term
// queries cannot be naively space-joined into one string — that would
// require all terms to appear contiguously. Instead each term is queried
// separately and the union is taken, which gives OR semantics over the
// corpus. MaxResults caps the final merged list (default 10).
func (r *PatentDomainRetriever) Search(ctx context.Context, query domain.DomainQuery) (*domain.DomainResults, error) {
	if r == nil {
		return &domain.DomainResults{Query: query, Source: SourceNamePatent}, nil
	}
	terms := buildQueryTerms(query)
	if len(terms) == 0 {
		return &domain.DomainResults{Query: query, Source: SourceNamePatent}, nil
	}
	topK := query.MaxResults
	if topK <= 0 {
		topK = 10
	}

	// Query each term, merge by chunk ID (first occurrence wins, keeping the
	// highest score seen for that chunk).
	merged := make(map[string]retrieval.ScoredChunk, topK*len(terms))
	var rawMax float64
	for _, term := range terms {
		chunks, err := r.store.FTSSearch(term, topK)
		if err != nil {
			return nil, fmt.Errorf("patent domain search (%q): %w", term, err)
		}
		for _, c := range chunks {
			if c.Score > rawMax {
				rawMax = c.Score
			}
			if _, exists := merged[c.ID]; !exists {
				merged[c.ID] = c
			} else if c.Score > merged[c.ID].Score {
				merged[c.ID] = c
			}
		}
	}

	docs := make([]domain.DomainDocument, 0, len(merged))
	for _, c := range merged {
		docs = append(docs, chunkToDocument(c, normalizeScore(c.Score, rawMax)))
	}
	// Sort by normalized score descending so downstream consumers (rerankers,
	// RRF fusion, evidence selection) get the strongest hits first, matching
	// the DomainRetriever contract that Documents are "ranked results".
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})
	// Cap at MaxResults (merged set may exceed a single term's topK).
	if len(docs) > topK {
		docs = docs[:topK]
	}
	return &domain.DomainResults{
		Query:      query,
		Documents:  docs,
		TotalCount: len(docs),
		Source:     SourceNamePatent,
	}, nil
}

// GetDocument fetches a single document by ID, concatenating its chunks into
// one DomainDocument. Used when a Search hit needs to be expanded to full
// text for evidence-span anchoring. Returns nil (no error) when the ID has
// no chunks — matching the DomainRetriever convention that a missing
// document is a normal "not found" rather than a failure.
func (r *PatentDomainRetriever) GetDocument(ctx context.Context, docID string) (*domain.DomainDocument, error) {
	if r == nil || docID == "" {
		return nil, nil
	}
	chunks, err := r.store.GetChunksByDocID(docID, 50)
	if err != nil {
		return nil, fmt.Errorf("patent domain get document: %w", err)
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	var b strings.Builder
	title := ""
	for i, c := range chunks {
		if i == 0 {
			if h, ok := c.Metadata["heading"]; ok && h != "" {
				title = h
			}
		}
		b.WriteString(c.Content)
		b.WriteByte('\n')
	}
	body := strings.TrimSpace(b.String())
	doc := &domain.DomainDocument{
		ID:       docID,
		Title:    firstNonEmpty(title, docID),
		Snippet:  truncate(body, 300),
		Content:  body,
		Metadata: map[string]string{"source": SourceNamePatent},
	}
	return doc, nil
}

// buildQueryTerms returns the deduplicated set of query terms to issue
// against FTS5: the natural-language Text first, then each Keyword. Empty
// terms are dropped. Order is preserved so the primary Text term is queried
// first (its hits dominate the merge on ties).
func buildQueryTerms(q domain.DomainQuery) []string {
	seen := make(map[string]bool)
	terms := []string{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		terms = append(terms, s)
	}
	add(q.Text)
	for _, k := range q.Keywords {
		add(k)
	}
	return terms
}

// chunkToDocument maps one FTS hit to a DomainDocument. The chunk's heading
// metadata is promoted to Title when present; the full chunk Content becomes
// both Snippet (truncated) and Content (full), so downstream consumers can
// anchor evidence spans to the original wording.
func chunkToDocument(c retrieval.ScoredChunk, score float64) domain.DomainDocument {
	title := c.DocID
	if h, ok := c.Metadata["heading"]; ok && h != "" {
		title = h
	}
	return domain.DomainDocument{
		ID:      c.DocID,
		Title:   title,
		Snippet: truncate(c.Content, 300),
		Content: c.Content,
		Metadata: map[string]string{
			"heading":    c.Metadata["heading"],
			"chunk_type": "section",
			"chunk_id":   c.ID,
			"position":   fmt.Sprintf("%d", c.Position),
		},
		Score: score,
	}
}

// normalizeScore scales a raw bm25 score into (0,1] by dividing by the max
// in the result set. A zero max (all-zero scores) yields uniform 0 so
// downstream code can treat 0 as "no signal"; any positive score maps to
// (0,1]. This keeps relative ordering intact for RRF fusion.
func normalizeScore(raw, max float64) float64 {
	if max <= 0 {
		return 0
	}
	s := raw / max
	if math.IsNaN(s) || math.IsInf(s, 0) {
		return 0
	}
	return s
}

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
