package component

// M75：关键卡片的渲染回归取样（golden）。
//
// tool_card / approval_card 的换行、截断、边框对齐与宽字符（CJK）宽度口径
// 曾被 M38 修过一次，但此后没有「整卡输出」级别的护栏——现有用例只断言
// 片段（颜色标记、是否含 diff 头等），改错一列宽度不会被发现。
//
// 本文件把若干代表性卡片的**完整输出**落到 testdata/render_golden.txt，
// 并额外断言每条渲染行不超过目标宽度（宽字符回归的硬约束）。
// 更新入口：go test ./component -run TestRenderGolden -count=1 -args -update

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

var updateRenderGolden = flag.Bool("update", false, "重建 component/testdata/render_golden.txt")

const renderGoldenPath = "testdata/render_golden.txt"

// identity 让 golden 只反映布局，不掺入样式字符。
func identity(s string) string { return s }

func goldenToolCardTheme() ToolCardTheme {
	return ToolCardTheme{
		Border:        identity,
		Success:       identity,
		Error:         identity,
		Title:         identity,
		Dim:           identity,
		MarkdownTheme: defaultMarkdownTheme(),
	}
}

func goldenApprovalTheme() ApprovalCardTheme {
	return ApprovalCardTheme{
		Title:         identity,
		Border:        identity,
		Warning:       identity,
		Dim:           identity,
		Action:        identity,
		Body:          identity,
		MarkdownTheme: defaultMarkdownTheme(),
	}
}

type renderCase struct {
	name  string
	width int64
	lines []string
}

func renderCases() []renderCase {
	toolTheme := goldenToolCardTheme()
	approvalTheme := goldenApprovalTheme()

	return []renderCase{
		{
			name:  "tool_card/running",
			width: 40,
			lines: RenderToolCard(ToolCardConfig{
				Name:   "grep",
				Status: "running",
				Seq:    3,
			}, toolTheme, 40),
		},
		{
			name:  "tool_card/completed-with-duration",
			width: 40,
			lines: RenderToolCard(ToolCardConfig{
				Name:     "patent_lookup",
				Status:   "✓ done",
				Duration: 1200 * time.Millisecond,
				Seq:      1,
			}, toolTheme, 40),
		},
		{
			// 长 CJK 错误信息：覆盖截断与宽字符口径。
			name:  "tool_card/failed-cjk-truncate",
			width: 40,
			lines: RenderToolCard(ToolCardConfig{
				Name:   "cnipr_search",
				Status: "✗ failed: 检索接口返回超时，请检查网络连接后重试，或改用号单检索",
			}, toolTheme, 40),
		},
		{
			name:  "tool_card/collapsed",
			width: 40,
			lines: RenderToolCard(ToolCardConfig{
				Name:      "edit",
				Status:    "✓ done",
				Collapsed: true,
				DiffText:  "--- a/x.md\n+++ b/x.md\n-旧内容\n+新内容",
			}, toolTheme, 40),
		},
		{
			name:  "tool_card/diff-expanded-cjk",
			width: 80,
			lines: RenderToolCard(ToolCardConfig{
				Name:     "edit",
				Status:   "✓ done",
				DiffText: "--- a/权利要求书.md\n+++ b/权利要求书.md\n-一种装置，包括处理器。\n+一种装置，包括处理器与存储器。",
			}, toolTheme, 80),
		},
		{
			name:  "approval_card/basic-cjk",
			width: 50,
			lines: RenderApprovalCard(&DomainMessage{
				Title:      "新颖性判断结论待确认",
				Confidence: 0.8,
				Spans: []EvidenceRef{
					{Snippet: "对比文件 1 公开了全部技术特征", Direction: DirectionSupporting},
					{Snippet: "对比文件 2 未涉及该区别特征", Direction: DirectionContradicting},
				},
				Actions: []DomainAction{
					{Label: "/approve", Command: "批准通过"},
					{Label: "/reject", Command: "驳回"},
				},
				Body: "请核对上述证据引用是否完整。",
			}, approvalTheme, 50),
		},
		{
			name:  "approval_card/narrow",
			width: 32,
			lines: RenderApprovalCard(&DomainMessage{
				Title:      "复核门：证据采信",
				Confidence: 0.55,
				Actions: []DomainAction{
					{Label: "/ok", Command: "采信"},
				},
			}, approvalTheme, 32),
		},
	}
}

func TestRenderGolden(t *testing.T) {
	cases := renderCases()

	var buf strings.Builder
	for _, c := range cases {
		buf.WriteString("### " + c.name + "\n")
		if len(c.lines) == 0 {
			t.Fatalf("用例 %s 渲染为空", c.name)
		}
		for _, line := range c.lines {
			// 宽字符回归的硬约束：任何一行都不得超出目标宽度。
			if got := core.VisibleWidth(line); got > c.width {
				t.Errorf("用例 %s 行宽 %d 超出目标宽度 %d：%q", c.name, got, c.width, line)
			}
			// golden 只记录布局：去掉 ANSI 色码（避免依赖调色板/终端能力），
			// 并裁掉行尾空格（避免与 pre-commit 的 trailing-whitespace 冲突）。
			buf.WriteString(strings.TrimRight(core.StripAnsi(line), " ") + "\n")
		}
		buf.WriteString("\n")
	}
	// 文件以单个换行结尾：多出的空行会被 pre-commit 的 end-of-file-fixer 裁掉，
	// 导致「刚 -update 的 golden 提交后再跑就失败」。
	got := strings.TrimRight(buf.String(), "\n") + "\n"

	if *updateRenderGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("创建 testdata 目录失败：%v", err)
		}
		if err := os.WriteFile(renderGoldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("写入 golden 失败：%v", err)
		}
		t.Logf("已更新 golden：%s（%d 个用例）", renderGoldenPath, len(cases))
		return
	}

	want, err := os.ReadFile(renderGoldenPath)
	if err != nil {
		t.Fatalf("读取 golden 失败（首次运行请加 -args -update）：%v", err)
	}
	if got == string(want) {
		return
	}
	t.Fatalf("渲染输出与 golden 不一致：确认布局改动符合预期后执行 " +
		"`go test ./component -run TestRenderGolden -count=1 -args -update` 更新 fixture")
}
