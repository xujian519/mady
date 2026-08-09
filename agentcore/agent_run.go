package agentcore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// --- 公开入口 ---

// maxTruncationContinuations 是纯文本输出被 max_tokens 截断时自动续写的
// 最大次数。续写一次后若仍截断，则按截断结果正常结束，由下游通道透传
// FinishReason 提示用户，避免无限续写造成死循环。
//
// 边界说明：自动续写针对"输出超出预算被截断"的场景（finish_reason="length"
// 而模型本可继续）。若用户显式配置了较小的 max_tokens（配置性截断），续写
// 轮可能仍被截断——此时不放大输出预算（放大可能超 provider 上限导致续写
// 轮报错、整轮失败，且绕过用户配置意图），而是按 length 正常结束并提示用户
// 输出可能不完整，由用户决定是否继续。
const maxTruncationContinuations = 1

// steering 提示词常量：runInnerLoop 检测到异常模式（截断/重复文本/重复
// 工具调用）时，通过 messageQueue 注入系统消息引导模型摆脱当前状态。
const (
	// steeringMsgTruncation 在纯文本输出被 max_tokens 截断时要求模型
	// 直接从断点续写，避免重复已输出内容。
	steeringMsgTruncation = "你的上一条回复因达到输出长度上限（max_tokens）被截断。请直接从上次中断处继续输出剩余内容，不要重复已输出的部分。"
	// steeringMsgRepeatText 在模型连续多轮输出相同文本时要求其停止循环。
	steeringMsgRepeatText = "You have been repeating the same response. Stop this loop immediately. Do not call any more tools. Give a final answer based on what you have so far, or clearly state that you cannot complete the request and ask the user for guidance."
	// steeringMsgRepeatTool 在模型反复调用相同工具而无进展时要求其停止。
	steeringMsgRepeatTool = "You have been calling the same tools repeatedly without progress. Stop this loop immediately. Do not call any more tools. Report to the user what you attempted and why it failed, and ask for guidance."
)

// LastFinishReason 返回最近一次 Run/Continue/Resume 的模型结束原因
// （如 "stop"/"length"/"error"）。"length" 表示输出触达 max_tokens 上限
// 可能被截断；"error" 表示流异常终止。未运行时返回空字符串。
func (a *Agent) LastFinishReason() string {
	if v, ok := a.lastFinishReason.Load().(string); ok {
		return v
	}
	return ""
}

// Run 启动 Agent 循环，传入新的用户输入。
// Agent 可跨多次 Run 调用复用 —— 会话状态在调用间保留，
// 系统提示词仅首次持久化。
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	// 快速失败：拒绝以无效配置运行。
	if a.configErr != nil {
		return "", fmt.Errorf("agentcore: agent configuration is invalid: %w", a.configErr)
	}

	// 快速失败：拒绝空输入。
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("agentcore: empty input in Run()")
	}

	a.todopad.Reset()

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
		arc := a.newRunContext(input, a.state.Messages(), 0)
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
		arc := a.newRunContext(input, a.state.Messages(), 0)
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
	// 每次运行开始时清空上次的结束原因：若本次运行在记录 finishReason
	// 之前失败（返回 err），LastFinishReason 保持为空，避免池化复用的
	// Agent 读到上次运行的残留值而误报截断。
	a.lastFinishReason.Store("")
	loopStartTurn := a.state.Turn()
	var finalOutput string
	var finishReason string
	var finished bool
	var err error

	for {
		finalOutput, finishReason, finished, err = a.runInnerLoop(ctx, loopStartTurn)
		if err != nil {
			return "", err
		}

		if finished {
			break
		}

		// Check for follow-up messages
		followUps := a.followUp.Drain()
		if len(followUps) == 0 {
			break
		}

		// Restart the loop with follow-up messages
		a.state.SetStatus(StatusRunning)
		for _, msg := range followUps {
			if err := a.persistMessage(ctx, msg); err != nil {
				ne := NewNodeError("lifecycle persist follow-up failed", err, a.config.Name, "follow_up")
				a.state.SetStatus(StatusError)
				a.emitMustDeliver(ctx, &AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: ne})
				return "", ne
			}
		}
	}

	// Unified terminal-event emission. Interrupt and error paths set a
	// different status; each gets its own event type here so there is
	// exactly one terminal event per run. The follow-up loop above also
	// emits AgentErrorEvent directly (with immediate return) and thus
	// does NOT reach this switch.
	//
	// Terminal events use EmitMustDeliver to ensure they reach subscribers
	// even when the internal buffer is under pressure.
	switch a.state.Status() {
	case StatusFinished:
		a.emitMustDeliver(ctx, &AgentEndEvent{
			baseEvent:    newBase(EventAgentEnd),
			AgentName:    a.config.Name,
			Output:       finalOutput,
			FinishReason: finishReason,
		})
	case StatusInterrupted:
		a.emitMustDeliver(ctx, &AgentInterruptEvent{
			baseEvent: newBase(EventAgentInterrupt),
			AgentName: a.config.Name,
			Reason:    a.state.GetInterruptReason(),
		})
	case StatusError:
		a.emitMustDeliver(ctx, &AgentErrorEvent{
			baseEvent: newBase(EventAgentError),
			Err:       NewNodeError("agent run loop exited with error", nil, a.config.Name, "runLoop"),
		})
	}
	// 记录模型结束原因，供同步调用方（server 同步端点 / ACP）判断
	// 输出是否因 max_tokens 截断或流异常而可能不完整。
	a.lastFinishReason.Store(finishReason)
	return finalOutput, nil
}

// runInnerLoop 执行内层轮次循环，直到模型停止调用工具或达到终止条件。
//
// 返回值：
//   - finalOutput:  Agent 的最终文本响应（可能为空）
//   - finishReason: 收尾轮次（无工具调用）模型上报的结束原因，
//     如 "stop"/"length"/"error"；其他路径为空
//   - finished:     是否达到终止状态（StatusFinished /
//     StatusError/StatusInterrupted）；false 表示模型停止调用工具，
//     可能存在跟随消息
//   - err:          不可恢复的错误
//
// 重复检测状态（lastContent/repeatCount/...）在每次调用中局部化，
// 有意不在跟随消息轮次间共享。
//
//nolint:gocognit // 原因：Agent 内循环，含状态机和重复检测逻辑
func (a *Agent) runInnerLoop(ctx context.Context, loopStartTurn int64) (string, string, bool, error) {
	var finalOutput string
	var finishReason string
	var lastContent string
	var repeatCount int
	var lastToolSignature string
	var toolRepeatCount int
	// truncationContinuations 记录本轮循环内因 max_tokens 截断触发的
	// 自动续写次数，超过 maxTruncationContinuations 后不再续写。
	var truncationContinuations int

	for a.state.Status() == StatusRunning {
		turn := a.state.NextTurn()

		if err := a.runPreTurn(ctx, loopStartTurn, turn); err != nil {
			return "", "", true, err
		}

		resp, err := a.runModelTurn(ctx, turn)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				// User canceled the request — maintain Before/After pairing
				// via endTurn, then exit as an interruption (not a clean finish).
				if e := a.endTurn(ctx, turn, TokenUsage{}, false); e != nil {
					slog.Debug("agent_run: endTurn failed during context cancellation", "turn", turn, "error", e)
				}
				a.state.SetStatus(StatusInterrupted)
				return "", "", true, nil
			}
			return "", "", true, a.failLoop(ctx, fmt.Sprintf("turn:%d|provider", turn), "provider call failed", err)
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
				return "", "", true, a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "lifecycle persist assistant failed", err)
			}
		}

		if len(resp.ToolCalls) == 0 {
			// 输出完整性保护：模型因 max_tokens 截断输出（finish_reason="length"）
			// 时，自动追加一轮续写，要求模型从断点继续完成剩余内容。仅续写
			// maxTruncationContinuations 次，续写后仍截断则按截断结果正常结束，
			// FinishReason 保留 "length" 供下游通道提示用户输出可能不完整。
			if resp.FinishReason == "length" && truncationContinuations < maxTruncationContinuations {
				truncationContinuations++
				if err := a.steering.Push(Message{
					Role:    RoleSystem,
					Content: steeringMsgTruncation,
				}); err == nil {
					if err := a.endTurn(ctx, turn, resp.Usage, false); err != nil {
						return "", "", true, err
					}
					continue
				}
				slog.Warn("agent: failed to push truncation continuation message", "error", err)
			}

			finalOutput = resp.Content
			finishReason = resp.FinishReason
			a.state.SetStatus(StatusFinished)
			if err := a.endTurn(ctx, turn, resp.Usage, false); err != nil {
				return "", "", true, err
			}
			break
		}

		// Truncation guard: when the provider reports finish_reason="length" the
		// model hit max_tokens and any tool-call arguments may be cut mid-JSON.
		if handled, gErr := a.guardTruncation(ctx, turn, resp); handled {
			if gErr != nil {
				return "", "", true, a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "truncation guard failed", gErr)
			}
			continue
		}

		earlyExit, err := a.executeToolCalls(ctx, resp.ToolCalls)
		if err != nil {
			if IsInterrupt(err) {
				a.state.SetStatus(StatusInterrupted)
				a.state.SetInterruptReason(a.interrupted.Load())
				return "", "", true, nil
			}
			return "", "", true, a.failLoop(ctx, fmt.Sprintf("turn:%d", turn), "tool execution persist failed", err)
		}

		// Early-exit: a tool returned a terminating result
		if earlyExit != "" {
			finalOutput = earlyExit
			a.state.SetStatus(StatusFinished)
			if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
				return "", "", true, err
			}
			break
		}

		// Context cancellation during tool execution
		if errors.Is(ctx.Err(), context.Canceled) {
			if e := a.endTurn(ctx, turn, resp.Usage, true); e != nil {
				slog.Debug("agent_run: endTurn failed during context cancellation", "turn", turn, "error", e)
			}
			a.state.SetStatus(StatusInterrupted)
			return "", "", true, nil
		}
		if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
			return "", "", true, err
		}

		// Transfer handoff
		if handoff := a.state.PendingHandoff(); handoff != nil {
			a.state.ClearPendingHandoff()
			out, err := a.handleTransfer(ctx, handoff)
			return out, "", true, err
		}

		// Repetition detection: if the model emits the same text 3+ turns in a
		// row it is stuck in a loop. Inject a steering message to break out.
		if turn-loopStartTurn >= 2 && resp.Content != "" && resp.Content == lastContent {
			repeatCount++
			if repeatCount >= 2 {
				if err := a.steering.Push(Message{
					Role:    RoleSystem,
					Content: steeringMsgRepeatText,
				}); err != nil {
					slog.Warn("agent: failed to push steering message", "error", err)
				}
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
					if err := a.steering.Push(Message{
						Role:    RoleSystem,
						Content: steeringMsgRepeatTool,
					}); err != nil {
						slog.Warn("agent: failed to push steering message", "error", err)
					}
					lastToolSignature = ""
					toolRepeatCount = 0
				}
			} else {
				lastToolSignature = sig
				toolRepeatCount = 0
			}
		}
	}

	return finalOutput, finishReason, false, nil
}
