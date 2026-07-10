package psychological

import "fmt"

// StrategyInput 策略匹配的输入信号，聚合管道所有前置阶段的输出
type StrategyInput struct {
	VAD             VADVector
	Dominant        OCCEmotion
	Intensities     map[OCCEmotion]float64
	SDT             SDTState
	EMA             EMAAssessment
	Distortions     []CognitiveDistortion
	DistortionCount int
	BeliefIntensity float64
}

// strategyCondition 策略与匹配条件的定义
type strategyCondition struct {
	Strategy       DialogueStrategy
	Conditions     []conditionRule
	PromptTemplate string
}

type conditionRule struct {
	Name   string
	Weight float64
	Match  func(StrategyInput) float64
}

// allStrategies 定义全部 9 种对话策略的匹配条件
// 参考: MultiAgentESC (EMNLP 2025) Strategy Selection Agent 逻辑
var allStrategies = []strategyCondition{
	{
		// == Validation (共情验证) — Rogers 人本主义 ==
		// 适用: 高强度负面情绪、高认知扭曲
		Strategy: StrategyValidation,
		Conditions: []conditionRule{
			{"negative_high_arousal", 2.0, func(i StrategyInput) float64 {
				return max(0.0, -i.VAD.Valence) * i.VAD.Arousal
			}},
			{"high_distortion", 1.0, func(i StrategyInput) float64 {
				return clamp(float64(i.DistortionCount)*0.2+i.BeliefIntensity*0.5, 0, 1)
			}},
			{"low_competence", 0.5, func(i StrategyInput) float64 {
				return max(0.0, 0.5-i.SDT.Competence)
			}},
		},
		PromptTemplate: "先共情验证用户的感受，不急于分析或给建议。用反射(reflection)确认你理解了他的情绪。",
	},
	{
		// == Reframing (认知重构) — Beck CBT ==
		// 适用: 检测到认知扭曲
		Strategy: StrategyReframing,
		Conditions: []conditionRule{
			{"distortion_present", 2.0, func(i StrategyInput) float64 {
				return clamp(float64(i.DistortionCount)*0.3, 0, 1)
			}},
			{"high_belief_intensity", 1.5, func(i StrategyInput) float64 {
				return i.BeliefIntensity
			}},
			{"ema_reappraisal_mode", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeReappraisal {
					return 0.7
				}
				return 0
			}},
		},
		PromptTemplate: "温和地提出另一种视角。不要否定用户的感受：先用「我理解你为什么会这么想」开头，再提出「有没有另一种可能性？」。聚焦于认知扭曲，避免贴标签。",
	},
	{
		// == Empowerment (赋能引导) — Bandura 自我效能 ==
		// 适用: 低胜任感 + 低自主性
		Strategy: StrategyEmpowerment,
		Conditions: []conditionRule{
			{"low_autonomy", 1.5, func(i StrategyInput) float64 {
				return max(0.0, 0.5-i.SDT.Autonomy)
			}},
			{"low_competence", 1.5, func(i StrategyInput) float64 {
				return max(0.0, 0.5-i.SDT.Competence)
			}},
			{"low_dominance", 1.0, func(i StrategyInput) float64 {
				return max(0.0, 0.5-i.VAD.Dominance)
			}},
		},
		PromptTemplate: "引导用户回忆类似场景的成功经验。用「你上次xx的情况也类似，你是怎么办的？」来唤醒自我效能感。让用户自己说出解决方案，你只需确认。",
	},
	{
		// == Cognitive Restructuring (认知重建) — CBT ==
		// 适用: 具体的认知扭曲需要系统性重构（比 reframing 更深）
		Strategy: StrategyCognitiveRestructuring,
		Conditions: []conditionRule{
			{"severe_distortion", 2.0, func(i StrategyInput) float64 {
				if i.DistortionCount >= 2 {
					return 0.8
				}
				return 0
			}},
			{"socratic_applicable", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeReappraisal {
					return 0.6
				}
				return 0
			}},
		},
		PromptTemplate: "使用苏格拉底式提问引导用户检验自己的信念：「支持这个想法的证据是什么？」「相反的证据呢？」「如果朋友遇到同样的情况，你会怎么说？」帮助用户自己发现认知偏差。",
	},
	{
		// == Socratic Questioning (苏格拉底式提问) — CBT (来自 MultiAgentESC) ==
		// 适用: 轻度认知扭曲、需要引导用户自己思考
		Strategy: StrategySocraticQuestioning,
		Conditions: []conditionRule{
			{"mild_distortion", 1.5, func(i StrategyInput) float64 {
				if i.DistortionCount == 1 {
					return 0.5
				}
				return 0
			}},
			{"problem_focused_mode", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeProblemFocused {
					return 0.4
				}
				return 0
			}},
			{"moderate_competence", 1.0, func(i StrategyInput) float64 {
				if i.SDT.Competence >= 0.4 && i.SDT.Competence <= 0.7 {
					return 0.5
				}
				return 0
			}},
		},
		PromptTemplate: "用开放性问题引导用户自己探索：「你说的是……能具体说说吗？」「你觉得为什么会这样？」「如果换一种方式做，会有什么不同？」避免直接给答案。",
	},
	{
		// == Light Humor (轻幽默) ==
		// 适用: 疲劳、无聊、中等压力、不需要深层干预
		Strategy: StrategyLightHumor,
		Conditions: []conditionRule{
			{"fatigue_boredom", 1.5, func(i StrategyInput) float64 {
				if i.Dominant == EmoFatigue || i.Dominant == EmoBoredom {
					return 0.7
				}
				return 0
			}},
			{"no_distortion", 1.0, func(i StrategyInput) float64 {
				if i.DistortionCount == 0 {
					return 0.4
				}
				return 0
			}},
		},
		PromptTemplate: "用轻松幽默的语气回应。用行业相关的比喻或网络梗，不要太正式。保持轻松自然，不要过度。",
	},
	{
		// == Normalizing (正常化) — 社会比较理论 ==
		// 适用: Impostor Syndrome、初次遇到困难
		Strategy: StrategyNormalizing,
		Conditions: []conditionRule{
			{"low_competence_low_relatedness", 1.5, func(i StrategyInput) float64 {
				if i.SDT.Competence < 0.4 && i.SDT.Relatedness < 0.4 {
					return 0.6
				}
				return 0
			}},
			{"impostor_signals", 1.0, func(i StrategyInput) float64 {
				for _, d := range i.Distortions {
					if d == DistLabeling {
						return 0.5
					}
				}
				return 0
			}},
		},
		PromptTemplate: "让用户知道这不是ta一个人的问题。用「这个阶段很多人都会这样」开头。分享一些行业共通经历（不指名具体人），但不要比较谁更难。",
	},
	{
		// == Accompany (信息型陪伴) ==
		// 适用: 困惑、焦虑、需要确认
		Strategy: StrategyAccompany,
		Conditions: []conditionRule{
			{"confusion_anxiety", 1.5, func(i StrategyInput) float64 {
				if i.Dominant == EmoConfusion || i.Dominant == EmoAnxiety {
					return 0.6
				}
				return 0
			}},
			{"requesting_advice", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeProblemFocused {
					return 0.4
				}
				return 0
			}},
		},
		PromptTemplate: "提供专业认可，不增加认知负担。简短回应，认可用户的判断力和专业性。用户问建议时，先确认ta的思考方向，再提供少量补充信息。",
	},
	{
		// == Redirect Action (行动引导) — Flow Theory ==
		// 适用: 反刍思维、过度纠结
		Strategy: StrategyRedirectAction,
		Conditions: []conditionRule{
			{"rumination", 1.5, func(i StrategyInput) float64 {
				if i.DistortionCount >= 2 && i.VAD.Arousal > 0.5 {
					return 0.5
				}
				return 0
			}},
			{"low_control_high_arousal", 1.0, func(i StrategyInput) float64 {
				if i.VAD.Dominance < 0.3 && i.VAD.Arousal > 0.6 {
					return 0.5
				}
				return 0
			}},
		},
		PromptTemplate: "引导用户注意力从「为什么」转移到「做什么」。「现在可以做的一件小事是什么？」帮用户找到可操作的第一步，降低行动门槛。",
	},
}

// incompatiblePairs 不可同时使用的策略组合
var incompatiblePairs = map[DialogueStrategy][]DialogueStrategy{
	StrategyValidation:             {StrategyRedirectAction, StrategyCognitiveRestructuring},
	StrategyRedirectAction:         {StrategyValidation, StrategySocraticQuestioning},
	StrategyCognitiveRestructuring: {StrategyLightHumor},
}

// matchStrategy 匹配最佳对话策略
//
// 参考 MultiAgentESC 的 Strategy Selection Agent 逻辑:
// 1. 计算每个策略的每个条件的加权激活分数
// 2. 取加权平均作为策略得分
// 3. 选择得分最高的主策略 (阈值 > 0.15)
// 4. 选择互补的辅策略 (阈值 > 0.2)
func matchStrategy(input StrategyInput) StrategyMatch {
	scores := make(map[DialogueStrategy]float64)
	for _, sc := range allStrategies {
		var weightedSum, totalWeight float64
		for _, cond := range sc.Conditions {
			weightedSum += cond.Match(input) * cond.Weight
			totalWeight += cond.Weight
		}
		if totalWeight > 0 {
			scores[sc.Strategy] = weightedSum / totalWeight
		}
	}

	// 排序：得分 > 0.15
	type scored struct {
		strategy DialogueStrategy
		score    float64
	}
	var sorted []scored
	for s, v := range scores {
		if v > 0.15 {
			sorted = append(sorted, scored{s, v})
		}
	}
	// 简单冒泡排序（策略数量固定为 9）
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].score > sorted[i].score {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	primary := StrategyValidation
	primaryScore := 0.5
	if len(sorted) > 0 {
		primary = sorted[0].strategy
		primaryScore = sorted[0].score
	}

	var secondary *DialogueStrategy
	for i := 1; i < len(sorted); i++ {
		if isComplementary(primary, sorted[i].strategy) && sorted[i].score > 0.2 {
			s := sorted[i].strategy
			secondary = &s
			break
		}
	}

	rationale := buildRationale(primary, input, primaryScore)
	prompt := getStrategyPrompt(primary)

	return StrategyMatch{
		Primary:        primary,
		Secondary:      secondary,
		Confidence:     primaryScore,
		Rationale:      rationale,
		StrategyPrompt: prompt,
	}
}

// isComplementary 判断两个策略是否互补（非互斥），双向检查确保对称
func isComplementary(primary, secondary DialogueStrategy) bool {
	// 检查 primary → secondary
	if incompatible, ok := incompatiblePairs[primary]; ok {
		for _, inc := range incompatible {
			if inc == secondary {
				return false
			}
		}
	}
	// 检查 secondary → primary（对称）
	if incompatible, ok := incompatiblePairs[secondary]; ok {
		for _, inc := range incompatible {
			if inc == primary {
				return false
			}
		}
	}
	return true
}

// buildRationale 构建策略选择理由
func buildRationale(strategy DialogueStrategy, input StrategyInput, score float64) string {
	var parts []string
	if input.DistortionCount > 0 {
		parts = append(parts, fmt.Sprintf("检测到%d种认知扭曲", input.DistortionCount))
	}
	if input.SDT.Motivation < 0.4 {
		parts = append(parts, fmt.Sprintf("动机水平偏低(%.2f)", input.SDT.Motivation))
	}
	if input.SDT.LastUpdatedNeed != "" {
		parts = append(parts, fmt.Sprintf("%s需求变化明显", input.SDT.LastUpdatedNeed))
	}
	if input.VAD.Valence < -0.3 {
		parts = append(parts, fmt.Sprintf("情绪偏负面(valence=%.2f)", input.VAD.Valence))
	}

	profile := "常规对话"
	if len(parts) > 0 {
		profile = ""
		for i, p := range parts {
			if i > 0 {
				profile += "; "
			}
			profile += p
		}
	}
	return fmt.Sprintf("策略=%s (置信度=%.2f) | %s", strategy, score, profile)
}

// getStrategyPrompt 获取策略对应的提示词模板
func getStrategyPrompt(strategy DialogueStrategy) string {
	for _, sc := range allStrategies {
		if sc.Strategy == strategy {
			return sc.PromptTemplate
		}
	}
	return ""
}
