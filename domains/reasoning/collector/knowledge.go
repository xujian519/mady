package collector

import (
	"context"
	"fmt"

	"github.com/xujian519/mady/domains/reasoning"
)

// KnowledgeCollector retrieves relevant background facts from the knowledge
// graph. It queries the KG with keywords extracted from the user input and
// existing blackboard facts.
type KnowledgeCollector struct {
	store    KnowledgeStore
	maxFacts int
}

// NewKnowledgeCollector creates a knowledge-graph fact collector.
func NewKnowledgeCollector(store KnowledgeStore, maxFacts int) *KnowledgeCollector {
	if maxFacts <= 0 {
		maxFacts = 10
	}
	return &KnowledgeCollector{store: store, maxFacts: maxFacts}
}

func (c *KnowledgeCollector) ID() reasoning.FactCollectorID {
	return reasoning.CollectorKnowledge
}

func (c *KnowledgeCollector) Collect(ctx context.Context, input string, bb *reasoning.FactBlackboard) (*reasoning.CollectResult, error) {
	result := &reasoning.CollectResult{
		CollectorID: reasoning.CollectorKnowledge,
	}

	if c.store == nil {
		result.Gaps = []string{"知识库未配置"}
		return result, nil
	}

	// Build a query from the input and existing blackboard facts.
	query := c.buildQuery(input, bb)

	facts, err := c.store.SearchFacts(ctx, query, c.maxFacts)
	if err != nil {
		result.Gaps = []string{fmt.Sprintf("知识库查询失败: %v", err)}
		return result, nil
	}

	for i, f := range facts {
		factID := fmt.Sprintf("kg_%s_%03d", bb.CaseID, i+1)
		bb.AddFact(reasoning.FactEntry{
			ID:          factID,
			Source:      "knowledge_graph",
			Content:     truncate(f.Content, 1500),
			Confidence:  f.Confidence,
			ExtractedAt: reasoning.NowISO(),
			CollectorID: reasoning.CollectorKnowledge,
			Category:    reasoning.FactCategoryTechnical,
			Tags:        []string{f.Source},
		})
		result.FactCount++
	}

	if result.FactCount == 0 {
		result.Gaps = []string{"知识库未找到相关事实"}
		result.Confidence = 0
	} else {
		result.Confidence = 0.7
	}

	return result, nil
}

// buildQuery constructs a search query from input and existing facts.
func (c *KnowledgeCollector) buildQuery(input string, bb *reasoning.FactBlackboard) string {
	// Combine the input with technical-field context.
	q := input
	if bb.TechnicalField != "" {
		q = bb.TechnicalField + " " + q
	}
	return truncate(q, 500)
}
