package patent

// InventivenessRules returns rules specific to inventiveness analysis using the
// three-step method (专利法第22条第3款).
func InventivenessRules() []CheckRule {
	return []CheckRule{
		{
			ID:          ruleInventivenessThreeStep,
			Name:        "创造性三步法",
			Description: "创造性分析须包含三步法：最接近现有技术→区别技术特征→技术启示",
			Level:       LevelMust,
			Severity:    SeverityCritical,
			Message:     "创造性分析缺少三步法",
			CheckType:   CheckInventiveness,
			StepElements: [][]string{
				{termClosestPriorArtFull, termClosestPriorArt},
				{termDistinguishingFeatures, termDiffFeatures},
				{termTechHint, "显而易见", "公知常识"},
			},
			Domain:        domainInventiveness,
			FixSuggestion: "明确最接近现有技术，提炼区别技术特征，论证是否存在技术启示",
		},
		{
			ID:               "INVENTIVENESS-TECHNICAL-PROBLEM",
			Name:             "实际解决技术问题",
			Description:      "创造性三步法第二步应明确发明实际解决的技术问题",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "创造性分析未明确实际解决的技术问题",
			CheckType:        CheckInventiveness,
			RequiredElements: []string{termDistinguishingFeatures},
			Domain:           domainInventiveness,
			FixSuggestion:    "基于区别技术特征，确定发明相对于最接近现有技术实际解决的技术问题",
		},
	}
}
