package ipc

import (
	"strings"
	"unicode/utf8"
)

// minConfidenceForKeyword 是关键词匹配触发的最小置信度。
const minConfidenceForKeyword = 0.50

// highConfidenceThreshold 是阈值，超过此值视为高置信度匹配。
const highConfidenceThreshold = 0.80

// Classify 从专利文本中识别 IPC 大类，返回 (IPCSection, 置信度)。
//
// 实现方式：
//  1. 关键词规则匹配——扫描文本中出现的各 IPC 大类的关键词，计算匹配得分
//  2. 无匹配时返回默认 IPCB（作业/运输）和低置信度 0.15
//
// 置信度计算：
//   - 匹配到某个大类的关键词占比超过总关键词的 20% 时，置信度 = min(0.50 + 得分 * 0.5, 1.0)
//   - 如果多个大类匹配，选择得分最高者
//   - 得分比例用于区分主分类和副分类
func Classify(text string) (IPCSection, float64) {
	result := ClassifyDetailed(text)
	return result.Section, result.Confidence
}

// ClassifyDetailed 返回详细的分类结果，包含匹配的关键词列表。
func ClassifyDetailed(text string) ClassificationResult {
	text = strings.ToLower(text)
	textLen := utf8.RuneCountInString(text)
	if textLen == 0 {
		return ClassificationResult{
			Section:    IPCB,
			Confidence: 0.15,
		}
	}

	type sectionScore struct {
		section  IPCSection
		score    int
		matched  []string
		maxWords int
	}

	var results []sectionScore
	for _, domain := range AllDomains {
		if len(domain.Keywords) == 0 {
			continue
		}
		matched := matchKeywordsInText(text, domain.Keywords)
		if len(matched) > 0 {
			results = append(results, sectionScore{
				section:  domain.Section,
				score:    len(matched),
				matched:  matched,
				maxWords: len(domain.Keywords),
			})
		}
	}

	if len(results) == 0 {
		return ClassificationResult{
			Section:         IPCB,
			Confidence:      0.15,
			MatchedKeywords: nil,
		}
	}

	// 选择最高分
	best := results[0]
	for _, r := range results[1:] {
		if r.score > best.score {
			best = r
		}
	}

	// 置信度计算
	ratio := float64(best.score) / float64(best.maxWords)
	confidence := minConfidenceForKeyword + ratio*0.5
	if confidence > 1.0 {
		confidence = 1.0
	}

	return ClassificationResult{
		Section:         best.section,
		Confidence:      confidence,
		MatchedKeywords: best.matched,
	}
}

// matchKeywordsInText 返回文本中匹配的所有关键词（不区分大小写）。
func matchKeywordsInText(text string, keywords []string) []string {
	text = strings.ToLower(text)
	var matched []string
	for _, kw := range keywords {
		kw = strings.ToLower(kw)
		if strings.Contains(text, kw) {
			matched = append(matched, kw)
		}
	}
	return matched
}

// IsHighConfidence 判断置信度是否为高。
func IsHighConfidence(confidence float64) bool {
	return confidence >= highConfidenceThreshold
}
