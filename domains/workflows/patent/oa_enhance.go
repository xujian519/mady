package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/prompt"
)

// oaEnhanceSystemPromptFallback 是 oa-response-enhance 模板未加载时的
// 内联兜底提示词，确保 prompt 模板系统不可用时仍能生成答复增强。
const oaEnhanceSystemPromptFallback = `你是资深的中国专利代理师，负责撰写审查意见（OA）答复书的实质论证部分。
请基于已有的答复骨架，撰写具体、有说服力的论证段落。

要求：
1. 针对审查员指出的驳回理由，逐条进行实质性反驳
2. 引用对比文件的具体技术特征，详细说明区别
3. 结合《专利审查指南》的相关规定，论证本发明的专利性
4. 使用专利代理实务中的标准措辞和专业表述
5. 论证应当具体、有针对性，避免空洞套话

输出格式：
直接输出增强后的完整答复书 Markdown 文本。在原有骨架的基础上，
在每个章节下补充具体的论证段落。不需要额外说明或前缀。`

// newOAEnhanceNode 创建 OA 答复的 LLM 增强节点。
// 在确定性骨架基础上，调用 LLM 生成实质性论证段落。
// provider 为 nil 时返回 no-op 节点（跳过增强）。
//
// 参考 disclosure/novelty.go 的内联工厂模式，使用 MaxTurns=1 的单次 LLM 调用。
// 节点只返回自己产生的增量 state（OAStateResponseDraft/OAStateOutput/OAStateLLMEnhanced），
// 其余 key 由 Pregel 合并机制自动保留。
func newOAEnhanceNode(provider agentcore.Provider) graph.PregelNode {
	if provider == nil {
		return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
			return graph.PregelState{OAStateLLMEnhanced: false}, nil
		}
	}

	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        "oa-enhance",
			Model:       "default",
			Provider:    provider,
			Temperature: 0.3,
		},
		SystemPrompt: prompt.ResolveSystemPromptOr("prompt://oa-response-enhance", oaEnhanceSystemPromptFallback),
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns:          1,
			ValidateArguments: true,
		},
	}

	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		// 读取确定性骨架。
		draft := state.GetString(OAStateResponseDraft)
		if draft == "" {
			return graph.PregelState{OAStateLLMEnhanced: false}, nil
		}

		// 构建 LLM 输入：原始 OA 文本 + 骨架。
		oaInput := state.GetString(OAStateInput)
		var promptText strings.Builder
		promptText.WriteString("【审查意见通知书原文】\n")
		promptText.WriteString(oaInput)
		promptText.WriteString("\n\n【现有答复骨架】\n")
		promptText.WriteString(draft)
		promptText.WriteString("\n\n请基于上述信息，撰写完整、有说服力的答复书。")

		agent := agentcore.New(cfg)
		output, err := agent.Run(ctx, promptText.String())
		agent.Close()
		if err != nil || output == "" {
			// LLM 失败时静默降级，保留确定性输出。
			return graph.PregelState{OAStateLLMEnhanced: false}, nil
		}

		return graph.PregelState{
			OAStateResponseDraft: output,
			OAStateOutput:        output,
			OAStateLLMEnhanced:   true,
		}, nil
	}
}

// rejectionTypeToQuery maps OA rejection types to knowledge-base search queries.
// Each query is crafted to retrieve the most relevant law articles and guideline
// excerpts for that rejection category.
func rejectionTypeToQuery(rejectionType string) string {
	switch OaRejectionType(rejectionType) {
	case OaNovelty:
		return "专利法第22条第2款 新颖性 单独对比 现有技术"
	case OaInventiveness:
		return "专利法第22条第3款 创造性 三步法 技术启示"
	case OaClarity:
		return "专利法第26条第4款 权利要求清楚 简明"
	case OaSupport:
		return "专利法第26条第4款 说明书支持 权利要求"
	case OaDisclosure:
		return "专利法第26条第3款 充分公开 能够实现"
	case OaScope:
		return "专利法第33条 修改 超出范围 原说明书"
	default:
		return "专利法 审查指南 答复策略"
	}
}

// newRuleRetrievalNode creates a Pregel node that dynamically retrieves applicable
// law articles for every detected rejection type. retriever 为 nil 时返回 no-op。
// 节点只返回增量（OAStateApplicableRules），其余 state 由 Pregel 合并保留。
func newRuleRetrievalNode(retriever OARuleRetriever) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if retriever == nil {
			return graph.PregelState{}, nil
		}

		merged, anyFailed := retrieveOARules(ctx, retriever, rejectionTypesFromState(state))

		out := graph.PregelState{}
		if len(merged) > 0 {
			out[OAStateApplicableRules] = renderOARules(merged)
		}
		if anyFailed {
			message := "部分法条动态检索失败: 已使用成功检索到的法条。"
			if len(merged) == 0 {
				message = "法条动态检索失败: 将使用内置模板法条。"
			}
			graph.MarkDegraded(out, OAStateApplicableRules, out[OAStateApplicableRules],
				graph.DegradationSearchFailed, message)
		}
		return out, nil
	}
}

// retrieveOARules queries the retriever once per rejection type and merges the
// results, deduplicating by article reference. The bool reports whether any
// per-type query failed (partial failure still returns the successful articles).
func retrieveOARules(ctx context.Context, retriever OARuleRetriever, rejectionTypes []OaRejectionType) ([]OALawArticle, bool) {
	var merged []OALawArticle
	seen := make(map[string]bool)
	anyFailed := false

	for _, rt := range rejectionTypes {
		articles, err := retriever.RetrieveRules(ctx, rejectionTypeToQuery(string(rt)))
		if err != nil {
			// 该驳回类型检索失败：置 anyFailed 由上层标记降级，继续其余类型。
			anyFailed = true
			continue
		}
		for _, a := range articles {
			if a.ArticleRef != "" && seen[a.ArticleRef] {
				continue
			}
			seen[a.ArticleRef] = true
			merged = append(merged, a)
		}
	}
	return merged, anyFailed
}

// renderOARules formats retrieved law articles as a Markdown section.
func renderOARules(articles []OALawArticle) string {
	var b strings.Builder
	b.WriteString("## 适用法条与审查指南\n\n")
	for _, a := range articles {
		fmt.Fprintf(&b, "- **%s**（%s）：%s\n", a.ArticleRef, a.Source, a.Title)
		if a.Content != "" {
			excerpt := a.Content
			if r := []rune(excerpt); len(r) > 200 {
				excerpt = string(r[:200]) + "…"
			}
			fmt.Fprintf(&b, "  > %s\n", excerpt)
		}
	}
	return b.String()
}

// OARuleRetriever abstracts dynamic retrieval of applicable law articles and
// examination guidelines for OA response drafting.
// When injected, the pipeline retrieves real law articles instead of using
// hardcoded template strings.
type OARuleRetriever interface {
	RetrieveRules(ctx context.Context, rejectionType string) ([]OALawArticle, error)
}

// OALawArticle is a single law/guideline provision relevant to a rejection type.
type OALawArticle struct {
	ArticleRef string // e.g. "专利法第22条第3款"
	Title      string // e.g. "创造性"（法条/章节标题）
	Content    string // provision text or guideline excerpt
	Source     string // e.g. "专利法", "审查指南第二部分第四章"
}
