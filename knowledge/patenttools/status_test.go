package patenttools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runStatusTool(t *testing.T, toolCfg KnowledgeStatusConfig) KnowledgeStatusReport {
	t.Helper()
	tool := NewKnowledgeStatusTool(nil, toolCfg)
	raw, err := tool.Func(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	report, ok := raw.(KnowledgeStatusReport)
	if !ok {
		t.Fatalf("result type = %T", raw)
	}
	return report
}

func TestKnowledgeStatusEmptyDeployment(t *testing.T) {
	report := runStatusTool(t, KnowledgeStatusConfig{MadyHome: t.TempDir()})
	if report.Documents != 0 || report.LawsAvailable {
		t.Errorf("report = %+v", report)
	}
	joined := strings.Join(report.Guidance, "\n")
	for _, want := range []string{"knowledge.db 尚无文档", "未检测到法规库"} {
		if !strings.Contains(joined, want) {
			t.Errorf("guidance 缺少 %q：%s", want, joined)
		}
	}
	if !strings.Contains(report.Rendered, "【知识库状态】") {
		t.Errorf("rendered = %q", report.Rendered)
	}
}

func TestKnowledgeStatusSourceWithoutInstall(t *testing.T) {
	home := t.TempDir()
	knowledgeDir := filepath.Join(home, "knowledge")
	if err := os.MkdirAll(knowledgeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(knowledgeDir, "laws-full-local.db")
	if err := os.WriteFile(source, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := runStatusTool(t, KnowledgeStatusConfig{MadyHome: home})
	if report.LegalInstalled {
		t.Error("仅有源库时不应标记为已安装")
	}
	if report.LegalSourcePath != source {
		t.Errorf("source = %q", report.LegalSourcePath)
	}
	joined := strings.Join(report.Guidance, "\n")
	if !strings.Contains(joined, "legal_knowledge_install") {
		t.Errorf("guidance 应提示安装快照：%s", joined)
	}
}

func TestKnowledgeStatusInstalledSnapshot(t *testing.T) {
	home := t.TempDir()
	installed := filepath.Join(home, "legal-knowledge", "legal.db")
	if err := os.MkdirAll(filepath.Dir(installed), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := runStatusTool(t, KnowledgeStatusConfig{MadyHome: home})
	if !report.LegalInstalled {
		t.Errorf("应识别已安装快照：%+v", report)
	}
	if report.LegalQueryPath != installed {
		t.Errorf("query path = %q", report.LegalQueryPath)
	}
}

func TestKnowledgeStatusRejectsBadArgs(t *testing.T) {
	tool := NewKnowledgeStatusTool(nil, KnowledgeStatusConfig{})
	if _, err := tool.Func(context.Background(), json.RawMessage(`[]`)); err == nil {
		t.Error("非对象参数应报错")
	}
}
