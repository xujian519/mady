package patent

import (
	"fmt"
	"strings"
)

// prepareTechComparisonTable generates a Markdown table comparing
// claim technical features against cited documents from the rejection decision.
func prepareTechComparisonTable(grounds []ReexamGround, info ReexamDecisionInfo) string {
	var b strings.Builder

	b.WriteString("### 权利要求技术特征与对比文件对比表\n\n")
	b.WriteString("| 序号 | 权利要求技术特征 | 对比文件对应特征 | 是否公开 | 分析结论 |\n")
	b.WriteString("|------|------------------|-----------------|----------|----------|\n")
	b.WriteString("| 1 | （原权利要求特征1） | （对比文件公开的特征） | 公开/未公开 | 具备新颖性/需修改 |\n")
	b.WriteString("| 2 | （原权利要求特征2） | （对比文件公开的特征） | 公开/未公开 | 具备新颖性/需修改 |\n")
	b.WriteString("| 3 | （原权利要求特征3） | （对比文件公开的特征） | 公开/未公开 | 具备新颖性/需修改 |\n")

	b.WriteString("\n**说明**：上表中每一行对应一个技术特征。请根据权利要求的实际技术方案逐项填写具体内容。\n")

	// Add amendment comparison section if relevant
	for _, g := range grounds {
		if g.Type == ReexamGroundAmendment {
			b.WriteString("\n### 修改前后特征对比\n\n")
			b.WriteString("| 修改前 | 修改后 | 原申请文件出处 | 是否超范围 |\n")
			b.WriteString("|--------|--------|----------------|----------|\n")
			b.WriteString("| （修改前内容） | （修改后内容） | （原说明书/权利要求书位置） | 未超范围 |\n")
			b.WriteString("\n**说明**：如已提交修改替换页，请在此逐项列明修改前后的内容对比及相应的原申请文件支持依据。\n")
			break
		}
	}

	if info.CitedDocs != "" {
		b.WriteString("\n### 引用对比文件清单\n\n")
		b.WriteString(info.CitedDocs)
		b.WriteString("\n")
	}

	return b.String()
}

// prepareAmendmentNonExtensionArgument generates a written argument
// on whether claim amendments comply with Article 33 (no extension beyond
// original disclosure). If no amendment ground exists, returns a placeholder.
func prepareAmendmentNonExtensionArgument(grounds []ReexamGround) string {
	var b strings.Builder

	hasAmendment := false
	for _, g := range grounds {
		if g.Type == ReexamGroundAmendment {
			hasAmendment = true
			break
		}
	}

	if !hasAmendment {
		b.WriteString("（本案驳回理由不涉及修改超范围问题。如复审过程中提交了修改替换页，仍需准备修改不超范围的书面论证。）\n\n")
		b.WriteString("### 论证框架（如需）\n\n")
		b.WriteString("1. **修改内容识别**：逐项列明修改内容\n")
		b.WriteString("2. **原申请文件支持依据**：指出每处修改在原说明书和/或权利要求书中的记载位置\n")
		b.WriteString("3. **论证结论**：修改未超出原说明书和权利要求书记载的范围，符合专利法第33条的规定\n\n")
		return b.String()
	}

	b.WriteString("### 修改内容概述\n\n")
	b.WriteString("（请在此列明复审阶段提交的修改替换页中的具体修改内容）\n\n")

	b.WriteString("### 修改依据与出处\n\n")
	b.WriteString("| 修改项 | 修改后内容 | 原申请文件依据 | 依据位置 |\n")
	b.WriteString("|--------|------------|----------------|----------|\n")
	b.WriteString("| 修改项1 | （修改后文本） | （原说明书/权利要求书原文） | （段落/行号） |\n")
	b.WriteString("| 修改项2 | （修改后文本） | （原说明书/权利要求书原文） | （段落/行号） |\n")
	b.WriteString("| 修改项3 | （修改后文本） | （原说明书/权利要求书原文） | （段落/行号） |\n")

	b.WriteString("\n### 法律论证\n\n")
	b.WriteString("根据专利法第33条和《专利审查指南》第二部分第八章第5.2节的规定，对申请文件的修改不得超出原说明书和权利要求书记载的范围。\n\n")
	b.WriteString("**判断标准**：修改内容是否能够从原说明书和权利要求书直接且毫无疑义地确定。\n\n")
	b.WriteString("**论证要点**：\n")
	b.WriteString("1. 上述修改内容在原申请文件中有明确记载，属于**直接且毫无疑义地确定**的内容\n")
	b.WriteString("2. 修改仅限于**消除驳回决定所指出的缺陷**，符合专利法实施细则第60条第1款关于复审修改范围的限制\n")
	b.WriteString("3. 修改未引入新的技术内容，未扩大保护范围\n\n")

	b.WriteString("### 结论\n\n")
	b.WriteString("综上所述，本次复审修改符合专利法第33条和专利法实施细则第60条第1款的规定，未超出原说明书和权利要求书记载的范围。\n")

	return b.String()
}

// preparePossibleChallenges generates a preview of questions the examiner
// panel may raise during the oral hearing, organized by rejection ground.
func preparePossibleChallenges(grounds []ReexamGround) string {
	var b strings.Builder

	b.WriteString("基于驳回决定中的审查意见，以下为合议组在口审中可能提出的质疑点预演：\n\n")

	for i, g := range grounds {
		fmt.Fprintf(&b, "### 质疑 %d：%s（%s）\n\n", i+1, g.Description, g.Article)

		switch g.Type {
		case ReexamGroundNovelty:
			b.WriteString("- Q1: 区别技术特征是否已被对比文件**隐含公开**？\n")
			b.WriteString("- Q2: 该区别技术特征是否属于本领域的**惯用手段**或**公知常识**？\n")
			b.WriteString("- Q3: 如果将对比文件与其他公知文献结合，是否可以得出权利要求的技术方案？\n")
			b.WriteString("- Q4: 请求人是否提交了修改后的权利要求？修改是否足以克服新颖性缺陷？\n")
			b.WriteString("\n**准备方向**：逐项对比技术特征，准备充分的技术对比证据，强调未被公开的区别特征。\n\n")

		case ReexamGroundInventiveness:
			b.WriteString("- Q1: 区别技术特征**实际解决的技术问题**是否被正确认定？\n")
			b.WriteString("- Q2: 现有技术整体上是否存在将区别特征应用到最接近现有技术的**技术启示**？\n")
			b.WriteString("- Q3: 技术效果是否为**可预料**的技术效果？是否存在**预料不到的技术效果**？\n")
			b.WriteString("- Q4: 辅助审查因素（商业成功、长期需求、他人失败）在本案中是否成立？\n")
			b.WriteString("- Q5: 审查员是否考虑了**正确的三步法**分析框架？\n")
			b.WriteString("\n**准备方向**：重点准备三步法反驳，尤其是技术启示的论证，准备实验数据证明预料不到的技术效果。\n\n")

		case ReexamGroundDisclosure:
			b.WriteString("- Q1: 说明书记载的内容是否足以使本领域技术人员**能够实现**该发明？\n")
			b.WriteString("- Q2: 哪些技术手段属于本领域技术人员的**常规实验范围**？说明书是否需要进一步细化？\n")
			b.WriteString("- Q3: 如涉及参数或效果数据，这些数据是否足以证明技术方案的可实现性？\n")
			b.WriteString("- Q4: 说明书中是否遗漏了实现发明所必需的技术内容？\n")
			b.WriteString("\n**准备方向**：准备补充实验数据或理论论证，证明本领域技术人员按说明书教导能够实现该发明。\n\n")

		case ReexamGroundClarity:
			b.WriteString("- Q1: 权利要求中的特定术语在说明书中有无**明确定义**或示例？\n")
			b.WriteString("- Q2: 权利要求的保护范围在阅读说明书和附图后是否能够**合理确定**？\n")
			b.WriteString("- Q3: 从属权利要求之间的引用关系是否存在不清楚之处？\n")
			b.WriteString("- Q4: 权利要求是否得到了说明书的**支持**（即权利要求范围与说明书公开的内容相适应）？\n")
			b.WriteString("\n**准备方向**：逐一确认每个有争议术语在说明书中的定义或示例，必要时提交修改后的权利要求。\n\n")

		case ReexamGroundAmendment:
			b.WriteString("- Q1: 修改内容是否能从原说明书和权利要求书中**直接且毫无疑义地**确定？\n")
			b.WriteString("- Q2: 修改是否引入了原申请文件**未记载**的技术内容？\n")
			b.WriteString("- Q3: 修改是否属于复审程序中允许的修改类型（**仅限于消除驳回缺陷**）？\n")
			b.WriteString("- Q4: 修改是否导致了保护范围的**扩大**？\n")
			b.WriteString("\n**准备方向**：准备修改前后的文本逐项对比，标注每处修改在原申请文件中的出处。\n\n")

		case ReexamGroundSubject:
			b.WriteString("- Q1: 权利要求限定的技术方案是否属于对产品的**形状、构造或其结合**？\n")
			b.WriteString("- Q2: 是否存在不属于实用新型保护客体的**方法、功能或材料**特征？\n")
			b.WriteString("- Q3: 如果存在非客体特征，能否通过修改将其删除或限缩？\n")
			b.WriteString("\n**准备方向**：准备基于产品结构特征的论证，考虑删除或限缩非客体特征。\n\n")
		}
	}

	b.WriteString("### 应考策略\n\n")
	b.WriteString("- **书面意见准备**：将上述质疑点的书面应答提前准备完整，口审时可提交书面补充意见\n")
	b.WriteString("- **证据准备**：准备好所有对比文件的技术特征对照表及关键段落摘录\n")
	b.WriteString("- **修改方案备选**：准备至少一套备选修改方案，以应对口审中合议组提出的新观点\n")
	b.WriteString("- **合议组背景**：了解合议组的技术背景和审查风格，有针对性地准备\n")
	b.WriteString("- **记录要点**：口审中的重要问题和答复要点应当场记录，以便后续补充书面意见\n")

	return b.String()
}

// prepareStatementOutline generates a timeline-based outline for oral
// hearing陈述 (statement), organized into four phases.
func prepareStatementOutline(grounds []ReexamGround) string {
	var b strings.Builder

	b.WriteString("### 口审陈述时间线\n\n")

	b.WriteString("#### 第一阶段：开场（约3-5分钟）\n\n")
	b.WriteString("1. **请求人自我介绍及身份确认**\n")
	b.WriteString("   - 说明请求人、代理机构、代理师姓名及执业资格\n")
	b.WriteString("2. **确认收到驳回决定**及相关对比文件\n")
	b.WriteString("3. **简要说明复审请求的核心观点**（不超过1分钟）\n")
	b.WriteString("4. 如有修改，简要说明修改类型和范围\n\n")

	b.WriteString("#### 第二阶段：复审理由陈述（约10-15分钟）\n\n")

	for i, g := range grounds {
		action := "论证"
		switch g.Type {
		case ReexamGroundNovelty:
			action = "论证新颖性"
		case ReexamGroundInventiveness:
			action = "论证创造性"
		case ReexamGroundDisclosure:
			action = "论证充分公开"
		case ReexamGroundClarity:
			action = "论证清楚/支持"
		case ReexamGroundAmendment:
			action = "论证修改合规"
		case ReexamGroundSubject:
			action = "论证客体合规"
		}
		fmt.Fprintf(&b, "%d. **%s**——%s\n", i+1, g.Description, action)
		b.WriteString("   - 指出驳回决定中的事实认定错误或法律适用不当\n")
		b.WriteString("   - 提出复审理由的核心论据和对比文件分析\n")
		b.WriteString("   - 引用对比文件中的具体段落或附图（如适用）\n")
		b.WriteString("   - 总结本项理由为什么能够成立\n\n")
	}

	b.WriteString("#### 第三阶段：合议组提问与应答（约15-20分钟）\n\n")
	b.WriteString("1. **认真听取合议组问题**，确认理解后再回答\n")
	b.WriteString("2. 回答时围绕以下核心论点展开：\n")
	b.WriteString("   - 技术方案的区别特征和独特技术效果\n")
	b.WriteString("   - 现有技术整体上未给出技术启示（针对创造性）\n")
	b.WriteString("   - 修改内容的原申请文件出处（针对A33修改合规性）\n")
	b.WriteString("   - 说明书记载的内容足以使本领域技术人员能够实现（针对A26.3）\n")
	b.WriteString("3. **对于不确定的问题**：\n")
	b.WriteString("   - 不仓促回答，可申请记录在案，在指定期限内提交书面补充意见\n")
	b.WriteString("   - 对于合议组提出的新对比文件或新理由，可请求给予答辩期限\n\n")

	b.WriteString("#### 第四阶段：总结陈述（约5分钟）\n\n")
	b.WriteString("1. **重申复审请求的核心立场**：驳回决定的事实认定和/或法律适用存在错误\n")
	b.WriteString("2. **强调技术方案的贡献和进步**：本发明相对于现有技术做出了实质性的改进\n")
	b.WriteString("3. **表达合作意愿**：愿意配合合议组进一步提供补充材料和说明\n")
	b.WriteString("4. **明确请求**：请求合议组撤销驳回决定，或发回原审查部门继续审查\n\n")

	b.WriteString("### 注意事项\n\n")
	b.WriteString("- **态度**：陈述时应保持专业、客观、尊重的态度，避免对抗性语言\n")
	b.WriteString("- **新观点处理**：对于合议组在口审中提出的新观点，切勿当场仓促回答，可要求给予时间准备书面意见\n")
	b.WriteString("- **口审记录**：口审记录将作为复审决定的依据，重要观点务必要求记录在案\n")
	b.WriteString("- **文件准备**：带齐所有原始文件——驳回决定、对比文件、修改替换页、复审请求书\n")
	b.WriteString("- **时间管理**：合理分配各环节时间，优先保证核心论证点的陈述质量\n")

	return b.String()
}
