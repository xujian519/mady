package novelty

import (
	"fmt"
	"strings"
)

// =============================================================================
// ArticleFramework 查询
// =============================================================================

// ArticleFrameworkProvider 是法条框架查询的抽象接口。
// 生产环境由 domains/rules.Engine 实现，测试/降级场景由 nil 实现。
// 使用接口而非直接引用 domains/rules 包，避免引入 transitive build 依赖。
type ArticleFrameworkProvider interface {
	Article(id string) ArticleFrameworkData
}

// ArticleFrameworkData 是法条框架的纯数据镜像（避免依赖 domains/rules 包）。
type ArticleFrameworkData struct {
	Name             string
	LawRef           string
	GuidelineRef     string
	Steps            []ArticleStepData
	ConclusionSchema map[string]string
	ApplicableTo     []string
}

// ArticleStepData 是单步判断步骤的数据镜像。
type ArticleStepData struct {
	Order        int
	Name         string
	InputHint    string
	OutputSchema map[string]string
}

// Framework 提供专利法第22条第2款（新颖性）的判断框架查询。
// provider 为 nil 时降级为内置默认框架文本。
type Framework struct {
	provider ArticleFrameworkProvider
}

// NewFramework 创建绑定到 ArticleFrameworkProvider 的 Framework 查询器。
func NewFramework(provider ArticleFrameworkProvider) *Framework {
	return &Framework{provider: provider}
}

// GetArticleFramework 返回 A22.2 新颖性的法条判断框架。
func (f *Framework) GetArticleFramework() string {
	if f.provider != nil {
		if af := f.provider.Article("patent-law-a22.2"); af.Name != "" {
			return formatArticleData(af)
		}
		if af := f.provider.Article("A22.2"); af.Name != "" {
			return formatArticleData(af)
		}
	}
	return defaultA222Framework()
}

// defaultA222Framework 返回内嵌的默认 A22.2 新颖性判断框架。
// 当 rules.Engine 未加载 YAML 时作为降级方案。
func defaultA222Framework() string {
	return strings.Join([]string{
		"## 专利法第22条第2款——新颖性判断框架",
		"",
		"**法条原文**：",
		"《中华人民共和国专利法》（2020 年修正）第 22 条第 2 款",
		"「新颖性，是指该发明或者实用新型不属于现有技术；也没有任何单位或者个人",
		"  就同样的发明或者实用新型在申请日以前向国务院专利行政部门提出过申请，",
		"  并记载在申请日以后公布的专利申请文件或者公告的专利文件中。」",
		"",
		"《专利法》第 22 条第 5 款（现有技术定义）：",
		"「本法所称现有技术，是指申请日以前在国内外为公众所知的技术。」",
		"",
		"《专利法》第 24 条（不丧失新颖性的宽限期）：",
		"「申请专利的发明创造在申请日以前六个月内，有下列情形之一的，不丧失新颖性：",
		"  （一）在中国政府主办或者承认的国际展览会上首次展出的；",
		"  （二）在规定的学术会议或者技术会议上首次发表的；",
		"  （三）他人未经申请人同意而泄露其内容的。」",
		"",
		"《专利法》第 29 条（优先权）：",
		"「申请人自发明或者实用新型在外国第一次提出专利申请之日起十二个月内，",
		"  又在中国就相同主题提出专利申请的，依照该外国同中国签订的协议或者",
		"  共同参加的国际条约，或者依照相互承认优先权的原则，可以享有优先权。」",
		"",
		"**审查指南依据**：审查指南（2023 修订）第二部分第三章",
		"",
		"**判断主体：「本领域的技术人员」**",
		"- 知晓申请日/优先权日之前所属技术领域所有的普通技术知识",
		"- 能够获知该领域中所有的现有技术",
		"- 具有应用该日期之前常规实验手段的能力",
		"- 不具有创造能力",
		"- 如技术问题促使其跨领域寻找技术手段，也具有从其他技术领域获知的能力",
		"",
		"### 新颖性判断流程",
		"",
		"**第 1 步：现有技术审查（A22.5）**",
		"- 确定有效申请日（或优先权日）",
		"- 判断在先文献是否在申请日前已为公众所知",
		"  - 严格标准：不负保密义务的人所能得知的状态",
		"  - 宽松标准（审查指南）：公众想得知就能够得知的状态",
		"- 书面公开：出版物范围极广，关键在流通渠道开放程度",
		"- 互联网公开：对公众开放的平台 + 公开时间认定",
		"- 公开使用：产品特征是否可从外部观察得知",
		"- 销售公开：广告能确认产品同一性即可",
		"- 充分公开（可实施性）要求：在先文献须使熟练技术人员能实施",
		"",
		"**第 2 步：单独对比原则**",
		"- 只能将一项权利要求与一份现有技术单独对比，不得组合多份",
		"- 全部特征对比：现有技术须公开权利要求的全部技术特征",
		"- 对比文件公开内容包括明确记载和隐含公开",
		"- 实施现有技术如对权利要求构成字面侵权→破坏新颖性",
		"",
		"**第 3 步：相同或实质相同判断**",
		"",
		"**3.1 四要素综合判断**：",
		"- 技术领域相同",
		"- 解决的技术问题相同",
		"- 技术方案实质上相同",
		"- 预期效果相同",
		"",
		"**3.2 上下位概念**：",
		"- 对比文件公开下位概念（铜）→ 破坏上位概念（金属）的新颖性",
		"- 对比文件公开上位概念（金属）→ 不破坏下位概念（铜）的新颖性",
		"",
		"**3.3 惯用手段的直接置换**：",
		"- 螺钉↔螺栓、皮带传动↔链条传动→视为直接置换，破坏新颖性",
		"- 需以本领域技术人员的知识水平为判断标准",
		"",
		"**3.4 数值范围（8 种情形）**：",
		"- 情形1：对比文件数值落入权利要求范围 → 破坏新颖性",
		"- 情形2：数值范围部分重叠或有共同端点 → 破坏新颖性",
		"- 情形3：权利要求的离散数值等于对比文件范围端点 → 破坏新颖性",
		"- 情形4：权利要求的离散数值在对比文件范围内但非端点 → 不破坏",
		"- 情形5：权利要求范围完全在对比文件范围内且无共同端点 → 不破坏",
		"- 情形6：对比文件范围的两个端点破坏离散两端点的新颖性",
		"- 情形7：对比文件范围端点不破坏中间离散值的新颖性",
		"- 情形8：权利要求数值范围窄于对比文件且无共同端点 → 不破坏",
		"",
		"**3.5 性能/参数/用途/制备方法特征**：",
		"- 性能/参数特征是否隐含了特定结构/组成",
		"- 用途特征是否由产品固有特性决定",
		"- 制备方法特征是否导致产品结构/组成不同",
		"",
		"**第 4 步：抵触申请审查**",
		"- 三要件：时间条件 + 公开条件 + 内容条件",
		"- 全文内容制：以在先申请的全文（说明书+权利要求书+附图）为比对基础",
		"- 效力的不可逆性：在先申请后续撤回/驳回/无效→抵触效力不变",
		"- 抵触申请仅用于新颖性判断（相同性），不用于创造性判断",
		"- 外观设计不能作为发明/实用新型的抵触申请",
		"- 同日申请不构成抵触申请（适用 A9 禁止重复授权原则）",
		"",
		"**第 5 步：宽限期例外（A24）**",
		"- 三种法定情形：国际展览会展出 / 学术会议发表 / 他人泄露",
		"- 宽限期 = 6 个月（自公开日起算）",
		"- 第三方独立公开 → 仍影响新颖性",
		"- 宽限期内以其他方式公开 → 仍影响新颖性",
		"- 程序要求：须在申请时声明 + 2 个月内提交证明文件",
		"",
		"**第 6 步：优先权例外（A29）**",
		"- 国际优先权：12 个月（巴黎公约）",
		"- 本国优先权：12 个月，在先申请即被视为撤回",
		"- 「相同主题」四要素：领域 / 问题 / 方案 / 效果",
		"- 优先权日后的公开不损害在后申请的新颖性",
		"- 增加在先申请未记载的特征 → 不能享受优先权",
		"",
		"### 结论模式",
		"- hasNovelty: bool — 是否具备新颖性",
		"- reasoning: string — 推理过程",
		"- confidence: high/medium/low — 置信度",
		"- failedClaims: []string — 不具备新颖性的权利要求编号",
		"",
		"**注意**：本判断由 AI 辅助生成，不构成正式法律意见。",
	}, "\n")
}

// formatArticleData 将 ArticleFrameworkData 格式化为 Markdown 文本。
func formatArticleData(af ArticleFrameworkData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", af.Name)
	fmt.Fprintf(&b, "**法条依据**：%s\n\n", af.LawRef)
	if af.GuidelineRef != "" {
		fmt.Fprintf(&b, "**审查指南依据**：%s\n\n", af.GuidelineRef)
	}

	b.WriteString("### 判断步骤\n\n")
	for _, step := range af.Steps {
		fmt.Fprintf(&b, "**第 %d 步：%s**\n", step.Order, step.Name)
		if step.InputHint != "" {
			fmt.Fprintf(&b, "- 输入：%s\n", step.InputHint)
		}
		for key, desc := range step.OutputSchema {
			fmt.Fprintf(&b, "- %s：%s\n", key, desc)
		}
		b.WriteString("\n")
	}

	b.WriteString("### 结论模式\n\n")
	for key, desc := range af.ConclusionSchema {
		fmt.Fprintf(&b, "- %s：%s\n", key, desc)
	}

	if len(af.ApplicableTo) > 0 {
		fmt.Fprintf(&b, "\n**适用场景**：%s\n", strings.Join(af.ApplicableTo, "、"))
	}

	return b.String()
}
