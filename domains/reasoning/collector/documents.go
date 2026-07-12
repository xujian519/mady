package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/reasoning"
)

// DocumentCollector reads files and extracts facts from their content.
// It delegates to a DocReader for file I/O and optionally uses an LLM
// for structured fact extraction.
type DocumentCollector struct {
	reader   DocReader
	llm      LLMClient
	maxFacts int
}

// NewDocumentCollector creates a document fact collector.
func NewDocumentCollector(reader DocReader, llm LLMClient, maxFacts int) *DocumentCollector {
	if maxFacts <= 0 {
		maxFacts = 20
	}
	return &DocumentCollector{reader: reader, llm: llm, maxFacts: maxFacts}
}

func (c *DocumentCollector) ID() reasoning.FactCollectorID {
	return reasoning.CollectorDocuments
}

func (c *DocumentCollector) Collect(ctx context.Context, input string, bb *reasoning.FactBlackboard) (*reasoning.CollectResult, error) {
	result := &reasoning.CollectResult{
		CollectorID: reasoning.CollectorDocuments,
	}

	if c.reader == nil {
		result.Gaps = []string{"文档读取器未配置"}
		return result, nil
	}

	// input may be a file path or raw document content.
	// Try to read it as a file first; if it fails, treat as inline content.
	text, err := c.reader.ReadText(ctx, input)
	if err != nil {
		// Fallback: treat the input itself as document content.
		text = input
	}
	if text == "" {
		result.Gaps = []string{"文档内容为空"}
		return result, nil
	}

	if c.llm != nil {
		return c.collectWithLLM(ctx, text, bb, result)
	}

	// No LLM: store as a single document fact.
	factID := fmt.Sprintf("doc_%s_001", bb.CaseID)
	bb.AddFact(reasoning.FactEntry{
		ID:          factID,
		Source:      "file",
		Content:     truncate(text, 2000),
		FilePath:    input,
		Confidence:  0.9,
		ExtractedAt: reasoning.NowISO(),
		CollectorID: reasoning.CollectorDocuments,
		Category:    reasoning.FactCategoryTechnical,
		ArtifactRef: input,
	})
	result.FactCount = 1
	return result, nil
}

func (c *DocumentCollector) collectWithLLM(ctx context.Context, text string, bb *reasoning.FactBlackboard, result *reasoning.CollectResult) (*reasoning.CollectResult, error) {
	prompt := fmt.Sprintf(`请从以下技术文档中提取关键事实。每条事实一行，格式为 "分类|事实内容"。
分类包括：technical(技术特征)、legal(法律要件)、procedural(程序事实)、temporal(时间期限)
最多提取 %d 条事实。关注可专利性相关的技术细节。

文档内容：
%s

请直接输出事实列表：`, c.maxFacts, truncate(text, 6000))

	resp, err := c.llm.Chat(ctx, prompt)
	if err != nil {
		return result, fmt.Errorf("LLM fact extraction: %w", err)
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
		factID := fmt.Sprintf("doc_%s_%03d", bb.CaseID, i+1)
		bb.AddFact(reasoning.FactEntry{
			ID:          factID,
			Source:      "file",
			Content:     truncate(content, 1000),
			Confidence:  0.85,
			ExtractedAt: reasoning.NowISO(),
			CollectorID: reasoning.CollectorDocuments,
			Category:    category,
		})
		result.FactCount++
	}

	if result.FactCount == 0 {
		result.Gaps = []string{"LLM 未能从文档中提取到关键事实"}
	}
	return result, nil
}
