// numeric_range.go 实现新颖性分析的确定性数值范围核验节点。
//
// 与 step_single_compare 的 LLM 语义轨并行的确定性判定轨（DSH numeric_range
// 节点引入）：用正则从权利要求与对比文件中提取数值范围/数值点，按审查指南
// 第二部分第三章 3.2.5 的典型情形做区间重叠判定，并与语义轨的
// CompareResult.NumericRangeResult 交叉对照。判定是保守的——只把"破坏性
// 重叠"记为 overlapped；数值点落在对比文件范围内但无共同端点（情形五，
// 不破坏新颖性）单独标记，供结论节点区分处理。

package novelty

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/xujian519/mady/graph"
)

// 数值范围判定结论。
const (
	// NumericOverlapped 存在破坏性重叠（情形一/二/三/八）。
	NumericOverlapped = "overlapped"
	// NumericInsideWithoutEndpoint 权利要求数值点落在对比文件范围内且无共同
	// 端点（情形五，不破坏新颖性，但值得提示）。
	NumericInsideWithoutEndpoint = "inside_without_endpoint"
	// NumericNoOverlap 未发现重叠。
	NumericNoOverlap = "no_overlap"
	// NumericInconclusive 任一侧未提取到带单位的强数值表述，无法判定。
	NumericInconclusive = "inconclusive"
)

// 与 LLM 语义轨的对照结论。
const (
	LLMAgree    = "agree"
	LLMDisagree = "disagree"
	LLMNA       = "n_a"
)

// rangeRe 匹配闭区间式数值范围：5-10、5～10、5~10、5至10。
var rangeRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:-|～|~|至)\s*(\d+(?:\.\d+)?)`)

// boundedRe 匹配单边数值表述（允许方向词后跟"为/是/约"）。注意 alternation
// 顺序：长词在前，否则"大于等于"会被"大于"截断。
var boundedRe = regexp.MustCompile(`(大于等于|小于等于|不低于|不大于|不超过|不小于|不少于|不多于|大于|小于|超过|低于|高于|至少|多于|≥|≤)\s*[为是约]?\s*(\d+(?:\.\d+)?)`)

// unitRe 匹配数值后的单位（字母/符号单位 1-8 位，或 1-2 个汉字单位）。
var unitRe = regexp.MustCompile(`^\s*([A-Za-zμ%％℃°]{1,8}|[\p{Han}]{1,2})`)

// NumericRangeFinding 是一个提取出的数值表述。
type NumericRangeFinding struct {
	Expression string  `json:"expression"` // 原文片段
	Lower      float64 `json:"lower"`      // 闭区间下界（单边时为 ±Inf）
	Upper      float64 `json:"upper"`      // 闭区间上界（单边时为 ±Inf）
	IsPoint    bool    `json:"is_point"`   // 单个数值（非范围）
	Unit       string  `json:"unit"`       // 归一化单位；空表示无单位（弱发现）
	Strong     bool    `json:"strong"`     // 带单位/百分比的强发现（参与判定）
	ClaimID    string  `json:"claim_id,omitempty"`
	DocID      string  `json:"doc_id,omitempty"`
}

// NumericRangeOverlap 是一对数值表述的重叠关系。
type NumericRangeOverlap struct {
	Claim NumericRangeFinding `json:"claim"`
	Prior NumericRangeFinding `json:"prior"`
	Kind  string              `json:"kind"` // destructive / inside_without_endpoint
	Note  string              `json:"note"`
}

// NumericRangeAnalysis 是确定性数值范围核验的完整结果。
type NumericRangeAnalysis struct {
	ClaimRanges  []NumericRangeFinding `json:"claim_ranges"`
	PriorRanges  []NumericRangeFinding `json:"prior_ranges"`
	Overlaps     []NumericRangeOverlap `json:"overlaps,omitempty"`
	Verdict      string                `json:"verdict"`
	LLMAgreement string                `json:"llm_agreement"` // agree / disagree / n_a
	Summary      string                `json:"summary"`
}

// AnalyzeNumericRanges 对权利要求与对比文件做确定性数值范围核验。
func AnalyzeNumericRanges(input *NoveltyInput) *NumericRangeAnalysis {
	a := &NumericRangeAnalysis{LLMAgreement: LLMNA}
	if input == nil {
		a.Verdict = NumericInconclusive
		a.Summary = "无输入，跳过数值范围核验"
		return a
	}

	for _, c := range input.Claims {
		a.ClaimRanges = append(a.ClaimRanges, extractFindings(c.Text, c.ID, "")...)
	}
	for _, d := range input.PriorArtDocs {
		a.PriorRanges = append(a.PriorRanges, extractFindings(d.Snippet, "", d.DocID)...)
	}

	a.Overlaps = findOverlaps(a.ClaimRanges, a.PriorRanges)
	a.Verdict = judgeVerdict(a.Overlaps, a.ClaimRanges, a.PriorRanges)
	a.Summary = buildSummary(a)
	return a
}

// CrossCheckLLM 与语义轨（CompareResult.NumericRangeResult）对照。
// compareJSON 为空或解析失败时保持 n_a——无法对照不等于对照失败。
// 对照后重建 Summary，使"与语义轨不一致"提示反映最新对照结论。
func (a *NumericRangeAnalysis) CrossCheckLLM(compareJSON string) {
	if compareJSON == "" || a.Verdict == NumericInconclusive {
		return
	}
	var cmp CompareResult
	if err := json.Unmarshal([]byte(compareJSON), &cmp); err != nil {
		return
	}
	switch {
	case a.Verdict == NumericOverlapped && cmp.NumericRangeResult == NumericNoOverlap:
		a.LLMAgreement = LLMDisagree
	case a.Verdict == NumericOverlapped && cmp.NumericRangeResult == NumericOverlapped,
		a.Verdict == NumericInsideWithoutEndpoint && cmp.NumericRangeResult == NumericInsideWithoutEndpoint,
		a.Verdict == NumericNoOverlap && cmp.NumericRangeResult == NumericNoOverlap:
		a.LLMAgreement = LLMAgree
	default:
		a.LLMAgreement = LLMNA
	}
	a.Summary = buildSummary(a)
}

// extractFindings 从一段文本提取数值表述。带单位的为强发现，无单位的
// 仅记录不参与判定（弱发现，降低 "权利要求1-3" 这类编号的误报影响）。
func extractFindings(text, claimID, docID string) []NumericRangeFinding {
	var findings []NumericRangeFinding
	add := func(lower, upper float64, expr string, isPoint bool) {
		f := NumericRangeFinding{
			Expression: expr,
			Lower:      lower,
			Upper:      upper,
			IsPoint:    isPoint,
			Unit:       extractUnit(text, expr),
			ClaimID:    claimID,
			DocID:      docID,
		}
		f.Strong = f.Unit != ""
		findings = append(findings, f)
	}

	for _, m := range rangeRe.FindAllStringSubmatch(text, -1) {
		lo, err1 := strconv.ParseFloat(m[1], 64)
		hi, err2 := strconv.ParseFloat(m[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if lo > hi {
			lo, hi = hi, lo
		}
		add(lo, hi, m[0], false)
	}
	for _, m := range boundedRe.FindAllStringSubmatch(text, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		lo, hi := bound(m[1], v)
		add(lo, hi, m[0], false)
	}
	// 单独数值点：仅当同一文本中没有范围表述时提取，避免重复。
	if len(findings) == 0 {
		for _, m := range plainNumberRe.FindAllString(text, -1) {
			v, err := strconv.ParseFloat(m, 64)
			if err != nil {
				continue
			}
			add(v, v, m, true)
		}
	}
	return findings
}

// plainNumberRe 匹配独立数值（弱发现兜底）。
var plainNumberRe = regexp.MustCompile(`\d+(?:\.\d+)?`)

// bound 把单边表述转为区间；方向词全按"包含端点"处理——保守方向，
// 宁可多提示不漏报。
func bound(direction string, v float64) (float64, float64) {
	switch direction {
	case "大于等于", "不低于", "不小于", "不少于", "至少", "≥":
		return v, math.Inf(1)
	case "小于等于", "不大于", "不超过", "不多于", "≤":
		return math.Inf(-1), v
	default: // 大于/超过/高于/多于/小于/低于
		return boundInclusive(direction, v)
	}
}

// boundInclusive 处理开方向词，仍按闭区间近似（保守）。
func boundInclusive(direction string, v float64) (float64, float64) {
	switch direction {
	case "小于", "低于":
		return math.Inf(-1), v
	default: // 大于/超过/高于/多于
		return v, math.Inf(1)
	}
}

// extractUnit 提取 expr 之后紧跟的单位并归一化（去空白、转小写）。
// 无单位返回空串。
func extractUnit(text, expr string) string {
	idx := strings.Index(text, expr)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(expr):]
	m := unitRe.FindStringSubmatch(rest)
	if len(m) < 2 {
		return ""
	}
	unit := strings.ToLower(strings.TrimSpace(m[1]))
	// 纯汉字单位里排除常见非单位字（"的""时"等误配已由 1-2 字限制缓解，
	// 这里再排除明显不是单位的字，宁可漏掉单位也不把编号当数值）。
	switch unit {
	case "的", "个", "中", "为", "是", "内", "外":
		return ""
	}
	return unit
}

// findOverlaps 计算破坏性/提示性重叠对。仅强发现参与；单位不同不可比。
func findOverlaps(claims, priors []NumericRangeFinding) []NumericRangeOverlap {
	var overlaps []NumericRangeOverlap
	for _, c := range claims {
		if !c.Strong {
			continue
		}
		for _, p := range priors {
			if !p.Strong || c.Unit != p.Unit {
				continue
			}
			if !intervalsOverlap(c, p) {
				continue
			}
			ov := NumericRangeOverlap{Claim: c, Prior: p}
			if c.IsPoint && c.Lower > p.Lower && c.Upper < p.Upper {
				// 情形五：数值点在对比文件范围内但非端点 → 不破坏新颖性。
				ov.Kind = NumericInsideWithoutEndpoint
				ov.Note = "情形五：权利要求数值点落在对比文件范围内且无共同端点，不破坏新颖性"
			} else {
				ov.Kind = NumericOverlapped
				ov.Note = "情形一/二：数值（范围）重叠或有共同端点，破坏新颖性"
			}
			overlaps = append(overlaps, ov)
		}
	}
	return overlaps
}

// intervalsOverlap 闭区间重叠判定（±Inf 视为闭端点）。
func intervalsOverlap(a, b NumericRangeFinding) bool {
	return a.Lower <= b.Upper && b.Lower <= a.Upper
}

func judgeVerdict(overlaps []NumericRangeOverlap, claims, priors []NumericRangeFinding) string {
	hasStrong := func(fs []NumericRangeFinding) bool {
		for _, f := range fs {
			if f.Strong {
				return true
			}
		}
		return false
	}
	if !hasStrong(claims) || !hasStrong(priors) {
		return NumericInconclusive
	}
	destructive := false
	for _, ov := range overlaps {
		if ov.Kind == NumericOverlapped {
			destructive = true
		}
	}
	switch {
	case destructive:
		return NumericOverlapped
	case len(overlaps) > 0:
		return NumericInsideWithoutEndpoint
	default:
		return NumericNoOverlap
	}
}

func buildSummary(a *NumericRangeAnalysis) string {
	if a.Verdict == NumericInconclusive {
		return "确定性数值范围核验：未提取到足够的带单位数值表述，无法判定"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "确定性数值范围核验：权利要求 %d 处数值表述（强 %d），对比文件 %d 处（强 %d）；",
		len(a.ClaimRanges), countStrong(a.ClaimRanges), len(a.PriorRanges), countStrong(a.PriorRanges))
	switch a.Verdict {
	case NumericOverlapped:
		b.WriteString("发现破坏性数值重叠，建议人工复核数值特征")
	case NumericInsideWithoutEndpoint:
		b.WriteString("数值点落入对比文件范围但无共同端点（不破坏新颖性）")
	default:
		b.WriteString("未发现数值范围重叠")
	}
	for _, ov := range a.Overlaps {
		fmt.Fprintf(&b, "；[%s × %s] %s ↔ %s（%s）",
			ov.Claim.ClaimID, ov.Prior.DocID, ov.Claim.Expression, ov.Prior.Expression, ov.Kind)
	}
	if a.LLMAgreement == LLMDisagree {
		b.WriteString("；与语义轨结论不一致，请重点复核")
	}
	return b.String()
}

func countStrong(fs []NumericRangeFinding) int {
	n := 0
	for _, f := range fs {
		if f.Strong {
			n++
		}
	}
	return n
}

// numericRangeNode 确定性数值范围核验节点（纯函数，不调 LLM）。
// 读取 NoveltyInput 与语义轨 stateKeyCompare 的产出，写 StateKeyNumericRange。
func numericRangeNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}
		input := extractInput(state)
		if input == nil {
			return state, nil
		}

		analysis := AnalyzeNumericRanges(input)
		analysis.CrossCheckLLM(getStateString(state, stateKeyCompare))
		state[StateKeyNumericRange] = analysis
		return state, nil
	}
}
