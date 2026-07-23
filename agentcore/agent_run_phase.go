package agentcore

import (
	"context"
	"fmt"
	"log/slog"
)

// runPreTurn executes the pre-model phase of a single turn:
// MaxTurns check, context compaction, steering injection, BeforeTurn lifecycle,
// turn checkpoint, and TurnStartEvent.
// Returns a terminal error (caller should abort the loop) or nil.
func (a *Agent) runPreTurn(ctx context.Context, loopStartTurn, turn int64) error {
	if turn-loopStartTurn > a.config.MaxTurns {
		err := NewNodeError("exceeded max turns", ErrExceedMaxSteps, a.config.Name, fmt.Sprintf("turn:%d", turn))
		a.state.SetStatus(StatusError)
		a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: err})
		return err
	}
	if err := a.maybeCompact(ctx); err != nil {
		return a.failLoop(ctx, fmt.Sprintf("turn:%d|compaction", turn), "compaction failed", err)
	}
	if steered := a.steering.Drain(); len(steered) > 0 {
		for _, msg := range steered {
			if err := a.persistMessage(ctx, msg); err != nil {
				return a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "lifecycle persist steering failed", err)
			}
		}
	}
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
		if err := lc.BeforeTurn(ctx, arc); err != nil {
			return a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "lifecycle before_turn failed", err)
		}
	}
	if err := a.checkpointTurnStart(ctx, turn); err != nil {
		a.state.SetStatus(StatusError)
		a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: err})
		return err
	}
	a.emit(&TurnStartEvent{baseEvent: newBase(EventTurnStart), Turn: turn})
	return nil
}

// runModelTurn builds the provider request, runs the BeforeModelCall lifecycle
// hook, and calls the LLM. On success it returns the provider response.
// A canceled context is returned as context.Canceled for the caller to handle;
// all other provider errors are wrapped via failLoop.
func (a *Agent) runModelTurn(ctx context.Context, turn int64) (*ProviderResponse, error) {
	msgs := a.buildRequestMessages(ctx)

	req := &ProviderRequest{
		Model:          a.config.Model,
		Messages:       msgs,
		Tools:          a.registry.Definitions(),
		Temperature:    a.config.Temperature,
		MaxTokens:      a.config.MaxTokens,
		ResponseFormat: a.config.ResponseFormat,
		Thinking:       a.config.Thinking,
	}

	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
		mcc := &ModelCallContext{Request: req}
		if lcErr := lc.BeforeModelCall(ctx, arc, mcc); lcErr != nil {
			return nil, a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "lifecycle before_model_call failed", lcErr)
		}
	}

	resp, err := a.callModelWithFallback(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// runAfterModelCall runs the AfterModelCall lifecycle hook.
// Returns true if the hook returned an error (meaning the caller should
// continue the outer loop after persisting error messages); false otherwise.
// When the hook has an error, the assistant + error messages are persisted.
func (a *Agent) runAfterModelCall(ctx context.Context, turn int64, resp *ProviderResponse) bool {
	lc := a.lifecycle()
	if lc == nil {
		return false
	}
	arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
	mcc := &ModelCallContext{Request: nil, Response: resp}
	lc.AfterModelCall(ctx, arc, mcc)
	if mcc.Err == nil {
		return false
	}
	if !resp.SuppressPersist {
		if pErr := a.persistMessage(ctx, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			Blocks:    resp.Blocks,
			ToolCalls: resp.ToolCalls,
		}); pErr != nil {
			_ = a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "lifecycle persist assistant failed", pErr)
			return true
		}
	}
	if err := a.persistMessage(ctx, Message{
		Role:    RoleSystem,
		Content: fmt.Sprintf("错误: %s", mcc.Err.Error()),
	}); err != nil {
		_ = a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "lifecycle persist guardrail error failed", err)
	}
	return true
}

// callModelWithFallback 调用 Provider，按 Context Overflow 触发一次压缩重试。
func (a *Agent) callModelWithFallback(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	resp, err := a.callProviderWithRetry(ctx, req)
	if err != nil {
		// Context overflow: attempt compaction then retry once
		if IsContextOverflowError(err) && a.config.ContextWindow > 0 {
			if compErr := a.ForceCompact(ctx); compErr == nil {
				req.Messages = a.buildRequestMessages(ctx)
				resp, err = a.callProviderWithRetry(ctx, req)
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// guardTruncation 检测模型输出是否被 max_tokens 截断且包含无效工具调用参数。
// 返回 (handled, err)：
//   - handled=true, err=nil  → 截断已处理，调用方应 continue
//   - handled=true, err!=nil → 截断处理中遇到致命错误
//   - handled=false          → 非截断场景，调用方继续正常流程
func (a *Agent) guardTruncation(ctx context.Context, turn int64, resp *ProviderResponse) (bool, error) {
	if resp.FinishReason != "length" || !hasInvalidToolCallArgs(resp.ToolCalls) {
		return false, nil
	}
	for _, tc := range resp.ToolCalls {
		if tc.ID == "" {
			slog.Debug("agent_run: guardTruncation found tool call with empty ID",
				"turn", turn, "tool", tc.Name)
			continue
		}
		if perr := a.persistMessage(ctx, Message{
			Role:       RoleTool,
			Content:    "错误: 此工具调用未被执行，因为模型输出被 max_tokens 截断，生成了无效的 JSON 参数。请重新生成包含完整参数的工具调用；如果调用内容较大，请拆分或减少输出长度。",
			ToolCallID: tc.ID,
			Name:       tc.Name,
		}); perr != nil {
			return true, perr
		}
	}
	if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
		return true, err
	}
	return true, nil
}

// failLoop is the standard error exit path from the run loop.
// It wraps err in a NodeError, sets error status, emits an error event,
// and returns the constructed NodeError. ctxTag is typically fmt.Sprintf("turn:%d", turn).
func (a *Agent) failLoop(ctx context.Context, ctxTag, description string, err error) error {
	ne := NewNodeError(description, err, a.config.Name, ctxTag)
	a.state.SetStatus(StatusError)
	a.emitMustDeliver(ctx, &AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: ne})
	return ne
}

// endTurn emits the TurnEndEvent, runs AfterTurn lifecycle, and checkpoints.
// Returns an error if checkpointing fails (the caller should return it).
func (a *Agent) endTurn(ctx context.Context, turn int64, usage TokenUsage, hadToolCalls bool) error {
	a.emit(&TurnEndEvent{baseEvent: newBase(EventTurnEnd), Turn: turn, Usage: usage})
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
		lc.AfterTurn(ctx, arc, TurnInfo{HadToolCalls: hadToolCalls})
	}
	if err := a.checkpointTurnEnd(ctx, turn); err != nil {
		a.state.SetStatus(StatusError)
		a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: err})
		return err
	}
	return nil
}
