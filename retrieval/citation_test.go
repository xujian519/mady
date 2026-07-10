package retrieval

import (
	"strings"
	"testing"
)

func TestExtractCitation(t *testing.T) {
	item := CitableItem{
		Title:    "专利法",
		Heading:  "第22条",
		Source:   "law",
		DocType:  "法条",
		Metadata: map[string]string{"caseNumber": "2020"},
	}
	meta := ExtractCitation(item)

	if meta.SourceTitle != "专利法 > 第22条" {
		t.Errorf("SourceTitle = %q", meta.SourceTitle)
	}
	if meta.SourcePath != "专利法 (2020)" {
		t.Errorf("SourcePath = %q", meta.SourcePath)
	}
}

func TestExtractCitation_NoHeading(t *testing.T) {
	item := CitableItem{Title: "判决书A", Source: "judgment", DocType: "判决"}
	meta := ExtractCitation(item)
	if meta.SourceTitle != "判决书A" {
		t.Errorf("SourceTitle = %q, want 判决书A", meta.SourceTitle)
	}
}

func TestCitationPrefix(t *testing.T) {
	items := []CitableItem{
		{Title: "专利法", Source: "law"},
		{Title: "审查指南", Source: "guideline"},
	}
	prefix := CitationPrefix(items)
	if !strings.Contains(prefix, "基于以下知识来源") {
		t.Error("should contain prefix text")
	}
	if !strings.Contains(prefix, "专利法") {
		t.Error("should contain first title")
	}

	if CitationPrefix(nil) != "" {
		t.Error("empty items should return empty")
	}
}

func TestFormatCitations(t *testing.T) {
	items := []CitableItem{
		{Title: "专利法", Source: "law", DocType: "法条", AuthorityWeight: 0.9},
		{Title: "案例X", Source: "case", DocType: "案例", AuthorityWeight: 0.5},
	}
	out := FormatCitations(items)
	if !strings.Contains(out, "参考来源") {
		t.Error("should contain header")
	}
	if !strings.Contains(out, "专利法") {
		t.Error("should contain first item")
	}
	if !strings.Contains(out, "权威度") {
		t.Error("should contain authority weight")
	}
}

func TestFormatCitationChain(t *testing.T) {
	items := []CitableItem{
		{Title: "专利法第22条", Source: "law"},
		{Title: "专利法第26条", Source: "law"},
		{Title: "审查指南", Source: "guideline"},
		{Title: "最高法判例", Source: "case"},
	}
	out := FormatCitationChain(items)
	if !strings.Contains(out, "法律依据") {
		t.Error("should group law under 法律依据")
	}
	if !strings.Contains(out, "审查指南") {
		t.Error("should contain guideline label")
	}
	if !strings.Contains(out, "→") {
		t.Error("chain should use arrow separator for same-type items")
	}
}

func TestFormatCitationChain_UnknownType(t *testing.T) {
	items := []CitableItem{
		{Title: "自定义文档", Source: "custom"},
	}
	out := FormatCitationChain(items)
	if !strings.Contains(out, "custom") {
		t.Error("should include unknown type label")
	}
}

func TestDetectConflicts(t *testing.T) {
	items := []CitableItem{
		{Title: "新颖性 判断标准", Source: "law", DocType: "法条", AuthorityWeight: 0.9},
		{Title: "新颖性 判断方法", Source: "note", DocType: "笔记", AuthorityWeight: 0.2},
	}
	warnings := DetectConflicts(items)
	if len(warnings) == 0 {
		t.Error("should detect authority conflict for keyword 新颖性")
	}
}

func TestDetectConflicts_NoConflict(t *testing.T) {
	items := []CitableItem{
		{Title: "专利法", Source: "law", DocType: "法条", AuthorityWeight: 0.5},
		{Title: "商标法", Source: "law", DocType: "法条", AuthorityWeight: 0.5},
	}
	warnings := DetectConflicts(items)
	if len(warnings) != 0 {
		t.Errorf("expected no conflicts, got %v", warnings)
	}
}

func TestChunkToCitable(t *testing.T) {
	chunk := Chunk{
		ID:      "doc1#chunk-0",
		DocID:   "CN123456",
		Content: "权利要求1...",
		Metadata: map[string]string{
			"source":    "patent",
			"type":      "专利文献",
			"domain":    "patent",
			"section":   "权利要求",
			"authority": "0.8",
		},
	}
	item := ChunkToCitable(chunk, 0.6)

	if item.Title != "CN123456" {
		t.Errorf("Title = %q", item.Title)
	}
	if item.Heading != "权利要求" {
		t.Errorf("Heading = %q", item.Heading)
	}
	if item.AuthorityWeight != 0.8 {
		t.Errorf("AuthorityWeight = %f, want 0.8", item.AuthorityWeight)
	}
}

func TestChunkToCitable_AuthorityFallback(t *testing.T) {
	chunk := Chunk{
		DocID:    "doc1",
		Metadata: map[string]string{"source": "case"},
	}
	item := ChunkToCitable(chunk, 0.7)
	if item.AuthorityWeight != 0.7 {
		t.Errorf("fallback to score: %f, want 0.7", item.AuthorityWeight)
	}
}

func TestScoredChunksToCitable(t *testing.T) {
	results := []ScoredChunk{
		{Chunk: Chunk{DocID: "A", Metadata: map[string]string{"source": "law"}}, Score: 0.9},
		{Chunk: Chunk{DocID: "B", Metadata: map[string]string{"source": "case"}}, Score: 0.5},
	}
	items := ScoredChunksToCitable(results)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "A" {
		t.Errorf("first item Title = %q", items[0].Title)
	}
}

func TestSplitKeywords(t *testing.T) {
	kw := splitKeywords("新颖性，创造性、实用性")
	if len(kw) == 0 {
		t.Fatal("expected keywords")
	}
	found := false
	for _, k := range kw {
		if k == "新颖性" {
			found = true
		}
	}
	if !found {
		t.Errorf("should contain 新颖性, got %v", kw)
	}
}
