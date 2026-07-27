package evidence

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName is the registration name for the evidence extension.
const ExtensionName = "evidence"

// EvidenceExtension auto-registers a Ledger into the agent lifecycle.
//
//nolint:revive // stutter: evidence.EvidenceExtension is intentional for clarity
type EvidenceExtension struct {
	ledger *Ledger
	agent  *agentcore.Agent
}

var (
	_ agentcore.Extension        = (*EvidenceExtension)(nil)
	_ agentcore.TurnObserver     = (*EvidenceExtension)(nil)
	_ agentcore.ToolCallObserver = (*EvidenceExtension)(nil)
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

// ---------------------------------------------------------------------------
// TurnObserver implementation
// ---------------------------------------------------------------------------

// BeforeTurn resets the ledger at the start of each turn.
func (e *EvidenceExtension) BeforeTurn(_ context.Context, _ *agentcore.AgentRunContext) error {
	e.ledger.Reset()
	return nil
}

// AfterTurn is a no-op required by the TurnObserver interface.
func (e *EvidenceExtension) AfterTurn(_ context.Context, _ *agentcore.AgentRunContext, _ agentcore.TurnInfo) {
}

// 注意：此前此处有一个 BeforeModelCall 实现，注释声称"把 ledger 注入 context"，
// 但 BeforeModelCall 签名返回 error、无法修改 ctx，实际函数体仅
// `_ = h.ext.agent` 什么也没注入，是死代码。已删除。
// ledger 通过 EvidenceExtension 实例直接访问（e.ledger），无需 context 注入。

// ---------------------------------------------------------------------------
// ToolCallObserver implementation
// ---------------------------------------------------------------------------

// BeforeToolExecution is a no-op required by the ToolCallObserver interface.
func (e *EvidenceExtension) BeforeToolExecution(_ context.Context, _ *agentcore.AgentRunContext, _ *agentcore.ToolExecutionContext) error {
	return nil
}

// AfterToolExecution records each tool call as a Receipt in the ledger.
func (e *EvidenceExtension) AfterToolExecution(_ context.Context, _ *agentcore.AgentRunContext, tec *agentcore.ToolExecutionContext) {
	if e.ledger == nil || tec == nil {
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
		e.ledger.Record(r)
	}
}
