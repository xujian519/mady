package patent

// DesignRules returns rules for design patent comparison (外观设计对比).
// Covers overall visual effect comparison, product category determination,
// design feature identification, direct copy evaluation, and multi-design
// comparison framework.
func DesignRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "DESIGN-01",
			Name:             "外观设计整体视觉效果对比",
			Description:      "外观设计对比应以整体视觉效果为准，综合判断是否构成相同或近似",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "外观设计对比缺少整体视觉效果分析",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{termDesignPatent, "整体视觉效果"},
			Domain:           domainDesign,
			FixSuggestion:    "以整体视觉效果为准进行外观设计对比，判断是否构成相同或近似",
		},
		{
			ID:               "DESIGN-02",
			Name:             "外观设计产品种类认定",
			Description:      "外观设计对比应在相同或相近种类的产品之间进行",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未明确认定产品种类是否相同或相近",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"产品种类"},
			Domain:           domainDesign,
			FixSuggestion:    "根据产品用途、功能、销售渠道等因素认定产品种类是否相同或相近",
		},
		{
			ID:               "DESIGN-03",
			Name:             "外观设计设计特征识别",
			Description:      "应识别外观设计的设计特征，区分创新设计部分与惯常设计",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未充分识别外观设计的设计特征",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"设计特征"},
			Domain:           domainDesign,
			FixSuggestion:    "识别外观设计中区别于现有设计的创新设计特征",
		},
		{
			ID:               "DESIGN-04",
			Name:             "外观设计直接模仿判断",
			Description:      "判断外观设计是否构成直接模仿或仅存在局部细微差异",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析外观设计是否构成直接模仿",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"直接模仿", "局部差异"},
			Domain:           domainDesign,
			FixSuggestion:    "判断局部差异是否对整体视觉效果产生显著影响",
		},
		{
			ID:               "DESIGN-05",
			Name:             "外观设计多设计对比框架",
			Description:      "涉及多项外观设计时，应逐项对比并明确各设计对象的对比结果",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "多设计对比未逐项分析",
			CheckType:        CheckDesignComparison,
			RequiredElements: []string{"逐项对比", termDesignPatent},
			Domain:           domainDesign,
			FixSuggestion:    "逐项对比每项外观设计与对比设计的整体视觉效果",
		},
	}
}
