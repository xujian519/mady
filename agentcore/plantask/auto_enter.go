package plantask

import (
	"context"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// ============================================================================
// 自动进入规划态（03-design §1.3 / 02-spec §N4）
//
// 触发链：Gateway.BeforeModelCall 分类为 High → OnHighComplexity 回调 →
// AutoEnterPlanning（按用户输入 turn 去重计数，连续 N 轮触发）。
// ============================================================================

// AutoEnterConfig 配置自动进入规划态的触发门槛。
type AutoEnterConfig struct {
	// Rounds 是连续 High 用户输入轮数（默认 2）；<=0 关闭自动进入。
	Rounds int
}

// autoEnterState 保存连续 High 计数（Gateway 回调可能并发，需加锁）。
type autoEnterState struct {
	mu          sync.Mutex
	lastTurn    int64
	consecutive int
}

// AutoEnterPlanning 是 agentcore.Gateway.OnHighComplexity 的实现。
//
// 轮 = 用户输入 turn（GatewayDecision.Turn 粒度，同一用户输入内多次模型
// 调用去重）；turn 出现缺口（跳号）视为中间存在非 High 轮，计数清零。
// 达到门槛且门控未激活、无活动会话时：创建 Planning 会话并激活 planmode。
func (e *Extension) AutoEnterPlanning(_ *agentcore.AgentRunContext, d agentcore.GatewayDecision) {
	if e.cfg.AutoEnter.Rounds <= 0 {
		return
	}
	// 计数去重与缺口复位。
	e.autoEnter.mu.Lock()
	reached := false
	if d.Turn != e.autoEnter.lastTurn {
		if e.autoEnter.lastTurn != 0 && d.Turn > e.autoEnter.lastTurn+1 {
			e.autoEnter.consecutive = 0
		}
		e.autoEnter.lastTurn = d.Turn
		e.autoEnter.consecutive++
		reached = e.autoEnter.consecutive >= e.cfg.AutoEnter.Rounds
		if reached {
			e.autoEnter.consecutive = 0 // 达标后重新起计数
		}
	}
	e.autoEnter.mu.Unlock()
	if !reached {
		return
	}

	// 门控已激活（用户手动 /plan 或已有规划态）→ 跳过。
	if e.gateActive() {
		return
	}
	// 已有活动会话 → 跳过（不重复创建）。
	if e.hasActiveSession(context.Background()) {
		return
	}

	// 创建 Planning 会话并激活门控（只读态）。
	s := NewSession(e.sessionID("auto"), "auto", "auto")
	if err := e.cfg.Store.Save(context.Background(), s); err != nil {
		return
	}
	e.cfg.Gate.Activate()
}

// hasActiveSession 报告是否存在未终态会话（auto-enter 防重复）。
func (e *Extension) hasActiveSession(ctx context.Context) bool {
	pending, err := e.cfg.Store.ListPending(ctx)
	if err != nil {
		return false
	}
	for _, s := range pending {
		if s.Status == StatusPlanning || s.Status == StatusAwaitingApproval ||
			s.Status == StatusExecuting || s.Status == StatusAwaitingFeedback ||
			s.Status == StatusReplanning {
			return true
		}
	}
	return false
}
