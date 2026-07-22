package domains

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExtractedCaseInfo 是从权威文档中提取的结构化案件信息。
// 各字段按文档类型分批填充，不需要一次性完整。
type ExtractedCaseInfo struct {
	SourceDocType string `json:"source_doc_type"`

	// --- 来自确认书 ---
	ClientName  string `json:"client_name,omitempty"`
	PatentTitle string `json:"patent_title,omitempty"`
	PatentType  string `json:"patent_type,omitempty"`
	Inventors   string `json:"inventors,omitempty"`
	Year        int    `json:"year,omitempty"`

	// --- 来自受理通知书 ---
	FilingNumber string `json:"filing_number,omitempty"`
	FilingDate   string `json:"filing_date,omitempty"`

	// --- 来自公开公告 ---
	PublicationNumber string `json:"publication_number,omitempty"`

	// --- 来自审查意见 ---
	OaRejectionType string `json:"oa_rejection_type,omitempty"`

	// --- 来自申请文件 ---
	ClaimCount       int    `json:"claim_count,omitempty"`
	IndependentCount int    `json:"independent_count,omitempty"`
	Abstract         string `json:"abstract,omitempty"`
}

// 专利号正则
var (
	// 中国申请号：13位数字或 13位数字.校验位，如 202410123456.7
	appNumRe1 = regexp.MustCompile(`(\d{13})\.?\d?`)
	// 中国专利公开号：CN + 9位数字 + 字母，如 CN117890001A
	pubNumRe = regexp.MustCompile(`CN\d{9}[A-Z]`)
	// PCT 申请号：PCT/CN2024/123456
	pctRe = regexp.MustCompile(`PCT/[A-Z]{2}\d{4}/\d{6}`)
	// 专利类型
	typeInventionRe = regexp.MustCompile(`发明专利`)
	typeUtilityRe   = regexp.MustCompile(`实用新型`)
	typeDesignRe    = regexp.MustCompile(`外观设计|外观专利`)
	// 年份
	yearRe = regexp.MustCompile(`(20\d{2})年`)
	// 客户/申请人标签后内容
	applicantLabelRe = regexp.MustCompile(`(?:申请人|客户|申请单位|代理机构客户)\s*[:：]\s*(.+)`)
	// 发明/专利名称标签后内容
	titleLabelRe = regexp.MustCompile(`(?:发明名称|专利名称|申请名称|案件名称)\s*[:：]\s*(.+)`)
	// 发明人
	inventorLabelRe = regexp.MustCompile(`(?:发明人|设计人)\s*[:：]\s*(.+)`)
	// 申请日
	filingDateRe = regexp.MustCompile(`(?:申请日|申请日期)\s*[:：]?\s*(\d{4})年(\d{1,2})月(\d{1,2})日`)
	// 权利要求数量
	claimCountRe = regexp.MustCompile(`共\s*(\d+)\s*项权利要求|权利要求\s*(\d+)\s*项`)
	indClaimRe   = regexp.MustCompile(`(?m)^\d+[\.、]\s`)
	// 摘要
	abstractLabelRe = regexp.MustCompile(`(?s)摘\s*要\s*\n(.+?)(?:\n\s*\n|\n附图|$)`)
)

// ExtractFromConfirmation 从专利申请确认书文本提取案件信息。
func ExtractFromConfirmation(text string) ExtractedCaseInfo {
	info := ExtractedCaseInfo{SourceDocType: DocConfirmation}

	if m := applicantLabelRe.FindStringSubmatch(text); len(m) > 1 {
		info.ClientName = cleanExtractedValue(m[1])
	}
	if m := titleLabelRe.FindStringSubmatch(text); len(m) > 1 {
		info.PatentTitle = cleanExtractedValue(m[1])
	}
	if m := inventorLabelRe.FindStringSubmatch(text); len(m) > 1 {
		info.Inventors = cleanExtractedValue(m[1])
	}
	info.PatentType = detectPatentType(text)
	info.Year = detectYear(text)

	return info
}

// ExtractFromFilingDoc 从申请文件（定稿）提取补充信息。
func ExtractFromFilingDoc(text string) ExtractedCaseInfo {
	info := ExtractedCaseInfo{SourceDocType: DocFiling}

	// 提取标题（权利要求书第一行或说明书首行常为发明名称）
	if m := titleLabelRe.FindStringSubmatch(text); len(m) > 1 {
		info.PatentTitle = cleanExtractedValue(m[1])
	}

	// 权利要求数量
	if m := claimCountRe.FindStringSubmatch(text); len(m) > 1 {
		if n, err := strconv.Atoi(m[1]); err == nil {
			info.ClaimCount = n
		}
	} else if m := claimCountRe.FindStringSubmatch(text); len(m) > 2 {
		if n, err := strconv.Atoi(m[2]); err == nil {
			info.ClaimCount = n
		}
	}
	// 独立权利要求（粗略统计）
	info.IndependentCount = len(indClaimRe.FindAllString(text, -1))

	// 摘要
	if m := abstractLabelRe.FindStringSubmatch(text); len(m) > 1 {
		abs := strings.TrimSpace(m[1])
		if len(abs) > 300 {
			abs = abs[:300] + "..."
		}
		info.Abstract = abs
	}

	return info
}

// ExtractFromAcceptance 从受理通知书提取申请号和申请日。
func ExtractFromAcceptance(text string) ExtractedCaseInfo {
	info := ExtractedCaseInfo{SourceDocType: DocAcceptance}

	// 申请号优先匹配 PCT，再匹配中国格式
	if m := pctRe.FindString(text); m != "" {
		info.FilingNumber = m
	} else if m := appNumRe1.FindString(text); m != "" {
		info.FilingNumber = m
	}

	// 申请日
	if m := filingDateRe.FindStringSubmatch(text); len(m) > 3 {
		info.FilingDate = fmt.Sprintf("%s-%s-%s", m[1], padMonthDay(m[2]), padMonthDay(m[3]))
	}

	return info
}

// ExtractFromPublication 从公开公告提取公开号。
func ExtractFromPublication(text string) ExtractedCaseInfo {
	info := ExtractedCaseInfo{SourceDocType: DocPublication}

	if m := pubNumRe.FindString(text); m != "" {
		info.PublicationNumber = m
	}

	return info
}

// ExtractFromOfficeAction 从审查意见提取法律状态信息。
func ExtractFromOfficeAction(text string) ExtractedCaseInfo {
	info := ExtractedCaseInfo{SourceDocType: DocOfficeAction}

	// 复用 oa_parser 的类型检测逻辑（通过导入间接调用）
	// 这里用内联关键词匹配避免跨包依赖
	info.OaRejectionType = detectOaType(text)

	// 尝试提取申请号
	if m := appNumRe1.FindString(text); m != "" {
		info.FilingNumber = m
	}

	return info
}

// ExtractFromGrant 从授权通知书提取信息。
func ExtractFromGrant(text string) ExtractedCaseInfo {
	info := ExtractedCaseInfo{SourceDocType: DocGrant}

	if m := pubNumRe.FindString(text); m != "" {
		info.PublicationNumber = m
	}

	return info
}

// MergeExtractions 将多次提取的信息合并，后提取的非空字段覆盖先前的。
func MergeExtractions(base ExtractedCaseInfo, updates ...ExtractedCaseInfo) ExtractedCaseInfo {
	result := base
	for _, u := range updates {
		if u.ClientName != "" {
			result.ClientName = u.ClientName
		}
		if u.PatentTitle != "" {
			result.PatentTitle = u.PatentTitle
		}
		if u.PatentType != "" {
			result.PatentType = u.PatentType
		}
		if u.Inventors != "" {
			result.Inventors = u.Inventors
		}
		if u.Year > 0 {
			result.Year = u.Year
		}
		if u.FilingNumber != "" {
			result.FilingNumber = u.FilingNumber
		}
		if u.FilingDate != "" {
			result.FilingDate = u.FilingDate
		}
		if u.PublicationNumber != "" {
			result.PublicationNumber = u.PublicationNumber
		}
		if u.OaRejectionType != "" {
			result.OaRejectionType = u.OaRejectionType
		}
		if u.ClaimCount > 0 {
			result.ClaimCount = u.ClaimCount
		}
		if u.IndependentCount > 0 {
			result.IndependentCount = u.IndependentCount
		}
		if u.Abstract != "" {
			result.Abstract = u.Abstract
		}
	}
	return result
}

// ExtractFromText 根据文档类型自动选择提取器。
func ExtractFromText(docType, text string) ExtractedCaseInfo {
	switch docType {
	case DocConfirmation:
		return ExtractFromConfirmation(text)
	case DocFiling:
		return ExtractFromFilingDoc(text)
	case DocAcceptance:
		return ExtractFromAcceptance(text)
	case DocPublication:
		return ExtractFromPublication(text)
	case DocOfficeAction:
		return ExtractFromOfficeAction(text)
	case DocGrant:
		return ExtractFromGrant(text)
	default:
		return ExtractedCaseInfo{SourceDocType: docType}
	}
}

// --- 内部辅助 ---

func detectPatentType(text string) string {
	if typeDesignRe.MatchString(text) {
		return "外观设计"
	}
	if typeUtilityRe.MatchString(text) {
		return "实用新型"
	}
	if typeInventionRe.MatchString(text) {
		return "发明专利"
	}
	return ""
}

func detectYear(text string) int {
	if m := yearRe.FindStringSubmatch(text); len(m) > 1 {
		if y, err := strconv.Atoi(m[1]); err == nil && y >= 2000 && y <= time.Now().Year() {
			return y
		}
	}
	return time.Now().Year()
}

func detectOaType(text string) string {
	t := strings.ToLower(text)
	patterns := []struct {
		typ   string
		words []string
	}{
		{"inventiveness", []string{"创造性", "显而易见", "22条第3款"}},
		{"novelty", []string{"新颖性", "22条第2款"}},
		{"clarity", []string{"不清楚", "26条第4款"}},
		{"support", []string{"不支持", "得不到说明书支持"}},
		{"disclosure", []string{"公开不充分", "26条第3款"}},
		{"scope", []string{"保护范围", "33条", "修改超范围"}},
	}
	for _, p := range patterns {
		for _, w := range p.words {
			if strings.Contains(t, strings.ToLower(w)) {
				return p.typ
			}
		}
	}
	return "other"
}

func cleanExtractedValue(s string) string {
	s = strings.TrimSpace(s)
	// 去除尾部换行后的其他字段
	s = strings.SplitN(s, "\n", 2)[0]
	// 去除常见分隔符尾部
	s = strings.TrimRight(s, "；;。.,，")
	return strings.TrimSpace(s)
}

func padMonthDay(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
