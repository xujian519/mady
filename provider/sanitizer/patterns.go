// Package sanitizer provides a transparent Provider wrapper that sanitizes
// PII (personally identifiable information) from LLM requests and restores
// it in responses.
//
// Designed for the Mady agent runtime: wraps any agentcore.Provider to
// automatically detect and replace structured PII (Chinese ID numbers, phone
// numbers, bank card numbers, email addresses) before forwarding to the LLM,
// then restores the original values in the response.
//
// Principle: precise regex matching only — no NLP-based name/address detection.
// Misclassification risk is minimal because we only match well-defined patterns.
package sanitizer

import (
	"fmt"
	"regexp"
	"sync"
)

// Rule defines a single PII detection and replacement rule.
type Rule struct {
	// Name is a human-readable category name, e.g. "身份证号".
	Name string
	// Pattern matches PII values in text. All matches are replaced.
	Pattern *regexp.Regexp
	// Placeholder is the fixed replacement text for this rule.
	Placeholder string
}

// defaultRules returns the built-in PII detection rules.
// Rules are applied in order; earlier rules take precedence for overlapping matches.
//
// Current rules:
//   - Chinese 18-digit ID number (公民身份号码)
//   - Mainland China mobile phone number (手机号)
//   - Bank card number (银行卡号)
//   - Email address (电子邮箱)
//
// Intentionally not matching:
//   - Chinese names (too high misclassification risk in legal text)
//   - Street addresses (too varied, no reliable regex)
//   - Passport / HK-Macau-Taiwan permit numbers (low prevalence in context)
func defaultRules() []Rule {
	return []Rule{
		{
			Name:        "身份证号",
			Pattern:     regexp.MustCompile(`\b[1-9]\d{5}(?:19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`),
			Placeholder: "[**身份证号**]",
		},
		{
			Name:        "手机号",
			Pattern:     regexp.MustCompile(`\b1[3-9]\d{9}\b`),
			Placeholder: "[**手机号**]",
		},
		{
			Name:        "银行卡号",
			Pattern:     regexp.MustCompile(`\b\d{16,19}\b`),
			Placeholder: "[**银行卡号**]",
		},
		{
			Name:        "电子邮箱",
			Pattern:     regexp.MustCompile(`\b[\w.+-]+@[\w-]+\.[\w.]+\b`),
			Placeholder: "[**电子邮箱**]",
		},
	}
}

// replacementMap tracks placeholder→original mappings for one request.
// It supports multiple occurrences of the same rule type; each match in the
// request text gets a unique sequential ID so that restoreResponse can
// correctly map each placeholder back to its original value.
type replacementMap struct {
	mu      sync.Mutex
	counter map[string]int    // rule name → how many matches seen
	entries map[string]string // e.g. "[**身份证号#1**]" → "110101199001011234"
}

func newReplacementMap() *replacementMap {
	return &replacementMap{
		counter: make(map[string]int),
		entries: make(map[string]string),
	}
}

// register records one PII match and returns the unique placeholder for it.
func (rm *replacementMap) register(ruleName string, original string) string {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.counter[ruleName]++
	seq := rm.counter[ruleName]
	// Use a unique placeholder: category + sequence number.
	placeholder := fmt.Sprintf("[**%s#%d**]", ruleName, seq)
	rm.entries[placeholder] = original
	return placeholder
}

// restore replaces all known placeholders in text with their original values.
func (rm *replacementMap) restore(text string) string {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for placeholder, original := range rm.entries {
		// ReplaceAll: a placeholder appears at most once per rule sequence.
		text = regexp.MustCompile(regexp.QuoteMeta(placeholder)).ReplaceAllLiteralString(
			text, original,
		)
	}
	return text
}

// sanitizeText replaces all PII matches with unique numbered placeholders.
// It registers each replaced value in the replacement map for later restoration.
func sanitizeText(text string, rules []Rule, rm *replacementMap) string {
	if text == "" {
		return ""
	}
	result := text
	for _, rule := range rules {
		result = rule.Pattern.ReplaceAllStringFunc(result, func(match string) string {
			placeholder := rm.register(rule.Name, match)
			return placeholder
		})
	}
	return result
}
