package patent

// InfringementRules returns rules for patent infringement analysis.
// Covers the full-coverage principle, equivalence doctrine, prosecution history
// estoppel (禁止反悔), and dedication rule (捐献规则).
func InfringementRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "INFRINGEMENT-FULL-COVERAGE",
			Name:             "侵权全面覆盖原则",
			Description:      "侵权分析应全面比对被控方案是否包含权利要求的全部技术特征",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "侵权分析缺少全面覆盖分析",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"全面覆盖", termTechFeature},
			Domain:           domainInfringement,
			FixSuggestion:    "分解权利要求为技术特征A/B/C，逐一判断被控方案是否包含",
		},
		{
			ID:               "INFRINGEMENT-EQUIVALENCE",
			Name:             "等同侵权判定",
			Description:      "侵权分析应评估区别特征是否构成等同替换（手段/功能/效果基本相同）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "侵权分析缺少等同原则评估",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"等同"},
			Domain:           domainInfringement,
			FixSuggestion:    "对不构成字面侵权的特征，检查是否满足等同三要素：手段/功能/效果基本相同+无需创造性劳动",
		},
		{
			ID:               "INFRINGEMENT-ESTOPPEL",
			Name:             "禁止反悔原则检查",
			Description:      "侵权分析应考虑审查过程中的修改是否导致权利放弃（禁止反悔）",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "侵权分析未考虑禁止反悔原则的限制",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"禁止反悔"},
			Domain:           domainInfringement,
			FixSuggestion:    "审查专利审查过程中的修改和陈述，确认是否对等同范围构成限制",
		},
		{
			ID:               "INFRINGEMENT-DEDICATION",
			Name:             "捐献规则检查",
			Description:      "说明书中披露但权利要求未保护的技术方案视为捐献公众",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "侵权分析未检查捐献规则的适用性",
			CheckType:        CheckInfringement,
			RequiredElements: []string{"捐献规则"},
			Domain:           domainInfringement,
			FixSuggestion:    "确认被控方案对应的技术特征是否在说明书中披露但未写入权利要求",
		},
	}
}
