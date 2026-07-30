package checker

import (
	"context"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Catalog tests
// ---------------------------------------------------------------------------

func TestNewCatalog_Empty(t *testing.T) {
	c := NewCatalog()
	if c == nil {
		t.Fatal("NewCatalog returned nil")
	}
	if len(c.List()) != 0 {
		t.Errorf("expected empty catalog, got %d entries", len(c.List()))
	}
}

func TestCatalog_RegisterAndGet(t *testing.T) {
	c := NewCatalog()
	entry := CheckerEntry{
		RoleID:      "test_checker",
		Tier:        TierReviewer,
		Name:        "测试检查器",
		Description: "用于测试",
	}
	c.Register(entry)

	got := c.Get("test_checker")
	if got == nil {
		t.Fatal("expected to find registered checker")
	}
	if got.Name != "测试检查器" {
		t.Errorf("expected name '测试检查器', got %q", got.Name)
	}
}

func TestCatalog_Get_NotFound(t *testing.T) {
	c := NewCatalog()
	got := c.Get("nonexistent")
	if got != nil {
		t.Fatal("expected nil for nonexistent checker")
	}
}

func TestCatalog_Register_ReplaceExisting(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "c1", Name: "v1"})
	c.Register(CheckerEntry{RoleID: "c1", Name: "v2"})

	got := c.Get("c1")
	if got.Name != "v2" {
		t.Errorf("expected 'v2', got %q", got.Name)
	}
	if len(c.List()) != 1 {
		t.Errorf("expected 1 entry after replace, got %d", len(c.List()))
	}
}

func TestCatalog_List_ReturnsCopy(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "c1", Name: "v1"})

	list1 := c.List()
	list1[0].Name = "modified"

	// Original should be unchanged
	got := c.Get("c1")
	if got.Name != "v1" {
		t.Errorf("expected 'v1' (unchanged), got %q", got.Name)
	}
}

func TestCatalog_Suggest_ByWildcard(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{
		RoleID:         "novelty_checker",
		Name:           "新颖性检查器",
		RequiredInputs: []string{"*novelty*", "*技术分析*"},
	})

	matches := c.Suggest("outputs/novelty-analysis.md")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for novelty path, got %d", len(matches))
	}
	if matches[0].RoleID != "novelty_checker" {
		t.Errorf("expected 'novelty_checker', got %q", matches[0].RoleID)
	}

	// Non-matching path
	matches = c.Suggest("outputs/quality-report.md")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for non-matching path, got %d", len(matches))
	}
}

func TestCatalog_Suggest_MultipleMatches(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "a", RequiredInputs: []string{"*.md"}})
	c.Register(CheckerEntry{RoleID: "b", RequiredInputs: []string{"*novelty*"}})

	matches := c.Suggest("report.md")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for .md, got %d", len(matches))
	}
	if matches[0].RoleID != "a" {
		t.Errorf("expected 'a', got %q", matches[0].RoleID)
	}
}

func TestDefaultCatalog_NotEmpty(t *testing.T) {
	c := DefaultCatalog()
	entries := c.List()
	if len(entries) == 0 {
		t.Fatal("DefaultCatalog should not be empty")
	}
	// Check all built-in checkers are present
	expectedIDs := []string{
		"reviewer", "quality_checker", "novelty_checker",
		"oa_checker", "inventiveness_checker",
		"infringement_checker", "invalidity_checker",
	}
	for _, id := range expectedIDs {
		if c.Get(id) == nil {
			t.Errorf("expected builtin checker %q not found", id)
		}
	}
}

// ---------------------------------------------------------------------------
// matchArtifact tests
// ---------------------------------------------------------------------------

func TestMatchArtifact_Exact(t *testing.T) {
	if !matchArtifact("output.md", "output.md") {
		t.Error("expected exact match")
	}
}

func TestMatchArtifact_WildcardSuffix(t *testing.T) {
	if !matchArtifact("output.md", "*.md") {
		t.Error("expected suffix wildcard match")
	}
	if !matchArtifact("data/novelty-report.md", "*.md") {
		t.Error("expected suffix wildcard match")
	}
}

func TestMatchArtifact_WildcardPrefix(t *testing.T) {
	if !matchArtifact("novelty-analysis.md", "*novelty*") {
		t.Error("expected prefix wildcard match")
	}
	if matchArtifact("quality-report.md", "*novelty*") {
		t.Error("expected no match for non-matching prefix")
	}
}

func TestMatchArtifact_EmptyPattern(t *testing.T) {
	if matchArtifact("test.md", "") {
		t.Error("expected no match for empty pattern")
	}
}

func TestMatchArtifact_StarOnly(t *testing.T) {
	if !matchArtifact("anything.go", "*") {
		t.Error("expected match for * pattern")
	}
}

// ---------------------------------------------------------------------------
// Dispatch tests
// ---------------------------------------------------------------------------

func TestNewDispatch(t *testing.T) {
	c := NewCatalog()
	d := NewDispatch(c)
	if d == nil {
		t.Fatal("NewDispatch returned nil")
	}
	if d.GetHandler("nonexistent") != nil {
		t.Error("expected nil handler for unregistered")
	}
}

func TestDispatch_RegisterAndGetHandler(t *testing.T) {
	c := NewCatalog()
	d := NewDispatch(c)

	handler := CheckerHandler(func(_ context.Context, _, _ string) (CheckerVerdict, error) {
		return CheckerVerdict{Status: StatusPass, Score: 1.0}, nil
	})
	d.RegisterHandler("test", handler)

	got := d.GetHandler("test")
	if got == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestDispatch_SuggestCheckers(t *testing.T) {
	c := DefaultCatalog()
	d := NewDispatch(c)

	matches := d.SuggestCheckers("outputs/novelty-analysis.md")
	if len(matches) == 0 {
		t.Fatal("expected at least one checker for novelty path")
	}
	foundNovelty := false
	foundReviewer := false
	for _, m := range matches {
		if m.RoleID == "novelty_checker" {
			foundNovelty = true
		}
		if m.RoleID == "reviewer" {
			foundReviewer = true
		}
	}
	if !foundNovelty {
		t.Error("expected novelty_checker in suggestions")
	}
	if !foundReviewer {
		t.Error("expected reviewer in suggestions (matches *.md)")
	}
}

func TestDispatch_RunChecker_NotFound(t *testing.T) {
	c := NewCatalog()
	d := NewDispatch(c)

	_, err := d.RunChecker(context.Background(), "nonexistent", "test.md", "content")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got %v", err)
	}
}

func TestDispatch_RunChecker_NoHandler(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "test_checker", Name: "测试检查器"})
	d := NewDispatch(c)

	v, err := d.RunChecker(context.Background(), "test_checker", "test.md", "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Status != StatusNeedsRevision {
		t.Errorf("expected StatusNeedsRevision for placeholder, got %v", v.Status)
	}
	if v.RoleID != "test_checker" {
		t.Errorf("expected RoleID 'test_checker', got %q", v.RoleID)
	}
}

func TestDispatch_RunChecker_WithHandler(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "test_checker", Name: "测试检查器"})
	d := NewDispatch(c)

	d.RegisterHandler("test_checker", func(_ context.Context, _, _ string) (CheckerVerdict, error) {
		return CheckerVerdict{
			Status:      StatusPass,
			Score:       0.95,
			Summary:     "一切正常",
			Suggestions: []string{"保持现有质量"},
		}, nil
	})

	v, err := d.RunChecker(context.Background(), "test_checker", "test.md", "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Status != StatusPass {
		t.Errorf("expected StatusPass, got %v", v.Status)
	}
	if v.Score != 0.95 {
		t.Errorf("expected score 0.95, got %v", v.Score)
	}
	if len(v.Suggestions) != 1 || v.Suggestions[0] != "保持现有质量" {
		t.Errorf("unexpected suggestions: %v", v.Suggestions)
	}
}

func TestDispatch_RunAllMatching_NoMatch(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "c1", RequiredInputs: []string{"*specific*"}})
	d := NewDispatch(c)

	_, _, err := d.RunAllMatching(context.Background(), "unrelated.md", "content")
	if err == nil {
		t.Fatal("expected error for no matching checkers")
	}
}

func TestDispatch_RunAllMatching_Aggregation(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "a", RequiredInputs: []string{"*.md"}})
	c.Register(CheckerEntry{RoleID: "b", RequiredInputs: []string{"*.md"}})
	d := NewDispatch(c)

	d.RegisterHandler("a", func(_ context.Context, _, _ string) (CheckerVerdict, error) {
		return CheckerVerdict{Status: StatusPass, Score: 0.9, Summary: "A ok"}, nil
	})
	d.RegisterHandler("b", func(_ context.Context, _, _ string) (CheckerVerdict, error) {
		return CheckerVerdict{Status: StatusNeedsRevision, Score: 0.6, Summary: "B issues"}, nil
	})

	individual, agg, err := d.RunAllMatching(context.Background(), "test.md", "content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(individual) != 2 {
		t.Errorf("expected 2 individual verdicts, got %d", len(individual))
	}
	if agg.Status != StatusNeedsRevision {
		t.Errorf("expected composite needs_revision, got %v", agg.Status)
	}
	if agg.Score < 0.74 || agg.Score > 0.76 {
		t.Errorf("expected avg score ~0.75, got %v", agg.Score)
	}
}

func TestFormatReviewPrompt_Basic(t *testing.T) {
	entry := CheckerEntry{
		RoleID:      "reviewer",
		Name:        "文件审查专家",
		Description: "检查文档形式规范",
	}
	prompt := FormatReviewPrompt(entry, "test.md", "示例内容")
	if !strings.Contains(prompt, "文件审查专家") {
		t.Error("prompt should contain checker name")
	}
	if !strings.Contains(prompt, "test.md") {
		t.Error("prompt should contain artifact path")
	}
	if !strings.Contains(prompt, "示例内容") {
		t.Error("prompt should contain content")
	}
	if !strings.Contains(prompt, "JSON") {
		t.Error("prompt should contain JSON output format instructions")
	}
}

func TestFormatReviewPrompt_ContentTruncation(t *testing.T) {
	entry := CheckerEntry{RoleID: "trunc", Name: "截断测试"}
	longContent := strings.Repeat("x", 10000)
	prompt := FormatReviewPrompt(entry, "big.txt", longContent)
	if !strings.Contains(prompt, "已截断") {
		t.Error("prompt should indicate content was truncated")
	}
	if len(prompt) > 10000 { // ensure prompt is reasonably sized
		t.Logf("prompt length is %d", len(prompt))
	}
}

// ---------------------------------------------------------------------------
// Verdict tests
// ---------------------------------------------------------------------------

func TestVerdictLabel(t *testing.T) {
	tests := []struct {
		status VerdictStatus
		want   string
	}{
		{StatusPass, "✅ 通过"},
		{StatusNeedsRevision, "⚠️ 需修订"},
		{StatusBlocked, "❌ 阻塞（需人工介入）"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := verdictLabel(tt.status)
		if got != tt.want {
			t.Errorf("verdictLabel(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestNewVerdictAggregator(t *testing.T) {
	va := NewVerdictAggregator(nil)
	if va == nil {
		t.Fatal("NewVerdictAggregator returned nil")
	}
}

func TestVerdictAggregator_Aggregate_Empty(t *testing.T) {
	v := NewVerdictAggregator([]CheckerVerdict{}).Aggregate()
	if v.Status != StatusPass {
		t.Errorf("expected Pass for empty verdicts, got %v", v.Status)
	}
	if v.Score != 1.0 {
		t.Errorf("expected score 1.0 for empty, got %v", v.Score)
	}
}

func TestVerdictAggregator_Aggregate_AllPass(t *testing.T) {
	verdicts := []CheckerVerdict{
		{Status: StatusPass, Score: 0.9, Summary: "OK"},
		{Status: StatusPass, Score: 0.95},
	}
	v := NewVerdictAggregator(verdicts).Aggregate()
	if v.Status != StatusPass {
		t.Errorf("expected Pass, got %v", v.Status)
	}
	// Average: (0.9 + 0.95) / 2 = 0.925
	if v.Score < 0.92 || v.Score > 0.93 {
		t.Errorf("expected score ~0.925, got %v", v.Score)
	}
}

func TestVerdictAggregator_Aggregate_NeedsRevisionDominates(t *testing.T) {
	verdicts := []CheckerVerdict{
		{Status: StatusPass, Score: 0.9, Issues: []Issue{{Severity: SeverityInfo, Description: "小问题"}}},
		{Status: StatusNeedsRevision, Score: 0.5},
	}
	v := NewVerdictAggregator(verdicts).Aggregate()
	if v.Status != StatusNeedsRevision {
		t.Errorf("expected NeedsRevision, got %v", v.Status)
	}
}

func TestVerdictAggregator_Aggregate_BlockedDominates(t *testing.T) {
	verdicts := []CheckerVerdict{
		{Status: StatusPass, Score: 0.9},
		{Status: StatusNeedsRevision, Score: 0.5},
		{Status: StatusBlocked, Score: 0.1},
	}
	v := NewVerdictAggregator(verdicts).Aggregate()
	if v.Status != StatusBlocked {
		t.Errorf("expected Blocked, got %v", v.Status)
	}
}

func TestVerdictAggregator_Aggregate_DeduplicateIssues(t *testing.T) {
	verdicts := []CheckerVerdict{
		{
			Status: StatusNeedsRevision, Score: 0.5,
			Issues: []Issue{
				{Severity: SeverityError, Description: "相同问题"},
				{Severity: SeverityWarning, Description: "不同问题"},
			},
		},
		{
			Status: StatusNeedsRevision, Score: 0.5,
			Issues: []Issue{
				{Severity: SeverityError, Description: "相同问题"},
				{Severity: SeverityWarning, Description: "另一个不同问题"},
			},
		},
	}
	v := NewVerdictAggregator(verdicts).Aggregate()
	if len(v.Issues) != 3 {
		t.Errorf("expected 3 deduplicated issues, got %d", len(v.Issues))
	}
}

// ---------------------------------------------------------------------------
// FormatVerdict tests
// ---------------------------------------------------------------------------

func TestFormatVerdict_Basic(t *testing.T) {
	v := CheckerVerdict{
		RoleID:      "test_checker",
		Status:      StatusPass,
		Score:       0.95,
		Summary:     "一切正常",
		Issues:      nil,
		Suggestions: nil,
	}
	out := FormatVerdict(v)
	if !strings.Contains(out, "test_checker") {
		t.Error("output should contain RoleID")
	}
	if !strings.Contains(out, "✅") {
		t.Error("output should contain pass emoji")
	}
	if !strings.Contains(out, "0.95") {
		t.Error("output should contain score")
	}
}

func TestFormatVerdict_WithIssues(t *testing.T) {
	v := CheckerVerdict{
		RoleID: "tester",
		Status: StatusNeedsRevision,
		Issues: []Issue{
			{Severity: SeverityError, Location: "section 3", Description: "发现错误", Suggestion: "修正"},
			{Severity: SeverityWarning, Description: "潜在问题"},
			{Severity: SeverityInfo, Description: "仅供参考"},
		},
		Suggestions: []string{"改进建议1", "改进建议2"},
	}
	out := FormatVerdict(v)
	for _, want := range []string{"发现错误", "潜在问题", "仅供参考", "改进建议1", "改进建议2", "section 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q", want)
		}
	}
}

func TestFormatVerdict_Empty(t *testing.T) {
	v := CheckerVerdict{RoleID: "empty", Status: StatusPass}
	out := FormatVerdict(v)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
}

// ---------------------------------------------------------------------------
// ParseVerdict tests
// ---------------------------------------------------------------------------

func TestParseVerdict_Pass(t *testing.T) {
	v := ParseVerdict("Status: pass\n全部通过")
	if v.Status != StatusPass {
		t.Errorf("expected Pass, got %v", v.Status)
	}
	// Also test with Chinese keywords
	v2 := ParseVerdict("✅ 通过 一切正常")
	if v2.Status != StatusPass {
		t.Errorf("expected Pass, got %v", v2.Status)
	}
}

func TestParseVerdict_Blocked(t *testing.T) {
	v := ParseVerdict("Status: blocked\n发现严重问题")
	if v.Status != StatusBlocked {
		t.Errorf("expected Blocked, got %v", v.Status)
	}
}

func TestParseVerdict_Default(t *testing.T) {
	v := ParseVerdict("普通文本无特殊标记")
	if v.Status != StatusNeedsRevision {
		t.Errorf("expected default NeedsRevision, got %v", v.Status)
	}
	if v.Score != 0.5 {
		t.Errorf("expected default score 0.5, got %v", v.Score)
	}
}

// ---------------------------------------------------------------------------
// Extension tests
// ---------------------------------------------------------------------------

func TestNewExtension_WithNilCatalog(t *testing.T) {
	e := NewExtension(nil)
	if e == nil {
		t.Fatal("NewExtension(nil) returned nil")
	}
	if e.Name() != ExtensionName {
		t.Errorf("expected name 'checker', got %q", e.Name())
	}
	// Should have default catalog
	c := e.Catalog()
	if c == nil || len(c.List()) == 0 {
		t.Error("expected non-empty default catalog")
	}
}

func TestNewExtension_WithCustomCatalog(t *testing.T) {
	c := NewCatalog()
	c.Register(CheckerEntry{RoleID: "custom"})
	e := NewExtension(c)
	if len(e.Catalog().List()) != 1 {
		t.Errorf("expected 1 entry from custom catalog, got %d", len(e.Catalog().List()))
	}
}

func TestExtension_Name(t *testing.T) {
	e := NewExtension(NewCatalog())
	if e.Name() != "checker" {
		t.Errorf("expected name 'checker', got %q", e.Name())
	}
}

func TestExtension_RegisterHandler(t *testing.T) {
	e := NewExtension(NewCatalog())
	e.RegisterHandler("test", func(_ context.Context, _, _ string) (CheckerVerdict, error) {
		return CheckerVerdict{Status: StatusPass}, nil
	})
	d := e.Dispatch()
	if d.GetHandler("test") == nil {
		t.Error("handler should be registered")
	}
}

func TestExtension_InitAndDispose(t *testing.T) {
	e := NewExtension(NewCatalog())
	if err := e.Init(context.Background(), nil); err != nil {
		t.Errorf("Init error: %v", err)
	}
	if err := e.Dispose(); err != nil {
		t.Errorf("Dispose error: %v", err)
	}
}

func TestExtension_Tools(t *testing.T) {
	e := NewExtension(NewCatalog())
	tools := e.Tools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["suggest_checkers"] {
		t.Error("expected 'suggest_checkers' tool")
	}
	if !names["run_checker_review"] {
		t.Error("expected 'run_checker_review' tool")
	}
}

func TestExtension_SystemPromptSuffix(t *testing.T) {
	e := NewExtension(NewCatalog())
	suffix := e.SystemPromptSuffix()
	if !strings.Contains(suffix, "suggest_checkers") {
		t.Error("suffix should mention suggest_checkers tool")
	}
	if !strings.Contains(suffix, "run_checker_review") {
		t.Error("suffix should mention run_checker_review tool")
	}
	// Should list all default checkers
	for _, entry := range e.catalog.List() {
		if !strings.Contains(suffix, entry.RoleID) {
			t.Errorf("suffix should mention checker %q", entry.RoleID)
		}
	}
}

// ---------------------------------------------------------------------------
// Interface compliance checks (compile-time)
// ---------------------------------------------------------------------------

var _ = NewExtension(NewCatalog()).Dispose
