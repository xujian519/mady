package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestOnAgentInterrupt verifies the interrupt handler: FSM transition,
// stream finalization, judgment summary, and guidance prompt.
func TestOnAgentInterrupt(t *testing.T) {
	t.Run("disclosure review gate", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onAgentStart(AgentStartChatEvent{})
		app.onMessageDelta(MessageDeltaChatEvent{Delta: "partial"})

		app.onAgentInterrupt(AgentInterruptChatEvent{
			Reason: "等待复核",
			Data:   map[string]any{"gate": "disclosure_review"},
		})

		msgs := app.History().Messages()
		last := msgs[len(msgs)-1]
		if !strings.Contains(last.Text, "/approve") || !strings.Contains(last.Text, "/reject") {
			t.Fatalf("interrupt guidance should mention /approve and /reject, got %q", last.Text)
		}
		if !strings.Contains(last.Text, "技术交底书分析已暂停") {
			t.Fatalf("disclosure guidance missing, got %q", last.Text)
		}
		// Stream must be finalized: no pending messages.
		for _, m := range msgs {
			if m.Pending {
				t.Fatalf("stream should be finalized, found pending msg %+v", m)
			}
		}
		app.mu.Lock()
		js := app.model.judgmentSummary
		app.mu.Unlock()
		if js.Phase == "" || js.Judgment != "等待复核" {
			t.Fatalf("judgment summary not set: %+v", js)
		}
		if app.State() != StateInterrupted {
			t.Fatalf("state = %s, want interrupted", app.State())
		}
	})

	t.Run("generic gate and empty reason", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onAgentInterrupt(AgentInterruptChatEvent{Reason: "", Data: map[string]any{"gate": "approval_gate"}})
		msgs := app.History().Messages()
		last := msgs[len(msgs)-1]
		if !strings.Contains(last.Text, "关卡：approval_gate") {
			t.Fatalf("gate tag should appear, got %q", last.Text)
		}
		if !strings.Contains(last.Text, "已暂停") {
			t.Fatalf("empty reason should fall back, got %q", last.Text)
		}
	})

	t.Run("wrong event type is ignored", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		before := len(app.History().Messages())
		app.onAgentInterrupt(AgentStartChatEvent{})
		if len(app.History().Messages()) != before {
			t.Fatal("wrong event type must be a no-op")
		}
	})
}

// TestOnEditorSubmitQueuesDuringStreaming verifies that input typed while the
// agent streams is queued and flushed on agent end.
func TestOnEditorSubmitQueuesDuringStreaming(t *testing.T) {
	var submitted []string
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(_ context.Context, in string) { submitted = append(submitted, in) },
	})
	app.MarkAgentReady()

	app.onAgentStart(AgentStartChatEvent{})
	app.onEditorSubmit("first queued")
	app.onEditorSubmit("second queued")
	if app.QueuedInputCount() != 2 {
		t.Fatalf("QueuedInputCount = %d, want 2", app.QueuedInputCount())
	}
	// While streaming, nothing reaches OnSubmit.
	if len(submitted) != 0 {
		t.Fatalf("no submit while streaming, got %v", submitted)
	}
	// User echo is NOT printed while queued.
	if n := len(app.History().Messages()); n != 0 {
		t.Fatalf("queued inputs must not echo, got %d messages", n)
	}

	app.onAgentEnd(AgentEndChatEvent{})
	if len(submitted) != 2 || submitted[0] != "first queued" || submitted[1] != "second queued" {
		t.Fatalf("flushed submits = %v", submitted)
	}
	if app.QueuedInputCount() != 0 {
		t.Fatal("queue should be empty after flush")
	}
	// Flushed inputs echo as user messages.
	msgs := app.History().Messages()
	if len(msgs) != 2 || msgs[0].Role != RoleUser || msgs[1].Role != RoleUser {
		t.Fatalf("expected 2 user echoes, got %+v", msgs)
	}
}

// TestOnEditorSubmitErrorFlush verifies flush also happens on agent error.
func TestOnEditorSubmitErrorFlush(t *testing.T) {
	var submitted []string
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(_ context.Context, in string) { submitted = append(submitted, in) },
	})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{})
	app.onEditorSubmit("queued")

	app.onAgentError(AgentErrorChatEvent{Err: errors.New("boom")})
	if len(submitted) != 1 || submitted[0] != "queued" {
		t.Fatalf("error should flush queue, got %v", submitted)
	}
}

// TestOnEditorSubmitEmptyAndWhitespace verifies empty/whitespace input is
// dropped and OnSubmit is not invoked.
func TestOnEditorSubmitEmptyAndWhitespace(t *testing.T) {
	calls := 0
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(_ context.Context, _ string) { calls++ },
	})
	app.MarkAgentReady()
	app.onEditorSubmit("")
	app.onEditorSubmit("   ")
	app.onEditorSubmit("\n\t")
	if calls != 0 {
		t.Fatalf("empty submits must not fire OnSubmit, got %d calls", calls)
	}
}

// TestOnEditorSubmitContextNil verifies context.Background() is used when
// cfg.Context is nil.
func TestOnEditorSubmitContextNil(t *testing.T) {
	var gotCtx context.Context
	app, _ := newTestChatApp(t, ChatAppConfig{
		OnSubmit: func(ctx context.Context, _ string) { gotCtx = ctx },
	})
	app.MarkAgentReady()
	app.onEditorSubmit("hi")
	if gotCtx == nil {
		t.Fatal("OnSubmit should receive a non-nil context")
	}
}

// TestOnTurnStartEnd verifies turn bookkeeping: dividers, usage accounting,
// tok/s, and the status bar context bar.
func TestOnTurnStartEnd(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{ShowTurns: true, ContextWindow: 1000})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{})

	app.onTurnStart(TurnStartChatEvent{Turn: 2})
	msgs := app.History().Messages()
	found := false
	for _, m := range msgs {
		if m.Role == RoleDivider && strings.Contains(m.Text, "turn 2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected turn divider, got %+v", msgs)
	}

	app.onTurnEnd(TurnEndChatEvent{
		Turn:  2,
		Usage: TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 700},
	})
	app.mu.Lock()
	prompt, completion := app.model.usagePrompt, app.model.usageCompletion
	app.mu.Unlock()
	if prompt != 100 || completion != 50 {
		t.Fatalf("usage accounting = %d/%d, want 100/50", prompt, completion)
	}
	rendered := strings.Join(app.StatusBar().Render(100), "\n") // context bar needs width >= 100
	if !strings.Contains(rendered, "70%") {
		t.Fatalf("status bar should show 70%% context occupancy (700/1000), got %q", rendered)
	}
}

// TestOnTurnStartWithoutShowTurns verifies no divider without ShowTurns.
func TestOnTurnStartWithoutShowTurns(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{})
	app.onTurnStart(TurnStartChatEvent{Turn: 2})
	for _, m := range app.History().Messages() {
		if m.Role == RoleDivider {
			t.Fatalf("no divider expected without ShowTurns, got %+v", m)
		}
	}
}

// TestOnTurnEndNonTurnEventCollapses verifies onTurnEnd with a non-turn event
// still collapses consecutive tools.
func TestOnTurnEndNonTurnEventCollapses(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	hist := app.History()
	hist.Append(ChatMessage{Role: RoleAssistant, Text: "answer"})
	hist.Append(ChatMessage{Role: RoleTool, Meta: "t1", Text: "..."})
	hist.Append(ChatMessage{Role: RoleTool, Meta: "t2", Text: "..."})

	app.onTurnEnd(AgentStartChatEvent{}) // wrong type → collapse path
	cols, _ := app.TerminalSize()
	app.layout.Render(cols)
	joined := strings.Join(hist.cachedAll, "\n")
	if !strings.Contains(joined, "[+]") {
		t.Fatalf("expected collapsed tool group after turn end, got %q", joined)
	}
}

// TestOnHandoffEvents verifies handoff start/end rendering and invisible mode.
func TestOnHandoffEvents(t *testing.T) {
	t.Run("visible start and end", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{ShowTimings: true})
		app.MarkAgentReady()
		app.onAgentStart(AgentStartChatEvent{})
		app.onMessageDelta(MessageDeltaChatEvent{Delta: "x"})

		app.onHandoffStart(HandoffStartChatEvent{TargetAgent: "patent-agent"})
		app.onHandoffEnd(HandoffEndChatEvent{TargetAgent: "patent-agent", Duration: 30 * time.Millisecond})

		msgs := app.History().Messages()
		if len(msgs) < 2 {
			t.Fatalf("expected handoff messages, got %+v", msgs)
		}
		if !strings.Contains(msgs[len(msgs)-2].Text, "已切换至 patent-agent") {
			t.Errorf("handoff start message = %q", msgs[len(msgs)-2].Text)
		}
		if !strings.Contains(msgs[len(msgs)-1].Text, "patent-agent 已完成") {
			t.Errorf("handoff end message = %q", msgs[len(msgs)-1].Text)
		}
	})

	t.Run("invisible start and end", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		before := len(app.History().Messages())
		app.onHandoffStart(HandoffStartChatEvent{TargetAgent: "x", Invisible: true})
		app.onHandoffEnd(HandoffEndChatEvent{TargetAgent: "x", Invisible: true})
		if len(app.History().Messages()) != before {
			t.Fatal("invisible handoff must not append messages")
		}
	})

	t.Run("failed handoff", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onHandoffEnd(HandoffEndChatEvent{TargetAgent: "legal-agent", Err: errors.New("transfer failed")})
		msgs := app.History().Messages()
		if len(msgs) != 1 || msgs[0].Role != RoleError || !strings.Contains(msgs[0].Text, "交接失败") {
			t.Fatalf("expected error message, got %+v", msgs)
		}
	})

	t.Run("wrong event types ignored", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		before := len(app.History().Messages())
		app.onHandoffStart(AgentStartChatEvent{})
		app.onHandoffEnd(AgentStartChatEvent{})
		if len(app.History().Messages()) != before {
			t.Fatal("wrong event types must be no-ops")
		}
	})
}

// TestOnAutoRetry verifies retry messages and suppression.
func TestOnAutoRetry(t *testing.T) {
	t.Run("shown by default", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onAutoRetry(AutoRetryChatEvent{Attempt: 2, MaxRetries: 3, Delay: 5 * time.Second})
		msgs := app.History().Messages()
		if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "retry 2/3") {
			t.Fatalf("expected retry message, got %+v", msgs)
		}
	})

	t.Run("suppressed", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.SuppressAutoRetry = true
		app.onAutoRetry(AutoRetryChatEvent{Attempt: 1, MaxRetries: 3})
		if n := len(app.History().Messages()); n != 0 {
			t.Fatalf("suppressed retry must not append, got %d", n)
		}
	})

	t.Run("wrong type ignored", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.onAutoRetry(AgentStartChatEvent{})
		if n := len(app.History().Messages()); n != 0 {
			t.Fatalf("wrong event type must be a no-op, got %d", n)
		}
	})
}

// TestSuppressHandoffToolDisplay verifies transfer_to_* tools are hidden in
// integrated mode.
func TestSuppressHandoffToolDisplay(t *testing.T) {
	t.Run("suppressed", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{SuppressHandoffToolDisplay: true})
		app.MarkAgentReady()
		app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "h1", Name: "transfer_to_patent"}})
		app.onToolEnd(ToolCallEndChatEvent{ToolCallID: "h1", ToolName: "transfer_to_patent"})
		if n := len(app.History().Messages()); n != 0 {
			t.Fatalf("suppressed handoff tool must not render, got %d messages", n)
		}
		app.mu.Lock()
		active := len(app.model.activeTools)
		app.mu.Unlock()
		if active != 0 {
			t.Fatalf("suppressed tool should not track ActiveTools, got %d", active)
		}
	})

	t.Run("not suppressed", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "h1", Name: "transfer_to_legal"}})
		msgs := app.History().Messages()
		if len(msgs) != 1 || msgs[0].Meta != "transfer_to_legal" {
			t.Fatalf("expected visible tool message, got %+v", msgs)
		}
	})
}

// TestExtractToolDiffErrors covers the unmarshal-error and empty-result paths.
func TestExtractToolDiffErrors(t *testing.T) {
	path, diff, added, removed, content := extractToolDiff("edit", "not-json{{")
	if path != "" || diff != "" || added != 0 || removed != 0 || content != "" {
		t.Fatalf("unmarshal error should return zeros, got %q %q %d %d %q", path, diff, added, removed, content)
	}

	// edit_block falls back from Patch to Diff.
	_, diff, _, _, _ = extractToolDiff("edit_block", `{"path":"a.go","patch":"","diff":"--- a\n+++ b"}`)
	if diff == "" {
		t.Fatal("edit_block should fall back to diff when patch empty")
	}

	// apply_patch counts +/- lines when new/old_lines are missing.
	_, diff, added, removed, _ = extractToolDiff("apply_patch", `{"patch":"@@ -1 +1 @@\n-old line\n+new line\n+another"}`)
	if diff == "" {
		t.Fatal("apply_patch should extract patch")
	}
	if added != 2 || removed != 1 {
		t.Fatalf("apply_patch line counting = +%d/-%d, want +2/-1", added, removed)
	}

	// write_file derives added from content line count.
	_, _, added, _, content = extractToolDiff("write", `{"path":"b.go","content":"line1\nline2"}`)
	if added != 2 || content != "line1\nline2" {
		t.Fatalf("write content lines = %d, want 2", added)
	}

	// Unknown tool names: path/content pass through, diff stays empty (it is
	// only populated inside the switch cases).
	path, diff, _, _, _ = extractToolDiff("other", `{"path":"x","diff":"d"}`)
	if path != "x" || diff != "" {
		t.Fatalf("unknown tool: path=%q diff=%q, want path x / empty diff", path, diff)
	}
}

// TestFormatContentPreview verifies preview truncation to 6 lines.
func TestFormatContentPreview(t *testing.T) {
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	preview := formatContentPreview(strings.Join(lines, "\n"), 10)
	got := strings.Count(preview, "\n")
	if got < 5 || got > 7 {
		t.Fatalf("preview line count = %d, want ~6", got)
	}
	if !strings.Contains(preview, "+4 lines") {
		t.Fatalf("preview should show overflow count, got %q", preview)
	}

	short := formatContentPreview("one", 1)
	if !strings.Contains(short, "one") || strings.Contains(short, "+") {
		t.Fatalf("short preview unexpected: %q", short)
	}
}

// TestOnToolEndDiffToolPaths verifies the write_file content-preview branch
// and the non-editor-tool skip.
func TestOnToolEndDiffToolPaths(t *testing.T) {
	t.Run("write_file shows content preview", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "w1", Name: "write_file"}})
		app.onToolEnd(ToolCallEndChatEvent{
			ToolCallID: "w1", ToolName: "write_file",
			Result: `{"path":"foo.go","content":"package main\nfunc main() {}"}`,
		})
		msgs := app.History().Messages()
		found := false
		for _, m := range msgs {
			if m.Meta == "diff" && strings.Contains(m.Text, "Wrote 2 lines to foo.go") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected write summary, got %+v", msgs)
		}
	})

	t.Run("non-editor tool no diff", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "s1", Name: "search"}})
		app.onToolEnd(ToolCallEndChatEvent{ToolCallID: "s1", ToolName: "search", Result: `{"hits":1}`})
		for _, m := range app.History().Messages() {
			if m.Meta == "diff" {
				t.Fatalf("search tool must not produce diff, got %+v", m)
			}
		}
	})

	t.Run("error result skips diff", func(t *testing.T) {
		app, _ := newTestChatApp(t, ChatAppConfig{})
		app.MarkAgentReady()
		app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "e1", Name: "edit"}})
		app.onToolEnd(ToolCallEndChatEvent{ToolCallID: "e1", ToolName: "edit", Err: errors.New("failed")})
		for _, m := range app.History().Messages() {
			if m.Meta == "diff" {
				t.Fatalf("failed tool must not produce diff, got %+v", m)
			}
		}
	})
}

// TestOnToolEndLongErrorTruncates verifies >400-width error messages are cut.
func TestOnToolEndLongErrorTruncates(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	longErr := strings.Repeat("x", 500)
	app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "l1", Name: "search"}})
	app.onToolEnd(ToolCallEndChatEvent{ToolCallID: "l1", ToolName: "search", Err: errors.New(longErr)})
	msgs := app.History().Messages()
	if !strings.Contains(msgs[0].Text, "failed") {
		t.Fatalf("expected failed status, got %q", msgs[0].Text)
	}
	if strings.Contains(msgs[0].Text, longErr) {
		t.Fatal("long error should be truncated")
	}
}

// TestOnToolStartResetSeq verifies tool sequence counter increments and the
// seq number lands on the tool message.
func TestOnToolStartResetSeq(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{}) // resets toolSeq to 0
	app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "a", Name: "t1"}})
	app.onToolStart(ToolCallStartChatEvent{ToolCall: ToolCallInfo{ID: "b", Name: "t2"}})
	msgs := app.History().Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 tool messages, got %d", len(msgs))
	}
	if msgs[0].Seq != 1 || msgs[1].Seq != 2 {
		t.Fatalf("tool seqs = %d/%d, want 1/2", msgs[0].Seq, msgs[1].Seq)
	}
}

// TestOnCompactionStartBusy verifies compaction start spins the loader and
// transitions the FSM (the end event appends the summary message).
func TestOnCompactionStartBusy(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{})
	app.onCompactionStart(CompactionStartChatEvent{TokensBefore: 12345})
	if !app.loader.IsRunning() {
		t.Fatal("loader should be running during compaction")
	}
	if app.State() != StateCompacting {
		t.Fatalf("state = %s, want compacting", app.State())
	}
	// The end event appends the compacted summary.
	app.onCompactionEnd(CompactionEndChatEvent{TokensBefore: 12345, TokensAfter: 6000, MessagesCut: 4})
	msgs := app.History().Messages()
	last := msgs[len(msgs)-1]
	if !strings.Contains(last.Text, "compacted 12345") || !strings.Contains(last.Text, "6000") {
		t.Fatalf("compaction summary = %q", last.Text)
	}
	if app.State() != StateStreaming {
		t.Fatalf("state = %s, want streaming after compaction end", app.State())
	}
}

// TestOnAgentErrorAndEndIdleState verifies error/end return to idle and clear
// the loader.
func TestOnAgentErrorAndEndIdleState(t *testing.T) {
	app, _ := newTestChatApp(t, ChatAppConfig{})
	app.MarkAgentReady()
	app.onAgentStart(AgentStartChatEvent{})
	app.onAgentError(AgentErrorChatEvent{Err: errors.New("e")})
	if app.loader.IsRunning() {
		t.Fatal("loader should stop after agent error")
	}
	if app.State() != StateIdle {
		t.Fatalf("state after error = %s, want idle", app.State())
	}

	app.onAgentStart(AgentStartChatEvent{})
	app.onAgentEnd(AgentEndChatEvent{})
	if app.loader.IsRunning() {
		t.Fatal("loader should stop after agent end")
	}
}
