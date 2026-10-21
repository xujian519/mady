// Package graph 实现进程内知识图谱：节点/关系存储、多跳查询与图增强检索（Graph RAG）。
//
// 主要类型：
//   - GraphStore: 线程安全的邻接表存储（正向 + 反向边）
//   - GraphBuilder: 从 ParsedDoc 两阶段构建节点与关系
//   - GraphCache: 带 TTL 的查询缓存
//   - RetrievalEnhancer: 用相似案例与引用链扩展检索结果
//   - Adapter: 桥接 graph.KnowledgeGraphStore 接口
//
// 使用示例：
//
//	store := graph.NewGraphStore()
//	builder := graph.NewGraphBuilder(store)
//	builder.Build(docs)
//	paths := graph.QueryPaths(store, sourceID, targetID, 3)
package graph
