// Package intentrules provides shared keyword lists and matching utilities
// used by both domains/ (router + legal_intent) and intent/ packages.
// This eliminates the 4-way duplication identified during code review.
package intentrules

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// --- Domain Keywords ---

// PatentKeywords triggers patent domain routing.
var PatentKeywords = []string{
	"专利", "权利要求", "发明", "实用新型", "外观设计",
	"新颖性", "创造性", "实用性", "prior art", "现有技术",
	"patent", "invention", "claim", "IPC", "分类号",
	"pct", "巴黎公约", "优先权",
}

// LegalKeywords triggers legal domain routing.
var LegalKeywords = []string{
	"法律", "法条", "法规", "判例", "判决", "裁定",
	"诉讼", "起诉", "被告", "原告", "法院", "法官",
	"合同", "侵权", "赔偿", "证据", "仲裁",
	"刑法", "民法", "行政法", "公司法", "劳动法",
	"司法解释", "指导性案例",
	"law", "legal", "court", "statute", "regulation",
}

// AssistantKeywords triggers assistant domain routing.
// Note: "分析" is intentionally excluded to avoid conflict with patent/legal.
var AssistantKeywords = []string{
	"查一下", "帮我搜", "搜索", "检索", "查找",
	"起草", "生成", "写一个", "创建", "整理", "导出", "统计",
	"写代码", "实现一个", "调试", "优化", "重构",
	"代码", "编程", "python", "javascript", "go语言",
	"bash", "shell", "命令行", "脚本",
}

// --- Patent Context Signals ---

// PatentContextSignals are domain-specific terms that indicate a patent
// context for disambiguating keywords like "清楚", "不清楚", "不支持".
var PatentContextSignals = []string{
	"权利要求", "专利", "说明书", "对比文件", "技术方案",
	"审查意见", "申请人", "专利权", "申请号", "公开号",
	"独立权利要求", "从属权利要求", "技术特征", "区别特征",
}

// --- Keyword Matching ---

// CountKeywordMatches counts how many keywords from the list appear in
// the (already lowercased) input. Longer keywords are matched first to
// prevent substring false positives — a longer match suppresses shorter
// sub-keywords that are fully contained within it.
func CountKeywordMatches(input string, keywords []string) (int, []string) {
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)
	sort.Slice(sorted, func(i, j int) bool {
		return utf8.RuneCountInString(sorted[i]) > utf8.RuneCountInString(sorted[j])
	})

	count := 0
	var matchedLong []string
	for _, kw := range sorted {
		if !strings.Contains(input, strings.ToLower(kw)) {
			continue
		}
		// Skip if a longer already-matched keyword fully contains this one.
		skip := false
		kwLen := utf8.RuneCountInString(kw)
		for _, ml := range matchedLong {
			mlLen := utf8.RuneCountInString(ml)
			if mlLen > kwLen && strings.Contains(strings.ToLower(ml), strings.ToLower(kw)) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		matchedLong = append(matchedLong, kw)
		count++
	}
	return count, matchedLong
}

// MatchAnyKeyword returns true if any keyword from the list appears in
// the (already lowercased) input. Keywords are lowercased for matching.
func MatchAnyKeyword(input string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(input, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}
