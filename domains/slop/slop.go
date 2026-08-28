// Package slop 实现专利文书草稿的反套话确定性评分闸门（slop gate）。
//
// 灵感来自 DSH 专利工作台的 slop-gate 原子：对起草产物做纯规则扫描，
// 命中套话/空话/无支撑断言时给出"需修订"判定与逐条修订建议，供工作流
// 回退重写或人工复核。与 LLM 语义评审并行的确定性判定轨——误报优先控制，
// 含否定词或带引证的句子不命中。
//
// 检查与 LLM 判断的分工：本包管"专业文书写作质量"的确定性拦截
// （可解释、可复现、零 token），语义层面的问题（论述是否成立）交给评审模型。
package slop

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/xujian519/mady/domains/analysiskit"
	"github.com/xujian519/mady/pkg/util"
)

// Verdict 判定结果。
const (
	// VerdictPass 表示套话密度可接受。
	VerdictPass = "pass"
	// VerdictNeedsRevision 表示套话/断言命中过多，建议回退修订。
	VerdictNeedsRevision = "needs_revision"
)

// 命中类别。
const (
	// CatUnsupportedEffect 无数据支撑的效果断言。
	CatUnsupportedEffect = "unsupported-effect"
	// CatConclusoryRemark 结论式评述（无引证的"显而易见"类断言）。
	CatConclusoryRemark = "conclusory-remark"
	// CatBoilerplateFiller 空话模板（广泛应用前景/不再赘述类）。
	CatBoilerplateFiller = "boilerplate-filler"
	// CatRepeatedBoilerplate 同一套话重复出现。
	CatRepeatedBoilerplate = "repeated-boilerplate"
)

// 各类别的加权分与修订建议。
var categoryWeights = map[string]int{
	CatUnsupportedEffect:   2,
	CatConclusoryRemark:    2,
	CatBoilerplateFiller:   1,
	CatRepeatedBoilerplate: 1,
}

var categorySuggestions = map[string]string{
	CatUnsupportedEffect:   "补充定量实验数据或对比数据支撑效果断言，删除无法佐证的形容词",
	CatConclusoryRemark:    "补充推理链或引证具体证据（对比文件/公知常识来源），避免光结论不论证",
	CatBoilerplateFiller:   "删除空话模板句，替换为具体技术内容",
	CatRepeatedBoilerplate: "同一表述重复出现，合并或改写",
}

// patternSpec 是一条套话模式。
type patternSpec struct {
	name     string
	category string
	re       *regexp.Regexp
	// guard 命中时放弃本条（否定/引证防线）。
	guard func(sentence string) bool
}

// negationRe 否定防线：句中含否定表述时放弃命中（"效果并不显著"不是断言）。
var negationRe = regexp.MustCompile(`并不|不具有|不具有益|没有产生|无法|并非|未出现|不能`)

// citationRe 引证防线：句中带对比文件引证时，结论式评述可接受。
var citationRe = regexp.MustCompile(`对比文件|证据[一二三四五六七八九十\d]|D\d+|公开号|参见`)

// digitsRe 数据支撑防线：句中含数字时效果断言视为有数据支撑。
var digitsRe = regexp.MustCompile(`[0-9０-９％%℃]`)

var patterns = []patternSpec{
	{
		name:     "unsupported-effect",
		category: CatUnsupportedEffect,
		re:       regexp.MustCompile(`显著的?(提高|提升|改善|增强|降低|减少)|大大(提高|降低|减少|简化)|意料不到的技术效果|显著的有益效果|效果(十分|非常|极其)?显著|优异的?[A-Za-z\p{Han}]{0,6}效果|极佳`),
		guard: func(s string) bool {
			return negationRe.MatchString(s) || digitsRe.MatchString(s)
		},
	},
	{
		name:     "conclusory-remark",
		category: CatConclusoryRemark,
		re:       regexp.MustCompile(`本领域技术人员(容易|很容易)?想到|显而易见地?|不言而喻|无疑是|显然是`),
		guard: func(s string) bool {
			return negationRe.MatchString(s) || citationRe.MatchString(s)
		},
	},
	{
		name:     "boilerplate-filler",
		category: CatBoilerplateFiller,
		re:       regexp.MustCompile(`广泛(的)?应用(的)?前景|在此不再(一一)?赘述|在此不(作|做)(任何)?限定|根据实际需要(进行)?(选择|调整|设置)|本领域常规(手段|做法|选择)`),
		guard:    nil,
	},
}

// Finding 是一条套话命中。
type Finding struct {
	Category   string `json:"category"`
	Pattern    string `json:"pattern"`
	Sentence   string `json:"sentence"`
	Reason     string `json:"reason"`
	Suggestion string `json:"suggestion"`
}

// Report 是一次套话扫描的完整结果。
type Report struct {
	Verdict    string    `json:"verdict"`
	Score      float64   `json:"score"` // 每千字加权命中数
	Findings   []Finding `json:"findings"`
	TotalRunes int       `json:"total_runes"`
	Summary    string    `json:"summary"`
}

// Check 对 text 做确定性套话扫描。text 为空时返回 pass 的空报告。
func Check(text string) Report {
	runes := len([]rune(strings.TrimSpace(text)))
	if runes == 0 {
		return Report{Verdict: VerdictPass, Findings: []Finding{}, Summary: "空文本，无需扫描"}
	}

	sentences := analysiskit.SplitSentences(text)
	var findings []Finding

	for _, sent := range sentences {
		for i := range patterns {
			p := &patterns[i]
			if !p.re.MatchString(sent) {
				continue
			}
			if p.guard != nil && p.guard(sent) {
				continue
			}
			findings = append(findings, Finding{
				Category:   p.category,
				Pattern:    p.name,
				Sentence:   truncate(sent, 60),
				Reason:     reasonFor(p.category),
				Suggestion: categorySuggestions[p.category],
			})
			break // 每句只记一次，避免同一句多模式刷分
		}
	}

	// 重复套话：长度≥8字的相同句子出现≥2次（最多记 3 条防刷分）。
	findings = append(findings, repeatedSentenceFindings(sentences)...)

	weighted := 0
	for _, f := range findings {
		weighted += categoryWeights[f.Category]
	}

	report := Report{
		Findings:   findings,
		TotalRunes: runes,
		Score:      float64(weighted) * 1000 / float64(runes),
	}
	report.Verdict, report.Summary = judge(weighted, runes, findings)
	return report
}

// judge 依据加权命中数与文本长度给出判定：
//   - 加权命中 ≥5：直接 needs_revision；
//   - 加权命中 ≥3 且文本短（<2000 字，命中密度高）：needs_revision；
//   - 其余 pass。
func judge(weighted, runes int, findings []Finding) (string, string) {
	if len(findings) == 0 {
		return VerdictPass, "未命中套话模式"
	}
	switch {
	case weighted >= 5:
		return VerdictNeedsRevision, "套话/无支撑断言命中过多，建议修订后重交"
	case weighted >= 3 && runes < 2000:
		return VerdictNeedsRevision, "短文本套话密度过高，建议修订"
	default:
		return VerdictPass, "存在少量套话，可接受；建议复核 findings 后决定是否修订"
	}
}

// repeatedSentenceFindings 检测重复出现的长句。
func repeatedSentenceFindings(sentences []string) []Finding {
	counts := make(map[string]int)
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len([]rune(s)) >= 8 {
			counts[s]++
		}
	}
	var findings []Finding
	for s, n := range counts {
		if n < 2 || len(findings) >= 3 {
			continue
		}
		findings = append(findings, Finding{
			Category:   CatRepeatedBoilerplate,
			Pattern:    "repeated-sentence",
			Sentence:   truncate(s, 60),
			Reason:     "同一句子重复出现 " + strconv.Itoa(n) + " 次",
			Suggestion: categorySuggestions[CatRepeatedBoilerplate],
		})
	}
	return findings
}

func reasonFor(category string) string {
	switch category {
	case CatUnsupportedEffect:
		return "效果断言句中无定量数据支撑"
	case CatConclusoryRemark:
		return "结论式评述且无引证"
	default:
		return "命中空话模板"
	}
}

func truncate(s string, maxLen int) string {
	return util.TruncateRunes(strings.TrimSpace(s), maxLen)
}
