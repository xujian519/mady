package enablement

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 EnablementInput 并验证有效性。
// 当 PFE 三元组为空或特征数为 0 时设置 Skipped=true，后续节点全部跳过。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[stateKeyInput]
		if !ok {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（enablement_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*EnablementInput)
		if !ok || input == nil {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		// 无特征数据时无法评估充分公开。
		if len(input.Features) == 0 && len(input.PFETriples) == 0 {
			state[stateKeyResult] = &EnablementResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提取到技术特征或 PFE 三元组，无法评估说明书充分公开",
			}
			return state, nil
		}

		// 存储已验证的输入，供下游节点使用。
		state[stateKeyInput] = input

		// 自动检测技术领域，供下游节点附加领域规则。
		domain := DetectDomain(input)
		state[stateKeyDomain] = string(domain)

		return state, nil
	}
}

// step1CompletenessNode 三步法第 1 步：检查说明书结构完整性。
// 对照 5 项必要章节（技术领域/背景技术/发明内容/附图说明/具体实施方式），
// 识别缺失章节并给出完整性评分。
func step1CompletenessNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 1 步：",
			"**检查说明书结构完整性**。",
			"",
			"根据审查指南第二部分第二章第 2.1.2 节，「完整」的说明书应包含三个层次：",
			"1. 帮助理解发明**不可缺少**的内容（技术领域、背景技术）",
			"2. 确定新颖性、创造性、实用性**所需**的内容（发明内容的核心技术方案）",
			"3. **实现**发明所需的内容（具体实施方式的详细描述）",
			"",
			"**必要章节**（缺失任一项即为结构不完整）：",
			"1. 技术领域",
			"2. 背景技术",
			"3. 发明内容（要解决的技术问题、技术方案、有益效果）",
			"4. 附图说明（如有附图）",
			"5. 具体实施方式（至少一个实施例）",
			"",
			"**内容质量检查**（不仅检查章节存在，还需评估内容充分性）：",
			"- 发明内容中是否明确记载了**要解决的技术问题**、**技术方案**和**有益效果**",
			"- 具体实施方式是否提供了足以实施的详细描述（非仅泛泛描述）",
			"- 背景技术中引证文件（如有）是否给出明确指引（出处和内容）",
			"",
			"**「完整 ≠ 面面俱到」原则**：",
			"- 所属领域技术人员基于常识能知晓的公知内容可以省略：",
			"  · 公知的常规载体信息可省略",
			"  · 公知的电子元件内部结构可省略",
			"  · 所属领域熟知的工艺流程可省略",
			"- 不可省略的是：发明核心创新点、关键技术参数、实施步骤",
			"",
			"请基于提供的说明书章节内容，判断每一项是否存在**及其内容质量**。",
			"如果某项章节缺失或内容过于简略（如仅有一句话），请标记为缺失。",
			"",
			"",
			"请输出 JSON 格式：",
			`{"has_tech_field": bool, "has_background": bool, "has_content": bool,`,
			`"has_drawings": bool, "has_embodiment": bool,`,
			`"missing_sections": ["缺失章节名称"], "score": 0.0-1.0,`,
			`"notes": "详细说明，包括内容质量问题"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step1", prompt, completenessSchema())
		defer agent.Close()

		output, err := agent.Run(ctx, buildCompletenessInput(input))
		if err != nil {
			return state, fmt.Errorf("step1_completeness: %w", err)
		}

		state[stateKeyStep1] = output
		return state, nil
	}
}

// step2ClarityNode 三步法第 2 步：检查说明书清楚性。
// 判断技术术语是否含义明确、无歧义；PFE 三元组中是否存在孤立的特征或效果。
func step2ClarityNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1Output, _ := state[stateKeyStep1].(string)
		input := extractInput(state)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请执行专利法第26条第3款（充分公开）评估的第 2 步：",
			"**检查说明书清楚性**。",
			"",
			"根据专利法第26条第3款和审查指南第二部分第二章第 2.1.1 节，「清楚」包括：",
			"- 主题明确：技术问题、技术方案和有益效果相互适应，不得矛盾",
			"- 表述准确：使用所属技术领域的技术术语，不得含糊不清或模棱两可",
			"",
			"**技术用语的三种常见问题**（审查指南 §2.1.1）：",
			"1. **歧义术语**：领域内存在多种理解且未界定 → 不符合26.3",
			"   （例如：「藤子暗消」中药异名指代两种不同药材）",
			"2. **自造词**：非领域常规术语，且说明书未给出明确定义或说明 → 不符合26.3",
			"   （例如：「气相指痕光谱」非领域常规术语）",
			"3. **明显错误**：技术人员能识别的错误。只有能确定**唯一**正确理解时才不影响充分公开；",
			"   存在多种合理解释时，不构成明显错误，不符合26.3",
			"   （例如：滤网位置笔误，附图与文字不一致，能确定唯一正确理解→不影响；",
			"   多种位置均可能→不构成明显错误）",
			"",
			"另外请检查：",
			"- 是否存在没有对应技术效果的特征（孤立特征）",
			"- 是否存在没有对应技术特征的效果（孤立效果）",
			"",
			"请输出 JSON 格式：",
			`{"is_clear": bool, "ambiguous_terms": ["歧义术语"],`,
			`"coined_terms": ["自造词"], "obvious_errors": ["明显错误描述"],`,
			`"orphan_features": ["孤立特征描述"], "orphan_effects": ["孤立效果描述"],`,
			`"notes": "详细说明"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-step2", prompt, claritySchema())
		defer agent.Close()

		inputText := buildPFEInput(input)

		// 追加领域特殊检查指令
		domainStr, _ := state[stateKeyDomain].(string)
		if supplement := DomainStep2Supplement(TechDomain(domainStr)); supplement != "" {
			inputText += "\n" + supplement
		}
		if step1Output != "" {
			inputText = "第 1 步（完整性检查）结论：\n" + step1Output + "\n\n" + inputText
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step2_clarity: %w", err)
		}

		state[stateKeyStep2] = output
		return state, nil
	}
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
			"- 判断是否过度实验时考虑因素（参考 In re Wands）：",
			"  所需试验数量、提示指导量、有无实施例、发明性质、",
			"  现有技术状况、该领域技术人员技能、技术可预见性、权利要求宽度",
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

// generateConclusionNode 汇总三步骤结果，生成结构化最终结论。
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1, _ := state[stateKeyStep1].(string)
		step2, _ := state[stateKeyStep2].(string)
		step3, _ := state[stateKeyStep3].(string)

		prompt := strings.Join([]string{
			"你是一名资深专利审查员。请基于专利法第26条第3款（充分公开）评估的",
			"三个步骤的产出，生成最终的结构化评估结论。",
			"",
			"结论应包含：",
			"1. 整体判断：该说明书是否满足 26.3 充分公开的要求（is_sufficient: true/false）",
			"2. 置信度：high（证据充分且结论明确）/ medium（有一定依据但存在不确定性）/",
			"   low（信息不足，难以形成确定判断）",
			"3. 具体缺陷列表（deficiencies）：如果 is_sufficient=false，列出所有具体的公开缺陷",
			"4. 26.4联动风险（support_warnings）：如果存在公开不充分，评估是否影响权利要求得到说明书支持。",
			"   根据授权确权规定第六条第2款：公开不充分必然导致不支持。",
			"   请列出哪些权利要求可能因公开不充分而得不到支持。",
			"5. 实验数据评估（experiment_data）：评估技术效果是否需要实验数据、是否提供、是否有效。",
			"6. 法律提示：本判断由 AI 辅助生成，不构成正式法律意见",
			"",
			"**区分注意**：",
			"- 公开不充分（26.3）针对的是**说明书**——能否实现",
			"- 不支持（26.4）针对的是**权利要求**——概括是否恰当",
			"- 公开不充分必然导致不支持，但不支持不必然意味着公开不充分",
			"- 实用性（22.4）关注的是技术方案是否违背自然规律，公开不充分关注的是记载是否完整",
			"",
			"请输出 JSON 格式：",
			`{"is_sufficient": bool, "reasoning": "综合推理过程",`,
			`"confidence": "high/medium/low",`,
			`"deficiencies": ["具体缺陷1", "具体缺陷2"],`,
			`"support_warnings": ["26.4联动风险提示"],`,
			`"experiment_data": {"data_needed": bool, "data_provided": bool, "is_valid": bool,`,
			`"missing_factors": ["缺失要素"], "notes": "说明"},`,
			`"legal_note": "本判断由 AI 辅助生成，不构成正式法律意见"}`,
		}, "\n")

		agent := newEnablementAgent(provider, "enablement-conclusion", prompt, conclusionSchemaDef())
		defer agent.Close()

		inputText := fmt.Sprintf(
			"第 1 步（完整性检查）:\n%s\n\n第 2 步（清楚性检查）:\n%s\n\n第 3 步（能够实现性检查）:\n%s",
			step1, step2, step3)

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("generate_conclusion: %w", err)
		}

		result := buildResult(step1, step2, step3, output)
		// 设置技术领域标签
		if domainStr, ok := state[stateKeyDomain].(string); ok && domainStr != "" {
			result.TechDomain = domainStr
		}
		state[stateKeyResult] = result
		return state, nil
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// newEnablementAgent 创建统一配置的 LLM Agent 节点。
// 所有评估节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt/schema 不同。
func newEnablementAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
	cfg := agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Name:        name,
			Model:       "default",
			Provider:    provider,
			Temperature: 0.2,
		},
		SystemPrompt: prompt,
		ExecutionConfig: agentcore.ExecutionConfig{
			MaxTurns: 1,
		},
	}
	if schema != nil {
		cfg.ResponseFormat = agentcore.NewJSONSchemaResponseFormat(name, schema)
	}
	return agentcore.New(cfg)
}

// completenessSchema 是 step1 的 JSON Schema。
func completenessSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"has_tech_field": map[string]any{"type": "boolean"},
			"has_background": map[string]any{"type": "boolean"},
			"has_content":    map[string]any{"type": "boolean"},
			"has_drawings":   map[string]any{"type": "boolean"},
			"has_embodiment": map[string]any{"type": "boolean"},
			"missing_sections": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"score": map[string]any{"type": "number"},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"has_tech_field", "has_background", "has_content", "has_embodiment", "missing_sections", "score"},
	}
}

// claritySchema 是 step2 的 JSON Schema。
func claritySchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_clear": map[string]any{"type": "boolean"},
			"ambiguous_terms": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"coined_terms": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"obvious_errors": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"orphan_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"orphan_effects": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"notes": map[string]any{"type": "string"},
		},
		"required": []string{"is_clear", "ambiguous_terms", "coined_terms", "obvious_errors", "orphan_features", "orphan_effects"},
	}
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

// conclusionSchemaDef 是 generateConclusion 的 JSON Schema。
func conclusionSchemaDef() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"is_sufficient": map[string]any{"type": "boolean"},
			"reasoning":     map[string]any{"type": "string"},
			"confidence": map[string]any{
				"type": "string",
				"enum": []string{"high", "medium", "low"},
			},
			"deficiencies": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"support_warnings": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"experiment_data": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data_needed":     map[string]any{"type": "boolean"},
					"data_provided":   map[string]any{"type": "boolean"},
					"is_valid":        map[string]any{"type": "boolean"},
					"missing_factors": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"notes":           map[string]any{"type": "string"},
				},
			},
			"legal_note": map[string]any{"type": "string"},
		},
		"required": []string{"is_sufficient", "reasoning", "confidence", "deficiencies"},
	}
}

// extractInput 从 state 中安全读取已验证的 EnablementInput。
func extractInput(state graph.PregelState) *EnablementInput {
	raw, ok := state[stateKeyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*EnablementInput)
	if !ok {
		return nil
	}
	return input
}

// stateHasSkip 检查 loadInputNode 是否设置了跳过标志。
func stateHasSkip(state graph.PregelState) bool {
	raw, ok := state[stateKeyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*EnablementResult)
	return ok && r != nil && r.Skipped
}

// buildCompletenessInput 构建 Step 1 的 LLM 输入文本。

// renderSimilarCases 将类案列表格式化为 Markdown 引用块。
// 若 cases 为空则跳过输出。
func renderSimilarCases(sb *strings.Builder, cases []string) {
	if len(cases) == 0 {
		return
	}
	sb.WriteString("## 类案参考\n")
	for i, c := range cases {
		fmt.Fprintf(sb, "- 案例%d: %s\n", i+1, c)
	}
	sb.WriteString("\n")
}

func buildCompletenessInput(input *EnablementInput) string {
	var sb strings.Builder
	sb.WriteString("## 说明书章节内容\n\n")

	if len(input.DocSections) > 0 {
		for name, content := range input.DocSections {
			fmt.Fprintf(&sb, "### %s\n%s\n\n", name, truncateText(content, 1000))
		}
	} else {
		sb.WriteString("（未提供章节切分结果，请基于技术特征和 PFE 三元组进行判断）\n\n")
	}

	fmt.Fprintf(&sb, "## 是否有附图\n%v\n\n", input.HasDrawings)

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (重要度: %s)\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}
	renderSimilarCases(&sb, input.SimilarCases)

	return sb.String()
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

// buildResult 从四步 LLM 输出构建结构化的 EnablementResult。
func buildResult(step1, step2, step3, conclusion string) *EnablementResult {
	result := &EnablementResult{
		Assessed: true,
	}

	// 解析 completeness 结果
	result.Completeness = parseCompleteness(step1)

	// 解析 clarity 结果
	result.Clarity = parseClarity(step2)

	// 解析 enablement 结果
	result.Enablement = parseEnablementJudgment(step3)

	// 解析最终结论
	cc := parseConclusion(conclusion)
	result.Conclusion = cc.Reasoning
	result.IsSufficient = cc.IsSufficient
	result.Confidence = cc.Confidence
	result.Deficiencies = cc.Deficiencies
	result.SupportIssue = len(cc.SupportWarnings) > 0
	result.SupportWarnings = cc.SupportWarnings
	result.DataAssessment = cc.ExperimentData

	return result
}

// parsedConclusion 是最终结论的解析结果。
type parsedConclusion struct {
	IsSufficient    bool                      `json:"is_sufficient"`
	Reasoning       string                    `json:"reasoning"`
	Confidence      string                    `json:"confidence"`
	Deficiencies    []string                  `json:"deficiencies"`
	SupportWarnings []string                  `json:"support_warnings"`
	ExperimentData  *ExperimentDataAssessment `json:"experiment_data"`
}

func parseCompleteness(output string) CompletenessResult {
	r := CompletenessResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		HasTechField    bool     `json:"has_tech_field"`
		HasBackground   bool     `json:"has_background"`
		HasContent      bool     `json:"has_content"`
		HasDrawings     bool     `json:"has_drawings"`
		HasEmbodiment   bool     `json:"has_embodiment"`
		MissingSections []string `json:"missing_sections"`
		Score           float64  `json:"score"`
		Notes           string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output
		return r
	}

	r.MissingSections = parsed.MissingSections
	r.Score = parsed.Score
	r.Notes = parsed.Notes
	return r
}

func parseClarity(output string) ClarityResult {
	r := ClarityResult{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Notes = output
		return r
	}

	var parsed struct {
		IsClear        bool     `json:"is_clear"`
		AmbiguousTerms []string `json:"ambiguous_terms"`
		CoinedTerms    []string `json:"coined_terms"`
		ObviousErrors  []string `json:"obvious_errors"`
		OrphanFeatures []string `json:"orphan_features"`
		OrphanEffects  []string `json:"orphan_effects"`
		Notes          string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Notes = output
		return r
	}

	r.IsClear = parsed.IsClear
	r.AmbiguousTerms = parsed.AmbiguousTerms
	r.CoinedTerms = parsed.CoinedTerms
	r.ObviousErrors = parsed.ObviousErrors
	r.OrphanFeatures = parsed.OrphanFeatures
	r.OrphanEffects = parsed.OrphanEffects
	r.Notes = parsed.Notes
	return r
}

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
		r.Notes = output
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

func parseConclusion(output string) parsedConclusion {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return parsedConclusion{
			Reasoning:  output,
			Confidence: "medium",
		}
	}

	var parsed parsedConclusion
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return parsedConclusion{
			Reasoning:  output,
			Confidence: "medium",
		}
	}

	switch parsed.Confidence {
	case "high", "medium", "low":
	default:
		parsed.Confidence = "medium"
	}

	return parsed
}

// extractJSON 从文本中提取第一个 JSON 对象。
func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}

// truncateText 截断文本到指定长度（rune 安全）。
func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "…"
}
