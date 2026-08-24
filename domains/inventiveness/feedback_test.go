package inventiveness

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAndLoadFeedback(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case-1")
	if err := AppendInventivenessFeedback(dir, FeedbackEntry{Action: ActionRejection, Reason: "理由不成立"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := AppendInventivenessFeedback(dir, FeedbackEntry{Action: ActionModification, Reason: "应提高置信度"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := LoadInventivenessFeedback(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Action != ActionRejection || got[0].Reason != "理由不成立" {
		t.Errorf("entry0: %+v", got[0])
	}
	if got[1].Action != ActionModification {
		t.Errorf("entry1 action = %s", got[1].Action)
	}
}

func TestLoadFeedback_MissingFile(t *testing.T) {
	got, err := LoadInventivenessFeedback(filepath.Join(t.TempDir(), "no-such"))
	if err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty for missing file, got %d", len(got))
	}
}

func TestSummarizeFeedback(t *testing.T) {
	entries := []FeedbackEntry{
		{Action: ActionRejection, Reason: "三步法分析缺失"},
		{Action: ActionModification, Reason: "补充预料不到的效果"},
	}
	summary := SummarizeInventivenessFeedback(entries)
	if !strings.Contains(summary, "驳回") || !strings.Contains(summary, "三步法分析缺失") {
		t.Errorf("summary should mention rejection + reason: %s", summary)
	}
	if !strings.Contains(summary, "修正") || !strings.Contains(summary, "补充预料不到的效果") {
		t.Errorf("summary should mention modification + reason: %s", summary)
	}

	if got := SummarizeInventivenessFeedback(nil); got != "" {
		t.Errorf("expected empty summary for nil, got %q", got)
	}
}

func TestFeedbackPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MADY_HOME", home)
	caseID := "case-7"
	dir := filepath.Join(home, "cases", caseID)

	if got := FeedbackPrompt(caseID); got != "" {
		t.Errorf("expected empty prompt before any feedback, got %q", got)
	}

	if err := AppendInventivenessFeedback(dir, FeedbackEntry{Action: ActionRejection, Reason: "事后诸葛亮"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	prompt := FeedbackPrompt(caseID)
	if !strings.Contains(prompt, "历史用户反馈") || !strings.Contains(prompt, "事后诸葛亮") {
		t.Errorf("expected prompt to include history + reason, got: %q", prompt)
	}
}

func TestCaseFeedbackDir_Sanitized(t *testing.T) {
	t.Setenv("MADY_HOME", t.TempDir())
	if got := CaseFeedbackDir("../evil"); got != "" {
		t.Errorf("expected empty for path-traversal caseID, got %q", got)
	}
	if got := CaseFeedbackDir(""); got != "" {
		t.Errorf("expected empty for empty caseID, got %q", got)
	}
	if got := CaseFeedbackDir("case-a"); got == "" {
		t.Error("expected a dir for a bare case id")
	}
}
