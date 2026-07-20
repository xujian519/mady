package psychological

// ============================================================================
// VAD 三维情绪空间
// ============================================================================

// VADVector 表示三维连续情绪空间中的坐标
type VADVector struct {
	Valence   float64 // -1.0 ~ 1.0 (愉悦度)
	Arousal   float64 // 0.0 ~ 1.0 (唤醒度)
	Dominance float64 // 0.0 ~ 1.0 (支配度)
}

// TextSignals 从文本中提取的量化信号
type TextSignals struct {
	Sentiment        float64 // 情感倾向 (-1负面 ~ 1正面)
	Uncertainty      float64 // 不确定性 (0确定 ~ 1不确定)
	BlameDirection   float64 // 归因倾向 (-1自己 ~ 1他人/环境)
	PerceivedControl float64 // 感知控制感 (0无 ~ 1有)
	SurpriseLevel    float64 // 意外程度
	GoalImportance   float64 // 目标重要性
}

// EmotionProfile 简化情绪画像
type EmotionProfile struct {
	VAD             VADVector          // 三维情绪坐标
	DominantEmotion string             // 主导情绪名称
	Intensities     map[string]float64 // 各情绪强度
}

// ============================================================================
// 对话策略
// ============================================================================

// DialogueStrategy 对话策略类型
type DialogueStrategy string

const (
	StrategyEmpathetic   DialogueStrategy = "empathetic"   // 共情
	StrategyProfessional DialogueStrategy = "professional" // 专业克制
	StrategyEncouraging  DialogueStrategy = "encouraging"  // 鼓励
	StrategyNeutral      DialogueStrategy = "neutral"      // 中性
	StrategyCalming      DialogueStrategy = "calming"      // 安抚
)

// StrategyMatch 策略匹配结果
type StrategyMatch struct {
	Primary        DialogueStrategy
	Confidence     float64
	Rationale      string
	StrategyPrompt string
}

// ============================================================================
// 管道综合结果
// ============================================================================

// NuoChatResult 心理分析结果
type NuoChatResult struct {
	Emotion  EmotionProfile
	Strategy StrategyMatch
	Metadata PipelineMetadata
}

// PipelineMetadata 管道元数据
type PipelineMetadata struct {
	Version    string
	Confidence float64
}

// PipelineConfig 管道运行配置
type PipelineConfig struct {
	SkipDistortionDetection bool
}

// ============================================================================
// Config — 心理引擎配置
// ============================================================================

// Config 心理引擎配置
type Config struct {
	SkipDistortionDetection bool
}
