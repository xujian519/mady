package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/retrieval/domain"
)

// writeMockEgoBrowser 生成一个 mock ego-browser 可执行脚本：
// 无论输入什么 heredoc，stdout 都输出固定 JSON（用于隔离测试解析与装配逻辑，
// 不依赖真实浏览器）。
func writeMockEgoBrowser(t *testing.T, out string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ego-browser")
	// 用 shell 包装：忽略 stdin，输出固定 JSON。
	script := "#!/bin/sh\nprintf '%s\\n' " + shellQuote(out) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // G306: 测试 mock 脚本
		t.Fatal(err)
	}
	return bin
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

const searchJSON = `[
  {"title": "测试专利一", "meta": "CN • CN106599773B • 马惠敏 • 清华大学", "dateLine": "Priority 2016-10-31 • Filed 2016-10-31 • Granted 2019-12-24 • Published 2019-12-24", "abstract": "本发明公开了…", "number": "CN106599773B", "pdfUrl": "https://patentimages.storage.googleapis.com/x/CN106599773B.pdf", "url": "", "itemId": "patent/CN106599773B"},
  {"title": "测试专利二", "meta": "CN • CN108734104B • 方航 • 杭州易舞科技有限公司", "dateLine": "Priority 2018-04-20 • Filed 2018-04-20", "abstract": "本发明公开了一种…", "number": "CN108734104B", "pdfUrl": "", "url": "", "itemId": "patent/CN108734104B"}
]`

func testConfig(t *testing.T, out string) BrowserRetrieverConfig {
	t.Helper()
	return BrowserRetrieverConfig{
		EgoBrowserPath: writeMockEgoBrowser(t, out),
	}
}

// TestNewBrowserRetrieverUnavailable 验证 ego-browser 缺失时工厂返回 nil。
func TestNewBrowserRetrieverUnavailable(t *testing.T) {
	cfg := BrowserRetrieverConfig{}
	if r := NewGooglePatentsRetriever(cfg); r != nil {
		t.Fatal("expected nil retriever when ego-browser is unavailable")
	}
}

// TestNewBrowserRetrieverAvailable 验证 ego-browser 可用时构造成功。
func TestNewBrowserRetrieverAvailable(t *testing.T) {
	cfg := testConfig(t, searchJSON)
	if r := NewGooglePatentsRetriever(cfg); r == nil {
		t.Fatal("expected non-nil retriever")
	}
}

// TestSearch 验证 Search 解析搜索结果并归一化 DomainDocument。
func TestSearch(t *testing.T) {
	r := NewGooglePatentsRetriever(testConfig(t, searchJSON))
	if r == nil {
		t.Fatal("retriever is nil")
	}

	ctx := context.Background()
	results, err := r.Search(ctx, domain.DomainQuery{
		Text:       "深度学习",
		Keywords:   []string{"图像识别"},
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results.Source != SourceNameGoogle {
		t.Errorf("source = %q, want %q", results.Source, SourceNameGoogle)
	}
	if len(results.Documents) != 2 {
		t.Fatalf("documents = %d, want 2", len(results.Documents))
	}
	d := results.Documents[0]
	if d.ID != "CN106599773B" {
		t.Errorf("ID = %q", d.ID)
	}
	if d.Title != "测试专利一" {
		t.Errorf("Title = %q", d.Title)
	}
	if d.Metadata["assignee"] != "清华大学" {
		t.Errorf("assignee = %q", d.Metadata["assignee"])
	}
	if d.Metadata["inventors"] != "马惠敏" {
		t.Errorf("inventors = %q", d.Metadata["inventors"])
	}
	if d.Metadata["country"] != "CN" {
		t.Errorf("country = %q", d.Metadata["country"])
	}
	if d.Metadata["pdf_url"] == "" {
		t.Error("pdf_url should not be empty for the first result")
	}
	// 排名靠前的结果分数更高。
	if !(results.Documents[0].Score > results.Documents[1].Score) {
		t.Error("first result should score higher than the second")
	}
	// URL 应回退为详情页 URL（页面 anchor 是 # 片段时）。
	if !strings.Contains(d.URL, "patents.google.com/patent/CN106599773B") {
		t.Errorf("URL = %q", d.URL)
	}
}

// TestSearchEmptyQuery 验证空查询返回空结果而非报错。
func TestSearchEmptyQuery(t *testing.T) {
	r := NewGooglePatentsRetriever(testConfig(t, searchJSON))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	results, err := r.Search(context.Background(), domain.DomainQuery{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results.Documents) != 0 {
		t.Errorf("documents = %d, want 0", len(results.Documents))
	}
}

// TestSearchInvalidOutput 验证非 JSON 输出返回错误。
func TestSearchInvalidOutput(t *testing.T) {
	r := NewGooglePatentsRetriever(testConfig(t, "not json at all"))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	if _, err := r.Search(context.Background(), domain.DomainQuery{Text: "x"}); err == nil {
		t.Fatal("expected error for invalid JSON output")
	}
}

// TestSearchGarbagePrefix 验证 stdout 含辅助输出时仍能提取 JSON。
func TestSearchGarbagePrefix(t *testing.T) {
	out := "ego-browser v1.2.3\nsome log line\n" + searchJSON
	r := NewGooglePatentsRetriever(testConfig(t, out))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	results, err := r.Search(context.Background(), domain.DomainQuery{Text: "x"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results.Documents) != 2 {
		t.Errorf("documents = %d, want 2", len(results.Documents))
	}
}

// TestGetDocument 验证详情页提取。
func TestGetDocument(t *testing.T) {
	docJSON := `{"title": "测试专利", "number": "CN:106599773:B", "abstract": "摘要", "claims": "权利要求", "description": "说明书"}`
	r := NewGooglePatentsRetriever(testConfig(t, docJSON))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	doc, err := r.GetDocument(context.Background(), "CN106599773B")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc == nil {
		t.Fatal("doc is nil")
	}
	// 专利号归一化（citation 格式 → 紧凑格式）。
	if doc.ID != "CN106599773B" {
		t.Errorf("ID = %q, want CN106599773B", doc.ID)
	}
	if doc.Title != "测试专利" {
		t.Errorf("Title = %q", doc.Title)
	}
	if !strings.Contains(doc.Content, "权利要求") || !strings.Contains(doc.Content, "说明书") {
		t.Errorf("Content 应包含权利要求与说明书: %.100s", doc.Content)
	}
	if doc.Metadata["full_text"] != "true" {
		t.Errorf("full_text = %q, want true (claims/description 非空)", doc.Metadata["full_text"])
	}
}

// TestGetDocumentEmpty 验证空 docID 与空结果。
func TestGetDocumentEmpty(t *testing.T) {
	r := NewGooglePatentsRetriever(testConfig(t, "{}"))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	if doc, err := r.GetDocument(context.Background(), ""); err != nil || doc != nil {
		t.Errorf("empty docID: doc=%v err=%v, want nil/nil", doc, err)
	}
	if doc, err := r.GetDocument(context.Background(), "CN123"); err != nil || doc != nil {
		t.Errorf("empty result: doc=%v err=%v, want nil/nil", doc, err)
	}
}

// TestGetDocumentBiblioOnly 验证仅目录信息（无 claims/description，如
// Espacenet biblio）时 full_text 元数据为 false，供上层区分"目录信息"与"全文"，
// 避免把仅有摘要的 biblio 当作成功全文返回。
func TestGetDocumentBiblioOnly(t *testing.T) {
	docJSON := `{"title": "Biblio 专利", "number": "CN107891199A", "abstract": "仅摘要"}`
	r := NewGooglePatentsRetriever(testConfig(t, docJSON))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	doc, err := r.GetDocument(context.Background(), "CN107891199A")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc == nil {
		t.Fatal("doc is nil")
	}
	if doc.Metadata["full_text"] != "false" {
		t.Errorf("full_text = %q, want false (biblio only)", doc.Metadata["full_text"])
	}
	if strings.Contains(doc.Content, "权利要求") || strings.Contains(doc.Content, "说明书") {
		t.Errorf("biblio Content 不应含 claims/description: %.100s", doc.Content)
	}
}

// TestSourceName 验证三个数据源的名称。
func TestSourceName(t *testing.T) {
	cases := []struct {
		name string
		new  func(BrowserRetrieverConfig) *BrowserRetriever
		want string
	}{
		{"google", NewGooglePatentsRetriever, SourceNameGoogle},
		{"cnipa", NewCNIPARetriever, SourceNameCNIPA},
		{"espacenet", NewEspacenetRetriever, SourceNameEspacenet},
	}
	for _, tc := range cases {
		r := tc.new(testConfig(t, searchJSON))
		if r == nil {
			t.Fatalf("%s: retriever is nil", tc.name)
		}
		if got := r.SourceName(); got != tc.want {
			t.Errorf("%s SourceName = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestBuildSearchScript 验证生成的 heredoc 脚本结构（单引号约束、max 替换、
// task space 清理）。
func TestBuildSearchScript(t *testing.T) {
	r := NewGooglePatentsRetriever(testConfig(t, searchJSON))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	script := r.buildSearchScript("深度学习 country:CN", 7)
	for _, want := range []string{
		"useOrCreateTaskSpace",
		"openOrReuseTab",
		"patents.google.com/?q=",
		"slice(0, 7)",
		"completeTaskSpace",
		"cliLog(JSON.stringify(data))",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script 缺少 %q:\n%s", want, script)
		}
	}
}

// TestBuildGetDocScript 验证详情页脚本包含滚动前置步骤。
func TestBuildGetDocScript(t *testing.T) {
	r := NewGooglePatentsRetriever(testConfig(t, "{}"))
	if r == nil {
		t.Fatal("retriever is nil")
	}
	script := r.buildGetDocScript("CN106599773B")
	for _, want := range []string{"scrollToBottomUntil", "section#claims", "patent/CN106599773B/zh"} {
		if !strings.Contains(script, want) {
			t.Errorf("script 缺少 %q:\n%s", want, script)
		}
	}
}

// TestEspacenetDetailURL 验证 Espacenet 公开号 → biblio URL 解析。
func TestEspacenetDetailURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CN107891199A", "https://worldwide.espacenet.com/publicationDetails/biblio?CC=CN&NR=107891199&KC=A"},
		{"EP1234567B1", "https://worldwide.espacenet.com/publicationDetails/biblio?CC=EP&NR=1234567&KC=B1"},
		{"cn107891199a", "https://worldwide.espacenet.com/publicationDetails/biblio?CC=CN&NR=107891199&KC=A"},
		{"https://example.com/x", "https://example.com/x"},
		// 含引号的 URL 会破坏 heredoc JS 模板，应拒绝并回退到搜索页。
		{"https://example.com/a'b", "https://worldwide.espacenet.com/patent/search"},
		{"https://example.com/a`b", "https://worldwide.espacenet.com/patent/search"},
	}
	for _, tc := range cases {
		if got := espacenetDetailURL(tc.in); got != tc.want {
			t.Errorf("espacenetDetailURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNormalizePatentNumber 验证 citation 格式专利号归一化。
func TestNormalizePatentNumber(t *testing.T) {
	cases := []struct{ in, want string }{
		{"CN:106599773:B", "CN106599773B"},
		{"CN106599773B", "CN106599773B"},
		{"", "fallback"},
	}
	for _, tc := range cases {
		if got := normalizePatentNumber(tc.in, "fallback"); got != tc.want {
			t.Errorf("normalizePatentNumber(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestParseMetaLine 验证 metadata 行解析（兼容 bullet 与空格分隔）。
func TestParseMetaLine(t *testing.T) {
	m := parseMetaLine("CN • CN106599773B • 马惠敏 • 清华大学")
	if m.country != "CN" || m.number != "CN106599773B" || m.inventors != "马惠敏" || m.assignee != "清华大学" {
		t.Errorf("parseMetaLine = %+v", m)
	}
	m2 := parseMetaLine("CN · CN108734104B · 方航 · 杭州易舞科技有限公司 · 追加权利人")
	if m2.assignee != "杭州易舞科技有限公司, 追加权利人" {
		t.Errorf("multi assignee = %q", m2.assignee)
	}
	// innerText 中 bullet 渲染为空格：国别列表 + 公开号 + 发明人 + 权利人。
	m3 := parseMetaLine("WO CN CN109964446B 李挥 北京大学深圳研究生院")
	if m3.country != "WO,CN" || m3.number != "CN109964446B" || m3.inventors != "李挥" || m3.assignee != "北京大学深圳研究生院" {
		t.Errorf("space format = %+v", m3)
	}
	// 空格格式无发明人时：权利人可能为空。
	m4 := parseMetaLine("CN CN106599773B 马惠敏")
	if m4.number != "CN106599773B" || m4.inventors != "马惠敏" || m4.assignee != "" {
		t.Errorf("no assignee = %+v", m4)
	}
	m5 := parseMetaLine("")
	if m5.country != "" {
		t.Errorf("empty line = %+v", m5)
	}
}

// TestExtractPubNumber 验证从 Espacenet 副标题行提取公开号。
func TestExtractPubNumber(t *testing.T) {
	cases := []struct{ in, want string }{
		{"EP4576022A1 • 2025-06-25 • DSPACE GMBH [DE]", "EP4576022A1"},
		{"CN107891199A (B) • 2018-04-10", "CN107891199A"},
		{"no number here", ""},
	}
	for _, tc := range cases {
		if got := extractPubNumber(tc.in); got != tc.want {
			t.Errorf("extractPubNumber(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTrimToJSON 验证辅助输出剥离。
func TestTrimToJSON(t *testing.T) {
	if got := string(trimToJSON([]byte("log line\n[1,2]"))); got != "[1,2]" {
		t.Errorf("trimToJSON = %q", got)
	}
	if got := string(trimToJSON([]byte("[1,2]"))); got != "[1,2]" {
		t.Errorf("trimToJSON plain = %q", got)
	}
	if got := trimToJSON([]byte("no json here")); got != nil {
		t.Errorf("trimToJSON garbage = %q, want nil", got)
	}
}
