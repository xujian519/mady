package psychological

import (
	"fmt"
	"strings"
)

// ExecuteFullPipeline 执行精简的心理分析管道
//
// 阶段:
//  1. 文本信号提取 (关键词规则)
//  2. VAD 情绪空间计算 (信号→VAD 直接映射)
//  3. 策略匹配 (VAD + 情绪→语调策略)
func ExecuteFullPipeline(text string, config *PipelineConfig) NuoChatResult {
	_ = config // reserved for future pipeline configuration

	// Step 1: 提取文本信号
	signals := extractTextualSignals(text)

	// Step 2: 计算 VAD 和情绪画像
	emotion := computeVADFromSignals(signals)

	// Step 3: 匹配对话策略
	strategy := matchStrategy(emotion)

	return NuoChatResult{
		Emotion:  emotion,
		Strategy: strategy,
		Metadata: PipelineMetadata{
			Version:    "v3",
			Confidence: strategy.Confidence,
		},
	}
}

// BuildContextBlock 从分析结果构建注入系统提示的心理上下文块
func BuildContextBlock(result NuoChatResult) string {
	e := result.Emotion
	s := result.Strategy

	lines := []string{
		"【当前感知的用户心理状态】",
	}

	if e.DominantEmotion != "" {
		lines = append(lines, fmt.Sprintf(
			"主导情绪: %s, VAD(愉悦度=%.2f, 唤醒度=%.2f, 支配度=%.2f)",
			e.DominantEmotion, e.VAD.Valence, e.VAD.Arousal, e.VAD.Dominance,
		))
	}

	lines = append(lines,
		"",
		fmt.Sprintf("【对话策略】%s (置信度=%.2f)", s.Primary, s.Confidence),
		s.StrategyPrompt,
		fmt.Sprintf("选择理由: %s", s.Rationale),
	)

	return strings.Join(lines, "\n")
}

// ============================================================================
// Step 1: 文本信号提取
// ============================================================================

func extractTextualSignals(text string) TextSignals {
	lower := strings.ToLower(text)

	s := TextSignals{}

	// 情感极性
	s.Sentiment = sentimentScore(lower)

	// 不确定性
	s.Uncertainty = uncertaintyScore(lower)

	// 归因倾向
	s.BlameDirection = blameScore(lower)

	// 感知控制感
	s.PerceivedControl = controlScore(lower)

	// 意外程度
	s.SurpriseLevel = surpriseScore(lower)

	// 目标重要性
	s.GoalImportance = goalImportanceScore(lower)

	return s
}

func sentimentScore(text string) float64 {
	positive := []string{"谢谢", "感谢", "很好", "不错", "满意", "开心", "高兴", "成功", "顺利",
		"优秀", "赞", "棒", "厉害", "专业", "完成", "达成", "通过", "支持", "帮助",
		"解决", "满意", "放心", "信任", "方便", "容易", "清晰", "明白", "理解"}
	negative := []string{"失败", "错误", "不行", "糟糕", "麻烦", "困难", "担心", "焦虑",
		"害怕", "生气", "失望", "不满", "讨厌", "烦", "累", "急", "崩溃", "受不了",
		"无法", "不能", "拒绝", "驳回", "侵权", "无效", "遗憾"}

	pos := countMatches(text, positive)
	neg := countMatches(text, negative)

	if pos+neg == 0 {
		return 0
	}
	return clip((pos-neg)/(pos+neg), -1, 1)
}

func uncertaintyScore(text string) float64 {
	markers := []string{"可能", "也许", "不确定", "不太清楚", "不知道", "应该",
		"大概", "或许", "好像", "似乎", "怎么办", "如何", "怎么"}

	count := countMatches(text, markers)
	return clip(float64(count)*0.3, 0, 1)
}

func blameScore(text string) float64 {
	selfBlame := []string{"我的错", "我没做好", "我不行", "我错了", "我的问题"}
	otherBlame := []string{"他们", "对方", "审查员", "你们", "别人", "这系统"}

	self := countMatches(text, selfBlame)
	other := countMatches(text, otherBlame)

	if self+other == 0 {
		return 0
	}
	return clip((other-self)/(self+other), -1, 1)
}

func controlScore(text string) float64 {
	highControl := []string{"我来", "我可以", "我能", "我决定", "我选择", "已经完成",
		"处理好了", "解决了", "做好了", "完成了"}
	lowControl := []string{"被动", "不得不", "没办法", "无法控制", "不由我"}

	high := countMatches(text, highControl)
	low := countMatches(text, lowControl)

	if high+low == 0 {
		return 0.5 // 默认中等
	}
	return clip(0.5+(high-low)*0.3, 0, 1)
}

func surpriseScore(text string) float64 {
	markers := []string{"竟然", "没想到", "意外", "居然", "突然", "出乎意料",
		"不可思议", "惊人", "震惊", "难以置信", "奇怪"}

	count := countMatches(text, markers)
	return clip(float64(count)*0.25, 0, 1)
}

func goalImportanceScore(text string) float64 {
	markers := []string{"重要", "关键", "必须", "一定", "迫切", "紧急", "优先",
		"核心", "根本", "至关重要", "决定性的"}

	count := countMatches(text, markers)
	return clip(float64(count)*0.3, 0, 1)
}

func countMatches(text string, keywords []string) float64 {
	count := 0.0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			count++
		}
	}
	return count
}

// ============================================================================
// Step 2: VAD 情绪计算 (从信号直接映射)
// ============================================================================

func computeVADFromSignals(s TextSignals) EmotionProfile {
	// Valence: 正面/负面情感
	valence := s.Sentiment*0.7 + s.PerceivedControl*0.2 + s.BlameDirection*0.1

	// Arousal: 激动/平静 (高不确定性 + 高重要性 + 高意外 → 高唤醒)
	arousal := s.Uncertainty*0.3 + s.GoalImportance*0.3 + s.SurpriseLevel*0.2 + 0.2

	// Dominance: 掌控/失控 (高控制感 → 高支配)
	dominance := s.PerceivedControl*0.6 + (1-s.Uncertainty)*0.3 + 0.1

	profile := EmotionProfile{
		VAD: VADVector{
			Valence:   clip(valence, -1, 1),
			Arousal:   clip(arousal, 0, 1),
			Dominance: clip(dominance, 0, 1),
		},
		Intensities: make(map[string]float64),
	}

	// 确定主导情绪
	profile.DominantEmotion = classifyEmotion(profile.VAD)

	return profile
}

func classifyEmotion(v VADVector) string {
	if v.Valence > 0.3 {
		if v.Arousal > 0.6 {
			if v.Dominance > 0.5 {
				return "excited" // 兴奋
			}
			return "hopeful" // 期待
		}
		return "satisfied" // 满意
	}
	if v.Valence < -0.3 {
		if v.Arousal > 0.6 {
			if v.Dominance < 0.3 {
				return "anxious" // 焦虑
			}
			return "frustrated" // 挫败
		}
		return "disappointed" // 失望
	}
	return "neutral" // 中性
}

// ============================================================================
// Step 3: 策略匹配
// ============================================================================

func matchStrategy(emo EmotionProfile) StrategyMatch {
	v := emo.VAD

	switch {
	case v.Valence < -0.4 && v.Arousal > 0.5:
		// 高负面 + 高唤醒 → 安抚策略
		return StrategyMatch{
			Primary:    StrategyCalming,
			Confidence: clip(0.5+(v.Arousal*(1+v.Valence)/2), 0.5, 0.95),
			Rationale:  "检测到高唤醒负面情绪，使用安抚策略降低紧张感",
			StrategyPrompt: "用户当前情绪较为激动。请保持冷静克制的语气，" +
				"先确认理解用户的困扰，再提供专业的解决方案。" +
				"避免使用过于积极或轻快的措辞。",
		}

	case v.Valence < -0.2:
		// 轻度负面 → 共情策略
		return StrategyMatch{
			Primary:    StrategyEmpathetic,
			Confidence: clip(0.5+v.Arousal*0.3, 0.5, 0.9),
			Rationale:  "检测到轻度负面情绪，使用共情策略建立信任",
			StrategyPrompt: "用户可能有些许不满或担忧。请先用1-2句表达理解共情，" +
				"再转入专业分析。避免直接忽略情绪而进入技术细节。",
		}

	case v.Valence > 0.3 && v.Arousal > 0.5:
		// 高正面 + 高唤醒 → 鼓励策略
		return StrategyMatch{
			Primary:    StrategyEncouraging,
			Confidence: clip(0.5+v.Valence*0.3, 0.5, 0.95),
			Rationale:  "检测到积极情绪，使用鼓励策略增强用户信心",
			StrategyPrompt: "用户情绪积极。可以适当使用肯定和鼓励的语气，" +
				"在专业分析的基础上保持积极的交流氛围。",
		}

	case v.Arousal < 0.3:
		// 低唤醒 → 中性策略
		return StrategyMatch{
			Primary:    StrategyNeutral,
			Confidence: 0.7,
			Rationale:  "情绪平和，使用中性专业策略",
			StrategyPrompt: "用户情绪平静。保持专业、清晰的分析风格即可，" +
				"无需特别调整语气。",
		}

	default:
		// 默认专业策略
		return StrategyMatch{
			Primary:    StrategyProfessional,
			Confidence: 0.6,
			Rationale:  "默认使用专业克制策略",
			StrategyPrompt: "保持专业克制的语气，以事实和分析为导向。" +
				"避免过度情绪化的表达。",
		}
	}
}

func clip(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
