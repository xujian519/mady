package server

import (
	"context"
	"sync"
	"testing"

	"github.com/xujian519/mady/a2ui"
	"github.com/xujian519/mady/agentcore"
)

// TestSendActionDeliversViaPromise verifies that SendAction delivers the
// action to the agent's A2UIPromise, and that consumePendingA2UIActions
// in runPreTurn picks it up and persists it as a user message.
func TestSendActionDeliversViaPromise(t *testing.T) {
	// Create a server with a simple provider that returns after one turn.
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "test-model",
			Provider: &historyProvider{},
		},
	})
	defer srv.Close()

	// Run a chat to populate the pool with an agent entry for a thread.
	ctx := context.Background()
	var mu sync.Mutex
	var events []agentcore.Event
	_, err := srv.Chat(ctx, ChatRequest{
		Message:  "hello",
		ThreadID: "test-thread-1",
	}, func(e agentcore.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	// Get the pool entry to install an A2UIPromise.
	srv.poolMu.Lock()
	cached, ok := srv.agentPool.Load("test-thread-1")
	srv.poolMu.Unlock()
	if !ok {
		t.Fatal("agent not found in pool after Chat")
	}
	entry := cached.(*poolEntry)

	// Install a promise so we can observe delivery.
	promise := agentcore.NewA2UIPromise()
	entry.agent.SetA2UIPromise(promise)

	// SendAction — the action payload.
	err = srv.SendAction("surface_test-thread-1", a2ui.NewClientAction("approve",
		"surface_test-thread-1", "approval-card",
		map[string]any{"id": "req-1", "reason": "looks good"},
	))
	if err != nil {
		t.Fatalf("SendAction failed: %v", err)
	}

	// Verify the action reached the promise.
	action := promise.TryGet()
	if action == nil {
		t.Fatal("SendAction did not deliver the action to the promise")
	}
	if action.Name != "approve" {
		t.Fatalf("action.Name = %q, want %q", action.Name, "approve")
	}
	if action.Context["reason"] != "looks good" {
		t.Fatalf("action.Context[\"reason\"] = %v, want %v", action.Context["reason"], "looks good")
	}

	// Now run a second Chat turn on the same thread.
	// consumePendingA2UIActions in runPreTurn should pick up the action
	// and persist it as a user message. The promise should now be consumed
	// (TryGet returns nil).
	_, err = srv.Chat(ctx, ChatRequest{
		Message:  "continue",
		ThreadID: "test-thread-1",
	}, func(e agentcore.Event) {})
	if err != nil {
		t.Fatalf("second Chat failed: %v", err)
	}

	// Verify the promise was consumed during the second Chat.
	if leftover := promise.TryGet(); leftover != nil {
		t.Fatal("promise still has an unconsumed action after the second Chat — consumePendingA2UIActions may not have run")
	}
}

// TestSendAction_NoAgentReturnsError verifies that SendAction returns a
// clear error when the surfaceID doesn't match an active agent thread.
func TestSendAction_NoAgentReturnsError(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "test-model",
			Provider: &historyProvider{},
		},
	})
	defer srv.Close()

	err := srv.SendAction("surface_nonexistent-thread",
		a2ui.NewClientAction("approve", "surface_nonexistent-thread", "test", nil),
	)
	if err == nil {
		t.Fatal("SendAction should fail for a non-existent thread")
	}
}

// TestSendAction_NilActionReturnsError verifies that SendAction rejects
// a nil action.
func TestSendAction_NilActionReturnsError(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "test-model",
			Provider: &historyProvider{},
		},
	})
	defer srv.Close()

	err := srv.SendAction("surface_test", nil)
	if err == nil {
		t.Fatal("SendAction should fail for nil action")
	}
}

// TestSendAction_EmptySurfaceIDReturnsError verifies that SendAction
// rejects an empty surfaceID.
func TestSendAction_EmptySurfaceIDReturnsError(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "test-model",
			Provider: &historyProvider{},
		},
	})
	defer srv.Close()

	err := srv.SendAction("", a2ui.NewClientAction("approve", "", "test", nil))
	if err == nil {
		t.Fatal("SendAction should fail for empty surfaceID")
	}
}

// TestExtractThreadID verifies surfaceID → threadID extraction.
func TestExtractThreadID(t *testing.T) {
	tests := []struct {
		surfaceID string
		want      string
	}{
		{"surface_abc123", "abc123"},
		{"surface_thread-1", "thread-1"},
		{"nosurface_prefix", ""},
		{"surface_", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractThreadID(tt.surfaceID)
		if got != tt.want {
			t.Errorf("extractThreadID(%q) = %q, want %q", tt.surfaceID, got, tt.want)
		}
	}
}
