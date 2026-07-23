package novelty

// =============================================================================
// 输入类型
// =============================================================================

// ClaimText 是单条权利要求的文本表示。
type ClaimText struct {
	ID   string `json:"id"`   // 权利要求编号，如 "1", "2"
	Text string `json:"text"` // 权利要求全文
	Type string `json:"type"` // independent / dependent
}

// PriorArtDoc 是一篇对比文件（现有技术文献）。
type PriorArtDoc struct {
	DocID   string  `json:"doc_id"`   // 文献标识
	Title   string  `json:"title"`    // 标题
	Snippet string  `json:"snippet"`  // 相关段落
	PubDate string  `json:"pub_date"` // 公开日期
	PubType string  `json:"pub_type"` // written / use / sale / online / oral
	Score   float64 `json:"score"`    // 相关度
}

// ConflictApp 是一篇抵触申请（在先申请在后公开）。
type ConflictApp struct {
	AppID      string `json:"app_id"`      // 申请号
	Title      string `json:"title"`       // 标题
	FilingDate string `json:"filing_date"` // 在先申请日
	PubDate    string `json:"pub_date"`    // 公开日（应在在后申请日之后）
	FullText   string `json:"full_text"`   // 全文内容（说明书+权利要求书+附图描述）
}

// PriorityInfo 记录优先权信息。
type PriorityInfo struct {
	HasPriority  bool   `json:"has_priority"`
	PriorityDate string `json:"priority_date"`
	PriorityType string `json:"priority_type"` // international / domestic
	IsFirstApp   bool   `json:"is_first_app"`  // 是否首次申请
}

// GraceInfo 记录宽限期信息。
type GraceInfo struct {
	HasGraceClaim   bool   `json:"has_grace_claim"` // 是否主张宽限期
	GraceType       string `json:"grace_type"`      // exhibition / conference / leak
	GraceDate       string `json:"grace_date"`      // 公开发生日期
	WithinSixMonths bool   `json:"within_six_months"`
}

// NoveltyInput 是新颖性分析的完整输入。
type NoveltyInput struct {
	Claims           []ClaimText   `json:"claims"`                      // 权利要求列表
	Spec             string        `json:"spec,omitempty"`              // 说明书（可选）
	TechDomain       string        `json:"tech_domain"`                 // 技术领域
	FilingDate       string        `json:"filing_date"`                 // 申请日
	PriorityDate     string        `json:"priority_date,omitempty"`     // 优先权日（可选）
	PriorityInfo     *PriorityInfo `json:"priority_info,omitempty"`     // 优先权信息（可选）
	PriorArtDocs     []PriorArtDoc `json:"prior_art_docs"`              // 对比文件列表
	ConflictApps     []ConflictApp `json:"conflict_apps,omitempty"`     // 抵触申请（可选）
	GracePeriodInfo  *GraceInfo    `json:"grace_period_info,omitempty"` // 宽限期信息（可选）
	EvidenceCoverage string        `json:"evidence_coverage"`           // full / partial / none
}

// =============================================================================
// 输出类型（子结果）
// =============================================================================

// PriorArtResult 现有技术审查结果。
type PriorArtResult struct {
	EffectiveDate          string `json:"effective_date"`           // 有效申请日
	IsPubliclyKnown        bool   `json:"is_publicly_known"`        // 是否"为公众所知"
	PublicKnownStd         string `json:"public_known_std"`         // strict / lenient
	IsSufficientDisclosure bool   `json:"is_sufficient_disclosure"` // 充分公开
	PriorArtType           string `json:"prior_art_type"`           // written / use / sale / online / oral
	DisclosureReason       string `json:"disclosure_reason"`        // 判断理由
}

// CompareResult 单独对比结果。
type CompareResult struct {
	ClaimFeatures       []string `json:"claim_features"`        // 权利要求技术特征列表
	DisclosedFeatures   []string `json:"disclosed_features"`    // 对比文件公开的特征
	MissingFeatures     []string `json:"missing_features"`      // 未被公开的特征
	SameField           bool     `json:"same_field"`            // 技术领域是否相同
	SameProblem         bool     `json:"same_problem"`          // 技术问题是否相同
	SameEffect          bool     `json:"same_effect"`           // 预期效果是否相同
	UpperLowerConcept   string   `json:"upper_lower_concept"`   // same / different / n_a
	DirectReplacement   bool     `json:"direct_replacement"`    // 惯用手段直接置换
	NumericRangeResult  string   `json:"numeric_range_result"`  // overlapped / inside_without_endpoint / no_overlap / n_a
	FullFeatureCoverage bool     `json:"full_feature_coverage"` // 是否全部特征被公开
}

// ConflictResult 抵触申请审查结果。
type ConflictResult struct {
	IsConflictApp      bool     `json:"is_conflict_app"` // 构成抵触申请
	ConflictReasons    []string `json:"conflict_reasons,omitempty"`
	FullContentCompare bool     `json:"full_content_compare"` // 全文比对
	ConflictDocID      string   `json:"conflict_doc_id,omitempty"`
}

// ExceptionResult 宽限期与优先权例外审查结果。
type ExceptionResult struct {
	HasGracePeriod bool   `json:"has_grace_period"`
	GraceType      string `json:"grace_type,omitempty"` // exhibition / conference / leak
	GraceWithin6m  bool   `json:"grace_within_6m"`
	HasPriority    bool   `json:"has_priority"`
	PriorityValid  bool   `json:"priority_valid"`
	SameSubject    string `json:"same_subject,omitempty"` // "相同主题"判断
}

// NoveltyResult 是新颖性分析的完整输出。
type NoveltyResult struct {
	Assessed   bool   `json:"assessed"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`

	PriorArtCheck PriorArtResult  `json:"prior_art_check"`
	SingleCompare CompareResult   `json:"single_compare"`
	ConflictCheck ConflictResult  `json:"conflict_check"`
	GracePriority ExceptionResult `json:"grace_priority"`

	Conclusion   string   `json:"conclusion"`
	HasNovelty   bool     `json:"has_novelty"`
	Confidence   string   `json:"confidence,omitempty"`    // high / medium / low
	FailedClaims []string `json:"failed_claims,omitempty"` // 不具备新颖性的权利要求 ID 列表
}

// parsedConclusion 是 LLM 最终结论输出的解析结构。
type parsedConclusion struct {
	Conclusion   string   `json:"conclusion"`
	HasNovelty   bool     `json:"has_novelty"`
	Confidence   string   `json:"confidence"`
	FailedClaims []string `json:"failed_claims"`
}

// =============================================================================
// State Key 常量
// =============================================================================

const (
	StateKeyNoveltyInput  = "novelty_input"
	StateKeyNoveltyResult = "novelty_result"
	stateKeyPriorArt      = "novelty_prior_art"
	stateKeyCompare       = "novelty_compare"
	stateKeyConflict      = "novelty_conflict"
	stateKeyGracePriority = "novelty_grace_priority"
	stateKeySpecialDomain = "novelty_special_domain"
)
