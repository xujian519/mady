package patent

// ReexaminationRules returns rules for patent reexamination request drafting
// (复审请求). Covers rejection-scope analysis, procedural legality, and new
// evidence relevance.
func ReexaminationRules() []CheckRule {
	return []CheckRule{
		{
			ID:            "REEXAM-GROUNDS-SCOPE",
			Name:          "复审理由范围审查",
			Description:   "复审理由应在驳回决定范围内，或提供新的证据/理由",
			Level:         LevelShould,
			Severity:      SeverityMajor,
			Message:       "复审理由分析不完整",
			CheckType:     CheckClaimAnalysis,
			Dimensions:    []string{dimClarity, dimConsistency},
			Domain:        domainReexamination,
			FixSuggestion: "逐条列出驳回理由，针对性回应或提交新证据克服",
		},
		{
			ID:               "REEXAM-NEW-EVIDENCE",
			Name:             "新证据关联性",
			Description:      "复审中提交的新证据应与克服驳回理由直接相关",
			Level:            LevelQuality,
			Severity:         SeverityMinor,
			Message:          "未说明新证据与驳回理由的关联性",
			CheckType:        CheckNovelty,
			RequiredElements: []string{"新证据"},
			Domain:           domainReexamination,
			FixSuggestion:    "对于每份新证据，说明其如何克服驳回决定中指出的缺陷",
		},
	}
}
