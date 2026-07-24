// Package patent — standardized legal reasoning patterns from the patent
// re-examination knowledge base. Each pattern encodes a canonical reasoning
// template that patent agents and examiners follow during invalidation and
// examination proceedings. Patterns are grouped by category (creativity /
// novelty / claims / other) and carry metadata about their usage frequency
// and invalidation success rate.
package patent

// ReasoningPattern represents a standardized legal reasoning template from
// the patent re-examination knowledge base. Each pattern corresponds to one
// of the 18 canonical reasoning forms used by patent examiners and agents.
type ReasoningPattern struct {
	ID               string      // unique pattern identifier (e.g. "RP-CREATIVITY-01")
	Category         string      // "creativity" / "novelty" / "claims" / "other"
	Name             string      // Chinese display name
	Frequency        float64     // occurrence frequency in practice (%)
	InvalidationRate float64     // success rate when used for invalidation (%)
	CoreLogic        string      // core reasoning logic description
	CheckRules       []CheckRule // associated deterministic check rules
	Template         string      // canonical reasoning template text (Chinese)
}

// AllPatterns returns all 18 standardized reasoning patterns ordered by
// priority batch: Creativity (1-6), Novelty (7-9), Claims (10-14), Other (15-18).
func AllPatterns() []ReasoningPattern {
	return []ReasoningPattern{
		// =========================================================================
		// Batch 1 — Creativity (创造性, 6 patterns)
		// =========================================================================
		{
			ID:               "RP-CREATIVITY-01",
			Category:         "creativity",
			Name:             "单对比文件+公知常识",
			Frequency:        54.0,
			InvalidationRate: 95.9,
			CoreLogic:        "区别特征属于公知常识是实践中最常用的创造性否定路径，根据三步法第三步，如果区别技术特征属于本领域的公知常识，则无需引入额外的对比文件即可认定不具备创造性",
			Template:         "最接近的现有技术为...，区别技术特征在于...，该区别特征属于本领域的公知常识/惯用技术手段，因此本领域技术人员在面对技术问题时不需付出创造性劳动即可获得该技术方案",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CREATIVITY-01A",
					Name:        "单对比文件+公知常识审查",
					Description: "区别特征属于公知常识的创造性否定路径：无需额外对比文件",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "创造性分析未完整论证公知常识路径",
					CheckType:   CheckInventiveness,
					Domain:      "patent_inventiveness",
					PathElements: [][]string{
						{"最接近的现有技术", "最接近对比文件"},
						{"区别技术特征", "区别特征"},
						{"公知常识", "惯用技术手段", "常规设计", "本领域常规"},
						{"显而易见", "无需创造性劳动", "非显而易见"},
					},
					FixSuggestion: "按三步法论证：最接近现有技术→区别特征→区别特征属于公知常识→无需创造性劳动",
				},
				{
					ID:               "REASON-CREATIVITY-01B",
					Name:             "公知常识证据支撑",
					Description:      "公知常识主张应提供足以使本领域技术人员信服的论证或证据",
					Level:            LevelShould,
					Severity:         SeverityMajor,
					Message:          "公知常识的认定缺乏充分论证",
					CheckType:        CheckInventiveness,
					RequiredElements: []string{"公知常识"},
					Domain:           "patent_inventiveness",
					FixSuggestion:    "提供公知常识性证据（教科书/工具书）或充分论述该技术手段的普遍性",
				},
			},
		},
		{
			ID:               "RP-CREATIVITY-02",
			Category:         "creativity",
			Name:             "多对比文件结合",
			Frequency:        14.3,
			InvalidationRate: 73.7,
			CoreLogic:        "将多篇对比文件结合时需要论证本领域技术人员具有结合的技术启示，不能仅因为多篇对比文件覆盖了全部区别特征就认定不具备创造性",
			Template:         "对比文件1公开了...，对比文件2公开了...，本领域技术人员有动机将对比文件2的技术方案结合到对比文件1中，因为...，从而得出要求保护的发明",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CREATIVITY-02",
					Name:        "多对比文件结合审查",
					Description: "多篇对比文件结合时须论证结合动机/技术启示",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "多对比文件结合缺少组合动机论证",
					CheckType:   CheckInventiveness,
					Domain:      "patent_inventiveness",
					PathElements: [][]string{
						{"最接近的现有技术", "最接近对比文件", "对比文件1"},
						{"区别技术特征", "区别特征"},
						{"技术启示", "组合动机", "结合动机", "结合启示"},
					},
					FixSuggestion: "说明本领域技术人员有动机将多篇对比文件结合的技术原因和现有技术教导",
				},
			},
		},
		{
			ID:               "RP-CREATIVITY-03",
			Category:         "creativity",
			Name:             "技术启示判断",
			Frequency:        8.1,
			InvalidationRate: 83.8,
			CoreLogic:        "三步法第三步的核心：判断现有技术整体上是否存在技术启示，该技术启示会使本领域技术人员面对所解决的技术问题时改进最接近的现有技术并获得要求保护的发明",
			Template:         "现有技术整体上是否存在技术启示，使得本领域技术人员在面对实际解决的技术问题...时，有动机改进最接近的现有技术...并获得要求保护的发明...",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CREATIVITY-03",
					Name:        "技术启示判断审查",
					Description: "三步法第三步：技术启示的客观判断",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "创造性分析未充分论证是否存在技术启示",
					CheckType:   CheckInventiveness,
					Domain:      "patent_inventiveness",
					PathElements: [][]string{
						{"技术启示", "teaching suggestion motivation"},
						{"显而易见", "obvious", "显而易见性"},
						{"本领域技术人员", "所属领域技术人员", "person skilled in the art"},
					},
					DependsOn:     []string{"INVENTIVENESS-THREE-STEP"},
					FixSuggestion: "以三步法为基础，重点论证现有技术整体上是否存在使本领域技术人员获得该发明的技术启示",
				},
			},
		},
		{
			ID:               "RP-CREATIVITY-04",
			Category:         "creativity",
			Name:             "惯用手段与常规选择",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "本领域惯用技术手段或常规设计选择可直接认定区别技术特征不具备创造性，无需额外对比文件佐证",
			Template:         "该区别特征...是本领域的惯用技术手段/常规设计选择，本领域技术人员在面对技术问题...时会常规性地选择该手段，无需付出创造性劳动",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CREATIVITY-04",
					Name:        "惯用手段与常规选择审查",
					Description: "惯用技术手段/常规选择可直接否定创造性",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "惯用技术手段认定缺乏充分论述",
					CheckType:   CheckInventiveness,
					Domain:      "patent_inventiveness",
					PathElements: [][]string{
						{"惯用技术手段", "惯用手段", "常规设计", "本领域常规"},
						{"众所周知", "本领域通用", "common general knowledge"},
					},
					FixSuggestion: "明确该技术手段在本领域的普遍性和常规性，可引用教科书或工具书佐证",
				},
			},
		},
		{
			ID:               "RP-CREATIVITY-05",
			Category:         "creativity",
			Name:             "用途限定的影响",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "用途特征对创造性判断的影响取决于该用途特征是否隐含了产品在结构/组成/工艺上的变化，不改变产品结构的单纯用途限定对创造性无明显贡献",
			Template:         "用途特征...是否隐含了产品在结构/组成/工艺上的变化。如果用途特征仅限定了使用方式而未改变产品本身，则该用途特征对创造性判断不具有实质性贡献",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CREATIVITY-05",
					Name:        "用途限定的影响审查",
					Description: "用途特征对创造性判断的影响分析",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "未分析用途特征对创造性的实际影响",
					CheckType:   CheckInventiveness,
					Domain:      "patent_inventiveness",
					PathElements: [][]string{
						{"用途特征", "用途限定", "use limitation", "用途"},
						{"产品本身", "产品结构", "产品组成", "产品工艺"},
						{"创造性", "非显而易见"},
					},
					FixSuggestion: "分析用途特征是否隐含了产品的结构/组成/工艺变化，说明其对创造性判断的贡献",
				},
			},
		},
		{
			ID:               "RP-CREATIVITY-06",
			Category:         "creativity",
			Name:             "预料不到的效果认定",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "预料不到的技术效果是创造性辅助判断因素之一，当发明产生了本领域技术人员无法合理预期的技术效果时，可作为具备创造性的有力证据",
			Template:         "本发明产生了预料不到的技术效果...，该效果超出了本领域技术人员在申请日前的合理预期，因此可作为创造性存在的辅助判断依据",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CREATIVITY-06",
					Name:        "预料不到的效果认定审查",
					Description: "预料不到的技术效果作为创造性辅助判断因素",
					Level:       LevelQuality,
					Severity:    SeverityMinor,
					Message:     "未分析是否存在预料不到的技术效果",
					CheckType:   CheckInventiveness,
					Domain:      "patent_inventiveness",
					PathElements: [][]string{
						{"预料不到", "出乎意料", "surprising", "unexpected"},
						{"技术效果", "有益效果", "效果"},
						{"创造性", "创造性的辅助判断"},
					},
					DependsOn:     []string{"INVENTIVENESS-THREE-STEP"},
					FixSuggestion: "说明发明产生了哪些本领域技术人员无法合理预期的技术效果及其证据",
				},
			},
		},

		// =========================================================================
		// Batch 2 — Novelty (新颖性, 3 patterns)
		// =========================================================================
		{
			ID:               "RP-NOVELTY-01",
			Category:         "novelty",
			Name:             "现有技术认定（单独对比+四相同）",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "新颖性判断采用单独对比原则，将一项权利要求与一份现有技术单独对比，判断是否属于相同的技术领域、解决相同的技术问题、采用相同的技术方案、达到相同的预期效果（四相同标准）",
			Template:         "将权利要求...与一份现有技术文件...单独对比：技术领域是否相同（...），技术问题是否相同（...），技术方案是否相同（...），预期效果是否相同（...）",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-NOVELTY-01A",
					Name:        "现有技术认定审查",
					Description: "新颖性判断的单独对比+四相同原则",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "新颖性分析未遵循单独对比和四相同标准",
					CheckType:   CheckNovelty,
					Domain:      "patent_novelty",
					PathElements: [][]string{
						{"现有技术", "prior art"},
						{"单独对比", "单独对比原则", "一一对比"},
						{"技术领域", "技术问题", "技术方案", "技术效果"},
					},
					SingleComparison: true,
					DependsOn:        []string{"NOVELTY-SINGLE-COMPARISON"},
					FixSuggestion:    "按四相同标准逐一比对：技术领域/技术问题/技术方案/预期效果",
				},
				{
					ID:               "REASON-NOVELTY-01B",
					Name:             "四相同标准审查",
					Description:      "四相同标准中技术方案和效果的全面比对",
					Level:            LevelShould,
					Severity:         SeverityMajor,
					Message:          "四相同标准分析不完整",
					CheckType:        CheckNovelty,
					RequiredElements: []string{"技术方案", "技术效果"},
					Domain:           "patent_novelty",
					FixSuggestion:    "确保技术方案四要素（领域/问题/方案/效果）均得到分析",
				},
			},
		},
		{
			ID:               "RP-NOVELTY-02",
			Category:         "novelty",
			Name:             "公开方式判断",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "现有技术的公开方式包括出版物公开（论文/期刊/书籍）、使用公开（销售/展出/公开实施）和互联网公开，核心判断标准是申请日前是否为公众所知",
			Template:         "现有技术通过...方式公开，公开日...，在申请日...之前，公众能够通过...途径获知其技术内容，因此构成现有技术",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-NOVELTY-02A",
					Name:        "公开方式判断审查",
					Description: "出版物、使用、互联网三种公开方式的认定",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "未充分分析现有技术的公开方式",
					CheckType:   CheckPublicAccess,
					Domain:      "patent_novelty",
					PathElements: [][]string{
						{"公开方式", "公开形式", "公开途径"},
						{"申请日", "公开日", "优先权日"},
					},
					FixSuggestion: "明确认定现有技术的公开方式（出版物/使用/互联网）和公开时间",
				},
				{
					ID:               "REASON-NOVELTY-02B",
					Name:             "互联网公开认定审查",
					Description:      "互联网公开的认定及其公开日的确定",
					Level:            LevelShould,
					Severity:         SeverityMajor,
					Message:          "互联网公开分析不完整",
					CheckType:        CheckPublicAccess,
					RequiredElements: []string{"互联网公开", "网络公开"},
					Domain:           "patent_novelty",
					FixSuggestion:    "确认网页公开日的确定方式及公众能够获知的途径",
				},
			},
		},
		{
			ID:               "RP-NOVELTY-03",
			Category:         "novelty",
			Name:             "抵触申请与优先权",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "抵触申请指在先申请在后公开的专利申请，仅可用于新颖性判断；优先权有效的以优先权日作为申请日判断现有技术时间节点",
			Template:         "专利申请...的申请日为...，优先权日为...（优先权有效），对比文件...属于/不属于抵触申请（构成要件分析），仅用于新颖性/创造性判断",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-NOVELTY-03A",
					Name:        "抵触申请审查",
					Description: "抵触申请的构成要件及其仅用于新颖性判断的限制",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "抵触申请分析不完整或误用于创造性判断",
					CheckType:   CheckNovelty,
					Domain:      "patent_novelty",
					PathElements: [][]string{
						{"抵触申请", "在先申请在后公开", "conflicting application"},
						{"新颖性", "新颖性判断"},
					},
					DependsOn:     []string{"NOVELTY-SINGLE-COMPARISON"},
					FixSuggestion: "确认对比文件是否构成抵触申请，仅用于新颖性判断",
				},
				{
					ID:          "REASON-NOVELTY-03B",
					Name:        "优先权审查",
					Description: "优先权日的认定及其对现有技术判断的影响",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "未充分核实优先权日及其有效性",
					CheckType:   CheckNovelty,
					Domain:      "patent_novelty",
					PathElements: [][]string{
						{"优先权", "优先权日", "priority date"},
						{"申请日", "filing date"},
						{"现有技术", "对比文件"},
					},
					FixSuggestion: "核实优先权是否有效，以优先权日作为现有技术判断的时间基准",
				},
			},
		},

		// =========================================================================
		// Batch 3 — Claims / Specification (权利要求/说明书, 5 patterns)
		// =========================================================================
		{
			ID:               "RP-CLAIMS-01",
			Category:         "claims",
			Name:             "不清楚认定",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "权利要求应当清楚、简明地限定保护范围，本领域技术人员根据权利要求文本即可确定保护范围，不得使用含义不确定的术语（如'优选地''大约'等）",
			Template:         "权利要求...中使用的术语...含义不确定，本领域技术人员无法确定其具体范围，导致权利要求不清楚",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CLAIMS-01",
					Name:        "不清楚认定审查",
					Description: "权利要求保护范围的清楚性判断",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "权利要求不清楚性分析不完整",
					CheckType:   CheckClaimAnalysis,
					Domain:      "patent_claims",
					Dimensions:  []string{"clarity"},
					PathElements: [][]string{
						{"清楚", "清晰", "明确", "简要"},
						{"权利要求", "保护范围"},
						{"含义不确定", "术语含义不明"},
					},
					FixSuggestion: "逐一审查权利要求每个术语的含义是否明确，删除含义不确定的表述",
				},
			},
		},
		{
			ID:               "RP-CLAIMS-02",
			Category:         "claims",
			Name:             "不支持认定",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "权利要求概括的范围应当在说明书记载的范围内，能够得到说明书的支持，不得超出具体实施方式和实施例的合理概括范围",
			Template:         "权利要求...概括的范围超出了说明书...记载的范围，说明书仅公开了...，而权利要求涵盖了...，构成不支持",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CLAIMS-02",
					Name:        "不支持认定审查",
					Description: "权利要求是否得到说明书支持",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "权利要求未充分分析是否得到说明书支持",
					CheckType:   CheckClaimAnalysis,
					Domain:      "patent_claims",
					Dimensions:  []string{"support"},
					PathElements: [][]string{
						{"以说明书为依据", "说明书支持", "支持"},
						{"合理概括", "概括范围"},
						{"权利要求", "保护范围"},
					},
					FixSuggestion: "比对权利要求与说明书的具体实施方式和实施例，确认概括范围是否合理",
				},
			},
		},
		{
			ID:               "RP-CLAIMS-03",
			Category:         "claims",
			Name:             "功能性限定",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "功能性限定的解释以说明书公开的实现该功能的具体实施方式为限，不得理解为覆盖能够实现该功能的所有方式",
			Template:         "权利要求中使用了功能性限定...，说明书公开了实现该功能的具体实施方式...，该功能性限定的保护范围应当以说明书公开的实施方式为限",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CLAIMS-03",
					Name:        "功能性限定审查",
					Description: "功能性限定的解释范围与审查规则",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "未充分分析功能性限定的解释范围",
					CheckType:   CheckClaimAnalysis,
					Domain:      "patent_claims",
					PathElements: [][]string{
						{"功能性限定", "功能限定", "功能性特征", "functional limitation"},
						{"具体实施方式", "实施例"},
						{"保护范围", "解释范围"},
					},
					DependsOn:     []string{"CLAIM-CLARITY-SUPPORT"},
					FixSuggestion: "确认说明书记载了实现该功能的具体实施方式，以此限定功能性特征的保护范围",
				},
			},
		},
		{
			ID:               "RP-CLAIMS-04",
			Category:         "claims",
			Name:             "充分公开认定",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "说明书应当清楚、完整地公开发明，使本领域技术人员能够实现该技术方案，解决技术问题并产生预期技术效果",
			Template:         "说明书对技术方案...的公开是否使本领域技术人员能够实现该发明：技术方案是否完整、技术问题是否明确、技术效果是否可预期",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CLAIMS-04",
					Name:        "充分公开认定审查",
					Description: "能够实现标准的审查逻辑",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "充分公开分析未完整覆盖能够实现标准的审查维度",
					CheckType:   CheckDisclosure,
					Domain:      "patent_disclosure",
					PathElements: [][]string{
						{"充分公开", "公开充分", "enablement"},
						{"能够实现", "可实施", "能够制造", "能够使用"},
						{"技术方案", "技术效果"},
					},
					DependsOn:     []string{"DISCLOSURE-SUFFICIENCY"},
					FixSuggestion: "从技术方案完整性、可实施性、技术效果三个维度论证充分公开",
				},
			},
		},
		{
			ID:               "RP-CLAIMS-05",
			Category:         "claims",
			Name:             "实验数据要求",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "在医药化学等领域，说明书应当提供足以证明技术方案能够实现所述用途/效果的实验数据，数据应真实可靠可重现",
			Template:         "说明书提供了实验数据...，该数据是否足以证明技术方案...能够实现所述用途.../效果...，数据是否真实、可靠、可重现",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-CLAIMS-05",
					Name:        "实验数据要求审查",
					Description: "医药化学领域实验数据的满足度判断",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "实验数据满足度分析不充分",
					CheckType:   CheckDisclosure,
					Domain:      "patent_disclosure",
					PathElements: [][]string{
						{"实验数据", "实验例", "试验数据"},
						{"能够实现", "证实", "证明", "验证"},
						{"充分公开", "公开充分"},
					},
					FixSuggestion: "审查实验数据是否足以证明技术方案能够实现所述用途/效果，数据是否可重现",
				},
			},
		},

		// =========================================================================
		// Batch 4 — Other (其他, 4 patterns)
		// =========================================================================
		{
			ID:               "RP-OTHER-01",
			Category:         "other",
			Name:             "保护客体-技术方案认定",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "第2条规定的可专利主题应当是技术方案（产品或方法），利用自然规律解决技术问题，不属于科学发现、智力活动规则、疾病诊断治疗方法或原子核变换方法",
			Template:         "要求保护的主题...是否构成专利法第2条意义上的技术方案：是否利用自然规律，是否解决技术问题，是否属于排除客体（科学发现/智力活动规则/诊断治疗方法/原子核变换）",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-OTHER-01A",
					Name:        "保护客体-技术方案认定审查",
					Description: "第2条可专利主题判断",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "保护客体分析不完整，未充分论证是否构成技术方案",
					CheckType:   CheckSubjectMatter,
					Domain:      "patent_examination",
					PathElements: [][]string{
						{"技术方案", "technical solution"},
						{"自然规律", "自然法则"},
						{"保护客体", "可专利主题", "授权客体", "patentable subject matter"},
					},
					FixSuggestion: "论证是否构成技术方案（利用自然规律、解决技术问题、产生技术效果），并排除非可专利客体",
				},
				{
					ID:               "REASON-OTHER-01B",
					Name:             "非可专利客体排除审查",
					Description:      "排除科学发现/智力活动规则/疾病诊断治疗方法的论证",
					Level:            LevelShould,
					Severity:         SeverityMajor,
					Message:          "未充分分析是否属于非可专利客体",
					CheckType:        CheckSubjectMatter,
					RequiredElements: []string{"科学发现", "智力活动规则", "疾病诊断方法"},
					Domain:           "patent_examination",
					FixSuggestion:    "逐项排除：科学发现、智力活动规则、疾病诊断治疗方法、原子核变换方法",
				},
			},
		},
		{
			ID:               "RP-OTHER-02",
			Category:         "other",
			Name:             "程序-修改超范围与优先权",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "修改内容应当能够从原说明书和权利要求书记载的范围中直接且毫无疑义地确定；优先权主张需在申请日前完成优先权转让",
			Template:         "修改内容...是否能够从原说明书和权利要求书记载的范围中直接且毫无疑义地确定。优先权主张...是否在申请日前完成转让",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-OTHER-02A",
					Name:        "修改超范围审查",
					Description: "修改内容是否超出原申请文件记载范围",
					Level:       LevelMust,
					Severity:    SeverityCritical,
					Message:     "修改超范围分析不完整",
					CheckType:   CheckAmendmentScope,
					Domain:      "patent_amendment",
					PathElements: [][]string{
						{"修改超范围", "超范围", "超出原范围", "超范围修改", "amendment beyond scope"},
						{"直接且毫无疑义", "直接毫无疑义", "原申请文件"},
						{"原说明书", "原权利要求", "原始公开"},
					},
					FixSuggestion: "确认修改内容是否能够从原说明书和权利要求书记载的范围中直接且毫无疑义地确定",
				},
				{
					ID:          "REASON-OTHER-02B",
					Name:        "优先权程序审查",
					Description: "优先权转让及主张的程序合规性",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "优先权程序审查不完整",
					CheckType:   CheckAmendmentScope,
					Domain:      "patent_amendment",
					PathElements: [][]string{
						{"优先权", "优先权日", "优先权转让"},
						{"申请日", "filing date"},
						{"转让", "transfer", "assign"},
					},
					FixSuggestion: "核实优先权转让是否在申请日前完成，优先权主张是否符合程序要求",
				},
			},
		},
		{
			ID:               "RP-OTHER-03",
			Category:         "other",
			Name:             "实用性-积极效果与产业应用",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "第22条第4款规定发明应当能够在产业上制造或使用并产生积极效果，能够大规模工业化实施",
			Template:         "发明...是否具备实用性：是否能够在产业上制造/使用，是否能够产生积极效果，是否能够大规模工业化实施",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-OTHER-03",
					Name:        "实用性-积极效果与产业应用审查",
					Description: "第22条第4款实用性判断",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "实用性分析不完整",
					CheckType:   CheckSubjectMatter,
					Domain:      "patent_examination",
					PathElements: [][]string{
						{"实用性", "工业实用性", "产业应用", "industrial applicability"},
						{"能够制造", "能够使用"},
						{"积极效果", "有益效果", "positive effect"},
					},
					FixSuggestion: "论证本领域技术人员能够在产业上制造或使用该发明，并产生积极效果",
				},
			},
		},
		{
			ID:               "RP-OTHER-04",
			Category:         "other",
			Name:             "整体视觉效果对比",
			Frequency:        0,
			InvalidationRate: 0,
			CoreLogic:        "外观设计对比以整体视觉效果为准，综合判断产品相同或相近种类的外观设计是否构成相同或近似，不进行局部细节的逐一比对",
			Template:         "产品种类：...，外观设计整体视觉效果：...，与对比设计...整体视觉效果相同/近似，局部差异...对整体视觉效果不产生显著影响",
			CheckRules: []CheckRule{
				{
					ID:          "REASON-OTHER-04",
					Name:        "整体视觉效果对比审查",
					Description: "外观设计四步推理结构：产品种类→整体视觉→相同/近似判断",
					Level:       LevelShould,
					Severity:    SeverityMajor,
					Message:     "外观设计对比分析不完整，缺少整体视觉效果判断",
					CheckType:   CheckDesignComparison,
					Domain:      "patent_design",
					PathElements: [][]string{
						{"外观设计", "工业设计", "design", "industrial design"},
						{"整体视觉效果", "视觉效果", "整体外观", "整体视觉", "overall visual effect"},
						{"产品种类", "产品类别", "同类产品", "相近种类"},
						{"相同", "近似", "实质相同"},
					},
					FixSuggestion: "按四步结构分析：确定产品种类→明确整体视觉效果→对比整体视觉效果→判断相同/近似",
				},
			},
		},
	}
}

// PatternsByCategory filters AllPatterns results by category.
// Valid categories: "creativity", "novelty", "claims", "other".
// An empty string returns all patterns.
func PatternsByCategory(category string) []ReasoningPattern {
	all := AllPatterns()
	if category == "" {
		return all
	}
	var out []ReasoningPattern
	for _, p := range all {
		if p.Category == category {
			out = append(out, p)
		}
	}
	return out
}
