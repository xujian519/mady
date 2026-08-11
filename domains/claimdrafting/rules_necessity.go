package claimdrafting

import (
	"strconv"
	"strings"
)

// =============================================================================
// 必要技术特征规则：独立权利要求包含全部必要技术特征
// =============================================================================

// necessityCompletenessRule 检查独立权利要求是否包含解决技术问题的全部必要技术特征。
// 依据：细则第21条第2款——独立权利要求应当记载解决技术问题的必要技术特征。
// 检查逻辑：确认每个 PFE triple 中的关联特征是否在独立权利要求中体现。
type necessityCompletenessRule struct{ baseRule }

func (r *necessityCompletenessRule) Check(claims []Claim, input DraftInput) []Violation {
	var violations []Violation

	if len(input.PFETriples) == 0 {
		return violations
	}

	// 收集独立权利要求中出现的特征标识
	independentFeatureDescs := make(map[string]bool)
	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}
		text := c.Preamble + c.Characterized
		for _, f := range input.Features {
			if strings.Contains(text, f.Description) {
				independentFeatureDescs[f.ID] = true
			}
		}
	}

	// 检查每个 PFE triple 的核心特征是否在独立权利要求中
	for _, triple := range input.PFETriples {
		missingFeatures := 0
		for _, fid := range triple.FeatureIDs {
			if !independentFeatureDescs[fid] {
				missingFeatures++
			}
		}
		if missingFeatures > 0 && len(triple.FeatureIDs) > 0 {
			violations = append(violations, Violation{
				RuleName:    r.Name(),
				RuleBasis:   r.LegalBasis(),
				Severity:    SeverityError,
				ClaimNumber: 1,
				Message:     "独立权利要求可能缺少解决技术问题的必要技术特征（与[" + triple.Problem + "]相关的" + strconv.Itoa(missingFeatures) + "个特征未体现）",
				Suggestion:  "从技术问题[" + triple.Problem + "]出发，反推解决该问题不可缺少的技术特征，将其补入独立权利要求",
			})
		}
	}
	return violations
}

// =============================================================================
// 必要技术特征规则：独立权利要求不包含非必要技术特征
// =============================================================================

// necessityNonEssentialRule 检查独立权利要求是否包含非必要技术特征。
// 依据：专利法第26条第4款（结合审查指南§3.1.2）。
type necessityNonEssentialRule struct{ baseRule }

func (r *necessityNonEssentialRule) Check(claims []Claim, input DraftInput) []Violation {
	var violations []Violation

	if len(input.Features) == 0 {
		return violations
	}

	for _, c := range claims {
		if c.Kind != "independent" {
			continue
		}

		// 收集标记为 low 重要性的特征
		lowImportanceFeatures := make(map[string]bool)
		for _, f := range input.Features {
			if f.Importance == "low" {
				lowImportanceFeatures[f.Description] = true
			}
		}

		// 检查独立权利要求中是否包含低重要性特征
		text := c.Preamble + c.Characterized
		for desc := range lowImportanceFeatures {
			if strings.Contains(text, desc) {
				violations = append(violations, Violation{
					RuleName:    r.Name(),
					RuleBasis:   r.LegalBasis(),
					Severity:    SeverityInfo,
					ClaimNumber: c.Number,
					Message:     "独立权利要求中可能包含非必要技术特征：[" + desc + "]",
					Suggestion:  "考虑将[" + desc + "]移至从属权利要求，仅在确认该特征为解决技术问题所不可缺少时才保留在独立权利要求中",
				})
			}
		}
	}
	return violations
}

// =============================================================================
// 单一性检查规则（专利法第31条）
// =============================================================================

type unityInventionRule struct{ baseRule }

// Check 使用单一性评分引擎（domains/claimdrafting/unity.go）判定多个独立权利要求
// 之间是否包含相同或相应的特定技术特征。任一权利要求对的特征相似度低于 0.6
// （实施细则第43条阈值）即提示不满足单一性。
func (r *unityInventionRule) Check(claims []Claim, _ DraftInput) []Violation {
	verdict := CheckUnity(claims)
	if verdict.HasUnity {
		return nil
	}

	var nums []string
	for _, p := range verdict.PairScores {
		nums = append(nums, strconv.Itoa(p.LeftNumber), strconv.Itoa(p.RightNumber))
	}
	if len(nums) == 0 {
		return nil
	}
	nums = dedupStrings(nums)

	return []Violation{{
		RuleName: r.Name(), RuleBasis: r.LegalBasis(),
		Severity: SeverityWarning, ClaimNumber: 0,
		Message:    "权利要求" + strings.Join(nums, "、") + "之间可能不满足单一性要求（专利法第31条）：独立权利要求对的特征相似度低于 0.6，缺少共同的特定技术特征（综合评分 " + strconv.FormatFloat(verdict.Score, 'f', 0, 64) + "）",
		Suggestion: "请检查各独立权利要求是否属于一个总的发明构思，必要时提出分案申请",
	}}
}

// dedupStrings 去重并保持首次出现顺序。
func dedupStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, s := range items {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
