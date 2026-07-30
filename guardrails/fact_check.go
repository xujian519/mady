package guardrails

import (
	"fmt"
	"regexp"
	"strings"
)

// FactCheckRule checks output for obviously false or unsupported factual claims.
// It uses heuristic pattern matching rather than LLM-based verification to
// avoid adding latency. For production use with LLM-backed verification,
// inject a Provider via WithFactCheckProvider.
//
// Current checks:
//   - Non-existent law articles (e.g., "专利法第99条" when max is 82)
//   - Absolute claims without evidence markers
type FactCheckRule struct {
	// MaxArticles maps statute names to their maximum article numbers.
	// Default includes Chinese patent law (82 articles).
	MaxArticles map[string]int

	// MaxFactualErrors is the maximum number of suspect claims before blocking.
	// Default: 3.
	MaxFactualErrors int

	// BlockOnViolation when true causes ActionBlock instead of ActionInject.
	BlockOnViolation bool
}

// NewFactCheckRule creates a FactCheckRule with sensible defaults.
func NewFactCheckRule() *FactCheckRule {
	return &FactCheckRule{
		MaxArticles: map[string]int{
			"专利法":   82,
			"商标法":   72,
			"著作权法":  60,
			"民法典":   1260,
			"刑法":    452,
			"民事诉讼法": 284,
			"行政诉讼法": 103,
		},
		MaxFactualErrors: 3,
	}
}

// Name returns the rule identifier.
func (r *FactCheckRule) Name() string { return "fact-check" }

// articlePattern matches law citations like "专利法第99条", "《专利法》第26条".
// The statute names are derived from MaxArticles keys at construction time
// to keep the regex and article limits in sync.
var articlePattern *regexp.Regexp

func init() {
	// Build the regex from the known statute names.
	rule := NewFactCheckRule()
	statutes := make([]string, 0, len(rule.MaxArticles))
	for name := range rule.MaxArticles {
		statutes = append(statutes, regexp.QuoteMeta(name))
	}
	// Match: optionally 《STATUTE》 followed by 第N条 optionally 之M or 第N款
	pattern := `(?:《)?(` + strings.Join(statutes, "|") + `)(?:》)?第(\d+)条(?:之(\d+))?(?:第(\d+)款)?`
	articlePattern = regexp.MustCompile(pattern)
}

// absoluteClaimPatterns are phrases that suggest an absolute claim
// without supporting citation.
var absoluteClaimPatterns = []string{
	"毫无疑问",
	"显然",
	"毋庸置疑",
	"铁证如山",
}

// Check implements Rule.
func (r *FactCheckRule) Check(content string, metadata map[string]any) RuleResult {
	suspects := r.findSuspectArticles(content)
	suspects = append(suspects, r.findAbsoluteClaims(content)...)

	if len(suspects) == 0 {
		return RuleResult{Passed: true}
	}

	maxErrs := r.MaxFactualErrors
	if maxErrs <= 0 {
		maxErrs = 3
	}

	action := ActionInject
	if r.BlockOnViolation && len(suspects) > maxErrs {
		action = ActionBlock
	}

	msg := "检测到以下可能存在的事实问题：\n"
	for i, s := range suspects {
		if i >= 5 { // limit to 5 items in message
			msg += fmt.Sprintf("  ... 及其他 %d 项\n", len(suspects)-5)
			break
		}
		msg += fmt.Sprintf("  • %s\n", s)
	}
	if action == ActionBlock {
		msg = "回复因事实准确性检查未通过被拦截。\n" + msg
	}

	return RuleResult{
		Passed:   false,
		Severity: SeverityError,
		Action:   action,
		Message:  msg,
	}
}

// findSuspectArticles checks for law articles that exceed the known maximum.
func (r *FactCheckRule) findSuspectArticles(content string) []string {
	var suspects []string
	matches := articlePattern.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		statute := m[1]
		articleNum := 0
		fmt.Sscanf(m[2], "%d", &articleNum) //nolint:errcheck,gosec // G104: best-effort parse; zero on error parse; zero on error

		maxArt, known := r.MaxArticles[statute]
		if !known {
			continue
		}
		if articleNum > maxArt {
			suspects = append(suspects,
				fmt.Sprintf("%s第%d条 — 该法仅有%d条，此引用可能错误", statute, articleNum, maxArt))
		}
	}
	return suspects
}

// findAbsoluteClaims finds absolute statements without evidence markers.
func (r *FactCheckRule) findAbsoluteClaims(content string) []string {
	lower := strings.ToLower(content)
	var suspects []string
	for _, pattern := range absoluteClaimPatterns {
		if strings.Contains(lower, pattern) {
			// Check if there's a nearby citation or evidence marker
			idx := strings.Index(lower, pattern)
			context := lower[max(0, idx-30):min(len(lower), idx+len(pattern)+50)]
			if !strings.Contains(context, "参见") &&
				!strings.Contains(context, "依据") &&
				!strings.Contains(context, "参考") {
				suspects = append(suspects,
					fmt.Sprintf("使用了绝对化表述「%s」但无引用支撑", pattern))
			}
		}
	}
	return suspects
}
