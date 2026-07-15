package agentcore

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// --- run ---

// Run starts the agent loop with a new user input.
// The Agent can be reused across multiple Run calls — conversation state is
// preserved between calls and system prompt is only persisted once.
func (a *Agent) Run(ctx context.Context, input string) (string, error) {
	// Fail fast: refuse to run with an invalid configuration.
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

// Continue resumes the agent loop from the current state without adding new input.
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

// Interrupted returns the interrupt reason if the agent was interrupted,
// or nil if it completed normally or hasn't run yet.
func (a *Agent) Interrupted() *InterruptReason {
	return a.interrupted.Load()
}

// Resume continues execution after an interrupt. The agent must have
// StatusInterrupted (check Interrupted() != nil). It replays the
// conversation from the tool result that triggered the interrupt,
// allowing the LLM to continue naturally.
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

// runLoop is the core turn loop shared by Run, Continue, and Resume.
// Outer loop handles follow-up messages; inner loop handles tool call turns.
// MaxTurns is enforced per runLoop invocation (not cumulative across the session).
func (a *Agent) runLoop(ctx context.Context) (string, error) {
	var finalOutput string
	loopStartTurn := a.state.Turn()

	var lastContent string
	var repeatCount int
	var lastToolSignature string
	var toolRepeatCount int

	for {
		// Inner loop: process turns until the model stops calling tools
		for a.state.Status() == StatusRunning {
			turn := a.state.NextTurn()

			if turn-loopStartTurn > a.config.MaxTurns {
				err := NewNodeError("exceeded max turns", ErrExceedMaxSteps, a.config.Name, fmt.Sprintf("turn:%d", turn))
				a.state.SetStatus(StatusError)
				a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: err})
				return "", err
			}

			// Context compaction
			if err := a.maybeCompact(ctx); err != nil {
				return a.failLoop(fmt.Sprintf("turn:%d|compaction", turn), "compaction failed", err)
			}

			// Inject steering messages before LLM call
			if steered := a.steering.Drain(); len(steered) > 0 {
				for _, msg := range steered {
					if err := a.persistMessage(ctx, msg); err != nil {
						return a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle persist steering failed", err)
					}
				}
			}

			if lc := a.lifecycle(); lc != nil {
				arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
				if err := lc.BeforeTurn(ctx, arc); err != nil {
					return a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle before_turn failed", err)
				}
			}

			if err := a.checkpointTurnStart(ctx, turn); err != nil {
				a.state.SetStatus(StatusError)
				a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: err})
				return "", err
			}

			a.emit(&TurnStartEvent{baseEvent: newBase(EventTurnStart), Turn: turn})

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

			// Lifecycle: BeforeModelCall
			if lc := a.lifecycle(); lc != nil {
				arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
				mcc := &ModelCallContext{Request: req}
				if lcErr := lc.BeforeModelCall(ctx, arc, mcc); lcErr != nil {
					return a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle before_model_call failed", lcErr)
				}
			}

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
					if errors.Is(err, context.Canceled) {
						// User interrupted — emit clean end event instead of cryptic error
						a.state.SetStatus(StatusFinished)
						a.emit(&AgentEndEvent{
							baseEvent: newBase(EventAgentEnd),
							AgentName: a.config.Name,
						})
						return "", nil
					}
					return a.failLoop(fmt.Sprintf("turn:%d|provider", turn), "provider call failed", err)
				}
			}

			// Lifecycle: AfterModelCall
			if lc := a.lifecycle(); lc != nil {
				arc := &AgentRunContext{Agent: a, Messages: a.state.Messages(), Turn: turn}
				mcc := &ModelCallContext{Request: req, Response: resp, Err: err}
				lc.AfterModelCall(ctx, arc, mcc)
				if mcc.Err != nil && err == nil {
					if !resp.SuppressPersist {
						if pErr := a.persistMessage(ctx, Message{
							Role:      RoleAssistant,
							Content:   resp.Content,
							Blocks:    resp.Blocks,
							ToolCalls: resp.ToolCalls,
						}); pErr != nil {
							return a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle persist assistant failed", pErr)
						}
					}
					if err := a.persistMessage(ctx, Message{
						Role:    RoleSystem,
						Content: fmt.Sprintf("错误: %s", mcc.Err.Error()),
					}); err != nil {
						return a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle persist guardrail error failed", err)
					}
					continue
				}
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
					return a.failLoop(fmt.Sprintf("turn:%d", turn), "lifecycle persist assistant failed", err)
				}
			}

			if len(resp.ToolCalls) == 0 {
				finalOutput = resp.Content
				a.state.SetStatus(StatusFinished)
				if err := a.endTurn(ctx, turn, resp.Usage, false); err != nil {
					return "", err
				}
				break
			}

			// Truncation guard: when the provider reports finish_reason="length" the
			// model hit max_tokens and any tool-call arguments may be cut mid-JSON.
			// The executor validates JSON per-call, but a partial batch (some calls
			// valid, some not) leaves the conversation in an inconsistent state.
			// Refuse the entire batch up front and persist error results so the
			// model regenerates with complete output.
			if resp.FinishReason == "length" && hasInvalidToolCallArgs(resp.ToolCalls) {
				for _, tc := range resp.ToolCalls {
					if perr := a.persistMessage(ctx, Message{
						Role:       RoleTool,
						Content:    "错误: 此工具调用未被执行，因为模型输出被 max_tokens 截断，生成了无效的 JSON 参数。请重新生成包含完整参数的工具调用；如果调用内容较大，请拆分或减少输出长度。",
						ToolCallID: tc.ID,
						Name:       tc.Name,
					}); perr != nil {
						return a.failLoop(fmt.Sprintf("turn:%d", turn), "truncation guard persist failed", perr)
					}
				}
				if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
					return "", err
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
					return "", nil
				}
				return a.failLoop(fmt.Sprintf("turn:%d", turn), "tool execution persist failed", err)
			}

			// Early-exit: a tool returned a terminating result; its content is
			// the final answer and no further LLM turn runs.
			if earlyExit != "" {
				finalOutput = earlyExit
				a.state.SetStatus(StatusFinished)
				if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
					return "", err
				}
				break
			}

			// Context cancellation during tool execution — exit cleanly.
			if errors.Is(ctx.Err(), context.Canceled) {
				a.state.SetStatus(StatusFinished)
				a.emit(&AgentEndEvent{
					baseEvent: newBase(EventAgentEnd),
					AgentName: a.config.Name,
				})
				return "", nil
			}
			if err := a.endTurn(ctx, turn, resp.Usage, true); err != nil {
				return "", err
			}

			// Transfer handoff
			if handoff := a.state.PendingHandoff(); handoff != nil {
				a.state.ClearPendingHandoff()
				return a.handleTransfer(ctx, handoff)
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
			// even though the text content differs each turn.
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

		// Outer loop: check for follow-up messages
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
				a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: ne})
				return "", ne
			}
		}
	}

	a.emit(&AgentEndEvent{
		baseEvent: newBase(EventAgentEnd),
		AgentName: a.config.Name,
		Output:    finalOutput,
	})
	return finalOutput, nil
}

// failLoop is the standard error exit path from the run loop.
// It wraps err in a NodeError, sets error status, emits an error event,
// and returns ("", ne). ctxTag is typically fmt.Sprintf("turn:%d", turn).
func (a *Agent) failLoop(ctxTag, description string, err error) (string, error) {
	ne := NewNodeError(description, err, a.config.Name, ctxTag)
	a.state.SetStatus(StatusError)
	a.emit(&AgentErrorEvent{baseEvent: newBase(EventAgentError), Err: ne})
	return "", ne
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

// toolCallSignature returns a stable string key for a set of tool calls,
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
	// Pre-allocate results so hooks (including deprecatedHookAdapter) can
	// pre-populate blocked tool results for per-tool blocking.
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

	// Lifecycle: AfterToolExecution (also handles deprecated
	// AfterToolCall/PostProcessResults via deprecatedHookAdapter).
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
	msgs := a.state.messagesNoClone()
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
