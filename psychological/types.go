package psychological

// ============================================================================
// VAD 三维情绪空间 — 参考 JEmAS / EmotionDetection
// ============================================================================

// VADVector 表示三维连续情绪空间中的坐标
type VADVector struct {
	Valence   float64 // -1.0 ~ 1.0 (愉悦度: 负面→正面)
	Arousal   float64 // 0.0 ~ 1.0 (唤醒度: 平静→激动)
	Dominance float64 // 0.0 ~ 1.0 (支配度: 失控→掌控)
}

// ============================================================================
// OCC 情绪类型 — 参考 Ortony-Clore-Collins 模型
// ============================================================================

// OCCEmotion 是 OCC 模型定义的情绪类型
type OCCEmotion string

const (
	EmoJoy            OCCEmotion = "joy"
	EmoDistress       OCCEmotion = "distress"
	EmoHope           OCCEmotion = "hope"
	EmoFear           OCCEmotion = "fear"
	EmoSatisfaction   OCCEmotion = "satisfaction"
	EmoDisappointment OCCEmotion = "disappointment"
	EmoRelief         OCCEmotion = "relief"
	EmoFearConfirmed  OCCEmotion = "fear_confirmed"
	EmoPride          OCCEmotion = "pride"
	EmoShame          OCCEmotion = "shame"
	EmoGratitude      OCCEmotion = "gratitude"
	EmoAnger          OCCEmotion = "anger"
	EmoGuilt          OCCEmotion = "guilt"
	EmoAdmiration     OCCEmotion = "admiration"
	EmoReproach       OCCEmotion = "reproach"
	EmoLiking         OCCEmotion = "liking"
	EmoDisliking      OCCEmotion = "disliking"
	EmoFrustration    OCCEmotion = "frustration"
	EmoAnxiety        OCCEmotion = "anxiety"
	EmoFatigue        OCCEmotion = "fatigue"
	EmoBoredom        OCCEmotion = "boredom"
	EmoConfusion      OCCEmotion = "confusion"
)

// OCCEmotionVAD 每种 OCC 情绪对应的 VAD 中心坐标
// 参考: JEmAS 情绪标注数据
var OCCEmotionVAD = map[OCCEmotion]VADVector{
	EmoJoy:            {0.81, 0.51, 0.64},
	EmoDistress:       {-0.61, 0.53, 0.37},
	EmoHope:           {0.52, 0.43, 0.43},
	EmoFear:           {-0.64, 0.60, 0.28},
	EmoSatisfaction:   {0.70, 0.32, 0.58},
	EmoDisappointment: {-0.63, 0.27, 0.32},
	EmoRelief:         {0.50, -0.21, 0.38},
	EmoFearConfirmed:  {-0.61, 0.40, 0.26},
	EmoPride:          {0.70, 0.49, 0.70},
	EmoShame:          {-0.58, 0.32, 0.17},
	EmoGratitude:      {0.64, 0.21, 0.40},
	EmoAnger:          {-0.51, 0.59, 0.59},
	EmoGuilt:          {-0.52, 0.34, 0.24},
	EmoAdmiration:     {0.62, 0.27, 0.30},
	EmoReproach:       {-0.56, 0.47, 0.46},
	EmoLiking:         {0.68, 0.24, 0.40},
	EmoDisliking:      {-0.60, 0.35, 0.44},
	EmoFrustration:    {-0.55, 0.55, 0.35},
	EmoAnxiety:        {-0.50, 0.65, 0.25},
	EmoFatigue:        {-0.30, -0.50, 0.20},
	EmoBoredom:        {-0.35, -0.45, 0.30},
	EmoConfusion:      {-0.20, 0.35, 0.20},
}

// ============================================================================
// OCC 评价框架 — 参考 jocc 实现 (iv4xr-project)
// ============================================================================

// AppraisalFrame 将用户表达量化为 8 个评价维度
type AppraisalFrame struct {
	Desirability      float64 // 期望值: ΔU = U(goal_state) - U(current_state), -1 ~ 1
	Likelihood        float64 // 可能性: P(event | evidence), 0 ~ 1
	Praiseworthiness  float64 // 赞扬性: 他人行为符合社会规范的程度, -1 ~ 1
	Deservingness     float64 // 应得性: 该受表扬/责备的程度, 0 ~ 1
	Appealingness     float64 // 吸引力: 对象物本身的吸引力, -1 ~ 1
	Unexpectedness    float64 // 出乎意料程度: 0 ~ 1
	CausalAttribution float64 // 因果归因: -1=自己, 0=环境, 1=他人
	Controllability   float64 // 可控性: 0 ~ 1
}

// OCCEmotionFormula 情绪强度计算公式
// 核心公式: intensity = max(0, Σ(wi × max(0, vi)) / Σ(wi))
type OCCEmotionFormula struct {
	Emotion   OCCEmotion
	Weights   []float64
	Variables func(frame AppraisalFrame) []float64
}

// ============================================================================
// EMA 认知评价 — 参考 Gratch & Marsella EMA 模型
// ============================================================================

// CopingMode 应对策略倾向
type CopingMode string

const (
	CopeProblemFocused CopingMode = "problem_focused" // 问题导向
	CopeEmotionFocused CopingMode = "emotion_focused" // 情绪导向
	CopeAvoidance      CopingMode = "avoidance"       // 回避
	CopeReappraisal    CopingMode = "reappraisal"     // 认知重构
)

// EMAAssessment 四维认知评价结果
type EMAAssessment struct {
	GoalRelevance    float64    // 目标相关性 (0~1)
	GoalCongruence   float64    // 目标一致性 (-1~1: 不一致→一致)
	CopingPotential  float64    // 应对能力 (0~1)
	Agency           float64    // 因果归因 (-1~1: 其他人→自己)
	FutureExpectancy float64    // 未来预期 (-1~1: 悲观→乐观)
	CopingMode       CopingMode // 应对策略倾向
}

// ============================================================================
// 认知扭曲 — 参考 Beck CBT 13 种认知扭曲
// ============================================================================

// CognitiveDistortion Beck 认知扭曲类型
type CognitiveDistortion string

const (
	DistAllOrNothing         CognitiveDistortion = "all_or_nothing"         // 非黑即白
	DistCatastrophizing      CognitiveDistortion = "catastrophizing"        // 灾难化
	DistOvergeneralization   CognitiveDistortion = "overgeneralization"     // 过度泛化
	DistMentalFiltering      CognitiveDistortion = "mental_filtering"       // 心理过滤
	DistDiscountingPositive  CognitiveDistortion = "discounting_positive"   // 贬低正面
	DistJumpingToConclusions CognitiveDistortion = "jumping_to_conclusions" // 妄下结论
	DistMindReading          CognitiveDistortion = "mind_reading"           // 读心术
	DistFortuneTelling       CognitiveDistortion = "fortune_telling"        // 算命式预测
	DistMagnifying           CognitiveDistortion = "magnifying"             // 夸大化
	DistEmotionalReasoning   CognitiveDistortion = "emotional_reasoning"    // 情绪推理
	DistShouldStatements     CognitiveDistortion = "should_statements"      // 应该陈述
	DistLabeling             CognitiveDistortion = "labeling"               // 贴标签
	DistPersonalization      CognitiveDistortion = "personalization"        // 过度自责
)

// DistortionDetection 认知扭曲检测结果
type DistortionDetection struct {
	Distortions       []CognitiveDistortion // 检测到的扭曲类型
	BeliefStatements  []string              // 扭曲信念的原句提取
	BeliefIntensity   float64               // 信念强度 (0~1)
	FactualStatements []string              // 已分离的事实陈述
}

// ============================================================================
// SDT 自我决定理论 — 参考 Deterding & Guckelsberger (2026)
// ============================================================================

// SDTState 自我决定理论的心理需求状态
type SDTState struct {
	Autonomy        float64 // 自主性需求满足度 (0~1)
	Competence      float64 // 胜任感需求满足度 (0~1)
	Relatedness     float64 // 归属感需求满足度 (0~1)
	Motivation      float64 // 综合动机得分 (0~1)
	LastUpdatedNeed string  // 最近更新的需求 ("autonomy" | "competence" | "relatedness" | "")
}

// SDTWeights SDT 需求权重
type SDTWeights struct {
	Autonomy    float64
	Competence  float64
	Relatedness float64
}

// ============================================================================
// 对话策略 — 参考 MultiAgentESC (EMNLP 2025)
// ============================================================================

// DialogueStrategy 对话策略类型
type DialogueStrategy string

const (
	StrategyValidation             DialogueStrategy = "validation"              // 共情验证 — Rogers
	StrategyReframing              DialogueStrategy = "reframing"               // 认知重构 — Beck
	StrategyEmpowerment            DialogueStrategy = "empowerment"             // 赋能引导 — Bandura
	StrategyLightHumor             DialogueStrategy = "light_humor"             // 轻幽默
	StrategyAccompany              DialogueStrategy = "accompany"               // 信息型陪伴
	StrategyNormalizing            DialogueStrategy = "normalizing"             // 正常化 — 社会比较
	StrategyRedirectAction         DialogueStrategy = "redirect_action"         // 行动引导 — Flow Theory
	StrategyCognitiveRestructuring DialogueStrategy = "cognitive_restructuring" // 认知重建 — CBT
	StrategySocraticQuestioning    DialogueStrategy = "socratic_questioning"    // 苏格拉底式提问 — CBT
)

// StrategyMatch 策略匹配结果
type StrategyMatch struct {
	Primary        DialogueStrategy  // 主策略
	Secondary      *DialogueStrategy // 辅策略 (nil 表示无)
	Confidence     float64           // 策略有效度 (0~1)
	Rationale      string            // 选择理由
	StrategyPrompt string            // 策略应用的具体提示词
}

// ============================================================================
// 管道综合结果
// ============================================================================

// DialogueUnderstanding 语义理解阶段输出
type DialogueUnderstanding struct {
	VAD                VADVector              // 三维情绪坐标
	DominantEmotion    OCCEmotion             // 主导情绪
	EmotionIntensities map[OCCEmotion]float64 // 所有情绪强度
	Distortions        DistortionDetection    // 认知扭曲检测
	SDT                SDTState               // SDT 需求状态
	Appraisal          EMAAssessment          // EMA 认知评价
}

// PipelineMetadata 管道元数据
type PipelineMetadata struct {
	Version    string   // 管道版本号
	Steps      []string // 已执行的阶段列表
	Confidence float64  // 综合置信度
}

// NuoChatResult 完整管道分析结果
type NuoChatResult struct {
	Understanding DialogueUnderstanding
	Strategy      StrategyMatch
	Metadata      PipelineMetadata
}

// ============================================================================
// 文本信号 — 从用户自然语言中提取的量化信号
// ============================================================================

// TextSignals 从文本中提取的 OCC/EMA 所需量化信号
type TextSignals struct {
	Sentiment        float64 // 情感倾向 (-1负面 ~ 1正面)
	Uncertainty      float64 // 不确定性 (0确定 ~ 1不确定)
	BlameDirection   float64 // 归因倾向 (-1自己 ~ 1他人/环境)
	PerceivedControl float64 // 感知控制感 (0无 ~ 1有)
	SurpriseLevel    float64 // 意外程度 (0预料之中 ~ 1意外)
	GoalImportance   float64 // 目标重要性 (0不重要 ~ 1重要)
}

// ============================================================================
// SDT 追踪器配置
// ============================================================================

// SDTTrackerConfig SDT 追踪器配置
type SDTTrackerConfig struct {
	Weights         SDTWeights // 需求权重
	CompetenceAlpha float64    // 胜任感学习率 α (默认 0.3)
	CompetenceBeta  float64    // 挑战-技能匹配敏感度 β (默认 0.1)
	DecayRate       float64    // 需求变化衰减率 (默认 0.05)
}

// DefaultSDTTrackerConfig 返回默认 SDT 追踪器配置
func DefaultSDTTrackerConfig() SDTTrackerConfig {
	return SDTTrackerConfig{
		Weights:         SDTWeights{Autonomy: 0.33, Competence: 0.34, Relatedness: 0.33},
		CompetenceAlpha: 0.3,
		CompetenceBeta:  0.1,
		DecayRate:       0.05,
	}
}
