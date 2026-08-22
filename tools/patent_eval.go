package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/evaluate"
)

// PatentEvalMode defines the category of evaluation to perform.
type PatentEvalMode string

const (
	EvalModeReport        PatentEvalMode = "report"
	EvalModeRetrieval     PatentEvalMode = "retrieval"
	EvalModeWorkflow      PatentEvalMode = "workflow"
	EvalModeCitations     PatentEvalMode = "citations"
	EvalModeComprehensive PatentEvalMode = "comprehensive"
)

// SlopVerdict is the distilled result of an AI-slop analysis: the raw total
// score (out of 50) and its pass/fail verdict. Declared locally so the tools
// package does not depend on the domains/rules types.
type SlopVerdict struct {
	Total  int
	Passed bool
}

// SlopAnalyzer abstracts AI-slop detection (typically backed by
// domains/rules.AnalyzeSlop). Injected at composition time, keeping the
// dependency direction one-way: bootstrap → {tools, domains/rules}.
type SlopAnalyzer func(content string) SlopVerdict

// PatentEvalToolConfig configures the patent_eval tool.
type PatentEvalToolConfig struct {
	// SlopAnalyzer is optional; when non-nil, AI slop detection is enabled.
	SlopAnalyzer SlopAnalyzer `json:"-"`
}

// PatentEvalResult is the structured output of a patent evaluation.
type PatentEvalResult struct {
	Mode    PatentEvalMode            `json:"mode"`
	Score   float64                   `json:"score"`
	Passed  bool                      `json:"passed"`
	Details map[string]DimensionScore `json:"details"`
	Summary string                    `json:"summary"`
}

// DimensionScore captures one evaluation dimension.
type DimensionScore struct {
	Score   float64 `json:"score"`
	Passed  bool    `json:"passed"`
	Details string  `json:"details,omitempty"`
}

// NewPatentEvalTool creates a tool that evaluates patent-related content quality.
// It wraps the existing evaluate metrics and slop engine into an agent-callable tool.
func NewPatentEvalTool(cfg *PatentEvalToolConfig) *agentcore.Tool {
	if cfg == nil {
		cfg = &PatentEvalToolConfig{}
	}

	return &agentcore.Tool{
		Name:        "patent_eval",
		Description: "评估专利相关产出的质量（报告/检索/流程/引用/综合）。返回结构化评分和通过/失败判定。支持 5 种评估模式（report/retrieval/workflow/citations/comprehensive），在提交人工复核前使用可提前发现质量问题。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode": map[string]any{
					"type":        "string",
					"description": "评估模式: report(分析报告质量) / retrieval(检索覆盖度) / workflow(流程完整性) / citations(引用合规性) / comprehensive(全面评估)",
					"enum":        []string{"report", "retrieval", "workflow", "citations", "comprehensive"},
				},
				"content": map[string]any{
					"type":        "string",
					"description": "待评估的内容文本（报告正文/检索关键词列表/工作流步骤/引文列表等）",
				},
				"report_path": map[string]any{
					"type":        "string",
					"description": "报告文件路径（可选），若提供则从文件读取内容",
				},
				"required_citations": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "要求必须包含的法条引用列表（如 [\"第二十二条第二款\", \"第二十二条第三款\"]）",
				},
			},
			"required": []string{"mode"},
		},
		ReadOnly: true,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Mode              string   `json:"mode"`
				Content           string   `json:"content,omitempty"`
				ReportPath        string   `json:"report_path,omitempty"`
				RequiredCitations []string `json:"required_citations,omitempty"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return fmt.Sprintf("参数解析错误: %v", err), nil
			}

			mode := PatentEvalMode(p.Mode)
			content := p.Content

			// Build evaluator with appropriate metrics for the mode
			eval := buildEvaluatorForMode(mode)

			// For comprehensive mode, run all sub-evaluations and composite
			if mode == EvalModeComprehensive {
				return runComprehensiveEval(content, p.RequiredCitations, cfg), nil
			}

			// Run the evaluation
			result := eval.Evaluate(content, "", p.RequiredCitations)

			// Additional mode-specific analysis
			var dims map[string]DimensionScore
			var summary string

			switch mode {
			case EvalModeReport:
				dims, summary = evaluateReport(content, cfg)
			case EvalModeRetrieval:
				dims, summary = evaluateRetrieval(content)
			case EvalModeWorkflow:
				dims, summary = evaluateWorkflow(content)
			case EvalModeCitations:
				dims, summary = evaluateCitations(content, p.RequiredCitations)
			}

			overall := result.Average
			if len(dims) > 0 {
				var sum float64
				for _, d := range dims {
					sum += d.Score
				}
				overall = sum / float64(len(dims))
			}

			return PatentEvalResult{
				Mode:    mode,
				Score:   overall,
				Passed:  overall >= 0.7,
				Details: dims,
				Summary: summary,
			}, nil
		},
	}
}

// buildEvaluatorForMode returns an Evaluator with metrics appropriate to the mode.
func buildEvaluatorForMode(mode PatentEvalMode) *evaluate.Evaluator {
	switch mode {
	case EvalModeReport:
		return evaluate.NewEvaluator(
			evaluate.F1Score{},
			evaluate.KeywordRecall{},
			evaluate.CitationCompleteness{},
		)
	case EvalModeRetrieval:
		return evaluate.NewEvaluator(
			evaluate.KeywordRecall{},
		)
	case EvalModeWorkflow:
		return evaluate.NewEvaluator(
			evaluate.WorkflowQuality{},
		)
	case EvalModeCitations:
		return evaluate.NewEvaluator(
			evaluate.CitationCompleteness{},
			evaluate.CitationValidity{},
		)
	default:
		return evaluate.NewEvaluator(
			evaluate.F1Score{},
			evaluate.KeywordRecall{},
			evaluate.CitationCompleteness{},
		)
	}
}

// Section headings for patent report structure detection.
var reportSectionPatterns = []struct {
	Name    string
	Pattern *regexp.Regexp
}{
	{"技术领域", regexp.MustCompile(`(?m)^#{1,3}\s*技术领域`)},
	{"背景技术", regexp.MustCompile(`(?m)^#{1,3}\s*背景技术`)},
	{"发明内容", regexp.MustCompile(`(?m)^#{1,3}\s*发明内容`)},
	{"技术方案", regexp.MustCompile(`(?m)^#{1,3}\s*技术方案`)},
	{"有益效果", regexp.MustCompile(`(?m)^#{1,3}\s*有益效果`)},
	{"附图说明", regexp.MustCompile(`(?m)^#{1,3}\s*附图说明`)},
	{"具体实施方式", regexp.MustCompile(`(?m)^#{1,3}\s*具体实施方式`)},
	{"法律依据", regexp.MustCompile(`(?m)^#{1,3}\s*法律依据`)},
	{"分析结论", regexp.MustCompile(`(?m)^#{1,3}\s*(分析结论|结论)`)},
	{"权利要求", regexp.MustCompile(`(?m)^#{1,3}\s*权利要求`)},
}

// evaluateReport checks report structure completeness and AI slop.
func evaluateReport(content string, cfg *PatentEvalToolConfig) (map[string]DimensionScore, string) {
	dims := make(map[string]DimensionScore)

	// 1. Structure completeness: check required sections
	sectionScore := scoreSectionCoverage(content)
	dims["结构完整性"] = DimensionScore{
		Score:   sectionScore,
		Passed:  sectionScore >= 0.6,
		Details: sectionCoverageDetail(content),
	}

	// 2. AI slop detection (if slop analyzer available)
	if cfg != nil && cfg.SlopAnalyzer != nil {
		verdict := cfg.SlopAnalyzer(content)
		slopScore := float64(verdict.Total) / 50.0
		dims["表达质量"] = DimensionScore{
			Score:   slopScore,
			Passed:  verdict.Passed,
			Details: fmt.Sprintf("AI套话评分 %d/50", verdict.Total),
		}
	}

	// 3. Content sufficiency
	sufficient := scoreContentSufficiency(content)
	dims["内容充分性"] = DimensionScore{
		Score:  sufficient,
		Passed: sufficient >= 0.5,
	}

	// Calculate overall summary
	var sum float64
	for _, d := range dims {
		sum += d.Score
	}
	overall := sum / float64(len(dims))
	summary := fmt.Sprintf("报告质量评分: %.1f/1.0 (%s)", overall, passText(overall >= 0.7))
	return dims, summary
}

// scoreSectionCoverage calculates what fraction of required sections are present.
func scoreSectionCoverage(content string) float64 {
	if len(strings.TrimSpace(content)) == 0 {
		return 0
	}
	found := 0
	required := len(reportSectionPatterns)
	for _, sp := range reportSectionPatterns {
		if sp.Pattern.MatchString(content) {
			found++
		}
	}
	return float64(found) / float64(required)
}

// sectionCoverageDetail lists which sections are present and which are missing.
func sectionCoverageDetail(content string) string {
	var present, missing []string
	for _, sp := range reportSectionPatterns {
		if sp.Pattern.MatchString(content) {
			present = append(present, sp.Name)
		} else {
			missing = append(missing, sp.Name)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "已覆盖 %d/%d 个章节。", len(present), len(reportSectionPatterns))
	if len(missing) > 0 {
		fmt.Fprintf(&b, " 缺失: %s", strings.Join(missing, "、"))
	}
	return b.String()
}

// scoreContentSufficiency checks content length and paragraph count.
func scoreContentSufficiency(content string) float64 {
	text := strings.TrimSpace(content)
	if len(text) == 0 {
		return 0
	}
	// Check minimum content length (100+ chars per section)
	chars := len([]rune(text))
	if chars < 50 {
		return 0.1
	} else if chars < 200 {
		return 0.3
	} else if chars < 500 {
		return 0.5
	} else if chars < 1000 {
		return 0.7
	}
	// Check paragraph count
	paras := len(strings.Split(text, "\n\n"))
	if paras < 3 {
		return 0.6
	}
	return 1.0
}

// evaluateRetrieval checks keyword coverage and relevance.
func evaluateRetrieval(content string) (map[string]DimensionScore, string) {
	dims := make(map[string]DimensionScore)
	text := strings.TrimSpace(content)

	// Count distinct keywords/concepts
	keywords := strings.Fields(text)
	keywordScore := 0.0
	if len(keywords) >= 3 {
		keywordScore = 1.0
	} else if len(keywords) >= 1 {
		keywordScore = 0.5
	}

	dims["关键词覆盖"] = DimensionScore{
		Score:   keywordScore,
		Passed:  keywordScore >= 0.5,
		Details: fmt.Sprintf("检索式含 %d 个关键词/分类号", len(keywords)),
	}

	summary := fmt.Sprintf("检索覆盖度评分: %.1f/1.0", keywordScore)
	return dims, summary
}

// evaluateWorkflow checks workflow step completeness.
func evaluateWorkflow(content string) (map[string]DimensionScore, string) {
	dims := make(map[string]DimensionScore)
	text := strings.TrimSpace(content)

	// Count workflow steps mentioned
	stepPattern := regexp.MustCompile(`(?m)^\s*(步骤|Step|阶段|Phase)\s*\d*`)
	steps := stepPattern.FindAllString(text, -1)
	stepScore := 0.0
	if len(steps) >= 5 {
		stepScore = 1.0
	} else if len(steps) >= 3 {
		stepScore = 0.6
	} else if len(steps) >= 1 {
		stepScore = 0.3
	}

	dims["流程完整性"] = DimensionScore{
		Score:   stepScore,
		Passed:  stepScore >= 0.6,
		Details: fmt.Sprintf("检出 %d 个工作流步骤", len(steps)),
	}

	summary := fmt.Sprintf("流程完整性评分: %.1f/1.0", stepScore)
	return dims, summary
}

// evaluateCitations checks citation correctness and completeness.
func evaluateCitations(content string, required []string) (map[string]DimensionScore, string) {
	dims := make(map[string]DimensionScore)

	// Use CitationCompleteness metric
	metric := evaluate.CitationCompleteness{}
	if len(required) > 0 {
		metric.Required = required
	}
	citationScore := metric.Compute(content, "")

	dims["引用合规性"] = DimensionScore{
		Score:   citationScore,
		Passed:  citationScore >= 0.7,
		Details: fmt.Sprintf("要求 %d 条引用，覆盖度 %.0f%%", len(required), citationScore*100),
	}

	// Check citation format (Chinese patent citation patterns)
	formatPattern := regexp.MustCompile(`第[零一二三四五六七八九十百千\d]+条`)
	formatMatches := formatPattern.FindAllString(content, -1)
	formatScore := 0.0
	if len(formatMatches) > 0 {
		formatScore = 1.0
	} else {
		formatScore = 0.3
	}

	dims["引用格式"] = DimensionScore{
		Score:   formatScore,
		Passed:  formatScore >= 0.5,
		Details: fmt.Sprintf("检出 %d 处法条引用格式", len(formatMatches)),
	}

	var sum float64
	for _, d := range dims {
		sum += d.Score
	}
	overall := sum / float64(len(dims))
	summary := fmt.Sprintf("引用合规性评分: %.1f/1.0 (%s)", overall, passText(overall >= 0.7))
	return dims, summary
}

// runComprehensiveEval runs all evaluation modes and composites the results.
func runComprehensiveEval(content string, citations []string, cfg *PatentEvalToolConfig) PatentEvalResult {
	reportDims, _ := evaluateReport(content, cfg)
	retrievalDims, _ := evaluateRetrieval(content)
	workflowDims, _ := evaluateWorkflow(content)
	citationDims, _ := evaluateCitations(content, citations)

	allDims := make(map[string]DimensionScore)
	for k, v := range reportDims {
		allDims[k] = v
	}
	for k, v := range retrievalDims {
		allDims[k] = v
	}
	for k, v := range workflowDims {
		allDims[k] = v
	}
	for k, v := range citationDims {
		allDims[k] = v
	}

	// Weighted composite: report 40%, citations 25%, retrieval 20%, workflow 15%
	var totalWeight float64
	var weightedSum float64
	weights := map[string]float64{
		"结构完整性": 0.15,
		"表达质量":  0.10,
		"内容充分性": 0.15,
		"关键词覆盖": 0.20,
		"流程完整性": 0.15,
		"引用合规性": 0.15,
		"引用格式":  0.10,
	}

	for k, d := range allDims {
		w := weights[k]
		if w == 0 {
			w = 0.1
		}
		weightedSum += d.Score * w
		totalWeight += w
	}

	composite := weightedSum / totalWeight
	var summaryParts []string
	summaryParts = append(summaryParts, fmt.Sprintf("综合质量评分: %.1f/1.0", composite))
	for k, d := range allDims {
		status := "✅" //nolint:gosec
		if !d.Passed {
			status = "❌" //nolint:gosec
		}
		summaryParts = append(summaryParts, fmt.Sprintf("  %s %s: %.1f", status, k, d.Score))
	}

	return PatentEvalResult{
		Mode:    EvalModeComprehensive,
		Score:   composite,
		Passed:  composite >= 0.7,
		Details: allDims,
		Summary: strings.Join(summaryParts, "\n"),
	}
}

func passText(passed bool) string {
	if passed {
		return "通过"
	}
	return "需修订"
}
