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
