package infringement

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"
)

// --- Helpers ---

func buildScopeInputText(input *InfringementInput) string {
	return fmt.Sprintf("## 专利权利要求\n%s\n\n## 说明书\n%s\n\n## 审查历史\n%s",
		input.PatentClaims,
		truncateText(input.PatentSpec, 3000),
		truncateText(input.ProsecutionHistory, 2000))
}

// toInputText serializes input for agent.Run(). Returns empty JSON object on error.
func toInputText(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		slog.Error("infringement: failed to marshal input for LLM agent", "err", err)
		return "{}"
	}
	return string(b)
}

// truncateText truncates s to at most maxRunes runes, preserving valid UTF-8.
// Unlike byte-level slicing, this correctly handles multi-byte characters (Chinese, etc.)
// and never produces invalid UTF-8 sequences.
func truncateText(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}

func joinLines(items []string) string {
	if len(items) == 0 {
		return "(无)"
	}
	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(". ")
		sb.WriteString(item)
		sb.WriteByte('\n')
	}
	return sb.String()
}
