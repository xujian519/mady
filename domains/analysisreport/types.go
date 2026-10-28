package analysisreport

import "github.com/xujian519/mady/domains/checker"

// AnalysisFeatureType 是技术特征类型（启发式判定）。
type AnalysisFeatureType string

// Feature type constants.
const (
	FeatureMethod   AnalysisFeatureType = "method"
	FeatureMaterial AnalysisFeatureType = "material"
	FeatureData     AnalysisFeatureType = "data"
	FeatureSystem   AnalysisFeatureType = "system"
	FeatureDevice   AnalysisFeatureType = "device"
	FeatureOther    AnalysisFeatureType = "other"
)

// FeatureImportance 是技术特征的重要性等级。
type FeatureImportance string

// Importance constants.
const (
	ImportanceHigh   FeatureImportance = "high"
	ImportanceMedium FeatureImportance = "medium"
	ImportanceLow    FeatureImportance = "low"
)

// QualityDomain 是质量评分维度。
type QualityDomain string

// Quality domain constants.
const (
	DomainNovelty           QualityDomain = "novelty"
	DomainClarity           QualityDomain = "clarity"
	DomainCompleteness      QualityDomain = "completeness"
	DomainTechnicalStrength QualityDomain = "technical_strength"
)

// ScoreOrder 是质量评分的稳定输出顺序。
var ScoreOrder = []QualityDomain{DomainNovelty, DomainClarity, DomainCompleteness, DomainTechnicalStrength}

// ScoreBasis 是评分来源。
type ScoreBasis string

// Score basis constants.
const (
	BasisDeterministic ScoreBasis = "deterministic"
	BasisModel         ScoreBasis = "model"
)

// ExtractedFeature 是一个已抽取的技术特征（v1 以权利要求为粒度）。
type ExtractedFeature struct {
	ClaimNo    int                 `json:"claim_no"`
	Text       string              `json:"text"`
	Type       AnalysisFeatureType `json:"type"`
	Importance FeatureImportance   `json:"importance"`
	Confidence float64             `json:"confidence"`
}

// FeatureStatistics 是特征分布统计。
type FeatureStatistics struct {
	ByType       map[AnalysisFeatureType]int `json:"by_type"`
	ByImportance map[FeatureImportance]int   `json:"by_importance"`
	Total        int                         `json:"total"`
}

// QualityScore 是单维质量评分（0-100）。
type QualityScore struct {
	Domain    QualityDomain `json:"domain"`
	Score     float64       `json:"score"`
	Basis     ScoreBasis    `json:"basis"`
	Rationale string        `json:"rationale"`
}

// ModelScore 是模型补分条目。
type ModelScore struct {
	Score     float64 `json:"score"`
	Rationale string  `json:"rationale"`
}

// SearchStrategy 是标准化检索策略记录。
type SearchStrategy struct {
	Query    string            `json:"query"`
	IPC      []string          `json:"ipc,omitempty"`
	Keywords []string          `json:"keywords,omitempty"`
	Filters  map[string]string `json:"filters,omitempty"`
	Hits     int               `json:"hits,omitempty"`
}

// IpcSummary 是 IPC 分类在报告中的摘要形态。
type IpcSummary struct {
	Section             string   `json:"section"`
	DomainName          string   `json:"domain_name"`
	Confidence          float64  `json:"confidence"`
	MatchedKeywords     []string `json:"matched_keywords,omitempty"`
	NoveltyImplications []string `json:"novelty_implications,omitempty"`
}

// CheckerFailure 是一条检查失败摘要。
type CheckerFailure struct {
	RuleID   string                `json:"rule_id"`
	Message  string                `json:"message"`
	Severity checker.IssueSeverity `json:"severity"`
}

// GraphConclusionSummary 是 graph 三性结论摘要。
type GraphConclusionSummary struct {
	Novelty       string `json:"novelty,omitempty"`
	Inventiveness string `json:"inventiveness,omitempty"`
	Enablement    string `json:"enablement,omitempty"`
	CitationsOK   *bool  `json:"citations_ok,omitempty"`
}

// KgAnalysisSummary 是知识图谱增强统计。
type KgAnalysisSummary struct {
	MatchedEntities int     `json:"matched_entities"`
	RelatedEntities int     `json:"related_entities"`
	Confidence      float64 `json:"confidence"`
}

// PatentAnalysisReport 是统一分析产物。
type PatentAnalysisReport struct {
	PatentID             string                  `json:"patent_id,omitempty"`
	Title                string                  `json:"title,omitempty"`
	IPC                  []IpcSummary            `json:"ipc"`
	TechnicalFeatures    []ExtractedFeature      `json:"technical_features"`
	FeatureStatistics    FeatureStatistics       `json:"feature_statistics"`
	Scores               []QualityScore          `json:"scores"`
	CheckerVerdict       checker.VerdictStatus   `json:"checker_verdict,omitempty"`
	CheckerRan           bool                    `json:"checker_ran"`
	CheckerFailures      []CheckerFailure        `json:"checker_failures"`
	GraphConclusions     *GraphConclusionSummary `json:"graph_conclusions,omitempty"`
	EvidenceReceiptCount *int                    `json:"evidence_receipt_count,omitempty"`
	KgAnalysis           *KgAnalysisSummary      `json:"kg_analysis,omitempty"`
	InnovationInsights   []string                `json:"innovation_insights"`
	ExpertConsiderations []string                `json:"expert_considerations"`
	SearchStrategy       *SearchStrategy         `json:"search_strategy,omitempty"`
}
