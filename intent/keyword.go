package intent

import (
	"sort"
	"strings"
	"unicode/utf8"
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

	// Step 1: Domain classification (same logic as domains/router.go)
	domain := k.classifyDomain(lower)

	// Step 2: Sub-intent classification for patent/legal domains
	var subIntent SubIntent
	var runMode RunMode
	var matchedKeywords []string
	var suggestion string
	confidence := 1.0

	if domain == DomainPatent || domain == DomainLegal {
		subIntent, runMode, matchedKeywords, suggestion = k.classifySubIntent(lower)
		if len(matchedKeywords) == 0 {
			confidence = 0.6 // domain matched but no sub-intent
			subIntent = SubIntentGeneral
			runMode = ModeFlexiblePlan
		}
	}

	// Step 3: Complexity classification
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

// classifyDomain matches the input against domain-specific keywords.
func (k *KeywordClassifier) classifyDomain(lower string) Domain {
	// Patent keywords (highest priority to avoid legal keyword collision)
	patentKeywords := []string{
		"专利", "权利要求", "发明", "实用新型", "外观设计",
		"新颖性", "创造性", "实用性", "prior art", "现有技术",
		"patent", "invention", "claim", "IPC", "分类号",
		"pct", "巴黎公约", "优先权",
	}
	for _, kw := range patentKeywords {
		if strings.Contains(lower, kw) {
			return DomainPatent
		}
	}

	// Legal keywords
	legalKeywords := []string{
		"法律", "法条", "法规", "判例", "判决", "裁定",
		"诉讼", "起诉", "被告", "原告", "法院", "法官",
		"合同", "侵权", "赔偿", "证据", "仲裁",
		"刑法", "民法", "行政法", "公司法", "劳动法",
		"司法解释", "指导性案例",
		"law", "legal", "court", "statute", "regulation",
	}
	for _, kw := range legalKeywords {
		if strings.Contains(lower, kw) {
			return DomainLegal
		}
	}

	// Assistant keywords
	assistantKeywords := []string{
		"查一下", "帮我搜", "搜索", "检索", "查找",
		"起草", "生成", "写一个", "创建", "整理", "导出", "统计",
		"写代码", "实现一个", "调试", "优化", "重构",
		"代码", "编程", "python", "javascript", "go语言",
		"bash", "shell", "命令行", "脚本",
	}
	for _, kw := range assistantKeywords {
		if strings.Contains(lower, kw) {
			return DomainAssistant
		}
	}

	return DomainChat
}

// subIntentPattern describes a keyword pattern for sub-intent detection.
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

var patentContextSignals = []string{
	"权利要求", "专利", "说明书", "对比文件", "技术方案",
	"审查意见", "申请人", "专利权", "申请号", "公开号",
	"独立权利要求", "从属权利要求", "技术特征", "区别特征",
}

// classifySubIntent detects fine-grained intent from patent/legal input.
func (k *KeywordClassifier) classifySubIntent(lower string) (SubIntent, RunMode, []string, string) {
	type matched struct {
		pat   subIntentPattern
		count int
	}
	var matches []matched

	for _, pat := range subIntentPatterns {
		count, _ := countKeywordMatches(lower, pat.keywords)
		if count > 0 {
			matches = append(matches, matched{pat: pat, count: count})
		}
	}

	if len(matches) == 0 {
		return "", "", nil, ""
	}

	// Sort by match count descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].count > matches[j].count
	})
	best := matches[0]

	// Collect matched keywords
	var matchedKeywords []string
	for _, kw := range best.pat.keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			matchedKeywords = append(matchedKeywords, kw)
		}
	}

	// Check patent context requirement
	if best.pat.requiresPatentContext {
		hasArticleID := false
		for _, kw := range best.pat.keywords {
			upper := strings.ToUpper(kw)
			if strings.HasPrefix(upper, "A") && len(kw) > 1 && strings.Contains(lower, strings.ToLower(kw)) {
				hasArticleID = true
				break
			}
		}
		hasPatentContext := false
		for _, signal := range patentContextSignals {
			if strings.Contains(lower, strings.ToLower(signal)) {
				hasPatentContext = true
				break
			}
		}
		if !hasArticleID && !hasPatentContext && len(matches) < 2 {
			return SubIntentGeneral, ModeFlexiblePlan, matchedKeywords, ""
		}
	}

	return best.pat.subIntent, best.pat.mode, matchedKeywords, ""
}

// classifyComplexity determines reasoning complexity from input characteristics.
func (k *KeywordClassifier) classifyComplexity(lower string, original string) Complexity {
	// High complexity keywords
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

	// Length-based classification
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

// countKeywordMatches counts how many keywords from the list appear in input.
// Longer keywords are matched first to prevent substring false positives.
// Returns the count and matched long keywords for deduplication.
func countKeywordMatches(input string, keywords []string) (int, []string) {
	sorted := make([]string, len(keywords))
	copy(sorted, keywords)
	sort.Slice(sorted, func(i, j int) bool {
		return utf8.RuneCountInString(sorted[i]) > utf8.RuneCountInString(sorted[j])
	})

	count := 0
	var matchedLong []string
	for _, kw := range sorted {
		if !strings.Contains(input, kw) {
			continue
		}
		// Skip if a longer already-matched keyword contains this one
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
