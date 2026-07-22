package enablement

import "strings"

// =============================================================================
// 技术领域检测与领域自适应规则
// =============================================================================

// TechDomain 标识技术领域类型。
type TechDomain string

const (
	DomainGeneral    TechDomain = "general"    // 通用/未确定
	DomainChemical   TechDomain = "chemical"   // 化学/医药
	DomainBiotech    TechDomain = "biotech"    // 生物技术
	DomainTCM        TechDomain = "tcm"        // 中药
	DomainComputer   TechDomain = "computer"   // 计算机/软件/AI
	DomainMechanical TechDomain = "mechanical" // 机械/结构
	DomainElectronic TechDomain = "electronic" // 电子/电气
)

// domainKeywords 是各技术领域的关键词匹配表。
var domainKeywords = map[TechDomain][]string{
	DomainChemical: {
		"化学", "化合物", "组合物", "催化剂", "反应", "分子式", "结构式",
		"聚合物", "高分子", "合金", "药物", "药品", "医药", "晶体", "晶型",
		"马库什", "原料药", "制剂", "活性成分", "CAS",
	},
	DomainBiotech: {
		"生物", "基因", "DNA", "RNA", "蛋白质", "酶", "抗体", "疫苗",
		"微生物", "菌", "细胞", "培养", "发酵", "保藏", "核苷酸", "氨基酸序列",
		"载体", "质粒", "克隆", "CRISPR",
	},
	DomainTCM: {
		"中药", "中医药", "草药", "方剂", "汤剂", "丸", "散", "膏", "丹",
		"药味", "饮片", "正名", "别名", "配伍", "君臣佐使", "药材",
	},
	DomainComputer: {
		"计算机", "软件", "程序", "算法", "数据", "模型", "AI", "人工智能",
		"机器学习", "深度学习", "神经网络", "流程图", "处理器", "存储器",
		"云端", "服务器", "API", "模块化",
	},
	DomainMechanical: {
		"机械", "装置", "结构", "壳体", "轴承", "齿轮", "弹簧", "铰链",
		"支架", "框架", "密封", "阀门", "活塞", "叶片", "焊接", "螺栓",
	},
	DomainElectronic: {
		"电子", "电路", "芯片", "半导体", "传感器", "电容器", "电阻",
		"晶体管", "CMOS", "封装", "印刷电路", "PCB", "电感", "信号",
		"电源", "电压", "电流",
	},
}

// DetectDomain 根据 EnablementInput 中的关键词推测技术领域。
// 优先级：化学 > 生物 > 中药 > 计算机 > 电子 > 机械 > 通用。
func DetectDomain(input *EnablementInput) TechDomain {
	if input == nil {
		return DomainGeneral
	}

	// 构建待匹配文本
	var sb strings.Builder
	for _, f := range input.Features {
		sb.WriteString(f.Description + " " + f.Category + " " + f.Function + " ")
	}
	for _, p := range input.Problems {
		sb.WriteString(p + " ")
	}
	for _, e := range input.Effects {
		sb.WriteString(e + " ")
	}
	for _, section := range input.DocSections {
		sb.WriteString(section + " ")
	}
	text := sb.String()

	// 按优先级匹配
	priorityOrder := []TechDomain{
		DomainChemical, DomainBiotech, DomainTCM,
		DomainComputer, DomainElectronic, DomainMechanical,
	}

	scores := make(map[TechDomain]int)
	for _, domain := range priorityOrder {
		for _, kw := range domainKeywords[domain] {
			if strings.Contains(text, kw) {
				scores[domain]++
			}
		}
	}

	bestDomain := DomainGeneral
	bestScore := 0
	for _, domain := range priorityOrder {
		if scores[domain] > bestScore {
			bestScore = scores[domain]
			bestDomain = domain
		}
	}

	return bestDomain
}

// DomainStep3Supplement 返回给定技术领域在 step3（能够实现性）的补充检查指令。
// 这些领域规则基于审查指南各技术领域章节和司法实践。
func DomainStep3Supplement(domain TechDomain) string {
	switch domain {
	case DomainChemical:
		return chemicalStep3Rules()
	case DomainBiotech:
		return biotechStep3Rules()
	case DomainTCM:
		return tcmStep3Rules()
	case DomainComputer:
		return computerStep3Rules()
	case DomainMechanical, DomainElectronic:
		return mechanicalElectronicStep3Rules()
	default:
		return ""
	}
}

// DomainStep2Supplement 返回给定技术领域在 step2（清楚性）的补充检查指令。
func DomainStep2Supplement(domain TechDomain) string {
	switch domain {
	case DomainTCM:
		return "## 中药领域额外检查\n" +
			"- 中药材名称是否使用**正名**（药典收录的标准名称）\n" +
			"- 别名/异名是否可能导致歧义（如「藤子暗消」可能指代多种药材）→ 歧义术语\n"
	default:
		return ""
	}
}

// =============================================================================
// 各领域具体规则文本
// =============================================================================

// chemicalStep3Rules 返回化学领域充分公开"三要素"规则。
// 来源：审查指南第二部分第十章 §3 + 阿托伐他汀案（2014行提字第8号）。
func chemicalStep3Rules() string {
	return strings.Join([]string{
		"",
		"## 化学领域特殊规则（审查指南第二部分第十章）",
		"",
		"**化学产品发明充分公开「三要素」**：必须同时满足以下三项，缺一即不符合26.3：",
		"",
		"### 要素1：化学产品的确认",
		"- 化学产品的化学名称、结构式或分子式是否公开",
		"- 与发明相关的化学物理性能参数（如熔点、溶解度、XRD图谱等）是否记载",
		"- 晶体化合物是否通过特征性数据（如XPRD、DSC）确认其晶型",
		"  （参考阿托伐他汀案：未证明含水量和XPRD特征→公开不充分）",
		"",
		"### 要素2：化学产品的制备",
		"- 是否记载了至少一种制备方法（包括原料物质、反应步骤和条件）",
		"- 制备方法中的关键参数（温度、压力、时间、催化剂、溶剂）是否具体",
		"",
		"### 要素3：化学产品的用途和/或使用效果",
		"- 即使是结构首创的化合物，也应当至少记载一种用途",
		"- 对于医药用途发明，必须提供实验数据证明对特定适应症的治疗效果",
		"- 仅断言式描述「具有XX活性」而无数据→公开不充分",
		"  （参考（2015）知行字第352号）",
		"",
		"**马库什权利要求特别注意**：",
		"- 如果说明书中公开的部分实施例不能达到发明目的，且删除后范围缩小→得不到支持",
		"  （伊莱利利案）",
		"",
		"**第二医药用途特别注意**：",
		"- 已知化合物的第二医药用途，须使本领域技术人员确信具有所述治疗效果",
		"- 如果需花费创造性劳动才能确信→公开不充分（辉瑞案）",
		"",
		"**补充实验数据**：",
		"- 申请日后补交的实验数据可用于证明充分公开（时间条件+主体条件）",
		"- 但不能用于引入说明书未记载的新技术效果",
	}, "\n")
}

// biotechStep3Rules 返回生物技术领域规则。
// 来源：审查指南第二部分第十章 §9 + 生物保藏制度。
func biotechStep3Rules() string {
	return strings.Join([]string{
		"",
		"## 生物技术领域特殊规则（审查指南第二部分第十章 §9）",
		"",
		"**生物材料保藏制度**：",
		"- 当生物材料是实现发明**必不可少**的要素，且是**公众不能得到**的，",
		"  必须在规定保藏机构（CGMCC/CCTCC）保藏",
		"- 保藏的两个条件（同时满足才须保藏）：",
		"  ① 生物材料是实现发明必不可少的要素",
		"    （如有替代路径如DNA序列可实现，则不需要保藏）",
		"  ② 生物材料是「公众不能得到」的",
		"    （无法根据说明书制备，无法商业购买）",
		"- 未按规定保藏或未在期限内提交保藏证明和存活证明 → 不符合26.3",
		"",
		"**核苷酸/氨基酸序列**：",
		"- 涉及序列的发明，说明书应包含序列表（符合 WIPO ST.25 标准）",
		"- 序列表缺失可能导致公开不充分",
		"",
		"**实验数据要求**：",
		"- 生物技术领域可预见性低，通常**必须**依赖实验数据证实效果",
		"- 专利审查标准 ≠ 药品上市标准（只要求证明可行性）",
	}, "\n")
}

// tcmStep3Rules 返回中药领域规则。
// 来源：审查指南第二部分第十一章。
func tcmStep3Rules() string {
	return strings.Join([]string{
		"",
		"## 中药领域特殊规则（审查指南第二部分第十一章）",
		"",
		"**中药材名称**：",
		"- 一般应记载中药材**正名**（药典收录的标准名称）",
		"- 别名指代不明确导致无法确认的 → 公开不充分",
		"  （第11647号：「藤子暗消」对应两种药材 → 公开不充分）",
		"",
		"**用量配比**：",
		"- 必须记载各中药原料的**用量配比关系**",
		"  （重量份、重量比例、重量百分比等）",
		"- 缺少配比信息 → 公开不充分",
		"",
		"**医药用途**：",
		"- 新的中药组合物应记载具体医药用途",
		"",
		"**可预测性判断**：",
		"- 如果各药味的已知功效可预测组合后疗效，即使无实验数据也充分公开",
		"  （例：葛根+砂仁+甘草解酒毒，根据各药味已知功效可预测 → 充分公开）",
	}, "\n")
}

// computerStep3Rules 返回计算机/软件/AI 领域规则。
// 来源：审查指南第二部分第九章（含2023修订新增AI条款）。
func computerStep3Rules() string {
	return strings.Join([]string{
		"",
		"## 计算机/软件/AI 领域特殊规则（审查指南第二部分第九章）",
		"",
		"**说明书特殊要求**：",
		"- 必须包含计算机程序的主要**流程图**",
		"- 缺少流程图可能被认为公开不充分",
		"- 应按时间顺序描述各步骤，以本领域技术人员能编制出达到所述效果的程序为准",
		"- 不需要提交全部源程序，但应给出关键步骤/算法的详细描述",
		"",
		"**2023年修订新增（AI模型）**：",
		"- 算法特征应与具体技术领域数据关联",
		"- 至少一个输入参数及输出结果的定义应与技术领域中具体数据对应",
		"- 不得使用纯抽象通用数据（如「特征值」），应使用具体技术术语",
		"  （如「图像像素灰度值」）",
		"",
		"**功能性描述边界**：",
		"- 如果算法/模块的实现方式是本领域公知的（如排序算法、特征提取），",
		"  则功能性描述不构成公开不充分",
		"- 但如果是创新的算法核心，必须给出详细步骤和参数",
	}, "\n")
}

// mechanicalElectronicStep3Rules 返回机械/电学领域规则。
func mechanicalElectronicStep3Rules() string {
	return strings.Join([]string{
		"",
		"## 机械/电学领域特殊规则",
		"",
		"**可预见性较高**：",
		"- 机械领域结构描述+附图通常足以实施，一个完整实施例即可",
		"- 电子领域类似，结构描述+附图标记清晰即可",
		"- 通常不需要实验数据",
		"",
		"**附图可实施性**：",
		"- 如有附图，各部件位置关系和装配方式应清晰可辨",
		"- 附图标记应与文字描述一致",
		"- 结构功能性描述（如「支撑结构」「连接件」）如果属本领域常规手段，",
		"  则不构成公开不充分",
	}, "\n")
}

// DomainLabel 返回技术领域的中文标签。
func DomainLabel(domain TechDomain) string {
	switch domain {
	case DomainChemical:
		return "化学/医药"
	case DomainBiotech:
		return "生物技术"
	case DomainTCM:
		return "中药"
	case DomainComputer:
		return "计算机/软件/AI"
	case DomainMechanical:
		return "机械/结构"
	case DomainElectronic:
		return "电子/电气"
	default:
		return "通用"
	}
}
