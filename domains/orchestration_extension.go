package domains

// orchestration_extension.go 提供 OrchestrationExtension —— 用于在 Handoff 子 Agent
// 初始化时注册 run_orchestration 工具的 Extension。
//
// PatentAgentConfig 通过 cfg.Extensions 注入此扩展，使 PatentAgent（手递创建的子 Agent）
// 在其 Init 阶段获得 run_orchestration 工具。这是目前唯一需要"创建时捕获 Agent 引用"
// 的场景——run_orchestration 的 Func 闭包依赖 Agent 实例（用于构建 OrchestrationExecutor），
// 而 PatentAgentConfig 运行在 Agent.New() 之前，无法直接获取 Agent 指针。
//
// 设计模式: agentcore.Extension.Init(ctx, agent) 在 New() 过程中调用，
// 此时 agent 已创建完毕，可以安全地注册工具。这与 agentcore.ToolProvider
// 的静态注册模式互补——ToolProvider 在 Init 前获取工具列表，而本扩展利用
// Init 的参数传递 Agent 引用。

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// orchestrationExtensionName 是 OrchestrationExtension 的唯一标识。
const orchestrationExtensionName = "run_orchestration"

// OrchestrationExtension 在 Init 阶段将 run_orchestration 工具注册到 Agent。
// 与 domains/orchestration_tools.go 中的 NewOrchestrationTool 配合使用。
type OrchestrationExtension struct{}

var _ agentcore.Extension = (*OrchestrationExtension)(nil)

// Name 返回扩展标识符。
func (e *OrchestrationExtension) Name() string { return orchestrationExtensionName }

// Init 在 Agent 初始化时注册 run_orchestration 工具。
// agent 参数由 ExtensionRegistry.Register 传入，此时 Agent 已创建完毕。
func (e *OrchestrationExtension) Init(_ context.Context, agent *agentcore.Agent) error {
	agent.RegisterTools(NewOrchestrationTool(agent))
	return nil
}

// Dispose 是空操作——不持有任何外部资源。
func (e *OrchestrationExtension) Dispose() error { return nil }
