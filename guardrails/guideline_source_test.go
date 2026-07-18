package guardrails

import (
	"testing"

	"github.com/xujian519/mady/pkg/lawcite"
)

func TestGuidelineSource_Topics_ExactMatch(t *testing.T) {
	gs := NewGuidelineSource()

	// Part 2, Chapter 4, Section 1 — "三步法" should be in topics
	topics, ok := gs.Topics(lawcite.StatuteExamGuideline, 20401)
	if !ok {
		t.Fatal("expected topics for guideline section 2.4.1")
	}

	found := false
	for _, topic := range topics {
		if topic == "三步法" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '三步法' in topics for 2.4.1, got %v", topics)
	}
}

func TestGuidelineSource_Topics_ProgressiveMatch(t *testing.T) {
	gs := NewGuidelineSource()

	// 2.4.1.2 → should match parent 20401 via progressive divisor
	topics, ok := gs.Topics(lawcite.StatuteExamGuideline, 2040102)
	if !ok {
		t.Fatal("expected topics via progressive matching for subsection")
	}

	found := false
	for _, topic := range topics {
		if topic == "三步法" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '三步法' in progressive-matched topics, got %v", topics)
	}
}

func TestGuidelineSource_Topics_UnknownSection(t *testing.T) {
	gs := NewGuidelineSource()

	_, ok := gs.Topics(lawcite.StatuteExamGuideline, 90909)
	if ok {
		t.Error("expected no topics for unknown section")
	}
}

func TestGuidelineSource_Topics_WrongStatute(t *testing.T) {
	gs := NewGuidelineSource()

	// Should return false for non-guideline statutes
	_, ok := gs.Topics(lawcite.StatutePatentLaw, 22)
	if ok {
		t.Error("expected false for non-guideline statute")
	}
}

func TestGuidelineSource_Topics_KeySections(t *testing.T) {
	gs := NewGuidelineSource()

	tests := []struct {
		code         int
		expectedWord string
		description  string
	}{
		{20301, "新颖性", "Part 2 Ch3 S1 — novelty concept"},
		{20303, "新颖性", "Part 2 Ch3 S3 — identical inventions"},
		{20403, "技术启示", "Part 2 Ch4 S3 — teaching motivation"},
		{20404, "预料不到的技术效果", "Part 2 Ch4 S4 — unexpected effect"},
		{20802, "专利法第33条", "Part 2 Ch8 S2 — amendment scope"},
		{40201, "无效宣告", "Part 4 Ch2 S1 — invalidation request"},
		{20201, "充分公开", "Part 2 Ch2 S1 — sufficient disclosure"},
		{20601, "单一性", "Part 2 Ch6 S1 — unity"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			topics, ok := gs.Topics(lawcite.StatuteExamGuideline, tt.code)
			if !ok {
				t.Fatalf("expected topics for code %d", tt.code)
			}
			found := false
			for _, topic := range topics {
				if topic == tt.expectedWord {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("code %d: expected topic %q in %v", tt.code, tt.expectedWord, topics)
			}
		})
	}
}

func TestGuidelineSource_MaxArticle(t *testing.T) {
	gs := NewGuidelineSource()
	if gs.MaxArticle(lawcite.StatuteExamGuideline) != 0 {
		t.Error("GuidelineSource.MaxArticle() should return 0 (no article-number upper bound)")
	}
}

func TestGuidelineSource_CompositeWithPatentLaw(t *testing.T) {
	patentSrc := DefaultCitationSource()
	guidelineSrc := NewGuidelineSource()
	composite := CompositeCitationSource(patentSrc, guidelineSrc)

	// Patent law topics should still work.
	patentTopics, ok := composite.Topics(lawcite.StatutePatentLaw, 22)
	if !ok {
		t.Fatal("composite source should return patent law topics for article 22")
	}
	if len(patentTopics) == 0 {
		t.Fatal("expected non-empty topics for patent law article 22")
	}

	// Guideline topics should also work.
	guidelineTopics, ok := composite.Topics(lawcite.StatuteExamGuideline, 20401)
	if !ok {
		t.Fatal("composite source should return guideline topics for section 2.4.1")
	}
	if len(guidelineTopics) == 0 {
		t.Fatal("expected non-empty guideline topics for section 2.4.1")
	}
}

func TestEncodeGuidelineSection(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"2.4.1", 20401},
		{"2.8.2", 20802},
		{"2.3.1", 20301},
		{"4.2.1", 40201},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := EncodeGuidelineSection(tt.input)
		if got != tt.want {
			t.Errorf("EncodeGuidelineSection(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
