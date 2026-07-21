package iface

import "context"

// =============================================================================
// 扩展系统
// =============================================================================

// Extension 是插件的核心接口。
// 实现者可以注入工具、钩子和生命周期回调到 Agent 中。
type Extension interface {
	Name() string
	Init(runner AgentRunner) error
	Dispose() error
}

// ExtensionInfo 是扩展注册时的简略信息。
type ExtensionInfo struct {
	Name string
}

// =============================================================================
// 追踪接口
// =============================================================================

// Span 表示一次追踪工作单元。
type Span interface {
	End()
	RecordError(err error)
	AddEvent(name string, attrs ...SpanAttribute)
}

// SpanAttribute 是附加到 Span 的键值对。
type SpanAttribute struct {
	Key   string
	Value any
}

// Tracer 创建追踪 span。
type Tracer interface {
	Start(ctx context.Context, name string, attrs ...SpanAttribute) (context.Context, Span)
}

// NoopTracer 返回丢弃所有追踪数据的空 tracer。
func NoopTracer() Tracer {
	return noopTracer{}
}

type noopTracer struct{}
type noopSpan struct{}

func (noopTracer) Start(ctx context.Context, _ string, _ ...SpanAttribute) (context.Context, Span) {
	return ctx, noopSpan{}
}
func (noopSpan) End()                                  {}
func (noopSpan) RecordError(_ error)                   {}
func (noopSpan) AddEvent(_ string, _ ...SpanAttribute) {}

// =============================================================================
// 工具接口
// =============================================================================

// ToolInfo 是工具的元信息，供消费方发现和选择工具。
type ToolInfo struct {
	Name        string
	Description string
	Categories  []string
}

// ToolProvider 是扩展提供工具的接口。
type ToolProvider interface {
	Tools() []ToolInfo
}

// ContextProvider 是扩展提供上下文的接口。
type ContextProvider interface {
	Provide(ctx context.Context) (role string, content string, err error)
}

// =============================================================================
// 存储接口
// =============================================================================

// Store 是 agent 状态快照的持久化接口。
type Store interface {
	Save(ctx context.Context, key string, data []byte) error
	Load(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context) ([]string, error)
	Has(ctx context.Context, key string) (bool, error)
}
