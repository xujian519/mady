package intentrules

import (
	"strings"
	"testing"
)

func TestCountKeywordMatches_Basic(t *testing.T) {
	input := strings.ToLower("分析这个专利的新颖性和创造性")
	keywords := []string{"新颖性", "创造性", "专利", "侵权"}

	count, matched := CountKeywordMatches(input, keywords)
	if count != 3 {
		t.Errorf("expected 3 matches, got %d: %v", count, matched)
	}
}

func TestCountKeywordMatches_SubstringDedup(t *testing.T) {
	// "独立权利要求" should suppress "权利要求" and "要求" as substrings
	input := strings.ToLower("独立权利要求包含技术特征")
	keywords := []string{"独立权利要求", "权利要求", "要求", "技术特征"}

	count, matched := CountKeywordMatches(input, keywords)
	if count != 2 {
		t.Errorf("expected 2 matches (独立权利要求, 技术特征), got %d: %v", count, matched)
	}
	for _, m := range matched {
		if m == "权利要求" || m == "要求" {
			t.Errorf("substring %q should have been deduplicated", m)
		}
	}
}

func TestCountKeywordMatches_CaseInsensitive(t *testing.T) {
	// Input is lowercase, keywords have mixed case
	input := strings.ToLower("Search for IPC and PCT patents")
	keywords := []string{"IPC", "PCT", "patent"}

	count, _ := CountKeywordMatches(input, keywords)
	if count != 3 {
		t.Errorf("expected 3 case-insensitive matches, got %d", count)
	}
}

func TestCountKeywordMatches_NoMatch(t *testing.T) {
	input := strings.ToLower("hello world")
	keywords := []string{"专利", "法律"}

	count, _ := CountKeywordMatches(input, keywords)
	if count != 0 {
		t.Errorf("expected 0 matches, got %d", count)
	}
}

func TestCountKeywordMatches_EmptyKeywords(t *testing.T) {
	input := strings.ToLower("any input")
	count, _ := CountKeywordMatches(input, nil)
	if count != 0 {
		t.Errorf("expected 0 matches for nil keywords, got %d", count)
	}
	// nil keywords produce nil matched slice (Go zero value), which is fine.
}

func TestMatchAnyKeyword_Found(t *testing.T) {
	input := strings.ToLower("专利检索分析")
	keywords := []string{"专利", "商标"}

	if !MatchAnyKeyword(input, keywords) {
		t.Error("expected keyword match")
	}
}

func TestMatchAnyKeyword_NotFound(t *testing.T) {
	input := strings.ToLower("一般对话内容")
	keywords := []string{"专利", "法律"}

	if MatchAnyKeyword(input, keywords) {
		t.Error("expected no keyword match")
	}
}

func TestMatchAnyKeyword_CaseInsensitive(t *testing.T) {
	input := strings.ToLower("check IPC classification")
	keywords := []string{"IPC"}

	if !MatchAnyKeyword(input, keywords) {
		t.Error("expected case-insensitive match for IPC")
	}
}

func TestPatentContextSignals_NotEmpty(t *testing.T) {
	if len(PatentContextSignals) == 0 {
		t.Error("PatentContextSignals should not be empty")
	}
}

func TestKeywordLists_NotEmpty(t *testing.T) {
	if len(PatentKeywords) == 0 {
		t.Error("PatentKeywords should not be empty")
	}
	if len(LegalKeywords) == 0 {
		t.Error("LegalKeywords should not be empty")
	}
	if len(AssistantKeywords) == 0 {
		t.Error("AssistantKeywords should not be empty")
	}
}

func TestKeywordLists_NoOverlap(t *testing.T) {
	// Patent and Legal keywords should not overlap significantly
	patentSet := make(map[string]bool)
	for _, kw := range PatentKeywords {
		patentSet[strings.ToLower(kw)] = true
	}
	overlap := 0
	for _, kw := range LegalKeywords {
		if patentSet[strings.ToLower(kw)] {
			overlap++
		}
	}
	// Some overlap is expected (e.g., "侵权" appears in both),
	// but it should be minimal.
	if overlap > 3 {
		t.Errorf("too many overlapping keywords between patent and legal: %d", overlap)
	}
}

func TestCountKeywordMatches_ChineseSubstring(t *testing.T) {
	// Test that "审查意见通知书" suppresses "审查意见"
	input := strings.ToLower("答复审查意见通知书的要求")
	keywords := []string{"审查意见通知书", "审查意见", "答复"}

	count, matched := CountKeywordMatches(input, keywords)
	if count != 2 {
		t.Errorf("expected 2 matches (审查意见通知书, 答复), got %d: %v", count, matched)
	}
	// "审查意见" should not be in matched because it's a substring of "审查意见通知书"
	for _, m := range matched {
		if m == "审查意见" {
			t.Error("审查意见 should have been deduplicated as substring of 审查意见通知书")
		}
	}
}
