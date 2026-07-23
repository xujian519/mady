package specdrafting

import "strings"

// =============================================================================
// 充分公开规则：五种"不能实现"情形检查
//
// 法律依据：
//   专利法第26条第3款——说明书应当对发明或者实用新型作出清楚、完整的说明，
//   以所属技术领域的技术人员能够实现为准。
//
// 审查指南第二部分第二章§2.1.3 列举的五种"不能实现"情形：
//   1. 只给出任务和/或设想，未给出任何使能技术手段
//   2. 给出了技术手段但含糊不清，无法具体实施
//   3. 所述手段无法解决所述技术问题
//   4. 方案中的某个技术手段本身无法实现
//   5. 有技术方案但无实验证据，而方案必须依赖实验结果证实（化学领域）
//
// 本文件覆盖情形1→2 的自动检测。
// =============================================================================

// enablementMeansExistRule 检查发明内容是否仅描述目标/效果而无具体技术手段。
//
// 对应审查指南情形1（只给任务/设想，未给技术手段）
// 和情形2（手段含糊不清，无法具体实施）。
//
// 检测策略：如果发明内容包含问题描述和效果描述，但缺乏具体技术手段
// 关键词（如包含/包括/由…组成/步骤/特征在于等），则判为违规。
type enablementMeansExistRule struct{ baseRule }

func (r *enablementMeansExistRule) Check(spec *SpecOutput, _ SpecInput) []Violation {
	if spec == nil {
		return nil
	}
	content := findSection(spec, SecContent)
	if content == "" {
		return nil
	}

	// 检测是否仅描述问题/愿望而无具体手段
	hasConcreteMeans := containsAnyOf(content, []string{
		"包括", "包含", "由...组成", "由以下组成",
		"技术方案如下", "采用如下技术方案",
		"步骤", "特征在于",
		"设置", "连接", "固定", "安装",
		"装置", "机构", "系统", "模块",
	})
	hasProblemGoal := containsAnyOf(content, []string{
		"技术问题", "要解决", "目的在于", "本发明的目的",
	})
	hasEffect := containsAnyOf(content, []string{
		"有益效果", "技术效果", "优点", "进步",
	})

	// 只有愿望/效果描述而无具体技术手段 → 违规
	if (hasProblemGoal || hasEffect) && !hasConcreteMeans {
		return []Violation{{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityError,
			SectionName: string(SecContent),
			Message:     "发明内容仅描述了技术问题或有益效果，未给出任何具体的使能技术手段",
			Suggestion:  "在发明内容中增加具体的技术方案描述，包括构成发明的必要技术特征及其结构或步骤关系",
		}}
	}

	// 如果技术方案部分只有高度笼统的表述，缺乏可实施细节
	hasVagueMeans := !containsAnyOf(content, []string{
		"包括", "包含",
		"连接", "设置", "固定", "安装", "组成",
		"所述", "该", "特征在于",
	})
	if hasProblemGoal && hasVagueMeans && ChineseCharCount(content) < 100 {
		return []Violation{{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityWarning,
			SectionName: string(SecContent),
			Message:     "技术方案的描述过于笼统，可能使所属领域技术人员无法具体实施",
			Suggestion:  "请提供更详细的技术方案描述，包括各组成部分的具体结构、连接关系或方法步骤",
		}}
	}

	return nil
}

// enablementExperimentEvidenceRule 检查需实验证据的领域是否提供了实验数据。
//
// 对应审查指南情形5——有技术方案但未给出实验证据，而方案必须依赖实验结果证实。
// 主要针对化学/材料领域，但也适用于其他需要效果验证的领域。
type enablementExperimentEvidenceRule struct{ baseRule }

func (r *enablementExperimentEvidenceRule) Check(spec *SpecOutput, input SpecInput) []Violation {
	if spec == nil {
		return nil
	}

	// 判断是否需要实验证据
	needsExperiment := false
	domain := input.TechDomain
	if domain == DomainChemical {
		needsExperiment = true
	} else {
		// 对于其他领域，如果特征含 material 类别也提示注意
		for _, f := range input.Features {
			if f.Category == "material" {
				needsExperiment = true
				break
			}
		}
	}
	if !needsExperiment {
		return nil
	}

	allText := ""
	for _, sec := range spec.Sections {
		allText += " " + sec.Content
	}
	if allText == "" {
		return nil
	}

	// 检测是否有实验/测试数据描述
	hasEvidence := containsAnyOf(allText, []string{
		"实施例1", "实施例2", "实施例 1", "实施例 2",
		"实验", "试验", "测试", "检测", "分析",
		"结果如表", "结果如图", "数据表明",
		"％", "%", "份", "摩尔",
		"效果", "性能",
	})
	if !hasEvidence {
		return []Violation{{
			RuleName:    r.Name(),
			RuleBasis:   r.LegalBasis(),
			Severity:    SeverityWarning,
			SectionName: string(SecEmbodiment),
			Message:     "化学/材料领域应提供实验数据以证实技术效果，缺少实验证据可能构成公开不充分",
			Suggestion:  "在具体实施方式中补充至少一个实验实施例，包含制备/合成等过程实施例和应用效果实施例，并用数据量化技术效果",
		}}
	}

	// 检查实施例数量是否充足（化学领域至少2个代表性实例）
	if domain == DomainChemical {
		embContent := findSection(spec, SecEmbodiment)
		if embContent != "" && strings.Count(embContent, "实施例") < 2 {
			return []Violation{{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityWarning,
				SectionName: string(SecEmbodiment),
				Message:     "化学领域通常需要至少2个代表性实施例来支持权利要求的范围",
				Suggestion:  "建议增加实施例的数量（至少2个以上），覆盖需要保护的不同技术方案范围",
			}}
		}
	}

	return nil
}

// =============================================================================
// 辅助函数
// =============================================================================

// strings.Count is used in the rule above for counting embodiment occurrences.
// Remove the unused import warning by importing "strings" already at the top.
