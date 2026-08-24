package disclosure

import (
	"context"
	"regexp"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// StateKeyClarity 保存交底书清晰度分值。
const StateKeyClarity = "clarity"

// StateKeyClaritySemantic 是语义清晰度来源（LLM 或调用方注入）；缺省取 0 为确定性下限。
const StateKeyClaritySemantic = "clarity_semantic"

// ClarityThreshold 低于该清晰度分值触发人工复核（HITL）。
const ClarityThreshold = 0.2

// SignalCounts 记录四维清晰度信号是否命中。
type SignalCounts struct {
	Problem    bool
	Solution   bool
	Effect     bool
	Enablement bool
}

// claritySignalRe 四维信号正则（Sati signals.ts 简体中文化移植）。
var claritySignalRe = struct {
	Problem, Solution, Effect, Enablement *regexp.Regexp
}{
	Problem:    regexp.MustCompile(`所要解决|现有技术.*(问题|缺陷|不足)|存在.*(问题|缺陷)|不足在于|尚需(改进|解决)`),
	Solution:   regexp.MustCompile(`本发明(提供|包括|采用|通过)|技术方案是|其特征在于`),
	Effect:     regexp.MustCompile(`有益效果|提高了|降低了|提升了|解决了|实现了|改善了`),
	Enablement: regexp.MustCompile(`本领域技术人员|具体实施方式|实施例|如图所示|参见图`),
}

// DetectSignals 检测文本中的四维清晰度信号。
func DetectSignals(text string) SignalCounts {
	return SignalCounts{
		Problem:    claritySignalRe.Problem.MatchString(text),
		Solution:   claritySignalRe.Solution.MatchString(text),
		Effect:     claritySignalRe.Effect.MatchString(text),
		Enablement: claritySignalRe.Enablement.MatchString(text),
	}
}

// SignalWeightedScore 按 Solution>Problem=Effect>Enablement 的权重归一（0..1）。
func SignalWeightedScore(c SignalCounts) float64 {
	const s, p, e, en = 0.3, 0.25, 0.25, 0.2
	score := 0.0
	if c.Solution {
		score += s
	}
	if c.Problem {
		score += p
	}
	if c.Effect {
		score += e
	}
	if c.Enablement {
		score += en
	}
	return score
}

// fuseClarity 融合语义分与信号分：0.75×semantic + 0.25×signal。
func fuseClarity(semantic, signal float64) float64 {
	return 0.75*semantic + 0.25*signal
}

// ComputeClarity 融合语义分与信号分：0.75×semantic + 0.25×signal。
func ComputeClarity(semantic float64, text string) float64 {
	return fuseClarity(semantic, SignalWeightedScore(DetectSignals(text)))
}

// clarityNode 在 merge_extractions 与 groundedness_filter 之间评估交底书清晰度；
// 低于阈值时触发人工复核（InterruptError），复用 review_gate 的 HITL 模式。
func clarityNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		text := extractionSignalText(state)
		if text == "" {
			return state, nil
		}
		sig := SignalWeightedScore(DetectSignals(text))
		semantic, ok := state[StateKeyClaritySemantic].(float64)
		if !ok {
			// 无外部语义注入时以信号分兜底为语义，避免正常样本被低估而误中断。
			semantic = sig
		}
		score := fuseClarity(semantic, sig)
		state[StateKeyClarity] = score
		if score < ClarityThreshold {
			return state, agentcore.NewInterruptErrorWithData(
				"技术交底书内容清晰度不足，请人工补充问题、方案、有益效果或公开实施细节。",
				map[string]any{
					"gate":    "disclosure_clarity",
					"stage":   "check_clarity",
					"clarity": score,
				},
			)
		}
		return state, nil
	}
}

// extractionSignalText 返回用于清晰度评估的文本。
// 优先取完整交底书原文（清晰度衡量的本体是提交的交底书，而非 LLM 提取的骨架短语）；
// 原文缺失时回退到提取结果（供依赖 SetExtraction 的单元测试使用）。
func extractionSignalText(state graph.PregelState) string {
	if doc, ok := state[StateKeyDoc].(*DisclosureDoc); ok && doc != nil && doc.RawText != "" {
		return doc.RawText
	}
	if input := state.GetString(StateKeyInput); input != "" {
		return input
	}
	ext, ok := GetExtraction(state)
	if !ok || ext == nil {
		return ""
	}
	var b strings.Builder
	for _, p := range ext.Problems {
		b.WriteString(p + "\n")
	}
	for _, f := range ext.Features {
		b.WriteString(f.Description + "\n")
	}
	for _, e := range ext.Effects {
		b.WriteString(e + "\n")
	}
	return b.String()
}
