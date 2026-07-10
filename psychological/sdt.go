package psychological

import (
	"math"
	"sync"
)

// SDTSignals 从用户输入提取的 SDT 需求信号
type SDTSignals struct {
	AutonomyFrustration   float64 // 0=正常, >0=自主受挫程度
	CompetenceAnxiety     float64 // 0=正常, >0=能力焦虑, <0=自信
	RelatednessLoneliness float64 // 0=正常, >0=孤独感, <0=连接感
	PerceivedDifficulty   float64 // 感知难度 (0=容易, 1=困难)
}

// SDTTracker 跨对话轮次追踪用户心理需求满足度
//
// 参考: Deterding & Guckelsberger "Advancing Self-Determination Theory
// via computational modeling" (Motivation and Emotion, 2026, 50:80-99)
//
// 核心公式:
//
//	motivation = w_a·A + w_c·C + w_r·R
//	C(t+1) = C(t) + α·(D - C(t))·(1 - e^(-β·(C(t)-D)²))
type SDTTracker struct {
	state      SDTState
	config     SDTTrackerConfig
	roundCount int
	mu         sync.RWMutex
}

// NewSDTTracker 创建 SDT 追踪器，初始状态为中等满足度 (0.5)
func NewSDTTracker(config *SDTTrackerConfig) *SDTTracker {
	cfg := DefaultSDTTrackerConfig()
	if config != nil {
		cfg = *config
	}
	t := &SDTTracker{config: cfg}
	t.state = SDTState{
		Autonomy:    0.5,
		Competence:  0.5,
		Relatedness: 0.5,
		Motivation:  t.computeMotivation(0.5, 0.5, 0.5),
	}
	return t
}

// computeMotivation 综合动机: motivation = w_a·A + w_c·C + w_r·R
func (t *SDTTracker) computeMotivation(a, c, r float64) float64 {
	w := t.config.Weights
	return clamp(w.Autonomy*a+w.Competence*c+w.Relatedness*r, 0, 1)
}

// updateCompetence 胜任感动态更新 — Deterding 2026 Eq.(1)
//
// C(t+1) = C(t) + α·(D - C(t))·(1 - e^(-β·(C(t)-D)²))
//
// 其中 D 是感知难度 (0=easy, 1=hard)
// 当难度与胜任感匹配 (差距小) 时，变化小
// 当难度略高于胜任感 (适度挑战) 时，增长最大
func (t *SDTTracker) updateCompetence(c, difficulty float64) float64 {
	alpha, beta := t.config.CompetenceAlpha, t.config.CompetenceBeta
	gap := difficulty - c
	challengeFactor := 1 - math.Exp(-beta*gap*gap)
	return clamp(c+alpha*gap*challengeFactor, 0, 1)
}

// UpdateFromSignals 应用信号更新 SDT 状态（每轮对话调用）
//
// 更新顺序:
// 1. 时间衰减 (所有需求向 0.5 回归)
// 2. 自主性更新 (基于自主受挫信号)
// 3. 胜任感更新 (基于 Deterding 2026 动态公式)
// 4. 归属感更新 (基于孤独/连接信号)
func (t *SDTTracker) UpdateFromSignals(signals SDTSignals) SDTState {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.roundCount++

	// 时间衰减: 每轮对话需求轻微向 0.5 回归
	decay := t.config.DecayRate
	A := t.state.Autonomy + decay*(0.5-t.state.Autonomy)
	C := t.state.Competence + decay*(0.5-t.state.Competence)
	R := t.state.Relatedness + decay*(0.5-t.state.Relatedness)

	// 更新自主性: 高 frust → 低 autonomy
	if signals.AutonomyFrustration > 0 {
		A = clamp(A-0.2*signals.AutonomyFrustration, 0, 1)
	}

	// 更新胜任感: 使用 Deterding 2026 的动态公式
	C = t.updateCompetence(C, signals.PerceivedDifficulty)
	if signals.CompetenceAnxiety > 0 {
		// 能力焦虑额外拉低胜任感
		C = clamp(C-0.15*signals.CompetenceAnxiety, 0, 1)
	} else if signals.CompetenceAnxiety < 0 {
		// 负焦虑 = 自信 → 提升
		C = clamp(C+0.1*math.Abs(signals.CompetenceAnxiety), 0, 1)
	}

	// 更新归属感
	if signals.RelatednessLoneliness > 0 {
		R = clamp(R-0.2*signals.RelatednessLoneliness, 0, 1)
	} else if signals.RelatednessLoneliness < 0 {
		R = clamp(R+0.2*math.Abs(signals.RelatednessLoneliness), 0, 1)
	}

	// 标记变化最大的需求
	deltaA := math.Abs(A - t.state.Autonomy)
	deltaC := math.Abs(C - t.state.Competence)
	deltaR := math.Abs(R - t.state.Relatedness)

	var lastUpdated string
	switch {
	case deltaA >= deltaC && deltaA >= deltaR && deltaA > 0.05:
		lastUpdated = "autonomy"
	case deltaC >= deltaA && deltaC >= deltaR && deltaC > 0.05:
		lastUpdated = "competence"
	case deltaR > 0.05:
		lastUpdated = "relatedness"
	}

	t.state = SDTState{
		Autonomy:        A,
		Competence:      C,
		Relatedness:     R,
		Motivation:      t.computeMotivation(A, C, R),
		LastUpdatedNeed: lastUpdated,
	}
	return t.state
}

// GetState 返回当前 SDT 状态（并发安全）
func (t *SDTTracker) GetState() SDTState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.state
}

// RestoreState 从持久化状态恢复
func (t *SDTTracker) RestoreState(state SDTState, round int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = state
	t.roundCount = round
}

// Reset 重置追踪器到初始状态
func (t *SDTTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = SDTState{
		Autonomy: 0.5, Competence: 0.5, Relatedness: 0.5,
		Motivation: t.computeMotivation(0.5, 0.5, 0.5),
	}
	t.roundCount = 0
}

// RoundCount 返回对话轮次
func (t *SDTTracker) RoundCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.roundCount
}

// LowestNeed 返回最低的心理需求（最需要关注的）
// 当所有需求 > 0.4 时返回空字符串
func (t *SDTTracker) LowestNeed() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s := t.state
	min := s.Autonomy
	if s.Competence < min {
		min = s.Competence
	}
	if s.Relatedness < min {
		min = s.Relatedness
	}
	if min > 0.4 {
		return ""
	}
	if s.Autonomy == min {
		return "autonomy"
	}
	if s.Competence == min {
		return "competence"
	}
	return "relatedness"
}
