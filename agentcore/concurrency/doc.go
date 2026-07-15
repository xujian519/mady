// Package concurrency 提供泛型并发控制工具，包括类型安全的 Worker Pool，
// 支持任务提交、结果收集和优雅关闭。
//
// 主要类型：
//   - Pool: 泛型 Worker Pool，管理 goroutine 生命周期
//   - Config: Pool 配置（Worker 数量、队列大小等）
//
// 使用示例：
//
//	pool := concurrency.NewPool[string, int](concurrency.Config{
//	    MinWorkers: 4,
//	    MaxWorkers: 10,
//	})
//	pool.Submit("task-1", func(ctx context.Context) (int, error) { ... })
//	result, _ := pool.Wait(ctx)
package concurrency
