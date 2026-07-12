package collector

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/xujian519/mady/domains/reasoning"
)

// DerivedCollector uses LLM reasoning to derive new facts from existing
// facts on the blackboard. It runs after the other 3 collectors have
// populated the blackboard with source facts.
//
// This collector is optional — it is only useful when the source facts
// imply conclusions that are not explicitly stated (e.g. a combination
// of features implies a technical effect).
type DerivedCollector struct {
	llm      LLMClient
	maxFacts int
}

// NewDerivedCollector creates a derived-fact collector.
func NewDerivedCollector(llm LLMClient, maxFacts int) *DerivedCollector {
	if maxFacts <= 0 {
		maxFacts = 10
	}
	return &DerivedCollector{llm: llm, maxFacts: maxFacts}
}

func (c *DerivedCollector) ID() reasoning.FactCollectorID {
	return reasoning.CollectorDerived
}

func (c *DerivedCollector) Collect(ctx context.Context, input string, bb *reasoning.FactBlackboard) (*reasoning.CollectResult, error) {
	result := &reasoning.CollectResult{
		CollectorID: reasoning.CollectorDerived,
	}

	if c.llm == nil {
		result.Gaps = []string{"LLM 未配置，跳过衍生推理"}
		return result, nil
	}

	// Collect existing active facts to reason from.
	existingFacts := bb.ActiveFacts()
	if len(existingFacts) == 0 {
		result.Gaps = []string{"没有可用的源事实进行衍生推理"}
		return result, nil
	}

	var factsText strings.Builder
	for _, f := range existingFacts {
		fmt.Fprintf(&factsText, "- [%s] %s\n", f.ID, truncate(f.Content, 200))
	}

	prompt := fmt.Sprintf(`基于以下已知事实，推理出可能的衍生事实（这些事实不是直接陈述的，但可以从已知事实中合理推断）。
每条衍生事实一行，格式为 "推理|事实内容|置信度(0-1)"。
最多产生 %d 条，只输出有合理依据的推断。

已知事实：
%s

衍生事实列表：`, c.maxFacts, factsText.String())

	resp, err := c.llm.Chat(ctx, prompt)
	if err != nil {
		result.Gaps = []string{fmt.Sprintf("LLM 衍生推理失败: %v", err)}
		return result, nil
	}

	lines := strings.Split(strings.TrimSpace(resp), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "推理|") {
			continue
		}
		if result.FactCount >= c.maxFacts {
			break
		}

		content, conf := parseDerivedLine(line)
		factID := fmt.Sprintf("derived_%s_%03d", bb.CaseID, i+1)
		bb.AddFact(reasoning.FactEntry{
			ID:          factID,
			Source:      "llm_derived",
			Content:     truncate(content, 1000),
			Confidence:  conf,
			ExtractedAt: reasoning.NowISO(),
			CollectorID: reasoning.CollectorDerived,
			Category:    reasoning.FactCategoryTechnical,
		})
		result.FactCount++
	}

	result.Confidence = 0.6 // Derived facts are inherently less certain.
	if result.FactCount == 0 {
		result.Gaps = []string{"未能从已知事实衍生出新事实"}
	}
	return result, nil
}

// parseDerivedLine parses "推理|内容|0.8" format.
func parseDerivedLine(line string) (content string, confidence float64) {
	parts := strings.SplitN(line, "|", 3)
	content = line
	confidence = 0.6
	if len(parts) >= 2 {
		content = strings.TrimSpace(parts[1])
	}
	if len(parts) >= 3 {
		if v, err := parseFloat(strings.TrimSpace(parts[2])); err == nil {
			confidence = v
		}
	}
	// Clamp confidence to [0, 1].
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}
	return
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
