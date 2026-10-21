// Package sqlite 提供基于 SQLite 的知识库访问层：只读 knowledge.db
// （FTS5 全文检索 + 余弦向量检索）与用户可写的 user.db。
//
// 主要类型：
//   - SQLiteStore: 只读知识库（FTSSearch / VectorSearch / GetChunksByDocID）
//   - WritableStore: 用户文档库（分块、嵌入、FTS + 向量检索）
//   - VectorIndex: 进程内向量索引，避免每次查询全表扫描
//
// 使用示例：
//
//	store, err := sqlite.NewSQLiteStore(knowledgeDBPath)
//	chunks, err := store.FTSSearch("专利侵权判定", 10)
package sqlite
