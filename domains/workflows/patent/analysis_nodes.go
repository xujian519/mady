package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/ipc"
	"github.com/xujian519/mady/graph"
	"github.com/xujian519/mady/retrieval/domain"
)

// parseNode extracts technical features from the invention description.
// It identifies key claim elements from the Chinese patent description format:
// "一种...方法/装置/系统" + "其特征在于" + "包括/由...组成".
func parseNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	input := state.GetString(StateInput)
	if input == "" {
		return nil, fmt.Errorf("patent: input is empty")
	}

	features := extractFeatures(input)

	out := graph.PregelState{
		StateFeatures:    features,
		StateSearchQuery: buildSearchQuery(features),
		StateInput:       input,
	}
	return out, nil
}

// extractFeatures identifies technical features from a Chinese patent description.
func extractFeatures(text string) []string {
	var features []string

	// Split by common feature delimiters.
	markers := []string{"包括", "包含", "由...组成", "其特征在于", "所述"}
	for _, marker := range markers {
		if idx := strings.Index(text, marker); idx >= 0 {
			rest := text[idx+len(marker):]
			// Extract the phrase after the marker (up to punctuation or next marker).
			for _, part := range strings.Split(rest, "；") {
				part = strings.TrimSpace(part)
				if len(part) > 5 && len(part) < 200 {
					features = append(features, part)
				}
			}
		}
	}

	// If no structured features found, use sentence splitting.
	if len(features) == 0 {
		for _, sentence := range strings.Split(text, "。") {
			sentence = strings.TrimSpace(sentence)
			if len(sentence) > 10 {
				features = append(features, sentence)
			}
		}
	}

	// Deduplicate and limit.
	seen := make(map[string]bool)
	var result []string
	for _, f := range features {
		f = strings.TrimSpace(f)
		if !seen[f] && len(f) > 3 {
			seen[f] = true
			result = append(result, f)
		}
		if len(result) >= 10 {
			break
		}
	}
	return result
}

// buildSearchQuery constructs a prior art search query from features.
func buildSearchQuery(features []string) string {
	if len(features) == 0 {
		return ""
	}
	// Use first 3 features as primary search terms.
	n := len(features)
	if n > 3 {
		n = 3
	}
	return strings.Join(features[:n], " ")
}

// FeatureRiskScanner scans technical feature combinations for historical
// invalidation risk. Implemented by *risk.Scanner.
type FeatureRiskScanner interface {
	ScanByFeatures(ctx context.Context, features []string) (*RiskScanResult, error)
}

// RiskScanResult is a minimal projection of risk.ScanResult to avoid a direct
// dependency on knowledge/risk from the workflow layer. The Markdown rendering
// is the only field consumed by the pipeline.
type RiskScanResult struct {
	Markdown string // pre-rendered Markdown report
}

// =============================================================================
// GraphOption — 可选依赖注入
// =============================================================================

// GraphOption 可选地配置 patent 分析图的依赖（如 search 节点的检索器）。
// 采用 functional option 模式，使无检索器的调用点零破坏。
type GraphOption func(*graphConfig)

type graphConfig struct {
	retriever domain.DomainRetriever
	scanner   FeatureRiskScanner
}

// WithRetriever 注入专利领域检索器，启用 search 节点的真实现有技术检索。
// 未注入时 search 节点返回占位文本，保持向后兼容。
func WithRetriever(r domain.DomainRetriever) GraphOption {
	return func(c *graphConfig) { c.retriever = r }
}

// WithRiskScanner 注入风险扫描器，在 analyze 与 rule_check 之间插入
// risk_scan 节点，对提取的技术特征进行历史无效宣告风险扫描。
// 未注入时不插入该节点，保持向后兼容。
func WithRiskScanner(s FeatureRiskScanner) GraphOption {
	return func(c *graphConfig) { c.scanner = s }
}

// =============================================================================
// 检索节点
// =============================================================================

// newSearchNode 创建现有技术检索的 Pregel 节点。
// retriever 为 nil 时返回占位结果（兼容旧行为）。
// retriever 非 nil 时查询专利领域检索器，产出真实现有技术列表。
func newSearchNode(retriever domain.DomainRetriever) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		query := state.GetString(StateSearchQuery)
		features, _ := state[StateFeatures].([]string)

		out := graph.PregelState{
			StateSearchQuery: query,
		}
		if features != nil {
			out[StateFeatures] = features
		}

		// 无检索器 → 标记降级，返回空结果。
		if retriever == nil {
			graph.MarkDegraded(out, StatePriorArt, []string{},
				graph.DegradationRetrieverNil,
				"未配置现有技术检索器，无法进行真实检索。建议在项目设置中配置专利数据库接入。")
			return out, nil
		}

		// 真实检索：用查询文本搜索专利领域检索器。
		results, err := retriever.Search(ctx, domain.DomainQuery{
			Text:       query,
			MaxResults: 8,
		})
		if err != nil {
			// 检索失败不阻断管线，但显式标记降级。
			graph.MarkDegraded(out, StatePriorArt, []string{},
				graph.DegradationSearchFailed,
				fmt.Sprintf("现有技术检索失败: %v。建议手动检索相关对比文件。", err))
			return out, nil
		}

		priorArt := make([]string, 0, len(results.Documents))
		for _, doc := range results.Documents {
			snippet := doc.Snippet
			if snippet == "" && doc.Content != "" {
				// 截取内容前 200 字作为摘要。
				runes := []rune(doc.Content)
				if len(runes) > 200 {
					snippet = string(runes[:200]) + "…"
				} else {
					snippet = doc.Content
				}
			}
			text := fmt.Sprintf("[%s] %s", doc.ID, doc.Title)
			if snippet != "" {
				text += ": " + snippet
			}
			priorArt = append(priorArt, text)
		}

		if len(priorArt) == 0 {
			priorArt = append(priorArt, "未检索到相关现有技术文献")
		}

		out[StatePriorArt] = priorArt
		return out, nil
	}
}

// analyzeNode performs feature-by-feature comparison between the invention
// and retrieved prior art. It produces a structured comparison result.
// IPC classification is automatically performed to inject domain-specific
// analysis hints for novelty and inventiveness assessment.
func analyzeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	features, _ := state[StateFeatures].([]string)
	priorArt, _ := state[StatePriorArt].([]string)
	input := state.GetString(StateInput)

	// IPC 分类识别——从发明描述中自动判定技术领域
	ipcSection, ipcConfidence := ipc.Classify(input)

	var comparison strings.Builder
	comparison.WriteString("## 技术特征比对分析\n\n")

	// IPC 技术领域信息
	comparison.WriteString("### 技术领域\n\n")
	fmt.Fprintf(&comparison, "- IPC 大类：%s（%s）\n", ipcSection, ipcSection.SectionOf())
	fmt.Fprintf(&comparison, "- 分类置信度：%.0f%%\n\n", ipcConfidence*100)

	// 领域特化提示——如果置信度较高，注入针对性的审查要点
	if ipc.IsHighConfidence(ipcConfidence) {
		if hints := ipc.GetNoveltyHints(ipcSection); hints != "" {
			comparison.WriteString(hints)
			comparison.WriteString("\n")
		}
	}

	if len(features) == 0 {
		comparison.WriteString("未识别到明确的技术特征。\n")
	} else {
		comparison.WriteString("### 识别到的技术特征\n\n")
		for i, f := range features {
			fmt.Fprintf(&comparison, "%d. %s\n", i+1, f)
		}
	}

	comparison.WriteString("\n### 现有技术参考\n\n")
	if mark := graph.GetDegradationMark(state, StatePriorArt); mark != nil {
		fmt.Fprintf(&comparison, "> ⚠️ 检索降级：%s\n\n", mark.Message)
	} else if len(priorArt) > 0 {
		for _, art := range priorArt {
			fmt.Fprintf(&comparison, "- %s\n", art)
		}
	} else {
		comparison.WriteString("未检索到相关现有技术。\n")
	}

	return graph.PregelState{
		StateComparison: comparison.String(),
		StateFeatures:   features,
		StatePriorArt:   priorArt,
		StateInput:      input,
	}, nil
}

// concludeNode generates the final novelty/creativity assessment report.
// In production, this would be an LLM call using the accumulated analysis.
func concludeNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	comparison := state.GetString(StateComparison)
	input := state.GetString(StateInput)

	var report strings.Builder
	report.WriteString("# 专利分析报告\n\n")
	report.WriteString("## 发明概述\n\n")
	if len(input) > 200 {
		input = input[:200] + "..."
	}
	report.WriteString(input)
	report.WriteString("\n\n")
	report.WriteString(comparison)
	report.WriteString("\n\n## 初步结论\n\n")
	report.WriteString("> ⚠️ 本分析由 AI 辅助生成，不构成正式法律意见。")
	report.WriteString("专利性判断应由具备资质的专利代理人或律师确认。\n\n")
	report.WriteString("基于现有技术的初步检索和分析：\n")
	report.WriteString("- 建议进行更全面的专利检索以确认新颖性\n")
	report.WriteString("- 根据审查指南的规定，进一步评估创造性\n")
	report.WriteString("- 权利要求宜得到说明书的充分支持，建议由代理人核实\n")

	return graph.PregelState{
		StateConclusion: report.String(),
		StateOutput:     report.String(),
	}, nil
}
