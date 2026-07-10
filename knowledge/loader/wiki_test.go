package loader

import (
	"os"
	"strings"
	"testing"

	"github.com/xujian519/mady/knowledge"
)

const testWikiPath = "/tmp/wiki_test"

func TestWikiFilter_ShouldImport(t *testing.T) {
	f := DefaultWikiFilter()

	tests := []struct {
		path   string
		should bool
	}{
		{"Wiki/专利侵权/侵权判定/全面覆盖原则.md", true},
		{"Wiki/专利侵权/index.md", false},
		{"Wiki/专利侵权/log.md", false},
		{"Wiki/专利侵权/CLAUDE.md", false},
		{"Wiki/专利侵权/审查指南/_orphan_analysis.md", false},
		{"Wiki/法律法规/法律/专利法-2020-拆分-01-分拆目录.md", false},
		{"cards/test-card.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := f.ShouldImport(tt.path)
			if got != tt.should {
				t.Errorf("ShouldImport(%q) = %v, want %v", tt.path, got, tt.should)
			}
		})
	}
}

func TestWikiFilter_ContentTooShort(t *testing.T) {
	f := DefaultWikiFilter()
	f.MinContentChars = 100

	if f.ContentTooShort(strings.Repeat("x", 50)) != true {
		t.Error("should report content as too short")
	}
	if f.ContentTooShort(strings.Repeat("x", 200)) != false {
		t.Error("should report content as long enough")
	}
}

func TestWikiFilter_IsSearchable(t *testing.T) {
	f := DefaultWikiFilter()

	if f.IsSearchable("Wiki/专利侵权/All-Concepts.md") != false {
		t.Error("All-Concepts should be non-searchable")
	}
	if f.IsSearchable("Wiki/专利侵权/侵权判定-全面覆盖原则.md") != true {
		t.Error("normal page should be searchable")
	}
}

func TestIsSplitFragment(t *testing.T) {
	if IsSplitFragment("test/file.md") != false {
		t.Error("normal file should not be split fragment")
	}
	if IsSplitFragment("test/权属-原理-职务发明权属-拆分-02-核心要点(1)(1)(1)(1)(2).md") != true {
		t.Error("path with repeated (1) markers should be split fragment")
	}
}

func TestExtractMetadata(t *testing.T) {
	content := `# 侵权判定-全面覆盖原则

> **来源：** 《侵权判定指南(2017)理解与适用》第二章，第35条
> **核心法条：** 《专利法》第五十九条第一款；《侵犯专利权司法解释（一）》第七条
> **关联页面：** [[权利保护范围-内部证据与外部证据]]、[[侵权判定-等同侵权的限制]]

## 核心要点

全面覆盖原则要求被控侵权技术方案必须包含权利要求中记载的全部技术特征。

## 第35条 全面覆盖原则

详细内容在此。`

	meta := ExtractMetadata(content, "Wiki/专利侵权/侵权判定/侵权判定-全面覆盖原则.md")

	if meta.Title != "侵权判定-全面覆盖原则" {
		t.Errorf("Title = %q", meta.Title)
	}
	if !strings.Contains(meta.Source, "侵权判定指南") {
		t.Errorf("Source = %q", meta.Source)
	}
	if len(meta.LawRefs) < 2 {
		t.Errorf("LawRefs = %v, want at least 2", meta.LawRefs)
	}
	if len(meta.WikiLinks) < 2 {
		t.Errorf("WikiLinks = %v, want at least 2", meta.WikiLinks)
	}
	if meta.Domain != "patent" {
		t.Errorf("Domain = %q, want patent", meta.Domain)
	}
	if meta.DocType != "judgment" {
		t.Errorf("DocType = %q, want judgment", meta.DocType)
	}
	if !strings.Contains(meta.Summary, "全面覆盖原则") {
		t.Errorf("Summary = %q", meta.Summary)
	}
}

func TestExtractMetadata_ReexamType(t *testing.T) {
	content := `# 创造性-审查标准-医药

> **标签：** 主题=创造性；子主题=审查标准
> **法律依据：** 专利法第二十二条第三款
> **技术领域：** 医药
> **覆盖决定数：** 15 件

## 核心审查标准

医药领域的创造性判断需考虑预料不到的技术效果。`

	meta := ExtractMetadata(content, "Wiki/复审无效/创造性/创造性-审查标准-医药.md")

	if meta.DocType != "reexam" {
		t.Errorf("DocType = %q, want reexam", meta.DocType)
	}
	if len(meta.Tags) == 0 {
		t.Errorf("Tags should not be empty")
	}
}

func TestExtractMetadata_GuidelineType(t *testing.T) {
	content := `# 审查-权利要求-清楚的要求

> **来源：** 《专利审查指南》第二部分第二章第3.2.2节
> **对应法条：** 《专利法》第二十六条第四款
> **最新修订：** 第XX号局令（2020年）

## 核心要点

权利要求应当清楚、简要地限定要求专利保护的范围。`

	meta := ExtractMetadata(content, "Wiki/审查指南/第二部分-实质审查/审查-权利要求-清楚的要求.md")

	if meta.DocType != "guideline" {
		t.Errorf("DocType = %q, want guideline", meta.DocType)
	}
}

func TestCardIndex_LoadAndLookup(t *testing.T) {
	idx, err := LoadCardIndex(testWikiPath)
	if err != nil {
		t.Fatalf("LoadCardIndex: %v", err)
	}
	if idx.TotalCards != 1 {
		t.Errorf("TotalCards = %d, want 1", idx.TotalCards)
	}

	card := idx.LookupCard("/tmp/wiki_test/Wiki/专利侵权/侵权判定/侵权判定-全面覆盖原则.md")
	if card == nil {
		t.Fatal("card not found")
	}
	if card.Concept != "全面覆盖原则" {
		t.Errorf("Concept = %q", card.Concept)
	}
	if card.Quality < 0.9 {
		t.Errorf("Quality = %f, want >= 0.9", card.Quality)
	}
}

func TestCardIndex_QualityCards(t *testing.T) {
	idx, _ := LoadCardIndex(testWikiPath)
	high := idx.QualityCards(0.9)
	if len(high) != 1 {
		t.Errorf("QualityCards(0.9) = %d, want 1", len(high))
	}
	low := idx.QualityCards(1.0)
	if len(low) != 0 {
		t.Errorf("QualityCards(1.0) = %d, want 0", len(low))
	}
}

func TestClassifyWikiPath(t *testing.T) {
	tests := []struct {
		path    string
		domain  string
		docType string
	}{
		{"Wiki/专利侵权/侵权判定/file.md", "patent", "judgment"},
		{"Wiki/专利判决/file.md", "patent", "judgment"},
		{"Wiki/复审无效/file.md", "patent", "reexam"},
		{"Wiki/审查指南/file.md", "patent", "guideline"},
		{"Wiki/专利实务/file.md", "patent", "practice"},
		{"Wiki/法律法规/file.md", "legal", "law"},
		{"Wiki/书籍/file.md", "reference", "book"},
		{"cards/file.md", "patent", "wiki_card"},
		{"unknown/path/file.md", "patent", "wiki_card"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			domain, docType := classifyWikiPath(tt.path)
			if domain != tt.domain {
				t.Errorf("domain = %q, want %q", domain, tt.domain)
			}
			if docType != tt.docType {
				t.Errorf("docType = %q, want %q", docType, tt.docType)
			}
		})
	}
}

func TestWikiLoader_ImportWiki(t *testing.T) {
	// Ensure test data exists.
	if _, err := os.Stat(testWikiPath); os.IsNotExist(err) {
		t.Skip("test wiki data not found, run setup first")
	}

	store := knowledge.NewStore()
	loader := NewWikiLoader(store, testWikiPath)

	stats, err := loader.ImportWiki()
	if err != nil {
		t.Fatalf("ImportWiki: %v", err)
	}

	t.Logf("Stats: total=%d imported=%d filter=%d short=%d errors=%d",
		stats.TotalScanned, stats.Imported, stats.SkippedFilter,
		stats.SkippedShort, stats.SkippedError)

	// Should have imported at least the real content files.
	if stats.Imported < 2 {
		t.Errorf("imported = %d, want at least 2 (one Wiki + one card)", stats.Imported)
	}

	// index.md, log.md, CLAUDE.md should be filtered.
	if stats.SkippedFilter < 3 {
		t.Errorf("skippedFilter = %d, want at least 3 (index, log, CLAUDE)", stats.SkippedFilter)
	}

	// Verify searchable flag: index page should NOT be searchable.
	storeStats := store.Stats()
	t.Logf("Store: %s", storeStats.String())

	// The real content doc should be in the store.
	doc, ok := store.GetDocument("专利侵权/侵权判定/侵权判定-全面覆盖原则")
	if !ok {
		// Try alternate ID (with Wiki prefix stripped).
		doc, ok = store.GetDocument("Wiki/专利侵权/侵权判定/侵权判定-全面覆盖原则")
	}
	if ok {
		if doc.Domain != "patent" {
			t.Errorf("document domain = %q", doc.Domain)
		}
		if !doc.Searchable {
			t.Error("content document should be searchable")
		}
	}
}

func TestSanitizeDocID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Wiki/专利侵权/侵权判定/file.md", "专利侵权/侵权判定/file"},
		{"cards/test-card.md", "test-card"},
		{"Wiki/审查指南/part1/chapter2.md", "审查指南/part1/chapter2"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeDocID(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeDocID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
