package rules

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SlopGroup categorizes a phrase-level slop rule.
type SlopGroup string

const (
	GroupFiller      SlopGroup = "filler"
	GroupQualifier   SlopGroup = "qualifier"
	GroupPassive     SlopGroup = "passive"
	GroupMeta        SlopGroup = "meta"
	GroupAdvisory    SlopGroup = "advisory"
	GroupSearch      SlopGroup = "search"
	GroupIntimacy    SlopGroup = "intimacy"
	GroupSubjectless SlopGroup = "subjectless"
)

// SlopRule is one phrase-level replacement rule.
type SlopRule struct {
	Pattern     *regexp.Regexp
	Replacement string
	Label       string
	Group       SlopGroup
}

// SlopChange records a single phrase-level edit.
type SlopChange struct {
	Original    string
	Replacement string
	Group       string
	Line        int
}

// StructureIssueType classifies a structural-level defect.
type StructureIssueType string

const (
	IssueEmptyThreeStep StructureIssueType = "empty_three_step"
	IssueFakeComparison StructureIssueType = "fake_comparison"
	IssueBinaryTurn     StructureIssueType = "binary_turn"
	IssueReasonPile     StructureIssueType = "reason_pile"
	IssuePassiveVoice   StructureIssueType = "passive_voice"
	IssueOaFormula      StructureIssueType = "oa_formula"
)

// StructureIssue records a structural defect found at a specific line.
type StructureIssue struct {
	Type       StructureIssueType
	Line       int
	Text       string
	Suggestion string
}

// SlopScore is the 50-point 5-dimension score.
type SlopScore struct {
	Directness   int
	Evidence     int
	Rhythm       int
	Practicality int
	Concision    int
	Total        int
	Passed       bool
}

// ChecklistItem is one pre-delivery quick-check entry.
type ChecklistItem struct {
	Question string
	Passed   bool
	Detail   string
}

// SlopAnalysis aggregates all three layers of analysis output.
type SlopAnalysis struct {
	Cleaned   string
	Changes   []SlopChange
	Issues    []StructureIssue
	Score     SlopScore
	Checklist []ChecklistItem
}

var groupLabels = map[SlopGroup]string{
	GroupFiller:      "填充废词",
	GroupQualifier:   "空泛修饰",
	GroupMeta:        "元叙述",
	GroupIntimacy:    "虚假亲密",
	GroupSubjectless: "虚假主体",
	GroupSearch:      "检索空话",
	GroupAdvisory:    "免责堆叠",
}

var typeLabels = map[StructureIssueType]string{
	IssueEmptyThreeStep: "假三步法",
	IssueFakeComparison: "假对比表",
	IssueBinaryTurn:     "假转折",
	IssueReasonPile:     "理由堆砌",
	IssuePassiveVoice:   "被动语态",
	IssueOaFormula:      "意见陈述公式",
}

var phraseRules = func() []SlopRule {
	mk := func(pattern, replacement, label string, group SlopGroup) SlopRule {
		return SlopRule{
			Pattern:     regexp.MustCompile(pattern),
			Replacement: replacement,
			Label:       label,
			Group:       group,
		}
	}
	return []SlopRule{
		mk(`进一步地[，,]?`, "", "填充词「进一步地」", GroupFiller),
		mk(`此外[，,]?`, "", "填充词「此外」", GroupFiller),
		mk(`值得一提的是[，,]?`, "", "填充词「值得一提的是」", GroupFiller),
		mk(`不难发现[，,]?`, "", "填充词「不难发现」", GroupFiller),
		mk(`毋庸置疑[，,]?`, "", "填充词「毋庸置疑」", GroupFiller),
		mk(`需要指出的是[，,]?`, "", "填充词「需要指出的是」", GroupFiller),
		mk(`综上所述[，,]?`, "", "填充词「综上所述」", GroupFiller),
		mk(`诚如前述[，,]?`, "", "填充词「诚如前述」", GroupFiller),
		mk(`显而易见地[，,]?`, "", "空泛「显而易见地」", GroupQualifier),
		mk(`本领域技术人员能够理解[，,]?`, "", "空泛「本领域技术人员能够理解」", GroupQualifier),
		mk(`创造性得以确立[。.]?`, "", "空泛「创造性得以确立」", GroupQualifier),
		mk(`保护范围合理[。.]?`, "", "空泛「保护范围合理」", GroupQualifier),
		mk(`具有显著进步[。.]?`, "", "空泛「具有显著进步」", GroupQualifier),
		mk(`质的飞跃[。.]?`, "", "空泛「质的飞跃」", GroupQualifier),
		mk(`革命性`, "", "夸张修饰「革命性」", GroupQualifier),
		mk(`颠覆性`, "", "夸张修饰「颠覆性」", GroupQualifier),
		mk(`深入[地]?分析`, "分析", "空洞修饰「深入分析」", GroupQualifier),
		mk(`全面[地]?论述`, "论述", "空洞修饰「全面论述」", GroupQualifier),
		mk(`系统[地]?(分析|论述|阐述)`, "$1", "空洞修饰「系统化」", GroupQualifier),
		mk(`一体化`, "", "空洞修饰「一体化」", GroupQualifier),
		mk(`新颖性是指[^，。]*[，。]?`, "", "科普腔「新颖性定义」", GroupQualifier),
		mk(`创造性判断(通常)?采用三步法[，。]?`, "", "科普腔「三步法介绍」", GroupQualifier),
		mk(`正如大家所知[，,]?`, "", "顾问腔「正如大家所知」", GroupQualifier),
		mk(`下文将分[一二三四五六七八九十]+部分[^，。]*[，。]?`, "", "元叙述「下文将分部分」", GroupMeta),
		mk(`首先分析[^，。]*，再分析[^，。]*[。]?`, "", "元叙述「首先…再…」", GroupMeta),
		mk(`综上所述[，]?`, "", "元叙述「综上所述」", GroupMeta),
		mk(`恕我直言[，,]?`, "", "虚假亲密「恕我直言」", GroupIntimacy),
		mk(`请允许我指出[，,]?`, "", "虚假亲密「请允许我指出」", GroupIntimacy),
		mk(`这是一个值得深思的问题[。.]?`, "", "表演强调「值得深思的问题」", GroupIntimacy),
		mk(`创造性障碍得以克服[。.]?`, "", "无主句「创造性障碍得以克服」", GroupSubjectless),
		mk(`审查意见所指缺陷得以消除[。.]?`, "", "无主句「缺陷得以消除」", GroupSubjectless),
		mk(`现有技术给出了启示[。.]?`, "", "无主句「现有技术给出了启示」", GroupSubjectless),
		mk(`创造性得以认可[。.]?`, "", "无主句「创造性得以认可」", GroupSubjectless),
		mk(`保护范围得以[^，。]*界定[。.]?`, "", "无主句「保护范围得以…界定」", GroupSubjectless),
		mk(`审查实践认为[^，。]*[。.]?`, "", "无主句「审查实践认为」", GroupSubjectless),
		mk(`检索范围广泛[，,]?结果丰富[。.]?`, "", "检索空话「范围广泛结果丰富」", GroupSearch),
		mk(`高相关文献若干[。.]?`, "", "检索空话「高相关文献若干」", GroupSearch),
		mk(`建议继续检索[。.]?`, "", "检索空话「建议继续检索」", GroupSearch),
		mk(`以上分析仅供参考[，,]不构成法律意见`, "以上分析仅供参考。", "免责堆叠「仅供参考不构成法律意见」", GroupAdvisory),
		mk(`存在不确定性[，,]结果可能因审查实践而异`, "", "免责堆叠「存在不确定性」", GroupAdvisory),
		mk(`[ \t]{2,}`, " ", "多余空格", GroupFiller),
		mk(`\n{3,}`, "\n\n", "多余空行", GroupFiller),
	}
}()

var (
	reEmptyThreeStep  = regexp.MustCompile(`区别特征`)
	reColonNoEvidence = regexp.MustCompile(`[：:][^D\d]`)
	reFakeComparison  = regexp.MustCompile(`\|.*特征.*\|`)
	reHasParagraph    = regexp.MustCompile(`¶\d{3,4}`)
	reHasYearRef      = regexp.MustCompile(`\[\d{4}\]`)
	reBinaryTurn      = regexp.MustCompile(`不是[^，]{2,20}问题[，,]\s*而是`)
	rePassiveVoice    = regexp.MustCompile(`被(驳回|认定为|视为|公开).*[。.]`)
	rePassivePrefix   = regexp.MustCompile(`^(申请人|审查员|法院|D\d)`)
	reOaFormula       = regexp.MustCompile(`(审查员认定有误|审查员之意见|请审查员重新考虑)`)
	reReasonPile      = regexp.MustCompile(`第[\d一二三四五六七八九十]+条|专利法第[\d]+条`)

	reMainPoint    = regexp.MustCompile(`(争点|问题|争议|焦点|核心)`)
	reEvidence     = regexp.MustCompile(`(D\d|权[\d]|第[\d]条|段落)`)
	reParagraphRef = regexp.MustCompile(`¶\d{3,4}`)
	reExaggeration = regexp.MustCompile(`(显著|突出|质的飞跃|革命性|颠覆性)`)
	reSearchTerms  = regexp.MustCompile(`(去重|命中|数据库)`)
)

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func runeSlice(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func detectStructureIssues(text string) []StructureIssue {
	lines := strings.Split(text, "\n")
	var issues []StructureIssue

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if reEmptyThreeStep.MatchString(line) && (runeLen(line) < 30 || reColonNoEvidence.MatchString(line)) {
			issues = append(issues, StructureIssue{
				Type:       IssueEmptyThreeStep,
				Line:       i + 1,
				Text:       runeSlice(trimmed, 80),
				Suggestion: "区别特征应指向对比文件具体段落：D1¶[段落号]：特征详情",
			})
		}

		if reFakeComparison.MatchString(line) && !reHasParagraph.MatchString(line) && !reHasYearRef.MatchString(line) {
			issues = append(issues, StructureIssue{
				Type:       IssueFakeComparison,
				Line:       i + 1,
				Text:       runeSlice(trimmed, 80),
				Suggestion: "对比表每格应含对比文件段落号：D1¶0123",
			})
		}

		if reBinaryTurn.MatchString(line) {
			issues = append(issues, StructureIssue{
				Type:       IssueBinaryTurn,
				Line:       i + 1,
				Text:       runeSlice(trimmed, 80),
				Suggestion: "直接写结论及证据，不用「不是X而是Y」转折",
			})
		}

		if rePassiveVoice.MatchString(line) && !rePassivePrefix.MatchString(strings.TrimSpace(line)) {
			issues = append(issues, StructureIssue{
				Type:       IssuePassiveVoice,
				Line:       i + 1,
				Text:       runeSlice(trimmed, 80),
				Suggestion: "写明主体：审查员驳回… / 对比文件D1公开…",
			})
		}

		if reOaFormula.MatchString(line) {
			issues = append(issues, StructureIssue{
				Type:       IssueOaFormula,
				Line:       i + 1,
				Text:       runeSlice(trimmed, 80),
				Suggestion: "直接写争点+法条+对比文件段落+修改对照",
			})
		}
	}

	reasons := reReasonPile.FindAllString(text, -1)
	if len(reasons) >= 4 {
		issues = append(issues, StructureIssue{
			Type:       IssueReasonPile,
			Line:       1,
			Text:       fmt.Sprintf("全文含 %d 条理由", len(reasons)),
			Suggestion: "主理由至多2条，其余删除或并入脚注",
		})
	}

	return issues
}

func scoreRhythm(issueCount int) int {
	if issueCount > 3 {
		return 4
	}
	if issueCount > 1 {
		return 6
	}
	return 8
}

func scorePracticality(changeCount int) int {
	if changeCount > 5 {
		return 5
	}
	if changeCount > 2 {
		return 7
	}
	return 9
}

func scoreConcision(paragraphCount int) int {
	if paragraphCount > 15 {
		return 5
	}
	return 8
}

func scoreDocument(text string, changes []SlopChange, issues []StructureIssue) SlopScore {
	lines := strings.Split(text, "\n")
	firstPara := strings.Join(lines[:min(10, len(lines))], "")
	firstPara = runeSlice(firstPara, 200)

	directness := 4
	if reMainPoint.MatchString(firstPara) {
		directness = 8
	}

	evidenceScore := 5
	if reEvidence.MatchString(text) {
		evidenceScore += 2
	}
	if reParagraphRef.MatchString(text) {
		evidenceScore += 2
	}
	if len(strings.Split(text, "\n\n")) < 3 {
		evidenceScore++
	}

	rhythmScore := scoreRhythm(len(issues))
	practicalScore := scorePracticality(len(changes))
	concisionScore := scoreConcision(len(strings.Split(text, "\n\n")))
	total := directness + evidenceScore + rhythmScore + practicalScore + concisionScore

	return SlopScore{
		Directness:   directness,
		Evidence:     evidenceScore,
		Rhythm:       rhythmScore,
		Practicality: practicalScore,
		Concision:    concisionScore,
		Total:        total,
		Passed:       total >= 35,
	}
}

func buildIssueDetail(issues []StructureIssue, typ StructureIssueType, label string) string {
	count := 0
	for _, iss := range issues {
		if iss.Type == typ {
			count++
		}
	}
	if count > 0 {
		return fmt.Sprintf("%d 处%s", count, label)
	}
	return "无"
}

func runChecklist(text string, issues []StructureIssue) []ChecklistItem {
	snippet := runeSlice(text, 500)

	hasFeature := strings.Contains(snippet, "特征")
	hasEvidenceMarker := strings.Contains(snippet, "权") ||
		regexp.MustCompile(`D\d`).MatchString(snippet) ||
		strings.Contains(snippet, "¶")
	featurePassed := !hasFeature || hasEvidenceMarker

	return []ChecklistItem{
		{
			Question: "有无未指向权项号或对比文件段落的特征论述？",
			Passed:   featurePassed,
			Detail:   "检查前500字中孤立出现的「特征」",
		},
		{
			Question: "有无被动句隐藏主体（「被认为」「得以」「得以克服」）？",
			Passed:   !hasIssueType(issues, IssuePassiveVoice),
			Detail:   buildIssueDetail(issues, IssuePassiveVoice, "被动句"),
		},
		{
			Question: "创造性段落是否仅有「区别特征+技术问题+显而易见」标题而无D1映射？",
			Passed:   !hasIssueType(issues, IssueEmptyThreeStep),
			Detail:   buildIssueDetail(issues, IssueEmptyThreeStep, "假三步法"),
		},
		{
			Question: "是否出现「不是X问题，而是Y问题」式假转折？",
			Passed:   !hasIssueType(issues, IssueBinaryTurn),
			Detail:   buildIssueDetail(issues, IssueBinaryTurn, "假转折"),
		},
		{
			Question: "无效理由是否超过3条且彼此无优先级？",
			Passed:   !hasIssueType(issues, IssueReasonPile),
			Detail:   buildIssueDetail(issues, IssueReasonPile, "理由超量"),
		},
		{
			Question: "是否用「显著」「突出」「质的飞跃」而无实验数据或对比效果？",
			Passed:   !reExaggeration.MatchString(text),
			Detail:   "含无数据夸张修饰",
		},
		{
			Question: "说明书「技术效果」是否与权利要求特征逐项对应？",
			Passed:   true,
			Detail:   "需人工确认",
		},
		{
			Question: "检索报告是否有命中逻辑与去重说明？",
			Passed:   reSearchTerms.MatchString(text) || runeLen(text) < 500,
			Detail: func() string {
				if reSearchTerms.MatchString(text) {
					return "有"
				}
				return "N/A（非检索报告）"
			}(),
		},
	}
}

func hasIssueType(issues []StructureIssue, typ StructureIssueType) bool {
	for _, iss := range issues {
		if iss.Type == typ {
			return true
		}
	}
	return false
}

// AnalyzeSlop runs all three analysis layers on the input text.
func AnalyzeSlop(text string) SlopAnalysis {
	var changes []SlopChange
	cleaned := text

	for _, rule := range phraseRules {
		before := cleaned
		cleaned = rule.Pattern.ReplaceAllString(cleaned, rule.Replacement)
		if cleaned != before {
			repl := rule.Replacement
			if len(repl) == 0 {
				repl = "（删除）"
			}
			changes = append(changes, SlopChange{
				Original:    rule.Label,
				Replacement: repl,
				Group:       string(rule.Group),
			})
		}
	}

	cleaned = strings.TrimRight(cleaned, " \t\n") + "\n"

	issues := detectStructureIssues(cleaned)
	score := scoreDocument(text, changes, issues)
	checklist := runChecklist(text, issues)

	return SlopAnalysis{
		Cleaned:   cleaned,
		Changes:   changes,
		Issues:    issues,
		Score:     score,
		Checklist: checklist,
	}
}

// FormatSlopAnalysis renders the full analysis as a Markdown report.
func FormatSlopAnalysis(a SlopAnalysis) string {
	var lines []string

	passText := "❌ 需修订"
	if a.Score.Passed {
		passText = "✅ 通过"
	}

	lines = append(lines,
		"## 反套话润色报告\n",
		fmt.Sprintf("**评分：%d/50 (%s)**", a.Score.Total, passText),
		"",
		"| 维度 | 得分 | 满分 |",
		"|------|------|------|",
		fmt.Sprintf("| 争点直陈 | %d | 10 |", a.Score.Directness),
		fmt.Sprintf("| 证据密度 | %d | 10 |", a.Score.Evidence),
		fmt.Sprintf("| 论证节奏 | %d | 10 |", a.Score.Rhythm),
		fmt.Sprintf("| 实务可信 | %d | 10 |", a.Score.Practicality),
		fmt.Sprintf("| 可删减性 | %d | 10 |", a.Score.Concision),
	)

	if len(a.Changes) > 0 {
		lines = append(lines, "", "### 短语删除/替换", "")
		byGroup := groupChanges(a.Changes)
		for _, g := range byGroup {
			lines = append(lines, fmt.Sprintf("**%s（%d处）**", g.label, len(g.items)))
			for _, c := range g.items {
				lines = append(lines, fmt.Sprintf("- %s → %s", c.Original, c.Replacement))
			}
			lines = append(lines, "")
		}
	}

	if len(a.Issues) > 0 {
		lines = append(lines, "### 结构缺陷", "")
		for _, iss := range a.Issues {
			lines = append(lines,
				fmt.Sprintf("- **L%d** [%s] %s", iss.Line, typeLabels[iss.Type], iss.Text),
				fmt.Sprintf("  ↳ %s", iss.Suggestion),
			)
		}
		lines = append(lines, "")
	}

	var failedItems []ChecklistItem
	for _, c := range a.Checklist {
		if !c.Passed {
			failedItems = append(failedItems, c)
		}
	}
	if len(failedItems) > 0 {
		lines = append(lines, "### 未通过快检项", "")
		for _, item := range failedItems {
			lines = append(lines, fmt.Sprintf("- ❌ %s", item.Question))
			if item.Detail != "" {
				lines = append(lines, fmt.Sprintf("  %s", item.Detail))
			}
		}
	}

	return strings.Join(lines, "\n")
}

type changeGroup struct {
	label string
	items []SlopChange
}

func groupChanges(changes []SlopChange) []changeGroup {
	groupOrder := []SlopGroup{
		GroupFiller, GroupQualifier, GroupPassive, GroupMeta,
		GroupAdvisory, GroupSearch, GroupIntimacy, GroupSubjectless,
	}
	groupMap := make(map[SlopGroup][]SlopChange)
	for _, c := range changes {
		groupMap[SlopGroup(c.Group)] = append(groupMap[SlopGroup(c.Group)], c)
	}
	var result []changeGroup
	for _, g := range groupOrder {
		items := groupMap[g]
		if len(items) == 0 {
			continue
		}
		result = append(result, changeGroup{
			label: groupLabels[g],
			items: items,
		})
	}
	return result
}
