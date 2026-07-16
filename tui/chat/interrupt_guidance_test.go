package chat

import (
	"strings"
	"testing"
)

func TestInterruptGuidance_DisclosureReview(t *testing.T) {
	got := interruptGuidance(AgentInterruptChatEvent{
		Reason: "技术交底书分析完成，请人工复核新颖性初判与分析报告",
		Data:   map[string]any{"gate": "disclosure_review", "report_id": "disclosure-001"},
	})

	assertContains(t, got, "技术交底书分析已暂停")
	assertContains(t, got, "/approve")
	assertContains(t, got, "/reject")
	assertContains(t, got, "新颖性初判")
	// Should not carry the generic "关卡：" label.
	if strings.Contains(got, "关卡：") {
		t.Errorf("disclosure_review prompt should not show generic 关卡 label: %s", got)
	}
}

func TestInterruptGuidance_OtherGate(t *testing.T) {
	got := interruptGuidance(AgentInterruptChatEvent{
		Reason: "需要人工确认",
		Data:   map[string]any{"gate": "custom_gate"},
	})

	assertContains(t, got, "已暂停等待人工确认")
	assertContains(t, got, "关卡：custom_gate")
	assertContains(t, got, "/approve")
	assertContains(t, got, "/reject")
}

func TestInterruptGuidance_NoGate(t *testing.T) {
	// ApprovalGate keyword soft-interrupt: no gate tag in Data.
	got := interruptGuidance(AgentInterruptChatEvent{
		Reason: "需要人工审核: 专利结论",
		Data:   nil,
	})

	assertContains(t, got, "已暂停等待人工确认")
	assertContains(t, got, "/approve")
	assertContains(t, got, "/reject")
	if strings.Contains(got, "关卡：") {
		t.Errorf("no-gate prompt should not show 关卡 label: %s", got)
	}
}

func TestInterruptGuidance_EmptyReason(t *testing.T) {
	// Empty reason should fall back to "已暂停" without panicking.
	got := interruptGuidance(AgentInterruptChatEvent{Reason: "", Data: nil})
	if !strings.Contains(got, "已暂停") {
		t.Errorf("empty reason should fall back to 已暂停: %s", got)
	}
}

func TestAgentInterruptChatEvent_Kind(t *testing.T) {
	if got := (AgentInterruptChatEvent{}).ChatEventKind(); got != ChatEventAgentInterrupt {
		t.Errorf("ChatEventKind = %q, want %q", got, ChatEventAgentInterrupt)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected guidance to contain %q, got:\n%s", substr, s)
	}
}
