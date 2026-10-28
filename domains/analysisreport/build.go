package analysisreport

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/domains/checker"
	"github.com/xujian519/mady/domains/ipc"
)

// 专利分析产物的确定性聚合器。
//
// 输入各引擎结果（checker 失败明细 / IPC 分类 / graph 结论 / 证据收据 / KG 统计）与
// 可选的模型补分（novelty / technical_strength），输出统一的 PatentAnalysisReport。
// clarity / completeness 由规则确定性推导；novelty / technical_strength 由调用方经
// 模型打分后注入。无模型补分时只含确定性维度。

const (
	baseScore                = 100.0
	fuzzyTermPenalty         = 5.0
	singleClaimPenalty       = 20.0
	missingAbstractPenalty   = 15.0
	narrowIndependentPenalty = 10.0
	independentMinLength     = 30
	highConfidenceScore      = 70.0
	newKnowledgePenalty      = 70.0
	// checkerNeedsRevisionWarnings：warning 级失败达到该数量时判 needs_revision
	// （对应上游「3 条 should 级失败」口径）。
	checkerNeedsRevisionWarnings = 3
)

// fuzzyTerms 是降低权利要求清晰度的模糊限定词。
var fuzzyTerms = []string{"约", "大约", "左右", "大概", "若干", "优选地"}

// Inputs 是聚合器输入。
type Inputs struct {
	PatentID string
	Title    string
	Claims   []string
	Abstract string
	// CheckerFailures 为 checker 失败明细；CheckerRan 为 true 时据此给出聚合判级。
	CheckerFailures []CheckerFailure
	CheckerRan      bool
	// IPC 为分类结果；为空时按 Claims+Abstract+IPCText 做确定性分类，未命中关键词则不写入。
	IPC                  []ipc.ClassificationResult
	IPCText              string
	Graph                *GraphConclusionSummary
	EvidenceReceiptCount *int
	KG                   *KgAnalysisSummary
	ModelScores          map[QualityDomain]ModelScore
	SearchStrategy       *SearchStrategy
}

// ComputeClarityScore 按模糊限定词数量给出清晰度评分。
func ComputeClarityScore(claims []string) QualityScore {
	if len(claims) == 0 {
		return QualityScore{Domain: DomainClarity, Score: 0, Basis: BasisDeterministic, Rationale: "缺少权利要求文本"}
	}
	found := make([]string, 0, len(fuzzyTerms))
	for _, term := range fuzzyTerms {
		for _, claim := range claims {
			if strings.Contains(claim, term) {
				found = append(found, term)
				break
			}
		}
	}
	score := baseScore - float64(len(found))*fuzzyTermPenalty
	if score < 0 {
		score = 0
	}
	rationale := "未检出模糊限定词"
	if len(found) > 0 {
		rationale = "检出模糊限定词：" + strings.Join(found, "、")
	}
	return QualityScore{Domain: DomainClarity, Score: score, Basis: BasisDeterministic, Rationale: rationale}
}

// ComputeCompletenessScore 按权利要求数量、摘要与独立权项长度给出完整性评分。
func ComputeCompletenessScore(claims []string, abstract string) QualityScore {
	if len(claims) == 0 {
		return QualityScore{Domain: DomainCompleteness, Score: 0, Basis: BasisDeterministic, Rationale: "缺少权利要求文本"}
	}
	score := baseScore
	var notes []string
	if len(claims) == 1 {
		score -= singleClaimPenalty
		notes = append(notes, "仅 1 条权利要求")
	}
	if strings.TrimSpace(abstract) == "" {
		score -= missingAbstractPenalty
		notes = append(notes, "缺摘要")
	}
	if len([]rune(claims[0])) < independentMinLength {
		score -= narrowIndependentPenalty
		notes = append(notes, "独立权利要求特征不充分")
	}
	if score < 0 {
		score = 0
	}
	rationale := "权利要求与摘要要素较完整"
	if len(notes) > 0 {
		rationale = strings.Join(notes, "；")
	}
	return QualityScore{Domain: DomainCompleteness, Score: score, Basis: BasisDeterministic, Rationale: rationale}
}

// MergeScores 合并确定性评分与模型补分（模型覆盖同维度），按稳定顺序输出。
func MergeScores(deterministic []QualityScore, modelScores map[QualityDomain]ModelScore) []QualityScore {
	byDomain := make(map[QualityDomain]QualityScore, len(deterministic)+len(modelScores))
	for _, score := range deterministic {
		byDomain[score.Domain] = score
	}
	for domain, model := range modelScores {
		byDomain[domain] = QualityScore{
			Domain: domain, Score: model.Score, Basis: BasisModel, Rationale: model.Rationale,
		}
	}
	merged := make([]QualityScore, 0, len(byDomain))
	for _, domain := range ScoreOrder {
		if score, ok := byDomain[domain]; ok {
			merged = append(merged, score)
		}
	}
	return merged
}

// AggregateCheckerVerdict 由失败明细给出聚合判级：任一 error 为 blocked，
// warning 达到 3 条为 needs_revision，否则 pass。
func AggregateCheckerVerdict(failures []CheckerFailure) checker.VerdictStatus {
	warnings := 0
	for _, failure := range failures {
		switch failure.Severity {
		case checker.SeverityError:
			return checker.StatusBlocked
		case checker.SeverityWarning:
			warnings++
		}
	}
	if warnings >= checkerNeedsRevisionWarnings {
		return checker.StatusNeedsRevision
	}
	return checker.StatusPass
}

// ToIpcSummary 把 IPC 分类映射为报告摘要（领域名与审查要点取自 IPC 领域表）。
func ToIpcSummary(classification ipc.ClassificationResult) IpcSummary {
	summary := IpcSummary{
		Section:         string(classification.Section),
		DomainName:      "未知",
		Confidence:      classification.Confidence,
		MatchedKeywords: classification.MatchedKeywords,
	}
	if domain, ok := ipc.AllDomains[classification.Section]; ok {
		summary.DomainName = domain.Name
		summary.NoveltyImplications = domain.InventivenessFocus
	}
	return summary
}

// BuildReport 聚合生成专利分析报告（确定性路径零模型依赖）。
func BuildReport(inputs Inputs) PatentAnalysisReport {
	features := ExtractTechnicalFeatures(inputs.Claims)
	statistics := ComputeFeatureStatistics(features)
	classifications := inputs.IPC
	if len(classifications) == 0 {
		text := strings.Join(inputs.Claims, " ") + " " + inputs.Abstract + " " + inputs.IPCText
		if classification := ipc.ClassifyDetailed(text); len(classification.MatchedKeywords) > 0 {
			classifications = []ipc.ClassificationResult{classification}
		}
	}
	ipcSummaries := make([]IpcSummary, 0, len(classifications))
	for _, classification := range classifications {
		ipcSummaries = append(ipcSummaries, ToIpcSummary(classification))
	}

	deterministic := []QualityScore{
		ComputeClarityScore(inputs.Claims),
		ComputeCompletenessScore(inputs.Claims, inputs.Abstract),
	}
	report := PatentAnalysisReport{
		PatentID:             inputs.PatentID,
		Title:                inputs.Title,
		IPC:                  ipcSummaries,
		TechnicalFeatures:    features,
		FeatureStatistics:    statistics,
		Scores:               MergeScores(deterministic, inputs.ModelScores),
		CheckerRan:           inputs.CheckerRan,
		CheckerFailures:      append([]CheckerFailure(nil), inputs.CheckerFailures...),
		GraphConclusions:     inputs.Graph,
		EvidenceReceiptCount: inputs.EvidenceReceiptCount,
		KgAnalysis:           inputs.KG,
		SearchStrategy:       inputs.SearchStrategy,
	}
	if report.CheckerFailures == nil {
		report.CheckerFailures = []CheckerFailure{}
	}
	if inputs.CheckerRan {
		report.CheckerVerdict = AggregateCheckerVerdict(inputs.CheckerFailures)
	}
	verdict := ""
	if report.CheckerRan {
		verdict = string(report.CheckerVerdict)
	}
	report.InnovationInsights = deriveInnovationInsights(ipcSummaries, report.Scores, verdict, len(features) > 0)
	report.ExpertConsiderations = deriveExpertConsiderations(inputs, report.CheckerFailures)
	return report
}

// deriveInnovationInsights 生成规则推导的创新性洞察。
func deriveInnovationInsights(ipcSummaries []IpcSummary, scores []QualityScore, verdict string, hasFeatures bool) []string {
	var insights []string
	if len(ipcSummaries) > 0 && ipcSummaries[0].Confidence >= 0.8 {
		insights = append(insights, fmt.Sprintf("请求保护主题定位于%s领域（IPC %s，置信度 %.2f）",
			ipcSummaries[0].DomainName, ipcSummaries[0].Section, ipcSummaries[0].Confidence))
	}
	for _, score := range scores {
		if score.Domain != DomainNovelty {
			continue
		}
		if score.Score >= highConfidenceScore {
			insights = append(insights, fmt.Sprintf("新颖性评估良好（%.0f 分）", score.Score))
		} else if score.Score < newKnowledgePenalty {
			insights = append(insights, fmt.Sprintf("新颖性评估偏低（%.0f 分），建议补充现有技术对比检索", score.Score))
		}
	}
	if hasFeatures {
		insights = append(insights, "已识别到分散于权利要求的可区别技术特征，可据此构建对比基础")
	}
	if verdict == string(checker.StatusBlocked) {
		insights = append(insights, "存在必须修订的质量缺陷，建议先修订再评估创新性")
	}
	return insights
}

// deriveExpertConsiderations 生成规则推导的专家考虑因素。
func deriveExpertConsiderations(inputs Inputs, failures []CheckerFailure) []string {
	var considerations []string
	for _, failure := range failures {
		if failure.Severity == checker.SeverityError {
			considerations = append(considerations, "存在 error 级检查失败，需优先核验")
			break
		}
	}
	if inputs.Graph != nil && (inputs.Graph.Novelty != "" || inputs.Graph.Inventiveness != "") {
		considerations = append(considerations, "新颖性/创造性判断需人工复核现有技术边界")
	}
	if inputs.EvidenceReceiptCount != nil && *inputs.EvidenceReceiptCount == 0 {
		considerations = append(considerations, "尚未记录证据收据，建议补充对比文件/实验证据闭环")
	}
	if inputs.KG != nil && inputs.KG.Confidence < 0.8 {
		considerations = append(considerations, fmt.Sprintf(
			"知识图谱关联置信度较低（%.2f），建议扩大检索", inputs.KG.Confidence))
	}
	if len(inputs.Claims) == 1 {
		considerations = append(considerations, "仅一条权利要求，保护范围需评估独立权利要求的概括是否过宽/过窄")
	}
	return considerations
}
