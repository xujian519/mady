package chat

// collapsible_test.go — ChatMessage.IsCollapsible 统一折叠判定 + evidence 卡
// 点击折叠的端到端验证。

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/tui/component"
	"github.com/xujian519/mady/tui/core"
)

func TestChatMessageIsCollapsible(t *testing.T) {
	cases := []struct {
		name string
		msg  ChatMessage
		want bool
	}{
		{"tool result", ChatMessage{Role: RoleTool, Text: "ok"}, true},
		{"assistant diff meta", ChatMessage{Role: RoleAssistant, Meta: "diff", Text: "x"}, true},
		{"assistant already collapsed", ChatMessage{Role: RoleAssistant, Collapsed: true, Text: "x"}, true},
		{"assistant plain", ChatMessage{Role: RoleAssistant, Text: "普通回复"}, false},
		{"user message", ChatMessage{Role: RoleUser, Text: "hi"}, false},
		{"system message", ChatMessage{Role: RoleSystem, Text: "note"}, false},
		{"evidence card", ChatMessage{Role: RoleAssistant, DomainMsg: &component.DomainMessage{Type: component.DomainMsgTypeEvidenceCard}}, true},
		{"conclusion card", ChatMessage{Role: RoleAssistant, DomainMsg: &component.DomainMessage{Type: component.DomainMsgTypeConclusionCard}}, false},
		{"approval card", ChatMessage{Role: RoleAssistant, DomainMsg: &component.DomainMessage{Type: component.DomainMsgTypeApprovalPrompt}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.msg.IsCollapsible(); got != c.want {
				t.Errorf("IsCollapsible() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestEvidenceCardClickToggle 验证证据卡点击折叠/展开（P0-4 新增能力：
// 之前 evidence card 无点击折叠入口，现经 IsCollapsible 统一支持）。
func TestEvidenceCardClickToggle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{
		Role: RoleAssistant,
		DomainMsg: &component.DomainMessage{
			Type:  component.DomainMsgTypeEvidenceCard,
			Title: "证据核对",
			Spans: []component.EvidenceRef{{SourceURI: "file://d.pdf", Direction: component.DirectionSupporting}},
		},
	})
	cols, _ := app.TerminalSize()
	hist.SetMaxRows(10)
	app.layout.Render(cols)

	// 初始展开：证据行可见。
	lines := hist.Render(cols)
	if !joinedContains(lines, "supporting") {
		t.Fatalf("expanded evidence card should show evidence rows, got:\n%s", joinLines(lines))
	}

	// 点击 header 折叠。
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	lines = hist.Render(cols)
	if joinedContains(lines, "supporting") {
		t.Errorf("collapsed evidence card should hide evidence rows, got:\n%s", joinLines(lines))
	}
	if !joinedContains(lines, "证据") {
		t.Errorf("collapsed evidence card should show summary title, got:\n%s", joinLines(lines))
	}

	// 再次点击展开。
	app.layout.handleMouseMsg(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 0, Button: 1})
	lines = hist.Render(cols)
	if !joinedContains(lines, "supporting") {
		t.Errorf("second click should expand the evidence card, got:\n%s", joinLines(lines))
	}
}

func joinedContains(lines []string, sub string) bool {
	return strings.Contains(strings.Join(lines, "\n"), sub)
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
