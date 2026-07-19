package loader

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 决定/判决文书元数据提取正则。
var (
	// blockquote 元数据行（(?m) 启用行首行尾匹配）。
	reJTags      = regexp.MustCompile(`(?m)^>\s*\*\*标签[：:]\s*\*\*\s*(.+)$`)
	reJSource    = regexp.MustCompile(`(?m)^>\s*\*\*来源[：:]\s*\*\*\s*(.+)$`)
	reJTechField = regexp.MustCompile(`(?m)^>\s*\*\*技术领域[：:]\s*\*\*\s*(.+)$`)
	reJCoverage  = regexp.MustCompile(`(?m)^>\s*\*\*覆盖决定数[：:]\s*\*\*\s*(.+)$`)
	reJTimeSpan  = regexp.MustCompile(`(?m)^>\s*\*\*时间跨度[：:]\s*\*\*\s*(.+)$`)

	// 决定要点区块（(?m) 启用行首行尾匹配）。
	reJPointTitle = regexp.MustCompile(`(?m)^###\s*要点(\d+)[：:]\s*(.+)$`)

	// 引用行：*引用：* "..."（第XXXXX号决定）。
	reJCitation  = regexp.MustCompile(`\*引用[：:]\*\s*"(.+?)"\s*（第(\d+)号决定）`)
	reJDecNumber = regexp.MustCompile(`第(\d+)号决定`)

	// 案号模式：如 "（2023）最高法知民终123号"（数字前无"第"）
	reJCaseNumber = regexp.MustCompile(`（(\d+)）([^）]+?)(\d+)号`)

	// H1 标题提取。
	reJH1 = regexp.MustCompile(`(?m)^#\s+(.+)$`)
)

// JudgmentDoc 表示一份结构化判决/决定分析文档。
//
// Wiki 中的"裁判规则分析"类文档是经人工/LLM 从大量决定书中
// 提取的规则摘要，而非决定书全文。每个文档覆盖一个技术
// 领域或法律主题，包含多个决定要点。
type JudgmentDoc struct {
	DocID         string          // 文档 ID
	Title         string          // H1 标题
	DocType       string          // reexam(复审无效) / judgment(判决) / infringement(侵权)
	Domain        string          // 领域标签（如 "创造性"、"等同侵权"）
	LawRefs       []string        // 引用的法条列表
	Source        string          // 来源描述
	TechField     string          // 技术领域
	DecisionCount int             // 覆盖决定数
	TimeSpan      string          // 时间跨度
	Tags          []string        // 其他标签
	DecPoints     []DecisionPoint // 决定要点列表
	CoreSummary   string          // 核心要点
	Content       string          // 全文
}

// DecisionPoint 是一个决定要点条目。
type DecisionPoint struct {
	Index     int        // 序号
	Title     string     // 要点标题
	Content   string     // 要点描述
	Citations []Citation // 引用
}

// Citation 是一条引用信息。
type Citation struct {
	Quote     string // 引用原文
	DecNumber int    // 决定号
	FullRef   string // 完整引用串
}

// ParseJudgmentDoc 解析一个判决/决定分析文档的 Markdown 内容。
func ParseJudgmentDoc(docID, content string) (*JudgmentDoc, error) {
	jd := &JudgmentDoc{
		DocID: docID,
	}

	jd.Title = extractJudgmentTitle(content)
	jd.Tags = extractMetadataTags(content)
	jd.Source = extractMetadataLine(content, reJSource)
	jd.LawRefs = extractLawRefs(content)
	jd.TechField = extractMetadataLine(content, reJTechField)

	// 覆盖决定数。
	if m := reJCoverage.FindStringSubmatch(content); len(m) >= 2 {
		// m[1] = "11 件" 或 "11件"，需要从中提取数字。
		cleaned := strings.TrimSpace(m[1])
		cleaned = strings.TrimSuffix(cleaned, "件")
		cleaned = strings.TrimSpace(cleaned)
		digitsRe := regexp.MustCompile(`\d+`)
		if digits := digitsRe.FindString(cleaned); digits != "" {
			if n, err := strconv.Atoi(digits); err == nil {
				jd.DecisionCount = n
			}
		}
	}
	jd.TimeSpan = extractMetadataLine(content, reJTimeSpan)

	// 核心要点。
	jd.CoreSummary = extractCoreSummary(content)

	// 决定要点。
	jd.DecPoints = extractDecisionPoints(content)

	// 全文保留。
	jd.Content = content

	return jd, nil
}

// extractJudgmentTitle 提取 H1 标题。
func extractJudgmentTitle(content string) string {
	if m := reJH1.FindStringSubmatch(content); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractMetadataTags 提取标签元数据。
func extractMetadataTags(content string) []string {
	if m := reJTags.FindStringSubmatch(content); len(m) >= 2 {
		tags := strings.TrimSpace(m[1])
		var result []string
		for _, tag := range strings.Split(tags, "；") {
			tag = strings.TrimSpace(tag)
			// 去掉 "主题=" "子主题=" "知识点=" 前缀。
			if idx := strings.Index(tag, "="); idx >= 0 {
				tag = strings.TrimSpace(tag[idx+1:])
			}
			if tag != "" {
				result = append(result, tag)
			}
		}
		return result
	}
	return nil
}

// extractMetadataLine 提取单行元数据。
func extractMetadataLine(content string, re *regexp.Regexp) string {
	if m := re.FindStringSubmatch(content); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractCoreSummary 提取核心要点摘要（## 核心要点 后的段落）。
func extractCoreSummary(content string) string {
	reSummary := regexp.MustCompile(`##\s*核心要点\s*\n([\s\S]*?)(?:\n##|\z)`)
	if m := reSummary.FindStringSubmatch(content); len(m) >= 2 {
		text := strings.TrimSpace(m[1])
		// 取第一个段落（空行分隔）。
		if idx := strings.Index(text, "\n\n"); idx >= 0 {
			text = strings.TrimSpace(text[:idx])
		}
		return text
	}
	return ""
}

// extractDecisionPoints 提取所有决定要点。
func extractDecisionPoints(content string) []DecisionPoint {
	sectionContent := extractSectionContent(content, "要点")
	if sectionContent == "" {
		return nil
	}

	lines := strings.Split(sectionContent, "\n")
	var points []DecisionPoint
	var current *DecisionPoint
	var bodyBuf strings.Builder
	inPoint := false

	flushPoint := func() {
		if current != nil {
			current.Content = strings.TrimSpace(bodyBuf.String())
			current.Citations = extractPointCitations(current.Content)
			points = append(points, *current)
			current = nil
		}
		bodyBuf.Reset()
		inPoint = false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := reJPointTitle.FindStringSubmatch(trimmed); m != nil {
			flushPoint()
			idx, _ := strconv.Atoi(m[1])
			current = &DecisionPoint{
				Index: idx,
				Title: strings.TrimSpace(m[2]),
			}
			inPoint = true
			continue
		}
		if inPoint && current != nil {
			if bodyBuf.Len() > 0 {
				bodyBuf.WriteByte('\n')
			}
			bodyBuf.WriteString(line)
		}
	}
	flushPoint()

	return points
}

// extractPointCitations 从决定要点内容中提取引用信息。
func extractPointCitations(content string) []Citation {
	matches := reJCitation.FindAllStringSubmatch(content, -1)
	var citations []Citation
	seen := make(map[string]bool)
	for _, m := range matches {
		quote := strings.TrimSpace(m[1])
		decNum, _ := strconv.Atoi(m[2])
		fullRef := fmt.Sprintf("第%d号决定", decNum)
		if !seen[fullRef] {
			seen[fullRef] = true
			citations = append(citations, Citation{
				Quote:     quote,
				DecNumber: decNum,
				FullRef:   fullRef,
			})
		}
	}
	return citations
}

// ExtractDecNumbers 从全文提取所有引用的决定号（去重）。
func ExtractDecNumbers(content string) []int {
	matches := reJDecNumber.FindAllStringSubmatch(content, -1)
	seen := make(map[int]bool)
	var nums []int
	for _, m := range matches {
		n, _ := strconv.Atoi(m[1])
		if !seen[n] {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	return nums
}

// ExtractCaseNumbers 从全文提取所有案号。
func ExtractCaseNumbers(content string) []string {
	matches := reJCaseNumber.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var cases []string
	for _, m := range matches {
		courtPart := strings.TrimSuffix(m[2], "第")
		caseStr := fmt.Sprintf("（%s）%s第%s号", m[1], courtPart, m[3])
		if !seen[caseStr] {
			seen[caseStr] = true
			cases = append(cases, caseStr)
		}
	}
	return cases
}

// extractSectionContent 提取 "## 标题" 到下一个 "##" 或文件结尾之间的内容。
// 不使用 regex lookahead（Go regexp 不支持），改为行级解析。
func extractSectionContent(content, sectionName string) string {
	lines := strings.Split(content, "\n")
	startIdx := -1
	sectionMarker := "## " + sectionName
	for i, line := range lines {
		if strings.TrimSpace(line) == sectionMarker {
			startIdx = i + 1
			break
		}
	}
	// 如果是 "决定要点" 也匹配。
	if startIdx < 0 {
		altMarker := "## 决定要点"
		for i, line := range lines {
			if strings.TrimSpace(line) == altMarker {
				startIdx = i + 1
				break
			}
		}
	}
	if startIdx < 0 {
		return ""
	}

	var result []string
	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		result = append(result, lines[i])
	}
	return strings.Join(result, "\n")
}
