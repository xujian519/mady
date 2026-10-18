package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetStyleCache 清空 LoadDefaultStyles 的包级缓存（仅测试使用）。
func resetStyleCache() {
	cachedStyles = nil
	cachedStylesErr = nil
	cachedStylesOnce = sync.Once{}
}

// withStylePaths 临时替换 defaultStylePaths 并重置缓存，测试结束后恢复。
func withStylePaths(t *testing.T, paths []string) {
	t.Helper()
	oldPaths := defaultStylePaths
	defaultStylePaths = paths
	resetStyleCache()
	t.Cleanup(func() {
		defaultStylePaths = oldPaths
		resetStyleCache()
	})
}

// writeStyleFile 在 dir 下写一个合法的风格 YAML 文件。
func writeStyleFile(t *testing.T, dir, name, domain, disclaimer string) {
	t.Helper()
	content := "name: " + name + "\n" +
		"domain: " + domain + "\n" +
		"version: \"1.0\"\n" +
		"sections:\n" +
		"  tone:\n" +
		"    formality: professional\n" +
		"  disclaimers:\n" +
		"    patent_analysis: \"" + disclaimer + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestStyleInjection_MatchingDomain 验证命中域时返回以空行开头的注入块，
// 且块内包含该风格的声明内容。这是 patent/legal 两个 Agent 的 system prompt
// 实际消费的路径。
func TestStyleInjection_MatchingDomain(t *testing.T) {
	dir := t.TempDir()
	writeStyleFile(t, dir, "patent-standard", "patent", "本分析由 AI 辅助生成")
	withStylePaths(t, []string{dir})

	got := StyleInjection("patent")
	if got == "" {
		t.Fatal("StyleInjection(patent) = \"\", want non-empty injection block")
	}
	if !strings.HasPrefix(got, "\n\n") {
		t.Fatalf("StyleInjection should prepend a blank line, got %q", got[:min(8, len(got))])
	}
	if !strings.Contains(got, "patent-standard") {
		t.Fatalf("injection block missing style name: %q", got)
	}
	if !strings.Contains(got, "本分析由 AI 辅助生成") {
		t.Fatalf("injection block missing disclaimer: %q", got)
	}
}

// TestStyleInjection_NoMatchingDomain 验证域不匹配时返回空串，而不是退化为
// 注入任意风格。
func TestStyleInjection_NoMatchingDomain(t *testing.T) {
	dir := t.TempDir()
	writeStyleFile(t, dir, "patent-standard", "patent", "本分析由 AI 辅助生成")
	withStylePaths(t, []string{dir})

	if got := StyleInjection("chat"); got != "" {
		t.Fatalf("StyleInjection(chat) = %q, want empty", got)
	}
}

// TestStyleInjection_NoStylesAvailable 验证无可用风格文件时返回空串——风格注入
// 是尽力而为的增强，不得阻断 Agent 启动。
func TestStyleInjection_NoStylesAvailable(t *testing.T) {
	withStylePaths(t, []string{filepath.Join(t.TempDir(), "nonexistent")})

	if got := StyleInjection("patent"); got != "" {
		t.Fatalf("StyleInjection with no style files = %q, want empty", got)
	}
}

// TestStyleInjection_FirstMatchWins 验证同域多个风格时取第一个（与
// StylesForDomain 的顺序语义一致）。
func TestStyleInjection_FirstMatchWins(t *testing.T) {
	dir := t.TempDir()
	writeStyleFile(t, dir, "a-patent", "patent", "第一个风格")
	writeStyleFile(t, dir, "b-patent", "patent", "第二个风格")
	withStylePaths(t, []string{dir})

	got := StyleInjection("patent")
	if !strings.Contains(got, "第一个风格") {
		t.Fatalf("StyleInjection should pick the first matching style, got %q", got)
	}
	if strings.Contains(got, "第二个风格") {
		t.Fatalf("StyleInjection must inject only one style, got %q", got)
	}
}
