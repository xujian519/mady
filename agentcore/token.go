package agentcore

import (
	"encoding/json"
	"unicode"
)

// EstimateTokens returns a rough token count.
//
// Base heuristic is chars/4 (good for ASCII/English/code). For CJK text
// (Chinese, Japanese, Korean) each character typically costs 1-2 tokens in
// mainstream tokenizers (BPE/SentencePiece), but UTF-8 encoding means len()
// counts 3 bytes per CJK character — yielding only ~0.75 tokens/char from the
// chars/4 base. We add a correction factor so Chinese-heavy conversations
// trigger compaction at the right time instead of silently overflowing.
func EstimateTokens(text string) int64 {
	if len(text) == 0 {
		return 0
	}
	base := int64(len(text)+3) / 4
	// CJK correction: add ~0.75 tokens per CJK rune to reach ~1.5 tokens/char
	// (midpoint of the 1-2 range observed in mainstream tokenizers).
	base += int64(float64(countCJKRunes(text)) * 0.75)
	return base
}

// countCJKRunes counts Unicode CJK characters in a string.
// Covers CJK Unified Ideographs, Hiragana, Katakana, Hangul, and CJK Extension A.
//
// Fast-path: if no byte ≥ 0x80 is present, the string is pure ASCII and
// contains zero CJK characters — we skip the rune iteration entirely.
// This keeps EstimateTokens O(len) only for strings that actually contain
// multi-byte characters.
func countCJKRunes(s string) int {
	// ASCII fast-path: any CJK character is encoded as multi-byte UTF-8 (≥ 0x80).
	hasNonASCII := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			hasNonASCII = true
			break
		}
	}
	if !hasNonASCII {
		return 0
	}
	count := 0
	for _, r := range s {
		if isCJK(r) {
			count++
		}
	}
	return count
}

// isCJK reports whether a rune is a CJK ideograph or Japanese/Korean syllabary.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) || // CJK Unified Ideographs + extensions
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// EstimateMessageTokens estimates token count for a single message,
// including role overhead, content, and tool call payloads.
func EstimateMessageTokens(msg Message) int64 {
	tokens := int64(4) // role + message framing overhead
	tokens += EstimateTokens(MessageTextBody(msg))
	tokens += EstimateTokens(msg.Name)
	for _, tc := range msg.ToolCalls {
		tokens += EstimateTokens(tc.Name)
		tokens += EstimateTokens(tc.Arguments)
		tokens += 4 // per-tool-call overhead
	}
	return tokens
}

// EstimateMessagesTokens estimates total token count for a slice of messages.
func EstimateMessagesTokens(msgs []Message) int64 {
	total := int64(3) // conversation framing overhead
	for _, msg := range msgs {
		total += EstimateMessageTokens(msg)
	}
	return total
}

// EstimateToolDefinitionsTokens estimates token overhead of tool definitions
// in the request (they count against the context window).
func EstimateToolDefinitionsTokens(defs []ToolDefinition) int64 {
	if len(defs) == 0 {
		return 0
	}
	total := int64(0)
	for _, def := range defs {
		total += EstimateTokens(def.Name)
		total += EstimateTokens(def.Description)
		if def.Parameters != nil {
			if data, err := json.Marshal(def.Parameters); err == nil {
				total += EstimateTokens(string(data))
			}
		}
		total += 10 // per-tool overhead (type, function wrapper, etc.)
	}
	return total
}
