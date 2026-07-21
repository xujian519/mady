package agentcore

import (
	"context"
	"errors"
	"fmt"
)

// --- 公开入口 ---

// Run 启动 Agent 循环，传入新的用户输入。
// Agent 可跨多次 Run 调用复用 —— 会话状态在调用间保留，
// 系统提示词仅首次持久化。
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	// 快速失败：拒绝以无效配置运行。
	if a.configErr != nil {
		return "", fmt.Errorf("agentcore: agent configuration is invalid: %w", a.configErr)
	}

	ctx, span := a.tracer().Start(ctx, "agent.run",
		Attr("agent.name", a.config.Name),
		Attr("agent.model", a.config.Model),
	)
	defer span.End()
	defer a.eventBus.Drain()

	a.state.SetStatus(StatusRunning)
	a.emit(&AgentStartEvent{
		baseEvent: newBase(EventAgentStart),
		AgentName: a.config.Name,
		Input:     input,
	})

	// Only persist system prompt if not already present in conversation history.
	if sp := a.systemPrompt(); sp != "" && !a.state.HasSystemPrompt() {
		if err := a.persistMessage(ctx, Message{Role: RoleSystem, Content: sp}); err != nil {
			span.RecordError(err)
			return "", WrapNodeError(err, "lifecycle:persist_system")
		}
	}
	if err := a.persistMessage(ctx, Message{Role: RoleUser, Content: input}); err != nil {
		span.RecordError(err)
		return "", WrapNodeError(err, "lifecycle:persist_user")
	}

	// Lifecycle: BeforeAgentRun
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Input: input, Messages: a.state.Messages()}
		if err := lc.BeforeAgentRun(ctx, arc); err != nil {
			span.RecordError(err)
			return "", WrapNodeError(err, "lifecycle:before_agent_run")
		}
	}

	if a.contextEngine != nil {
		a.contextEngine.OnSessionStart(ctx, a.config.Model, a.config.ContextWindow)
	}

	output, err := a.runLoop(ctx)

	// Lifecycle: AfterAgentRun
	if lc := a.lifecycle(); lc != nil {
		arc := &AgentRunContext{Agent: a, Input: input, Messages: a.state.Messages()}
		lc.AfterAgentRun(ctx, arc, output, err)
	}

	if err != nil {
		span.RecordError(err)
	}
	return output, err
}

// Continue 从当前状态恢复 Agent 循环，不添加新输入。
func (a *Agent) Continue(ctx context.Context) (string, error) {
	ctx, span := a.tracer().Start(ctx, "agent.continue",
		Attr("agent.name", a.config.Name),
	)
	defer span.End()
	defer a.eventBus.Drain()

	a.state.SetStatus(StatusRunning)
	a.emit(&AgentStartEvent{
		baseEvent: newBase(EventAgentStart),
		AgentName: a.config.Name,
	})

	output, err := a.runLoop(ctx)
	if err != nil {
		span.RecordError(err)
	}
	return output, err
}

// Interrupted 返回中断原因，如果 Agent 被中断；
// 正常完成或尚未运行时返回 nil。
func (a *Agent) Interrupted() *InterruptReason {
	return a.interrupted.Load()
}

// Resume 在中断后继续执行。Agent 必须处于 StatusInterrupted 状态
// （检查 Interrupted() != nil）。它会从中断触发的工具结果处重放对话，
// 允许 LLM 自然继续。
func (a *Agent) Resume(ctx context.Context) (string, error) {
	ir := a.Interrupted()
	if ir == nil {
		return "", fmt.Errorf("agent is not interrupted (status: %s)", a.state.Status())
	}
	a.interrupted.Store(nil)
	a.state.ClearInterruptReason()
	a.state.SetStatus(StatusRunning)
	a.emit(&AgentStartEvent{
		baseEvent: newBase(EventAgentStart),
		AgentName: a.config.Name,
	})
	defer a.eventBus.Drain()
	output, err := a.runLoop(ctx)
	if err != nil {
		return "", WrapNodeError(err, "resume")
	}
	return output, nil
}

// --- 核心运行循环 ---

// runLoop 是 Run、Continue、Resume 共用的核心轮次循环。
// 外层循环处理跟随消息；内层循环处理工具调用轮次。
// MaxTurns 按每次 runLoop 调用执行（不跨会话累积）。
func (a *Agent) runLoop(ctx context.Context) (string, error) {
	loopStartTurn := a.state.Turn()

	for {
		finalOutput, finished, err := a.runInnerLoop(ctx, loopStartTurn)
		if err != nil {
			return "", err
		}
		if finished {
			return finalOutput, nil
		}

		// Outer loop: check for follow-up messages
		followUps := a.followUp.Drain()
		if len(followUps) == 0 {
			// No follow-ups: emit clean end event with whatever output we have.
			a.emit(&AgentEndEvent{
				baseEvent: newBase(EventAgentEnd),
				AgentName: a.config.Name,
				Output:    finalOutput,
			})
			return finalOutput, nil
		}

		// Restart the loop with follow-up messages
		a.state.SetStatus(StatusRunning)
		for _, msg := range followUps {
			if err := a.persistMessage(ctx, msg); err != nil {
				ne := NewNodeError("lifecycle persist follow-up failed", err, a.config.Name, "follow_up")
				a.state.SetStatus(StatusError)
				a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: ne})
				return "", ne
			}
		}
	}
}

// runInnerLoop 执行内层轮次循环，直到模型停止调用工具或达到终止条件。
//
// 返回值：
//   - finalOutput: Agent 的最终文本响应（可能为空）
//   - finished:    是否达到终止状态（StatusFinished /
//     StatusError/StatusInterrupted）；false 表示模型停止调用工具，
//     可能存在跟随消息
//   - err:         不可恢复的错误
//
// 重复检测状态（lastContent/repeatCount/...）在每次调用中局部化，
// 有意不在跟随消息轮次间共享。
func (a *Agent) runInnerLoop(ctx context.Context, loopStartTurn int64) (string, bool, error) {
	var finalOutput string
	var lastContent string
	var repeatCount int
	var lastToolSignature string
	var toolRepeatCount int

	for a.state.Status() == StatusRunning {
		turn := a.state.NextTurn()

		if err := a.runPreTurn(ctx, loopStartTurn, turn); err != nil {
			return "", true, err
		}

		resp, err := a.runModelTurn(ctx, turn)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// User interrupted — emit clean end event instead of cryptic error.
				a.state.SetStatus(StatusFinished)
				a.emit(&AgentEndEvent{
					baseEvent: newBase(EventAgentEnd),
					AgentName: a.config.Name,
				})
				return "", true, nil
			}
			return "", true, a.failLoop(fmt.Sprintf("turn:%d|provider", turn), "provider call failed", err)
		}

		// Lifecycle: AfterModelCall — error is non-fatal, persist and continue
		if a.runAfterModelCall(ctx, turn, resp) {
			continue
		}

		// Accumulate usage
		if resp.Usage.TotalTokens > 0 {
			a.state.AddUsage(resp.Usage)
			if a.contextEngine != nil {
				a.contextEngine.UpdateFromResponse(resp.Usage)
			}
		}

		if !resp.SuppressPersist {
			if err := a.persistMessage(ctx, Message{
				Role:      RoleAssistant,
				Content:   resp.Content,
				Blocks:    resp.Blocks,
				ToolCalls: resp.ToolCalls,
			}); err != nil {
				return "", true, a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle persist assistant failed", err)
			}
		}

		if len(resp.ToolCalls) == 0 {
			finalOutput = resp.Content
			a.state.SetStatus(StatusFinished)
			if err := a.endTurn(ctx, turn, resp.Usage, false); err != nil {
				return "", true, err
			}
			break
		}

		// Truncation guard: when the provider reports finish_reason="length" the
		// model hit max_tokens and any tool-call arguments may be cut mid-JSON.
		if handled, gErr := a.guardTruncation(ctx, turn, resp); handled {
			if gErr != nil {
				return "", true, a.failLoop(fmt.Sprintf("turn:%d", turn), "truncation guard failed", gErr)
			}
			continue
		}

		earlyExit, err := a.executeToolCalls(ctx, resp.ToolCalls)
		if err != nil {
			if IsInterrupt(err) {
				a.state.SetStatus(StatusInterrupted)
				a.state.SetInterruptReason(a.interrupted.Load())
				a.emit(&AgentInterruptEvent{
					baseEvent: newBase(EventAgentInterrupt),
					AgentName: a.config.Name,
					Reason:    a.interrupted.Load(),
				})
				return "", true, nil
			}
			return "", true, a.failLoop(fmt.Sprintf("turn:%d", turn), "tool execution persist failed", err)
		}

		// Early-exit: a tool returned a terminating result
		if earlyExit != "" {
			finalOutput = earlyExit
			a.state.SetStatus(StatusFinished)
			if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
				return "", true, err
			}
			break
		}

		// Context cancellation during tool execution
		if errors.Is(ctx.Err(), context.Canceled) {
			a.state.SetStatus(StatusFinished)
			a.emit(&AgentEndEvent{
				baseEvent: newBase(EventAgentEnd),
				AgentName: a.config.Name,
			})
			return "", true, nil
		}
		if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
			return "", true, err
		}

		// Transfer handoff
		if handoff := a.state.PendingHandoff(); handoff != nil {
			a.state.ClearPendingHandoff()
			out, err := a.handleTransfer(ctx, handoff)
			return out, true, err
		}

		// Repetition detection: if the model emits the same text 3+ turns in a
		// row it is stuck in a loop. Inject a steering message to break out.
		if turn-loopStartTurn >= 2 && resp.Content != "" && resp.Content == lastContent {
			repeatCount++
			if repeatCount >= 2 {
				a.steering.Push(Message{
					Role:    RoleSystem,
					Content: "You have been repeating the same response. Stop this loop immediately. Do not call any more tools. Give a final answer based on what you have so far, or clearly state that you cannot complete the request and ask the user for guidance.",
				})
				lastContent = ""
				repeatCount = 0
			}
		} else if resp.Content != "" {
			lastContent = resp.Content
			repeatCount = 0
		}

		// Tool-call repetition detection: if the model makes the same set of
		// tool calls (by name) 3+ turns in a row, it is stuck in a retry loop
		if len(resp.ToolCalls) > 0 {
			sig := toolCallSignature(resp.ToolCalls)
			if sig == lastToolSignature {
				toolRepeatCount++
				if toolRepeatCount >= 2 {
					a.steering.Push(Message{
						Role:    RoleSystem,
						Content: "You have been calling the same tools repeatedly without progress. Stop this loop immediately. Do not call any more tools. Report to the user what you attempted and why it failed, and ask for guidance.",
					})
					lastToolSignature = ""
					toolRepeatCount = 0
				}
			} else {
				lastToolSignature = sig
				toolRepeatCount = 0
			}
		}
	}

	return finalOutput, false, nil
}
