// Package sqlite 提供基于 SQLite 的推理工作流检查点持久化，
// 实现 reasoning.CheckpointStore（Save/Load/Delete/ListByCase）。
// 推理中间状态以 JSON blob 存于 stage_checkpoints 表，附带索引元数据列
// （case_id / case_type / current_stage）以支持按案件级查询。
//
// 本包是 domains/reasoning 与 store.CaseStore/Closer 之间的基础设施适配层：
// 把具体 SQLite 后端接到接口上，而不污染领域包的 import。
//
// 使用示例：
//
//	store, err := sqlite.NewCheckpointStore("reasoning.db")
//	if err != nil {
//		return err
//	}
//	defer store.Close()
//
//	cp := reasoning.StageCheckpoint{...}
//	if err := store.Save(ctx, &cp); err != nil {
//		return err
//	}
package sqlite
