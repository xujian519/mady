package ipc

import (
	"math"
	"strings"
	"unicode/utf8"
)

// minConfidenceForKeyword 是关键词匹配触发的最小置信度。
const minConfidenceForKeyword = 0.50

// highConfidenceThreshold 是阈值，超过此值视为高置信度匹配。
const highConfidenceThreshold = 0.80

// confidenceSaturationK 是置信度饱和常数：命中数达到 K 个不同关键词时
// 置信度约为 0.5+0.5*(1-1/e)≈0.82，恰好越过高置信阈值。置信度只依赖
// 绝对命中数，与各部关键词表长度无关——否则关键词表小的大类会被少量
// 命中虚高（占比偏差）。
const confidenceSaturationK = 8.0

// Classify 从专利文本中识别 IPC 大类，返回 (IPCSection, 置信度)。
//
// 实现方式：
//  1. 关键词规则匹配——扫描文本中出现的各 IPC 大类的关键词，计算匹配得分
//  2. 无匹配时返回默认 IPCB（作业/运输）和低置信度 0.15
//
// 置信度计算（饱和函数校准）：
//   - confidence = 0.50 + 0.5*(1-e^(-hits/K))，hits 为该大类命中的不同关键词数
//   - 置信度只依赖绝对命中数，消除关键词表长度差异带来的占比偏差
//   - 如果多个大类匹配，选择命中数最高者
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
		section IPCSection
		score   int
		matched []string
	}

	var results []sectionScore
	for _, domain := range AllDomains {
		if len(domain.Keywords) == 0 {
			continue
		}
		matched := matchKeywordsInText(text, domain.Keywords)
		if len(matched) > 0 {
			results = append(results, sectionScore{
				section: domain.Section,
				score:   len(matched),
				matched: matched,
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

	// 置信度计算：饱和函数只依赖绝对命中数（见 confidenceSaturationK 注释）
	confidence := minConfidenceForKeyword + 0.5*(1.0-math.Exp(-float64(best.score)/confidenceSaturationK))
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
