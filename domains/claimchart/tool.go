// Package claimchart provides the claim_chart_build tool for patent claim
// element mapping against prior-art or accused-product evidence.
//
// This is a Mady-native implementation aligned with Sati's ClaimChart schema
// but without the Sati atom/LLM runtime. It performs deterministic claim
// element splitting and simple evidence matching; the resulting chart is
// intended as a structured draft for attorney/agent review.
package claimchart

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/provenance"
)

// provenanceLog 是包级溯源日志器；nil 时静默（Log 自身 nil-safe）。
var provenanceLog *provenance.ProvenanceLogger

// SetProvenance 注入溯源日志器（bootstrap 装配）；传 nil 时溯源静默关闭。
func SetProvenance(l *provenance.ProvenanceLogger) { provenanceLog = l }

// 解析与匹配用的编译缓存正则（避免在元素×目标循环里每次调用重新编译）。
var (
	claimStartRe    = regexp.MustCompile(`^(?:权利要求?\s*)?(\d+)[\.、,，:：]\s*(.+)$`)
	transitionRe    = regexp.MustCompile(`(其特征在于|特征在于|其中| wherein)`)
	featureSplitRe  = regexp.MustCompile(`[，,；;]`)
	claimKeywordRe  = regexp.MustCompile(`[^\p{Han}a-zA-Z]+`)
	claimSentenceRe = regexp.MustCompile(`[。！？\n;；]`)
)

// ElementKind identifies the type of a parsed claim element.
type ElementKind = string

// ElementKind classifies parsed claim elements.
const (
	ElementPreamble          ElementKind = "preamble"
	ElementTransitional      ElementKind = "transitional"
	ElementLimitation        ElementKind = "limitation"
	ElementMeansPlusFunction ElementKind = "means-plus-function"
	ElementMarkushMember     ElementKind = "markush-member"
)

// Mapping is the mapping classification for a chart row.
type Mapping = string

// Mapping classifies the mapping state of a chart row against a target.
const (
	MappingLiteral                      Mapping = "literal"
	MappingLiteralConstructionDependent Mapping = "literal-construction-dependent"
	MappingDOE                          Mapping = "doe"
	MappingAnticipation                 Mapping = "anticipation"
	MappingObviousnessCombination       Mapping = "obviousness-combination"
	MappingPartial                      Mapping = "partial"
	MappingNotFound                     Mapping = "not-found"
	MappingNeedsEvidence                Mapping = "needs-evidence"
	MappingConstructionDependent        Mapping = "construction-dependent"
)

// ChartMode matches Sati's claim chart modes.
type ChartMode = string

// ChartMode selects the analysis scenario for a chart.
const (
	ModeInfringement  ChartMode = "infringement"
	ModeInvalidity    ChartMode = "invalidity"
	ModeOAResponse    ChartMode = "oa-response"
	ModeReexamination ChartMode = "reexamination"
	ModePatentability ChartMode = "patentability"
)

// ClaimElement is a parsed element of a patent claim.
type ClaimElement struct {
	ID           string      `json:"id"`
	ClaimNo      int         `json:"claimNo"`
	Text         string      `json:"text"`
	Kind         ElementKind `json:"kind"`
	DisputedTerm string      `json:"disputedTerm,omitempty"`
}

// TargetKind identifies the kind of chart target.
type TargetKind = string

// TargetKind identifies a chart target source.
const (
	TargetPriorArt       TargetKind = "prior-art"
	TargetAccusedProduct TargetKind = "accused-product"
)

// ChartTarget is a mapping target (prior art or accused product).
type ChartTarget struct {
	ID         string     `json:"id"`
	Kind       TargetKind `json:"kind"`
	Title      string     `json:"title,omitempty"`
	SourcePath string     `json:"sourcePath,omitempty"`
}

// ChartRow maps one element to one target.
type ChartRow struct {
	ElementID string  `json:"elementId"`
	TargetID  string  `json:"targetId"`
	Quote     string  `json:"quote"`
	PinCite   string  `json:"pinCite"`
	Mapping   Mapping `json:"mapping"`
	Verified  bool    `json:"verified"`
	Note      string  `json:"note,omitempty"`
}

// GapEntry represents a missing or weak evidence mapping.
type GapEntry struct {
	ElementID  string  `json:"elementId"`
	TargetID   string  `json:"targetId"`
	Mapping    Mapping `json:"mapping"`
	Reason     string  `json:"reason"`
	Suggestion string  `json:"suggestion"`
}

// ClaimChart is the structured output schema aligned with Sati.
type ClaimChart struct {
	ChartID     string         `json:"chartId"`
	Mode        ChartMode      `json:"mode"`
	CaseID      string         `json:"caseId"`
	Elements    []ClaimElement `json:"elements"`
	ClaimNos    []int          `json:"claimNos"`
	Targets     []ChartTarget  `json:"targets"`
	Rows        []ChartRow     `json:"rows"`
	Gaps        []GapEntry     `json:"gaps"`
	DraftNotice string         `json:"draftNotice"`
}

// ChartTargetInput is the tool input shape for a target.
type ChartTargetInput struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title,omitempty"`
	SourcePath string `json:"source_path,omitempty"`
}

// ChartInput is the tool input shape.
type ChartInput struct {
	Mode      ChartMode          `json:"mode"`
	ClaimText string             `json:"claim_text"`
	Targets   []ChartTargetInput `json:"targets"`
	CaseID    string             `json:"case_id,omitempty"`
}

// DraftNotice is appended to every generated chart.
const DraftNotice = "本表为分析草稿，供代理人与律师核验使用，不构成正式法律意见或诉讼主张。每一行映射均须对照源文件人工复核。"

var validModes = map[string]bool{
	ModeInfringement:  true,
	ModeInvalidity:    true,
	ModeOAResponse:    true,
	ModeReexamination: true,
	ModePatentability: true,
}

// NewClaimChartTool creates the claim_chart_build tool.
func NewClaimChartTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "claim_chart_build",
		Description: "构建权利要求对照表（claim chart）：把权利要求拆分为编号要素，逐要素映射到对比文件或产品证据，并输出 gap list（证据薄弱的要素）。适用于撰写（可专利性布局）、OA 答复、无效/复审、侵权比对等场景。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode": map[string]any{
					"type":        "string",
					"enum":        []string{"infringement", "invalidity", "oa-response", "reexamination", "patentability"},
					"description": "场景模式：infringement=侵权/invalidity=无效/oa-response=审查意见答复/reexamination=复审/patentability=撰写前可专利性",
				},
				"claim_text": map[string]any{"type": "string", "description": "权利要求原文（需拆分的权利要求，可含多条）"},
				"targets": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string", "description": "目标标识，如 D1/D2/产品A"},
							"kind":        map[string]any{"type": "string", "enum": []string{"prior-art", "accused-product"}},
							"title":       map[string]any{"type": "string", "description": "目标名称（可选）"},
							"source_path": map[string]any{"type": "string", "description": "目标全文文件路径（提供时启用 pin-cite 与引用存在性校验）"},
						},
						"required": []string{"id", "kind"},
					},
				},
				"case_id": map[string]any{"type": "string", "description": "案卷 ID（提供时结果落盘 data/cases/<case_id>/outputs/）"},
			},
			"required": []string{"mode", "claim_text", "targets"},
		},
		ReadOnly: true,
		Func:     handleClaimChart,
	}
}

func handleClaimChart(_ context.Context, args json.RawMessage) (any, error) {
	var input ChartInput
	if err := json.Unmarshal(args, &input); err != nil {
		// 参数损坏按工具失败结果返回给 LLM 重试，不向上传播中断会话。
		return agentcore.NewFailureResult("参数解析失败", "权利要求对照表参数格式错误"), nil
	}
	if input.ClaimText == "" {
		return agentcore.NewFailureResult("输入为空", "claim_text 不能为空"), nil
	}
	if len(input.Targets) == 0 {
		return agentcore.NewFailureResult("目标为空", "至少需要提供一个 target"), nil
	}
	if !validModes[input.Mode] {
		input.Mode = ModePatentability
	}

	chart, err := BuildClaimChart(input)
	if err != nil {
		return agentcore.NewFailureResult("构建失败", err.Error()), nil
	}

	output := map[string]any{
		"ok":        true,
		"chart":     chart,
		"gap_count": len(chart.Gaps),
		"claim_nos": chart.ClaimNos,
		"mode":      chart.Mode,
	}
	if input.CaseID != "" {
		output["case_id"] = input.CaseID
	}

	// 溯源是 fail-open 旁路：写失败不影响建表结果（Log 对 nil 接收者静默）。
	_ = provenanceLog.Log(provenance.ProvenanceEvent{
		Kind:    provenance.KindWorkflowStep,
		Tool:    "claim_chart_build",
		CaseID:  input.CaseID,
		Details: fmt.Sprintf("建表：%d 要素 / %d 行 / %d 缺口", len(chart.Elements), len(chart.Rows), len(chart.Gaps)),
	})

	data, err := json.Marshal(output)
	if err != nil {
		return agentcore.NewFailureResult("序列化失败", err.Error()), nil
	}
	return string(data), nil
}

// BuildClaimChart parses claims and builds a structured claim chart.
func BuildClaimChart(input ChartInput) (*ClaimChart, error) {
	elements := parseClaimElements(input.ClaimText)
	targets := make([]ChartTarget, len(input.Targets))
	targetTexts := make(map[string]string)
	for i, t := range input.Targets {
		kind := TargetPriorArt
		if t.Kind == TargetAccusedProduct {
			kind = TargetAccusedProduct
		}
		targets[i] = ChartTarget{
			ID:         t.ID,
			Kind:       kind,
			Title:      t.Title,
			SourcePath: t.SourcePath,
		}
		if t.SourcePath != "" {
			data, err := os.ReadFile(t.SourcePath) //nolint:gosec // path provided by caller
			// 读取失败按"无源文"处理：该目标各行降级为 needs-evidence，不中断建表。
			if err == nil {
				targetTexts[t.ID] = string(data)
			}
		}
	}

	claimNoSet := make(map[int]bool)
	for _, e := range elements {
		claimNoSet[e.ClaimNo] = true
	}
	claimNos := make([]int, 0, len(claimNoSet))
	for n := range claimNoSet {
		claimNos = append(claimNos, n)
	}
	sort.Ints(claimNos)

	rows := []ChartRow{}
	gaps := []GapEntry{}
	for _, e := range elements {
		for _, t := range targets {
			row := matchElementToTarget(e, t, targetTexts[t.ID])
			rows = append(rows, row)
			if row.Mapping == MappingNeedsEvidence || row.Mapping == MappingNotFound {
				gaps = append(gaps, GapEntry{
					ElementID:  e.ID,
					TargetID:   t.ID,
					Mapping:    row.Mapping,
					Reason:     row.Note,
					Suggestion: suggestAction(input.Mode, t.Kind),
				})
			}
		}
	}

	chartID := chartID(input)
	return &ClaimChart{
		ChartID:     chartID,
		Mode:        input.Mode,
		CaseID:      input.CaseID,
		Elements:    elements,
		ClaimNos:    claimNos,
		Targets:     targets,
		Rows:        rows,
		Gaps:        gaps,
		DraftNotice: DraftNotice,
	}, nil
}

func chartID(input ChartInput) string {
	sum := sha256.Sum256([]byte(input.Mode + "|" + input.ClaimText[:min(len(input.ClaimText), 200)]))
	return "cc-" + hex.EncodeToString(sum[:])[:12]
}

// parseClaimElements extracts numbered claim elements from claim text.
// It handles common Chinese claim conventions:
//   - "1. 一种..." independent claims
//   - "其特征在于" transitions
//   - comma/semicolon separated limitations
func parseClaimElements(text string) []ClaimElement {
	var elements []ClaimElement
	lines := strings.Split(text, "\n")

	var currentClaimNo int
	var elementIdx int
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		if m := claimStartRe.FindStringSubmatch(line); m != nil {
			currentClaimNo = parseInt(m[1])
			elementIdx = 0
			// Split preamble and features by transition phrase.
			parts := transitionRe.Split(m[2], 2)
			preamble := strings.TrimSpace(parts[0])
			if preamble != "" {
				elements = append(elements, ClaimElement{
					ID:      fmt.Sprintf("%d%s", currentClaimNo, elementLabel(elementIdx)),
					ClaimNo: currentClaimNo,
					Text:    preamble,
					Kind:    ElementPreamble,
				})
				elementIdx++
			}
			if len(parts) > 1 {
				var n int
				elements, n = appendFeatureElements(elements, currentClaimNo, parts[1], elementIdx, true)
				elementIdx = n
			}
			continue
		}

		// Continuation lines for the current claim.
		if currentClaimNo > 0 {
			parts := transitionRe.Split(line, 2)
			if len(parts) > 1 {
				var n int
				elements, n = appendFeatureElements(elements, currentClaimNo, parts[1], elementIdx, false)
				elementIdx = n
			}
		}
	}

	return elements
}

// appendFeatureElements splits a feature body on [,；;] and appends a ClaimElement
// for each non-empty fragment. When meansPlus is true it classifies fragments
// containing 装置/单元/模块 as means-plus-function.
func appendFeatureElements(elements []ClaimElement, claimNo int, featureText string, elementIdx int, meansPlus bool) ([]ClaimElement, int) {
	for _, feat := range featureSplitRe.Split(strings.TrimSpace(featureText), -1) {
		feat = strings.TrimSpace(feat)
		if feat == "" {
			continue
		}
		kind := ElementLimitation
		if meansPlus && (strings.Contains(feat, "装置") || strings.Contains(feat, "单元") || strings.Contains(feat, "模块")) {
			kind = ElementMeansPlusFunction
		}
		elements = append(elements, ClaimElement{
			ID:      fmt.Sprintf("%d%s", claimNo, elementLabel(elementIdx)),
			ClaimNo: claimNo,
			Text:    feat,
			Kind:    kind,
		})
		elementIdx++
	}
	return elements, elementIdx
}

func elementLabel(idx int) string {
	if idx < 26 {
		return string(rune('a' + idx))
	}
	return fmt.Sprintf("x%d", idx)
}

func parseInt(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}

// matchElementToTarget performs simple keyword overlap matching between an
// element and a target text. It is intentionally lightweight; high-stakes
// mapping still requires human review.
func matchElementToTarget(e ClaimElement, t ChartTarget, targetText string) ChartRow {
	row := ChartRow{
		ElementID: e.ID,
		TargetID:  t.ID,
		Mapping:   MappingNeedsEvidence,
		Verified:  false,
		Note:      "未提供源文或尚未完成证据定位",
	}
	if targetText == "" {
		return row
	}

	elementWords := extractKeywords(e.Text)
	if len(elementWords) == 0 {
		return row
	}

	// Find the best matching sentence/phrase in target text.
	best, score := findBestMatch(e.Text, elementWords, targetText)
	if best != "" {
		row.Quote = best
		row.PinCite = fmt.Sprintf("[%s 命中片段]", t.ID)
		row.Note = fmt.Sprintf("关键词重叠度 %.0f%%", score*100)
		if score >= 0.7 {
			if t.Kind == TargetPriorArt {
				row.Mapping = MappingAnticipation
			} else {
				row.Mapping = MappingLiteral
			}
		} else if score >= 0.4 {
			row.Mapping = MappingPartial
		} else {
			row.Mapping = MappingNotFound
		}
	}
	return row
}

func extractKeywords(text string) []string {
	// Strip punctuation and numbers, keep CJK chars and ASCII letters.
	parts := claimKeywordRe.Split(text, -1)
	var words []string
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Ignore very short fragments and common stop words.
		if len([]rune(p)) < 2 {
			continue
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		words = append(words, p)
	}
	return words
}

func findBestMatch(elementText string, keywords []string, targetText string) (string, float64) {
	sentences := splitSentences(targetText)
	if len(sentences) == 0 {
		return "", 0
	}

	best := ""
	bestScore := 0.0
	for _, sent := range sentences {
		sent = strings.TrimSpace(sent)
		if len([]rune(sent)) < 5 {
			continue
		}
		score := overlapScore(keywords, sent)
		if score > bestScore {
			bestScore = score
			best = sent
		}
	}

	// If no good sentence match, try sliding windows.
	if bestScore < 0.3 {
		window := len([]rune(elementText))
		if window < 20 {
			window = 20
		}
		best, bestScore = findBestWindow(keywords, targetText, window)
	}

	if bestScore <= 0 {
		return "", 0
	}
	return truncateSentence(best, 300), bestScore
}

func splitSentences(text string) []string {
	return claimSentenceRe.Split(text, -1)
}

func overlapScore(keywords []string, text string) float64 {
	if len(keywords) == 0 {
		return 0
	}
	textLower := strings.ToLower(text)
	matched := 0
	for _, kw := range keywords {
		if strings.Contains(textLower, strings.ToLower(kw)) {
			matched++
		}
	}
	return float64(matched) / float64(len(keywords))
}

func findBestWindow(keywords []string, text string, window int) (string, float64) {
	runes := []rune(text)
	if len(runes) <= window {
		return text, overlapScore(keywords, text)
	}

	best := ""
	bestScore := 0.0
	for i := 0; i+window < len(runes); i += window / 2 {
		windowText := string(runes[i : i+window])
		score := overlapScore(keywords, windowText)
		if score > bestScore {
			bestScore = score
			best = windowText
		}
	}
	return best, bestScore
}

func truncateSentence(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	return string(r[:maxChars]) + "…"
}

func suggestAction(mode ChartMode, kind TargetKind) string {
	switch mode {
	case ModeInfringement:
		return "补充侵权证据或进行等同分析"
	case ModeInvalidity:
		return "补充检索或确认新颖性/创造性论证"
	case ModeOAResponse, ModeReexamination:
		return "补充对比文件定位或修改权利要求"
	default:
		if kind == TargetPriorArt {
			return "补充现有技术检索"
		}
		return "补充证据材料"
	}
}
