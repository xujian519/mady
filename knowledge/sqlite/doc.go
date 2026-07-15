// Package sqlite 提供基于 SQLite 的只读知识库层，集成 FTS5 全文检索
// 和向量余弦相似度搜索，作为 lightweight 本地知识检索方案。
//
// 主要功能：
//   - FTS5 全文索引与搜索（中文分词支持）
//   - 向量存储与余弦相似度搜索
//   - 结构化字段过滤（分类/来源/日期范围）
//   - 只读查询优化（WAL 模式 + 内存映射）
//
// 主要类型：
//   - Store: SQLite 只读存储，封装 FTS5 和向量查询
//   - VectorIndex: 向量索引，支持余弦相似度 Top-K 检索
//   - Writable: 用于索引构建的可写封装（非运行时路径）
//
// 使用示例：
//
//	store, _ := sqlite.NewStore("knowledge.db")
//	results, _ := store.Search(ctx, "专利侵权判定", 10)
package sqlite
