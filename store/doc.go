// Package store provides common storage interfaces and implementations
// for the Mady platform.
//
// 本包包含：
//   - SnapshotStore: file-based agent state persistence (agentcore.Store)
//   - CaseStore: unified interface for case-scoped storage (checkpoint/memory/approval)
//   - VectorStore: unified interface for vector search (knowledge + memory)
//   - DocumentStore: unified interface for document storage (knowledge)
//   - GraphStore: unified interface for knowledge graph storage
//   - Closer: resource cleanup interface for stores holding connections
//
// 背景：Mady 项目中存在多个存储系统（knowledge/、memory/、retrieval/），
// 各自有独立的 SQLite 管理、嵌入序列化、RRF 融合实现。
// 本包通过统一接口抽象消除重复，使各存储系统可以共享同一套底层抽象。
package store
