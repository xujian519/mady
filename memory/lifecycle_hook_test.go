package memory

import (
	"context"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// ---------------------------------------------------------------------------
// MemoryExtension.LifecycleHook
// ---------------------------------------------------------------------------

func TestMemoryExtension_LifecycleHook(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}}
	hook := ext.LifecycleHook()
	if hook == nil {
		t.Fatal("expected non-nil LifecycleHook")
	}
}

func TestMemoryExtension_LifecycleHook_ReturnsMemoryLifecycleHook(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}}
	hook := ext.LifecycleHook()
	if _, ok := hook.(*memoryLifecycleHook); !ok {
		t.Fatalf("expected *memoryLifecycleHook, got %T", hook)
	}
}

// ---------------------------------------------------------------------------
// memoryLifecycleHook.AfterModelCall — early return paths
// ---------------------------------------------------------------------------

func TestMemoryLifecycleHook_AfterModelCall_AutoExtractDisabled(t *testing.T) {
	// AutoExtract=false → hook returns without doing anything.
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: false}}
	hook := &memoryLifecycleHook{ext: ext}

	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, &agentcore.ModelCallContext{
		Request:  &agentcore.ProviderRequest{},
		Response: &agentcore.ProviderResponse{Content: "some response"},
	})
}

func TestMemoryLifecycleHook_AfterModelCall_NilManager(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}, manager: nil}
	hook := &memoryLifecycleHook{ext: ext}

	// Must not panic when manager is nil.
	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, &agentcore.ModelCallContext{
		Request:  &agentcore.ProviderRequest{},
		Response: &agentcore.ProviderResponse{Content: "some response"},
	})
}

func TestMemoryLifecycleHook_AfterModelCall_NilMCC(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}, manager: &Manager{}}
	hook := &memoryLifecycleHook{ext: ext}

	// Must not panic when mcc is nil.
	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, nil)
}

func TestMemoryLifecycleHook_AfterModelCall_NilRequest(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}, manager: &Manager{}}
	hook := &memoryLifecycleHook{ext: ext}

	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, &agentcore.ModelCallContext{
		Request:  nil,
		Response: &agentcore.ProviderResponse{Content: "response"},
	})
}

func TestMemoryLifecycleHook_AfterModelCall_NilResponse(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}, manager: &Manager{}}
	hook := &memoryLifecycleHook{ext: ext}

	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, &agentcore.ModelCallContext{
		Request:  &agentcore.ProviderRequest{},
		Response: nil,
	})
}

func TestMemoryLifecycleHook_AfterModelCall_EmptyMessages(t *testing.T) {
	ext := &MemoryExtension{cfg: ExtensionConfig{AutoExtract: true}, manager: &Manager{}}
	hook := &memoryLifecycleHook{ext: ext}

	// Both user message and response content are empty → returns early.
	hook.AfterModelCall(context.Background(), &agentcore.AgentRunContext{}, &agentcore.ModelCallContext{
		Request:  &agentcore.ProviderRequest{},
		Response: &agentcore.ProviderResponse{Content: ""},
	})
}
