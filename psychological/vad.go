package psychological

// occToVAD 将 OCC 强度向量映射到 VAD 三维空间
// 加权平均: VAD = Σ(intensity_i × VAD_center_i) / Σ(intensity_i)
func occToVAD(intensities map[OCCEmotion]float64) VADVector {
	var totalV, totalA, totalD, totalW float64
	for emo, intensity := range intensities {
		if intensity <= 0 {
			continue
		}
		vad, ok := OCCEmotionVAD[emo]
		if !ok {
			continue
		}
		totalV += intensity * vad.Valence
		totalA += intensity * vad.Arousal
		totalD += intensity * vad.Dominance
		totalW += intensity
	}
	if totalW == 0 {
		return VADVector{Valence: 0, Arousal: 0.5, Dominance: 0.5}
	}
	return VADVector{
		Valence:   clamp(totalV/totalW, -1, 1),
		Arousal:   clamp(totalA/totalW, 0, 1),
		Dominance: clamp(totalD/totalW, 0, 1),
	}
}

// mergeIntensities 合并 OCC 和 EMA 情绪强度，取最大值
// OCC 和 EMA 互补覆盖不同的情绪触发路径
func mergeIntensities(occ, ema map[OCCEmotion]float64) map[OCCEmotion]float64 {
	merged := make(map[OCCEmotion]float64)
	for k, v := range occ {
		merged[k] = v
	}
	for k, v := range ema {
		if v > merged[k] {
			merged[k] = v
		}
	}
	return merged
}
