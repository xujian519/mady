package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// HandoffMode determines how control is transferred to a target agent.
type HandoffMode string

const (
	// HandoffDelegate runs the target agent as a sub-task and returns its output
	// as a tool result back to the calling agent. The calling agent continues.
	HandoffDelegate HandoffMode = "delegate"

	// HandoffTransfer fully transfers the conversation to the target agent.
	// The calling agent stops and the target agent takes over.
	HandoffTransfer HandoffMode = "transfer"
)

// HandoffConfig describes a sub-agent that the current agent can hand off to.
type HandoffConfig struct {
	Name        string
	Description string // shown to the LLM so it can decide when to hand off
	Mode        HandoffMode
	AgentConfig Config

	// AllowedSources 限制哪些 Agent 可以交接到此目标。
	// 为空或包含 "*" 时表示不限制。仅在 createHandoffTool 的 Func 中运行时校验。
	AllowedSources []string

	// FallbackMsg 是交接失败或校验不通过时展示给用户的兜底文案。
	// 为空时使用默认文案。
	FallbackMsg string

	// Invisible 为 true 时，交接过程在 UI 中不可见：
	//   - 子 Agent 的事件不会转发到事件总线
	//   - HandoffStartEvent/HandoffEndEvent 标记为不可见，供 UI 层静默处理
	// 用户看不到"切换 Agent"的痕迹，适合 Chat Agent 内部路由场景。
	Invisible bool
}

// PendingHandoff is set on state when a transfer-mode handoff tool is called.
type PendingHandoff struct {
	TargetName     string
	TargetConfig   Config
	Context        string
	Invisible      bool
	AllowedSources []string
}

// registerHandoffs creates a synthetic tool for each configured handoff target.
func (a *Agent) registerHandoffs() {
	for _, h := range a.config.Handoffs {
		a.registry.Register(a.createHandoffTool(h))
	}
}

func (a *Agent) createHandoffTool(h HandoffConfig) *Tool {
	return &Tool{
		Name:        "transfer_to_" + h.Name,
		Description: fmt.Sprintf("Hand off to %s. %s", h.Name, h.Description),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "Context or instructions for the target agent",
				},
			},
			"required":             []string{"message"},
			"additionalProperties": false,
		},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var p struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return nil, err
			}

			// 白名单校验：确保来源 Agent 在目标的允许列表中。
			if !a.isHandoffAllowed(h) {
				fallback := h.FallbackMsg
				if fallback == "" {
					fallback = fmt.Sprintf("该功能（%s）暂不可用，请稍后重试。", h.Name)
				}
				return NewFailureResult("交接被拦截", fallback), nil
			}

			switch h.Mode {
			case HandoffDelegate:
				return a.executeDelegate(ctx, h, p.Message)
			case HandoffTransfer:
				a.state.SetPendingHandoff(&PendingHandoff{
					TargetName:     h.Name,
					TargetConfig:   h.AgentConfig,
					Context:        p.Message,
					Invisible:      h.Invisible,
					AllowedSources: h.AllowedSources,
				})
				return map[string]string{"status": "transferring to " + h.Name}, nil
			default:
				return nil, fmt.Errorf("unknown handoff mode: %s", h.Mode)
			}
		},
	}
}

// executeDelegate creates a sub-agent, runs it, and returns its output as a tool result.
func (a *Agent) executeDelegate(ctx context.Context, h HandoffConfig, input string) (any, error) {
	start := time.Now()

	// 构建结构化交接上下文，减少 token 消耗并提升交接质量。
	hc := a.ExtractHandoffContext(h.Name, 6)
	hcJSON, marshalErr := json.Marshal(hc)
	if marshalErr != nil {
		hcJSON = []byte("{}")
	}

	a.emit(&HandoffStartEvent{
		baseEvent:   newBase(EventHandoffStart),
		SourceAgent: a.config.Name,
		TargetAgent: h.Name,
		Mode:        string(HandoffDelegate),
		Context:     string(hcJSON),
		Invisible:   h.Invisible,
	})

	// 将原始用户消息和结构化 HandoffContext 合并传给子 Agent。
	// 子 Agent 的 System Prompt 可据此解析上下文。
	enrichedInput := fmt.Sprintf("【交接上下文】\n%s\n\n【用户消息】\n%s", string(hcJSON), input)

	sub := New(h.AgentConfig)
	if !h.Invisible {
		sub.SetEventBus(a.eventBus) // 可见交接才转发子 Agent 事件
	}
	defer sub.Close()

	output, err := sub.Run(ctx, enrichedInput)

	a.emit(&HandoffEndEvent{
		baseEvent:   newBase(EventHandoffEnd),
		TargetAgent: h.Name,
		Output:      output,
		Duration:    time.Since(start),
		Err:         err,
		Invisible:   h.Invisible,
	})

	if err != nil {
		// 返回 HandoffResult 而不是裸错误，让调用方 Agent 能做优雅降级。
		fallback := h.FallbackMsg
		if fallback == "" {
			fallback = "这个任务处理遇到点问题，要不换个方式再说一遍，或稍后再试？"
		}
		return NewFailureResult("执行失败", fallback), nil
	}

	// 尝试将输出解析为 HandoffResult，支持结构化和纯文本两种模式。
	if hr, ok := ParseHandoffResult(output); ok {
		hr.RawOutput = output
		return hr, nil
	}
	// 回退：纯文本输出包装为 HandoffResult
	return NewHandoffResult("执行完成", output), nil
}

// handleTransfer creates a target agent, inherits the conversation and runtime
// state from the source agent, and transfers control.
func (a *Agent) handleTransfer(ctx context.Context, handoff *PendingHandoff) (string, error) {
	// Belt-and-suspenders: re-check that the handoff is still allowed.
	if !a.isHandoffAllowed(HandoffConfig{
		Name:           handoff.TargetName,
		AgentConfig:    handoff.TargetConfig,
		AllowedSources: handoff.AllowedSources,
	}) {
		return "", fmt.Errorf("handoff to %s is not allowed (re-check)", handoff.TargetName)
	}
	start := time.Now()

	// 构建结构化交接上下文
	hc := a.ExtractHandoffContext(handoff.TargetName, 6)
	hcJSON, marshalErr := json.Marshal(hc)
	if marshalErr != nil {
		hcJSON = []byte("{}")
	}

	a.emit(&HandoffStartEvent{
		baseEvent:   newBase(EventHandoffStart),
		SourceAgent: a.config.Name,
		TargetAgent: handoff.TargetName,
		Mode:        string(HandoffTransfer),
		Context:     string(hcJSON),
		Invisible:   handoff.Invisible,
	})

	target := New(handoff.TargetConfig)
	if !handoff.Invisible {
		target.SetEventBus(a.eventBus)
	}
	defer target.Close()

	// Inherit runtime state from the source agent.
	a.inheritRuntime(target)

	// Inherit conversation: replace source system prompt with target's, keep the rest.
	if handoff.TargetConfig.SystemPrompt != "" {
		if err := target.persistMessage(ctx, Message{Role: RoleSystem, Content: handoff.TargetConfig.SystemPrompt}); err != nil {
			return "", err
		}
	}
	for _, msg := range a.state.Messages() {
		if msg.Role == RoleSystem {
			continue
		}
		if err := target.persistMessage(ctx, msg); err != nil {
			return "", err
		}
	}

	output, err := target.Continue(ctx)

	a.emit(&HandoffEndEvent{
		baseEvent:   newBase(EventHandoffEnd),
		TargetAgent: handoff.TargetName,
		Output:      output,
		Duration:    time.Since(start),
		Err:         err,
		Invisible:   handoff.Invisible,
	})

	a.state.SetStatus(StatusFinished)
	return output, err
}

// inheritRuntime copies the source agent's tools, extensions, and config-level
// runtime state onto the target agent.
func (a *Agent) inheritRuntime(target *Agent) {
	// Copy tools from source to target (excluding handoff tools).
	for _, t := range a.registry.Tools() {
		if isHandoffTool(t.Name) {
			continue
		}
		target.registry.Register(t)
	}

	// Snapshot source config under read lock to avoid data race with concurrent
	// config writes (ApplyCallConfig, SetThinkingConfig, etc.).
	a.configMu.RLock()
	srcMiddleware := a.config.Middleware
	srcGlobalBefore := a.config.GlobalBefore
	srcGlobalAfter := a.config.GlobalAfter
	srcLifecycle := a.config.Lifecycle
	srcTransformCtx := a.config.TransformContext
	srcExtensions := a.config.Extensions
	a.configMu.RUnlock()

	// Re-register source extensions on target.
	if len(srcExtensions) > 0 {
		if err := target.extensions.Register(context.Background(), target, srcExtensions...); err != nil {
			slog.Default().Warn("inheritRuntime: extensions.Register failed", "agent", a.config.Name, "error", err)
		}
	}

	// Merge config-level runtime state.
	target.configMu.Lock()
	defer target.configMu.Unlock()

	if len(srcMiddleware) > 0 {
		target.config.Middleware = append(target.config.Middleware, srcMiddleware...)
	}
	if len(srcGlobalBefore) > 0 {
		target.config.GlobalBefore = append(target.config.GlobalBefore, srcGlobalBefore...)
	}
	if len(srcGlobalAfter) > 0 {
		target.config.GlobalAfter = append(target.config.GlobalAfter, srcGlobalAfter...)
	}
	if srcLifecycle != nil {
		target.config.Lifecycle = appendLifecycleHook(target.config.Lifecycle, srcLifecycle)
	}
	if srcTransformCtx != nil {
		prev := target.config.TransformContext
		// Copy the function pointer, not the source Agent pointer, to avoid
		// retaining the source Agent after it is closed/GC'd.
		fn := srcTransformCtx
		target.config.TransformContext = func(ctx context.Context, msgs []Message) []Message {
			if prev != nil {
				msgs = prev(ctx, msgs)
			}
			return fn(ctx, msgs)
		}
	}
}

func isHandoffTool(name string) bool {
	return strings.HasPrefix(name, "transfer_to_")
}

// isHandoffAllowed checks whether the source agent is in the target's
// AllowedSources whitelist.
// - Empty AllowedSources → DENY (default-deny; no "*" means locked down).
// - "*" in AllowedSources → allow any source.
// - Otherwise, allow if a.config.Name matches an entry.
func (a *Agent) isHandoffAllowed(h HandoffConfig) bool {
	if len(h.AllowedSources) == 0 {
		return false
	}
	for _, src := range h.AllowedSources {
		if src == "*" || src == a.config.Name {
			return true
		}
	}
	return false
}
