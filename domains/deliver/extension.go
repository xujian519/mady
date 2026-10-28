package deliver

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// ExtensionName 是扩展注册名。
const ExtensionName = "deliver_register"

// Extension 把交付件登记工具注入 Agent，并把登记事件发到事件总线。
type Extension struct {
	tool  *agentcore.Tool
	agent *agentcore.Agent
}

var (
	_ agentcore.Extension    = (*Extension)(nil)
	_ agentcore.ToolProvider = (*Extension)(nil)
)

// NewExtension 创建交付件登记扩展。
func NewExtension(cfg Config) *Extension {
	e := &Extension{}
	onRegistered := cfg.OnRegistered
	cfg.OnRegistered = func(d Deliverable) {
		if onRegistered != nil {
			onRegistered(d)
		}
		if e.agent != nil {
			e.agent.EmitEvent(agentcore.NewDeliverRegisteredEvent(
				d.TemplateID, d.Verdict, len(d.Manifest), d.Issues, d.Unverified))
		}
	}
	e.tool = NewTool(cfg)
	return e
}

// Name 返回扩展标识。
func (e *Extension) Name() string { return ExtensionName }

// Init 记住 Agent，供登记完成后发事件。
func (e *Extension) Init(_ context.Context, agent *agentcore.Agent) error {
	e.agent = agent
	return nil
}

// Dispose 清理扩展——当前无副作用。
func (e *Extension) Dispose() error { return nil }

// Tools 返回交付件登记工具。
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{e.tool}
}
