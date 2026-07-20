package chat

// This file groups ChatApp event handlers for the assistant streaming
// lifecycle: editor submit (user input → OnSubmit), agent start/delta/end,
// and agent error. The streaming StreamID is mutated under ChatApp.mu in a
// single critical section (see onMessageDelta) to prevent concurrent deltas
// from corrupting the stream.

import (
	"context"
	"strings"
	"time"

	"github.com/xujian519/mady/tui/component"
)

func (a *ChatApp) onEditorSubmit(value string) {
	a.mu.Lock()
	// 当 autocomplete 激活时，先隐藏它然后正常提交。
	// 之前直接 return 会阻止 Enter 提交，而 chat_layout.Update 把 Enter
	// 转发给 autocomplete 的 SelectList，后者会 apply 当前选中项（带上 trigger
	// 前缀），导致用户输入被篡改（例如 /help → //help），斜杠命令失效。
	if a.ac != nil {
		a.ac.Hide()
	}
	onSubmit := a.cfg.OnSubmit
	ctx := a.cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	a.mu.Unlock()

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	// PrintUser / PushInputHistory / SetValue operate on history and editor
	// components, which own their own internal locks — they do not touch
	// ChatApp.model, so holding ChatApp.mu here is unnecessary and would
	// serialize with the event loop for no benefit.
	a.PrintUser(trimmed)
	a.host.RequestRender()
	a.editor.PushInputHistory(trimmed)
	a.editor.SetValue("")
	if onSubmit != nil {
		onSubmit(ctx, trimmed)
	}
}

func (a *ChatApp) onAgentStart(e ChatEvent) {
	if _, ok := e.(AgentStartChatEvent); !ok {
		return
	}
	a.mu.Lock()
	a.model.StreamID = ""
	// Reset per-run token accounting and mark the turn start so onTurnEnd can
	// compute tok/s. Also reset the StatusBar so stale numbers from a previous
	// run don't linger.
	a.model.usagePrompt = 0
	a.model.usageCompletion = 0
	a.model.turnStarted = time.Now()
	a.mu.Unlock()
	a.Busy("thinking...")
}

func (a *ChatApp) onMessageDelta(e ChatEvent) {
	d, ok := e.(MessageDeltaChatEvent)
	if !ok {
		return
	}
	// Read-modify-write StreamID under a single critical section. The
	// previous code released the lock between reading StreamID and writing
	// the new one, so two concurrent deltas could both read the same old id
	// and both append to the same baseline — corrupting the stream.
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.model.StreamID
	newID := a.history.AppendDelta(id, d.Delta)
	if newID != id {
		a.model.StreamID = newID
	}
}

func (a *ChatApp) onAgentError(e ChatEvent) {
	ev, ok := e.(AgentErrorChatEvent)
	if !ok {
		return
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.Idle()
	a.PrintError(ev.Err)
}

func (a *ChatApp) onAgentEnd(e ChatEvent) {
	if _, ok := e.(AgentEndChatEvent); !ok {
		return
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.Idle()
}

// onAgentInterrupt handles an agent pausing for human review. It finalizes
// any in-flight stream, returns the UI to idle (so the user can type), and
// prints a guidance prompt telling the user how to resume (/approve) or
// reject (/reject). The prompt is tailored to the interrupt source: a
// gate-tagged hard interrupt (e.g. disclosure review_gate) gets an explicit
// "review the report" hint, while a generic ApprovalGate keyword interrupt
// gets a standard prompt.
func (a *ChatApp) onAgentInterrupt(e ChatEvent) {
	ev, ok := e.(AgentInterruptChatEvent)
	if !ok {
		return
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.Idle()
	a.PrintSystem(interruptGuidance(ev))
}

// interruptGuidance builds the human-readable prompt shown when an agent
// pauses for review. It inspects the interrupt Data to tailor the wording:
//
//   - gate == "disclosure_review": the disclosure pipeline finished and the
//     novelty assessment / report awaits review — point the user at /approve.
//   - any other gate tag: a generic hard-interrupt review prompt.
//   - no gate tag (ApprovalGate keyword soft-interrupt): standard prompt.
func interruptGuidance(ev AgentInterruptChatEvent) string {
	reason := ev.Reason
	if reason == "" {
		reason = "已暂停"
	}
	gate, _ := ev.Data["gate"].(string)
	switch gate {
	case "disclosure_review":
		return strings.Join([]string{
			"⏸️  技术交底书分析已暂停，等待人工复核",
			"原因：" + reason,
			"",
			"操作方式：",
			"  • /approve — 确认新颖性初判与分析报告，继续执行",
			"  • /reject  — 拒绝，要求修改后重新分析",
		}, "\n")
	default:
		extra := ""
		if gate != "" {
			extra = "（关卡：" + gate + "）"
		}
		return strings.Join([]string{
			"⏸️  已暂停等待人工确认" + extra,
			"原因：" + reason,
			"",
			"操作方式：",
			"  • /approve — 确认并继续",
			"  • /reject  — 拒绝并要求修改",
		}, "\n")
	}
}

// onApprovalPrompt handles an ApprovalPromptEvent emitted by the ApprovalGate
// lifecycle hook. It renders an approval card (DomainMsgTypeApprovalPrompt)
// as a system message so the user sees the /approve and /reject actions.
func (a *ChatApp) onApprovalPrompt(e ChatEvent) {
	ev, ok := e.(ApprovalPromptChatEvent)
	if !ok {
		return
	}
	a.mu.Lock()
	a.finalizeStreamLocked()
	a.mu.Unlock()
	a.Idle()

	dm := &component.DomainMessage{
		Type: component.DomainMsgTypeApprovalPrompt,
		Body: ev.Content,
		Actions: []component.DomainAction{
			{Label: "确认并继续", Command: "/approve"},
			{Label: "拒绝并要求修改", Command: "/reject"},
		},
	}
	a.history.Append(ChatMessage{Role: RoleSystem, DomainMsg: dm})
}
