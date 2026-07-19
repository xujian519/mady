package writing

import (
	"strings"
	"testing"
)

func TestPatternStore_AddAndGet(t *testing.T) {
	store := NewPatternStore()
	p := &WritingPattern{
		ID:       "test-001",
		Name:     "测试模式",
		Category: "claim_drafting",
		Summary:  "这是一个测试模式",
	}
	if err := store.AddPattern(p); err != nil {
		t.Fatalf("AddPattern() error = %v", err)
	}
	got := store.Get("test-001")
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.Name != "测试模式" {
		t.Errorf("Name = %q, want %q", got.Name, "测试模式")
	}
}

func TestPatternStore_Search(t *testing.T) {
	store := NewPatternStore()
	store.AddPattern(&WritingPattern{
		ID: "p1", Name: "创造性三步法答辩",
		Category: "oa_inventiveness", Summary: "三步法答辩框架",
	})
	store.AddPattern(&WritingPattern{
		ID: "p2", Name: "新颖性单独对比",
		Category: "oa_novelty", Summary: "单独对比原则",
	})
	store.AddPattern(&WritingPattern{
		ID: "p3", Name: "权利要求撰写基础",
		Category: "claim_drafting", Summary: "独立权利要求结构",
	})

	results := store.SearchPatterns("创造性", "", 10)
	if len(results) == 0 {
		t.Fatal("SearchPatterns('创造性') returned 0 results")
	}
	if results[0].ID != "p1" {
		t.Errorf("top result ID = %q, want 'p1'", results[0].ID)
	}

	// Filter by category.
	results = store.SearchPatterns("", "oa_novelty", 10)
	if len(results) != 1 || results[0].ID != "p2" {
		t.Errorf("expected 1 result for oa_novelty, got %d", len(results))
	}
}

func TestPatternStore_Match(t *testing.T) {
	store := NewPatternStore()
	store.AddPattern(&WritingPattern{
		ID: "p1", Name: "创造性三步法答辩",
		Category: "oa_inventiveness", Summary: "三步法答辩",
	})
	store.AddPattern(&WritingPattern{
		ID: "p2", Name: "权利要求撰写基础",
		Category: "claim_drafting", Summary: "权利要求",
	})

	matched := store.MatchPatterns("oa_inventiveness", []string{"创造性"})
	if len(matched) == 0 {
		t.Fatal("MatchPatterns returned 0 results")
	}
	if matched[0].ID != "p1" {
		t.Errorf("expected p1, got %s", matched[0].ID)
	}
}

func TestSeedPatterns_LoadAll(t *testing.T) {
	store := NewPatternStore()
	count, err := store.LoadSeedDir("seed-patterns")
	if err != nil {
		t.Fatalf("LoadSeedDir() error = %v", err)
	}
	if count != 10 {
		t.Errorf("loaded %d seed files, want 10", count)
	}
	all := store.All()
	if len(all) != 10 {
		t.Errorf("total patterns = %d, want 10", len(all))
	}
	// Verify all have required fields.
	for _, p := range all {
		if p.ID == "" {
			t.Error("pattern has empty ID")
		}
		if p.Name == "" {
			t.Errorf("pattern %s has empty Name", p.ID)
		}
		if p.Summary == "" {
			t.Errorf("pattern %s has empty Summary", p.ID)
		}
		if p.Quality <= 0 {
			t.Errorf("pattern %s has Quality=%f", p.ID, p.Quality)
		}
	}
	// Verify specific patterns exist.
	expectedIDs := []string{
		"wp-claim-utility-model",
		"wp-claim-dependent-layering",
		"wp-spec-background",
		"wp-spec-invention-content",
		"wp-oa-inventiveness-3step",
		"wp-oa-novelty-separate",
		"wp-oa-clarity-support",
		"wp-disclosure-pfe",
		"wp-ipc-strategy",
		"wp-embodiment-writing",
	}
	for _, id := range expectedIDs {
		if store.Get(id) == nil {
			t.Errorf("expected pattern %s not found", id)
		}
	}
}

func TestSkillCompiler_MatchAndCompile(t *testing.T) {
	store := NewPatternStore()
	store.AddPattern(&WritingPattern{
		ID: "test-1", Name: "创造性三步法答辩",
		Category: "oa_inventiveness",
		Summary:  "三步法答辩框架",
		Steps:    []Step{{Order: 1, Name: "第一步", Instruction: "确认区别特征"}},
	})
	compiler := NewSkillCompiler(store)

	// Test XML output (MatchAndCompile compiles to XML).
	result := compiler.MatchAndCompile("oa_inventiveness", []string{"创造性"})
	if result == "" {
		t.Fatal("MatchAndCompile returned empty")
	}
	if !strings.Contains(result, "<skill") {
		t.Error("expected <skill> tag in output")
	}
	if !strings.Contains(result, "第一步") {
		t.Error("expected step content in output")
	}

	// Test Markdown output.
	patterns := store.All()
	md := compiler.CompileMarkdown(patterns)
	if !strings.Contains(md, "推荐的写作模式") {
		t.Error("expected '推荐的写作模式' in markdown output")
	}
}

func TestQualityEvaluator_Basic(t *testing.T) {
	store := NewPatternStore()
	store.AddPattern(&WritingPattern{
		ID: "test", Name: "测试", Category: "test", Summary: "test",
	})
	fs := NewFeedbackStore()
	eval := NewQualityEvaluator(store, fs)

	report := eval.Evaluate("out-001", "根据专利法第22条第3款的规定，权利要求1与对比文件1的区别在于……因此，具备创造性。", []string{"test"})
	if report.AutoScore <= 0 {
		t.Error("AutoScore should be > 0")
	}
	if report.Dimensions.Citation < 60 {
		t.Errorf("Citation score = %.0f, expected >= 60", report.Dimensions.Citation)
	}

	// Test feedback collection.
	if err := eval.CollectFeedback("out-001", RatingGood, "分析合理"); err != nil {
		t.Fatalf("CollectFeedback() error = %v", err)
	}
	stats := eval.Stats()
	if stats.ByRating[RatingGood] != 1 {
		t.Errorf("expected 1 good rating, got %d", stats.ByRating[RatingGood])
	}
}

func TestDimensionEval(t *testing.T) {
	// Structure test.
	s := "# 标题\n## 章节1\n内容1\n\n## 章节2\n内容2\n\n段落3"
	score := evalStructure(s)
	if score < 70 {
		t.Errorf("Structure score = %.0f, expected >= 70", score)
	}

	// Citation test.
	s2 := "根据专利法第22条第3款，参见第566088号决定，对比文件CN123456A"
	score = evalCitation(s2)
	if score < 70 {
		t.Errorf("Citation score = %.0f, expected >= 70", score)
	}

	// Slop penalty test.
	s3 := "进一步地，值得一提的是，显而易见地"
	score = evalArgument(s3)
	if score > 30 {
		t.Errorf("Argument score = %.0f with slop phrases, expected <= 30", score)
	}

	// Terminology - bad phrases.
	s4 := "这是最好的方案，绝对没有问题"
	score = evalTerminology(s4)
	if score > 40 {
		t.Errorf("Terminology score = %.0f with banned phrases, expected <= 40", score)
	}
}
