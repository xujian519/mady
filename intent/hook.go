package intent

import (
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Hook is a LifecycleHook that classifies user intent before each model call.
// The result can be retrieved via LastResult().
type Hook struct {
	Router     *UnifiedRouter
	lastResult IntentResult
	mu         sync.RWMutex
}

// NewHook creates a new intent classification hook.
func NewHook(router *UnifiedRouter) *Hook {
	return &Hook{Router: router}
}

// BeforeModelCall classifies the latest user input and stores the result.
func (h *Hook) BeforeModelCall(arc *agentcore.AgentRunContext) error {
	if h.Router == nil || arc == nil {
		return nil
	}

	input := latestUserInput(arc)
	if input == "" {
		return nil
	}

	result := h.Router.Classify(input)

	h.mu.Lock()
	h.lastResult = result
	h.mu.Unlock()

	return nil
}

// LastResult returns the most recent intent classification result.
func (h *Hook) LastResult() IntentResult {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastResult
}

// latestUserInput finds the most recent user message in the conversation.
func latestUserInput(arc *agentcore.AgentRunContext) string {
	for i := len(arc.Messages) - 1; i >= 0; i-- {
		if arc.Messages[i].Role == agentcore.RoleUser && arc.Messages[i].Content != "" {
			return arc.Messages[i].Content
		}
	}
	return ""
}
