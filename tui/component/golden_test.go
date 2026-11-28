package component

// golden_test.go — 渲染输出字节级快照测试（对齐 charmbracelet/catwalk 思路）。
//
// 覆盖四个纯渲染函数：RenderToolCard / RenderEvidenceCard /
// RenderConclusionCard / RenderApprovalCard。每个函数输入固定（数据 + theme +
// width），输出与 testdata/*.golden 逐字节比对，捕获宽度/样式/布局漂移等
// "断言包含子串"式测试无法发现的回归。
//
// 更新基线（仅在有意的渲染变更时执行，并必须人工审阅 git diff）：
//
//	GOLDEN_UPDATE=1 go test ./component/ -run Golden -count=1

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/theme"
)

// TestMain 固定全局调色板为 Light + 256 色模式，使 golden 基线在任意终端
// 环境（CI 16 色 / 本地 truecolor）下稳定：颜色编码统一为 \x1b[38;5;Nm。
// 在并行测试开始前执行完毕，无竞态；现有测试仅断言文本子串，不受影响。
func TestMain(m *testing.M) {
	theme.SyncPaletteGlobals(theme.DefaultSemanticLight(), theme.ColorMode256)
	// 颜色开关必须显式固定：CI 的 stdout 不是 TTY，ColorEnabled() 会自动
	// 关闭颜色，导致同一份 golden 在本地与 CI 产生不同字节。
	theme.ForceColor(true)
	// 宽度口径同样必须显式固定：Ambiguous 字符（→、—、…）在 CJK 模式下
	// 按 2 列计算，会让同一输入在不同 LANG 下产生不同 padding 字节。
	// golden 基线按非 CJK 口径生成，这里固定为 false 以保证跨环境稳定。
	core.SetCJKMode(false)
	os.Exit(m.Run())
}

// goldenMatch 将 lines 与 testdata/<name>.golden 比对；GOLDEN_UPDATE=1 时
// 重新生成基线文件。
func goldenMatch(t *testing.T, name string, lines []string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	got := strings.Join(lines, "\n") + "\n"
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with GOLDEN_UPDATE=1): %v", path, err)
	}
	if got != string(want) {
		t.Errorf("golden mismatch for %s\n--- want ---\n%s--- got ---\n%s", name, string(want), got)
	}
}

// goldenWidths 是快照覆盖的宽度矩阵：窄窗（折叠截断）/ 常规 / 宽窗。
var goldenWidths = []int64{40, 80, 120}

// --- RenderToolCard ---

func TestGoldenToolCard(t *testing.T) {
	cfg := ToolCardConfig{
		Name:     "edit_block",
		Status:   "done",
		Duration: 1250 * time.Millisecond,
		DiffText: "-func old() {\n+func new() {\n-}\n+}\n",
		Seq:      3,
	}
	for _, w := range goldenWidths {
		for _, collapsed := range []bool{false, true} {
			name := fmt.Sprintf("tool_card_w%d", w)
			if collapsed {
				name += "_collapsed"
			}
			t.Run(name, func(t *testing.T) {
				cfg.Collapsed = collapsed
				goldenMatch(t, name, RenderToolCard(cfg, DefaultToolCardTheme(), w))
			})
		}
	}
}

// DefaultToolCardTheme 从当前（已固定）调色板构建 ToolCardTheme，与
// chat 层 chat_history_render_message.go 的构造口径一致（→ 前缀 + Dim 样式）。
func DefaultToolCardTheme() ToolCardTheme {
	p := theme.CurrentPalette()
	return ToolCardTheme{
		Border:        p.BorderMuted.Render,
		Success:       p.Success.Render,
		Error:         p.Error.Render,
		Title:         func(s string) string { return p.Dim.Render(theme.SymbolArrow + " " + s) },
		Dim:           p.Dim.Render,
		MarkdownTheme: DefaultMarkdownTheme(),
	}
}

// TestGoldenToolCardStates 覆盖 RenderToolCard 的状态维度：上面那张表只跑
// 完成态（done），这里补 running（无结果摘要）与 failed（错误样式 + 长 CJK
// 截断），以及 CJK diff 的折行——三者都是「只有整卡快照才能发现错位」的场景。
func TestGoldenToolCardStates(t *testing.T) {
	cases := []struct {
		name string
		cfg  ToolCardConfig
	}{
		{
			name: "tool_card_running_w40",
			cfg:  ToolCardConfig{Name: "grep", Status: "running", Seq: 1},
		},
		{
			name: "tool_card_failed_cjk_w40",
			cfg: ToolCardConfig{
				Name:   "cnipr_search",
				Status: "✗ failed: 检索接口返回超时，请检查网络连接后重试，或改用号单检索",
			},
		},
		{
			name: "tool_card_diff_cjk_w80",
			cfg: ToolCardConfig{
				Name:     "edit",
				Status:   "✓ done",
				DiffText: "--- a/权利要求书.md\n+++ b/权利要求书.md\n-一种装置，包括处理器。\n+一种装置，包括处理器与存储器。",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			width := int64(40)
			if strings.HasSuffix(c.name, "w80") {
				width = 80
			}
			goldenMatch(t, c.name, RenderToolCard(c.cfg, DefaultToolCardTheme(), width))
		})
	}
}

// --- RenderEvidenceCard ---

func TestGoldenEvidenceCard(t *testing.T) {
	msg := &DomainMessage{
		Type:       DomainMsgTypeEvidenceCard,
		Title:      "新颖性证据核对",
		Confidence: 0.85,
		Spans: []EvidenceRef{
			{Snippet: "对比文件1公开了技术特征A（铰接部结构）", SourceURI: "file://d1.pdf", PageRange: "p3", Direction: DirectionSupporting},
			{Snippet: "对比文件2未公开区别特征B", SourceURI: "file://d2.pdf", PageRange: "p5", Direction: DirectionContradicting},
		},
		Body: "经比对，权利要求1相对对比文件1-2具备新颖性。",
	}
	for _, w := range goldenWidths {
		for _, collapsed := range []bool{false, true} {
			name := fmt.Sprintf("evidence_card_w%d", w)
			if collapsed {
				name += "_collapsed"
			}
			t.Run(name, func(t *testing.T) {
				goldenMatch(t, name, RenderEvidenceCard(msg, collapsed, DefaultEvidenceCardTheme(), w))
			})
		}
	}
}

// --- RenderConclusionCard ---

func TestGoldenConclusionCard(t *testing.T) {
	msg := &DomainMessage{
		Type:       DomainMsgTypeConclusionCard,
		Title:      "新颖性分析结论",
		Confidence: 0.85,
		Spans: []EvidenceRef{
			{Snippet: "D1公开特征A", Direction: DirectionSupporting},
			{Snippet: "D1未公开特征B", Direction: DirectionContradicting},
			{Snippet: "D2公开特征C", Direction: DirectionSupporting},
		},
		Extra: map[string]string{"classification": "novel"},
		Body:  "权利要求1-5具备新颖性，对比文件D1未公开特征B。",
	}
	for _, w := range goldenWidths {
		name := fmt.Sprintf("conclusion_card_w%d", w)
		t.Run(name, func(t *testing.T) {
			goldenMatch(t, name, RenderConclusionCard(msg, DefaultConclusionCardTheme(), w))
		})
	}
}

// --- RenderApprovalCard ---

func TestGoldenApprovalCard(t *testing.T) {
	msg := &DomainMessage{
		Type:       DomainMsgTypeApprovalPrompt,
		Title:      "权利要求修改审批",
		Confidence: 0.8,
		Spans:      []EvidenceRef{{Snippet: "修改未超出原说明书范围", Direction: DirectionSupporting}},
		Actions: []DomainAction{
			{Label: "批准", Command: "/approve"},
			{Label: "驳回", Command: "/reject"},
		},
		Body: "拟将技术特征A并入权利要求1。",
	}
	for _, w := range goldenWidths {
		name := fmt.Sprintf("approval_card_w%d", w)
		t.Run(name, func(t *testing.T) {
			goldenMatch(t, name, RenderApprovalCard(msg, DefaultApprovalCardTheme(), w))
		})
	}
}

// --- Input（输入区）---

// TestGoldenInput 覆盖输入区三种状态：空态占位符、CJK 多行折行（含光标）、
// 以及带 Markdown 片段的长输入。输入区是每轮对话必经路径，折行错位会直接
// 把光标放到错误的列上。
func TestGoldenInput(t *testing.T) {
	t.Run("input_placeholder_w40", func(t *testing.T) {
		in := NewInput(nil)
		in.SetPlaceholder("输入消息，/help 查看命令")
		goldenMatch(t, "input_placeholder_w40", in.Render(40))
	})

	t.Run("input_cjk_multiline_w80", func(t *testing.T) {
		in := NewInput(nil)
		in.SetFocused(true)
		in.SetValue("核对权利要求 1 的必要技术特征\n并给出修改建议：\n- 补充区别特征 B")
		goldenMatch(t, "input_cjk_multiline_w80", in.Render(80))
	})

	t.Run("input_long_wrap_w40", func(t *testing.T) {
		in := NewInput(nil)
		in.SetFocused(true)
		in.SetValue("请检索对比文件并逐项比对技术特征，输出差异表与结论。")
		goldenMatch(t, "input_long_wrap_w40", in.Render(40))
	})
}

// --- TodoPanel（任务面板 / plantask）---

// TestGoldenTodoPanel 覆盖任务面板的四种状态图标与摘要计数，以及窄宽度下的
// CJK 截断：plantask 事件（created/updated/interrupted）最终都汇入这里渲染。
func TestGoldenTodoPanel(t *testing.T) {
	items := []TodoItem{
		{ID: "t1", Content: "读取对比文件 D1", Status: "completed", Priority: "high"},
		{ID: "t2", Content: "逐项比对技术特征并记录差异", Status: "in_progress"},
		{ID: "t3", Content: "撰写新颖性结论", Status: "pending", Priority: "medium"},
		{ID: "t4", Content: "人工复核引用来源", Status: "canceled"},
	}

	t.Run("todo_panel_mixed_w60", func(t *testing.T) {
		p := NewTodoPanel()
		p.SetItems(items)
		goldenMatch(t, "todo_panel_mixed_w60", p.Render(60))
	})

	t.Run("todo_panel_narrow_w40", func(t *testing.T) {
		p := NewTodoPanel()
		p.SetItems([]TodoItem{
			{ID: "t1", Content: "逐项比对技术特征并记录差异（含附图标记核对与法条引用检查）", Status: "in_progress"},
		})
		goldenMatch(t, "todo_panel_narrow_w40", p.Render(40))
	})
}
