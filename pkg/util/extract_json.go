// Package util provides general-purpose utility functions used across Mady.
package util

import "strings"

// ExtractJSON extracts the first JSON object from text.
//
// It first tries to find a JSON code block (```json ... ```), then falls back
// to the first '{' to the last '}' in the text. Returns "" when no JSON
// object is found.
func ExtractJSON(text string) string {
	text = strings.TrimSpace(text)

	// Prefer ```json ... ``` code blocks.
	const jsonFence = "```json"
	const fenceEnd = "```"
	if idx := strings.Index(text, jsonFence); idx >= 0 {
		rest := text[idx+len(jsonFence):]
		if end := strings.Index(rest, fenceEnd); end >= 0 {
			block := strings.TrimSpace(rest[:end])
			start := strings.Index(block, "{")
			end := strings.LastIndex(block, "}")
			if start >= 0 && end > start {
				return block[start : end+1]
			}
		}
	}

	// Fallback: first '{' to last '}'.
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}

// ExtractJSONSimple extracts the first '{' ... '}' JSON object from text.
//
// Unlike ExtractJSON, it does not attempt to find ```json code blocks.
// It is a simpler alternative for callers that know their input does not
// contain markdown fences.
func ExtractJSONSimple(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return text[start : end+1]
	}
	return ""
}
