// pincite.go 实现对照表的逐格 pin-cite 确定性校验：
// 每一格引用（quote + pinCite）都必须能在源文中定位——引文剥空白后必须是
// 源文的逐字子串（容忍 PDF 折行/换行与全角空格），pinCite 引用的段号
// 定位符必须真实存在于源文。校验是纯函数，不依赖 LLM。

package claimchart

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// PinCiteIssueKind 区分校验问题的类别。
type PinCiteIssueKind = string

const (
	// IssueQuoteMismatch 表示引文剥空白后不在源文中（疑似虚构或转述）。
	IssueQuoteMismatch PinCiteIssueKind = "quote-mismatch"
	// IssueLocatorMissing 表示 pinCite 引用的段号在源文中不存在。
	IssueLocatorMissing PinCiteIssueKind = "locator-missing"
	// IssueLocatorAbsent 表示源文含段号定位符但本行 pinCite 未定位到段号。
	IssueLocatorAbsent PinCiteIssueKind = "locator-absent"
)

// PinCiteIssue 是一行对照的 pin-cite 校验问题。
type PinCiteIssue struct {
	RowIndex  int              `json:"rowIndex"`
	ElementID string           `json:"elementId"`
	TargetID  string           `json:"targetId"`
	Kind      PinCiteIssueKind `json:"kind"`
	Message   string           `json:"message"`
}

// paragraphLocatorRe 匹配专利公开文本中的段落号定位符：[0032]、［0032］。
var paragraphLocatorRe = regexp.MustCompile(`[\[［](\d{3,4})[\]］]`)

// maxQuoteRunes 是单条引文的最大字符数（超长截取逐字前缀，不追加省略号，
// 以保证引文始终是源文的逐字子串）。
const maxQuoteRunes = 300

// stripWhitespace 剥除所有 Unicode 空白（含全角空格与 PDF 折行带来的换行）。
func stripWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// ValidateQuote 报告 quote（剥空白后）是否为 source（剥空白后）的逐字子串。
// 空 quote 视为未通过——空引文无从核验。
func ValidateQuote(quote, source string) bool {
	q := stripWhitespace(quote)
	if q == "" {
		return false
	}
	return strings.Contains(stripWhitespace(source), q)
}

// extractParagraphLocators 返回 text 中出现的段号定位符（如 "[0032]"），按出现顺序；
// 无匹配时返回 nil。
func extractParagraphLocators(text string) []string {
	matches := paragraphLocatorRe.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	locs := make([]string, 0, len(matches))
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			locs = append(locs, m)
		}
	}
	return locs
}

// ValidateChart 对 chart 的每一行做 pin-cite 校验，返回问题清单（按 Rows 顺序）。
// sources 以 target ID 为键提供源文全文；缺失源文的目标跳过校验——无法验证
// 不等于验证失败，由调用方以 needs-evidence 语义处理。
func ValidateChart(chart *ClaimChart, sources map[string]string) []PinCiteIssue {
	if chart == nil {
		return nil
	}
	var issues []PinCiteIssue
	for i, row := range chart.Rows {
		source, ok := sources[row.TargetID]
		if !ok || source == "" {
			continue
		}
		issues = append(issues, validateRow(i, row, source)...)
	}
	return issues
}

// validateRow 校验单行；引文核验通过且无问题时该行才值得标记 Verified。
func validateRow(idx int, row ChartRow, source string) []PinCiteIssue {
	var issues []PinCiteIssue
	add := func(kind PinCiteIssueKind, format string, args ...any) {
		issues = append(issues, PinCiteIssue{
			RowIndex:  idx,
			ElementID: row.ElementID,
			TargetID:  row.TargetID,
			Kind:      kind,
			Message:   fmt.Sprintf(format, args...),
		})
	}

	if row.Quote != "" && !ValidateQuote(row.Quote, source) {
		add(IssueQuoteMismatch, "引文剥空白后不在源文中，疑似虚构或转述：%q", truncateForMessage(row.Quote))
	}

	sourceLocs := extractParagraphLocators(source)
	if len(sourceLocs) == 0 {
		// 源文本身无段号（如产品说明），定位符核验无从谈起。
		return issues
	}
	pinLocs := extractParagraphLocators(row.PinCite)
	if len(pinLocs) == 0 {
		if row.Quote != "" {
			add(IssueLocatorAbsent, "源文含段号定位符但本行 pinCite 未定位到段号")
		}
		return issues
	}
	sourceSet := make(map[string]bool, len(sourceLocs))
	for _, loc := range sourceLocs {
		sourceSet[normalizeLocator(loc)] = true
	}
	for _, loc := range pinLocs {
		if !sourceSet[normalizeLocator(loc)] {
			add(IssueLocatorMissing, "pinCite 引用的段号 %s 在源文中不存在", loc)
		}
	}
	return issues
}

// normalizeLocator 把定位符归一化为纯数字（兼容全角括号）。
func normalizeLocator(loc string) string {
	var b strings.Builder
	for _, r := range loc {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truncateForMessage 供问题消息展示，超长截断并允许省略号（仅用于展示，非引文）。
func truncateForMessage(s string) string {
	r := []rune(s)
	if len(r) <= 40 {
		return s
	}
	return string(r[:40]) + "…"
}
