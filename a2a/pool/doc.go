// Package pool 提供 Agent 健康检查心跳池，用于维护 A2A 联邦网络中
// Agent 节点的活跃状态。
//
// Pool 定期对已注册的 Agent 执行健康检查（默认 GET /health），
// 连续失败达到阈值（默认 3 次）的 Agent 会自动从活跃列表摘除。
//
// 使用示例：
//
//	pool := pool.New(pool.DefaultCheckFunc).
//	    WithInterval(30 * time.Second)
//
//	pool.Join(&registry.Registration{
//	    Name: "patent-agent",
//	    URL:  "http://localhost:8080",
//	})
//
//	pool.Start(ctx)
//	defer pool.Stop()
//
//	// 获取当前存活的 Agent
//	alive := pool.Alive()
package pool
