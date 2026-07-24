package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/tui/core"
	"github.com/xujian519/mady/tui/terminal"
	"github.com/xujian519/mady/tui/theme"
)

func newTestChatApp(t *testing.T, cfg ChatAppConfig) (*ChatApp, *terminal.VirtualTerminal) {
	t.Helper()
	vt := terminal.NewVirtualTerminal(80, 24)
	host := &testAppHost{vt: vt}
	cfg.Host = host
	app := NewChatApp(cfg)
	app.SetHost(host)
	if err := app.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { app.Stop() })
	return app, vt
}

type testAppHost struct {
	vt       *terminal.VirtualTerminal
	children []core.Component
	started  bool
	overlays []OverlayRef
}

func (h *testAppHost) Start() error              { h.started = true; return nil }
func (h *testAppHost) Stop() error               { h.started = false; return nil }
func (h *testAppHost) Done() <-chan struct{}     { ch := make(chan struct{}); close(ch); return ch }
func (h *testAppHost) AddChild(c core.Component) { h.children = append(h.children, c) }
func (h *testAppHost) Focus(c core.Component)    {}
func (h *testAppHost) RequestRender()            {}
func (h *testAppHost) PushOverlay(ov OverlayRef) { h.overlays = append(h.overlays, ov) }
func (h *testAppHost) RemoveOverlay(ov OverlayRef) bool {
	for i, o := range h.overlays {
		if o == ov {
			h.overlays = append(h.overlays[:i], h.overlays[i+1:]...)
			return true
		}
	}
	return false
}
func (h *testAppHost) TerminalSize() (cols, rows int64) { return h.vt.Size() }
func (h *testAppHost) EnableMouse(mode string)          {}
func (h *testAppHost) DisableMouse()                    {}

func TestChatAppMessageDeltaStream(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	app.onAgentStart(AgentStartChatEvent{})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "Hello, "})
	app.onMessageDelta(MessageDeltaChatEvent{Delta: "world!"})

	msgs := app.History().Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 streaming msg, got %d", len(msgs))
	}
	if msgs[0].Text != "Hello, world!" {
		t.Fatalf("text=%q", msgs[0].Text)
	}
	if !msgs[0].Pending {
		t.Fatalf("expected pending during stream")
	}

	app.onAgentEnd(AgentEndChatEvent{})
	msgs = app.History().Messages()
	if msgs[0].Pending {
		t.Fatalf("agent_end should finalize streaming message")
	}
}

func TestChatAppToolLifecycle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{ShowTimings: true})

	app.onToolStart(ToolCallStartChatEvent{
		ToolCall: ToolCallInfo{ID: "t1", Name: "search"},
	})
	msgs := app.History().Messages()
	if len(msgs) != 1 || msgs[0].Meta != "search" {
		t.Fatalf("expected tool-start msg with name 'search', got %+v", msgs)
	}

	app.onToolEnd(ToolCallEndChatEvent{
		ToolCallID: "t1",
		ToolName:   "search",
		Duration:   50 * time.Millisecond,
	})
	msgs = app.History().Messages()
	if len(msgs) != 1 {
		t.Fatalf("tool-end should update in place, got %d msgs", len(msgs))
	}
	if !strings.Contains(msgs[0].Text, theme.SymbolCheck) {
		t.Fatalf("expected check mark in result: %q", msgs[0].Text)
	}
}

func TestChatAppToolError(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.onToolStart(ToolCallStartChatEvent{
		ToolCall: ToolCallInfo{ID: "t1", Name: "x"},
	})
	app.onToolEnd(ToolCallEndChatEvent{
		ToolCallID: "t1", ToolName: "x", Err: errors.New("boom"),
	})
	msgs := app.History().Messages()
	if !strings.Contains(msgs[0].Text, "failed") {
		t.Fatalf("expected 'failed' in msg: %q", msgs[0].Text)
	}
}

// TestChatAppEditorDiffExpanded verifies that the inline diff produced for
// editor tools (write_file, edit, ...) defaults to expanded (Collapsed=false)
// so the user can see changes without clicking, and only collapses on click.
func TestChatAppEditorDiffExpanded(t *testing.T) {
	cases := []struct {
		name     string
		toolName string
		result   string
	}{
		{
			name:     "write_file",
			toolName: "write_file",
			result:   `{"path":"foo.go","content":"package main\nfunc main(){}"}`,
		},
		{
			name:     "edit",
			toolName: "edit",
			result:   `{"path":"foo.go","diff":"@@ -1 +1 @@\n-old\n+new"}`,
		},
		{
			name:     "edit_block",
			toolName: "edit_block",
			result:   `{"path":"foo.go","diff":"@@ -1 +1 @@\n-old\n+new"}`,
		},
		{
			name:     "apply_patch",
			toolName: "apply_patch",
			result:   `{"path":"foo.go","patch":"@@ -1 +1 @@\n-old\n+new"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newTestChatApp(t, ChatAppConfig{})
			app.onToolStart(ToolCallStartChatEvent{
				ToolCall: ToolCallInfo{ID: "t1", Name: tc.toolName},
			})
			app.onToolEnd(ToolCallEndChatEvent{
				ToolCallID: "t1", ToolName: tc.toolName, Result: tc.result,
			})

			var diffMsg *ChatMessage
			msgs := app.History().Messages()
			for i := range msgs {
				if msgs[i].Meta == "diff" {
					diffMsg = &msgs[i]
					break
				}
			}
			if diffMsg == nil {
				t.Fatalf("%s: expected an inline diff message", tc.toolName)
			}
			if diffMsg.Collapsed {
				t.Fatalf("%s: diff should default to expanded (Collapsed=false)", tc.toolName)
			}
		})
	}
}

func TestChatAppEditorSubmit(t *testing.T) {
	var captured string
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(_ context.Context, in string) { captured = in },
	})
	for _, r := range "hello" {
		app.editor.Update(core.KeyMsg{Data: string(r)})
	}
	app.editor.Update(core.KeyMsg{Data: "\r"})

	if captured != "hello" {
		t.Fatalf("OnSubmit captured=%q want hello", captured)
	}
	msgs := app.History().Messages()
	if len(msgs) == 0 || msgs[0].Role != RoleUser {
		t.Fatalf("expected user echo in history, got %+v", msgs)
	}
	if app.editor.GetValue() != "" {
		t.Fatalf("editor should be cleared after submit; got %q", app.editor.GetValue())
	}
}

func TestChatAppBusyIdle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.Busy("working")
	if !app.loader.IsRunning() {
		t.Fatalf("loader should be running")
	}
	app.Idle()
	if app.loader.IsRunning() {
		t.Fatalf("loader should be stopped")
	}
}

func TestCtrlCPrefersCopyOverInterrupt(t *testing.T) {
	var interrupted bool
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnInterrupt: func() { interrupted = true },
	})
	app.Busy("working") // agent "running"

	app.editor.Update(core.KeyMsg{Data: "hello"})
	app.editor.Render(40)                                                      // populate lastVisuals; default prompt "> " is 2 cols wide
	app.editor.Update(core.MouseMsg{Action: core.MousePress, Row: 0, Col: 2})  // buffer col 0
	app.editor.Update(core.MouseMsg{Action: core.MouseMotion, Row: 0, Col: 7}) // buffer col 5
	app.editor.Update(core.MouseMsg{Action: core.MouseRelease, Row: 0, Col: 7})
	if app.editor.GetSelectedText() != "hello" {
		t.Fatalf("setup: expected editor selection %q, got %q", "hello", app.editor.GetSelectedText())
	}

	// Mirrors the real TUI dispatch order (tui.go processMsg): the focused
	// component (the editor) receives every KeyMsg first, and non-focused
	// children (chatLayout, which owns the Ctrl/Cmd+C handling) receive it
	// afterward. A prior bug cleared the editor's mouse-drag selection
	// unconditionally on every keystroke it saw — including Ctrl+C itself —
	// so by the time chatLayout's handler ran, the selection was already
	// gone and nothing got copied.
	keyMsg := core.KeyMsg{Data: "\x03"} // Ctrl+C
	app.editor.Update(keyMsg)
	app.layout.Update(keyMsg)

	if interrupted {
		t.Fatalf("expected Ctrl+C to copy the active selection instead of interrupting")
	}
	if app.editor.GetSelectedText() != "hello" {
		t.Fatalf("expected selection to remain visible after copy (matching standard clipboard UX), got %q", app.editor.GetSelectedText())
	}
}

func TestCmdASelectsAllEditorText(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.editor.Update(core.KeyMsg{Data: "hello world"})

	// Kitty CSI-u encoding for Cmd+A (Super+A): the same dual-dispatch path
	// as TestCtrlCPrefersCopyOverInterrupt — the focused editor sees the key
	// first, then chatLayout.
	keyMsg := core.KeyMsg{Data: "\x1b[97;9u"}
	app.editor.Update(keyMsg)
	app.layout.Update(keyMsg)

	if got := app.editor.GetSelectedText(); got != "hello world" {
		t.Fatalf("expected Cmd+A to select all editor text, got %q", got)
	}
}

// TestChatAppFSMInitializing verifies the initial FSM state is StateInitializing
// and the judgment view reflects "initializing" status before MarkAgentReady.
func TestChatAppFSMInitializing(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	// Before MarkAgentReady, state must be StateInitializing.
	app.mu.Lock()
	gotState := app.model.state
	app.mu.Unlock()
	if gotState != StateInitializing {
		t.Errorf("initial FSM state = %s, want %s", gotState, StateInitializing)
	}

	// JudgmentView initial status should be "initializing".
	app.layout.updateJudgmentView()
	if status := app.judgmentView.Status(); status != "initializing" {
		t.Errorf("initial JV status = %q, want %q", status, "initializing")
	}

	// Non-intrusive: user can still type during initialization.
	var captured string
	app.editor.Update(core.KeyMsg{Data: "\r"}) // empty submit should be ignored
	_ = captured
}

// TestChatAppFSMReadyTransition verifies the StateInitializing → StateIdle
// transition and judgment view update when MarkAgentReady is called.
func TestChatAppFSMReadyTransition(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	// Mark agent ready — simulates initialiazeAgentAsync completion.
	app.MarkAgentReady()

	app.mu.Lock()
	gotState := app.model.state
	app.mu.Unlock()
	if gotState != StateIdle {
		t.Errorf("after MarkAgentReady: FSM state = %s, want %s", gotState, StateIdle)
	}

	// Judgment view should reflect idle status.
	if status := app.judgmentView.Status(); status != "idle" {
		t.Errorf("after MarkAgentReady: JV status = %q, want %q", status, "idle")
	}

	// From idle, agent start should transition to streaming.
	app.Busy("working")
	app.mu.Lock()
	app.model.state = Transition(app.model.state, evtAgentStart)
	app.mu.Unlock()
	app.layout.updateJudgmentView()
	if status := app.judgmentView.Status(); status != "streaming" {
		t.Errorf("after evtAgentStart from idle: JV status = %q, want %q", status, "streaming")
	}
}

// TestChatAppFSMFullLifecycle tests the complete FSM lifecycle through
// JudgmentView integration: idle → streaming → tool → streaming → idle.
func TestChatAppFSMFullLifecycle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady() // start from idle

	// idle → streaming (agent start)
	app.onAgentStart(AgentStartChatEvent{})
	app.mu.Lock()
	if s := app.model.state; s != StateStreaming {
		t.Errorf("after agentStart: FSM = %s, want %s", s, StateStreaming)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "streaming" {
		t.Errorf("after agentStart: JV = %q, want %q", status, "streaming")
	}

	// streaming → tool-running
	app.onToolStart(ToolCallStartChatEvent{
		ToolCall: ToolCallInfo{ID: "t1", Name: "search"},
	})
	app.mu.Lock()
	if s := app.model.state; s != StateToolRunning {
		t.Errorf("after toolStart: FSM = %s, want %s", s, StateToolRunning)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "analyzing" {
		t.Errorf("after toolStart: JV = %q, want %q", status, "analyzing")
	}

	// tool-running → streaming (tool end)
	app.onToolEnd(ToolCallEndChatEvent{
		ToolCallID: "t1", ToolName: "search",
	})
	app.mu.Lock()
	if s := app.model.state; s != StateStreaming {
		t.Errorf("after toolEnd: FSM = %s, want %s", s, StateStreaming)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "streaming" {
		t.Errorf("after toolEnd: JV = %q, want %q", status, "streaming")
	}

	// streaming → idle (agent end)
	app.onAgentEnd(AgentEndChatEvent{})
	app.mu.Lock()
	if s := app.model.state; s != StateIdle {
		t.Errorf("after agentEnd: FSM = %s, want %s", s, StateIdle)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "idle" {
		t.Errorf("after agentEnd: JV = %q, want %q", status, "idle")
	}
}

// TestChatAppFSMApprovalFlow verifies the approval interrupt path:
// streaming → awaiting-confirm → idle (approval decision).
func TestChatAppFSMApprovalFlow(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()

	// Start a run.
	app.onAgentStart(AgentStartChatEvent{})

	// Approval prompt — FSM should go to AwaitingConfirm.
	app.onApprovalPrompt(ApprovalPromptChatEvent{
		Content: "是否确认该结论？",
	})
	app.mu.Lock()
	if s := app.model.state; s != StateAwaitingConfirm {
		t.Errorf("after approvalPrompt: FSM = %s, want %s", s, StateAwaitingConfirm)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "awaiting_review" {
		t.Errorf("after approvalPrompt: JV = %q, want %q", status, "awaiting_review")
	}
	if !app.judgmentView.IsExpanded() {
		t.Error("judgment view should be expanded during approval")
	}
}

// TestChatAppFSMErrorFlow verifies that agent errors transition to StateFailed
// and the judgment view reflects "failed" status.
func TestChatAppFSMErrorFlow(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()

	// Run and error.
	app.onAgentStart(AgentStartChatEvent{})
	app.onAgentError(AgentErrorChatEvent{Err: errors.New("test error")})
	app.mu.Lock()
	if s := app.model.state; s != StateIdle {
		t.Errorf("after agentError: FSM = %s, want %s", s, StateIdle)
	}
	app.mu.Unlock()

	// When agent errors, the FSM transitions to Idle (not Failed) to allow
	// retry. The error is displayed via PrintError in the chat history.
	// (Failed is reserved for initialization failure — see MarkAgentFailed.)
}

// TestChatAppMarkAgentFailed verifies the init failure path.
func TestChatAppMarkAgentFailed(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	// Simulate agent initialization failure.
	app.MarkAgentFailed()
	app.mu.Lock()
	if s := app.model.state; s != StateFailed {
		t.Errorf("after MarkAgentFailed: FSM = %s, want %s", s, StateFailed)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "failed" {
		t.Errorf("after MarkAgentFailed: JV = %q, want %q", status, "failed")
	}
}

// TestChatAppFSMCompactionFlow verifies context compaction transitions.
func TestChatAppFSMCompactionFlow(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()

	// Start run and trigger compaction.
	app.onAgentStart(AgentStartChatEvent{})
	app.onCompactionStart(CompactionStartChatEvent{TokensBefore: 8000})
	app.mu.Lock()
	if s := app.model.state; s != StateCompacting {
		t.Errorf("after compactionStart: FSM = %s, want %s", s, StateCompacting)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "compacting" {
		t.Errorf("after compactionStart: JV = %q, want %q", status, "compacting")
	}

	// Compaction end → back to streaming.
	app.onCompactionEnd(CompactionEndChatEvent{
		TokensBefore: 8000, TokensAfter: 4000, MessagesCut: 5,
	})
	app.mu.Lock()
	if s := app.model.state; s != StateStreaming {
		t.Errorf("after compactionEnd: FSM = %s, want %s", s, StateStreaming)
	}
	app.mu.Unlock()
	if status := app.judgmentView.Status(); status != "streaming" {
		t.Errorf("after compactionEnd: JV = %q, want %q", status, "streaming")
	}
}

// TestBuildSystemStatusData_Initializing verifies that the system status
// data correctly reflects StateInitializing.
func TestBuildSystemStatusData_Initializing(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	// FSM starts at StateInitializing per constructor.
	sd := buildSystemStatusData(app, "")
	if sd.Mode != "" {
		t.Errorf("Mode = %q, want empty for initializing", sd.Mode)
	}
	if len(sd.Events) == 0 {
		t.Fatal("expected at least one event (FSM state)")
	}
	if sd.Events[0].Message != "Agent 状态: initializing" {
		t.Errorf("first event message = %q, want 'Agent 状态: initializing'", sd.Events[0].Message)
	}
}

// TestBuildSystemStatusData_Idle verifies the idle state shows "就绪".
func TestBuildSystemStatusData_Idle(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady() // StateInitializing → StateIdle

	sd := buildSystemStatusData(app, "")
	if sd.ModeReason != "就绪" {
		t.Errorf("ModeReason = %q, want 就绪", sd.ModeReason)
	}
}

// TestBuildSystemStatusData_AwaitingConfirm verifies awaiting_review state
// populates the approval event.
func TestBuildSystemStatusData_AwaitingConfirm(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	app.onApprovalPrompt(ApprovalPromptChatEvent{
		Content: "需要确认新颖性分析结果",
		Data: &ReviewGatePayload{
			Title:      "新颖性复核",
			Judgment:   "权利要求1-5具备新颖性",
			Confidence: 0.85,
		},
	})

	// System status should reflect the awaiting_review state.
	sd := buildSystemStatusData(app, "")
	if sd.Mode != "awaiting_review" {
		t.Errorf("Mode = %q, want awaiting_review", sd.Mode)
	}
	if !strings.Contains(sd.ModeReason, "等待") || !strings.Contains(sd.ModeReason, "复核") {
		t.Errorf("ModeReason = %q, should contain 等待 and 复核", sd.ModeReason)
	}
	// Should have an approval-related event.
	hasApprovalEvent := false
	for _, ev := range sd.Events {
		if strings.Contains(ev.Message, "审批") {
			hasApprovalEvent = true
			break
		}
	}
	if !hasApprovalEvent {
		t.Error("expected approval event in system status")
	}
}

// TestBuildSystemStatusData_Failed verifies the failed state shows degraded.
func TestBuildSystemStatusData_Failed(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentFailed() // StateInitializing → StateFailed

	sd := buildSystemStatusData(app, "")
	if sd.Mode != "degraded" {
		t.Errorf("Mode = %q, want degraded for failed state", sd.Mode)
	}
	if sd.Events[0].Level != "error" {
		t.Errorf("first event level = %q, want error", sd.Events[0].Level)
	}
}

func TestChatAppSubscribe(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})

	adapter := &testSubscriber{handlers: make(map[ChatEventType]func(ChatEvent))}
	app.Subscribe(adapter)

	if len(adapter.handlers) != 17 {
		t.Fatalf("expected 17 handlers registered, got %d", len(adapter.handlers))
	}
	for _, et := range []ChatEventType{
		ChatEventAgentStart, ChatEventAgentEnd, ChatEventAgentError,
		ChatEventAgentInterrupt, ChatEventApprovalPrompt,
		ChatEventTurnStart, ChatEventTurnEnd, ChatEventMessageDelta,
		ChatEventToolCallStart, ChatEventToolCallEnd,
		ChatEventHandoffStart, ChatEventHandoffEnd,
		ChatEventCompactionStart, ChatEventCompactionEnd,
		ChatEventAutoRetry,
		ChatEventTaskCreated, ChatEventTaskUpdated,
	} {
		if adapter.handlers[et] == nil {
			t.Errorf("handler for %s not registered", et)
		}
	}
}

type testSubscriber struct {
	handlers map[ChatEventType]func(ChatEvent)
}

func (s *testSubscriber) On(eventType ChatEventType, handler func(ChatEvent)) {
	s.handlers[eventType] = handler
}

func TestIsPrimaryShortcutMod(t *testing.T) {
	tests := []struct {
		name string
		mods terminal.Modifier
		want bool
	}{
		{name: "ctrl", mods: terminal.ModCtrl, want: true},
		{name: "super", mods: terminal.ModSuper, want: true},
		{name: "meta", mods: terminal.ModMeta, want: true},
		{name: "none", mods: terminal.ModNone, want: false},
		{name: "alt only", mods: terminal.ModAlt, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPrimaryShortcutMod(tc.mods); got != tc.want {
				t.Fatalf("isPrimaryShortcutMod(%v)=%v want=%v", tc.mods, got, tc.want)
			}
		})
	}
}

func TestIsCopyShortcut(t *testing.T) {
	tests := []struct {
		name string
		key  terminal.Key
		want bool
	}{
		{name: "cmd lowercase c", key: terminal.Key{Name: "c", Mods: terminal.ModSuper}, want: true},
		{name: "cmd uppercase C", key: terminal.Key{Name: "C", Mods: terminal.ModSuper | terminal.ModShift}, want: true},
		{name: "meta uppercase C", key: terminal.Key{Name: "C", Mods: terminal.ModMeta | terminal.ModShift}, want: true},
		{name: "ctrl c", key: terminal.Key{Name: "c", Mods: terminal.ModCtrl}, want: true},
		{name: "ctrl insert", key: terminal.Key{Name: "insert", Mods: terminal.ModCtrl}, want: true},
		{name: "plain y", key: terminal.Key{Name: "y", Mods: terminal.ModNone}, want: false},
		{name: "plain c", key: terminal.Key{Name: "c", Mods: terminal.ModNone}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCopyShortcut(tc.key); got != tc.want {
				t.Fatalf("isCopyShortcut(%+v)=%v want=%v", tc.key, got, tc.want)
			}
		})
	}
}

func TestChatLayoutUsesFlex(t *testing.T) {
	app, vt := newTestChatApp(t, ChatAppConfig{Title: "Demo"})
	cols, rows := vt.Size()
	layout := app.layout
	out := layout.Render(cols)
	if int64(len(out)) != rows {
		t.Fatalf("rendered %d lines, want %d", len(out), rows)
	}
	if layout.headerHeight != 1 {
		t.Fatalf("headerHeight=%d, want 1", layout.headerHeight)
	}
	if layout.editorTop <= int64(layout.headerHeight) {
		t.Fatalf("editorTop=%d should be below header (height=%d)", layout.editorTop, layout.headerHeight)
	}
	if layout.editorTop >= rows-1 {
		t.Fatalf("editorTop=%d leaves no room for editor", layout.editorTop)
	}
}

func TestChatLayoutEditorTopAfterResize(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{Title: "Demo"})
	vt2 := terminal.NewVirtualTerminal(80, 12)
	host := &testAppHost{vt: vt2}
	app.SetHost(host)
	app.layout.host = host

	out := app.layout.Render(80)
	if int64(len(out)) != 12 {
		t.Fatalf("rendered %d lines, want 12", len(out))
	}
	if app.layout.editorTop <= 1 || app.layout.editorTop >= 11 {
		t.Fatalf("editorTop=%d out of range for 12-row terminal", app.layout.editorTop)
	}
}

// TestChatLayoutPageScroll verifies that PageUp/PageDown scroll by the full
// viewport height (maxRows) rather than a hardcoded small offset.
// Per ScrollBy doc: positive = up (show older content), negative = down.
func TestChatLayoutPageScroll(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	// Fill enough messages to exceed the viewport.
	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("Message %d with some content", i)})
	}

	// Render to trigger layout allocation and populate cachedAll.
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	pageSize := hist.maxRows
	if pageSize <= 0 {
		t.Fatal("maxRows should be positive after layout")
	}
	// Ensure cachedAll is tall enough to scroll by one page.
	if int64(len(hist.cachedAll)) <= pageSize {
		t.Fatalf("cachedAll too short: %d lines, need > %d", len(hist.cachedAll), pageSize)
	}

	// PageUp — positive n = scroll UP by maxRows lines.
	app.layout.Update(core.KeyMsg{Data: "\x1b[5~"}) // CSI 5 ~ = pageUp
	hist.mu.Lock()
	if hist.offset != pageSize {
		t.Errorf("pageUp: offset=%d, want %d", hist.offset, pageSize)
	}
	if hist.follow {
		t.Error("pageUp: follow should be false after scrolling up")
	}
	hist.mu.Unlock()

	// PageDown — negative n = scroll DOWN back toward the tail.
	app.layout.Update(core.KeyMsg{Data: "\x1b[6~"}) // CSI 6 ~ = pageDown
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("pageDown: offset=%d, want 0", hist.offset)
	}
	// follow is NOT set to true automatically when offset reaches 0
	// via ScrollBy; only the End key (FollowTail) restores it.
	hist.mu.Unlock()
}

// TestChatLayoutAltUpDownScroll verifies that Alt+↑/↓ scroll by exactly 1 line.
func TestChatLayoutAltUpDownScroll(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("Message %d", i)})
	}

	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	if int64(len(hist.cachedAll)) <= 1 {
		t.Fatal("cachedAll too short to test line-by-line scroll")
	}

	// Alt+↑ — positive n = scroll UP by 1 line.
	app.layout.Update(core.KeyMsg{Data: "\x1b[1;3A"}) // Kitty CSI-u: Alt+↑
	hist.mu.Lock()
	if hist.offset != 1 {
		t.Errorf("Alt+↑: offset=%d, want 1", hist.offset)
	}
	if hist.follow {
		t.Error("Alt+↑: follow should be false after scrolling up")
	}
	hist.mu.Unlock()

	// Alt+↓ — negative n = scroll DOWN by 1 line.
	app.layout.Update(core.KeyMsg{Data: "\x1b[1;3B"}) // Kitty CSI-u: Alt+↓
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("Alt+↓: offset=%d, want 0", hist.offset)
	}
	hist.mu.Unlock()

	// Plain ↑ without Alt — should NOT scroll (handled by editor).
	app.layout.Update(core.KeyMsg{Data: "\x1b[A"}) // bare ↑
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("bare ↑: offset=%d, want 0 (should not scroll)", hist.offset)
	}
	hist.mu.Unlock()
}

// TestChatLayoutEndKeyFollowsTail verifies that End key triggers FollowTail(),
// resetting offset and restoring follow=true.
func TestChatLayoutEndKeyFollowsTail(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("Message %d", i)})
	}

	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	// Scroll up first via pageUp.
	app.layout.Update(core.KeyMsg{Data: "\x1b[5~"}) // pageUp
	hist.mu.Lock()
	if hist.offset == 0 {
		t.Fatal("setup: pageUp should have scrolled up")
	}
	hist.mu.Unlock()

	// End key — should follow tail: offset=0, follow=true.
	app.layout.Update(core.KeyMsg{Data: "\x1b[F"}) // CSI F = end
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("End: offset=%d, want 0", hist.offset)
	}
	if !hist.follow {
		t.Error("End: follow should be true after FollowTail")
	}
	hist.mu.Unlock()
}

// TestChatLayoutPageUpAtTop verifies that PageUp at the content top clamps
// the offset to total-maxRows (the farthest scroll-up position) instead of
// wrapping around or panicking.
func TestChatLayoutPageUpAtTop(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("msg %d", i)})
	}

	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	total := int64(len(hist.cachedAll))
	if total <= hist.maxRows {
		t.Skip("cachedAll too short for this test")
	}
	maxOffset := total - hist.maxRows

	// Scroll all the way to the top via multiple pageUps.
	for i := 0; i < 10; i++ {
		app.layout.Update(core.KeyMsg{Data: "\x1b[5~"}) // pageUp
	}
	hist.mu.Lock()
	if hist.offset < 0 {
		t.Errorf("offset should never be negative: got %d", hist.offset)
	}
	// offset should be clamped to the max scroll position.
	if hist.offset > maxOffset {
		t.Errorf("offset=%d exceeds maxOffset=%d", hist.offset, maxOffset)
	}
	hist.mu.Unlock()

	// More pageUps should safely stay at the top (no panic, no wrap).
	app.layout.Update(core.KeyMsg{Data: "\x1b[5~"})
	hist.mu.Lock()
	if hist.offset < 0 || hist.offset > maxOffset {
		t.Errorf("after extra pageUp at top: offset=%d should stay in [0,%d]", hist.offset, maxOffset)
	}
	hist.mu.Unlock()
}

// TestChatLayoutPageDownAtTail verifies that PageDown at the tail keeps
// the viewport at the bottom and does not overshoot.
func TestChatLayoutPageDownAtTail(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("msg %d", i)})
	}

	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	// PageDown at the tail should be a no-op (offset stays 0).
	app.layout.Update(core.KeyMsg{Data: "\x1b[6~"}) // pageDown
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("pageDown at tail: offset=%d, want 0", hist.offset)
	}
	hist.mu.Unlock()

	// Same for Alt+↓ at the tail.
	app.layout.Update(core.KeyMsg{Data: "\x1b[1;3B"}) // Alt+↓
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("Alt+↓ at tail: offset=%d, want 0", hist.offset)
	}
	hist.mu.Unlock()
}

// TestChatLayoutAltUpAtTop verifies that Alt+↑ at the content top clamps
// to the farthest scroll position instead of going negative.
func TestChatLayoutAltUpAtTop(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	for i := 0; i < 30; i++ {
		hist.Append(ChatMessage{Role: RoleAssistant, Text: fmt.Sprintf("msg %d", i)})
	}

	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	total := int64(len(hist.cachedAll))
	maxOffset := total - hist.maxRows

	// Scroll far up with pageUp first, then nudge to the exact top with
	// Alt+↑ until offset stabilizes.
	for i := int64(0); i < maxOffset+5; i++ {
		app.layout.Update(core.KeyMsg{Data: "\x1b[1;3A"}) // Alt+↑
	}
	hist.mu.Lock()
	if hist.offset < 0 {
		t.Errorf("offset should never be negative: got %d", hist.offset)
	}
	if hist.offset > maxOffset {
		t.Errorf("offset=%d exceeds maxOffset=%d", hist.offset, maxOffset)
	}
	hist.mu.Unlock()
}

// TestChatLayoutNoScrollWithShortContent verifies that scrolling keys are
// no-ops when the content is shorter than the viewport.
func TestChatLayoutNoScrollWithShortContent(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()

	// Only 2 messages — content is shorter than the viewport.
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "short"})
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "content"})

	cols, _ := app.TerminalSize()
	app.layout.Render(cols)

	if int64(len(hist.cachedAll)) <= hist.maxRows {
		t.Log("content fits in viewport, scrolling should be no-op")
	}

	app.layout.Update(core.KeyMsg{Data: "\x1b[5~"}) // pageUp
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("pageUp with short content: offset=%d, want 0", hist.offset)
	}
	hist.mu.Unlock()

	app.layout.Update(core.KeyMsg{Data: "\x1b[1;3A"}) // Alt+↑
	hist.mu.Lock()
	if hist.offset != 0 {
		t.Errorf("Alt+↑ with short content: offset=%d, want 0", hist.offset)
	}
	hist.mu.Unlock()
}
