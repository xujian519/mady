package psychological

import (
	"fmt"
	"strings"
)

// DistortionLLMVerifier 认知扭曲 LLM 验证器（可选实现）
// 用于对正则检测到的认知扭曲进行二次验证，降低专利/法律语言误报率
type DistortionLLMVerifier interface {
	Verify(text string, d DistortionDetection) DistortionDetection
}

// PipelineConfig 管道运行配置
type PipelineConfig struct {
	SDTTracker              *SDTTracker             // SDT 追踪器（nil 则新建）
	LLMVerifier             DistortionLLMVerifier   // 可选 LLM 验证器
	SkipDistortionDetection bool                    // 跳过认知扭曲检测
}

// ExecuteFullPipeline 执行完整的 7 阶段心理分析管道
//
// 阶段:
//  1. signal_extraction - 文本信号提取 (关键词规则)
//  2. occ_emotion       - OCC AppraisalFrame → 情绪强度
//  3. ema_appraisal     - EMA 认知评价
//  4. vad_fusion        - OCC+EMA → VAD 融合
//  5. distortion_detection - 认知扭曲检测 (可选 LLM 验证)
//  6. sdt_update        - SDT 跨轮次需求追踪
//  7. strategy_matching - 对话策略匹配
//
// 参考: MultiAgentESC 三阶段架构 + OCC/EMA/SDT 模型
func ExecuteFullPipeline(text string, config *PipelineConfig) NuoChatResult {
	if config == nil {
		config = &PipelineConfig{}
	}
	steps := make([]string, 0, 7)
	tracker := config.SDTTracker
	if tracker == nil {
		tracker = NewSDTTracker(nil)
	}

	// Step 1: 提取文本信号
	signals := extractTextualSignals(text)
	steps = append(steps, "signal_extraction")

	// Step 2: 构建 AppraisalFrame → OCC 情绪强度
	frame := buildAppraisalFrame(signals)
	occIntensities := computeOCCEmotions(frame)
	steps = append(steps, "occ_emotion")

	// Step 3: EMA 认知评价 + OCC 情绪映射
	ema := computeEMA(frame)
	emaMapped := emaToOCCEmotions(ema)
	steps = append(steps, "ema_appraisal")

	// Step 4: 合并 OCC + EMA → VAD 空间
	merged := mergeIntensities(occIntensities, emaMapped)
	vad := occToVAD(merged)
	dominant, _ := getDominantEmotion(merged)
	steps = append(steps, "vad_fusion")

	// Step 5: 认知扭曲检测 (可选 LLM 验证)
	var distortions DistortionDetection
	severe := false
	if !config.SkipDistortionDetection {
		distortions = detectDistortions(text)
		if config.LLMVerifier != nil && len(distortions.Distortions) > 0 {
			distortions = config.LLMVerifier.Verify(text, distortions)
		}
		severe = hasSevereDistortion(distortions)
	}
	steps = append(steps, "distortion_detection")

	// Step 6: SDT 状态更新
	sdtSignals := SDTSignals{
		AutonomyFrustration:   boolToFloat(signals.BlameDirection > 0.5) * 0.6,
		CompetenceAnxiety:     boolToFloat(signals.Sentiment < -0.3 && signals.BlameDirection < -0.3) * 0.5,
		RelatednessLoneliness: boolToFloat(signals.BlameDirection > 0.5 && signals.Sentiment < -0.2) * 0.4,
		PerceivedDifficulty:   1 - signals.PerceivedControl,
	}
	sdtState := tracker.UpdateFromSignals(sdtSignals)
	steps = append(steps, "sdt_update")

	// Step 7: 策略匹配
	strategy := matchStrategy(StrategyInput{
		VAD:             vad,
		Dominant:        dominant,
		Intensities:     merged,
		SDT:             sdtState,
		EMA:             ema,
		Distortions:     distortions.Distortions,
		DistortionCount: len(distortions.Distortions),
		BeliefIntensity: distortions.BeliefIntensity,
	})
	steps = append(steps, "strategy_matching")

	confidence := strategy.Confidence
	if severe {
		confidence *= 0.85
	}

	return NuoChatResult{
		Understanding: DialogueUnderstanding{
			VAD:                vad,
			DominantEmotion:    dominant,
			EmotionIntensities: merged,
			Distortions:        distortions,
			SDT:                sdtState,
			Appraisal:          ema,
		},
		Strategy: strategy,
		Metadata: PipelineMetadata{
			Version:    "v2",
			Steps:      steps,
			Confidence: confidence,
		},
	}
}

// boolToFloat 将 bool 转为 float64 (true → 1, false → 0)
func boolToFloat(cond bool) float64 {
	if cond {
		return 1
	}
	return 0
}

// BuildContextBlock 从分析结果构建注入系统提示的心理上下文块
func BuildContextBlock(result NuoChatResult) string {
	u := result.Understanding

	lines := []string{
		"【当前感知的用户心理状态】",
	}

	// 情绪状态
	if u.DominantEmotion != "" {
		lines = append(lines, fmt.Sprintf(
			"主导情绪: %s, VAD(愉悦度=%.2f, 唤醒度=%.2f, 支配度=%.2f)",
			u.DominantEmotion, u.VAD.Valence, u.VAD.Arousal, u.VAD.Dominance,
		))
	}

	// 应对模式
	lines = append(lines, fmt.Sprintf("用户应对模式: %s", string(u.Appraisal.CopingMode)))

	// 认知扭曲
	if len(u.Distortions.Distortions) > 0 {
		distNames := make([]string, len(u.Distortions.Distortions))
		for i, d := range u.Distortions.Distortions {
			distNames[i] = string(d)
		}
		lines = append(lines, fmt.Sprintf(
			"检测到认知扭曲: %v (强度=%.2f)", distNames, u.Distortions.BeliefIntensity,
		))
	}

	// SDT 需求
	lines = append(lines, fmt.Sprintf(
		"SDT 需求状态: 自主性=%.2f, 胜任感=%.2f, 归属感=%.2f, 动机=%.2f",
		u.SDT.Autonomy, u.SDT.Competence, u.SDT.Relatedness, u.SDT.Motivation,
	))
	if u.SDT.LastUpdatedNeed != "" {
		lines = append(lines, fmt.Sprintf("最近变化的需求: %s", u.SDT.LastUpdatedNeed))
	}

	// 策略指引
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("【对话策略】%s (置信度=%.2f)", result.Strategy.Primary, result.Strategy.Confidence))
	lines = append(lines, result.Strategy.StrategyPrompt)
	lines = append(lines, fmt.Sprintf("选择理由: %s", result.Strategy.Rationale))

	return strings.Join(lines, "\n")
}
