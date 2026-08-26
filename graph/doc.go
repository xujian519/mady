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
// 使用示例（DAG）：
//
//	g := graph.NewGraph()
//	_ = g.AddNode("parse", parseStep)
//	_ = g.AddNode("analyze", analyzeStep)
//	_ = g.AddEdge("parse", "analyze")
//	cg, _ := g.Compile(graph.CompileOptions{EntryNode: "parse"})
//	result, _ := cg.Run(ctx, input)
//
// 使用示例（Pregel）：
//
//	pg := graph.NewPregelGraph()
//	_ = pg.AddNode("parse", parseFn)
//	_ = pg.AddEdge("parse", "analyze")
//	cpg, _ := pg.Compile("parse")
//	state, _ := cpg.Run(ctx, graph.PregelState{"input": "..."})
package graph
