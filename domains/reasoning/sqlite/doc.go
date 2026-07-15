// Package sqlite 提供基于 SQLite 的推理工作流检查点存储，用于持久化
// Syllogism 引擎和 ReasoningWalker 的推理中间状态。
//
// 主要功能：
//   - 检查点写入与恢复
//   - 推理步骤的增量持久化
//   - 并发安全的读写访问
//
// 主要类型：
//   - CheckpointStore: 检查点存储，支持 Save/Load/List/Delete
//
// 使用示例：
//
//	store := sqlite.NewCheckpointStore("reasoning.db")
//	store.Save(ctx, "case-123", step, state)
package sqlite
