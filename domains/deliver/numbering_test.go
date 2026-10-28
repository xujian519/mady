package deliver

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// renderNumbered 渲染一个带编号声明的片段，返回渲染后的 HTML。
func renderNumbered(t *testing.T, inner string) string {
	t.Helper()
	out, err := Render("claims-spec", map[string]string{
		"specification-body": `<section data-paragraph-numbering="[0001]">` + inner + `</section>`,
	}, RenderOptions{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

// 图与表格内的段落不编号，也不占号。
func TestNumberingSkipsFigureAndTable(t *testing.T) {
	out := renderNumbered(t, `<p>正文一。</p>
<figure><p>图注段落。</p></figure>
<table><tr><td><p>表格内段落。</p></td></tr></table>
<p>正文二。</p>`)
	if strings.Contains(out, "[0002] 图注段落") || strings.Contains(out, "[0003] 表格内段落") {
		t.Fatalf("图与表格内段落不应编号：%s", snippet(out))
	}
	if !strings.Contains(out, "[0002] 正文二") {
		t.Fatalf("图/表不占号，正文二应为 0002：%s", snippet(out))
	}
}

// 只有图片或空白的段落不占号。
func TestNumberingSkipsEmptyParagraphs(t *testing.T) {
	out := renderNumbered(t, `<p>正文一。</p><p><img src="x.png" alt=""></p><p>   </p><p>正文二。</p>`)
	if !strings.Contains(out, "[0002] 正文二") {
		t.Fatalf("空段与图片段不应占号：%s", snippet(out))
	}
}

// li 与其内 p 不重复编号。
func TestNumberingDoesNotNest(t *testing.T) {
	out := renderNumbered(t, `<ul><li><p>列表项一。</p></li><li>列表项二。</li></ul><p>正文二。</p>`)
	// 两个 li 各占一个号；li 内的 p 不再二次编号；正文为 0003。
	// 编号写在 li 的开头（其内 p 之前），因此断言标签前缀而不是连续文本。
	if !strings.Contains(out, "<li>[0001] ") || !strings.Contains(out, "<li>[0002] ") {
		t.Fatalf("两个列表项应依次编号：%s", snippet(out))
	}
	if strings.Contains(out, "<p>[0001] ") || strings.Contains(out, "<p>[0002] ") {
		t.Fatalf("列表项内的 p 不应二次编号：%s", snippet(out))
	}
	if !strings.Contains(out, "[0003] 正文二") {
		t.Fatalf("两个列表项后正文应为 0003：%s", snippet(out))
	}
}

// 段落开头已有的同形态编号先剥掉再重写，重复渲染不叠加。
func TestNumberingStripsExisting(t *testing.T) {
	out := renderNumbered(t, `<p>[0009] 已带编号的段落。</p><p>另一段。</p>`)
	if strings.Contains(out, "[0009]") {
		t.Fatalf("既有编号应被剥掉：%s", snippet(out))
	}
	if !strings.Contains(out, "[0001] 已带编号的段落") || !strings.Contains(out, "[0002] 另一段") {
		t.Fatalf("应按序重编号：%s", snippet(out))
	}
}

// 证据编号左侧不能是 ASCII 字母数字：END2 不是 D2 的引用。
func TestEvidencePrefixBoundary(t *testing.T) {
	htmlDoc := `<html><body><p>见 END2 字段，以及证据1。</p>
<table><thead><tr><th>编号</th><th>标识</th><th>证据名称</th></tr></thead>
<tbody><tr><td>D2</td><td>CN1</td><td>对比文件</td></tr>
<tr><td>证据1</td><td>CN2</td><td>教科书</td></tr></tbody></table></body></html>`
	issues, _ := CheckCompliance(htmlDoc, ScopeExternalFiling)
	// 若 END2 被误当成 D2 的引用，D2 就不会报「未被引用」——这正是要防的边界。
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "「D2」在正文中未被引用") {
			found = true
		}
	}
	if !found {
		t.Fatalf("D2 未引用应报出：%+v", issues)
	}
}

// 正文判定不含证据清单表本身的文字。
func TestEvidenceTableExcludedFromBody(t *testing.T) {
	htmlDoc := `<html><body><p>主张成立。</p>
<table><thead><tr><th>编号</th><th>标识</th><th>证据名称</th></tr></thead>
<tbody><tr><td>证据1</td><td>CN1</td><td>对比文件</td></tr></tbody></table></body></html>`
	issues, _ := CheckCompliance(htmlDoc, ScopeExternalFiling)
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "「证据1」在正文中未被引用") {
			found = true
		}
	}
	if !found {
		t.Fatalf("清单内文字不应被当作正文引用：%+v", issues)
	}
}

// 仅用于确认 html 包被使用（编号测试直接解析片段时）。
var _ = html.Parse
