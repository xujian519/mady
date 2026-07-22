package inventiveness

// =============================================================================
// Input/Output 类型（值类型，不依赖 disclosure 包）
// =============================================================================

// EvidenceChunk 是检索到的现有技术证据片段（镜像 disclosure.EvidenceChunk 结构）。
type EvidenceChunk struct {
	DocID   string  `json:"doc_id"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// TechFeature 是技术特征（镜像 disclosure.TechFeature 结构）。
type TechFeature struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Function    string `json:"function"`
	Importance  string `json:"importance"`
}

// PFETriple 是问题-特征-效果三元组（镜像 disclosure.PFETriple 结构）。
type PFETriple struct {
	ID      string `json:"id"`
	Problem string `json:"problem"`
	Effect  string `json:"effect"`
}

// =============================================================================
// 发明类型常量
// =============================================================================

const (
	InventionTypeGeneric       = ""               // 通用（默认，不指定类型）
	InventionTypePioneering    = "pioneering"     // 开拓性发明：技术史上无先例的全新方案
	InventionTypeCombination   = "combination"    // 组合发明：将已知技术特征组合
	InventionTypeSelection     = "selection"      // 选择发明：从宽范围中选择窄范围或个体
	InventionTypeTransfer      = "transfer"       // 转用发明：将已知技术转用于新领域
	InventionTypeNewUse        = "new_use"        // 已知产品新用途发明
	InventionTypeElementChange = "element_change" // 要素变更发明：关系改变/替代/省略
)

// InventivenessInput 是创造性分析子图的完整输入。
type InventivenessInput struct {
	PriorArtChunks    []EvidenceChunk `json:"prior_art_chunks"`
	Features          []TechFeature   `json:"features"`
	PFETriples        []PFETriple     `json:"pfe_triples"`
	NoveltyConclusion string          `json:"novelty_conclusion"`
	EvidenceCoverage  string          `json:"evidence_coverage"`        // "full" / "partial" / "none"
	InventionType     string          `json:"invention_type,omitempty"` // 发明类型（可选，留空时自动分类）
	TechDomain        string          `json:"tech_domain,omitempty"`    // 技术领域：chemistry/computer/tcm（可选）
}

// =============================================================================
// 显著的进步类型常量
// =============================================================================

const (
	ProgressTypeEffectImprove = "effect_improve" // 效果改善型：与现有技术相比具有更好的技术效果
	ProgressTypeDifferentPath = "different_path" // 异途同归型：提供技术构思不同的技术方案，效果基本达到现有技术水平
	ProgressTypeTrendLeading  = "trend_leading"  // 趋势引领型：代表某种新技术发展趋势
	ProgressTypeTradeoff      = "tradeoff"       // 利弊权衡型：某些方面有负面效果，但其他方面具有明显积极的技术效果
)

// =============================================================================
// 三步法子结果类型（对标 enablement 的 CompletenessResult / ClarityResult / EnablementJudgment）
// =============================================================================

// Step1Result 三步法第 1 步：确定最接近的现有技术。
type Step1Result struct {
	ClosestPriorArt string `json:"closest_prior_art"` // 最接近的现有技术文献标识
	SelectionReason string `json:"selection_reason"`  // 选择理由
}

// Step2Result 三步法第 2 步：确定区别特征和实际解决的技术问题。
type Step2Result struct {
	DistinguishingFeatures  []string `json:"distinguishing_features"`   // 区别技术特征列表
	NonContributingFeatures []string `json:"non_contributing_features"` // 无贡献特征（2023审查指南新增）
	TechEffects             []string `json:"tech_effects"`              // 区别特征对应的技术效果
	ActualTechProblem       string   `json:"actual_tech_problem"`       // 重新确定的实际技术问题
}

// Step3Result 三步法第 3 步：判断现有技术整体上是否存在技术启示。
type Step3Result struct {
	TechnicalSuggestion bool   `json:"technical_suggestion"` // 是否存在技术启示
	SuggestionType      string `json:"suggestion_type"`      // common_knowledge/same_doc/other_doc/functional_equivalent/universal_need
	HasReverseTeaching  bool   `json:"has_reverse_teaching"` // 是否存在反向教导（对比文件明确教导不要采用该技术手段）
	IsCrossDomain       bool   `json:"is_cross_domain"`      // 是否跨领域结合
	Rationale           string `json:"rationale"`            // 详细推理过程
	Confidence          string `json:"confidence"`           // high / medium / low
}

// Step4Result 显著的进步判断：评估发明是否具有有益技术效果。
// 创造性 = 突出的实质性特点（Step3 非显而易见） AND 显著的进步（Step4 有益效果）。
type Step4Result struct {
	HasSignificantProgress bool   `json:"has_significant_progress"` // 是否具有显著的进步
	ProgressType           string `json:"progress_type"`            // effect_improve/different_path/trend_leading/tradeoff
	Rationale              string `json:"rationale"`                // 判断理由
}

// =============================================================================
// 输出类型
// =============================================================================

// ThreeStepResult 封装三步法的每一步产出（向后兼容，保留原有字段）。
type ThreeStepResult struct {
	ClosestPriorArt        string   `json:"closest_prior_art"`
	DistinguishingFeatures []string `json:"distinguishing_features,omitempty"`
	ActualTechProblem      string   `json:"actual_tech_problem"`
	TechnicalSuggestion    bool     `json:"technical_suggestion"`
	SuggestionRationale    string   `json:"suggestion_rationale"`
}

// InventivenessResult 是创造性分析子图的完整输出。
type InventivenessResult struct {
	Assessed   bool   `json:"assessed"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`

	// 三步法各步骤的结构化结果（v2 新增，细粒度）
	Step1 Step1Result `json:"step1"`
	Step2 Step2Result `json:"step2"`
	Step3 Step3Result `json:"step3"`
	Step4 Step4Result `json:"step4"` // 显著的进步判断（v3 新增）

	// 向后兼容的汇总字段（与旧版 API 保持一致）
	ThreeStep ThreeStepResult `json:"three_step_analysis"`

	// 最终结论
	Conclusion  string   `json:"conclusion"`            // 整体判断文本
	IsInventive bool     `json:"is_inventive"`          // 是否具备创造性
	Confidence  string   `json:"confidence"`            // high / medium / low
	AuxFactors  []string `json:"aux_factors,omitempty"` // 辅助考虑因素（商业成功、预料不到的技术效果等）
}

// parsedConclusion 是最终结论的 LLM 输出解析结构。
type parsedConclusion struct {
	Conclusion             string   `json:"conclusion"`
	IsInventive            bool     `json:"is_inventive"`
	HasSignificantProgress bool     `json:"has_significant_progress"` // 是否具有显著的进步
	Confidence             string   `json:"confidence"`
	AuxFactors             []string `json:"aux_factors"`
}
