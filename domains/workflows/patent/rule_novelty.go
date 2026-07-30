package patent

// ----------------------------------------------------------------------------

// NoveltyRules returns rules specific to patent novelty analysis (专利法第22条第2款).
// These rules strengthen the baseline novelty checks with feature-coverage and
// search-completeness verification.
func NoveltyRules() []CheckRule {
	return []CheckRule{
		{
			ID:               ruleNoveltySingleComparison,
			Name:             "新颖性单独对比原则",
			Description:      "新颖性分析必须采用单独对比原则，不得结合多份对比文件",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "新颖性分析未遵循单独对比原则",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termNovelty, termPriorArtDoc},
			SingleComparison: true,
			Domain:           domainNovelty,
			FixSuggestion:    "对每项权利要求与一份对比文件进行单独对比，明确相同或实质相同的技术方案",
		},
		{
			ID:               "NOVELTY-FEATURE-COVERAGE",
			Name:             "新颖性特征覆盖分析",
			Description:      "新颖性分析应逐一比对权利要求的所有技术特征与对比文件",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "新颖性分析缺少技术特征的逐一比对",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termTechFeature},
			Domain:           domainNovelty,
			FixSuggestion:    "列出权利要求的全部技术特征，逐一标注对比文件是否公开",
		},
	}
}
