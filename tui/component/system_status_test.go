package component

import (
	"testing"

	"github.com/xujian519/mady/tui/core"
)

func TestSystemStatusRenderMode(t *testing.T) {
	ss := NewSystemStatus()
	ss.SetMode("degraded", "Provider 不支持 json_schema")

	lines := ss.Render(60)
	if len(lines) == 0 {
		t.Fatal("expected non-empty render output")
	}

	// Should contain mode and reason.
	found := false
	for _, l := range lines {
		if sysStatusContains(l, "degraded") || sysStatusContains(l, "Provider") {
			found = true
		}
	}
	if !found {
		t.Error("render should contain mode and reason, got:", lines)
	}
}

func TestSystemStatusRenderEvents(t *testing.T) {
	ss := NewSystemStatus()
	ss.SetMode("normal", "")
	ss.SetEvents([]SysEvent{
		{Time: "18:41", Message: "MCP 发现超时，已跳过", Level: "warn"},
		{Time: "18:42", Message: "判断生成成功", Level: "info"},
	})

	lines := ss.Render(60)
	eventsFound := 0
	for _, l := range lines {
		if sysStatusContains(l, "18:41") || sysStatusContains(l, "18:42") {
			eventsFound++
		}
	}
	if eventsFound == 0 {
		t.Error("render should contain event timestamps, got:", lines)
	}
}

func TestSystemStatusRenderEventsMax3(t *testing.T) {
	ss := NewSystemStatus()
	ss.SetMode("normal", "")
	ss.SetEvents([]SysEvent{
		{Time: "18:40", Message: "Event 1", Level: "info"},
		{Time: "18:41", Message: "Event 2", Level: "info"},
		{Time: "18:42", Message: "Event 3", Level: "info"},
		{Time: "18:43", Message: "Event 4 (should not appear)", Level: "info"},
	})

	lines := ss.Render(60)
	count := 0
	for _, l := range lines {
		if sysStatusContains(l, "Event") {
			count++
		}
	}
	// Event 4 should not appear.
	for _, l := range lines {
		if sysStatusContains(l, "Event 4") {
			t.Error("only 3 events should be displayed, found Event 4: ", lines)
		}
	}
}

func TestSystemStatusRenderImpacts(t *testing.T) {
	ss := NewSystemStatus()
	ss.SetMode("degraded", "Provider 降级")
	ss.SetImpacts([]string{
		"输出仍可继续",
		"结构化格式能力受限",
	})

	lines := ss.Render(60)
	impactFound := false
	for _, l := range lines {
		if sysStatusContains(l, "输出仍可继续") {
			impactFound = true
		}
	}
	if !impactFound {
		t.Error("render should contain impact items, got:", lines)
	}
}

func TestSystemStatusRenderEmptyImpacts(t *testing.T) {
	ss := NewSystemStatus()
	ss.SetMode("normal", "")
	// No impacts set.
	lines := ss.Render(60)
	for _, l := range lines {
		if sysStatusContains(l, "当前影响") {
			t.Error("should not show current impact section when empty")
		}
	}
}

func TestSystemStatusFocus(t *testing.T) {
	ss := NewSystemStatus()
	if ss.Focused() {
		t.Error("should start unfocused")
	}
	ss.SetFocused(true)
	if !ss.Focused() {
		t.Error("should be focused after SetFocused(true)")
	}
	ss.SetFocused(false)
	if ss.Focused() {
		t.Error("should be unfocused after SetFocused(false)")
	}
}

func TestSystemStatusUpdateEsc(t *testing.T) {
	ss := NewSystemStatus()
	closed := false
	ss.SetOnClose(func() { closed = true })

	ss.Update(core.KeyMsg{Data: "\x1b"})
	if !closed {
		t.Error("Esc should trigger onClose")
	}
}

func TestSystemStatusUpdateLogDetail(t *testing.T) {
	ss := NewSystemStatus()
	logCalled := false
	ss.SetOnLogDetail(func() { logCalled = true })

	ss.Update(core.KeyMsg{Data: "l"})
	if !logCalled {
		t.Error("'l' key should trigger onLogDetail")
	}
}

func TestSystemStatusUpdateEscClosesNoCrash(t *testing.T) {
	// Esc without onClose set should not crash.
	ss := NewSystemStatus()
	ss.Update(core.KeyMsg{Data: "\x1b"})
	// No assertion — must not panic.
}

func TestSystemStatusNonKeyMsgIgnored(t *testing.T) {
	ss := NewSystemStatus()
	closed := false
	ss.SetOnClose(func() { closed = true })

	// Non-key messages should be ignored.
	ss.Update(core.WindowSizeMsg{Width: 80, Height: 24})
	if closed {
		t.Error("non-key message should not trigger onClose")
	}
}

func TestSystemStatusInvalidate(t *testing.T) {
	ss := NewSystemStatus()
	ss.Render(60)
	// After Render, cache is valid; Invalidate should mark it dirty.
	ss.Invalidate()
	// Render again with different width to confirm cache miss.
	lines := ss.Render(80)
	if len(lines) == 0 {
		t.Error("render after invalidate should produce output")
	}
}

// sysStatusContains is a test helper for substring matching.
func sysStatusContains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
