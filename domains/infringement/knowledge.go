package infringement

import "context"

// KnowledgeRetriever supplies examination guidelines and similar cases
// to enrich infringement analysis inputs. Following the same pattern as
// domains/enablement/types.go.
type KnowledgeRetriever interface {
	// SearchGuidelines retrieves relevant examination guideline sections.
	SearchGuidelines(ctx context.Context, query string) ([]GuidelineRef, error)

	// SearchSimilarCases retrieves relevant judicial cases.
	SearchSimilarCases(ctx context.Context, query string) ([]CaseRef, error)

	// SearchLegalProvisions retrieves full text of specified legal articles.
	SearchLegalProvisions(ctx context.Context, articles []string) ([]LegalProvision, error)
}

// LegalProvision is a full-text legal article.
type LegalProvision struct {
	Article string `json:"article"`
	Law     string `json:"law"`
	Content string `json:"content"`
}
