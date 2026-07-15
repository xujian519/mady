// Package sqlite 提供领域层的 SQLite 持久化支持，当前用于审批记录
// （ApprovalGate 日志）的存储和查询。
//
// 主要功能：
//   - 审批记录的持久化存储
//   - 按条件查询和统计
//   - 与 guardrails/ApprovalGate 集成
//
// 主要类型：
//   - ApprovalStore: 审批记录存储，支持 CRUD 和查询
//
// 使用示例：
//
//	store := sqlite.NewApprovalStore(db)
//	store.Save(ctx, approval)
//	records, _ := store.Query(ctx, "user-123", 10)
package sqlite
