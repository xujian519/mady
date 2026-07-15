// Package workflow 提供工作流编排原语，包括 Pipeline（顺序执行）、
// Parallel（并行执行）和 Router（条件路由），支持构建复杂的任务执行管线。
//
// 核心原语：
//   - Pipeline: 将多个步骤串行编排，前一步输出作为后一步输入
//   - Parallel: 并行执行多个步骤，等待全部完成后聚合结果
//   - Router: 基于条件/分类将任务路由到不同的处理分支
//
// 主要类型：
//   - Pipeline: 顺序执行管线
//   - Per (Parallel): 并行执行器
//   - Router: 条件路由分发器
//   - Workflow: 统一工作流抽象（组合 Pipeline/Parallel/Router）
//
// 使用示例：
//
//	pipe := workflow.NewPipeline(steps...)
//	result, _ := pipe.Execute(ctx, input)
package workflow
