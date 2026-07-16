package disclosure

import (
	"context"
	"strings"

	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// retrievePriorArtNode 返回现有技术检索的 Pregel 节点（对齐
// docs/specs/design-prior-art-retrieval-stage.md 第三节）。
//
// 节点职责：以 generate_keywords 产出的检索关键词 + merge 后的技术特征为
// 锚点，查询专利领域检索器，产出带 DocID/原文片段/相似度的 EvidenceChunk
// 列表，写入 state 供 check_novelty 注入比对。retriever 为 nil 时（无
// knowledge.db 配置）标记 evidence_coverage=none，让 check_novelty 知道
// 无法基于真实语料判断，避免凭空给"新颖"结论。
func retrievePriorArtNode(retriever domain.DomainRetriever) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		// 无检索器 → 标记无证据覆盖，check_novelty 据此降级。
		if retriever == nil {
			state[StateKeyEvidenceCoverage] = "none"
			state[StateKeyEvidence] = []EvidenceChunk{}
			return state, nil
		}

		// 构建查询：关键词优先，回退到技术特征描述。
		keywords := extractKeywords(state)
		features := extractFeatureTexts(state)
		queryText := strings.TrimSpace(strings.Join(keywords, " "))
		if queryText == "" {
			queryText = strings.TrimSpace(strings.Join(features, " "))
		}
		if queryText == "" {
			state[StateKeyEvidenceCoverage] = "none"
			state[StateKeyEvidence] = []EvidenceChunk{}
			return state, nil
		}

		results, err := retriever.Search(ctx, domain.DomainQuery{
			Text:       queryText,
			Keywords:   keywords,
			MaxResults: 8,
		})
		if err != nil {
			// 检索失败不应阻断管线——标记无证据，让 novelty 降级而非崩溃。
			state[StateKeyEvidenceCoverage] = "none"
			state[StateKeyEvidence] = []EvidenceChunk{}
			return state, nil
		}

		chunks := make([]EvidenceChunk, 0, len(results.Documents))
		for _, doc := range results.Documents {
			snippet := doc.Snippet
			if snippet == "" {
				snippet = truncateStr(doc.Content, 300)
			}
			chunks = append(chunks, EvidenceChunk{
				DocID:   doc.ID,
				Title:   doc.Title,
				Snippet: snippet,
				Score:   doc.Score,
			})
		}

		state[StateKeyEvidence] = chunks
		state[StateKeyEvidenceCoverage] = coverageLevel(len(chunks))
		return state, nil
	}
}

// extractKeywords 从 state 取 generate_keywords 产出的检索关键词。
func extractKeywords(state graph.PregelState) []string {
	raw, ok := state[StateKeySearchKeywords]
	if !ok {
		return nil
	}
	kw, ok := raw.([]string)
	if !ok {
		return nil
	}
	return kw
}

// extractFeatureTexts 从 state 取 merge 后的技术特征描述，作为查询补充。
func extractFeatureTexts(state graph.PregelState) []string {
	raw, ok := state[StateKeyExtraction]
	if !ok {
		return nil
	}
	ext, ok := raw.(*ExtractionResult)
	if !ok || ext == nil {
		return nil
	}
	out := make([]string, 0, len(ext.Features))
	for _, f := range ext.Features {
		if f.Description != "" {
			out = append(out, f.Description)
		}
	}
	return out
}

// coverageLevel 将命中数量映射为证据覆盖等级，供 check_novelty 决定是否
// 在结论中强调"基于 N 条现有技术比对"或"无法基于外部语料判断"。
func coverageLevel(n int) string {
	switch {
	case n >= 5:
		return "full"
	case n > 0:
		return "partial"
	default:
		return "none"
	}
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
