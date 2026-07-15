// Package graph 实现知识图谱的存储、查询和图增强检索（Graph RAG），
// 将知识库中的实体关系建模为图结构以支持多跳推理。
//
// 主要功能：
//   - 图谱存储与查询（Store/Query）
//   - 图构建（Builder）
//   - 增量更新（Incremental）
//   - 检索增强（RetrievalEnhancer）：利用图结构扩展检索结果
//   - 缓存层（Cache）
//   - 适配器（Adapter）桥接外部图数据库
//
// 主要类型：
//   - Store: 图数据持久化存储
//   - Query: 图查询接口（支持跳数和方向过滤）
//   - Builder: 从文档/三元组构建图谱
//   - RetrievalEnhancer: 图增强检索器
//   - Cache: 查询结果缓存
//
// 使用示例：
//
//	g := graph.NewStore(db)
//	g.AddEdge(ctx, "专利A", "引用", "专利B")
//	results, _ := g.Query(ctx, "专利A", 2)
package graph
