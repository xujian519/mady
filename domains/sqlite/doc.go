// Package sqlite 提供领域层的 SQLite 持久化支持：审批记录（ApprovalGate
// 日志）、图级检查点与 Agent 生命周期事件。
//
// 主要功能：
//   - 审批记录的持久化存储与按会话/状态查询
//   - 图级检查点的保存与加载
//   - 生命周期事件写入、按运行查询与过期清理
//   - 与 guardrails/ApprovalGate 集成
//
// 主要类型：
//   - ApprovalStore: 审批记录与待审批项存储
//   - GraphCheckpointStore: 图级检查点存储
//   - EventStore: 生命周期事件存储
//
// 使用示例：
//
//	store, err := sqlite.NewApprovalStore(dbPath)
//	if err != nil {
//		return err
//	}
//	records, err := store.List(ctx, sessionID)
package sqlite
