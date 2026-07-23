package inventiveness

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

// Framework 提供专利法第22条第3款（创造性三步法）的判断框架查询。
// provider 为 nil 时降级为内置默认框架文本。
type Framework struct {
	provider ArticleFrameworkProvider
}

// NewFramework 创建绑定到 ArticleFrameworkProvider 的 Framework 查询器。
func NewFramework(provider ArticleFrameworkProvider) *Framework {
	return &Framework{provider: provider}
}

// GetArticleFramework 返回 A22.3 创造性三步法的法条判断框架。
func (f *Framework) GetArticleFramework() string {
	if f.provider != nil {
		if af := f.provider.Article("patent-law-a22.3"); af.Name != "" {
			return formatArticleData(af)
		}
		if af := f.provider.Article("A22.3"); af.Name != "" {
			return formatArticleData(af)
		}
	}
	return defaultA223Framework()
}

// defaultA223Framework 返回内嵌的默认 A22.3 创造性判断框架。
// 当 rules.Engine 未加载 YAML 时作为降级方案。
func defaultA223Framework() string {
	return strings.Join([]string{
		"## 专利法第22条第3款——创造性判断框架",
		"",
		"**法条原文**：《中华人民共和国专利法》（2020 年修正）第 22 条第 3 款",
		"「创造性，是指与现有技术相比，该发明具有突出的实质性特点和显著的进步，",
		"  该实用新型具有实质性特点和进步。」",
		"",
		"**审查指南依据**：审查指南（2023 修订）第二部分第四章（含第84号局令修订）",
		"",
		"**判断主体：「本领域的技术人员」**",
		"- 知晓申请日/优先权日之前所属技术领域所有的普通技术知识",
		"- 能够获知该领域中所有的现有技术",
		"- 具有应用该日期之前常规实验手段的能力",
		"- 不具有创造能力",
		"- 如技术问题促使其跨领域寻找技术手段，也具有从其他技术领域获知的能力",
		"",
		"### 三步法判断 + 显著的进步",
		"",
		"**第 1 步：确定最接近的现有技术**",
		"- 技术领域相同或相近的现有技术",
		"- 公开技术特征最多的现有技术",
		"- 要解决的技术问题最接近的现有技术",
		"- 技术效果最接近的现有技术",
		"- 优先选择相同技术领域，但也可选取不同领域的现有技术",
		"",
		"**第 2 步：确定区别特征和实际解决的技术问题**",
		"- 区别特征必须是权利要求中明确记载的技术特征",
		"- 逐项列出目标方案相对于最接近现有技术的全部区别技术特征",
		"- 基于区别特征的技术效果，重新确定发明实际解决的技术问题",
		"- 注意：实际解决的技术问题可以不同于申请文件声称的问题",
		"",
		"**无贡献特征识别**（2023年审查指南第84号局令新增）：",
		"- 对技术问题解决没有作出贡献的特征，不影响创造性判断",
		"- 四维度判断：与技术问题的关联、技术效果、常规性、可获知性",
		"- 常见无贡献特征：主题常规组成部分、公知常识、常规参数选择",
		"",
		"**第 3 步：判断是否显而易见（技术启示）**",
		"",
		"技术启示的五种情形：",
		"1. 区别特征属于本领域公知常识（惯用手段、教科书/技术词典记载）",
		"2. 区别特征在同一对比文件其他部分已披露且作用相同",
		"3. 区别特征在另一份对比文件中已披露且作用相同",
		"4. 其他对比文件披露了功能类似但形式不同的手段，可通过公知变化获得",
		"5. 出于解决领域公认问题或满足普遍需求（更便宜/更快/更耐久）的动机",
		"",
		"特殊规则：",
		"- 对比文件给出反向教导 → 不存在技术启示",
		"- 对比文件间存在结合障碍（功能冲突、原理矛盾）→ 不存在技术启示",
		"- 跨领域结合需有更充分理由",
		"- 区别特征在对比文件中的作用与本发明中不同 → 不存在启示",
		"- 禁止「事后诸葛亮」式分析",
		"",
		"改进动机三维度：",
		"- 发现技术问题的难易程度",
		"- 不同现有技术结合的动机",
		"- 最接近现有技术教导的改进方向",
		"",
		"**第 4 步：判断显著的进步（有益技术效果）**",
		"- 创造性 = 突出的实质性特点（Step 3） AND 显著的进步（Step 4）",
		"",
		"显著的进步四种类型：",
		"1. 效果改善型：与现有技术相比具有更好的技术效果",
		"2. 异途同归型：提供技术构思不同的技术方案，效果基本达到现有技术水平",
		"3. 趋势引领型：代表某种新技术发展趋势",
		"4. 利弊权衡型：某些方面有负面效果，但其他方面具有明显积极的技术效果",
		"",
		"### 辅助判断因素",
		"- 预料不到的技术效果（**充分条件而非必要条件**——具有则肯定创造性，",
		"  但不能以「不具有」为由得出不具备创造性的结论）",
		"- 克服了技术偏见",
		"- 长期未满足的技术需求",
		"- 商业上的成功（需与技术特征关联）",
		"",
		"### 结论模式",
		"- isInventive: bool — 是否具备创造性（NonObvious AND HasSignificantProgress）",
		"- hasSignificantProgress: bool — 是否具有显著的进步",
		"- reasoning: string — 推理过程",
		"- confidence: high/medium/low",
		"- auxiliary_factors: []string — 辅助考虑因素",
		"",
		"### 综合来源备注（权威法理参考）",
		"- 【三步法与后见之明】三步法要求事后回溯确定「实际解决的技术问题」，",
		"  不可避免地受后见之明影响。所有权威来源均强调应严格防范「事后诸葛亮」式分析。",
		"- 【两步要件的关系】「突出的实质性特点」和「显著的进步」是综合判断的，",
		"  而非各自独立达标的——崔国斌《专利法》强调此消彼长的弹性空间。",
		"- 【预料不到效果的性质】预料不到的技术效果是创造性的充分条件而非必要条件——",
		"  不能以「无预料不到的技术效果」为由得出不具备创造性的结论",
		"  （尹新天《专利法详解》第300页）。",
		"",
		"**注意**：本判断由 AI 辅助生成，不构成正式法律意见。",
	}, "\n")
}

// =============================================================================
// 六种发明类型判断框架
// =============================================================================

// InventionTypeFramework 根据发明类型返回对应的判断框架文本。
// 返回空字符串表示通用类型（使用默认三步法框架）。
func InventionTypeFramework(inventionType string) string {
	switch inventionType {
	case InventionTypePioneering:
		return pioneeringFramework()
	case InventionTypeCombination:
		return combinationFramework()
	case InventionTypeSelection:
		return selectionFramework()
	case InventionTypeTransfer:
		return transferFramework()
	case InventionTypeNewUse:
		return newUseFramework()
	case InventionTypeElementChange:
		return elementChangeFramework()
	default:
		return ""
	}
}

func pioneeringFramework() string {
	return strings.Join([]string{
		"## 开拓性发明 — 创造性判断框架",
		"",
		"**定义**：全新的技术方案，在技术史上未曾有过先例，为人类科学技术开创了新纪元。",
		"",
		"**判断规则**：",
		"- 开拓性发明原则上均具备创造性",
		"- 无需严格适用三步法",
		"- 重点确认：发明是否确实属于开拓性（而非仅相对于检索到的现有技术是新的）",
		"",
		"**典型示例**：指南针、造纸术、蒸汽机、白炽灯、雷达、激光器",
	}, "\n")
}

func combinationFramework() string {
	return strings.Join([]string{
		"## 组合发明 — 创造性判断框架",
		"",
		"**定义**：将已知的技术特征或技术方案进行组合而形成的发明。",
		"",
		"**判断规则**：",
		"- 关键因素：组合后的技术效果是否产生了协同作用（1+1>2）",
		"- 简单叠加：各自以常规方式工作、总效果为各部分之和、功能上无相互作用 → **不具备创造性**",
		"- 有机组合：功能上彼此支持、取得新的技术效果、效果优于各部分之和 → **可能具备创造性**",
		"",
		"**判断要素**：",
		"- 功能上是否彼此相互支持",
		"- 组合的难易程度",
		"- 现有技术中是否存在组合的启示",
		"- 组合后的技术效果",
	}, "\n")
}

func selectionFramework() string {
	return strings.Join([]string{
		"## 选择发明 — 创造性判断框架",
		"",
		"**定义**：从现有技术公开的宽范围中，有目的地选出现有技术中未提到的窄范围或个体。",
		"",
		"**判断规则**：",
		"- 关键因素：选择是否带来了预料不到的技术效果",
		"- 从已知可能性中的常规选择（如常规尺寸/温度范围）→ **不具备创造性**",
		"- 可从现有技术直接推导的选择 → **不具备创造性**",
		"- 选择产生了预料不到的技术效果 → **具备创造性**",
		"",
		"**注意**：选择发明的创造性高度依赖于实验数据支撑。",
	}, "\n")
}

func transferFramework() string {
	return strings.Join([]string{
		"## 转用发明 — 创造性判断框架",
		"",
		"**定义**：将已知技术从原技术领域转用到新的技术领域而形成的发明。",
		"",
		"**判断规则**：",
		"- 关键因素：领域远近、是否克服了原领域未曾遇到的困难",
		"- 类似或相近技术领域之间的转用，未产生预料不到的技术效果 → **不具备创造性**",
		"- 跨领域转用且产生预料不到的技术效果 → **具备创造性**",
		"- 转用克服了原技术领域中未曾遇到的困难 → **具备创造性**",
	}, "\n")
}

func newUseFramework() string {
	return strings.Join([]string{
		"## 已知产品新用途发明 — 创造性判断框架",
		"",
		"**定义**：将已知产品用于新的目的（即新的用途）的发明。",
		"",
		"**判断规则**：",
		"- 关键因素：新用途是否利用了已知产品新发现的性质",
		"- 新用途仅使用已知材料的已知性质 → **不具备创造性**",
		"- 新用途利用了已知产品新发现的性质并产生预料不到的技术效果 → **具备创造性**",
	}, "\n")
}

func elementChangeFramework() string {
	return strings.Join([]string{
		"## 要素变更发明 — 创造性判断框架",
		"",
		"**定义**：通过改变已知技术方案中的要素关系、要素替代或要素省略而形成的发明。",
		"",
		"**三种子类型及规则**：",
		"",
		"1. **要素关系改变**（如大小、比例、位置、形状的改变）",
		"   - 改变未导致效果/功能/用途变化，或变化可预料 → 不具备创造性",
		"   - 改变导致预料不到的技术效果 → 具备创造性",
		"",
		"2. **要素替代**（用另一种已知要素替换）",
		"   - 相同功能的已知手段的等效替代 → 不具备创造性",
		"   - 替代产生预料不到的技术效果 → 具备创造性",
		"",
		"3. **要素省略**（省去一项或多项要素）",
		"   - 省略后功能也相应消失 → 不具备创造性",
		"   - 省略后保持全部原有功能或带来预料不到的技术效果 → 具备创造性",
	}, "\n")
}

// =============================================================================
// 实务统计数据（GAP-11, GAP-20）
// =============================================================================

// EmpiricalStatistics 返回基于复审/无效决定的实证统计数据。
// 用于辅助 LLM 校准创造性判断的置信度。
func EmpiricalStatistics() string {
	return strings.Join([]string{
		"## 创造性判断实务统计参考",
		"",
		"基于39,496份复审/无效决定的元数据分析（样本中12,798份涉及创造性）：",
		"",
		"| 推理模式 | 出现频率 | 全部无效成功率 |",
		"|---------|---------|--------------|",
		"| 单对比文件+公知常识 | 54.0% | 95.9% |",
		"| 多对比文件结合 | 14.3% | 73.7% |",
		"| 技术启示判断 | 8.1% | 83.8% |",
		"| 用途限定的影响 | 4.3% | 75.2% |",
		"| 预料不到的效果抗辩 | 3.1% | 84.0%（维持有效仅5.7%）|",
		"| 惯用手段与常规选择 | 0.1% | 68.8% |",
		"",
		"**置信度校准指引**：",
		"- 单对比文件+公知常识认定无创造性 → 置信度 high（实证成功率95.9%）",
		"- 多对比文件结合认定无创造性 → 置信度 medium（实证成功率73.7%，需充分论证结合动机）",
		"- 预料不到效果认定有创造性 → 置信度 low-medium（维持有效率仅5.7%，需严格审查实验数据）",
		"",
		"**注意**：统计数据反映无效程序的整体倾向（偏向无效请求人胜诉），仅供参考，不替代个案分析。",
	}, "\n")
}

// =============================================================================
// 特殊领域判断框架
// =============================================================================

// TechDomainFramework 根据技术领域返回对应的判断框架文本。
func TechDomainFramework(domain string) string {
	switch domain {
	case "chemistry":
		return chemistryDomainFramework()
	case "computer":
		return computerDomainFramework()
	case "tcm":
		return tcmDomainFramework()
	default:
		return ""
	}
}

func chemistryDomainFramework() string {
	return strings.Join([]string{
		"## 化学领域 — 创造性判断框架",
		"",
		"### 化合物发明",
		"- 判断结构差异 → 基于用途和/或效果确定技术问题 → 判断技术启示",
		"- 结构接近的化合物：改造不能带来预料不到的技术效果 → 显而易见",
		"- 经典电子等排体置换（NH2/CH3、-O-/-NH-、-S-/-O-等）：仅置换且效果相当 → 无创造性",
		"- 电子等排体置换但产生预料不到效果 → 仍有创造性",
		"- 已知必然趋势排除：效果是已知必然趋势导致的 → 无创造性",
		"",
		"### 晶体化合物",
		"- 与已知化学结构相同或接近 → 须产生预料不到的效果才可能具备创造性",
		"",
		"### 基因/单克隆抗体",
		"- 已知结构基因的天然突变基因且性质功能相同 → 无创造性",
		"- 已知抗原的单克隆抗体 → 通常无创造性（除非有其他限定产生预料不到效果）",
		"",
		"### 组合物发明",
		"- 药物联用需分析：现有技术是否有联用指引、是否存在技术障碍或偏见",
		"- 联用后是否取得预料不到效果",
		"",
		"### 制备方法发明",
		"- 围绕技术问题梳理多个区别特征，确定评判重点",
		"",
		"### 制药用途发明",
		"- 核心：新适应症与已知适应症的关系 + 是否产生预料不到效果",
	}, "\n")
}

func computerDomainFramework() string {
	return strings.Join([]string{
		"## 计算机/AI领域 — 创造性判断框架",
		"",
		"**整体考量原则**：包含算法特征或商业规则和方法特征的发明，",
		"应将技术特征与功能上彼此相互支持、存在相互作用关系的算法/商业规则特征作为整体考虑。",
		"",
		"**「功能上彼此相互支持」的三种情形**：",
		"1. 算法应用于具体技术领域解决技术问题",
		"2. 算法与计算机系统内部结构存在特定关联、提升硬件效率",
		"3. 商业规则实施需要技术手段调整或改进",
		"",
		"**创造性判断参考**：",
		"- 算法应用于具体技术领域解决技术问题 → 可能具备创造性",
		"- 用户体验提升由技术特征或技术特征与算法的共同作用带来 → 应予以考虑",
		"- 已知算法的简单替换或常规计算机实现 → 不具备创造性",
		"- 仅规定商业规则而技术手段与现有技术相同 → 不具备创造性",
	}, "\n")
}

func tcmDomainFramework() string {
	return strings.Join([]string{
		"## 中药领域 — 创造性判断框架",
		"",
		"**判断核心**：以「君臣佐使」组方结构为核心，从「理、法、方、药」四层面分析区别特征。",
		"",
		"**加减方发明**（药味增减/药味替换/药量加减）：",
		"- 药味增减：现有技术无增减启示 + 产生有益效果 → 具备创造性",
		"  公知药对配伍 → 无创造性",
		"- 药味替换：属于已知功效相同替代 + 无预料不到效果 → 无创造性",
		"- 药量加减：不改变组方结构 + 常规加减 + 无预料不到效果 → 无创造性",
		"",
		"**合方发明**：",
		"- 现有技术无合方启示 + 有益效果 → 具备创造性",
		"- 简单叠加且效果仅为加和 → 无创造性",
		"",
		"**自组方发明**：",
		"- 无已知方基础，需记载组方原则、组方结构和实验数据",
		"- 无法从现有技术得到配伍启示 + 产生有益效果 → 具备创造性",
		"",
		"**注意**：药味按主次地位分层（主要药味 vs 次要药味），主要药味的变化对创造性影响更大。",
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
