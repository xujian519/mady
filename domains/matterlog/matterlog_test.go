package matterlog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func defectRules(defects []Defect) []string {
	rules := make([]string, 0, len(defects))
	for _, defect := range defects {
		rules = append(rules, defect.Rule)
	}
	return rules
}

func hasDefect(defects []Defect, rule string) bool {
	for _, defect := range defects {
		if defect.Rule == rule {
			return true
		}
	}
	return false
}

func alwaysExists(string) bool { return true }

func TestDefectsValid(t *testing.T) {
	defects := Defects("", Request{
		Action: "建案", Artifacts: []string{"docs/claim.md"}, Changes: "完成初始权利要求",
	}, alwaysExists)
	if len(defects) != 0 {
		t.Fatalf("defects = %v", defectRules(defects))
	}
}

func TestDefectsBasicRules(t *testing.T) {
	defects := Defects("", Request{Action: "撰写", Changes: " "}, func(string) bool { return false })
	for _, rule := range []string{"no_artifacts", "no_changes"} {
		if !hasDefect(defects, rule) {
			t.Errorf("缺少 %s：%v", rule, defectRules(defects))
		}
	}
	missing := Defects("", Request{Action: "撰写", Artifacts: []string{"a.md"}, Changes: "x"},
		func(string) bool { return false })
	if !hasDefect(missing, "artifact_missing") {
		t.Errorf("missing = %v", defectRules(missing))
	}
}

func TestDefectsSupersedeRules(t *testing.T) {
	existing := RenderEntry(Request{
		Action: "撰写", Artifacts: []string{"v1.md"}, Changes: "初稿",
	}, time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)) + "\n"

	overwrite := Defects(existing, Request{
		Action: "修订", Artifacts: []string{"v1.md"}, Changes: "改", Supersedes: []string{"v1.md"},
	}, alwaysExists)
	if !hasDefect(overwrite, "overwrite_in_place") {
		t.Errorf("overwrite = %v", defectRules(overwrite))
	}

	unknown := Defects(existing, Request{
		Action: "修订", Artifacts: []string{"v2.md"}, Changes: "改", Supersedes: []string{"ghost.md"},
	}, alwaysExists)
	if !hasDefect(unknown, "unknown_supersede") {
		t.Errorf("unknown = %v", defectRules(unknown))
	}

	reused := Defects(existing, Request{
		Action: "修订", Artifacts: []string{"v1.md"}, Changes: "改",
	}, alwaysExists)
	if !hasDefect(reused, "reused_artifact_path") {
		t.Errorf("reused = %v", defectRules(reused))
	}
}

func TestDefectsForkedChain(t *testing.T) {
	first := RenderEntry(Request{Action: "撰写", Artifacts: []string{"v1.md"}, Changes: "初稿"},
		time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC))
	second := RenderEntry(Request{
		Action: "修订", Artifacts: []string{"v2.md"}, Changes: "修订", Supersedes: []string{"v1.md"},
	}, time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC))
	existing := first + "\n" + second + "\n"

	defects := Defects(existing, Request{
		Action: "修订", Artifacts: []string{"v3.md"}, Changes: "再改", Supersedes: []string{"v1.md"},
	}, alwaysExists)
	if !hasDefect(defects, "forked_chain") {
		t.Errorf("defects = %v", defectRules(defects))
	}
}

func TestRenderAndParseEntry(t *testing.T) {
	now := time.Date(2026, 10, 28, 9, 30, 0, 0, time.FixedZone("CST", 8*3600))
	entry := RenderEntry(Request{
		Action: "撰写", Artifacts: []string{"v1.md", "figs/fig1.svg"}, Changes: "完成初稿 | 含分隔符",
		Approver: "徐健", Note: "补充附图",
	}, now)
	if !strings.Contains(entry, "产物 v1.md、figs/fig1.svg") {
		t.Errorf("entry = %q", entry)
	}
	if !strings.Contains(entry, "| 取代 — | 徐健 |") {
		t.Errorf("entry = %q", entry)
	}
	if !strings.Contains(entry, "完成初稿 | 含分隔符（备注：补充附图）") {
		t.Errorf("自由文本应保持在最后一个字段：%q", entry)
	}

	parsed := ParseEntries(entry + "\n")
	if len(parsed) != 1 {
		t.Fatalf("parsed = %+v", parsed)
	}
	if parsed[0].Action != "撰写" || len(parsed[0].Artifacts) != 2 || parsed[0].Artifacts[1] != "figs/fig1.svg" {
		t.Errorf("parsed = %+v", parsed[0])
	}
	if len(parsed[0].Supersedes) != 0 {
		t.Errorf("supersedes = %v", parsed[0].Supersedes)
	}
}

func runMatterTool(t *testing.T, workDir, args string) (ToolResult, error) {
	t.Helper()
	raw, err := NewTool(ToolConfig{WorkingDir: workDir}).Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return ToolResult{}, err
	}
	result, ok := raw.(ToolResult)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return result, nil
}

func TestMatterToolLifecycle(t *testing.T) {
	workDir := t.TempDir()
	caseDir := filepath.Join(workDir, "case-001")
	if err := os.MkdirAll(filepath.Join(caseDir, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docs/v1.md", "docs/v2.md"} {
		if err := os.WriteFile(filepath.Join(caseDir, name), []byte("内容"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// 日志不存在时非「建案」动作被拒。
	if _, err := runMatterTool(t, workDir, `{"case_dir":"case-001","action":"撰写","artifacts":["docs/v1.md"],"changes":"初稿"}`); err == nil {
		t.Fatal("日志不存在时非建案动作应报错")
	}

	first, err := runMatterTool(t, workDir, `{"case_dir":"case-001","action":"建案","artifacts":["docs/v1.md"],"changes":"建立案卷"}`)
	if err != nil {
		t.Fatalf("建案: %v", err)
	}
	if first.PriorEntries != 0 || first.LogPath != filepath.Join("case-001", LogFileName) {
		t.Errorf("first = %+v", first.AppendResult)
	}

	second, err := runMatterTool(t, workDir, `{"case_dir":"case-001","action":"修订","artifacts":["docs/v2.md"],"changes":"限缩权1","supersedes":["docs/v1.md"]}`)
	if err != nil {
		t.Fatalf("修订: %v", err)
	}
	if second.PriorEntries != 1 || len(second.Superseded) != 1 {
		t.Errorf("second = %+v", second.AppendResult)
	}
	if !strings.Contains(second.Rendered, "取代链：docs/v1.md") {
		t.Errorf("rendered = %q", second.Rendered)
	}

	// 原地覆盖被拒。
	if _, err := runMatterTool(t, workDir, `{"case_dir":"case-001","action":"修订","artifacts":["docs/v2.md"],"changes":"再改","supersedes":["docs/v2.md"]}`); err == nil ||
		!strings.Contains(err.Error(), "overwrite_in_place") {
		t.Fatalf("原地覆盖应被拒，err = %v", err)
	}

	// 越界目录被拒。
	if _, err := runMatterTool(t, workDir, `{"case_dir":"../outside","action":"建案","artifacts":["a.md"],"changes":"x"}`); err == nil {
		t.Fatal("越界 case_dir 应报错")
	}
}

func TestMatterToolRejectsUnknownAction(t *testing.T) {
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "c"), 0o750); err != nil {
		t.Fatal(err)
	}
	_, err := runMatterTool(t, workDir, `{"case_dir":"c","action":"随便","artifacts":["a.md"],"changes":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "闭集") {
		t.Fatalf("err = %v", err)
	}
}
