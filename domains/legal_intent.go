package domains

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/xujian519/mady/domains/reasoning"
)

// RunMode selects how a legal case should be processed.
type RunMode string

const (
	ModeDirect       RunMode = "direct"
	ModeJudgment     RunMode = "judgment"
	ModeFlexiblePlan RunMode = "flexible_plan"
)

// LegalIntentResult describes the detected legal intent in user input.
type LegalIntentResult struct {
	IsLegalIntent   bool
	SuggestedMode   RunMode
	CaseType        reasoning.CaseType
	Confidence      float64
	MatchedKeywords []string
	Suggestion      string
	ExplicitTrigger bool
}

type keywordPattern struct {
	keywords              []string
	caseType              reasoning.CaseType
	mode                  RunMode
	requiresPatentContext bool
}

var keywordPatterns = []keywordPattern{
	{
		keywords: []string{"无效", "宣告", "无效宣告", "无效请求"},
		caseType: reasoning.CaseInvalidation,
		mode:     ModeFlexiblePlan,
	},
	{
		keywords: []string{"侵权", "侵权分析", "侵权判断", "全面覆盖"},
		caseType: reasoning.CaseInfringement,
		mode:     ModeFlexiblePlan,
	},
	{
		keywords: []string{"新颖性", "新颖性判断"},
		caseType: reasoning.CaseNoveltySearch,
		mode:     ModeJudgment,
	},
	{
		keywords: []string{"创造性", "创造性判断", "三步法"},
		caseType: reasoning.CasePatentability,
		mode:     ModeJudgment,
	},
	{
		keywords: []string{"撰写", "专利申请", "写专利", "专利撰写"},
		caseType: reasoning.CaseDrafting,
		mode:     ModeFlexiblePlan,
	},
	{
		keywords:              []string{"OA", "审查意见", "答复", "OA答复", "审查意见通知书"},
		caseType:              reasoning.CaseOAResponse,
		mode:                  ModeFlexiblePlan,
		requiresPatentContext: true,
	},
	{
		keywords: []string{"驳回", "复审", "驳回复审"},
		caseType: reasoning.CaseReexamination,
		mode:     ModeFlexiblePlan,
	},
	{
		keywords: []string{"FTO", "自由实施", "自由实施分析"},
		caseType: reasoning.CaseFTO,
		mode:     ModeFlexiblePlan,
	},
	{
		keywords: []string{"充分公开", "公开不充分", "A26.3"},
		caseType: reasoning.CaseInvalidation,
		mode:     ModeJudgment,
	},
	{
		keywords:              []string{"清楚", "不清楚", "不支持", "A26.4"},
		caseType:              reasoning.CaseInvalidation,
		mode:                  ModeJudgment,
		requiresPatentContext: true,
	},
	{
		keywords: []string{"修改超范围", "超范围", "A33"},
		caseType: reasoning.CaseInvalidation,
		mode:     ModeJudgment,
	},
	{
		keywords: []string{"授权客体", "不授权", "A25", "专利客体"},
		caseType: reasoning.CasePatentability,
		mode:     ModeJudgment,
	},
	{
		keywords: []string{"等同", "等同原则", "等同侵权"},
		caseType: reasoning.CaseInfringement,
		mode:     ModeJudgment,
	},
	{
		keywords: []string{"商标", "商标查询", "商标检索", "商标申请"},
		caseType: reasoning.CaseGeneralLegal,
		mode:     ModeFlexiblePlan,
	},
	{
		keywords: []string{"专利法", "法条", "法律依据", "法律条款", "审查指南"},
		caseType: reasoning.CaseGeneralLegal,
		mode:     ModeFlexiblePlan,
	},
}

var patentContextSignals = []string{
	"权利要求", "专利", "说明书", "对比文件", "技术方案",
	"审查意见", "申请人", "专利权", "申请号", "公开号",
	"独立权利要求", "从属权利要求", "技术特征", "区别特征",
}

const legalPrefix = "@legal"

type matchedPattern struct {
	pat   keywordPattern
	count int
}

func detectExplicitTrigger(userInput string) (bool, string) {
	trimmed := strings.TrimLeft(userInput, " \t\r\n")
	if strings.HasPrefix(trimmed, legalPrefix) {
		rest := strings.TrimSpace(trimmed[len(legalPrefix):])
		return true, rest
	}
	return false, ""
}

func countKeywordMatches(input string, keywords []string) (int, []string) {
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)
	sort.Slice(sorted, func(i, j int) bool {
		return utf8.RuneCountInString(sorted[i]) > utf8.RuneCountInString(sorted[j])
	})

	count := 0
	var matchedLong []string
	for _, kw := range sorted {
		if !strings.Contains(input, strings.ToLower(kw)) {
			continue
		}
		skip := false
		kwLen := utf8.RuneCountInString(kw)
		for _, ml := range matchedLong {
			if utf8.RuneCountInString(ml)-kwLen <= 1 && strings.Contains(ml, kw) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		matchedLong = append(matchedLong, kw)
		count++
	}
	return count, matchedLong
}

// DetectLegalIntent analyzes user input and returns the detected legal intent.
func DetectLegalIntent(userInput string) LegalIntentResult {
	explicit, cleanedInput := detectExplicitTrigger(userInput)
	effectiveInput := userInput
	if explicit {
		effectiveInput = cleanedInput
	}
	input := strings.ToLower(effectiveInput)

	var matched []matchedPattern

	for _, pat := range keywordPatterns {
		count, _ := countKeywordMatches(input, pat.keywords)
		if count > 0 {
			matched = append(matched, matchedPattern{pat: pat, count: count})
		}
	}

	if explicit && len(matched) == 0 {
		return LegalIntentResult{
			IsLegalIntent:   true,
			SuggestedMode:   ModeFlexiblePlan,
			CaseType:        reasoning.CaseGeneralLegal,
			Confidence:      1,
			MatchedKeywords: []string{"@legal"},
			Suggestion:      "已通过 @legal 显式触发专业模式。",
			ExplicitTrigger: true,
		}
	}

	if len(matched) == 0 {
		return LegalIntentResult{
			IsLegalIntent: false,
			SuggestedMode: ModeDirect,
			Confidence:    0,
		}
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].count > matched[j].count
	})
	best := matched[0]

	var matchedKeywords []string
	for _, kw := range best.pat.keywords {
		if strings.Contains(input, strings.ToLower(kw)) {
			matchedKeywords = append(matchedKeywords, kw)
		}
	}
	if explicit {
		matchedKeywords = append([]string{"@legal"}, matchedKeywords...)
	}

	confidence := float64(best.count) / float64(len(best.pat.keywords))
	if confidence > 1 {
		confidence = 1
	}
	if explicit {
		confidence = 1
	}

	if best.pat.requiresPatentContext && !explicit {
		hasArticleID := false
		for _, kw := range best.pat.keywords {
			if strings.HasPrefix(strings.ToUpper(kw), "A") && len(kw) > 1 && strings.Contains(input, strings.ToLower(kw)) {
				hasArticleID = true
				break
			}
		}
		hasPatentContext := false
		for _, signal := range patentContextSignals {
			if strings.Contains(input, strings.ToLower(signal)) {
				hasPatentContext = true
				break
			}
		}
		hasOtherPattern := len(matched) >= 2

		if !hasArticleID && !hasPatentContext && !hasOtherPattern {
			return LegalIntentResult{
				IsLegalIntent: false,
				SuggestedMode: ModeDirect,
				Confidence:    0,
			}
		}
	}

	return LegalIntentResult{
		IsLegalIntent:   true,
		SuggestedMode:   best.pat.mode,
		CaseType:        best.pat.caseType,
		Confidence:      confidence,
		MatchedKeywords: matchedKeywords,
		Suggestion:      buildLegalSuggestion(best.pat.caseType, best.pat.mode, matchedKeywords),
		ExplicitTrigger: explicit,
	}
}

// SelectRunMode chooses the processing mode based on whether predefined steps
// exist for the case type and user preference. The caseType parameter is
// reserved for future per-case-type logic.
func SelectRunMode(_ reasoning.CaseType, hasPredefinedSteps bool, userPrefersFlexible bool) RunMode {
	if hasPredefinedSteps && !userPrefersFlexible {
		return ModeJudgment
	}
	return ModeFlexiblePlan
}

var caseTypeLabels = map[reasoning.CaseType]string{
	reasoning.CaseInvalidation:  "无效宣告分析",
	reasoning.CaseInfringement:  "侵权分析",
	reasoning.CaseNoveltySearch: "新颖性检索",
	reasoning.CasePatentability: "可专利性分析",
	reasoning.CaseDrafting:      "专利撰写",
	reasoning.CaseOAResponse:    "OA 答复",
	reasoning.CaseRejection:     "驳回答复",
	reasoning.CaseReexamination: "复审请求",
	reasoning.CaseFTO:           "自由实施分析",
	reasoning.CaseValidity:      "有效性评估",
	reasoning.CaseLegalStatus:   "法律状态查询",
	reasoning.CaseGeneralLegal:  "法律咨询",
}

func buildLegalSuggestion(caseType reasoning.CaseType, mode RunMode, keywords []string) string {
	modeLabel := "专业工作流模式"
	if mode == ModeJudgment {
		modeLabel = "专业判断模式"
	}
	label, ok := caseTypeLabels[caseType]
	if !ok {
		label = string(caseType)
	}
	return fmt.Sprintf("检测到 %s 意图（关键词: %s），建议进入%s。", label, strings.Join(keywords, ", "), modeLabel)
}
