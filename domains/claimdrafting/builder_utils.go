package claimdrafting

import (
	"sort"
	"strings"
)

// filterFeaturesByCategory 按类别过滤特征列表。
func filterFeaturesByCategory(features []Feature, category string) []Feature {
	var result []Feature
	for _, f := range features {
		if f.Category == category {
			result = append(result, f)
		}
	}
	return result
}

// formatParallelFeatureDesc 格式化并列独立权利要求的特征描述。
func formatParallelFeatureDesc(f Feature, mode string) string {
	desc := strings.TrimSpace(f.Description)
	if desc == "" {
		return "[特征]"
	}
	if mode == "method" || mode == "manufacturing" {
		return desc + "步骤"
	}
	if f.Function != "" {
		return desc + "，用于" + f.Function
	}
	return desc
}

// buildPFECountMap 构建特征ID到PFE三元组关联数的映射。
// 一个特征关联到越多PFE三元组，说明其在发明中越核心。
func buildPFECountMap(triples []PFETriple) map[string]int {
	counts := make(map[string]int)
	for _, t := range triples {
		for _, fid := range t.FeatureIDs {
			counts[fid]++
		}
	}
	return counts
}

// =============================================================================
// 辅助函数
// =============================================================================

// classifyFeatures 将特征分类为必要特征和可选特征。
// 必要特征：直接关联到 PFE triple 且重要性为 high 的特征。
// 可选特征：其他特征（将放入从属权利要求）。
func classifyFeatures(features []Feature, triples []PFETriple) (essential, optional []Feature) {
	tripleFeatureIDs := make(map[string]bool)
	for _, t := range triples {
		for _, fid := range t.FeatureIDs {
			tripleFeatureIDs[fid] = true
		}
	}

	for _, f := range features {
		if tripleFeatureIDs[f.ID] && f.Importance == "high" {
			essential = append(essential, f)
		} else {
			optional = append(optional, f)
		}
	}
	return
}

// determineClaimTypeByFeatures 根据特征类型判断权利要求类型。
func determineClaimTypeByFeatures(features []Feature) ClaimType {
	for _, f := range features {
		if f.Category == "method" {
			return ClaimTypeMethod
		}
	}
	return ClaimTypeProduct
}

// classifyDomain 根据输入推断技术领域。
func classifyDomain(input DraftInput) TechDomain {
	// 基于特征类别统计
	catCount := make(map[string]int)
	for _, f := range input.Features {
		catCount[f.Category]++
	}

	// 基于关键词检测
	allText := input.Title + " " + strings.Join(input.Problems, " ")
	for _, f := range input.Features {
		allText += " " + f.Description
	}

	mechKeywords := []string{"机械", "装置", "机构", "连接", "固定", "支撑", "壳体", "弹簧", "齿轮"}
	elecKeywords := []string{"电路", "电压", "电流", "信号", "电极", "导线", "半导体", "放大", "传感器"}
	chemKeywords := []string{"组合物", "化合物", "组分", "含量", "重量", "百分比", "摩尔", "催化剂"}
	softKeywords := []string{"数据", "方法", "步骤", "程序", "处理", "计算", "算法", "图像", "信号处理"}

	score := map[TechDomain]int{
		DomainMechanical: 0,
		DomainElectrical: 0,
		DomainChemical:   0,
		DomainSoftware:   0,
	}

	score[DomainMechanical] += countKeywords(allText, mechKeywords) + catCount["structure"]*2
	score[DomainElectrical] += countKeywords(allText, elecKeywords) + catCount["parameter"]*2
	score[DomainChemical] += countKeywords(allText, chemKeywords) + catCount["material"]*3
	score[DomainSoftware] += countKeywords(allText, softKeywords) + catCount["method"]*2

	var bestDomain TechDomain
	bestScore := 0
	for d, s := range score {
		if s > bestScore {
			bestScore = s
			bestDomain = d
		}
	}

	if bestScore == 0 {
		return DomainGeneral
	}
	return bestDomain
}

// countKeywords 统计文本中包含的关键词数量。
func countKeywords(text string, keywords []string) int {
	count := 0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			count++
		}
	}
	return count
}

// buildPreambleFromProblem 从技术问题构建前序部分基础。
func buildPreambleFromProblem(problem string) string {
	problem = strings.TrimPrefix(problem, "技术问题：")
	problem = strings.TrimPrefix(problem, "现有技术中")
	problem = strings.TrimSuffix(problem, "的问题")
	problem = strings.TrimSuffix(problem, "的缺陷")
	problem = strings.TrimSuffix(problem, "的不足")

	if len(problem) > 3 {
		return problem
	}
	return ""
}

// formatFeatureDesc 格式化技术特征为权利要求表述。
func formatFeatureDesc(f Feature) string {
	desc := strings.TrimSpace(f.Description)
	if desc == "" {
		return "[特征]"
	}
	if f.Function != "" {
		return desc + "，用于" + f.Function
	}
	return desc
}

// sortFeaturesByScore 按综合得分排序特征（金字塔型布局的基础排序）。
// 得分 = importance 权重 × PFE 关联数加权。
// 高得分特征应写入靠前的从属权利要求（保护范围较宽）。
// 低得分特征应写入靠后的从属权利要求（递进限定）。
func sortFeaturesByScore(features []Feature, pfeCount map[string]int) []Feature {
	importanceWeight := map[string]int{"high": 100, "medium": 50, "low": 10}
	sorted := make([]Feature, len(features))
	copy(sorted, features)
	sort.SliceStable(sorted, func(i, j int) bool {
		scoreI := importanceWeight[sorted[i].Importance] + pfeCount[sorted[i].ID]*15
		scoreJ := importanceWeight[sorted[j].Importance] + pfeCount[sorted[j].ID]*15
		if scoreI != scoreJ {
			return scoreI > scoreJ // 高分优先
		}
		// 同分时按描述长度降序（更具体的在前）
		if len(sorted[i].Description) != len(sorted[j].Description) {
			return len(sorted[i].Description) > len(sorted[j].Description)
		}
		// 最终兜底：按 ID 字典序，确保严格弱序
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}
