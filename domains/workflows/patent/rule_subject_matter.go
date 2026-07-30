package patent

// SubjectMatterRules returns rules for patent subject matter eligibility analysis
// under Article 2 of Chinese Patent Law (专利法第2条). Covers technical solution
// definition, technical problem, technical means, non-patentable subject matter
// exclusion, and technical effect evaluation.
func SubjectMatterRules() []CheckRule {
	return []CheckRule{
		{
			ID:               "SUBJECT-01",
			Name:             "技术方案构成审查",
			Description:      "保护客体应是利用自然规律解决技术问题的技术方案",
			Level:            LevelMust,
			Severity:         SeverityCritical,
			Message:          "未充分论证要求保护的主题是否构成技术方案",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术方案", "自然规律"},
			Domain:           domainExamination,
			FixSuggestion:    "论证该主题是否利用自然规律、解决技术问题、产生技术效果",
		},
		{
			ID:               "SUBJECT-02",
			Name:             "技术问题认定",
			Description:      "技术方案应解决明确的技术问题",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未明确技术方案解决的技术问题",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术问题"},
			Domain:           domainExamination,
			FixSuggestion:    "明确技术方案所要解决的技术问题",
		},
		{
			ID:               "SUBJECT-03",
			Name:             "技术手段审查",
			Description:      "技术方案应采用技术手段实现技术问题的解决",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未充分分析技术方案所采用的技术手段",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术手段"},
			Domain:           domainExamination,
			FixSuggestion:    "说明技术方案采用了哪些技术手段来解决技术问题",
		},
		{
			ID:               "SUBJECT-04",
			Name:             "非可专利客体排除",
			Description:      "排除科学发现、智力活动规则、疾病诊断治疗方法、原子核变换方法",
			Level:            LevelShould,
			Severity:         SeverityMajor,
			Message:          "未逐一排除非可专利客体",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{termSciDiscovery, "智力活动规则"},
			Domain:           domainExamination,
			FixSuggestion:    "逐项排除科学发现、智力活动规则、疾病诊断治疗方法、原子核变换方法",
		},
		{
			ID:               "SUBJECT-05",
			Name:             "技术效果分析",
			Description:      "技术方案应产生与解决的技术问题相对应的技术效果",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未分析技术方案的技术效果",
			CheckType:        CheckSubjectMatter,
			RequiredElements: []string{"技术效果"},
			Domain:           domainExamination,
			FixSuggestion:    "说明技术方案所产生的技术效果与解决的技术问题之间的对应关系",
		},
	}
}
