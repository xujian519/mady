package chat

// M76：对话转录（ChatHistory.Render）的整段 golden 取样。
//
// M75 只覆盖了 tool_card / approval_card 的组件级输出；真正最常被看到的是
// 对话转录本体——用户消息换行、助手 Markdown（含表格）列宽、思考块折叠、
// 连续工具调用归并、窄宽度下的代码块降级。这些都由 Render 的分支组合决定，
// 单点断言（含某字符/某头）无法发现「整体错位一列」的回归。
//
// 本文件把整段渲染结果落到 testdata/render_golden.golden，并断言每行不超过目标宽度。
// 更新入口：go test ./chat -run TestRenderGoldenTranscript -count=1 -args -update

import (
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
)

var updateTranscriptGolden = flag.Bool("update", false, "重建 chat/testdata/render_golden.golden")

const transcriptGoldenPath = "testdata/render_golden.golden"

type transcriptCase struct {
	name   string
	width  int64
	render func() []string
}

func userMessage(text string) ChatMessage {
	return ChatMessage{Role: RoleUser, Text: text, At: time.Unix(0, 0).UTC()}
}

func assistantMessage(text string) ChatMessage {
	return ChatMessage{Role: RoleAssistant, Text: text, At: time.Unix(0, 0).UTC()}
}

func transcriptCases() []transcriptCase {
	return []transcriptCase{
		{
			// 纯 CJK 用户消息：断行不能在多字节字符中间切开。
			name:  "user/cjk-wrap",
			width: 40,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(userMessage("请核对权利要求 1 中的必要技术特征是否与说明书一致，并给出修改建议。"))
				return h.Render(40)
			},
		},
		{
			// 助手 Markdown 表格：列宽对齐依赖 CJK 宽度口径。
			name:  "assistant/markdown-table",
			width: 60,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(assistantMessage("结论如下：\n\n| 项目 | 判断 |\n|---|---|\n| 新颖性 | 具备 |\n| 创造性 | 待补充证据 |"))
				return h.Render(60)
			},
		},
		{
			// 流式中的思考段：仅 Pending 时参与光标追加，正文为空时降级为 …。
			name:  "assistant/thinking-streaming",
			width: 50,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(ChatMessage{
					Role:    RoleAssistant,
					Text:    "",
					Pending: true,
					ThinkingSegments: []ThinkingSegment{
						{Text: "先确认对比文件 1 的公开日，再逐项比对技术特征。", Collapsed: false},
					},
					At: time.Unix(0, 0).UTC(),
				})
				return h.Render(50)
			},
		},
		{
			// 流式未完成：Pending 态的光标与增量文本拼接。
			name:  "assistant/streaming-pending",
			width: 40,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(ChatMessage{Role: RoleAssistant, Text: "正在核对证据", Pending: true, At: time.Unix(0, 0).UTC()})
				return h.Render(40)
			},
		},
		{
			// 连续工具调用：CollapseConsecutiveTools 行为与摘要行文案。
			name:  "tool/consecutive-collapsed",
			width: 50,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(ChatMessage{Role: RoleTool, Text: "匹配到 12 处", Meta: "grep", Seq: 1, At: time.Unix(0, 0).UTC()})
				h.Append(ChatMessage{Role: RoleTool, Text: "读取完成", Meta: "read", Seq: 2, At: time.Unix(0, 0).UTC()})
				h.CollapseConsecutiveTools()
				return h.Render(50)
			},
		},
		{
			// 窄宽度下的代码块：必须降级而非溢出。
			name:  "assistant/code-block-narrow",
			width: 36,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(assistantMessage("示例：\n\n```go\nfunc main() { println(\"证据采信\") }\n```"))
				return h.Render(36)
			},
		},
		{
			// 错误消息：角色前缀与换行。
			name:  "error/long-cjk",
			width: 40,
			render: func() []string {
				h := NewChatHistory()
				h.SetMaxRows(200)
				h.Append(ChatMessage{Role: RoleError, Text: "工具调用失败：检索接口返回 429，请稍后重试或改用号单检索。", At: time.Unix(0, 0).UTC()})
				return h.Render(40)
			},
		},
	}
}

func TestRenderGoldenTranscript(t *testing.T) {
	cases := transcriptCases()

	var buf strings.Builder
	for _, c := range cases {
		lines := c.render()
		if len(lines) == 0 {
			t.Fatalf("用例 %s 渲染为空", c.name)
		}
		buf.WriteString("### " + c.name + "\n")
		for _, line := range lines {
			if got := core.VisibleWidth(line); got > c.width {
				t.Errorf("用例 %s 行宽 %d 超出目标宽度 %d：%q", c.name, got, c.width, line)
			}
			// golden 只记录布局：去 ANSI 色码、裁行尾空格（避免钩子改写）。
			buf.WriteString(strings.TrimRight(core.StripAnsi(line), " ") + "\n")
		}
		buf.WriteString("\n")
	}
	// 文件以单个换行结尾，避免 end-of-file-fixer 改写后与生成结果不一致。
	got := strings.TrimRight(buf.String(), "\n") + "\n"

	if *updateTranscriptGolden {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatalf("创建 testdata 目录失败：%v", err)
		}
		if err := os.WriteFile(transcriptGoldenPath, []byte(got), 0o600); err != nil {
			t.Fatalf("写入 golden 失败：%v", err)
		}
		t.Logf("已更新 golden：%s（%d 个用例）", transcriptGoldenPath, len(cases))
		return
	}

	want, err := os.ReadFile(transcriptGoldenPath)
	if err != nil {
		t.Fatalf("读取 golden 失败（首次运行请加 -args -update）：%v", err)
	}
	if got == string(want) {
		return
	}
	t.Fatalf("转录渲染与 golden 不一致：确认布局改动符合预期后执行 " +
		"`go test ./chat -run TestRenderGoldenTranscript -count=1 -args -update` 更新 fixture")
}
