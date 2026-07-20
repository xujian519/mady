package agentcore

import (
	"context"
	"errors"
	"sync"
)

// Extension is a plugin that can augment an agent with tools, hooks, and lifecycle callbacks.
type Extension interface {
	// Name returns a unique identifier for the extension.
	Name() string
	// Init is called once when the extension is registered with an agent.
	Init(ctx context.Context, agent *Agent) error
	// Dispose is called when the agent is shutting down or the extension is unloaded.
	Dispose() error
}

// ToolProvider is an optional interface extensions can implement to contribute tools.
type ToolProvider interface {
	Tools() []*Tool
}

// HookProvider is an optional interface extensions can implement to contribute hooks.
type HookProvider interface {
	BeforeHooks() []BeforeHook
	AfterHooks() []AfterHook
}

// MiddlewareProvider is an optional interface extensions can implement to contribute middleware.
type MiddlewareProvider interface {
	Middleware() []Middleware
}

// SystemPromptProvider is an optional interface extensions can implement
// to append content to the system prompt.
type SystemPromptProvider interface {
	SystemPromptSuffix() string
}

// TransformContextProvider is an optional interface extensions can implement
// to inject or rewrite messages before they are sent to the provider.
type TransformContextProvider interface {
	TransformContext(ctx context.Context, msgs []Message) []Message
}

// LifecycleProvider is an optional interface extensions can implement
// to participate in the agent execution lifecycle.
//
// For new code, consider implementing one or more of the fine-grained
// observer interfaces (AgentRunObserver, TurnObserver, ModelCallObserver,
// ToolCallObserver, MessagePersistObserver) instead. The Register function
// automatically detects them via ObserversToHook.
type LifecycleProvider interface {
	LifecycleHook() LifecycleHook
}

// EventSnapshotProvider is an optional interface extensions can implement
// to expose their current state as events for newly attached listeners.
type EventSnapshotProvider interface {
	SnapshotEvents() []Event
}

// ExtensionRegistry manages the lifecycle of extensions attached to an agent.
type ExtensionRegistry struct {
	mu         sync.RWMutex
	extensions []Extension
}

func NewExtensionRegistry() *ExtensionRegistry {
	return &ExtensionRegistry{}
}

// Register adds extensions and immediately initializes them.
//
// 若第 N 个扩展 Init 失败，前面已 Init 成功的扩展会被逆序 Dispose，
// 以释放已申请的资源（goroutine、文件句柄、订阅等），避免泄漏。
// 注意：已写入 agent.config 的 Tools/Hooks/Middleware 等不回滚
// （调用方在 Register 失败后通常丢弃该 agent）。
func (r *ExtensionRegistry) Register(ctx context.Context, agent *Agent, exts ...Extension) error {
	for _, ext := range exts {
		if err := ext.Init(ctx, agent); err != nil {
			// 逆序 Dispose 已成功 Init 的扩展，释放资源。
			r.mu.Lock()
			for i := len(r.extensions) - 1; i >= 0; i-- {
				_ = r.extensions[i].Dispose()
			}
			r.extensions = nil
			r.mu.Unlock()
			return err
		}

		agent.configMu.Lock()
		if tp, ok := ext.(ToolProvider); ok {
			agent.RegisterTools(tp.Tools()...)
		}
		if hp, ok := ext.(HookProvider); ok {
			agent.config.GlobalBefore = append(agent.config.GlobalBefore, hp.BeforeHooks()...)
			agent.config.GlobalAfter = append(agent.config.GlobalAfter, hp.AfterHooks()...)
		}
		if mp, ok := ext.(MiddlewareProvider); ok {
			agent.config.Middleware = append(agent.config.Middleware, mp.Middleware()...)
		}
		if sp, ok := ext.(SystemPromptProvider); ok {
			suffix := sp.SystemPromptSuffix()
			if suffix != "" {
				if agent.config.SystemPrompt != "" {
					agent.config.SystemPrompt += "\n\n" + suffix
				} else {
					agent.config.SystemPrompt = suffix
				}
			}
		}
		if tp, ok := ext.(TransformContextProvider); ok {
			// 注意：每个 TransformContextProvider 会包裹当前已注册的 TransformContext，
			// 形成 LIFO 嵌套链（最后注册的最外层先执行）。扩展数量多时会形成
			// 深嵌套调用栈。若需扁平执行，可改用 []func 列表顺序应用。
			prev := agent.config.TransformContext
			agent.config.TransformContext = func(ctx context.Context, msgs []Message) []Message {
				if prev != nil {
					msgs = prev(ctx, msgs)
				}
				return tp.TransformContext(ctx, msgs)
			}
		}
		if lp, ok := ext.(LifecycleProvider); ok {
			agent.config.Lifecycle = appendLifecycleHook(agent.config.Lifecycle, lp.LifecycleHook())
		}
		// Observer 接口独立检查（不因 LifecycleProvider 的实现而跳过）
		if hook := ObserversToHook(ext); hook != nil {
			agent.config.Lifecycle = appendLifecycleHook(agent.config.Lifecycle, hook)
		}
		agent.configMu.Unlock()

		r.mu.Lock()
		r.extensions = append(r.extensions, ext)
		r.mu.Unlock()
	}
	return nil
}

// AppendLifecycle 安全地将两个 LifecycleHook 组合成链。
// 若两者均为 nil 则返回 nil；任一为 nil 则返回另一个；
// 若 existing 已是 LifecycleChain 则将 next 追加到链尾，
// 否则新建 LifecycleChain 包装两者。
func AppendLifecycle(existing, next LifecycleHook) LifecycleHook {
	if next == nil {
		return existing
	}
	if existing == nil {
		return next
	}
	if chain, ok := existing.(LifecycleChain); ok {
		return append(chain, next)
	}
	return LifecycleChain{existing, next}
}

func appendLifecycleHook(existing, next LifecycleHook) LifecycleHook {
	return AppendLifecycle(existing, next)
}

// Dispose tears down all registered extensions in reverse order.
func (r *ExtensionRegistry) Dispose() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var errs []error
	for i := len(r.extensions) - 1; i >= 0; i-- {
		if err := r.extensions[i].Dispose(); err != nil {
			errs = append(errs, err)
		}
	}
	r.extensions = nil
	return errors.Join(errs...)
}

// Names returns the names of all registered extensions.
func (r *ExtensionRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.extensions))
	for i, ext := range r.extensions {
		names[i] = ext.Name()
	}
	return names
}

// Visit calls fn for the extension with the given name.
func (r *ExtensionRegistry) Visit(name string, fn func(Extension)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ext := range r.extensions {
		if ext.Name() == name {
			fn(ext)
			return
		}
	}
}

// SnapshotEvents collects current-state events from extensions that expose them.
func (r *ExtensionRegistry) SnapshotEvents() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Event
	for _, ext := range r.extensions {
		if sp, ok := ext.(EventSnapshotProvider); ok {
			out = append(out, sp.SnapshotEvents()...)
		}
	}
	return out
}
