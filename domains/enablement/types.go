package enablement

// =============================================================================
// 输入/输出类型（值类型，不依赖 disclosure 包）
// =============================================================================

// TechFeature 是技术特征（镜像 disclosure.TechFeature 结构）。
type TechFeature struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`   // structure / method / parameter / material
	Function    string `json:"function"`   // 技术功能
	Importance  string `json:"importance"` // high / medium / low
}

// PFETriple 是问题-特征-效果三元组（镜像 disclosure.PFETriple 结构）。
type PFETriple struct {
	ID         string   `json:"id"`
	Problem    string   `json:"problem"`
	FeatureIDs []string `json:"feature_ids"`
	Effect     string   `json:"effect"`
}

// EnablementInput 是充分公开评估子图的完整输入。
type EnablementInput struct {
	// 技术特征与 PFE 因果链（来自 disclosure 管线的 ExtractionResult）
	Features   []TechFeature `json:"features"`
	PFETriples []PFETriple   `json:"pfe_triples"`
	Problems   []string      `json:"problems"`
	Effects    []string      `json:"effects"`

	// 说明书章节内容（9 段切分结果，key 为章节名如 "technical_field"）
	DocSections map[string]string `json:"doc_sections"`
	HasDrawings bool              `json:"has_drawings"` // 是否有附图

	// 知识检索增强
	GuidelineRefs []string `json:"guideline_refs,omitempty"` // 审查指南相关条款引用
	SimilarCases  []string `json:"similar_cases,omitempty"`  // 类似案例引用

	// 证据覆盖状态
	EvidenceCoverage string `json:"evidence_coverage"` // "full" / "partial" / "none"
}

// EnablementResult 是充分公开评估子图的完整输出。
type EnablementResult struct {
	Assessed   bool   `json:"assessed"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`

	// 三步骤评估结果
	Completeness CompletenessResult `json:"completeness"`
	Clarity      ClarityResult      `json:"clarity"`
	Enablement   EnablementJudgment `json:"enablement"`

	// 最终结论
	Conclusion   string   `json:"conclusion"`    // 整体判断文本
	IsSufficient bool     `json:"is_sufficient"` // 是否满足 26.3 充分公开要求
	Confidence   string   `json:"confidence"`    // high / medium / low
	Deficiencies []string `json:"deficiencies"`  // 具体缺陷列表
}

// CompletenessResult 是 Step 1（结构完整性）的评估结果。
type CompletenessResult struct {
	MissingSections []string `json:"missing_sections"` // 缺失的必要章节
	Score           float64  `json:"score"`            // 完整性得分 0.0-1.0
	Notes           string   `json:"notes"`            // 评估说明
}

// ClarityResult 是 Step 2（清楚性）的评估结果。
type ClarityResult struct {
	IsClear        bool     `json:"is_clear"`        // 技术术语是否含义明确
	AmbiguousTerms []string `json:"ambiguous_terms"` // 存在歧义的术语
	OrphanFeatures []string `json:"orphan_features"` // 孤立特征（无效果对应）
	OrphanEffects  []string `json:"orphan_effects"`  // 孤立效果（无特征对应）
	Notes          string   `json:"notes"`           // 评估说明
}

// EnablementJudgment 是 Step 3（能够实现性）的评估结果。
// 覆盖审查指南第二部分第二章第 2.1 节规定的四种公开不充分经典情形。
type EnablementJudgment struct {
	CanImplement   bool     `json:"can_implement"`   // 本领域技术人员能否实施
	FailureReasons []string `json:"failure_reasons"` // 不能实施的具体原因

	// 四种公开不充分情形的命中标志
	MissingKeyMeans  bool `json:"missing_key_means"`  // 缺少关键技术手段的说明
	VagueMeans       bool `json:"vague_means"`        // 技术手段含糊不清
	OnlyTaskNoMeans  bool `json:"only_task_no_means"` // 仅给出任务/设想，未给出具体技术手段
	InsufficientData bool `json:"insufficient_data"`  // 实验数据不足以证明技术效果

	Notes string `json:"notes"` // 评估说明
}

// =============================================================================
// 5 项必要章节（patent-core.yaml 中定义）
// =============================================================================

// requiredSections 是说明书必须包含的 5 项章节标签。
var requiredSections = []string{
	"技术领域",
	"背景技术",
	"发明内容（要解决的技术问题、技术方案、有益效果）",
	"附图说明（如有附图）",
	"具体实施方式（至少一个实施例）",
}

// RequiredSectionCount 返回必备章节数量。
func RequiredSectionCount() int {
	return len(requiredSections)
}

// SectionLabel 返回第 i 个必要章节的中文标签。
func SectionLabel(i int) string {
	if i < 0 || i >= len(requiredSections) {
		return ""
	}
	return requiredSections[i]
}
