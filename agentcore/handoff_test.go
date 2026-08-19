package agentcore

import (
	"context"
	"errors"
	"testing"
)

// TestHandoff_DelegateDepthLimit verifies that executeDelegate refuses to
// run once the delegation depth reaches DefaultMaxDelegationDepth, breaking
// A→B→A→… loops before they overflow the goroutine stack.
func TestHandoff_DelegateDepthLimit(t *testing.T) {
	src := New(StubConfig(&stubProvider{}))
	defer src.Close()
	ctx := WithDepth(context.Background(), DefaultMaxDelegationDepth)
	_, err := src.executeDelegate(ctx, HandoffConfig{
		Name:        "leaf",
		AgentConfig: StubConfig(&stubProvider{}),
	}, "hi")
	if !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("expected ErrDepthExceeded, got %v", err)
	}
}

// TestHandoff_TransferDepthLimit does the same for the transfer path.
func TestHandoff_TransferDepthLimit(t *testing.T) {
	src := New(StubConfig(&stubProvider{}))
	defer src.Close()
	ctx := WithDepth(context.Background(), DefaultMaxDelegationDepth)
	_, err := src.handleTransfer(ctx, &PendingHandoff{
		TargetName:     "leaf",
		TargetConfig:   StubConfig(&stubProvider{}),
		AllowedSources: []string{"*"},
	})
	if !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("expected ErrDepthExceeded, got %v", err)
	}
}

// TestHandoff_ExecuteDelegateStructuredResult verifies that executeDelegate
// returns a parsed HandoffResult when the sub-agent emits a structured JSON
// result, and that RawOutput is preserved for tracing.
func TestHandoff_ExecuteDelegateStructuredResult(t *testing.T) {
	src := New(StubConfig(&stubProvider{}))
	defer src.Close()

	sub := &constantProvider{content: `{"action":"检索专利","result":"已检索到相关专利","success":true}`}
	out, err := src.executeDelegate(context.Background(), HandoffConfig{
		Name:        "leaf",
		AgentConfig: StubConfig(sub),
	}, "帮我检索专利")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr, ok := out.(HandoffResult)
	if !ok {
		t.Fatalf("expected HandoffResult, got %T", out)
	}
	if hr.Action != "检索专利" || hr.Result != "已检索到相关专利" || !hr.Success {
		t.Fatalf("unexpected result: %+v", hr)
	}
	if hr.RawOutput == "" {
		t.Fatalf("expected RawOutput to be set, got empty")
	}
}

// TestHandoff_ExecuteDelegatePlainTextFallback verifies that a non-JSON
// sub-agent output is wrapped as a plain-text HandoffResult.
func TestHandoff_ExecuteDelegatePlainTextFallback(t *testing.T) {
	src := New(StubConfig(&stubProvider{}))
	defer src.Close()

	sub := &constantProvider{content: "这是纯文本结果"}
	out, err := src.executeDelegate(context.Background(), HandoffConfig{
		Name:        "leaf",
		AgentConfig: StubConfig(sub),
	}, "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr, ok := out.(HandoffResult)
	if !ok {
		t.Fatalf("expected HandoffResult, got %T", out)
	}
	if hr.Action != "执行完成" || hr.Result != "这是纯文本结果" || !hr.Success {
		t.Fatalf("unexpected result: %+v", hr)
	}
}

// TestHandoff_ExecuteDelegateSubAgentFailure verifies that when the sub-agent
// fails, executeDelegate returns a graceful failure HandoffResult instead of
// a raw error, so the calling agent can degrade gracefully.
func TestHandoff_ExecuteDelegateSubAgentFailure(t *testing.T) {
	src := New(StubConfig(&stubProvider{}))
	defer src.Close()

	sub := &hangingStreamProvider{} // Complete returns an error
	out, err := src.executeDelegate(context.Background(), HandoffConfig{
		Name:        "leaf",
		AgentConfig: StubConfig(sub),
	}, "hi")
	if err != nil {
		t.Fatalf("expected nil error (graceful fallback), got %v", err)
	}
	hr, ok := out.(HandoffResult)
	if !ok {
		t.Fatalf("expected HandoffResult, got %T", out)
	}
	if hr.Success {
		t.Fatalf("expected failure result, got %+v", hr)
	}
	if hr.Action != "执行失败" || !hr.NeedsFollowup {
		t.Fatalf("unexpected result: %+v", hr)
	}
}

// TestHandoff_ExecuteDelegateCustomFallbackMsg verifies that a custom
// FallbackMsg is surfaced in the failure HandoffResult.
func TestHandoff_ExecuteDelegateCustomFallbackMsg(t *testing.T) {
	src := New(StubConfig(&stubProvider{}))
	defer src.Close()

	sub := &hangingStreamProvider{}
	out, err := src.executeDelegate(context.Background(), HandoffConfig{
		Name:        "leaf",
		AgentConfig: StubConfig(sub),
		FallbackMsg: "自定义兜底文案",
	}, "hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hr, ok := out.(HandoffResult)
	if !ok {
		t.Fatalf("expected HandoffResult, got %T", out)
	}
	if hr.Result != "自定义兜底文案" {
		t.Fatalf("expected custom fallback msg, got %q", hr.Result)
	}
}
