package generate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func runTool(t *testing.T, tool *agentcore.Tool, args string) (ToolResult, error) {
	t.Helper()
	result, err := tool.Func(context.Background(), json.RawMessage(args))
	if err != nil {
		return ToolResult{}, err
	}
	typed, ok := result.(ToolResult)
	if !ok {
		t.Fatalf("expected ToolResult, got %T", result)
	}
	return typed, nil
}

func TestToolGeneratesFlowchart(t *testing.T) {
	requireDot(t)
	workDir := t.TempDir()
	tool := NewTool(ToolConfig{WorkingDir: workDir})

	result, err := runTool(t, tool, `{
		"kind": "flowchart",
		"output": "figures/fig1.svg",
		"figure_number": 1,
		"steps": [
			{"id": "start", "label": "开始", "shape": "ellipse", "next": ["check"]},
			{"id": "check", "label": "合格？", "shape": "diamond",
			 "next": [{"id": "done", "label": "是"}, {"id": "start", "label": "否"}]},
			{"id": "done", "label": "结束", "shape": "ellipse"}
		]
	}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.File != filepath.Join("figures", "fig1.svg") {
		t.Errorf("file = %q", result.File)
	}
	if result.WidthMm <= 0 || result.HeightMm <= 0 {
		t.Errorf("缺少图面尺寸: %+v", result)
	}
	if !strings.Contains(result.DOT, `label="100. 开始"`) {
		t.Errorf("DOT 标号不符:\n%s", result.DOT)
	}
	if _, err := os.Stat(filepath.Join(workDir, result.File)); err != nil {
		t.Fatalf("产物不存在: %v", err)
	}
}

func TestToolSupportsChineseFilename(t *testing.T) {
	requireDot(t)
	workDir := t.TempDir()
	tool := NewTool(ToolConfig{WorkingDir: workDir})
	result, err := runTool(t, tool, `{
		"kind": "hierarchy",
		"output": "附图/图1.svg",
		"nodes": [{"id": "root", "label": "系统", "children": [{"id": "a", "label": "部件"}]}]
	}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.File != filepath.Join("附图", "图1.svg") {
		t.Errorf("file = %q", result.File)
	}
	if _, err := os.Stat(filepath.Join(workDir, "附图", "图1.svg")); err != nil {
		t.Fatalf("中文文件名产物不存在: %v", err)
	}
}

func TestToolTemplateAndBlock(t *testing.T) {
	requireDot(t)
	workDir := t.TempDir()
	tool := NewTool(ToolConfig{WorkingDir: workDir})

	if _, err := runTool(t, tool, `{"kind":"template","template":"system_block","output":"block.pdf"}`); err != nil {
		t.Fatalf("template: %v", err)
	}
	if _, err := runTool(t, tool, `{"kind":"block","output":"state.png","blocks":[{"id":"a","label":"输入","type":"input"}],"connections":[]}`); err != nil {
		t.Fatalf("block: %v", err)
	}
}

func TestToolAnnotatesReferences(t *testing.T) {
	requireDot(t)
	workDir := t.TempDir()
	tool := NewTool(ToolConfig{WorkingDir: workDir})
	result, err := runTool(t, tool, `{
		"kind": "hierarchy",
		"output": "refs.svg",
		"embed_numerals": false,
		"nodes": [{"id": "controller", "label": "控制器"}],
		"references": [{"label": "控制器", "numeral": "20"}, {"label": "缺失部件", "numeral": "30"}]
	}`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(workDir, result.File))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "控制器 (20)") {
		t.Errorf("标号未写入 SVG")
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "缺失部件" {
		t.Errorf("未命中参考应作为 warning: %v", result.Warnings)
	}
}

func TestToolRejectsBadInput(t *testing.T) {
	workDir := t.TempDir()
	tool := NewTool(ToolConfig{WorkingDir: workDir})
	typed := tool

	cases := []struct {
		name string
		args string
		want string
	}{
		{"输出越界", `{"kind":"flowchart","output":"../escape.svg","steps":[{"id":"a","label":"x"}]}`, "必须位于工作目录"},
		{"格式与扩展名不一致", `{"kind":"flowchart","output":"a.png","format":"svg","steps":[{"id":"a","label":"x"}]}`, "不一致"},
		{"未知 kind", `{"kind":"unknown","output":"a.svg"}`, "未知的 kind"},
		{"模板缺失", `{"kind":"template","output":"a.svg"}`, "必须提供 template"},
		{"引用仅限 SVG", `{"kind":"flowchart","output":"a.png","steps":[{"id":"a","label":"x"}],"references":[{"label":"x","numeral":"10"}]}`, "仅支持 svg"},
		{"缺少输出", `{"kind":"flowchart","steps":[{"id":"a","label":"x"}]}`, "output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := typed.Func(context.Background(), json.RawMessage(tc.args))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// 工作目录内的符号链接不能把产物写到边界之外。
func TestToolRejectsSymlinkEscape(t *testing.T) {
	workDir := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(workDir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("当前平台不支持符号链接: %v", err)
	}
	tool := NewTool(ToolConfig{WorkingDir: workDir})
	_, err := tool.Func(context.Background(), json.RawMessage(
		`{"kind":"flowchart","output":"escape/fig.svg","steps":[{"id":"a","label":"x"}]}`))
	if err == nil || !strings.Contains(err.Error(), "符号链接") {
		t.Fatalf("expected symlink escape error, got %v", err)
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Errorf("越界目录不应有产物: %v %v", entries, readErr)
	}
}

func TestToolDefaultsCJKFont(t *testing.T) {
	workDir := t.TempDir()
	tool := NewTool(ToolConfig{WorkingDir: workDir})
	result, err := runTool(t, tool, `{
		"kind": "flowchart", "output": "font.svg",
		"steps": [{"id": "a", "label": "开始"}]
	}`)
	if err != nil {
		t.Skipf("skip when Graphviz is unavailable: %v", err)
	}
	if !strings.Contains(result.DOT, "fontname=\"") {
		t.Errorf("DOT 缺少字体声明:\n%s", result.DOT)
	}
}
