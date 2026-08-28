package enablement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

func parseEnablementJudgment(output string) EnablementJudgment {
	r := EnablementJudgment{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		CanImplement       bool     `json:"can_implement"`
		MissingKeyMeans    bool     `json:"missing_key_means"`
		VagueMeans         bool     `json:"vague_means"`
		OnlyTaskNoMeans    bool     `json:"only_task_no_means"`
		InsufficientData   bool     `json:"insufficient_data"`
		MeansCannotSolve   bool     `json:"means_cannot_solve"`
		PartialMeansUnreal bool     `json:"partial_means_unrealizable"`
		FailureReasons     []string `json:"failure_reasons"`
		Notes              string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output // LLM 返回非 JSON：降级为原始文本作为判断依据
		return r
	}

	r.CanImplement = parsed.CanImplement
	r.MissingKeyMeans = parsed.MissingKeyMeans
	r.VagueMeans = parsed.VagueMeans
	r.OnlyTaskNoMeans = parsed.OnlyTaskNoMeans
	r.InsufficientData = parsed.InsufficientData
	r.MeansCannotSolve = parsed.MeansCannotSolve
	r.PartialMeansUnreal = parsed.PartialMeansUnreal
	r.FailureReasons = parsed.FailureReasons
	r.Notes = parsed.Notes
	return r
}

// step3EnablementNode 三步法第 3 步（核心）：检查能够实现性。
// 判断本领域技术人员根据说明书记载能否无需创造性劳动即可实施，
// 逐一检测六种公开不充分的经典情形（审查指南 §2.1.3）。
func step3EnablementNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step2Output, _ := state[stateKeyStep2].(string)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 3 步：",
			"**检查能够实现性**（这是 26.3 的核心标准）。",
			"",
			"根据专利法第26条第3款和审查指南第二部分第二章第 2.1.3 节，",
			"「能够实现」是指所属技术领域的技术人员根据说明书记载，",
			"无需创造性劳动即可实施该发明，解决其技术问题，并且产生预期的技术效果。",
			"",
			"**「能够实现」要求三者同时满足**：实现技术方案 + 解决技术问题 + 产生预期效果。",
			"",
			"请逐一检查以下六种公开不充分的经典情形（审查指南 §2.1.3 + 司法解释）：",
			"",
			"1. **仅给出任务/设想，未给出任何技术手段**：说明书是否只提出了要解决的问题或希望达到的目标，",
			"   而未给出任何使本领域技术人员能够实施的技术手段？",
			"   （例如：仅记载「可用交流电点烟」但未记载具体结构）",
			"2. **技术手段含糊不清，无法具体实施**：技术描述是否具体、可操作？",
			"   （例如：使用「助燃剂1号」等未定义自造词，或「合适的」「根据需要调节」等开放性表述）",
			"3. **给出了技术手段，但不能解决技术问题**：所记载的技术手段从物理/化学原理上",
			"   是否能够真正实现声称的技术效果？",
			"   （例如：折叠椅用不可折叠的折弯件连接椅背椅座；飞行汽车违背物理原理）",
			"4. **多手段方案中某一手段不能实现**：如果方案包含多个技术手段的组合，",
			"   其中某一个技术手段是否缺少实现方式，导致整体方案不能实施？",
			"   （例如：多功能设备中某个功能模块无具体实现手段）",
			"5. **缺少关键技术手段的说明**：说明书是否给出了实施发明所需的关键技术手段？",
			"   （例如：未公开核心算法、关键参数、特定材料）",
			"6. **方案须依赖实验结果但未给出实验证据**：如果技术效果必须依赖实验结果才能成立",
			"   （如化学新化合物、新用途），说明书是否提供了充分的实验数据？",
			"",
			"**领域可预见性差异**：",
			"- 机械/电学领域可预见性高，结构描述+附图通常足以实施",
			"- 化学/医药/生物领域可预见性低，通常**必须**依赖实验数据证实效果",
			"",
			"**技术问题认定规则**：",
			"- 技术问题可以是：说明书中明确记载的、通过阅读说明书能直接确定的、",
			"  或根据技术效果/技术方案能确定的",
			"- 当记载多个技术问题时，只要技术方案能解决**至少一个**，即满足「解决其技术问题」",
			"",
			"**「无需过度实验」标准**：",
			"- 即使需要经过简单试验确定具体实施方法，只要试验是惯常的而非过度的，",
			"  即认为「能够实现」",
			"- 判断是否过度实验时考虑因素（参考《专利审查指南》第二部分第二章 §2.1.3）：",
			"  所需试验数量、说明书中的指导量、有无实施例、发明性质、",
			"  所属领域技术状况、本领域技术人员技能、技术可预见性、权利要求宽度",
			"",
			"**明显夸大的技术效果处理**：",
			"- 如果发明确实可以解决现有技术问题，技术效果的明显夸大**通常不导致**公开不充分",
			"- **除非**申请人坚持以夸大效果作为充分公开的判断基础",
			"",
			"请基于 PFE 三元组（问题→特征→效果因果链）判断：",
			"每个 Problem 是否都能通过一条完整的 Feature→Effect 链路实现？",
			"如果某个 Problem 缺少对应的 Feature，即为公开不充分。",
			"",
			"**类案参考（如有提供）**：下面「类案参考」部分列出了与本案相似的类案判断，",
			"请参考其中充分公开的判断标准，特别是典型案例中的审查实践。",
			"但注意每个案件的判断应基于本案说明书的具体记载内容，类比时需考虑领域差异。",
			"请输出 JSON 格式：",
			`{"can_implement": bool,`,
			`"missing_key_means": bool, "vague_means": bool,`,
			`"only_task_no_means": bool, "insufficient_data": bool,`,
			`"means_cannot_solve": bool, "partial_means_unrealizable": bool,`,
			`"failure_reasons": ["具体原因"], "notes": "详细推理过程"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step3", prompt, enablementSchema())
		defer agent.Close()

		inputText := "第 2 步（清楚性检查）结论：\n" + step2Output

		// 追加领域特殊检查指令
		domainStr, _ := state[stateKeyDomain].(string)
		if supplement := DomainStep3Supplement(TechDomain(domainStr)); supplement != "" {
			inputText += "\n" + supplement
		}

		// 追加原始 PFE 输入，以便 LLM 获取完整上下文
		if input := extractInput(state); input != nil {
			inputText += "\n\n" + buildPFEInput(input)
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step3_enablement: %w", err)
		}

		state[stateKeyStep3] = output
		return state, nil
	}
}

// buildPFEInput 构建 Step 2/3 的 LLM 输入文本（基于 PFE 三元组）。
func buildPFEInput(input *EnablementInput) string {
	var sb strings.Builder

	if len(input.Problems) > 0 {
		sb.WriteString("## 技术问题\n")
		for _, p := range input.Problems {
			fmt.Fprintf(&sb, "- %s\n", p)
		}
		sb.WriteString("\n")
	}

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (功能: %s, 重要度: %s)\n",
				f.Category, f.Description, f.Function, f.Importance)
		}
		sb.WriteString("\n")
	}

	if len(input.Effects) > 0 {
		sb.WriteString("## 技术效果\n")
		for _, e := range input.Effects {
			fmt.Fprintf(&sb, "- %s\n", e)
		}
		sb.WriteString("\n")
	}

	if len(input.PFETriples) > 0 {
		sb.WriteString("## PFE 三元组（问题→特征→效果因果链）\n")
		for _, t := range input.PFETriples {
			fmt.Fprintf(&sb, "- [%s] 问题: %s → 特征: %v → 效果: %s\n",
				t.ID, t.Problem, t.FeatureIDs, t.Effect)
		}
		sb.WriteString("\n")
	}

	if len(input.GuidelineRefs) > 0 {
		sb.WriteString("## 审查指南参考\n")
		for _, ref := range input.GuidelineRefs {
			fmt.Fprintf(&sb, "- %s\n", ref)
		}
		sb.WriteString("\n")
	}

	renderSimilarCases(&sb, input.SimilarCases)

	return sb.String()
}

// enablementSchema 是 step3 的 JSON Schema。
func enablementSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"can_implement":              map[string]any{"type": "boolean"},
			"missing_key_means":          map[string]any{"type": "boolean"},
			"vague_means":                map[string]any{"type": "boolean"},
			"only_task_no_means":         map[string]any{"type": "boolean"},
			"insufficient_data":          map[string]any{"type": "boolean"},
			"means_cannot_solve":         map[string]any{"type": "boolean"},
			"partial_means_unrealizable": map[string]any{"type": "boolean"},
			"failure_reasons": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"can_implement", "missing_key_means", "vague_means", "only_task_no_means", "insufficient_data", "means_cannot_solve", "partial_means_unrealizable"},
	}
}
