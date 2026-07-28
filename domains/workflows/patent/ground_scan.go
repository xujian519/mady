// Package patent — shared text-scanning helpers used by invalidation,
// reexamination, and other workflows that identify legal grounds from text.
package patent

import "strings"

// groundPattern is a data-driven rule for identifying a legal ground from text.
// Each workflow defines its own []groundPattern table; scanGrounds does the
// matching. TypeKey is a string that the caller casts to its own ~string enum.
type groundPattern struct {
	Patterns []string // keywords/phrases to match (case-insensitive)
	Article  string   // legal article reference
	Desc     string   // human-readable description
	TypeKey  string   // caller-specific type identifier (cast to ~string enum)
}

// scanGrounds scans text against a pattern table and returns matched patterns
// (deduplicated, in table order). The caller maps TypeKey to its own domain type.
func scanGrounds(text string, rules []groundPattern) []groundPattern {
	lower := strings.ToLower(text)
	seen := make(map[int]bool)
	var matched []groundPattern
	for i, r := range rules {
		if seen[i] {
			continue
		}
		for _, p := range r.Patterns {
			if strings.Contains(lower, strings.ToLower(p)) {
				matched = append(matched, r)
				seen[i] = true
				break
			}
		}
	}
	return matched
}

// truncate returns at most n runes of s, appending "…" if truncated.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
