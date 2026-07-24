// Package util provides shared utilities used across Mady.
//
// String matching functions are gathered here to avoid duplication across
// domains/case_classifier.go, domains/evidence/engine.go, and
// domains/orchestration_bridge.go.
package util

import "strings"

// ContainsAny reports whether s contains any of the given keywords.
// Matching is case-insensitive: "A22.3" matches "a22.3".
// Use for LLM-produced output where casing is unpredictable.
//
// When case-sensitive matching is needed, use strings.Contains directly.
func ContainsAny(s string, keywords ...string) bool {
	lower := strings.ToLower(s)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// ContainsAnyFold is an alias for ContainsAny; prefer ContainsAny.
func ContainsAnyFold(s string, keywords ...string) bool {
	return ContainsAny(s, keywords...)
}

// ContainsRejectionKeyword reports whether s contains any standard
// patent rejection keywords: 创造性, 新颖性, 充分公开, 22.2, 22.3, etc.
func ContainsRejectionKeyword(s string) bool {
	return ContainsAny(s,
		"创造性", "新颖性", "充分公开", "不清楚", "不支持",
		"22.2", "22.3", "26.3", "26.4", "33条", "A33",
	)
}
