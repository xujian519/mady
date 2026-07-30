package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

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
		prompt += "**现有技术信息质量检查**（在判断技术启示前先确认现有技术的准确理解）：\n"
		prompt += "1. 是否存在对现有技术的局部片面理解？（如仅看最佳实施例而忽略说明书整体教导）\n"
		prompt += "2. 是否需要识别明显错误？（文字笔误、前后不一致，基于整体内容和技术常识判断）\n"
		prompt += "3. 现有技术中的负面描述（如「不宜」「不适合」）是「彻底否定」还是「特定场景下的选择」？\n"
		prompt += "   - 负面描述针对的问题在发明中是否同样存在？\n"
		prompt += "   - 其他现有技术是否仍在使用该技术手段？\n"
		prompt += "4. 上述质量检查结论是否影响对技术启示的判断？\n\n"
		prompt += "**第 1 阶段：发明构思比对与改进动机分析**\n\n"
		prompt += "发明构思是发明人为解决技术问题在谋求解决方案过程中提出的技术改进思路，\n"
		prompt += "体现在「技术问题→解决思路→技术手段」的脉络中。\n\n"
		prompt += "**步骤一：提炼双方的发明构思**\n"
		prompt += "1. 提炼发明（专利/申请）的发明构思：从说明书记载的技术问题和解决思路出发\n"
		prompt += "2. 提炼最接近现有技术的发明构思：从对比文件的整体技术方案理解其核心思路\n\n"
		prompt += "**步骤二：比较构思差异并推断改进动机**\n"
		prompt += "1. 比较两者在基本工作原理、改进途径、核心设计思想上是否存在本质差异\n"
		prompt += "2. 如果构思迥异甚至相反 → 技术人员通常缺乏改进动机 → 趋向于不存在技术启示\n"
		prompt += "3. 如果构思一致或相似 → 技术人员可能存在改进动机 → 进入第 2 阶段深度分析\n"
		prompt += "4. 关键原则：发明构思不同时，形式相似 ≠ 技术启示。构思差异往往直接影响改进动机\n"
		prompt += "5. 即使属于替代方案（解决问题和效果相同），也应从构思差异入手进行具体分析\n\n"
		prompt += "**第 2 阶段：技术启示判断**（基于第 1 阶段的构思比对与改进动机结论）\n\n"
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
		prompt += "**分析推理与有限试验结构化判断**：\n\n"
		prompt += "**1. 分析推理**\n"
		prompt += "- 区别特征是否为公知常识、惯用手段或功能相同的已知手段的等效替换？\n"
		prompt += "  如果是且有合理成功预期 → 推理链条成立 → 趋向显而易见\n"
		prompt += "- 推理链条是否严密、无跳跃？\n"
		prompt += "- 现有技术面临的技术问题是否明确？替代手段是否属于公知的可选方案？\n"
		prompt += "  如果均满足 → 经过合乎逻辑的分析推理即可得到 → 无创造性\n\n"
		prompt += "**2. 有限的试验**\n"
		prompt += "- 「有限」不专指数量的多寡，应综合考量以下三个维度：\n"
		prompt += "  a) 试验手段本身是否属于常规？（公知验证手段 vs 需专门设计的实验）\n"
		prompt += "  b) 试验难度和强度是否属于常规？（可选方案范围大小/参数调整方向清晰度）\n"
		prompt += "  c) 现有技术教导是否充分？（方案方向越明确、结果预判越强→越「有限」）\n"
		prompt += "- 少数可选方案 + 公知验证手段 → 有限试验 → 显而易见\n"
		prompt += "- 现有技术给出具体数值起点 + 知晓调整方向 → 有限试验 → 显而易见\n"
		prompt += "- 教导模糊、需大量摸索 → 超出有限试验 → 支持创造性\n"
		prompt += "- 大量可能组合 + 无现有技术指引缩小范围 → 超出有限试验 → 支持创造性\n\n"
		prompt += "**用途限定特征的创造性判断**：\n"
		prompt += "- 仅在于使用环境或用途的限定，未带来产品结构/组成/方法改变→通常不影响创造性判断\n"
		prompt += "- 用途限定隐含了产品具有特定结构或性能→该隐含特征应在创造性判断中予以考虑\n"
		prompt += "- 「同类性」前提：对比前应先证明对比文件之间的同类性和作用一致性\n\n"
		prompt += "**改进动机三维度系统分析**（逐维度推理后综合结论）：\n\n"
		prompt += "**维度一：发现技术问题的难易程度**\n"
		prompt += "- 现有技术是否存在该技术缺陷或待解决的问题？\n"
		prompt += "- 导致该缺陷的内在原因是否已被本领域技术人员认识？\n"
		prompt += "- 该原因的发现是否超出了申请日前技术人员的能力和水平？\n"
		prompt += "- 如果原因长期未知且超出技术水平 → 即使解决手段本身容易，仍不具备改进动机\n"
		prompt += "→ 结论：有改进动机 / 无改进动机 / 不确定\n\n"
		prompt += "**维度二：不同现有技术结合的动机**\n"
		prompt += "- 将多篇现有技术结合后是否有合理的成功预期？\n"
		prompt += "- 结合是否会破坏原有系统的核心功能或结构？\n"
		prompt += "- 区别特征与其所在整体技术方案是否密不可分（不可拆分移植）？\n"
		prompt += "- 最接近现有技术是否为应用区别特征提供了适格的改进基础？\n"
		prompt += "- 技术发展趋势是积极推动还是反向排斥该技术方案？\n"
		prompt += "→ 结论：有结合动机 / 无结合动机 / 不确定\n\n"
		prompt += "**维度三：技术发展趋势与行业规范的引导作用**\n"
		prompt += "- 该技术在申请日前处于早期发展阶段还是成熟期？\n"
		prompt += "  （早期：可借鉴信息少，需更多独立摸索 → 趋向于无改进动机）\n"
		prompt += "- 技术发展趋势/行业规范是否明确支持该改进方向？\n"
		prompt += "- 发展趋势是否在较长时间内排斥该技术方案（给出相反启示）？\n"
		prompt += "→ 结论：正向引导（支持改进）/ 反向排斥（阻碍改进）/ 中性\n\n"
		prompt += "**改进动机综合判断**：基于以上三个维度的系统分析，综合判断本领域技术人员\n"
		prompt += "是否存在动机将区别特征应用于最接近现有技术以获得发明。\n"
		prompt += "三个维度中任一维度明确「无动机」时，应审慎认定存在技术启示。\n\n"
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

// step3Schema 三步法第 3 步的 JSON Schema（扩展版：五种情形+反向教导+跨领域）。
func step3Schema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"technical_suggestion": map[string]any{jsFieldType: jsTypeBoolean},
			"suggestion_type": map[string]any{
				jsFieldType: jsTypeString,
				jsFieldEnum: []string{"common_knowledge", "same_doc", "other_doc", "functional_equivalent", "universal_need"},
			},
			"has_reverse_teaching": map[string]any{jsFieldType: jsTypeBoolean},
			"is_cross_domain":      map[string]any{jsFieldType: jsTypeBoolean},
			jsFieldRationale:       map[string]any{jsFieldType: jsTypeString},
			jsFieldConfidence: map[string]any{
				jsFieldType: jsTypeString,
				jsFieldEnum: []string{jsValHigh, jsValMedium, jsValLow},
			},
		},
		jsFieldRequired: []string{"technical_suggestion", jsFieldRationale, jsFieldConfidence},
	}
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
	case jsValHigh, jsValMedium, jsValLow:
	default:
		r.Confidence = jsValMedium
	}
	return r
}
