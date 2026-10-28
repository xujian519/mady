package specdrafting

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// 说明书形式检查：标题集合、章节顺序、附图标记与图号写法、段落编号、摘要标题。
//
// 依据：《专利法实施细则》第二十条（说明书由技术领域、背景技术、发明内容、
// 附图说明、具体实施方式五部分组成并按该顺序撰写）；《专利审查指南》第二部分
// 第二章 §2.2.6（附图标记放在技术名称之后、不加括号）；第一部分第一章 4.5.1
// 与第二章 7.5（摘要不得使用标题）。段落编号无法条依据，只作提示级。

// specSectionHeadings 是实施细则第二十条规定的五部分标题。
var specSectionHeadings = []string{"技术领域", "背景技术", "发明内容", "附图说明", "具体实施方式"}

// wrapperHeadings 是渲染模板在五部分之外加的文档级标题。
var wrapperHeadings = []string{"说明书"}

// nonMarkSuffixes 是括号数字前的收尾字：它们引入列举或引用，不是部件名。
var nonMarkSuffixes = []string{"例", "骤", "式", "图", "表", "项", "条", "章", "节", "次", "者"}

// nameBoundaries 是附图标记名称的切分词（无法分词，按最后一个边界词截取）。
var nameBoundaries = []string{
	"所述", "上述", "还有", "其中", "以及", "并且", "或者",
	"包括", "包含", "设有", "设置", "连接", "固定", "安装", "具有", "位于", "用于",
	"形成", "构成", "通过", "分别", "若干", "多个",
	"的", "与", "和", "及", "或",
}

const markContextWindow = 12

var (
	headingLineRE        = regexp.MustCompile(`^#{1,6}[ \t]*\S`)
	parenthesizedMarkRE  = regexp.MustCompile(`[\p{Han}]{2,20}（[0-9]{1,3}）`)
	spacedFigureNumberRE = regexp.MustCompile("图[ \t]+[0-9一二三四五六七八九十]+")
	paragraphNumberRE    = regexp.MustCompile(`\[[0-9]{4,5}\]`)
	abstractHeadingRE    = regexp.MustCompile(`(?m)^#{1,6}[ \t]*\S`)
)

// specHeading 是一行标题及其行号。
type specHeading struct {
	line int
	text string
}

func collectHeadings(text string) []specHeading {
	var found []specHeading
	for index, line := range strings.Split(text, "\n") {
		if !headingLineRE.MatchString(line) {
			continue
		}
		trimmed := strings.TrimLeft(line, "#")
		found = append(found, specHeading{line: index + 1, text: strings.TrimSpace(trimmed)})
	}
	return found
}

// CheckHeadingSet 检出五部分与文档级标题之外的标题。
func CheckHeadingSet(text string) []Violation {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	allowed := make(map[string]bool, len(specSectionHeadings)+len(wrapperHeadings))
	for _, name := range specSectionHeadings {
		allowed[name] = true
	}
	for _, name := range wrapperHeadings {
		allowed[name] = true
	}
	var extra []string
	for _, heading := range collectHeadings(text) {
		if !allowed[heading.text] {
			extra = append(extra, heading.text)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return []Violation{{
		RuleName:   "heading_set",
		RuleBasis:  "《专利法实施细则》第二十条",
		Severity:   SeverityError,
		Message:    fmt.Sprintf("说明书出现 %d 个规定之外的标题：%s（实施细则第二十条只规定技术领域、背景技术、发明内容、附图说明、具体实施方式五部分标题）", len(extra), strings.Join(extra, "、")),
		Suggestion: "删除这些标题，其下内容以自然段落平铺书写；发明名称写在说明书首页正文上方居中，不作标题",
	}}
}

// CheckSectionOrder 检查五部分是否按法定顺序排列；五部分不齐时不报（由缺失检查负责）。
func CheckSectionOrder(text string) []Violation {
	headings := collectHeadings(text)
	indexOf := make(map[string]int, len(headings))
	for index, heading := range headings {
		if _, exists := indexOf[heading.text]; !exists {
			indexOf[heading.text] = index
		}
	}
	type positioned struct {
		name  string
		index int
	}
	positions := make([]positioned, 0, len(specSectionHeadings))
	for _, name := range specSectionHeadings {
		index, ok := indexOf[name]
		if !ok {
			return nil
		}
		positions = append(positions, positioned{name: name, index: index})
	}
	inOrder := true
	for i, position := range positions {
		if i > 0 && position.index < positions[i-1].index {
			inOrder = false
		}
	}
	if inOrder {
		return nil
	}
	sorted := append([]positioned(nil), positions...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].index < sorted[j].index })
	actualOrder := make([]string, len(sorted))
	for i, position := range sorted {
		actualOrder[i] = position.name
	}
	return []Violation{{
		RuleName:   "section_order",
		RuleBasis:  "《专利法实施细则》第二十条",
		Severity:   SeverityError,
		Message:    "说明书各部分顺序不符：实际为 " + strings.Join(actualOrder, "、") + "，应为 " + strings.Join(specSectionHeadings, "、") + "（实施细则第二十条要求按该方式和顺序撰写）",
		Suggestion: "按" + strings.Join(specSectionHeadings, "、") + "的顺序重排各部分",
	}}
}

// markName 截取括号前的中文名称（按边界词切分；不足两字时回退整段窗口）。
func markName(run string) string {
	runes := []rune(run)
	window := runes
	if len(window) > markContextWindow {
		window = window[len(window)-markContextWindow:]
	}
	text := string(window)
	start := 0
	for _, word := range nameBoundaries {
		if at := strings.LastIndex(text, word); at >= 0 {
			if candidate := at + len(word); candidate > start {
				start = candidate
			}
		}
	}
	name := text[start:]
	if len([]rune(name)) >= 2 {
		return name
	}
	return text
}

// CheckFigureMarkParentheses 检出声说明正文中加括号的附图标记（权利要求与摘要相反，需加括号，故只查正文）。
func CheckFigureMarkParentheses(text string) []Violation {
	var violations []Violation
	seen := make(map[string]bool)
	for _, raw := range parenthesizedMarkRE.FindAllString(text, -1) {
		namePart, rest, found := strings.Cut(raw, "（")
		if !found {
			continue
		}
		name := markName(namePart)
		if endsWithAny(name, nonMarkSuffixes) {
			continue
		}
		digits := strings.TrimSuffix(rest, "）")
		mark := name + "（" + digits + "）"
		if seen[mark] {
			continue
		}
		seen[mark] = true
		violations = append(violations, Violation{
			RuleName:   "figure_mark_parentheses",
			RuleBasis:  "《专利审查指南》第二部分第二章 §2.2.6",
			Severity:   SeverityError,
			Message:    "说明书正文给附图标记加括号：「" + mark + "」（标记应放在相应技术名称之后，不加括号）",
			Suggestion: "改写为「" + name + digits + "」，把标记直接跟在技术名称后",
		})
	}
	return violations
}

// CheckFigureNumberSpacing 检出「图 1」这类图号中夹空格的写法。
func CheckFigureNumberSpacing(text string) []Violation {
	var violations []Violation
	seen := make(map[string]bool)
	for _, hit := range spacedFigureNumberRE.FindAllString(text, -1) {
		if seen[hit] {
			continue
		}
		seen[hit] = true
		normalized := strings.NewReplacer(" ", "", "\t", "").Replace(hit)
		violations = append(violations, Violation{
			RuleName:   "figure_number_spacing",
			Severity:   SeverityError,
			Message:    "图号写作「" + hit + "」，不应在「图」与图号之间留空格",
			Suggestion: "改写为「" + normalized + "」",
		})
	}
	return violations
}

// CheckParagraphNumbering 检出段号（[0001] 一类）。法条与指南均未要求，只作提示。
func CheckParagraphNumbering(text string) []Violation {
	hit := paragraphNumberRE.FindString(text)
	if hit == "" {
		return nil
	}
	return []Violation{{
		RuleName:   "paragraph_numbering",
		Severity:   SeverityInfo,
		Message:    "说明书出现段落编号「" + hit + "」，法条与《专利审查指南》均未要求段落编号",
		Suggestion: "若所在部署的提交体例不使用段落编号，删除全部段落编号；由渲染模板控制，不要在正文里手工增删",
	}}
}

// CheckAbstractHeading 检查摘要文字是否使用了标题。
func CheckAbstractHeading(abstract string) []Violation {
	if strings.TrimSpace(abstract) == "" || !abstractHeadingRE.MatchString(abstract) {
		return nil
	}
	return []Violation{{
		RuleName:    "abstract_heading",
		RuleBasis:   "《专利审查指南》第一部分第一章 4.5.1、第二部分第二章 7.5",
		Severity:    SeverityError,
		SectionName: "摘要",
		Message:     "摘要文字部分使用了标题（摘要文字部分不得使用标题）",
		Suggestion:  "删除摘要中的标题，改为一段连续文字；摘要不超过 300 字，正文中出现的附图标记加括号",
	}}
}

// CheckSpecStyle 按报告顺序返回全部形式违规。
func CheckSpecStyle(text, abstract string) []Violation {
	violations := CheckHeadingSet(text)
	violations = append(violations, CheckSectionOrder(text)...)
	violations = append(violations, CheckFigureMarkParentheses(text)...)
	violations = append(violations, CheckFigureNumberSpacing(text)...)
	violations = append(violations, CheckParagraphNumbering(text)...)
	violations = append(violations, CheckAbstractHeading(abstract)...)
	return violations
}

func endsWithAny(text string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}
