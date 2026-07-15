// Package agentcore 提供 Agent 核心运行时，包括 LLM-工具循环、事件系统、
// 生命周期钩子、上下文引擎、以及 Handoff 委托机制。
//
// 核心职责：
//   - Agent 生命周期管理（Run/Continue/Resume/Persist）
//   - 消息压缩与结构化压缩（Compaction）
//   - 预算控制与检查点（Budget/Checkpoint）
//   - 上下文构建（ContextBuilder）和 Block 管理
//   - 工具调用编排与证据账本集成
//
// 主要类型：
//   - Agent: Agent 运行时，驱动 LLM-工具交互循环
//   - Config: 运行时配置（Provider、Model、Budget 等）
//   - EventBus: 类型安全事件发布-订阅
//   - LifecycleHook: Agent 执行阶段拦截器
//   - HandoffConfig: 子 Agent 委派配置（Invisible Handoff 支持）
//   - ContextBuilder: 消息历史构建策略
//
// 使用示例：
//
//	a := agentcore.New(cfg)
//	result, err := a.Run(ctx, "分析这件专利的独立权利要求")
package agentcore
