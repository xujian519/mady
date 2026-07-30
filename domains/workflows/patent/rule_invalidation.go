package patent

// InvalidationRules returns rules for patent invalidation analysis (无效宣告).
// Key constraints:
//   - Each invalidation ground MUST be argued independently per claim
//   - Multi-document combinations MUST justify combination motivation
//   - Prior-art publication dates MUST be verified against priority date
func InvalidationRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "INVALID-NOVELTY-SINGLE-COMPARISON",
			Name:             "无效新颖性单独对比",
			Description:      "无效宣告中的新颖性理由须采用单独对比原则",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "无效宣告中新颖性论证未遵循单独对比原则",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termNovelty, termPriorArtDoc},
			SingleComparison: true,
			Domain:           domainInvalidation,
			FixSuggestion:    "对每项权利要求逐一与单份对比文件进行新颖性比对",
		},
		{
			ID:          "INVALID-COMBINATION-MOTIVATION",
			Name:        "无效组合动机论证",
			Description: "多篇对比文件组合攻击创造性时，须论证组合的技术启示/动机",
			Level:       LevelMust,
			Severity:    SeverityCritical,
			Message:     "无效宣告中多篇组合缺乏组合动机论证",
			CheckType:   CheckInventiveness,
			StepElements: [][]string{
				{termClosestPriorArtFull, termClosestPriorArt},
				{termDistinguishingFeatures, termDiffFeatures},
				{termCombinationMotivation, termTechHint, termCombinationHint},
			},
			Domain:        domainInvalidation,
			FixSuggestion: "论证本领域技术人员有动机将对比文件组合，说明组合的合理性",
		},
		{
			ID:               "INVALID-PRIORITY-DATE-CHECK",
			Name:             "对比文件公开日核实",
			Description:      "无效宣告中引用的对比文件公开日须早于涉案专利的优先权日",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未核实对比文件的公开日是否早于优先权日",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termPriorityDate},
			Domain:           domainInvalidation,
			FixSuggestion:    "核实每份对比文件的公开日，标注是否早于涉案专利的优先权日/申请日",
		},
	}
}
