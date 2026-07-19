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
