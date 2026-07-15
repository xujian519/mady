// Package graph 提供图引擎基础设施，支持 DAG（有向无环图）编排和
// Pregel 分布式图处理范式，是 disclosure 分析管线和 workflow 系统的底层引擎。
//
// 核心概念：
//   - Graph: DAG 节点-边结构，支持拓扑排序和条件分支
//   - Pregel: 顶点编程模型，以 SuperStep 为单位的迭代计算
//   - State: 图执行状态的读取/写入接口
//   - Checkpoint: 执行到一半的图可序列化恢复
//   - Stream: Node 输出的事件流式处理
//
// 主要类型：
//   - Graph: 带条件边的 DAG 编排引擎
//   - PregelGraph: Pregel 分布式计算图
//   - State: 键值状态管理（可快照/恢复）
//   - Checkpoint: 执行断点持久化
//
// 使用示例：
//
//	g := graph.New(graph.WithPregel())
//	g.AddNode("parse", parseFn)
//	g.AddEdge("parse", "analyze")
//	result, _ := g.Run(ctx, input)
package graph
