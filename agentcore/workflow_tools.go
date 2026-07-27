package agentcore

import "context"

// WorkflowTools bundles all workflow execution tools into a single Extension
// for registration on any Agent config. This replaces the pattern of manually
// registering run_orchestration and run_plugin separately.
//
// Usage:
//
//	toolExt, err := NewWorkflowToolsExtension(agent, orcExec)
//	cfg.Extensions = append(cfg.Extensions, toolExt)
//
// Or with a PluginManager:
//
//	toolExt, err := NewWorkflowToolsExtension(agent, orcExec, WithPluginManager(pm))
//	cfg.Extensions = append(cfg.Extensions, toolExt)
type WorkflowTools struct {
	orchestrationTool *Tool
	pluginTool        *Tool
}

// WorkflowToolsOption configures a WorkflowTools instance.
type WorkflowToolsOption func(*WorkflowTools)

// WithPluginManager attaches a PluginManager to provide the run_plugin tool.
func WithPluginManager(pm *PluginManager) WorkflowToolsOption {
	return func(wt *WorkflowTools) {
		if pm != nil {
			wt.pluginTool = pm.RunPluginTool()
		}
	}
}

// NewWorkflowToolsExtension creates an Extension that provides the
// run_orchestration tool (required) and optionally the run_plugin tool.
//
// The orchestrationTool parameter is typically created by
// domains.NewOrchestrationTool(agent). Pass nil to create an extension
// without the orchestration tool.
func NewWorkflowToolsExtension(orchTool *Tool, opts ...WorkflowToolsOption) Extension {
	wt := &WorkflowTools{
		orchestrationTool: orchTool,
	}
	for _, opt := range opts {
		opt(wt)
	}
	return wt
}

// Name returns the extension identifier.
func (wt *WorkflowTools) Name() string { return "workflow-tools" }

// Init is a no-op — the WorkflowTools extension requires no initialization.
func (wt *WorkflowTools) Init(_ context.Context, _ *Agent) error { return nil }

// Dispose is a no-op.
func (wt *WorkflowTools) Dispose() error { return nil }

// Tools returns the workflow execution tools provided by this extension.
func (wt *WorkflowTools) Tools() []*Tool {
	var tools []*Tool
	if wt.orchestrationTool != nil {
		tools = append(tools, wt.orchestrationTool)
	}
	if wt.pluginTool != nil {
		tools = append(tools, wt.pluginTool)
	}
	return tools
}
