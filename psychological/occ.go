package psychological

import "sort"

// occFormulaList 14 条 OCC 情绪强度计算公式（包级别单例）
// 分为三类：事件类(8) + 行为类(4) + 物品类(2)
var occFormulaList = []OCCEmotionFormula{
	// --- 事件类情绪 (Event-based) ---
	{EmoJoy, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Unexpectedness} }},
	{EmoDistress, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, f.Unexpectedness} }},
	{EmoHope, []float64{1.0, 0.8},
		func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Likelihood} }},
	{EmoFear, []float64{1.0, 0.8},
		func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, 1 - f.Likelihood} }},
	{EmoSatisfaction, []float64{1.0, 0.3},
		func(f AppraisalFrame) []float64 {
			return []float64{f.Desirability, f.Likelihood * (1 - f.Unexpectedness)}
		}},
	{EmoDisappointment, []float64{1.0, 0.3},
		func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, 1 - f.Likelihood} }},
	{EmoRelief, []float64{0.5, 1.0}, // higher weight on Unexpectedness per OCC theory
		func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Unexpectedness} }},
	{EmoFearConfirmed, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, 1 - f.Unexpectedness} }},
	// --- 行为类情绪 (Agent-based) ---
	{EmoPride, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 {
			return []float64{max(0.0, f.Praiseworthiness) * max(0.0, -f.CausalAttribution), f.Deservingness}
		}},
	{EmoShame, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 {
			return []float64{max(0.0, -f.Praiseworthiness) * max(0.0, -f.CausalAttribution), f.Deservingness}
		}},
	{EmoGratitude, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 {
			return []float64{max(0.0, f.Praiseworthiness) * max(0.0, f.CausalAttribution), f.Deservingness}
		}},
	{EmoAnger, []float64{1.0, 0.5},
		func(f AppraisalFrame) []float64 {
			return []float64{max(0.0, -f.Praiseworthiness) * max(0.0, f.CausalAttribution), f.Deservingness}
		}},
	// --- 物品类情绪 (Object-based) ---
	{EmoLiking, []float64{1.0},
		func(f AppraisalFrame) []float64 { return []float64{f.Appealingness} }},
	{EmoDisliking, []float64{1.0},
		func(f AppraisalFrame) []float64 { return []float64{-f.Appealingness} }},
}

// computeOCCEmotions 计算所有 OCC 情绪强度
// 核心公式: intensity = max(0, Σ(wi × max(0, vi)) / Σ(wi))
func computeOCCEmotions(frame AppraisalFrame) map[OCCEmotion]float64 {
	intensities := make(map[OCCEmotion]float64)
	for _, f := range occFormulaList {
		vars := f.Variables(frame)
		if len(vars) != len(f.Weights) {
			continue
		}
		var weightedSum, weightSum float64
		for i, v := range vars {
			clamped := max(0.0, v)
			weightedSum += f.Weights[i] * clamped
			weightSum += f.Weights[i]
		}
		if weightSum > 0 {
			intensities[f.Emotion] = clamp(weightedSum/weightSum, 0, 1)
		}
	}
	return intensities
}

// getDominantEmotion 返回强度最大的情绪（阈值 > 0.1），并列时按字母序取第一个以保证确定性
func getDominantEmotion(intensities map[OCCEmotion]float64) (OCCEmotion, float64) {
	type entry struct {
		emotion   OCCEmotion
		intensity float64
	}
	var candidates []entry
	for emo, intensity := range intensities {
		if intensity > 0.1 {
			candidates = append(candidates, entry{emo, intensity})
		}
	}
	if len(candidates) == 0 {
		return "", 0
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].intensity != candidates[j].intensity {
			return candidates[i].intensity > candidates[j].intensity
		}
		return string(candidates[i].emotion) < string(candidates[j].emotion)
	})
	return candidates[0].emotion, candidates[0].intensity
}
