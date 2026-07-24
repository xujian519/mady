package agentcore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// toolCallSignature builds a stable, ordered string key from a set of tool call
// names. Used by the repetition detector to catch retry loops where the model
// calls the same tools each turn but with varying text content.
func toolCallSignature(calls []ToolCall) string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// isToolPermanentlyUnavailable 检测工具错误是否表明底层服务已不可恢复。
// 当前通过错误消息模式匹配来识别 MCP 客户端断开等致命错误。
// 匹配的错误表示重试无意义，LLM 应停止调用该工具并寻找替代方案。
func isToolPermanentlyUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// MCP stdio 客户端已关闭（无法重连）
	if strings.Contains(msg, "mcp client closed") {
		return true
	}
	// MCP HTTP 客户端已关闭
	if strings.Contains(msg, "MCP client is closed") {
		return true
	}
	return false
}

// persistMessage appends a message after lifecycle BeforeMessagePersist /
// AfterMessagePersist hooks. ReplaceMessages (compaction) bypasses this.
func (a *Agent) persistMessage(ctx context.Context, m Message) error {
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: a.state.Turn()}
		cp := m
		if err := lc.BeforeMessagePersist(ctx, arc, &cp); err != nil {
			return err
		}
		m = cp
	}
	a.state.AddMessage(m)
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: a.state.Turn()}
		lc.AfterMessagePersist(ctx, arc, m)
	}
	return nil
}

func (a *Agent) checkpointTurnStart(ctx context.Context, turn int64) error {
	c := a.config.Checkpoint
	if c == nil || c.Saver == nil || !c.SaveOnTurnStart {
		return nil
	}
	if err := a.appendCheckpoint(ctx); err != nil {
		return NewNodeError("checkpoint failed", err, a.config.Name, fmt.Sprintf("turn:%d", turn), "checkpoint_turn_start")
	}
	return nil
}

func (a *Agent) checkpointTurnEnd(ctx context.Context, turn int64) error {
	c := a.config.Checkpoint
	if c == nil || c.Saver == nil || c.SkipSaveOnTurnEnd {
		return nil
	}
	if err := a.appendCheckpoint(ctx); err != nil {
		return NewNodeError("checkpoint failed", err, a.config.Name, fmt.Sprintf("turn:%d", turn), "checkpoint_turn_end")
	}
	return nil
}

// executeToolCalls runs tool calls with lifecycle hooks and persist results.
// It returns the early-exit content (non-empty) when a tool requested loop
// termination via TerminateResult; otherwise it returns "" and the error.
func (a *Agent) executeToolCalls(ctx context.Context, calls []ToolCall) (string, error) {
	// Lifecycle: BeforeToolExecution
	// Pre-allocate results so lifecycle hooks can pre-populate blocked tool
	results := make([]ToolResult, len(calls))
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: a.state.Turn()}
		tec := &ToolExecutionContext{ToolCalls: calls, Results: results}
		if err := lc.BeforeToolExecution(ctx, arc, tec); err != nil {
			for _, tc := range calls {
				if perr := a.persistMessage(ctx, Message{
					Role:       RoleTool,
					Content:    fmt.Sprintf("错误: 工具执行被生命周期钩子阻止: %v", err),
					ToolCallID: tc.ID,
					Name:       tc.Name,
				}); perr != nil {
					return "", perr
				}
			}
			return "", nil
		}
	}

	// Filter out tools already blocked by BeforeToolExecution hooks.
	var activeCalls []ToolCall
	activeIdx := make([]int, 0, len(calls))
	for i, tc := range calls {
		// A blocked tool has a pre-populated result with non-empty ToolCallID.
		if results[i].ToolCallID == "" {
			activeCalls = append(activeCalls, tc)
			activeIdx = append(activeIdx, i)
		}
	}

	cb := &ExecuteCallbacks{
		OnStart: func(tc ToolCall) {
			a.emit(&ToolCallStartEvent{
				baseEvent: newBase(EventToolCallStart),
				ToolCall:  tc,
			})
		},
		OnEnd: func(r ToolResult) {
			a.emit(&ToolCallEndEvent{
				baseEvent:  newBase(EventToolCallEnd),
				ToolCallID: r.ToolCallID,
				ToolName:   r.ToolName,
				Result:     r.Result,
				Err:        r.Err,
				Duration:   r.Duration,
			})
		},
	}

	if len(activeCalls) > 0 {
		activeResults := a.executor.ExecuteAll(ctx, activeCalls, a.state, cb)
		for j, idx := range activeIdx {
			results[idx] = activeResults[j]
		}
	}

	// Lifecycle: AfterToolExecution.
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: a.state.Turn()}
		tec := &ToolExecutionContext{ToolCalls: calls, Results: results}
		lc.AfterToolExecution(ctx, arc, tec)
		results = tec.Results
	}

	// Persist all completed tool results first (even in parallel mode where
	// one tool may interrupt while others completed successfully).
	var earlyExit string
	var interrupt *InterruptReason
	for i, tc := range calls {
		r := results[i]

		// Skip unexecuted tools (serial mode stopped early after interrupt).
		if r.ToolCallID == "" && r.ToolName == "" && r.Result == "" && r.Err == nil {
			continue
		}

		// Early-exit: a tool requested loop termination; its result is the
		// final answer. First terminating tool wins.
		if r.Terminate && earlyExit == "" {
			earlyExit = r.Result
			if earlyExit == "" {
				earlyExit = r.EffectiveResult()
			}
		}

		content := r.Result
		if r.Err != nil {
			switch {
			case errors.Is(r.Err, context.Canceled):
				content = "工具执行被中断"
			case IsInterrupt(r.Err):
				content = r.Result
				if content == "" {
					content = r.Err.Error()
				}
			default:
				if isToolPermanentlyUnavailable(r.Err) {
					content = fmt.Sprintf("错误: %s\n\n此工具对应的底层服务当前不可用（连接已断开且无法恢复）。请不要再重试此工具，改用其他可用方式完成任务，或告知用户该服务暂时不可用。", r.Err.Error())
				} else {
					content = fmt.Sprintf("错误: %s", r.Err.Error())
				}
			}
		}
		if err := a.persistMessage(ctx, Message{
			Role:       RoleTool,
			Content:    content,
			ToolCallID: tc.ID,
			Name:       tc.Name,
		}); err != nil {
			return "", err
		}

		if IsInterrupt(r.Err) && interrupt == nil {
			interrupt = &InterruptReason{
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				Reason:     InterruptMessage(r.Err),
				Data:       InterruptData(r.Err),
			}
		}
	}
	if earlyExit != "" {
		return earlyExit, nil
	}
	if interrupt != nil {
		a.interrupted.Store(interrupt)
		a.state.SetInterruptReason(interrupt)
		if err := a.appendCheckpoint(ctx); err != nil {
			return "", err
		}
		return "", ErrInterrupt
	}
	return "", nil
}

func (a *Agent) buildRequestMessages(ctx context.Context) []Message {
	msgs := a.state.messagesReadOnly()
	if cb := a.contextBuilder(); cb != nil {
		buildInput := BuildInput{
			Messages:      msgs,
			ToolDefs:      a.registry.Definitions(),
			SystemPrompt:  a.systemPrompt(),
			ContextWindow: a.config.ContextWindow,
			ReserveTokens: applyDefaultReserveTokens(a.config.ContextWindow, a.config.ReserveTokens),
			LayerConfigs:  a.config.LayerConfigs,
		}
		output := cb.Build(ctx, buildInput)
		msgs = output.Messages
	} else if tc := a.transformContext(); tc != nil {
		msgs = tc(ctx, msgs)
	}
	converter := a.config.ConvertToLLM
	if converter == nil {
		converter = DefaultConvertToLLM
	}
	return converter(msgs)
}

// applyDefaultReserveTokens returns ReserveTokens or defaults to ContextWindow/4.
func applyDefaultReserveTokens(contextWindow, reserveTokens int64) int64 {
	if reserveTokens > 0 {
		return reserveTokens
	}
	if contextWindow > 0 {
		def := contextWindow / 4
		if def > 0 {
			return def
		}
	}
	return 0
}
