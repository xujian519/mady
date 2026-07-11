package compiler

import (
	"math/rand"
	"strings"
	"time"
)

// Strategy records the historical performance of an execution approach.
type Strategy struct {
	ID            string    `json:"id"`
	Description   string    `json:"description"`
	Preconditions []string  `json:"preconditions"` // keywords that trigger this strategy
	Guidance      string    `json:"guidance"`      // compiled advice for the agent
	Successes     int       `json:"successes"`
	Failures      int       `json:"failures"`
	LastUsedAt    time.Time `json:"last_used_at"`
	CreatedAt     time.Time `json:"created_at"`
}

// Samples returns total number of uses.
func (s Strategy) Samples() int { return s.Successes + s.Failures }

// SuccessRate returns the success ratio (0-1). Returns 0.5 for untested strategies
// (optimistic prior to encourage exploration).
func (s Strategy) SuccessRate() float64 {
	if s.Samples() == 0 {
		return 0.5 // optimistic prior
	}
	return float64(s.Successes) / float64(s.Samples())
}

// MatchesGoal checks whether any precondition keyword appears in the goal.
func (s Strategy) MatchesGoal(goal string) bool {
	goalLower := strings.ToLower(goal)
	for _, cond := range s.Preconditions {
		if strings.Contains(goalLower, strings.ToLower(cond)) {
			return true
		}
	}
	return false
}

// StrategyPick is the result of strategy selection.
type StrategyPick struct {
	Strategy *Strategy
	Explored bool   // true if this was an exploration pick
	Reason   string // human-readable explanation
}

// SelectStrategy picks the best strategy for a goal using ε-greedy.
//
//	With probability explorationRate/100, it picks a random matching strategy (exploration).
//	Otherwise, it picks the highest success-rate matching strategy (exploitation).
//
// If no strategies match, returns nil.
func SelectStrategy(goal string, strategies []Strategy, explorationRate int, rng *rand.Rand) StrategyPick {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	// Filter matching strategies
	var matching []int
	for i, s := range strategies {
		if s.MatchesGoal(goal) {
			matching = append(matching, i)
		}
	}
	if len(matching) == 0 {
		return StrategyPick{}
	}

	// ε-greedy: explore with probability explorationRate%
	if explorationRate > 0 && rng.Intn(100) < explorationRate {
		idx := matching[rng.Intn(len(matching))]
		return StrategyPick{
			Strategy: &strategies[idx],
			Explored: true,
			Reason:   "exploration: 随机选择策略以探索新路径",
		}
	}

	// Exploitation: pick highest success rate
	bestIdx := matching[0]
	bestRate := strategies[bestIdx].SuccessRate()
	for _, idx := range matching[1:] {
		rate := strategies[idx].SuccessRate()
		if rate > bestRate || (rate == bestRate && strategies[idx].Samples() > strategies[bestIdx].Samples()) {
			bestRate = rate
			bestIdx = idx
		}
	}
	return StrategyPick{
		Strategy: &strategies[bestIdx],
		Explored: false,
		Reason:   "exploitation: 选择成功率最高的策略",
	}
}

// DefaultStrategies returns preset strategies for the patent/legal domain.
func DefaultStrategies() []Strategy {
	now := time.Now()
	return []Strategy{
		{
			ID:            "oa_three_step",
			Description:   "审查意见三步法答复策略",
			Preconditions: []string{"审查意见", "答复", "OA", "office action"},
			Guidance:      "使用三步法：(1)理解审查员论点 (2)逐一反驳 (3)提出修改建议",
			CreatedAt:     now,
		},
		{
			ID:            "claim_split",
			Description:   "权利要求拆分分析",
			Preconditions: []string{"权利要求", "拆分", "技术特征", "claim"},
			Guidance:      "将独立权利要求拆解为技术特征，逐一分析新颖性和创造性",
			CreatedAt:     now,
		},
		{
			ID:            "invalidity_search",
			Description:   "无效检索策略",
			Preconditions: []string{"无效", "检索", "对比文件", "invalidity"},
			Guidance:      "构建检索式：关键词 + IPC分类 + 引证追踪，优先检索近5年文献",
			CreatedAt:     now,
		},
		{
			ID:            "patent_draft",
			Description:   "专利撰写流程",
			Preconditions: []string{"撰写", "说明书", "交底书", "draft"},
			Guidance:      "按交底书→权利要求→说明书→摘要顺序撰写，确保权利要求有说明书支持",
			CreatedAt:     now,
		},
		{
			ID:            "legal_analysis",
			Description:   "法律分析框架",
			Preconditions: []string{"法律", "分析", "合规", "legal"},
			Guidance:      "按事实认定→法律适用→结论建议三段式分析，附置信度标注",
			CreatedAt:     now,
		},
	}
}
