// Package bootstrap 提供所有 mady 入口（tui/serve/acp/desktop）共享的装配逻辑。
// 注意：bootstrap 是全局装配器，已知会跨层引用 domains/mcp/guardrails 等上层包。
// 这是设计上接受的"必要之恶"，不应被其他基础设施层包导入。
package bootstrap

import (
	"context"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/infringement"
	"github.com/xujian519/mady/knowledge"
)

// NewInfringementKnowledgeRetriever 从 agentcore.Extension 创建 infringement.KnowledgeRetriever。
// 内部类型断言为 *knowledge.KnowledgeExtension 以获取 LawSearcher 和 GraphContext；
// 当 ext 为 nil 或类型不匹配时返回 nil（降级为纯 LLM 知识评估）。
func NewInfringementKnowledgeRetriever(ext agentcore.Extension) infringement.KnowledgeRetriever {
	if ext == nil {
		return nil
	}
	kext, ok := ext.(*knowledge.KnowledgeExtension)
	if !ok {
		return nil
	}
	return &infringementKRAdapter{
		lawSearcher: kext.LawSearcher(),
		graphCtxFn:  kext.GraphContext,
	}
}

// infringementKRAdapter 将知识库检索能力适配为 infringement.KnowledgeRetriever 接口。
type infringementKRAdapter struct {
	lawSearcher knowledge.LawSearcher
	graphCtxFn  func() string
}

// SearchGuidelines 搜索审查指南相关条款，基于 LawSearcher 模糊匹配。
func (a *infringementKRAdapter) SearchGuidelines(ctx context.Context, query string) ([]infringement.GuidelineRef, error) {
	if a.lawSearcher == nil {
		return nil, nil
	}
	results, err := a.lawSearcher(query, 5)
	if err != nil {
		return nil, err
	}
	refs := make([]infringement.GuidelineRef, 0, len(results))
	for _, r := range results {
		refs = append(refs, infringement.GuidelineRef{
			Source:  r.Name,
			Section: r.Subtitle,
			Content: r.Content,
		})
	}
	return refs, nil
}

// SearchSimilarCases 从最近一次知识图谱增强结果中检索相似案例。
func (a *infringementKRAdapter) SearchSimilarCases(ctx context.Context, query string) ([]infringement.CaseRef, error) {
	if a.graphCtxFn == nil {
		return nil, nil
	}
	graphCtx := a.graphCtxFn()
	if graphCtx == "" {
		return nil, nil
	}
	return nil, nil // 图谱上下文的类案结构化提取留给后续迭代
}

// SearchLegalProvisions 搜索法律法规全文，通过 LawSearcher 匹配条文关键词。
func (a *infringementKRAdapter) SearchLegalProvisions(ctx context.Context, articles []string) ([]infringement.LegalProvision, error) {
	if a.lawSearcher == nil || len(articles) == 0 {
		return nil, nil
	}
	var provisions []infringement.LegalProvision
	seen := make(map[string]bool)
	for _, article := range articles {
		if seen[article] {
			continue
		}
		seen[article] = true
		results, err := a.lawSearcher(article, 1)
		if err != nil {
			continue
		}
		for _, r := range results {
			provisions = append(provisions, infringement.LegalProvision{
				Article: article,
				Law:     r.Name,
				Content: r.Content,
			})
		}
	}
	return provisions, nil
}
