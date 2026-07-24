package ipc

import (
	"fmt"
	"strings"
)

// GetInventivenessHints 返回特定 IPC 领域的创造性审查要点提示词。
// 这些提示词可作为 LLM 分析节点的 system prompt 增强片段。
func GetInventivenessHints(section IPCSection) string {
	domain, ok := AllDomains[section]
	if !ok {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## IPC 领域特化提示：%s（%s）\n\n", domain.Name, section)
	b.WriteString("### 创造性审查特化要点\n\n")
	for i, hint := range domain.InventivenessFocus {
		fmt.Fprintf(&b, "%d. %s\n", i+1, hint)
	}
	b.WriteString("\n### 本领域技术人员认知边界\n\n")
	b.WriteString("判断主体「本领域的技术人员」对所属技术领域的认知水平需要考虑：\n")
	b.WriteString("- 知晓申请日/优先权日之前所属技术领域所有的普通技术知识\n")
	b.WriteString("- 能够获知该领域中所有的现有技术\n")
	b.WriteString("- 具有应用该日期之前常规实验手段的能力\n")
	b.WriteString("- 不具有创造能力\n\n")

	knowledge := GetCommonKnowledge(section)
	if len(knowledge) > 0 {
		b.WriteString("### 常规公知常识参考\n\n")
		for _, k := range knowledge {
			fmt.Fprintf(&b, "- %s\n", k)
		}
		b.WriteString("\n> 注意：公知常识的认定应以申请日/优先权日之前的普通技术知识为准，\n> 所列内容仅为示例，实际认定需结合具体技术领域的发展水平。\n")
	}

	return b.String()
}

// GetNoveltyHints 返回特定 IPC 领域的新颖性审查要点提示。
func GetNoveltyHints(section IPCSection) string {
	domain, ok := AllDomains[section]
	if !ok {
		return ""
	}

	switch section {
	case IPCC:
		return fmt.Sprintf(`## 化学领域新颖性判断特化提示（%s）

### 化学领域特殊规则

1. **通式化合物**：通式化合物的公开不破坏该通式范围内具体化合物的新颖性，
   除非对比文件明确提到了该具体化合物（即使只是列举）。

2. **异构体**：外消旋混合物的公开不破坏特定对映异构体的新颖性。

3. **晶体**：X射线粉末衍射峰应作为整体特征比对，衍射峰整体不同即具备新颖性。

4. **组合物**：开放式表达（包含/含有）与封闭式表达（由……组成）对新颖性判断有显著影响。

5. **制药用途**：制药用途的新颖性取决于疾病适应症是否与现有技术相同。

6. **参数/性能特征**：参数特征如果隐含了特定结构/组成则具有新颖性，
   否则可推定无新颖性。

7. **Markush 通式**：通式范围内的具体化合物需判断对比文件是否明确提及。
`, domain.Name)
	case IPCH:
		return fmt.Sprintf(`## 电学领域新颖性判断特化提示（%s）

### 电学领域注意要点

1. **电路结构**：电路拓扑结构的差异通常构成新颖性区别。

2. **通信协议**：协议参数的优化通常不影响新颖性判断的客体认定。

3. **信号处理**：信号处理方法的步骤顺序不同可能构成区别技术特征。

4. **半导体**：器件层结构的差异（材料/厚度/掺杂浓度等）属于技术特征。
`, domain.Name)
	case IPCA:
		return fmt.Sprintf(`## 人类生活必需领域新颖性判断特化提示（%s）

### 注意要点

1. **药物组合物**：辅料的选择和配比属于技术特征，不同于活性成分。
2. **医疗方法**：中国专利法第25条不保护疾病的诊断和治疗方法，
   但医疗器械、药物组合物和制药用途可授权。
3. **食品配方**：配方组成的差异是新颖性判断的关键。
`, domain.Name)
	default:
		return fmt.Sprintf(`## %s领域新颖性判断注意要点

按专利法第22条第2款进行新颖性判断时，遵循以下一般原则：

1. 采用单独对比原则，将每项权利要求与一份对比文件进行比对
2. 判断权利要求的技术方案是否被对比文件完整公开
3. 考虑上下位概念关系
4. 注意惯用手段的直接置换
5. 数值范围的8种特殊情形
`, domain.Name)
	}
}

// GetCommonKnowledge 返回该领域的公知常识示例列表。
// 返回的内容仅为示例参考，实际公知常识的认定依赖于具体案件事实和审查实践。
func GetCommonKnowledge(section IPCSection) []string {
	domain, ok := AllDomains[section]
	if !ok {
		return nil
	}
	// 返回副本以防外部修改
	result := make([]string, len(domain.CommonKnowledge))
	copy(result, domain.CommonKnowledge)
	return result
}
