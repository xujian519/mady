package domains

import (
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/doomloop"
)

// defaultDoomLoopHook 返回领域统一的死循环检测 hook。
// 各领域 Agent 共享同一组默认阈值；如需领域定制阈值，
// 在共享 hook 之外另接 doomloop 覆盖（见 assistant.go 示例）。
func defaultDoomLoopHook() agentcore.LifecycleHook {
	return doomloop.New(
		doomloop.WithToolCallLoop(5),
		doomloop.WithTextRepetition(4),
		doomloop.WithCycleLength(2),
		doomloop.WithEmptyResultMax(5),
		doomloop.WithCircuitBreaker(100),
		doomloop.WithCompactionMax(5),
	).AsHook()
}
