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

func (r *unityInventionRule) Check(claims []Claim, _ DraftInput) []Violation {
	var indClaims []Claim
	for _, c := range claims {
		if c.Kind == "independent" {
			indClaims = append(indClaims, c)
		}
	}
	if len(indClaims) <= 1 {
		return nil
	}
	claimTerms := make([][]string, len(indClaims))
	for i, c := range indClaims {
		text := c.Preamble + " " + c.Characterized
		claimTerms[i] = extractTechnicalTerms(text)
	}
	commonTerms := findCommonTerms(claimTerms)
	if len(commonTerms) < 2 {
		var nums []string
		for _, c := range indClaims {
			nums = append(nums, strconv.Itoa(c.Number))
		}
		return []Violation{{
			RuleName: r.Name(), RuleBasis: r.LegalBasis(),
			Severity: SeverityWarning, ClaimNumber: 0,
			Message:    "权利要求" + strings.Join(nums, "、") + "之间可能不满足单一性要求（专利法第31条），缺少共同的特定技术特征",
			Suggestion: "请检查各独立权利要求是否属于一个总的发明构思，必要时提出分案申请",
		}}
	}
	return nil
}

func extractTechnicalTerms(text string) []string {
	var terms []string
	seen := make(map[string]bool)
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '，' || r == '；' || r == '。' || r == '、' || r == ' ' || r == '：'
	})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len([]rune(part)) < 2 || len([]rune(part)) > 8 {
			continue
		}
		if isStopTerm(part) {
			continue
		}
		if !seen[part] {
			seen[part] = true
			terms = append(terms, part)
		}
	}
	return terms
}

func isStopTerm(s string) bool {
	stops := []string{"所述", "包括", "其特征在于", "一种", "及", "和", "与", "或",
		"该", "其", "之", "的", "了", "在于", "特征", "属于", "用于", "按照",
		"其中", "之间", "以上", "以下", "至少", "根据", "一个", "同时", "另外", "不是"}
	for _, stop := range stops {
		if s == stop {
			return true
		}
	}
	return false
}

func findCommonTerms(claimTerms [][]string) []string {
	if len(claimTerms) == 0 {
		return nil
	}
	common := make(map[string]int)
	for _, term := range claimTerms[0] {
		common[term] = 1
	}
	for i := 1; i < len(claimTerms); i++ {
		set := make(map[string]bool)
		for _, t := range claimTerms[i] {
			set[t] = true
		}
		for t := range common {
			if !set[t] {
				delete(common, t)
			}
		}
	}
	var result []string
	for t, cnt := range common {
		if cnt >= len(claimTerms) {
			result = append(result, t)
		}
	}
	return result
}
