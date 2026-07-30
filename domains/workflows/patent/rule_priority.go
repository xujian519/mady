package patent

// PriorityRules returns rules for priority date determination and analysis
// (优先权规则). Covers priority date determination, priority transfer review,
// priority claim validity, priority date comparison, and partial priority
// handling.
func PriorityRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "PRIORITY-01",
			Name:             "优先权日认定",
			Description:      "确认专利申请的优先权日及其法律效力",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未准确认定优先权日",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termPriorityDate, termPriority},
			Domain:           domainNovelty,
			FixSuggestion:    "确认优先权主张的依据和优先权日的准确日期",
		},
		{
			ID:               "PRIORITY-02",
			Name:             "优先权转让审查",
			Description:      "优先权人变更应在申请日前完成转让手续",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未审查优先权转让的程序合规性",
			CheckType:        CheckAmendmentScope,
			RequiredElements: []string{termPriority, "转让"},
			Domain:           domainAmendment,
			FixSuggestion:    "确认优先权转让是否在申请日前完成，手续是否完整",
		},
		{
			ID:               "PRIORITY-03",
			Name:             "优先权主张有效性",
			Description:      "优先权主张应符合形式条件和实质条件",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分审查优先权主张的有效性",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termPriority, "有效性"},
			Domain:           domainNovelty,
			FixSuggestion:    "审查优先权主张是否符合形式条件和实质条件（在先申请是否相同主题）",
		},
		{
			ID:               "PRIORITY-04",
			Name:             "优先权日与申请日对比",
			Description:      "以有效的优先权日作为现有技术判断的时间基准",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未以有效的优先权日作为现有技术判断的时间基准",
			CheckType:        CheckNovelty,
			RequiredElements: []string{termPriorityDate, termFilingDateLit, "现有技术"},
			Domain:           domainNovelty,
			FixSuggestion:    "确认优先权有效后，以优先权日作为判断现有技术的时间基准",
		},
		{
			ID:               "PRIORITY-05",
			Name:             "部分优先权处理",
			Description:      "同一申请中包含多项优先权时，应区分不同优先权对应的事项",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析部分优先权的适用性",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"部分优先权", "多项优先权"},
			Domain:           domainNovelty,
			FixSuggestion:    "逐项确定各技术方案对应的优先权日，区分部分优先权和多项优先权",
		},
	}
}
