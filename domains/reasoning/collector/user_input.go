package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/reasoning"
)

// UserInputCollector extracts key facts from the user's natural-language input.
// It uses an LLM to identify and structure factual claims.
type UserInputCollector struct {
	llm      LLMClient
	maxFacts int
}

// NewUserInputCollector creates a user-input fact collector.
// llm may be nil — in that case, the input is treated as a single fact.
func NewUserInputCollector(llm LLMClient, maxFacts int) *UserInputCollector {
	if maxFacts <= 0 {
		maxFacts = 10
	}
	return &UserInputCollector{llm: llm, maxFacts: maxFacts}
}

func (c *UserInputCollector) ID() reasoning.FactCollectorID {
	return reasoning.CollectorUserInput
}

func (c *UserInputCollector) Collect(ctx context.Context, input string, bb *reasoning.FactBlackboard) (*reasoning.CollectResult, error) {
	result := &reasoning.CollectResult{
		CollectorID: reasoning.CollectorUserInput,
		Confidence:  1.0,
	}

	if input == "" {
		result.Gaps = []string{"用户输入为空"}
		return result, nil
	}

	// With LLM: extract individual facts.
	if c.llm != nil {
		return c.collectWithLLM(ctx, input, bb, result)
	}

	// Fallback: treat the entire input as one fact.
	factID := fmt.Sprintf("ui_%s_001", bb.CaseID)
	bb.AddFact(reasoning.FactEntry{
		ID:          factID,
		Source:      "user_text",
		Content:     truncate(input, 2000),
		Confidence:  1.0,
		ExtractedAt: reasoning.NowISO(),
		CollectorID: reasoning.CollectorUserInput,
		Category:    reasoning.FactCategoryTechnical,
	})
	result.FactCount = 1
	return result, nil
}

func (c *UserInputCollector) collectWithLLM(ctx context.Context, input string, bb *reasoning.FactBlackboard, result *reasoning.CollectResult) (*reasoning.CollectResult, error) {
	prompt := fmt.Sprintf(`请从以下用户输入中提取关键事实，每条事实一行，格式为 "分类|事实内容"。
分类包括：technical(技术特征)、legal(法律要件)、procedural(程序事实)、temporal(时间期限)
最多提取 %d 条事实。

用户输入：
%s

请直接输出事实列表，每行一条：`, c.maxFacts, input)

	resp, err := c.llm.Chat(ctx, prompt)
	if err != nil {
		// LLM failed → fallback to single fact.
		factID := fmt.Sprintf("ui_%s_001", bb.CaseID)
		bb.AddFact(reasoning.FactEntry{
			ID:          factID,
			Source:      "user_text",
			Content:     truncate(input, 2000),
			Confidence:  0.8,
			ExtractedAt: reasoning.NowISO(),
			CollectorID: reasoning.CollectorUserInput,
			Category:    reasoning.FactCategoryTechnical,
		})
		result.FactCount = 1
		result.Confidence = 0.8
		return result, nil
	}

	lines := strings.Split(strings.TrimSpace(resp), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if result.FactCount >= c.maxFacts {
			break
		}

		category, content := parseCategoryLine(line)
		factID := fmt.Sprintf("ui_%s_%03d", bb.CaseID, i+1)
		bb.AddFact(reasoning.FactEntry{
			ID:          factID,
			Source:      "user_text",
			Content:     truncate(content, 1000),
			Confidence:  0.9,
			ExtractedAt: reasoning.NowISO(),
			CollectorID: reasoning.CollectorUserInput,
			Category:    category,
		})
		result.FactCount++
	}

	if result.FactCount == 0 {
		result.Gaps = []string{"LLM 未能提取到有效事实"}
	}
	return result, nil
}

// parseCategoryLine parses "category|content" or plain content.
func parseCategoryLine(line string) (reasoning.FactCategory, string) {
	parts := strings.SplitN(line, "|", 2)
	if len(parts) < 2 {
		return reasoning.FactCategoryTechnical, line
	}
	cat := strings.TrimSpace(parts[0])
	content := strings.TrimSpace(parts[1])

	switch cat {
	case "legal":
		return reasoning.FactCategoryLegal, content
	case "procedural":
		return reasoning.FactCategoryProcedural, content
	case "temporal":
		return reasoning.FactCategoryTemporal, content
	default:
		return reasoning.FactCategoryTechnical, content
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
