package risk

import (
	"context"
	"maps"
	"sort"
	"strings"

	"github.com/xujian519/mady/knowledge"
)

// StoreCaseSearcher wraps a knowledge.Store as a CaseSearcher.
// It searches patent-domain documents and filters by reexam/judgment type.
type StoreCaseSearcher struct {
	store *knowledge.Store
}

// NewStoreCaseSearcher creates a searcher backed by the knowledge store.
func NewStoreCaseSearcher(store *knowledge.Store) *StoreCaseSearcher {
	return &StoreCaseSearcher{store: store}
}

// SearchCases implements CaseSearcher.
// It searches patent-domain documents whose metadata or content matches
// the given feature tags, prioritizing reexam/judgment-type documents.
func (s *StoreCaseSearcher) SearchCases(ctx context.Context, features []string, maxResults int) ([]CaseResult, error) {
	if s.store == nil {
		return nil, nil
	}
	allIDs := s.store.AllDocIDs()
	if len(allIDs) == 0 {
		return nil, nil
	}

	var results []CaseResult

	for _, id := range allIDs {
		doc, ok := s.store.GetDocument(id)
		if !ok || doc == nil {
			continue
		}
		// Only search patent-domain documents.
		if doc.Domain != "patent" {
			continue
		}
		// Skip non-searchable documents (index/directory/fragment).
		if !doc.Searchable {
			continue
		}
		// Only reexam/judgment type documents.
		docType := doc.Metadata["type"]
		if docType != "reexam" && docType != "judgment" && docType != "case" {
			continue
		}

		score := scoreDocument(doc, features)
		if score <= 0 {
			continue
		}
		// 深拷贝 metadata 以避免引用 Store 内部 map 导致数据污染。
		metaCopy := make(map[string]string, len(doc.Metadata))
		maps.Copy(metaCopy, doc.Metadata)
		results = append(results, CaseResult{
			DocID:    doc.ID,
			Title:    doc.Title,
			DocType:  docType,
			Score:    score,
			Metadata: metaCopy,
		})
	}

	// Sort by score descending.
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	if results == nil {
		return []CaseResult{}, nil
	}
	return results, nil
}

// scoreDocument computes a relevance score for a document against feature tags.
// Returns 0 if no features match.
func scoreDocument(doc *knowledge.Document, features []string) float64 {
	if doc == nil || doc.Metadata == nil {
		return 0
	}
	score := 0.0
	matchedAny := false

	for _, f := range features {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// Title match: +3 (highest weight).
		if strings.Contains(doc.Title, f) {
			score += 3.0
			matchedAny = true
		}
		// Tags match: +2.
		if tags, ok := doc.Metadata["tags"]; ok && strings.Contains(tags, f) {
			score += 2.0
			matchedAny = true
		}
		// Keywords match: +1.5.
		if kw, ok := doc.Metadata["keywords"]; ok && strings.Contains(kw, f) {
			score += 1.5
			matchedAny = true
		}
		// Law refs match: +1.
		if lr, ok := doc.Metadata["law_refs"]; ok && strings.Contains(lr, f) {
			score += 1.0
			matchedAny = true
		}
		// Content match (first 500 chars): +0.5.
		if len(doc.Content) > 0 {
			contentPreview := doc.Content
			if len(contentPreview) > 500 {
				contentPreview = contentPreview[:500]
			}
			if strings.Contains(contentPreview, f) {
				score += 0.5
				matchedAny = true
			}
		}
	}
	if !matchedAny {
		return 0
	}
	// Boost reexam-type documents.
	if docType := doc.Metadata["type"]; docType == "reexam" {
		score *= 1.2
	}
	// Boost documents with decision_count metadata.
	if dc, ok := doc.Metadata["decision_count"]; ok && dc != "" {
		score *= 1.1
	}
	return score
}

var _ CaseSearcher = (*StoreCaseSearcher)(nil)
