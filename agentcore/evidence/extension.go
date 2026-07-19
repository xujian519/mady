package evidence

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the evidence extension.
const ExtensionName = "evidence"

// EvidenceExtension auto-registers a Ledger into the agent lifecycle:
//   - BeforeTurn: Reset the ledger for a fresh turn.
//   - AfterToolExecution: Record each tool call as a Receipt.
//
// The Ledger is available to other components via context.Context
// (evidence.WithLedger / evidence.FromContext).
type EvidenceExtension struct {
	ledger *Ledger
	agent  *agentcore.Agent
}

var (
	_ agentcore.Extension         = (*EvidenceExtension)(nil)
	_ agentcore.LifecycleProvider = (*EvidenceExtension)(nil)
)

// NewExtension creates an evidence extension with a fresh ledger.
func NewExtension() *EvidenceExtension {
	return &EvidenceExtension{ledger: NewLedger()}
}

// Ledger returns the extension's ledger for direct access.
func (e *EvidenceExtension) Ledger() *Ledger { return e.ledger }

// Name implements agentcore.Extension.
func (e *EvidenceExtension) Name() string { return ExtensionName }

// Init implements agentcore.Extension.
func (e *EvidenceExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose implements agentcore.Extension.
func (e *EvidenceExtension) Dispose() error { return nil }

// LifecycleHook implements agentcore.LifecycleProvider, returning a hook that
// resets the ledger per turn and records receipts after each tool execution.
func (e *EvidenceExtension) LifecycleHook() agentcore.LifecycleHook {
	return &evidenceHook{ext: e}
}

type evidenceHook struct {
	agentcore.BaseLifecycleHook
	ext *EvidenceExtension
}

func (h *evidenceHook) BeforeTurn(_ context.Context, arc *agentcore.AgentRunContext) error {
	h.ext.ledger.Reset()
	return nil
}

// 注意：此前此处有一个 BeforeModelCall 实现，注释声称"把 ledger 注入 context"，
// 但 LifecycleHook.BeforeModelCall 签名返回 error、无法修改 ctx，实际函数体仅
// `_ = h.ext.agent` 什么也没注入，是死代码。已删除。
// ledger 通过 EvidenceExtension 实例直接访问（h.ext.ledger），无需 context 注入。

func (h *evidenceHook) AfterToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) {
	if h.ext.ledger == nil || tec == nil {
		return
	}
	for i, tc := range tec.ToolCalls {
		var success bool
		var dur int64
		if i < len(tec.Results) {
			success = tec.Results[i].Err == nil
			dur = tec.Results[i].Duration.Milliseconds()
		}
		r := ReceiptFromToolCall(tc.Name, []byte(tc.Arguments), success, dur)
		h.ext.ledger.Record(r)
	}
}
