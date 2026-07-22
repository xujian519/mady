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

	// 技术领域（自动检测）
	TechDomain string `json:"tech_domain,omitempty"` // chemical/biotech/tcm/computer/mechanical/electronic/general

	// 三步骤评估结果
	Completeness CompletenessResult `json:"completeness"`
	Clarity      ClarityResult      `json:"clarity"`
	Enablement   EnablementJudgment `json:"enablement"`

	// 实验数据检查（领域自适应）
	DataAssessment *ExperimentDataAssessment `json:"data_assessment,omitempty"`

	// 最终结论
	Conclusion   string   `json:"conclusion"`    // 整体判断文本
	IsSufficient bool     `json:"is_sufficient"` // 是否满足 26.3 充分公开要求
	Confidence   string   `json:"confidence"`    // high / medium / low
	Deficiencies []string `json:"deficiencies"`  // 具体缺陷列表

	// 联动提示
	SupportIssue    bool     `json:"support_issue,omitempty"`    // 是否提示26.4不支持联动风险
	SupportWarnings []string `json:"support_warnings,omitempty"` // 26.4联动风险说明
}

// CompletenessResult 是 Step 1（结构完整性）的评估结果。
type CompletenessResult struct {
	MissingSections []string `json:"missing_sections"` // 缺失的必要章节
	Score           float64  `json:"score"`            // 完整性得分 0.0-1.0
	Notes           string   `json:"notes"`            // 评估说明
}

// ClarityResult 是 Step 2（清楚性）的评估结果。
// 覆盖审查指南第二部分第二章 §2.1.1「清楚」的三种常见问题。
type ClarityResult struct {
	IsClear        bool     `json:"is_clear"`        // 技术术语是否含义明确
	AmbiguousTerms []string `json:"ambiguous_terms"` // 歧义术语：领域内存在多种理解且未界定
	CoinedTerms    []string `json:"coined_terms"`    // 自造词：非领域常规术语且未给出明确定义
	ObviousErrors  []string `json:"obvious_errors"`  // 明显错误：技术人员能识别但有歧义的笔误/矛盾
	OrphanFeatures []string `json:"orphan_features"` // 孤立特征（无效果对应）
	OrphanEffects  []string `json:"orphan_effects"`  // 孤立效果（无特征对应）
	Notes          string   `json:"notes"`           // 评估说明
}

// EnablementJudgment 是 Step 3（能够实现性）的评估结果。
// 覆盖审查指南第二部分第二章第 2.1.3 节规定的六种公开不充分经典情形。
type EnablementJudgment struct {
	CanImplement   bool     `json:"can_implement"`   // 本领域技术人员能否实施
	FailureReasons []string `json:"failure_reasons"` // 不能实施的具体原因

	// 六种公开不充分情形的命中标志（对应审查指南 §2.1.3 列举的五种 + 实验数据）
	MissingKeyMeans    bool `json:"missing_key_means"`          // ① 缺少关键技术手段的说明
	VagueMeans         bool `json:"vague_means"`                // ② 技术手段含糊不清，无法具体实施
	OnlyTaskNoMeans    bool `json:"only_task_no_means"`         // ③ 仅给出任务/设想，未给出任何技术手段
	MeansCannotSolve   bool `json:"means_cannot_solve"`         // ④ 给出了技术手段，但不能解决技术问题（如违背物理原理）
	PartialMeansUnreal bool `json:"partial_means_unrealizable"` // ⑤ 多手段方案中某一手段不能实现
	InsufficientData   bool `json:"insufficient_data"`          // ⑥ 方案须依赖实验结果但未给出实验证据

	Notes string `json:"notes"` // 评估说明
}

// ExperimentDataAssessment 是实验数据有效性的专项检查结果。
// 基于审查指南和司法实践中的实验数据规则体系。
type ExperimentDataAssessment struct {
	DataNeeded     bool     `json:"data_needed"`     // 技术效果是否需要实验数据支撑
	DataProvided   bool     `json:"data_provided"`   // 说明书是否提供了实验数据
	IsValid        bool     `json:"is_valid"`        // 提供的实验数据是否有效（四要素齐全）
	MissingFactors []string `json:"missing_factors"` // 实验数据缺失的要素
	Notes          string   `json:"notes"`           // 评估说明
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
