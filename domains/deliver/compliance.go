package deliver

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Scope 是合规检查的范围。
type Scope string

const (
	// ScopeExternalFiling 是对外提交件：不得含内部工作记录。
	ScopeExternalFiling Scope = "external-filing"
	// ScopeInternalRecord 是内部记录件（如无效分析意见）：只查章节编号与证据编号。
	ScopeInternalRecord Scope = "internal-record"
)

// Issue 是一条合规问题。
type Issue struct {
	// Rule 是命中的规则。
	Rule string
	// Message 是可读描述。
	Message string
}

// 内部工作章节标题用语：命中即表示内部记录进入了对外件。
var internalSectionKeywords = []string{
	"待办", "待填", "核验记录", "引用核验", "修订记录", "修订说明",
	"最坏情形", "缺口汇总", "口径",
}

// 正文中的内部工作用语。
var internalBodyMarkers = []struct {
	label   string
	pattern *regexp.Regexp
}{
	{"待办/待填", regexp.MustCompile(`待办|待填|TODO|FIXME`)},
	{"核验记录", regexp.MustCompile(`核验记录|引用核验`)},
	{"修订记录", regexp.MustCompile(`修订记录|修订说明`)},
	{"最坏情形", regexp.MustCompile(`最坏情形`)},
	{"缺口汇总", regexp.MustCompile(`缺口汇总|缺口清单`)},
	{"成功概率分档", regexp.MustCompile(`成功率|胜诉率|成功概率|胜率`)},
	{"内部文件路径", regexp.MustCompile(`(?i)source_?path`)},
}

// 章节编号：中文数码开头并后接「、」。
var sectionNumberPattern = regexp.MustCompile(`^(十[一二三四五六七八九]?|[一二三四五六七八九]十[一二三四五六七八九]?|[一二三四五六七八九])、`)

// 证据编号：前缀 + 数字，如「证据1」「D2」。
var evidenceIDPattern = regexp.MustCompile(`^(\D+?)\s*(\d+)$`)

// CheckCompliance 检查渲染后的文书 HTML。
func CheckCompliance(source string, scope Scope) ([]Issue, error) {
	document, err := html.Parse(bytes.NewReader([]byte(source)))
	if err != nil {
		return nil, fmt.Errorf("deliver: 解析待检 HTML: %w", err)
	}
	isExternal := scope != ScopeInternalRecord

	issues := make([]Issue, 0)
	issues = append(issues, checkSectionNumbering(document)...)
	if isExternal {
		issues = append(issues, checkInternalSections(document)...)
		issues = append(issues, checkInternalBody(document)...)
	}
	issues = append(issues, checkEvidenceCrossReference(document)...)
	return issues, nil
}

// checkSectionNumbering 检查中文数码章节编号递增且不重复。
func checkSectionNumbering(document *html.Node) []Issue {
	var issues []Issue
	seen := make(map[int]string, 8)
	last := 0
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && isHeading(node.Data) {
			text := strings.TrimSpace(textContent(node))
			if match := sectionNumberPattern.FindString(text); match != "" {
				number := cnNumber(match[:len(match)-len("、")])
				if number > 0 {
					if previous, dup := seen[number]; dup {
						issues = append(issues, Issue{
							Rule:    "section-numbering",
							Message: fmt.Sprintf("章节编号「%s」重复（另有标题「%s」）", match, previous),
						})
					} else if last > 0 && number < last {
						issues = append(issues, Issue{
							Rule:    "section-numbering",
							Message: fmt.Sprintf("章节编号未按出现顺序递增：「%s」出现在更大的编号之后", match),
						})
					}
					seen[number] = text
					last = number
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return issues
}

// checkInternalSections 检查对外件是否含内部工作章节标题。
func checkInternalSections(document *html.Node) []Issue {
	var issues []Issue
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && isHeading(node.Data) {
			text := textContent(node)
			for _, keyword := range internalSectionKeywords {
				if strings.Contains(text, keyword) {
					issues = append(issues, Issue{
						Rule:    "internal-section",
						Message: fmt.Sprintf("对外文书的标题「%s」含内部工作用语「%s」", strings.TrimSpace(text), keyword),
					})
					break
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	return issues
}

// checkInternalBody 检查对外件正文是否含内部工作用语。
func checkInternalBody(document *html.Node) []Issue {
	text := visibleText(document)
	var issues []Issue
	for _, marker := range internalBodyMarkers {
		if match := marker.pattern.FindString(text); match != "" {
			issues = append(issues, Issue{
				Rule:    "internal-body",
				Message: fmt.Sprintf("对外文书正文含内部工作用语（%s）：「%s」", marker.label, match),
			})
		}
	}
	return issues
}

// checkEvidenceCrossReference 双向核对证据清单与正文引用。
func checkEvidenceCrossReference(document *html.Node) []Issue {
	table := findEvidenceTable(document)
	if table == nil {
		return nil
	}
	registered := make(map[string]string, 8)
	prefixes := make(map[string]bool, 4)
	for index, id := range evidenceIDs(table) {
		if strings.Trim(id, "_ ") == "" {
			return []Issue{{
				Rule:    "evidence-cross-reference",
				Message: fmt.Sprintf("证据清单第 %d 行未填编号：正文无法引用该条证据", index+1),
			}}
		}
		match := evidenceIDPattern.FindStringSubmatch(id)
		if match == nil {
			continue
		}
		prefix := match[1]
		canonical := canonicalEvidenceID(prefix, match[2])
		registered[canonical] = prefix
		prefixes[prefix] = true
	}
	if len(registered) == 0 {
		return nil
	}

	// 正文 = 整篇可见文本减去清单表所在子树（按节点切除，不按字符串替换）。
	bodyText := visibleTextExcept(document, table)
	cited := make(map[string]bool, 8)
	for _, prefix := range mapKeys(prefixes) {
		pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(prefix) + `\s*(\d+)`)
		for _, match := range pattern.FindAllStringSubmatchIndex(bodyText, -1) {
			// 前缀左侧不能是 ASCII 字母数字：避免把 END2 的尾部当成 D2。
			if match[0] > 0 && isASCIIAlnum(bodyText[match[0]-1]) {
				continue
			}
			cited[canonicalEvidenceID(prefix, bodyText[match[2]:match[3]])] = true
		}
	}

	var issues []Issue
	for id := range registered {
		if !cited[id] {
			issues = append(issues, Issue{
				Rule:    "evidence-cross-reference",
				Message: fmt.Sprintf("证据清单登记的「%s」在正文中未被引用", id),
			})
		}
	}
	for id := range cited {
		if _, ok := registered[id]; !ok {
			issues = append(issues, Issue{
				Rule:    "evidence-cross-reference",
				Message: fmt.Sprintf("正文引用的「%s」未登记在证据清单", id),
			})
		}
	}
	return issues
}

// findEvidenceTable 找表头同时含「编号」与「证据名称」的表格。
func findEvidenceTable(document *html.Node) *html.Node {
	return findElementMatching(document, func(node *html.Node) bool {
		if node.Type != html.ElementNode || node.Data != "table" {
			return false
		}
		headers := make([]string, 0, 4)
		var walk func(*html.Node)
		walk = func(candidate *html.Node) {
			if candidate.Type == html.ElementNode && candidate.Data == "th" {
				headers = append(headers, strings.TrimSpace(textContent(candidate)))
			}
			for child := candidate.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(node)
		joined := strings.Join(headers, "|")
		return strings.Contains(joined, "编号") && strings.Contains(joined, "证据名称")
	})
}

// evidenceIDs 取证据表的编号列文本。
func evidenceIDs(table *html.Node) []string {
	var ids []string
	var walk func(node *html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tbody" {
			for row := node.FirstChild; row != nil; row = row.NextSibling {
				if row.Type != html.ElementNode || row.Data != "tr" {
					continue
				}
				for cell := row.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
						ids = append(ids, strings.TrimSpace(textContent(cell)))
						break
					}
				}
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(table)
	return ids
}

// canonicalEvidenceID 把编号规范化为「前缀+去零序号」。
func canonicalEvidenceID(prefix, number string) string {
	trimmed := strings.TrimLeft(number, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	return prefix + trimmed
}

// isHeading 判断是否为标题元素。
func isHeading(tag string) bool {
	return len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6'
}

// textContent 返回元素的可见文本（不含注释与脚本样式）。
func textContent(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(candidate *html.Node) {
		switch candidate.Type {
		case html.TextNode:
			builder.WriteString(candidate.Data)
		case html.ElementNode:
			if candidate.Data == "style" || candidate.Data == "script" {
				return
			}
		}
		for child := candidate.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

// visibleText 返回整篇文档的可见文本。
func visibleText(document *html.Node) string {
	return textContent(findElement(document, "body"))
}

// visibleTextExcept 返回整篇可见文本，但跳过 skip 子树。
func visibleTextExcept(document *html.Node, skip *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == skip {
			return
		}
		switch node.Type {
		case html.TextNode:
			builder.WriteString(node.Data)
		case html.ElementNode:
			if node.Data == "style" || node.Data == "script" {
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	if body := findElement(document, "body"); body != nil {
		walk(body)
	}
	return builder.String()
}

// isASCIIAlnum 判断字节是否为 ASCII 字母或数字。
func isASCIIAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// cnNumber 把中文数码转成整数（仅支持 1–99）。
func cnNumber(text string) int {
	digits := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	if text == "十" {
		return 10
	}
	runes := []rune(text)
	if len(runes) == 1 {
		return digits[runes[0]]
	}
	if len(runes) == 2 {
		if runes[0] == '十' {
			return 10 + digits[runes[1]]
		}
		if runes[1] == '十' {
			return digits[runes[0]] * 10
		}
		return 0
	}
	if len(runes) == 3 && runes[1] == '十' {
		return digits[runes[0]]*10 + digits[runes[2]]
	}
	return 0
}

// mapKeys 返回映射的键列表。
func mapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
