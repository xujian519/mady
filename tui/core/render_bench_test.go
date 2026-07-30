package core

import (
	"strings"
	"testing"
)

func BenchmarkParseLine(b *testing.B) {
	// Medium-length line with ANSI colors, typical of LLM output.
	line := "\x1b[32mfunction\x1b[0m \x1b[33mfoo\x1b[0m() {\n    \x1b[31mreturn\x1b[0m \x1b[34m42\x1b[0m;\n}\n"
	b.ResetTimer()
	for b.Loop() {
		ParseLine(line)
	}
}

func BenchmarkParseLinePlain(b *testing.B) {
	// Plain text without ANSI (common for user messages).
	line := "This is a user message with some ~80 characters of plain text content for parsing"
	b.ResetTimer()
	for b.Loop() {
		ParseLine(line)
	}
}

func BenchmarkSerializeRow(b *testing.B) {
	row := ParseLine("\x1b[31mhello\x1b[0m \x1b[32mworld\x1b[0m")
	b.ResetTimer()
	for b.Loop() {
		SerializeRow(row)
	}
}

func BenchmarkDiffRows(b *testing.B) {
	old := make([]Row, 3)
	new := make([]Row, 3)
	old[0] = ParseLine("this is the original first line")
	old[1] = ParseLine("this is the original second line of text for diffing")
	old[2] = ParseLine("same third line")
	new[0] = ParseLine("this is the modified  first line")
	new[1] = ParseLine("this is the modified  second line of text  for diffing")
	new[2] = ParseLine("same third line")
	b.ResetTimer()
	for b.Loop() {
		DiffRows(old, new)
	}
}

func BenchmarkSanitizeRawBasic(b *testing.B) {
	// Input with dangerous OSC 8 hyperlink that should be stripped.
	input := "Click here: \x1b]8;;file:///etc/passwd\x1b\\link\x1b]8;;\x1b\\ more text"
	b.ResetTimer()
	for b.Loop() {
		SanitizeRawContent(input)
	}
}

func BenchmarkSanitizeClean(b *testing.B) {
	// Clean input with only legal SGR sequences.
	input := "\x1b[32mclean\x1b[0m \x1b[1mtext\x1b[0m"
	b.ResetTimer()
	for b.Loop() {
		SanitizeRawContent(input)
	}
}

func BenchmarkParseSGR(b *testing.B) {
	seq := "38;2;255;128;0;48;2;0;0;255;1;4"
	b.ResetTimer()
	for b.Loop() {
		ParseSGR(seq, DefaultStyle)
	}
}

func BenchmarkVisibleWidth(b *testing.B) {
	tests := []string{
		"short",
		"a longer sentence with more characters",
		strings.Repeat("中", 40), // CJK wide chars
		"\x1b[31mcolored\x1b[0m text",
	}
	b.ResetTimer()
	for b.Loop() {
		for _, s := range tests {
			VisibleWidth(s)
		}
	}
}
