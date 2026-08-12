package memory

import (
	"math"
	"strings"
	"time"
	"unicode"
)

// estimateTokens 粗略估计一段文本的 token 数（4 chars/token）。
func estimateTokens(content string) int64 {
	return int64(len([]rune(content)) / 4)
}

// recencyScore 计算新鲜度分。借鉴 CrewAI 指数衰减公式：
//
//	score = 0.5^(age_in_days / halfLifeDays)
func recencyScore(lastAccess time.Time, now time.Time, halfLifeDays float64) float64 {
	age := now.Sub(lastAccess).Hours() / 24 // 天
	if age <= 0 {
		return 1.0
	}
	return math.Pow(0.5, age/halfLifeDays)
}

// --- 关键词检索（Phase 1 Simple Fallback）---

// tokenize 将文本分词为小写词干列表。
// 对 CJK（中日韩）文字做单字拆分，对拉丁文字按空格/标点拆分。
func tokenize(text string) []string {
	var tokens []string
	var buf strings.Builder

	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}

	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			// CJK 字符：每个字独立作为 token
			flush()
			tokens = append(tokens, string(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			buf.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// keywordScore 计算基于关键词匹配的语义相似度估计（0~1）。
// 使用 TF-like 加权：查询词在命中内容中的占比 + 覆盖度。
func keywordScore(query string, content string) float64 {
	if query == "" || content == "" {
		return 0
	}
	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return 0
	}
	cTokens := tokenize(content)
	if len(cTokens) == 0 {
		return 0
	}

	// 建 content 词频 map
	cFreq := make(map[string]int, len(cTokens))
	for _, t := range cTokens {
		cFreq[t]++
	}

	// 统计查询词命中数和权值
	hits := 0
	totalWeight := 0.0
	weightSum := 0.0
	for i, qt := range qTokens {
		w := 1.0 + float64(len(qTokens)-i)/float64(len(qTokens)) // 位置权值：越靠前权重越大
		weightSum += w
		if count, ok := cFreq[qt]; ok && count > 0 {
			hits++
			totalWeight += w
		}
	}

	coverage := float64(hits) / float64(len(qTokens)) // 覆盖度
	weightedScore := totalWeight / weightSum          // 加权匹配度
	return (coverage*0.4+weightedScore*0.6)*0.8 + 0.2 // 映射到 0.2~1.0 区间
}

// --- 复合评分 ---

// computeCompositeScore 计算单条记忆的复合评分。
func (s *InMemoryStore) computeCompositeScore(semantic, importance float64, lastAccess time.Time) float64 {
	now := s.nowTime()
	recency := recencyScore(lastAccess, now, s.scoring.RecencyHalfLife)

	return s.scoring.SemanticWeight*semantic +
		s.scoring.RecencyWeight*recency +
		s.scoring.ImportanceWeight*importance
}

// --- 机器生成的重要性估计（无 LLM 时使用）---

// estimateImportance 基于内容特征估算重要性（0~1）。
// 当 LLM 提取器不可用时作为 fallback。
func estimateImportance(content string) float64 {
	if content == "" {
		return 0
	}

	score := 0.3 // 基础分

	// 长度因子：过短的内容不太重要
	runes := []rune(content)
	lenScore := float64(len(runes)) / 500.0
	if lenScore > 0.4 {
		lenScore = 0.4
	}
	score += lenScore

	// 关键词启发：含有决策/事实等关键词的重要性更高
	importanceKeywords := []string{"决定", "决策", "重要", "关键", "必须", "一定要",
		"prefer", "favorite", "important", "critical", "decision",
		"like", "dislike", "want", "need", "记住", "记住我"}
	lower := strings.ToLower(content)
	for _, kw := range importanceKeywords {
		if strings.Contains(lower, kw) {
			score += 0.05
		}
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}
