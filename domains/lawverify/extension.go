package lawverify

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是扩展注册名。
const ExtensionName = "law_verify"

// Extension 把 law_verify 工具注入 Agent。
type Extension struct {
	tool *agentcore.Tool
}

var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

// NewExtension 创建法条核验扩展。
func NewExtension(cfg Config) *Extension {
	return &Extension{tool: NewTool(cfg)}
}

// Name 返回扩展标识。
func (e *Extension) Name() string { return ExtensionName }

// Init 初始化扩展——当前无副作用。
func (e *Extension) Init(_ context.Context, _ *agentcore.Agent) error { return nil }

// Dispose 清理扩展——当前无副作用。
func (e *Extension) Dispose() error { return nil }

// Tools 返回法条核验工具。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{e.tool}
}
