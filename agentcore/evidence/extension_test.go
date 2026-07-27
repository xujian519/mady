package evidence

import (
	"context"
	"errors"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// 本文件中的测试传递 nil 作为 *agentcore.AgentRunContext 参数是
// 有意的：当前 EvidenceExtension 的 Observer 实现不依赖 ARC 中的
// 任何字段（它只访问 e.ledger 和 tec），传入 nil 验证了 nil-ARC
// 保护的可靠性。如果未来需要从 ARC 读取数据，请为这些测试提供真实的 ARC。

// ---------------------------------------------------------------------------
// TurnObserver: BeforeTurn
// ---------------------------------------------------------------------------

func TestEvidence_BeforeTurn_ResetsLedger(t *testing.T) {
	ext := NewExtension()

	// Record a receipt first.
	ext.ledger.Record(Receipt{ToolName: "test_tool"})
	if ext.ledger.Len() == 0 {
		t.Fatal("expected ledger to have receipts before BeforeTurn")
	}

	// BeforeTurn should reset the ledger.
	ext.BeforeTurn(context.Background(), &agentcore.AgentRunContext{Turn: 1})
	if ext.ledger.Len() != 0 {
		t.Fatalf("expected ledger to be empty after BeforeTurn, got %d", ext.ledger.Len())
	}
}

func TestEvidence_BeforeTurn_AcceptsNilARC(t *testing.T) {
	ext := NewExtension()

	// Must not panic.
	ext.BeforeTurn(context.Background(), nil)

	// Ledger should still be empty.
	if ext.ledger.Len() != 0 {
		t.Fatalf("expected empty ledger, got %d", ext.ledger.Len())
	}
}

// ---------------------------------------------------------------------------
// ToolCallObserver: AfterToolExecution
// ---------------------------------------------------------------------------

func TestEvidence_AfterToolExecution_RecordsReceipts(t *testing.T) {
	ext := NewExtension()

	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{
			{Name: "read", Arguments: `{"path":"test.go"}`},
			{Name: "write_file", Arguments: `{"path":"out.go"}`},
		},
		Results: []agentcore.ToolResult{
			{Result: "file content", Err: nil},
			{Result: "written", Err: nil},
		},
	}

	ext.AfterToolExecution(context.Background(), nil, tec)

	if ext.ledger.Len() != 2 {
		t.Fatalf("expected 2 receipts, got %d", ext.ledger.Len())
	}

	snap := ext.ledger.Snapshot()
	if snap[0].ToolName != "read" {
		t.Errorf("expected first receipt ToolName='read', got %q", snap[0].ToolName)
	}
	if !snap[0].Success {
		t.Error("expected first receipt Success=true")
	}
	if snap[1].ToolName != "write_file" {
		t.Errorf("expected second receipt ToolName='write_file', got %q", snap[1].ToolName)
	}
	if !snap[1].Success {
		t.Error("expected second receipt Success=true")
	}
}

func TestEvidence_AfterToolExecution_RecordsFailure(t *testing.T) {
	ext := NewExtension()

	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{
			{Name: "write_file", Arguments: `{"path":"out.go"}`},
		},
		Results: []agentcore.ToolResult{
			{Result: "", Err: errors.New("boom")},
		},
	}

	ext.AfterToolExecution(context.Background(), nil, tec)

	snap := ext.ledger.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(snap))
	}
	if snap[0].Success {
		t.Error("expected receipt Success=false for failed tool")
	}
}

func TestEvidence_AfterToolExecution_NilTEC(t *testing.T) {
	ext := NewExtension()

	// Must not panic.
	ext.AfterToolExecution(context.Background(), nil, nil)
}

func TestEvidence_AfterToolExecution_NilLedger(t *testing.T) {
	ext := &EvidenceExtension{ledger: nil}

	// Must not panic.
	ext.AfterToolExecution(context.Background(), nil, &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{{Name: "test"}},
		Results:   []agentcore.ToolResult{{Result: "ok"}},
	})
}

func TestEvidence_AfterToolExecution_MismatchedLengths(t *testing.T) {
	ext := NewExtension()

	// More ToolCalls than Results — should not panic, the hook guards with
	// `i < len(tec.Results)`.
	tec := &agentcore.ToolExecutionContext{
		ToolCalls: []agentcore.ToolCall{
			{Name: "tool_a"},
			{Name: "tool_b"},
		},
		Results: []agentcore.ToolResult{
			{Result: "ok"},
		},
	}

	ext.AfterToolExecution(context.Background(), nil, tec)
	if ext.ledger.Len() != 2 {
		t.Fatalf("expected 2 receipts (missing result uses zero value), got %d", ext.ledger.Len())
	}
}
