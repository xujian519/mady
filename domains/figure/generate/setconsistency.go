package generate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// 跨图一致性：同一案件多张附图的标记归并、跨图名称冲突、图号连续性、权利要求/
// 说明书引用对齐，以及摘要附图指定是否有效。
//
// 与 domains/figure 的电学一致性核验互补：那边核验符号/网络（依赖识图结果），
// 这边核验「标记—图号—文本引用」这条纯文本链路，两者输入不同、互不替代。

// HasNumeralPrefix 报告数字前文是否以编号词收尾（是则该数字不是附图标记）。
func HasNumeralPrefix(before string) bool { return hasNumeralPrefixRunes([]rune(before)) }

// hasNumeralPrefixRunes 只看紧邻数字的窗口：判定只依赖词尾，无需整段前缀。
func hasNumeralPrefixRunes(runes []rune) bool {
	const window = 24
	if len(runes) > window {
		runes = runes[len(runes)-window:]
	}
	text := strings.TrimRight(string(runes), " \t\r\n（(.")
	for _, suffix := range []string{
		"公式", "实施例", "实施方式", "步骤", "第", "共", "约",
		"权利要求", "权利要求书", "标记", "编号", "序号", "年", "月", "日",
	} {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	lower := strings.ToLower(text)
	for _, suffix := range []string{"formula", "figure", "fig", "claims", "claim", "steps", "step"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	if strings.HasSuffix(text, "式") {
		prefix := strings.TrimSuffix(text, "式")
		if prefix == "" {
			return true
		}
		// 「方式/形式/样式/模式/程式」里的「式」是名词结尾，不是编号词。
		if !strings.ContainsRune("方形样模程", lastRune(prefix)) {
			return true
		}
	}
	for _, soft := range []string{"图", "表", "条", "项", "章", "节", "款", "例"} {
		if !strings.HasSuffix(text, soft) {
			continue
		}
		prefix := strings.TrimSuffix(text, soft)
		if prefix == "" || strings.ContainsRune("，。；：、,.;:!?（）()〔〕【】[]「」“”\"'《》 \t\n如见参附前后上下本该此其与和及或在由对至从把将的", lastRune(prefix)) {
			return true
		}
	}
	return false
}

func lastRune(text string) rune {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	return runes[len(runes)-1]
}

// ExtractNumeralRefs 抽取权利要求/说明书中的纯数字附图标记（`壳体(10)`、`壳体10`）。
//
// 手写扫描而非正则：Go RE2 无 lookaround。右边界若写死为标点，「壳体10与盖板20」
// 会漏掉 10；若放开为任意字符，又要逐类排除单位与日期。分步判定更可读可测：
// 前接汉字/左括号、后接任意非数字（排除拉丁字母、百分号、角度与温度单位），
// 再用编号词前缀表排除「图1/公式(3)/权利要求1/2020年」这类非标记数字。
func ExtractNumeralRefs(text string, limit int) []int {
	if limit <= 0 {
		limit = 40
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	runes := []rune(text)
	found := map[int]bool{}
	for i := 0; i < len(runes); {
		if !isDigitRune(runes[i]) {
			i++
			continue
		}
		start := i
		for i < len(runes) && isDigitRune(runes[i]) {
			i++
		}
		end := i
		if end-start > 3 || !numeralPreceded(runes, start) || !numeralFollowedOK(runes, end) {
			continue
		}
		if hasNumeralPrefixRunes(runes[:start]) {
			continue
		}
		value, err := strconv.Atoi(string(runes[start:end]))
		if err != nil {
			continue
		}
		found[value] = true
		if len(found) >= limit {
			break
		}
	}
	return sortedRefs(found)
}

// numeralPreceded 报告数字是否处在标记位置：前接汉字、左括号，或行首（编号清单）。
func numeralPreceded(runes []rune, start int) bool {
	if start == 0 {
		return true
	}
	prev := runes[start-1]
	if isHanRune(prev) || prev == '(' || prev == '（' || prev == '\n' {
		return true
	}
	if unicode.IsSpace(prev) {
		before := start - 1
		for before >= 0 && (runes[before] == ' ' || runes[before] == '\t' || runes[before] == '\r') {
			before--
		}
		return before < 0 || runes[before] == '\n'
	}
	return false
}

// numeralFollowedOK 排除跟随拉丁字母（10mm）、百分号（10%）、角度与温度（10°）
// 以及小数（10.5）的数字；其余字符均可作为右边界。
func numeralFollowedOK(runes []rune, end int) bool {
	if end >= len(runes) {
		return true
	}
	next := runes[end]
	switch {
	case isDigitRune(next), next == '%', next == '％', next == '‰', next == '°', next == '℃':
		return false
	case unicode.IsLetter(next) && !isHanRune(next):
		return false
	case next == '.' || next == '．':
		return end+1 >= len(runes) || !isDigitRune(runes[end+1])
	default:
		return true
	}
}

func isHanRune(r rune) bool { return unicode.Is(unicode.Han, r) }

func isDigitRune(r rune) bool { return r >= '0' && r <= '9' }

func sortedRefs(set map[int]bool) []int {
	if len(set) == 0 {
		return nil
	}
	out := make([]int, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

// SetRef 是一张图上识别出的标记与名称。
type SetRef struct {
	Ref  string `json:"ref"`
	Name string `json:"name,omitempty"`
}

// SetFigure 是一张附图参与跨图核验所需的最小事实。
type SetFigure struct {
	Number int      `json:"number"`
	Refs   []SetRef `json:"refs"`
}

// SetConflictValue 是某图号下冲突维度的一个取值。
type SetConflictValue struct {
	FigureNumber int    `json:"figure_number"`
	Value        string `json:"value"`
}

// SetConflict 是同一标记在不同图中的名称不一致。
type SetConflict struct {
	Ref           string             `json:"ref"`
	FigureNumbers []int              `json:"figure_numbers"`
	Values        []SetConflictValue `json:"values"`
	Message       string             `json:"message"`
}

// SetConsistencyInput 是跨图一致性核验输入。
type SetConsistencyInput struct {
	Figures []SetFigure `json:"figures"`
	// Text 为权利要求书或说明书正文，用于抽取文本引用的附图标记。
	Text string `json:"text,omitempty"`
	// AbstractFigure 为指定作为摘要附图的图号；0 表示未指定。
	AbstractFigure int `json:"abstract_figure,omitempty"`
	// RefLimit 为文本抽取上限（默认 40）。
	RefLimit int `json:"ref_limit,omitempty"`
}

// SetConsistencyReport 是跨图一致性报告。
type SetConsistencyReport struct {
	FigureCount            int           `json:"figure_count"`
	MissingFigureNumbers   []int         `json:"missing_figure_numbers,omitempty"`
	DuplicateFigureNumbers []int         `json:"duplicate_figure_numbers,omitempty"`
	Conflicts              []SetConflict `json:"conflicts,omitempty"`
	MissingRefs            []int         `json:"missing_refs,omitempty"`
	AbstractFigure         int           `json:"abstract_figure,omitempty"`
	AbstractFigureProblem  string        `json:"abstract_figure_problem,omitempty"`
	Warnings               []string      `json:"warnings,omitempty"`
	Summary                string        `json:"summary"`
	Consistent             bool          `json:"consistent"`
}

// CheckFigureSet 核验同一案件多张附图的标记一致性与图号连续性。
func CheckFigureSet(input SetConsistencyInput) (SetConsistencyReport, error) {
	if len(input.Figures) == 0 {
		return SetConsistencyReport{}, newError(ErrEmptyInput, "跨图核验至少需要一张附图")
	}
	byNumber, duplicates, missing, err := collectFigureNumbers(input.Figures)
	if err != nil {
		return SetConsistencyReport{}, err
	}
	refOrder, numericRefs := collectRefs(input.Figures)

	report := SetConsistencyReport{
		FigureCount:            len(input.Figures),
		MissingFigureNumbers:   missing,
		DuplicateFigureNumbers: duplicates,
		Conflicts:              findNameConflicts(input.Figures, refOrder),
		MissingRefs:            missingTextRefs(input.Text, numericRefs, input.RefLimit),
		AbstractFigure:         input.AbstractFigure,
	}
	if input.AbstractFigure != 0 {
		if !byNumber[input.AbstractFigure] {
			report.AbstractFigureProblem = fmt.Sprintf("摘要附图指定的图 %d 不在附图集合中", input.AbstractFigure)
		} else {
			report.Warnings = append(report.Warnings,
				"摘要附图仅核验图号有效性；该图能否代表技术方案要点需另行判断")
		}
	}
	if len(report.DuplicateFigureNumbers) > 0 {
		report.Warnings = append(report.Warnings, "图号重复")
	}
	if len(report.MissingFigureNumbers) > 0 {
		report.Warnings = append(report.Warnings, "图号不连续")
	}

	report.Consistent = len(report.Conflicts) == 0 && len(report.MissingRefs) == 0 &&
		len(report.DuplicateFigureNumbers) == 0 && len(report.MissingFigureNumbers) == 0 &&
		report.AbstractFigureProblem == ""
	report.Summary = buildSetSummary(report)
	return report, nil
}

// collectFigureNumbers 汇总图号，并给出重复号与 1..最大图号 之间的缺号。
func collectFigureNumbers(figures []SetFigure) (map[int]bool, []int, []int, error) {
	byNumber := map[int]bool{}
	var duplicates []int
	maxNumber := 0
	for _, figure := range figures {
		if figure.Number < 1 {
			return nil, nil, nil, newError(ErrInvalidLabel, "图号必须是正整数：%d", figure.Number)
		}
		if byNumber[figure.Number] {
			duplicates = append(duplicates, figure.Number)
		}
		byNumber[figure.Number] = true
		if figure.Number > maxNumber {
			maxNumber = figure.Number
		}
	}
	sort.Ints(duplicates)
	var missing []int
	for number := 1; number <= maxNumber; number++ {
		if !byNumber[number] {
			missing = append(missing, number)
		}
	}
	return byNumber, duplicates, missing, nil
}

// collectRefs 返回按首次出现排序的标记与其中的纯数字标记集合。
func collectRefs(figures []SetFigure) ([]string, map[int]bool) {
	numeric := map[int]bool{}
	seen := map[string]bool{}
	var order []string
	for _, figure := range figures {
		for _, ref := range figure.Refs {
			key := strings.TrimSpace(ref.Ref)
			if key == "" {
				continue
			}
			if !seen[key] {
				seen[key] = true
				order = append(order, key)
			}
			if value, err := strconv.Atoi(key); err == nil {
				numeric[value] = true
			}
		}
	}
	return order, numeric
}

// findNameConflicts 找出同一标记在不同图上名称不一致的情况。
func findNameConflicts(figures []SetFigure, refOrder []string) []SetConflict {
	var conflicts []SetConflict
	for _, key := range refOrder {
		names := map[string]bool{}
		var values []SetConflictValue
		for _, figure := range figures {
			for _, ref := range figure.Refs {
				if strings.TrimSpace(ref.Ref) != key {
					continue
				}
				name := strings.TrimSpace(ref.Name)
				if name == "" || names[name] {
					continue
				}
				names[name] = true
				values = append(values, SetConflictValue{FigureNumber: figure.Number, Value: name})
			}
		}
		if len(names) <= 1 {
			continue
		}
		figureNumbers := make([]int, 0, len(values))
		for _, value := range values {
			figureNumbers = append(figureNumbers, value.FigureNumber)
		}
		sort.Ints(figureNumbers)
		conflicts = append(conflicts, SetConflict{
			Ref: key, FigureNumbers: figureNumbers, Values: values,
			Message: fmt.Sprintf("标记 %s 在不同图中名称不一致：%s", key, describeConflictValues(values)),
		})
	}
	return conflicts
}

// missingTextRefs 返回文本引用、但任何图中都未识别的数字标记。
func missingTextRefs(text string, numericRefs map[int]bool, limit int) []int {
	var missing []int
	for _, value := range ExtractNumeralRefs(text, limit) {
		if !numericRefs[value] {
			missing = append(missing, value)
		}
	}
	return missing
}

func describeConflictValues(values []SetConflictValue) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = fmt.Sprintf("图%d=%s", value.FigureNumber, value.Value)
	}
	return strings.Join(parts, "；")
}

func buildSetSummary(report SetConsistencyReport) string {
	if report.Consistent {
		return fmt.Sprintf("%d 张附图的图号、标记与文本引用一致。", report.FigureCount)
	}
	var parts []string
	if len(report.DuplicateFigureNumbers) > 0 {
		parts = append(parts, fmt.Sprintf("图号重复 %v", report.DuplicateFigureNumbers))
	}
	if len(report.MissingFigureNumbers) > 0 {
		parts = append(parts, fmt.Sprintf("缺图号 %v", report.MissingFigureNumbers))
	}
	if len(report.Conflicts) > 0 {
		parts = append(parts, fmt.Sprintf("%d 处标记名称冲突", len(report.Conflicts)))
	}
	if len(report.MissingRefs) > 0 {
		parts = append(parts, fmt.Sprintf("文本引用但图中未识别 %v", report.MissingRefs))
	}
	if report.AbstractFigureProblem != "" {
		parts = append(parts, report.AbstractFigureProblem)
	}
	return strings.Join(parts, "；") + "。"
}
