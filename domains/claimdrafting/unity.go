package claimdrafting

import (
	"math"
	"strings"
)

// =============================================================================
// 单一性评分引擎（移植自 Sati assets/patent-rules/unity.yaml 规范）
//
// 依据：专利法第31条第1款、实施细则第43条——一件专利申请应当限于一项发明；
// 多个独立权利要求应包含"相同或相应的特定技术特征"（体现发明对现有技术贡献
// 的技术特征）才满足单一性。
//
// 评分模型（对齐 unity.yaml）：
//   - 特征相似度 = 0.2×字符重叠 + 0.5×Jaccard + 0.3×Bigram 余弦
//   - 相似度 ≥ 0.6 视为存在对应特征（阈值，实施细则第43条）
//   - 综合得分 = 60（基础分） + 40×技术关联度（最弱权利要求对的相似度）
//   - 评级：≥80 良好；≥60 一般；<60（或存在低于阈值的权利要求对）建议分案
// =============================================================================

// 单一性阈值与权重常量（对齐 unity.yaml）。
const (
	unitySimThreshold = 0.6  // 特征对应判定阈值（实施细则第43条）
	unityScoreBase    = 60.0 // 有单一性基础分
	unityScoreBonus   = 40.0 // 技术关联度奖金系数
	unityScoreGood    = 80.0 // 单一性良好阈值
	unityScoreFair    = 60.0 // 单一性一般阈值
	unityCharOverlapW = 0.2  // 字符重叠权重
	unityJaccardW     = 0.5  // Jaccard 权重
	unityBigramW      = 0.3  // Bigram 余弦权重
)

// UnityGrade 单一性评级。
type UnityGrade string

// Unity grade constants.
const (
	UnityGradeGood UnityGrade = "good" // 单一性良好（≥80）
	UnityGradeFair UnityGrade = "fair" // 单一性一般（≥60 且无低相似度对）
	UnityGradePoor UnityGrade = "poor" // 建议分案（存在低于阈值的权利要求对）
)

// UnityVerdict 单一性判定结果。
type UnityVerdict struct {
	HasUnity    bool        `json:"has_unity"`   // 是否满足单一性（无低相似度权利要求对）
	Score       float64     `json:"score"`       // 综合评分 0-100
	Grade       UnityGrade  `json:"grade"`       // 评级
	PairScores  []UnityPair `json:"pair_scores"` // 各独立权利要求对的相似度
	Diagnostics []string    `json:"diagnostics,omitempty"`
}

// UnityPair 独立权利要求对的相似度结果。
type UnityPair struct {
	LeftNumber  int     `json:"left_number"`
	RightNumber int     `json:"right_number"`
	Similarity  float64 `json:"similarity"`
}

// CheckUnity 检查多个独立权利要求之间的单一性。
// 少于 2 个独立权利要求时视为天然满足单一性（评分取良好线）。
func CheckUnity(claims []Claim) UnityVerdict {
	indClaims := make([]Claim, 0, 2)
	for _, c := range claims {
		if c.Kind == "independent" {
			indClaims = append(indClaims, c)
		}
	}

	verdict := UnityVerdict{HasUnity: true, Score: unityScoreGood, Grade: UnityGradeGood}
	if len(indClaims) < 2 {
		return verdict
	}

	texts := make([]string, len(indClaims))
	for i, c := range indClaims {
		texts[i] = c.Preamble + " " + c.Characterized
	}

	// 逐对计算特征相似度；取最弱对作为整体单一性依据（短板原则）。
	minSim := 1.0
	for i := 0; i < len(indClaims); i++ {
		for j := i + 1; j < len(indClaims); j++ {
			sim := unitySimilarity(texts[i], texts[j])
			verdict.PairScores = append(verdict.PairScores, UnityPair{
				LeftNumber:  indClaims[i].Number,
				RightNumber: indClaims[j].Number,
				Similarity:  sim,
			})
			if sim < minSim {
				minSim = sim
			}
		}
	}

	verdict.Score = unityScoreBase + unityScoreBonus*minSim
	if minSim < unitySimThreshold {
		verdict.HasUnity = false
		verdict.Grade = UnityGradePoor
		verdict.Diagnostics = append(verdict.Diagnostics,
			"存在独立权利要求对的特征相似度低于 0.6，未发现相同或相应的特定技术特征，可能不满足单一性要求")
	} else if verdict.Score >= unityScoreGood {
		verdict.Grade = UnityGradeGood
	} else {
		verdict.Grade = UnityGradeFair
	}
	return verdict
}

// unitySimilarity 计算两段权利要求文本的特征相似度（0-1）：
// 0.2×字符重叠 + 0.5×Jaccard + 0.3×Bigram 余弦。
// 比较前先经 normalizeUnityText 清洗，避免"一种/包括/和"等结构样板词稀释技术词汇信号。
func unitySimilarity(a, b string) float64 {
	a = normalizeUnityText(a)
	b = normalizeUnityText(b)
	runesA, runesB := []rune(a), []rune(b)
	if len(runesA) == 0 || len(runesB) == 0 {
		return 0
	}
	return unityCharOverlapW*charOverlap(runesA, runesB) +
		unityJaccardW*jaccardSimilarity(runesA, runesB) +
		unityBigramW*bigramCosine(runesA, runesB)
}

// unityStopWords 结构/语法样板词（不参与"特定技术特征"相似度比较）。
// 注："由…组成"（U+2026 省略号字面量）无法命中真实文本，统一收敛为 "组成"。
var unityStopWords = []string{
	"一种", "所述", "其特征在于", "包括", "包含", "组成",
	"和", "及", "与", "或", "的", "之", "该", "其", "等",
	"用于", "按照", "根据", "至少",
}

// normalizeUnityText 清洗权利要求文本：去除标点、空白与结构样板词，保留技术词汇。
func normalizeUnityText(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '，', '；', '。', '、', '：', ',', ';', '.', ':', '（', '）', '(', ')',
			'\n', '\t', '"', '\'', '“', '”', '‘', '’':
			return -1
		}
		return r
	}, s)
	for _, w := range unityStopWords {
		s = strings.ReplaceAll(s, w, "")
	}
	return s
}

func runeSet(s []rune) map[rune]bool {
	set := make(map[rune]bool, len(s))
	for _, r := range s {
		set[r] = true
	}
	return set
}

func countIntersection(setA, setB map[rune]bool) int {
	inter := 0
	for r := range setA {
		if setB[r] {
			inter++
		}
	}
	return inter
}

// charOverlap 字符重叠率：交集大小 / 较小集合大小。
func charOverlap(a, b []rune) float64 {
	setA, setB := runeSet(a), runeSet(b)
	inter := countIntersection(setA, setB)
	minLen := len(setA)
	if len(setB) < minLen {
		minLen = len(setB)
	}
	if minLen == 0 {
		return 0
	}
	return float64(inter) / float64(minLen)
}

// jaccardSimilarity Jaccard 系数：交集大小 / 并集大小。
func jaccardSimilarity(a, b []rune) float64 {
	setA, setB := runeSet(a), runeSet(b)
	inter := countIntersection(setA, setB)
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func bigramSet(s []rune) map[string]int {
	m := make(map[string]int)
	for i := 0; i+1 < len(s); i++ {
		m[string(s[i:i+2])]++
	}
	return m
}

// bigramCosine Bigram 余弦相似度。
func bigramCosine(a, b []rune) float64 {
	setA, setB := bigramSet(a), bigramSet(b)
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	dot := 0
	for g, ca := range setA {
		if cb, ok := setB[g]; ok {
			dot += ca * cb
		}
	}
	var magA, magB float64
	for _, c := range setA {
		magA += float64(c * c)
	}
	for _, c := range setB {
		magB += float64(c * c)
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return float64(dot) / (math.Sqrt(magA) * math.Sqrt(magB))
}
