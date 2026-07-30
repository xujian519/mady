package intent

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/xujian519/mady/internal/intentrules"
)

// KeywordClassifier classifies user input using deterministic keyword matching.
// It covers both domain routing (chat/patent/legal/assistant) and sub-intent
// detection (case type + run mode) in a single pass.
type KeywordClassifier struct{}

// NewKeywordClassifier creates a KeywordClassifier.
func NewKeywordClassifier() *KeywordClassifier {
	return &KeywordClassifier{}
}

// Name returns the classifier identifier.
func (k *KeywordClassifier) Name() string { return "keyword" }

// Classify implements Classifier using multi-level keyword matching.
func (k *KeywordClassifier) Classify(input string) IntentResult {
	lower := strings.ToLower(input)

	// Step 1: Domain classification using shared keyword lists.
	domain := k.classifyDomain(lower)

	// Step 2: Sub-intent classification for patent/legal domains.
	var subIntent SubIntent
	var runMode RunMode
	var matchedKeywords []string
	var suggestion string
	confidence := 1.0

	if domain == DomainPatent || domain == DomainLegal {
		subIntent, runMode, matchedKeywords, suggestion = k.classifySubIntent(lower)
		if len(matchedKeywords) == 0 {
			confidence = 0.6
			subIntent = SubIntentGeneral
			runMode = ModeFlexiblePlan
		}
	}

	// Step 3: Complexity classification.
	complexity := k.classifyComplexity(lower, input)

	return IntentResult{
		Domain:          domain,
		SubIntent:       subIntent,
		RunMode:         runMode,
		Complexity:      complexity,
		Confidence:      confidence,
		Sources:         []string{"keyword"},
		MatchedKeywords: matchedKeywords,
		Suggestion:      suggestion,
	}
}

// classifyDomain matches the input against shared domain-specific keyword lists.
func (k *KeywordClassifier) classifyDomain(lower string) Domain {
	for _, kw := range intentrules.PatentKeywords {
		if strings.Contains(lower, kw) {
			return DomainPatent
		}
	}
	for _, kw := range intentrules.LegalKeywords {
		if strings.Contains(lower, kw) {
			return DomainLegal
		}
	}
	for _, kw := range intentrules.AssistantKeywords {
		if strings.Contains(lower, kw) {
			return DomainAssistant
		}
	}
	return DomainChat
}

// subIntentPattern describes a keyword pattern for sub-intent detection.
// NOTE: These patterns overlap significantly with domains/legal_intent.go's
// keywordPatterns. The two lists should be kept in sync manually until a
// shared schema is designed. See B1 in code review report.
type subIntentPattern struct {
	keywords              []string
	subIntent             SubIntent
	mode                  RunMode
	requiresPatentContext bool
}

var subIntentPatterns = []subIntentPattern{
	{
		keywords:  []string{"无效", "宣告", "无效宣告", "无效请求"},
		subIntent: SubIntentInvalidation,
		mode:      ModeFlexiblePlan,
	},
	{
		keywords:  []string{"侵权", "侵权分析", "侵权判断", "全面覆盖"},
		subIntent: SubIntentInfringement,
		mode:      ModeFlexiblePlan,
	},
	{
		keywords:  []string{"新颖性", "新颖性判断"},
		subIntent: SubIntentNovelty,
		mode:      ModeJudgment,
	},
	{
		keywords:  []string{"创造性", "创造性判断", "三步法"},
		subIntent: SubIntentInventiveness,
		mode:      ModeJudgment,
	},
	{
		keywords:  []string{"撰写", "专利申请", "写专利", "专利撰写"},
		subIntent: SubIntentDrafting,
		mode:      ModeFlexiblePlan,
	},
	{
		keywords:              []string{"OA", "审查意见", "答复", "OA答复", "审查意见通知书"},
		subIntent:             SubIntentOAResponse,
		mode:                  ModeFlexiblePlan,
		requiresPatentContext: true,
	},
	{
		keywords:  []string{"驳回", "复审", "驳回复审"},
		subIntent: SubIntentReexamination,
		mode:      ModeFlexiblePlan,
	},
	{
		keywords:  []string{"FTO", "自由实施", "自由实施分析"},
		subIntent: SubIntentFTO,
		mode:      ModeFlexiblePlan,
	},
	{
		keywords:  []string{"充分公开", "公开不充分", "A26.3", "26.3"},
		subIntent: SubIntentEnablement,
		mode:      ModeJudgment,
	},
	{
		keywords:              []string{"清楚", "不清楚", "不支持", "A26.4"},
		subIntent:             SubIntentInvalidation,
		mode:                  ModeJudgment,
		requiresPatentContext: true,
	},
	{
		keywords:  []string{"修改超范围", "超范围", "A33"},
		subIntent: SubIntentInvalidation,
		mode:      ModeJudgment,
	},
	{
		keywords:  []string{"等同", "等同原则", "等同侵权"},
		subIntent: SubIntentInfringement,
		mode:      ModeJudgment,
	},
}

// classifySubIntent detects fine-grained intent from patent/legal input.
func (k *KeywordClassifier) classifySubIntent(lower string) (SubIntent, RunMode, []string, string) {
	type matched struct {
		pat   subIntentPattern
		count int
	}
	var matches []matched

	for _, pat := range subIntentPatterns {
		count, _ := intentrules.CountKeywordMatches(lower, pat.keywords)
		if count > 0 {
			matches = append(matches, matched{pat: pat, count: count})
		}
	}

	if len(matches) == 0 {
		return "", "", nil, ""
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].count > matches[j].count
	})
	best := matches[0]

	var matchedKeywords []string
	for _, kw := range best.pat.keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			matchedKeywords = append(matchedKeywords, kw)
		}
	}

	if best.pat.requiresPatentContext {
		hasArticleID := false
		for _, kw := range best.pat.keywords {
			upper := strings.ToUpper(kw)
			if strings.HasPrefix(upper, "A") && len(kw) > 1 && strings.Contains(lower, strings.ToLower(kw)) {
				hasArticleID = true
				break
			}
		}
		hasPatentContext := intentrules.MatchAnyKeyword(lower, intentrules.PatentContextSignals)
		if !hasArticleID && !hasPatentContext && len(matches) < 2 {
			return SubIntentGeneral, ModeFlexiblePlan, matchedKeywords, ""
		}
	}

	return best.pat.subIntent, best.pat.mode, matchedKeywords, ""
}

// classifyComplexity determines reasoning complexity from input characteristics.
func (k *KeywordClassifier) classifyComplexity(lower string, original string) Complexity {
	highKeywords := []string{
		"分析", "推理", "对比", "论证", "侵权", "新颖性", "创造性",
		"专利", "法律", "debug", "调试", "重构", "架构",
		"analyze", "troubleshoot", "architect", "design",
	}
	for _, kw := range highKeywords {
		if strings.Contains(lower, kw) {
			return ComplexityHigh
		}
	}

	runes := utf8.RuneCountInString(original)
	switch {
	case runes > 800:
		return ComplexityHigh
	case runes > 200:
		return ComplexityMedium
	default:
		return ComplexityLow
	}
}
