package inventiveness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/graph"
)

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
		prompt += "**技术问题确定的五种情形**（判断当前属于哪种，并在结论中标注情形类型）：\n"
		prompt += "情形一（相同现有技术）：最接近现有技术 = 申请人描述的现有技术\n"
		prompt += "  → 技术问题通常与说明书记载相同，但仍需验证区别特征是否确实解决了声称的问题\n"
		prompt += "情形二（不同现有技术，最常见）：最接近现有技术 ≠ 申请人描述的现有技术\n"
		prompt += "  → 需重新确定技术问题，并以区别特征客观上达到的技术效果为准\n"
		prompt += "情形三（领域相同但方案差异大）：技术问题在层次和方向上可能有较大差异\n"
		prompt += "  → 注意概括粒度：过宽（上层抽象）会放大创造性，过窄（下层具体）会缩小创造性\n"
		prompt += "情形四（多特征且功能相互支持）：多个特征协同解决同一技术问题\n"
		prompt += "  → 应整体考虑，确定一个统一的技术问题，不得拆分后分别确定\n"
		prompt += "情形五（所有效果均相当）：区别特征未带来不同于现有技术的技术效果\n"
		prompt += "  → 技术问题确定为「提供一种不同于最接近现有技术的可供选择的技术方案」\n"
		prompt += "  → 此时创造性判断的核心在于发明构思差异（而非技术效果）\n\n"
		prompt += "**⚠️ 技术问题确定的红线规则**：\n"
		prompt += "- 技术问题中不应包含解决手段本身（如应写「如何减少积绒」而非「如何通过减小底面积减少积绒」）\n"
		prompt += "- 技术效果必须是区别特征带来的、且在整个权利要求保护范围内均能实现\n"
		prompt += "- 技术问题确定后，应标注所属情形类型（情形一~五），供第 3 步跨领域判断参考\n\n"
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
		prompt += "**参数特征特殊规则**：\n"
		prompt += "- 对于参数限定的特征（性能参数/物理参数），如果能从结构、组分或制备方法等方面\n"
		prompt += "  确定现有技术产品必然具备相应参数值，则该参数不构成实质区别特征\n"
		prompt += "- 推定规则：当结构和组分决定性能时，参数特征在对比中不予考虑\n\n"
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

// step2Schema 三步法第 2 步的 JSON Schema。
func step2Schema() map[string]any {
	return map[string]any{
		jsFieldType: jsTypeObject,
		jsFieldProperties: map[string]any{
			"distinguishing_features": map[string]any{
				jsFieldType:  jsTypeArray,
				jsFieldItems: map[string]any{jsFieldType: jsTypeString},
			},
			"non_contributing_features": map[string]any{
				jsFieldType:  jsTypeArray,
				jsFieldItems: map[string]any{jsFieldType: jsTypeString},
			},
			"tech_effects": map[string]any{
				jsFieldType:  jsTypeArray,
				jsFieldItems: map[string]any{jsFieldType: jsTypeString},
			},
			"actual_tech_problem": map[string]any{jsFieldType: jsTypeString},
		},
		jsFieldRequired: []string{"distinguishing_features", "actual_tech_problem"},
	}
}

// parseStep2 从 LLM 输出解析 Step2Result（含无贡献特征）。
func parseStep2(output string) Step2Result {
	r := Step2Result{}
	jsonStr := extractJSON(output)
	if jsonStr == "" {
		r.ActualTechProblem = output
		return r
	}

	var parsed struct {
		DistinguishingFeatures  []string `json:"distinguishing_features"`
		NonContributingFeatures []string `json:"non_contributing_features"`
		TechEffects             []string `json:"tech_effects"`
		ActualTechProblem       string   `json:"actual_tech_problem"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		r.ActualTechProblem = output
		return r
	}

	r.DistinguishingFeatures = parsed.DistinguishingFeatures
	r.NonContributingFeatures = parsed.NonContributingFeatures
	r.TechEffects = parsed.TechEffects
	r.ActualTechProblem = parsed.ActualTechProblem
	// 对结构化输出的"实际解决的技术问题"执行原子化四检验（不绑方案/单一因果/可测效果）。
	if r.ActualTechProblem != "" {
		r.ProblemChecks = CheckAtomicProblem(r.ActualTechProblem)
	}
	return r
}
