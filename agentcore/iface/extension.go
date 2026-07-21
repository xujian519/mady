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

// 追踪接口定义已迁移到 agentcore/tracer.go。
// 本包不重复定义 Tracer/Span/SpanAttribute。
// 如需使用追踪功能，请通过 agentcore.NewTracerAdapter() 或直接引用 agentcore.Tracer。

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
