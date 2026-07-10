package psychological

import "math"

const (
	emaCopingThreshold     = 0.35
	emaAgencySelf          = -0.3
	emaAgencyOther         = 0.3
	emaCongruenceThreshold = 0.3
)

// computeEMA 执行 EMA 四维认知评价
//
// 初级评价 (Primary Appraisal):
//   - goal_relevance: 事件对用户目标的重要性
//   - goal_congruence: 事件是否符合用户期望
//
// 次级评价 (Secondary Appraisal):
//   - coping_potential: 用户认为自己能否应对
//   - agency: 事件的原因归因
//   - future_expectancy: 对未来的预期
//
// 参考: EMA model (Gratch & Marsella)
func computeEMA(frame AppraisalFrame) EMAAssessment {
	goalRelevance := math.Abs(frame.Desirability)*0.5 + frame.Unexpectedness*0.5
	goalCongruence := frame.Desirability
	copingPotential := frame.Controllability * frame.Likelihood
	agency := frame.CausalAttribution
	futureExpectancy := frame.Desirability + frame.Likelihood - 1

	return EMAAssessment{
		GoalRelevance:    clamp(goalRelevance, 0, 1),
		GoalCongruence:   clamp(goalCongruence, -1, 1),
		CopingPotential:  clamp(copingPotential, 0, 1),
		Agency:           clamp(agency, -1, 1),
		FutureExpectancy: clamp(futureExpectancy, -1, 1),
		CopingMode:       selectCopingMode(goalCongruence, copingPotential, agency),
	}
}

// selectCopingMode 基于 EMA 模型选择应对策略
func selectCopingMode(congruence, coping, agency float64) CopingMode {
	// 接近中性的事件直接使用情绪导向（默认）
	if math.Abs(congruence) <= emaCongruenceThreshold {
		return CopeEmotionFocused
	}
	// 事件正面 + 高应对能力 → 问题导向
	if congruence > 0 && coping > emaCopingThreshold {
		return CopeProblemFocused
	}
	// 事件负面 + 高应对能力 → 认知重构
	if congruence < 0 && coping > emaCopingThreshold {
		return CopeReappraisal
	}
	// 事件负面 + 低应对能力 → 按归因方向分流
	if congruence < 0 && coping <= emaCopingThreshold {
		if agency > emaAgencyOther {
			return CopeAvoidance // 因在他人 → 回避
		}
		return CopeEmotionFocused // 因在自己/环境 → 情绪导向
	}
	return CopeEmotionFocused
}

// emaToOCCEmotions 将 EMA 评价映射到具体 OCC 情绪
// 参考: EMA emotion_map function
func emaToOCCEmotions(a EMAAssessment) map[OCCEmotion]float64 {
	result := make(map[OCCEmotion]float64)

	if a.GoalCongruence > emaCongruenceThreshold {
		result[EmoJoy] = a.GoalCongruence * a.CopingPotential
		if a.FutureExpectancy > 0 {
			result[EmoHope] = a.FutureExpectancy * a.GoalRelevance
		}
	}
	if a.GoalCongruence < -emaCongruenceThreshold {
		result[EmoDistress] = math.Abs(a.GoalCongruence) * a.GoalRelevance
		if a.FutureExpectancy < 0 {
			result[EmoFear] = math.Abs(a.FutureExpectancy) * a.GoalRelevance
		}
	}
	// 归因相关情绪 (参考 EMA: agency + congruence)
	if math.Abs(a.Agency) > emaCongruenceThreshold {
		if a.GoalCongruence > 0 && a.Agency < emaAgencySelf {
			result[EmoPride] = a.GoalCongruence
		}
		if a.GoalCongruence < 0 && a.Agency > emaAgencyOther {
			result[EmoAnger] = math.Abs(a.GoalCongruence)
		}
		if a.GoalCongruence < 0 && a.Agency < emaAgencySelf {
			result[EmoGuilt] = math.Abs(a.GoalCongruence) * 0.5
		}
	}
	return result
}
