package specdrafting

import (
	"time"

	"github.com/xujian519/mady/disclosure"
)

// =============================================================================
// 枚举类型
// =============================================================================

// PatentType 区分发明专利与实用新型。
type PatentType string

const (
	PatentTypeInvention    PatentType = "invention"     // 发明专利
	PatentTypeUtilityModel PatentType = "utility_model" // 实用新型专利
)

// TechDomain 枚举技术领域，用于领域自适应撰写策略。
type TechDomain string

const (
	DomainMechanical TechDomain = "mechanical" // 机械领域
	DomainElectrical TechDomain = "electrical" // 电学/电路领域
	DomainChemical   TechDomain = "chemical"   // 化学/材料领域
	DomainSoftware   TechDomain = "software"   // 软件/计算机领域
	DomainGeneral    TechDomain = "general"    // 通用（未识别）
)

// SpecSectionName 标识说明书的标准章节。
type SpecSectionName string

const (
	SecTitle      SpecSectionName = "title"      // 发明/实用新型名称
	SecTechField  SpecSectionName = "tech_field" // 技术领域
	SecBackground SpecSectionName = "background" // 背景技术
	SecContent    SpecSectionName = "content"    // 发明/实用新型内容
	SecDrawings   SpecSectionName = "drawings"   // 附图说明
	SecEmbodiment SpecSectionName = "embodiment" // 具体实施方式
	SecAbstract   SpecSectionName = "abstract"   // 摘要
)

// requiredSections 是说明书必须包含的标准章节列表（按撰写顺序）。
var requiredSections = []SpecSectionName{
	SecTechField,
	SecBackground,
	SecContent,
	SecDrawings,
	SecEmbodiment,
}

// Severity 表示违规的严重程度（复用 claimdrafting 的模式）。
type Severity string

const (
	SeverityError   Severity = "error"   // 严重违法
	SeverityWarning Severity = "warning" // 潜在风险
	SeverityInfo    Severity = "info"    // 建议改进
)

// =============================================================================
// 输入类型（轻量镜像，避免循环依赖）
// =============================================================================

// SpecFeature 是最小技术单元——不可再分的原子技术手段。
type SpecFeature struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`               // structure / method / parameter / material
	Function    string `json:"function"`               // 技术功能
	Importance  string `json:"importance"`             // high / medium / low
	PriorStatus string `json:"prior_status,omitempty"` // known / unknown / partial
}

// SpecPFETriple 是问题-特征-效果的因果关系链。
type SpecPFETriple struct {
	ID         string   `json:"id"`
	Problem    string   `json:"problem"`
	FeatureIDs []string `json:"feature_ids"`
	Effect     string   `json:"effect"`
}

// SpecInput 是说明书撰写的完整输入数据。
// 通过 SpecInputFromExtraction 从 disclosure.ExtractionResult 转换而来。
type SpecInput struct {
	Title       string            `json:"title"`                  // 发明/实用新型名称
	PatentType  PatentType        `json:"patent_type"`            // 发明或实用新型
	TechDomain  TechDomain        `json:"tech_domain,omitempty"`  // 技术领域（空则自动识别）
	HasDrawings bool              `json:"has_drawings"`           // 是否有附图
	Problems    []string          `json:"problems"`               // 要解决的技术问题
	Features    []SpecFeature     `json:"features"`               // 技术特征列表
	Effects     []string          `json:"effects"`                // 技术效果
	PFETriples  []SpecPFETriple   `json:"pfe_triples"`            // 问题-特征-效果关联
	PriorArt    string            `json:"prior_art,omitempty"`    // 最接近的现有技术描述
	Claims      []string          `json:"claims,omitempty"`       // 权利要求文本（辅助撰写）
	DocSections map[string]string `json:"doc_sections,omitempty"` // 原始交底书章节内容
}

// =============================================================================
// 输出类型
// =============================================================================

// SpecSection 是说明书的一个标准章节。
type SpecSection struct {
	Name    SpecSectionName `json:"name"`
	Title   string          `json:"title"`    // 章节标题（如"技术领域"）
	Content string          `json:"content"`  // 章节正文
	WordCnt int             `json:"word_cnt"` // 中文字数
}

// SpecOutput 是说明书撰写的完整输出。
type SpecOutput struct {
	Title     string        `json:"title"`    // 发明/实用新型名称
	Abstract  string        `json:"abstract"` // 说明书摘要（不超过300字）
	Sections  []SpecSection `json:"sections"` // 各章节内容（有序）
	Warnings  []string      `json:"warnings,omitempty"`
	Score     float64       `json:"score,omitempty"` // 综合质量评分（0-100）
	Metadata  SpecMetadata  `json:"metadata"`
	Timestamp string        `json:"timestamp"`
}

// SpecMetadata 携带说明书撰写过程的诊断数据。
type SpecMetadata struct {
	PatentType   PatentType `json:"patent_type"`
	TechDomain   TechDomain `json:"tech_domain"`
	FeatureCount int        `json:"feature_count"`
	HasDrawings  bool       `json:"has_drawings"`
	TemplateUsed string     `json:"template_used,omitempty"`
	WordCount    int        `json:"word_count"`
}

// =============================================================================
// 验证与评分
// =============================================================================

// Violation 记录一条规则违规信息。
type Violation struct {
	RuleName    string   `json:"rule_name"`
	RuleBasis   string   `json:"rule_basis,omitempty"`
	Severity    Severity `json:"severity"`
	SectionName string   `json:"section_name,omitempty"`
	Message     string   `json:"message"`
	Suggestion  string   `json:"suggestion"`
}

// ScoreReport 是质量评估的完整报告。
type ScoreReport struct {
	OverallScore    float64            `json:"overall_score"`
	DimensionScores map[string]float64 `json:"dimension_scores"`
	Violations      []Violation        `json:"violations"`
	Suggestions     []string           `json:"suggestions"`
	Grade           string             `json:"grade"` // A/B/C/D
}

// 评分维度常量
const (
	DimCompleteness     = "completeness"      // 结构完整性
	DimClarity          = "clarity"           // 清楚性
	DimSupport          = "support"           // 支持性
	DimFormality        = "formality"         // 形式规范
	DimDomainAdaptation = "domain_adaptation" // 领域适配性
)

// SpecDimensionWeights 各评分维度的权重。
var SpecDimensionWeights = map[string]float64{
	DimCompleteness:     0.25,
	DimClarity:          0.25,
	DimSupport:          0.20,
	DimFormality:        0.15,
	DimDomainAdaptation: 0.15,
}

// =============================================================================
// Pregel State Keys
// =============================================================================

const (
	StateKeyInput      = "spec_input"
	StateKeyDomain     = "spec_domain"
	StateKeyTitle      = "spec_title"
	StateKeyTechField  = "spec_tech_field"
	StateKeyBackground = "spec_background"
	StateKeyContent    = "spec_content"
	StateKeyDrawings   = "spec_drawings"
	StateKeyEmbodiment = "spec_embodiment"
	StateKeyAbstract   = "spec_abstract"
	StateKeyOutput     = "spec_output"
	StateKeyScore      = "spec_score"
)

// =============================================================================
// 输入转换
// =============================================================================

// SpecInputFromExtraction 将 disclosure.ExtractionResult 转换为 SpecInput。
// 这是本模块与 disclosure 管线的标准集成入口。
func SpecInputFromExtraction(ext *disclosure.ExtractionResult, patentType PatentType, hasDrawings bool, claims []string) *SpecInput {
	input := &SpecInput{
		PatentType:  patentType,
		HasDrawings: hasDrawings,
		Claims:      claims,
	}
	if ext == nil {
		return input
	}
	if len(ext.Problems) > 0 {
		input.Title = ext.Problems[0]
	}
	if input.Title == "" && len(ext.PFETriples) > 0 {
		input.Title = ext.PFETriples[0].Problem
	}
	input.Problems = ext.Problems
	input.Effects = ext.Effects
	for _, f := range ext.Features {
		input.Features = append(input.Features, SpecFeature{
			ID:          f.ID,
			Description: f.Description,
			Category:    string(f.Category),
			Function:    f.Function,
			Importance:  f.Importance,
			PriorStatus: f.PriorArtStatus,
		})
	}
	for _, t := range ext.PFETriples {
		input.PFETriples = append(input.PFETriples, SpecPFETriple{
			ID:         t.ID,
			Problem:    t.Problem,
			FeatureIDs: t.FeatureIDs,
			Effect:     t.Effect,
		})
	}
	return input
}

// =============================================================================
// 帮助函数
// =============================================================================

// ChineseCharCount 统计字符串中的中文字符数。
func ChineseCharCount(s string) int {
	count := 0
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			count++
		}
	}
	return count
}

// timestamp 返回当前 UTC 时间戳。
func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
