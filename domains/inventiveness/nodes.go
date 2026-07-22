package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

// =============================================================================
// State keys
// =============================================================================

const (
	// StateKeyInput 是 PregelState 中存储 InventivenessInput 的 key。
	StateKeyInput = "inventiveness_input"
	// StateKeyResult 是 PregelState 中存储 InventivenessResult 的 key。
	StateKeyResult = "inventiveness_result"
	stateKeyStep1  = "step1_closest_prior_art"
	stateKeyStep2  = "step2_distinguishing_features"
	stateKeyStep3  = "step3_technical_suggestion"
	stateKeyStep4  = "step4_significant_progress"
)

// =============================================================================
// 节点实现
// =============================================================================

// loadInputNode 从 PregelState 读取 InventivenessInput。
// 当 EvidenceCoverage == "none" 时跳过，设置 Skipped=true。
func loadInputNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		raw, ok := state[StateKeyInput]
		if !ok {
			state[StateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "未提供输入数据（inventiveness_input state key 为空）",
			}
			return state, nil
		}

		input, ok := raw.(*InventivenessInput)
		if !ok || input == nil {
			state[StateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "输入数据格式无效",
			}
			return state, nil
		}

		if input.EvidenceCoverage == "none" {
			state[StateKeyResult] = &InventivenessResult{
				Assessed:   false,
				Skipped:    true,
				SkipReason: "无检索证据，无法进行三步法创造性评估",
			}
			return state, nil
		}

		// Store validated input for downstream nodes.
		state[StateKeyInput] = input
		return state, nil
	}
}

// step1ClosestPriorArtNode 三步法第 1 步：确定最接近的现有技术。
func step1ClosestPriorArtNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		input := extractInput(state)
		if input == nil {
			return state, nil // skipped already
		}

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 1 步：\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "从以下现有技术证据中，确定与目标技术方案最接近的一篇对比文件。\n"
		prompt += "最接近的现有技术是指与目标方案技术领域相同、要解决的技术问题最接近、"
		prompt += "或技术特征最多的现有技术文献。\n\n"
		if typeGuidance := inventionTypeGuidance(input.InventionType); typeGuidance != "" {
			prompt += typeGuidance + "\n\n"
		}
		if domainGuidance := techDomainGuidance(input.TechDomain); domainGuidance != "" {
			prompt += domainGuidance + "\n\n"
		}
		prompt += "请列出选定文献的标题和理由。"

		inputText := buildInputText(input)
		agent := newInventivenessAgent(provider, "inventiveness-step1", prompt, step1Schema())
		defer agent.Close()

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step1: %w", err)
		}

		state[stateKeyStep1] = output
		return state, nil
	}
}

// step2DistinguishingFeaturesNode 三步法第 2 步：确定区别特征和实际解决的技术问题。
func step2DistinguishingFeaturesNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1Output, ok := state[stateKeyStep1].(string)
		if !ok {
			step1Output = ""
		}
		input := extractInput(state)

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 2 步：\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "基于第 1 步确定的最接近现有技术，进行以下分析：\n\n"
		prompt += "1. 逐项列出目标方案相对于最接近现有技术的区别技术特征\n"
		prompt += "2. 基于区别技术特征，重新确定发明实际解决的技术问题\n"
		prompt += "   （注意：不是「原要解决的技术问题」，而是区别特征客观上实际解决的问题）\n\n"
		prompt += "**技术问题层次分析法**（防止问题层次偏差）：\n"
		prompt += "1. 表层问题：直接对应区别特征的功能描述（如「如何在设备中增加X部件」）\n"
		prompt += "2. 中层问题：对应区别特征在系统中的技术效果（如「如何提高设备的运行效率」）← 通常应选择此层\n"
		prompt += "3. 深层问题：对应发明整体方案解决的根本技术难题（如「如何在保证效率的同时降低能耗」）\n"
		prompt += "注意：层次过高会放大区别特征的创造性，层次过低会缩小。应优先选择中层问题。\n\n"
		if typeGuidance := inventionTypeGuidance(input.InventionType); typeGuidance != "" {
			prompt += typeGuidance + "\n\n"
		}
		if domainGuidance := techDomainGuidance(input.TechDomain); domainGuidance != "" {
			prompt += domainGuidance + "\n\n"
		}
		prompt += "**技术问题确定的五种情形**（判断当前属于哪种）：\n"
		prompt += "情形一（相同现有技术）：最接近现有技术 = 申请人描述的现有技术 → 技术问题通常与说明书记载相同\n"
		prompt += "情形二（不同现有技术，最常见）：最接近现有技术 ≠ 申请人描述的现有技术 → 需重新确定\n"
		prompt += "情形三（领域相同但方案差异大）：技术问题在层次和方向上可能有较大差异\n"
		prompt += "情形四（多特征且功能相互支持）：应整体考虑，确定一个统一的技术问题\n"
		prompt += "情形五（所有效果均相当）：技术问题确定为「提供一种不同于最接近现有技术的可供选择的技术方案」\n\n"
		prompt += "**发明形成过程分析**（影响技术问题认定的客观性）：\n"
		prompt += "识别发明形成的出发点：\n"
		prompt += "1. 新的构思或尚未认识的技术问题 → 问题本身有创造性（即使解决手段显而易见，发明仍可能具备创造性）\n"
		prompt += "2. 为已知技术问题设计新的解决手段 → 最常见类型\n"
		prompt += "3. 发现已知现象的内在原因 → 从现象反推原因\n"
		prompt += "特别警示：对于第1种出发点，不得因为「解决手段本身是显而易见的」而否定创造性。\n\n"
		prompt += "**区别特征的划分规则**：\n"
		prompt += "- 互不依存、各自解决不同技术问题的特征 → 应拆分为独立的区别特征分别分析\n"
		prompt += "- 紧密联系、协同作用、功能上相互支持的特征 → 应整体考虑，作为一个统一的技术手段\n"
		prompt += "- 不得将协同特征拆分为碎片化特征逐一评价（这是「事后诸葛亮」的常见表现）\n\n"
		prompt += "**无贡献特征识别**（2023年审查指南第84号局令新增）：\n"
		prompt += "对技术问题的解决没有作出贡献的特征，即使写入权利要求中，通常也不会对技术方案的创造性产生影响。\n"
		prompt += "四维度判断标准：\n"
		prompt += "- 与技术问题的关联：特征是否直接参与技术问题的解决过程？\n"
		prompt += "- 技术效果：特征是否带来了进一步的技术效果？\n"
		prompt += "- 常规性：是否属于主题本身的常规组成部分？\n"
		prompt += "- 可获知性：是否本领域技术人员基于普通知识即可得到？\n"
		prompt += "常见无贡献特征示例：主题蕴含的常规组成部分（如照相机外壳形状、显示屏大小）、\n"
		prompt += "本领域公知常识、常规实验手段可得的参数、说明书中未记载关联技术效果的特征。\n\n"
		prompt += "请输出 JSON 格式，包含：\n"
		prompt += "- distinguishing_features: 区别特征列表（仅列出有贡献的区别特征）\n"
		prompt += "- non_contributing_features: 无贡献特征列表（如有）\n"
		prompt += "- tech_effects: 区别特征对应的技术效果\n"
		prompt += "- actual_tech_problem: 实际解决的技术问题"

		agent := newInventivenessAgent(provider, "inventiveness-step2", prompt, step2Schema())
		defer agent.Close()

		inputText := buildInputText(input)
		if step1Output != "" {
			inputText = "第 1 步结论：\n" + step1Output + "\n\n" + inputText
		}

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step2: %w", err)
		}

		state[stateKeyStep2] = output
		return state, nil
	}
}

// step3TechnicalSuggestionNode 三步法第 3 步：判断现有技术整体上是否存在技术启示。
// 覆盖技术启示的五种情形、反向教导规则、跨领域结合标准、改进动机分析。
func step3TechnicalSuggestionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step2Output, ok := state[stateKeyStep2].(string)
		if !ok {
			step2Output = ""
		}

		prompt := "你是一名资深专利审查员。请执行三步法创造性评估的第 3 步：判断现有技术整体上是否给出了技术启示。\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		input := extractInput(state)
		if input != nil {
			if typeGuidance := inventionTypeGuidance(input.InventionType); typeGuidance != "" {
				prompt += typeGuidance + "\n\n"
			}
			if domainGuidance := techDomainGuidance(input.TechDomain); domainGuidance != "" {
				prompt += domainGuidance + "\n\n"
			}
		}
		prompt += "**发明构思分析**（防止「事后诸葛亮」的关键工具）：\n"
		prompt += "发明构思是发明人为解决技术问题在谋求解决方案过程中提出的技术改进思路，体现在「技术问题→解决思路→技术手段」的脉络中。\n\n"
		prompt += "五步构思比对法：\n"
		prompt += "1. 提炼专利的发明构思（技术问题→解决思路→技术手段）\n"
		prompt += "2. 找到最接近现有技术的发明构思\n"
		prompt += "3. 比较两者在基本工作原理、改进途径、核心设计思想上是否存在本质差异\n"
		prompt += "4. 如果构思迥异甚至相反，分析该差异是否构成技术结合的障碍\n"
		prompt += "5. 即使属于替代方案，也应从构思差异入手进行具体分析\n\n"
		prompt += "关键原则：发明构思不同时，形式相似 ≠ 技术启示。构思差异往往直接影响改进动机——不同构思意味着改进方向和路径根本不同。\n\n"
		prompt += "**技术启示的五种情形**（逐一排除）：\n"
		prompt += "1. 区别特征属于本领域公知常识（惯用手段、教科书/技术词典/技术手册记载）？\n"
		prompt += "2. 区别特征在同一对比文件的其他部分已披露，且作用相同？\n"
		prompt += "3. 区别特征在另一份对比文件中已披露，且作用相同？\n"
		prompt += "4. 其他对比文件披露了功能类似但形式不同的技术手段，可通过公知变化或原理改型获得？\n"
		prompt += "5. 出于解决领域公认问题或满足普遍需求（更便宜/更快/更耐久）的动机？\n\n"
		prompt += "**特殊规则**：\n"
		prompt += "- 对比文件给出反向教导（明确教导不要采用该技术手段）→ 不存在技术启示\n"
		prompt += "- 对比文件之间存在结合障碍（功能冲突、原理矛盾）→ 不存在技术启示\n"
		prompt += "- 跨领域结合（对比文件与申请分属不同技术领域）→ 需要有更充分的理由才可认定存在启示\n"
		prompt += "- 区别特征在对比文件中的作用与在本发明中不同 → 不存在技术启示\n"
		prompt += "- 禁止「事后诸葛亮」式分析（不得在知晓发明后反向推导）\n\n"
		prompt += examinerErrorPrevention() + "\n\n"
		prompt += "**分析推理与有限试验**：\n"
		prompt += "如果本领域技术人员仅通过合乎逻辑的分析推理或有限的试验即可得到发明 → 显而易见。\n"
		prompt += "- 分析推理：推理链条必须严密，不能有跳跃。公知功能相同手段的替换/选择→显而易见\n"
		prompt += "- 有限试验（「有限」不专指数量，综合考量手段/难度/强度）：\n"
		prompt += "  少数可选方案+公知验证手段→有限试验；现有技术给出具体数值起点+知晓调整方向→有限试验\n"
		prompt += "  教导模糊、需大量摸索→超出有限试验→支持创造性\n"
		prompt += "  大量可能组合+无指引缩小范围→非有限试验→支持创造性\n\n"
		prompt += "**用途限定特征的创造性判断**：\n"
		prompt += "- 仅在于使用环境或用途的限定，未带来产品结构/组成/方法改变→通常不影响创造性判断\n"
		prompt += "- 用途限定隐含了产品具有特定结构或性能→该隐含特征应在创造性判断中予以考虑\n"
		prompt += "- 「同类性」前提：对比前应先证明对比文件之间的同类性和作用一致性\n\n"
		prompt += "**改进动机三维度分析**：\n"
		prompt += "1. 发现技术问题的难易程度：问题原因超出本领域技术水平→无改进动机；原因已为公知常识→有动机\n"
		prompt += "2. 不同现有技术结合的动机：无法预见结合结果或结合会破坏原有系统→无结合动机；\n"
		prompt += "   技术发展趋势长期排斥该技术方案→无动机；普遍需求+明确技术指引→有动机\n"
		prompt += "3. 最接近现有技术教导的改进方向：教导了与发明相同的改进方向→增强动机；\n"
		prompt += "   长期未被改进的事实→佐证缺乏动机\n\n"
		prompt += "请输出 JSON 格式：\n"
		prompt += "- technical_suggestion: true/false（是否存在技术启示）\n"
		prompt += "- suggestion_type: 适用情形（common_knowledge/same_doc/other_doc/functional_equivalent/universal_need）\n"
		prompt += "- has_reverse_teaching: true/false（是否存在反向教导）\n"
		prompt += "- is_cross_domain: true/false（是否涉及跨领域结合）\n"
		prompt += "- rationale: 详细推理过程\n"
		prompt += "- confidence: high/medium/low"

		agent := newInventivenessAgent(provider, "inventiveness-step3", prompt, step3Schema())
		defer agent.Close()

		inputText := "第 2 步结论：\n" + step2Output

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step3: %w", err)
		}

		state[stateKeyStep3] = output
		return state, nil
	}
}

// step4SignificantProgressNode 显著的进步判断：评估发明是否具有有益技术效果。
// 创造性 = 突出的实质性特点（Step3：非显而易见） AND 显著的进步（Step4：有益效果）。
func step4SignificantProgressNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step3Output, ok := state[stateKeyStep3].(string)
		if !ok {
			step3Output = ""
		}

		prompt := "你是一名资深专利审查员。请执行创造性评估的「显著的进步」判断：\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "根据专利法第22条第3款，创造性包含两个独立要件：\n"
		prompt += "(1) 突出的实质性特点（非显而易见性，已在第3步判断）；\n"
		prompt += "(2) 显著的进步（有益技术效果，本步骤判断）。\n\n"
		prompt += "**显著的进步四种类型**：\n"
		prompt += "1. 效果改善型：与现有技术相比具有更好的技术效果（质量改善、产量提高、节约能源等）\n"
		prompt += "2. 异途同归型：提供技术构思不同的技术方案，效果基本达到现有技术水平\n"
		prompt += "3. 趋势引领型：代表某种新技术发展趋势\n"
		prompt += "4. 利弊权衡型：某些方面有负面效果，但其他方面具有明显积极的技术效果\n\n"
		prompt += "**重要提示**：\n"
		prompt += "- 即使三步法第3步认定非显而易见，如果发明没有任何有益技术效果，仍不具备创造性\n"
		prompt += "- 「显著的进步」门槛较低：只要具有有益的技术效果（哪怕不是「预料不到」的），通常满足此要件\n"
		prompt += "- 在大多数情况下，非显而易见的发明通常也具有某种有益效果\n\n"
		prompt += "请输出 JSON 格式：\n"
		prompt += "- has_significant_progress: true/false\n"
		prompt += "- progress_type: effect_improve/different_path/trend_leading/tradeoff\n"
		prompt += "- rationale: 判断理由"

		agent := newInventivenessAgent(provider, "inventiveness-step4", prompt, step4Schema())
		defer agent.Close()

		inputText := "第 3 步（技术启示判断）结论：\n" + step3Output

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("step4: %w", err)
		}

		state[stateKeyStep4] = output
		return state, nil
	}
}

// generateConclusionNode 汇总所有步骤的产出，生成最终创造性评估结论。
// 结论逻辑：IsInventive = Step3.NonObvious AND Step4.HasSignificantProgress
func generateConclusionNode(provider agentcore.Provider) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		step1, ok := state[stateKeyStep1].(string)
		if !ok {
			step1 = ""
		}
		step2, ok := state[stateKeyStep2].(string)
		if !ok {
			step2 = ""
		}
		step3, ok := state[stateKeyStep3].(string)
		if !ok {
			step3 = ""
		}
		step4, ok := state[stateKeyStep4].(string)
		if !ok {
			step4 = ""
		}

		prompt := "你是一名资深专利审查员。请基于创造性评估各步骤的产出，生成最终的结构化评估结论。\n\n"
		prompt += personSkilledDefinition() + "\n\n"
		prompt += "**判断逻辑**：创造性 = 突出的实质性特点（Step3 非显而易见） AND 显著的进步（Step4 有益效果）\n\n"
		prompt += "结论应包含：\n"
		prompt += "1. 整体判断：该技术方案是否具备创造性\n"
		prompt += "2. 是否具有显著的进步（基于 Step4 结论）\n"
		prompt += "3. 置信度：high/medium/low\n"
		prompt += "4. 辅助考虑因素（如有）：商业成功、预料不到的技术效果、长期需求等\n\n"
		prompt += confidenceCalibration() + "\n\n"
		prompt += "**特别注意**：预料不到的技术效果是创造性的充分条件而非必要条件。\n"
		prompt += "三步法已认定非显而易见且具有有益效果的，不需强调预料不到的效果。\n"
		prompt += "不能以「不具有预料不到的技术效果」为由得出不具备创造性的结论。\n\n"
		prompt += "请输出 JSON 格式：\n"
		prompt += "- conclusion: 整体结论\n"
		prompt += "- is_inventive: true/false\n"
		prompt += "- has_significant_progress: true/false\n"
		prompt += "- confidence: high/medium/low\n"
		prompt += "- aux_factors: 辅助考虑因素（可选）"

		agent := newInventivenessAgent(provider, "inventiveness-conclusion", prompt, conclusionSchema())
		defer agent.Close()

		inputText := fmt.Sprintf("第 1 步（最接近现有技术）:\n%s\n\n第 2 步（区别特征与技术问题）:\n%s\n\n第 3 步（技术启示判断）:\n%s\n\n第 4 步（显著的进步）:\n%s",
			step1, step2, step3, step4)

		output, err := agent.Run(ctx, inputText)
		if err != nil {
			return state, fmt.Errorf("conclusion: %w", err)
		}

		result := buildResult(step1, step2, step3, step4, output)

		state[StateKeyResult] = result
		return state, nil
	}
}

// =============================================================================
// JSON Schema 定义（对标 enablement 的 Schema 函数）
// =============================================================================

// step1Schema 三步法第 1 步的 JSON Schema。
func step1Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"closest_prior_art": map[string]any{"type": "string"},
			"selection_reason":  map[string]any{"type": "string"},
		},
		"required": []string{"closest_prior_art", "selection_reason"},
	}
}

// step2Schema 三步法第 2 步的 JSON Schema。
func step2Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"distinguishing_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"non_contributing_features": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"tech_effects": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"actual_tech_problem": map[string]any{"type": "string"},
		},
		"required": []string{"distinguishing_features", "actual_tech_problem"},
	}
}

// step3Schema 三步法第 3 步的 JSON Schema（扩展版：五种情形+反向教导+跨领域）。
func step3Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"technical_suggestion": map[string]any{"type": "boolean"},
			"suggestion_type": map[string]any{
				"type": "string",
				"enum": []string{"common_knowledge", "same_doc", "other_doc", "functional_equivalent", "universal_need"},
			},
			"has_reverse_teaching": map[string]any{"type": "boolean"},
			"is_cross_domain":      map[string]any{"type": "boolean"},
			"rationale":            map[string]any{"type": "string"},
			"confidence": map[string]any{
				"type": "string",
				"enum": []string{"high", "medium", "low"},
			},
		},
		"required": []string{"technical_suggestion", "rationale", "confidence"},
	}
}

// step4Schema 显著的进步判断的 JSON Schema。
func step4Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"has_significant_progress": map[string]any{"type": "boolean"},
			"progress_type": map[string]any{
				"type": "string",
				"enum": []string{"effect_improve", "different_path", "trend_leading", "tradeoff"},
			},
			"rationale": map[string]any{"type": "string"},
		},
		"required": []string{"has_significant_progress", "rationale"},
	}
}

// conclusionSchema 最终结论的 JSON Schema（扩展版：含 has_significant_progress）。
func conclusionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conclusion":               map[string]any{"type": "string"},
			"is_inventive":             map[string]any{"type": "boolean"},
			"has_significant_progress": map[string]any{"type": "boolean"},
			"confidence": map[string]any{
				"type": "string",
				"enum": []string{"high", "medium", "low"},
			},
			"aux_factors": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"conclusion", "is_inventive", "has_significant_progress", "confidence"},
	}
}

// =============================================================================
// 解析函数（对标 enablement 的 parse 函数）
// =============================================================================

// parseStep1 从 LLM 输出解析 Step1Result。
func parseStep1(output string) Step1Result {
	r := Step1Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.SelectionReason = output
		return r
	}

	var parsed struct {
		ClosestPriorArt string `json:"closest_prior_art"`
		SelectionReason string `json:"selection_reason"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.SelectionReason = output
		return r
	}

	r.ClosestPriorArt = parsed.ClosestPriorArt
	r.SelectionReason = parsed.SelectionReason
	return r
}

// parseStep2 从 LLM 输出解析 Step2Result（含无贡献特征）。
func parseStep2(output string) Step2Result {
	r := Step2Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return r
	}

	var parsed struct {
		DistinguishingFeatures  []string `json:"distinguishing_features"`
		NonContributingFeatures []string `json:"non_contributing_features"`
		TechEffects             []string `json:"tech_effects"`
		ActualTechProblem       string   `json:"actual_tech_problem"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return r
	}

	r.DistinguishingFeatures = parsed.DistinguishingFeatures
	r.NonContributingFeatures = parsed.NonContributingFeatures
	r.TechEffects = parsed.TechEffects
	r.ActualTechProblem = parsed.ActualTechProblem
	return r
}

// parseStep3 从 LLM 输出解析 Step3Result（扩展版）。
func parseStep3(output string) Step3Result {
	r := Step3Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Rationale = output
		return r
	}

	var parsed struct {
		TechnicalSuggestion bool   `json:"technical_suggestion"`
		SuggestionType      string `json:"suggestion_type"`
		HasReverseTeaching  bool   `json:"has_reverse_teaching"`
		IsCrossDomain       bool   `json:"is_cross_domain"`
		Rationale           string `json:"rationale"`
		Confidence          string `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Rationale = output
		return r
	}

	r.TechnicalSuggestion = parsed.TechnicalSuggestion
	r.SuggestionType = parsed.SuggestionType
	r.HasReverseTeaching = parsed.HasReverseTeaching
	r.IsCrossDomain = parsed.IsCrossDomain
	r.Rationale = parsed.Rationale
	r.Confidence = parsed.Confidence
	switch r.Confidence {
	case "high", "medium", "low":
	default:
		r.Confidence = "medium"
	}
	return r
}

// parseStep4 从 LLM 输出解析 Step4Result。
func parseStep4(output string) Step4Result {
	r := Step4Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.Rationale = output
		return r
	}

	var parsed struct {
		HasSignificantProgress bool   `json:"has_significant_progress"`
		ProgressType           string `json:"progress_type"`
		Rationale              string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.Rationale = output
		return r
	}

	r.HasSignificantProgress = parsed.HasSignificantProgress
	r.ProgressType = parsed.ProgressType
	r.Rationale = parsed.Rationale
	return r
}

// parseConclusion 从 LLM 输出解析最终结论。
func parseConclusion(output string) parsedConclusion {
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		return parsedConclusion{
			Conclusion: output,
			Confidence: "medium",
		}
	}

	var parsed parsedConclusion
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return parsedConclusion{
			Conclusion: output,
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

// buildResult 从各步 LLM 输出构建完整的 InventivenessResult。
// 结论逻辑：IsInventive = Step3.NonObvious AND Step4.HasSignificantProgress
func buildResult(step1, step2, step3, step4, conclusion string) *InventivenessResult {
	result := &InventivenessResult{
		Assessed: true,
	}

	// Parse individual step results.
	result.Step1 = parseStep1(step1)
	result.Step2 = parseStep2(step2)
	result.Step3 = parseStep3(step3)
	result.Step4 = parseStep4(step4)

	// Build backward-compatible ThreeStep summary.
	result.ThreeStep = ThreeStepResult{
		ClosestPriorArt:        result.Step1.ClosestPriorArt,
		DistinguishingFeatures: result.Step2.DistinguishingFeatures,
		ActualTechProblem:      result.Step2.ActualTechProblem,
		TechnicalSuggestion:    result.Step3.TechnicalSuggestion,
		SuggestionRationale:    result.Step3.Rationale,
	}

	// Parse final conclusion.
	cc := parseConclusion(conclusion)
	result.Conclusion = cc.Conclusion
	result.Confidence = cc.Confidence
	result.AuxFactors = cc.AuxFactors

	// 核心结论逻辑：IsInventive = NonObvious AND HasSignificantProgress
	// 优先使用 LLM conclusion 节点的综合判断，其次使用 Step3 + Step4 的计算结果
	switch {
	case cc.IsInventive:
		// LLM 综合判断为具备创造性
		result.IsInventive = true
	case cc.HasSignificantProgress:
		// LLM 认定有显著进步，但综合判断 is_inventive=false（可能因非显而易见性不成立等因素否决）
		result.IsInventive = false
	default:
		// 默认逻辑：非显而易见 AND 有显著进步 → 具备创造性
		result.IsInventive = !result.Step3.TechnicalSuggestion && result.Step4.HasSignificantProgress
	}

	return result
}

// =============================================================================
// Agent 工厂函数
// =============================================================================

// newInventivenessAgent 创建统一配置的 Agent 节点。
// 所有三步法 LLM 节点共享 Temperature=0.2 和 MaxTurns=1，仅 name/prompt/schema 不同。
func newInventivenessAgent(provider agentcore.Provider, name, prompt string, schema map[string]any) *agentcore.Agent {
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

// =============================================================================
// 辅助函数
// =============================================================================

// extractInput 从 state 中安全读取已验证的 InventivenessInput。
// loadInputNode 已确保类型正确且 EvidenceCoverage != "none"，此处仅做防御性断言。
func extractInput(state graph.PregelState) *InventivenessInput {
	raw, ok := state[StateKeyInput]
	if !ok {
		return nil
	}
	input, ok := raw.(*InventivenessInput)
	if !ok {
		return nil
	}
	return input
}

// stateHasSkip 检查 loadInputNode 是否设置了跳过标志。
func stateHasSkip(state graph.PregelState) bool {
	raw, ok := state[StateKeyResult]
	if !ok {
		return false
	}
	r, ok := raw.(*InventivenessResult)
	return ok && r != nil && r.Skipped
}

// buildInputText 将结构化输入格式化为 LLM 友好的 Markdown 文本。
func buildInputText(input *InventivenessInput) string {
	var sb strings.Builder
	if input == nil {
		return ""
	}

	if len(input.Features) > 0 {
		sb.WriteString("## 技术特征\n")
		for _, f := range input.Features {
			fmt.Fprintf(&sb, "- [%s] %s (%s)\n", f.Category, f.Description, f.Importance)
		}
		sb.WriteString("\n")
	}

	if len(input.PriorArtChunks) > 0 {
		sb.WriteString("## 现有技术证据\n")
		for i, c := range input.PriorArtChunks {
			fmt.Fprintf(&sb, "[%d] %s\n    %s\n\n", i+1, c.Title, c.Snippet)
		}
	}

	if len(input.PFETriples) > 0 {
		sb.WriteString("## PFE 三元组（问题→特征→效果因果链）\n")
		for _, t := range input.PFETriples {
			fmt.Fprintf(&sb, "- [%s] 问题: %s → 效果: %s\n", t.ID, t.Problem, t.Effect)
		}
		sb.WriteString("\n")
	}

	if input.NoveltyConclusion != "" {
		fmt.Fprintf(&sb, "## 新颖性初判结论\n%s\n\n", input.NoveltyConclusion)
	}

	return sb.String()
}

// =============================================================================
// 迭代二：公共提示词片段
// =============================================================================

// personSkilledDefinition 返回「本领域的技术人员」标准定义。
// 嵌入各 LLM 节点提示词中，确保判断标准一致性。
func personSkilledDefinition() string {
	return strings.Join([]string{
		"**判断主体：「本领域的技术人员」**",
		"一种假设的「人」，具有以下属性：",
		"- 知晓申请日/优先权日之前所属技术领域所有的普通技术知识",
		"- 能够获知该领域中所有的现有技术",
		"- 具有应用该日期之前常规实验手段的能力",
		"- 不具有创造能力",
		"- 如果技术问题促使其在其他技术领域寻找技术手段，也具有从其他技术领域获知的能力",
	}, "\n")
}

// inventionTypeGuidance 根据发明类型返回针对性的判断指导。
func inventionTypeGuidance(inventionType string) string {
	switch inventionType {
	case InventionTypePioneering:
		return strings.Join([]string{
			"**开拓性发明特殊规则**：",
			"- 开拓性发明是指技术史上未曾有过先例、为人类科学技术开创了新纪元的技术方案",
			"- 开拓性发明原则上均具备创造性，技术启示的检查标准应适当降低",
			"- 重点确认该发明是否确实属于开拓性（而非仅相对于检索到的现有技术是新的）",
		}, "\n")
	case InventionTypeCombination:
		return strings.Join([]string{
			"**组合发明特殊规则**：",
			"- 关键判断因素：组合后的技术效果是否产生了协同作用（1+1>2）",
			"- 简单叠加（各自以常规方式工作、总效果为各部分之和、功能上无相互作用）→ 不具备创造性",
			"- 有机组合（功能上彼此支持、取得新的技术效果、效果优于各部分之和）→ 可能具备创造性",
			"- 需检查现有技术中是否存在组合的启示或教导",
		}, "\n")
	case InventionTypeSelection:
		return strings.Join([]string{
			"**选择发明特殊规则**：",
			"- 关键判断因素：选择是否产生了预料不到的技术效果",
			"- 从已知可能性中进行的常规选择、常规尺寸/温度范围选择 → 不具备创造性",
			"- 可从现有技术直接推导的选择 → 不具备创造性",
			"- 选择产生了预料不到的技术效果 → 具备创造性",
		}, "\n")
	case InventionTypeTransfer:
		return strings.Join([]string{
			"**转用发明特殊规则**：",
			"- 关键判断因素：转用的领域远近、是否克服了原领域未曾遇到的困难",
			"- 类似或相近技术领域之间转用、未产生预料不到的技术效果 → 不具备创造性",
			"- 跨领域转用且产生预料不到的技术效果或克服了原领域未曾遇到的困难 → 具备创造性",
		}, "\n")
	case InventionTypeNewUse:
		return strings.Join([]string{
			"**已知产品新用途发明特殊规则**：",
			"- 关键判断因素：新用途是否利用了已知产品新发现的性质",
			"- 新用途仅使用已知材料的已知性质 → 不具备创造性",
			"- 新用途利用了已知产品新发现的性质并产生预料不到的技术效果 → 具备创造性",
		}, "\n")
	case InventionTypeElementChange:
		return strings.Join([]string{
			"**要素变更发明特殊规则**：",
			"- 要素关系改变：改变未导致效果/功能/用途变化或变化可预料 → 不具备创造性",
			"- 要素替代：相同功能的已知手段等效替代 → 不具备创造性；产生预料不到效果 → 具备创造性",
			"- 要素省略：省略后功能相应消失 → 不具备创造性；省略后保持全部原有功能 → 具备创造性",
		}, "\n")
	default:
		return ""
	}
}

// techDomainGuidance 根据技术领域返回针对性的判断指导。
func techDomainGuidance(domain string) string {
	switch domain {
	case "chemistry":
		return "**化学领域特殊规则**：化合物创造性需分析结构差异→用途/效果→技术启示；" +
			"结构接近的化合物若改造不能带来预料不到效果则显而易见；" +
			"电子等排体（NH2/CH3、-O-/-NH-、-S-/-O-等）仅置换且效果相当→无创造性。"
	case "computer":
		return "**计算机/AI领域特殊规则**：应将技术特征与功能上彼此相互支持、存在相互作用关系的算法特征作为整体考虑；" +
			"算法应用于具体技术领域解决技术问题→支持创造性；" +
			"已知算法的简单替换或常规计算机实现→不充分支持创造性。"
	case "tcm":
		return "**中药领域特殊规则**：以君臣佐使组方结构为核心，从理、法、方、药四层面分析区别特征；" +
			"药味增减/替换/药量加减如仅遵循一般组方规律→无创造性；" +
			"合方发明如仅为简单叠加且效果加和→无创造性。"
	default:
		return ""
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// examinerErrorPrevention 返回审查员常见错误防范指令。
func examinerErrorPrevention() string {
	return strings.Join([]string{
		"**常见审查错误防范**：",
		"1. 「事后诸葛亮」：不得在知晓发明内容后反向推导显而易见性 ✅",
		"2. 技术特征割裂：不得将功能上相互支持、存在协同作用的特征拆分为独立特征逐一评价",
		"3. 「本领域技术人员」标准把握不当：",
		"   - 标准过高（使用专家级知识要求）→ 低估创造性",
		"   - 标准过低（使用外行标准）→ 高估创造性",
		"   - 跨领域判断失当（错误假定技术人员具有跨领域知识）→ 需谨慎",
		"4. 忽视技术效果的整体性：注意特征之间的协同作用和组合后整体技术效果",
	}, "\n")
}

// confidenceCalibration 返回基于实务统计数据的置信度校准指引。
func confidenceCalibration() string {
	return strings.Join([]string{
		"**置信度校准参考**（基于39,496份复审/无效决定的实证数据）：",
		"- 「单对比文件+公知常识」路径：占无效决定的54%，成功率95.9% → 若沿此路径认定不具备创造性，置信度应为 high",
		"- 「多对比文件结合」路径：占14.3%，成功率73.7% → 置信度应适中（medium），需充分论证结合动机",
		"- 「惯用手段与常规选择」路径：占0.1%，成功率68.8% → 需充分举证该手段确实属于惯用/常规",
		"- 「预料不到的技术效果」抗辩：维持有效率仅5.7% → 如依赖此因素认定具备创造性，需极为审慎，要求充分的实验数据支撑",
	}, "\n")
}

// extractJSON 从文本中提取第一个 JSON 对象字符串。
func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}
