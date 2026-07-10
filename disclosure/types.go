// Package disclosure 实现技术交底书分析的 Pregel 管线。
//
// 管线接收原始交底书文本，依次执行：
//   - 预处理（9 段章节切分）
//   - 并行提取（技术问题 / 技术特征 / 技术效果，各写独立 state key）
//   - 合并提取（merge_extractions 节点统合三个输出为 ExtractionResult）
//   - 一致性校验（PFE 三元组闭包检查，最多 2 轮回退）
//   - 检索关键词生成 + 新颖性初判
//   - 报告生成 + 人工复核
package disclosure

import (
	"encoding/json"
	"time"

	"github.com/xujian519/mady/graph"
)

// ExtractReportFromState 从 PregelState 中提取分析报告。
// 供 server 包和工具层复用，避免重复代码。
func ExtractReportFromState(state graph.PregelState) *AnalysisReport {
	if report, ok := state[StateKeyReport].(*AnalysisReport); ok {
		return report
	}
	if raw, ok := state[StateKeyReport].(string); ok && raw != "" {
		var report AnalysisReport
		if err := json.Unmarshal([]byte(raw), &report); err == nil {
			return &report
		}
	}
	return nil
}

// DocSection 枚举中文专利交底书的 9 个标准段落。
type DocSection string

const (
	SecTitle       DocSection = "invention_title"    // 发明名称
	SecTechField   DocSection = "technical_field"    // 技术领域
	SecBackground  DocSection = "background"         // 背景技术
	SecContent     DocSection = "content"            // 发明内容
	SecProblem     DocSection = "technical_problem"  // 要解决的技术问题
	SecSolution    DocSection = "technical_solution" // 技术方案
	SecEffect      DocSection = "beneficial_effect"  // 有益效果
	SecEmbodiments DocSection = "embodiments"        // 具体实施方式
	SecDrawings    DocSection = "drawings"           // 附图说明
)

// sectionPatterns 按优先级排列的章节标题关键词，用于分段匹配。
var sectionPatterns = []struct {
	Key      DocSection
	Keywords []string
}{
	{SecTitle, []string{"发明名称", "实用新型名称"}},
	{SecTechField, []string{"技术领域"}},
	{SecBackground, []string{"背景技术", "技术背景"}},
	{SecContent, []string{"发明内容", "实用新型内容", "发明概述"}},
	{SecProblem, []string{"要解决的技术问题", "本发明所要解决的技术问题"}},
	{SecSolution, []string{"技术方案", "技术解决方案"}},
	{SecEffect, []string{"有益效果", "技术效果", "积极效果"}},
	{SecEmbodiments, []string{"具体实施方式", "具体实施例", "实施例"}},
	{SecDrawings, []string{"附图说明"}},
}

// DisclosureDoc 是解析后的结构化技术交底书。
type DisclosureDoc struct {
	ID          string                `json:"id"`
	Title       string                `json:"title"`
	RawText     string                `json:"raw_text"`
	Sections    map[DocSection]string `json:"sections"`
	FigureRefs  []string              `json:"figure_refs"`
	HasDrawings bool                  `json:"has_drawings"`
	Format      string                `json:"format"` // "txt", "word", "pdf"
	ParsedAt    time.Time             `json:"parsed_at"`
}

// TechFeatureCategory 是技术特征的分类（参照 XiaoNuo "最小技术单元" 4 分类）。
type TechFeatureCategory string

const (
	CatStructure TechFeatureCategory = "structure" // 结构特征
	CatMethod    TechFeatureCategory = "method"    // 方法/工艺特征
	CatParameter TechFeatureCategory = "parameter" // 参数特征
	CatMaterial  TechFeatureCategory = "material"  // 材料特征
)

// TechFeature 是一个最小技术单元——不可再分的原子技术手段。
type TechFeature struct {
	ID              string              `json:"id"`
	Description     string              `json:"description"`
	Category        TechFeatureCategory `json:"category"`
	Function        string              `json:"function"`
	RelatedEffectID string              `json:"related_effect_id,omitempty"`
	PriorArtStatus  string              `json:"prior_art_status"` // "known" / "unknown" / "partial"
	Importance      string              `json:"importance"`       // "high" / "medium" / "low"
	Confidence      float64             `json:"confidence"`
}

// PFETriple 是问题-特征-效果的因果关系链（Problem-Feature-Effect）。
type PFETriple struct {
	ID         string   `json:"id"`
	Problem    string   `json:"problem"`
	FeatureIDs []string `json:"feature_ids"`
	Effect     string   `json:"effect"`
	LogicChain string   `json:"logic_chain"`
}

// ConsistencyIssue 描述一致性校验发现的一处不一致。
type ConsistencyIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`    // "error" / "warning" / "info"
	SourceNode  string `json:"source_node"` // "extract_problem" / "extract_features" / "extract_effects"
}

// ConsistencyResult 是一致性校验节点的输出。
type ConsistencyResult struct {
	Pass             bool               `json:"pass"`
	Issues           []ConsistencyIssue `json:"issues"`
	OverallScore     float64            `json:"overall_score"`
	Feedback         string             `json:"feedback"`          // 回退重提取时的改进提示
	RetriesExhausted bool               `json:"retries_exhausted"` // 超过最大重试次数后 fail-open
}

// ExtractionResult 聚合三个提取节点的输出。
type ExtractionResult struct {
	Problems   []string      `json:"problems"`
	Features   []TechFeature `json:"features"`
	Effects    []string      `json:"effects"`
	PFETriples []PFETriple   `json:"pfe_triples"`
}

// NoveltyResult 是新颖性初判的输出（Phase 2 stub 实现）。
type NoveltyResult struct {
	Assessed   bool   `json:"assessed"`
	Conclusion string `json:"conclusion"`
	Notes      string `json:"notes"`
}

// AnalysisReport 是最终的结构化分析报告。
type AnalysisReport struct {
	ID              string             `json:"id"`
	Document        *DisclosureDoc     `json:"document"`
	Extraction      *ExtractionResult  `json:"extraction"`
	Consistency     *ConsistencyResult `json:"consistency"`
	SearchKeywords  []string           `json:"search_keywords"`
	Novelty         *NoveltyResult     `json:"novelty"`
	ReportText      string             `json:"report_text"`
	GeneratedAt     time.Time          `json:"generated_at"`
	ReviewedByHuman bool               `json:"reviewed_by_human"`
}

// =============================================================================
// PregelState 键值常量
// =============================================================================

const (
	StateKeyInput           = "input"
	StateKeyOutput          = "output"
	StateKeyDoc             = "document"
	StateKeyExtractProblem  = "extract_problem_output"  // 问题提取 Agent 的原始输出
	StateKeyExtractFeatures = "extract_features_output" // 特征提取 Agent 的原始输出
	StateKeyExtractEffects  = "extract_effects_output"  // 效果提取 Agent 的原始输出
	StateKeyExtraction      = "extraction_result"       // 合并后的 ExtractionResult
	StateKeyConsistency     = "consistency_result"
	StateKeyRetryCount      = "retry_count"
	StateKeyRetryFeedback   = "retry_feedback" // 一致性校验失败时传递给提取 Agent 的反馈
	StateKeySearchKeywords  = "search_keywords"
	StateKeyNovelty         = "novelty_result"
	StateKeyReport          = "report"
	StateKeyErrors          = "errors"
)
