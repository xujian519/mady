package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

// markdownWidthFixture is a self-contained Markdown sample covering the
// shapes that historically broke the renderer: CJK headings with quotes,
// horizontal tables with bold headers, blockquotes, lists, long CJK
// paragraphs without spaces, and emoji. It replaces the old repro tests'
// dependency on an absolute desktop file path (which silently skipped on
// other machines and in CI).
const markdownWidthFixture = `### "原说明书和权利要求书记载的范围"如何认定？

依据《专利审查指南》第 33 条：

> 包括**原说明书和权利要求书文字记载的内容** + **根据文字记载内容以及说明书附图能直接地、毫无疑义地确定的内容**。

| 允许修改的理由 | 限制修改的理由 |
|---|---|
| ① 申请人的表达和认知能力存在局限 | ① 促使申请人在申请阶段**充分公开**发明 |
| ② 提高申请文件质量，便于公众理解和运用 | ② 防止申请人**不正当取得先申请利益**（把申请日后的新内容补进去） |
| ③ 保障社会公众对专利信息的**信赖** |  |

- 第一点：人的表达认知能力存在局限，促使申请人在申请阶段充分公开
- 第二点：提高申请文件质量，便于公众理解和运用
- 第三点：保障社会公众对专利信息的信赖

①人的表达认知能力存在局限促使人在充分公开②提高质量便于公众理解和运用防止不正当取得利益把日后的新补进去③保障社会信息的信赖

✅ 完成 ⚠️ 注意 ❌ 错误 ⭐ 重点`

// assertWidths reports every line exceeding width as a test error.
func assertWidths(t *testing.T, lines []string, width int64) {
	t.Helper()
	for i, ln := range lines {
		if vw := core.VisibleWidth(ln); vw > width {
			t.Errorf("line %d exceeds width %d: visible=%d %q", i, width, vw, core.StripAnsi(ln))
		}
	}
}

// TestMarkdownRenderWidthInvariant locks the core invariant that prompted the
// repro tests: every rendered line must fit the viewport width at all common
// widths, including CJK-heavy content, tables, and emoji.
func TestMarkdownRenderWidthInvariant(t *testing.T) {
	for _, w := range []int64{60, 80, 100, 120} {
		md := NewMarkdown(markdownWidthFixture)
		md.SetTheme(DefaultMarkdownTheme())
		lines := md.Render(w)
		assertWidths(t, lines, w)
	}
}

// TestMarkdownIncrementalWidthInvariant renders the fixture one byte at a
// time through the streaming path and asserts every intermediate frame stays
// within width — the intermediate frames (with partial UTF-8 sequences and
// incomplete table separators) are exactly where the old streaming layout
// bugs lived. The final frame must also match a fresh full render.
func TestMarkdownIncrementalWidthInvariant(t *testing.T) {
	width := int64(80)
	cache := &BlockCache{}
	theme := DefaultMarkdownTheme()

	for i := 0; i < len(markdownWidthFixture); i++ {
		chunk := markdownWidthFixture[:i+1]
		lines := RenderMarkdownIncremental(chunk, width, theme, cache)
		assertWidths(t, lines, width)
	}

	// Final incremental render must agree with a fresh full render.
	incremental := RenderMarkdownIncremental(markdownWidthFixture, width, theme, &BlockCache{})
	full := NewMarkdown(markdownWidthFixture)
	full.SetTheme(theme)
	fresh := full.Render(width)
	if !sameLines(incremental, fresh) {
		t.Error("incremental final render differs from fresh full render")
	}
}

// TestMarkdownRenderSpecificShapes covers each problem shape individually at
// narrow widths where they used to overflow.
func TestMarkdownRenderSpecificShapes(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		width int64
	}{
		{"table", "| **维度** | **要点** |\n| --- | --- |\n| 新颖性 | 区别于现有技术 |", 20},
		{"quote", "> 包括**原说明书和权利要求书文字记载的内容** + **根据文字记载内容**。", 30},
		{"long-cjk", "①人的表达认知能力存在局限促使人在充分公开②提高质量便于公众理解和运用", 30},
		{"emoji", "✅ 完成 ⚠️ 注意 ❌ 错误 ⭐ 重点", 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			md := NewMarkdown(c.src)
			md.SetTheme(DefaultMarkdownTheme())
			assertWidths(t, md.Render(c.width), c.width)
		})
	}
}

// sameLines reports whether two render outputs are identical.
func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
