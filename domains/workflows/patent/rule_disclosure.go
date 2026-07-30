package patent

// DisclosureRules returns rules for disclosure sufficiency and claim analysis.
func DisclosureRules() []CheckRule {
	return []CheckRule{
		{
			ID:              "DISCLOSURE-SUFFICIENCY",
			Name:            "充分公开审查",
			Description:     "说明书应充分公开发明，使本领域技术人员能够实现",
			Level:           LevelShould,
			Severity:        SeverityMajor,
			Message:         "充分公开分析不完整",
			CheckType:       CheckDisclosure,
			RequiredAspects: []string{termSufficientDisclosure, termEnable},
			Domain:          domainDisclosure,
			FixSuggestion:   "确认说明书是否提供足够的技术细节使本领域技术人员能够实现该发明",
		},
		{
			ID:            "CLAIM-CLARITY-SUPPORT",
			Name:          "权利要求清楚性与支持",
			Description:   "权利要求应当清楚、得到说明书支持",
			Level:         LevelShould,
			Severity:      SeverityMajor,
			Message:       "权利要求分析缺少必要维度",
			CheckType:     CheckClaimAnalysis,
			Dimensions:    []string{dimClarity, dimSupport},
			Domain:        domainClaims,
			FixSuggestion: "检查权利要求是否清楚简明、是否得到说明书支持",
		},
		{
			ID:            "CLAIM-ESSENTIAL-FEATURES",
			Name:          "必要技术特征完整性",
			Description:   "独立权利要求应包含解决技术问题的必要技术特征",
			Level:         LevelQuality,
			Severity:      SeverityMinor,
			Message:       "权利要求可能缺少必要技术特征",
			CheckType:     CheckClaimAnalysis,
			Dimensions:    []string{dimEssential, dimConsistency},
			Domain:        domainClaims,
			FixSuggestion: "核对独立权利要求是否包含全部必要技术特征",
		},
	}
}
