package claimdrafting

import (
	"strconv"
	"strings"
)

// =============================================================================

// TechDomain 枚举技术领域，用于选择领域特定的撰写规则和模板。
type TechDomain string

const (
	DomainMechanical TechDomain = "mechanical" // 机械领域：零部件+配置关系+联系形式
	DomainElectrical TechDomain = "electrical" // 电路领域：元器件+导线连接+电回路+功能
	DomainChemical   TechDomain = "chemical"   // 化学领域：组分及含量
	DomainSoftware   TechDomain = "software"   // 计算机程序：方法或功能模块架构
	DomainGeneral    TechDomain = "general"    // 通用（未识别领域）
)

// =============================================================================
// 权利要求类型
// =============================================================================

// ClaimType 区分专利权利要求的法律范畴。
type ClaimType string

const (
	ClaimTypeProduct ClaimType = "product" // 产品权利要求（装置/设备/系统/组合物）
	ClaimTypeMethod  ClaimType = "method"  // 方法权利要求（制造/使用/处理方法）
)

// ClaimStrategy 定义权利要求书的撰写策略，控制独立权利要求的数量与类型布局。
type ClaimStrategy string

const (
	StrategyProductOnly             ClaimStrategy = "product_only"
	StrategyProductAndMethod        ClaimStrategy = "product_and_method"
	StrategyProductAndManufacturing ClaimStrategy = "product_and_manufacturing"
	StrategyProductAndUse           ClaimStrategy = "product_and_use"
	StrategyPharmaUse               ClaimStrategy = "pharma_use" // 瑞士型权利要求（物质X在制备Y病药物中的应用）
	StrategyMarkush                 ClaimStrategy = "markush"    // 马库什权利要求（通式化合物）
)

// =============================================================================
// 权利要求结构
// =============================================================================

// Claim 表示一项权利要求（独立或从属）。
type Claim struct {
	Number    int       `json:"number"`     // 权利要求编号（阿拉伯数字顺序）
	ClaimType ClaimType `json:"claim_type"` // 类型：product / method
	Kind      string    `json:"kind"`       // "independent" / "dependent"

	// 独立权利要求专用字段
	Preamble      string `json:"preamble,omitempty"`      // 前序部分：主题名称 + 与现有技术共有的必要技术特征
	Characterized string `json:"characterized,omitempty"` // 特征部分："其特征在于"之后的区别技术特征

	// 从属权利要求专用字段
	DependsOn  []int  `json:"depends_on,omitempty"` // 引用的权利要求编号（支持多项引用）
	Limitation string `json:"limitation,omitempty"` // 限定部分
}

// IsMultipleDependent 是否多项从属（引用两项以上在先权利要求）。
func (c Claim) IsMultipleDependent() bool {
	return c.Kind == "dependent" && len(c.DependsOn) > 1
}

// ClaimSet 是一套完整的权利要求集合。
type ClaimSet struct {
	IndependentClaims []Claim `json:"independent_claims"`
	DependentClaims   []Claim `json:"dependent_claims"`
}

// Claims 返回所有权利要求（独立+从属，按编号排序）。
func (cs *ClaimSet) Claims() []Claim {
	claimMap := make(map[int]Claim)
	maxNum := 0
	for _, c := range cs.IndependentClaims {
		claimMap[c.Number] = c
		if c.Number > maxNum {
			maxNum = c.Number
		}
	}
	for _, c := range cs.DependentClaims {
		claimMap[c.Number] = c
		if c.Number > maxNum {
			maxNum = c.Number
		}
	}
	all := make([]Claim, 0, len(claimMap))
	for i := 1; i <= maxNum; i++ {
		if c, ok := claimMap[i]; ok {
			all = append(all, c)
		}
	}
	return all
}

// =============================================================================
// 输入/输出
// =============================================================================

// DraftInput 是权利要求撰写的输入数据。
// 与 disclosure 包的 ExtractionResult 语义等价，保持独立定义以避免循环依赖。
type DraftInput struct {
	Title       string        `json:"title"`                 // 发明名称
	TechDomain  TechDomain    `json:"tech_domain,omitempty"` // 技术领域（空则自动识别）
	Strategy    ClaimStrategy `json:"strategy,omitempty"`    // 撰写策略（空则 product_only）
	Problems    []string      `json:"problems"`              // 要解决的技术问题
	Features    []Feature     `json:"features"`              // 技术特征列表
	Effects     []string      `json:"effects"`               // 技术效果
	PFETriples  []PFETriple   `json:"pfe_triples"`           // 问题-特征-效果关联
	PriorArt    string        `json:"prior_art,omitempty"`   // 最接近的现有技术描述
	Description string        `json:"description,omitempty"` // 说明书摘要（用于支持性验证）
}

// Feature 是最小技术单元——不可再分的原子技术手段。
type Feature struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`               // structure / method / parameter / material
	Function    string `json:"function"`               // 技术功能描述
	Importance  string `json:"importance"`             // high / medium / low
	PriorStatus string `json:"prior_status,omitempty"` // known / unknown / partial
}

// PFETriple 是问题-特征-效果的因果关系链。
type PFETriple struct {
	ID         string   `json:"id"`
	Problem    string   `json:"problem"`
	FeatureIDs []string `json:"feature_ids"`
	Effect     string   `json:"effect"`
}

// DraftOutput 是权利要求撰写的输出结果。
type DraftOutput struct {
	Claims    *ClaimSet `json:"claims"`             // 生成的权利要求
	Warnings  []string  `json:"warnings,omitempty"` // 警告信息（非阻断性问题）
	InputMeta struct {
		Domain       TechDomain `json:"domain"`
		ClaimType    ClaimType  `json:"claim_type"`
		FeatureCount int        `json:"feature_count"`
	} `json:"input_meta"`
	Score     float64 `json:"score,omitempty"` // 综合质量评分（0-100）
	Timestamp string  `json:"timestamp"`       // 生成时间
}

// =============================================================================
// 验证相关
// =============================================================================

// Severity 表示违规的严重程度。
type Severity string

const (
	SeverityError   Severity = "error"   // 严重违法（如多项从属互引）
	SeverityWarning Severity = "warning" // 潜在风险（如使用不确定用语）
	SeverityInfo    Severity = "info"    // 建议改进（如保护范围可优化）
)

// Violation 记录一条规则违规信息。
type Violation struct {
	RuleName    string   `json:"rule_name"`
	RuleBasis   string   `json:"rule_basis,omitempty"` // 法律依据
	Severity    Severity `json:"severity"`
	ClaimNumber int      `json:"claim_number"` // 关联的权利要求编号（0 表示整体问题）
	Message     string   `json:"message"`      // 违规描述
	Suggestion  string   `json:"suggestion"`   // 修改建议
}

// =============================================================================
// 质量评分
// =============================================================================

// ScoreReport 是质量评估的完整报告。
type ScoreReport struct {
	OverallScore    float64            `json:"overall_score"`    // 0-100
	DimensionScores map[string]float64 `json:"dimension_scores"` // 各维度得分
	Violations      []Violation        `json:"violations"`
	Suggestions     []string           `json:"suggestions"`
	Grade           string             `json:"grade"` // A/B/C/D
}

// 评分维度常量
const (
	DimClarity   = "clarity"   // 清楚性（类型清楚+用词清楚）
	DimSupport   = "support"   // 支持性（以说明书为依据）
	DimNecessity = "necessity" // 必要性（必要技术特征完整）
	DimFormality = "formality" // 形式规范（编号/格式/引用）
	DimScope     = "scope"     // 保护范围合理性（多层次布局+上位概括）
)

// 维度权重
var DimensionWeights = map[string]float64{
	DimClarity:   0.25,
	DimSupport:   0.25,
	DimNecessity: 0.20,
	DimFormality: 0.15,
	DimScope:     0.15,
}

// =============================================================================
// 帮助函数
// =============================================================================

// String renders the claim in standard Chinese patent format.
func (c Claim) String() string {
	var b strings.Builder
	if c.Kind == "independent" {
		b.WriteString(c.Preamble)
		b.WriteString("，其特征在于，")
		b.WriteString(c.Characterized)
	} else {
		b.WriteString("根据权利要求")
		switch len(c.DependsOn) {
		case 1:
			b.WriteString(strconv.Itoa(c.DependsOn[0]))
		default:
			for i, dep := range c.DependsOn {
				if i > 0 {
					b.WriteString("或")
				}
				b.WriteString(strconv.Itoa(dep))
			}
		}
		b.WriteString("所述的")
		b.WriteString(c.Limitation)
	}
	b.WriteString("。")
	return b.String()
}
