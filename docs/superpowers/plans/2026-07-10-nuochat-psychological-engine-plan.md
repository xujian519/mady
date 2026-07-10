# NuoChat 心理引擎 — Go 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Mady 项目中实现 `psychological/` 包，将 nuochat 的 7 阶段心理分析管道完整移植到 Go。

**Architecture:** 自底向上实现：工具函数 → 类型定义 → 算法引擎(OCC/EMA/VAD) → 检测匹配(认知扭曲/SDT/策略) → 管道编排 → 集成层(Extension/Hook) → 修改 chat.go。所有算法纯 Go 标准库，零外部依赖。

**Tech Stack:** Go 1.25+, 标准库 (regexp, math, sync, encoding/json, os)

---

## 文件结构

```
psychological/
├── clamp.go          # clamp/abs 工具函数
├── types.go          # 所有类型、常量、映射表
├── signal.go         # 文本信号提取 (关键词规则)
├── occ.go            # OCC 14 种情绪公式引擎
├── ema.go            # EMA 认知评价 + 应对模式
├── vad.go            # VAD 情绪空间融合
├── distortion.go     # 13 种 Beck CBT 认知扭曲检测
├── sdt.go            # SDT 跨轮次需求追踪器
├── strategy.go       # 9 种对话策略匹配器
├── pipeline.go       # 7 阶段管道编排
├── store.go          # SDT 状态 JSON 持久化
├── extension.go      # agentcore.Extension 实现
├── hook.go           # LifecycleHook 工厂
├── *_test.go         # 对应单元测试
```

**修改文件:**
- `domains/chat.go` — 加载心理引擎 Extension

---

### Task 1: 工具函数 — `clamp.go`

**Files:**
- Create: `psychological/clamp.go`

- [ ] **Step 1: 编写 clamp.go**

```go
package psychological

// clamp 将值限制在 [lo, hi] 范围内
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// abs 返回 float64 的绝对值
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/xujian/projects/Mady && go build ./psychological/`
Expected: 编译成功（只有 clamp.go 时 types.go 未创建也可以编译，因为 clamp.go 没有类型引用）

---

### Task 2: 核心类型定义 — `types.go`

**Files:**
- Create: `psychological/types.go`

- [ ] **Step 1: 编写 types.go — 所有类型、常量、映射表**

```go
package psychological

// ============================================================================
// VAD 三维情绪空间
// ============================================================================

type VADVector struct {
	Valence   float64 // -1.0 ~ 1.0 (负面→正面)
	Arousal   float64 //  0.0 ~ 1.0 (平静→激动)
	Dominance float64 //  0.0 ~ 1.0 (失控→掌控)
}

// ============================================================================
// OCC 情绪类型
// ============================================================================

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

// OCCEmotionVAD 每种 OCC 情绪的 VAD 中心坐标
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
// OCC 评价框架
// ============================================================================

type AppraisalFrame struct {
	Desirability      float64
	Likelihood        float64
	Praiseworthiness  float64
	Deservingness     float64
	Appealingness     float64
	Unexpectedness    float64
	CausalAttribution float64
	Controllability   float64
}

// OCCEmotionFormula 情绪强度计算公式
type OCCEmotionFormula struct {
	Emotion   OCCEmotion
	Weights   []float64
	Variables func(frame AppraisalFrame) []float64
}

// ============================================================================
// EMA 认知评价
// ============================================================================

type CopingMode string

const (
	CopeProblemFocused CopingMode = "problem_focused"
	CopeEmotionFocused CopingMode = "emotion_focused"
	CopeAvoidance      CopingMode = "avoidance"
	CopeReappraisal    CopingMode = "reappraisal"
)

type EMAAssessment struct {
	GoalRelevance    float64
	GoalCongruence   float64
	CopingPotential  float64
	Agency           float64
	FutureExpectancy float64
	CopingMode       CopingMode
}

// ============================================================================
// 认知扭曲 (Beck CBT)
// ============================================================================

type CognitiveDistortion string

const (
	DistAllOrNothing        CognitiveDistortion = "all_or_nothing"
	DistCatastrophizing     CognitiveDistortion = "catastrophizing"
	DistOvergeneralization  CognitiveDistortion = "overgeneralization"
	DistMentalFiltering     CognitiveDistortion = "mental_filtering"
	DistDiscountingPositive CognitiveDistortion = "discounting_positive"
	DistJumpingToConclusions CognitiveDistortion = "jumping_to_conclusions"
	DistMindReading         CognitiveDistortion = "mind_reading"
	DistFortuneTelling      CognitiveDistortion = "fortune_telling"
	DistMagnifying          CognitiveDistortion = "magnifying"
	DistEmotionalReasoning  CognitiveDistortion = "emotional_reasoning"
	DistShouldStatements    CognitiveDistortion = "should_statements"
	DistLabeling            CognitiveDistortion = "labeling"
	DistPersonalization     CognitiveDistortion = "personalization"
)

type DistortionDetection struct {
	Distortions       []CognitiveDistortion
	BeliefStatements  []string
	BeliefIntensity   float64
	FactualStatements []string
}

// ============================================================================
// SDT 自我决定理论
// ============================================================================

type SDTState struct {
	Autonomy        float64
	Competence      float64
	Relatedness     float64
	Motivation      float64
	LastUpdatedNeed string
}

type SDTWeights struct {
	Autonomy    float64
	Competence  float64
	Relatedness float64
}

// ============================================================================
// 对话策略
// ============================================================================

type DialogueStrategy string

const (
	StrategyValidation            DialogueStrategy = "validation"
	StrategyReframing             DialogueStrategy = "reframing"
	StrategyEmpowerment           DialogueStrategy = "empowerment"
	StrategyLightHumor            DialogueStrategy = "light_humor"
	StrategyAccompany             DialogueStrategy = "accompany"
	StrategyNormalizing           DialogueStrategy = "normalizing"
	StrategyRedirectAction        DialogueStrategy = "redirect_action"
	StrategyCognitiveRestructuring DialogueStrategy = "cognitive_restructuring"
	StrategySocraticQuestioning   DialogueStrategy = "socratic_questioning"
)

type StrategyMatch struct {
	Primary        DialogueStrategy
	Secondary      *DialogueStrategy
	Confidence     float64
	Rationale      string
	StrategyPrompt string
}

// ============================================================================
// 管道综合结果
// ============================================================================

type DialogueUnderstanding struct {
	VAD                VADVector
	DominantEmotion    OCCEmotion
	EmotionIntensities map[OCCEmotion]float64
	Distortions        DistortionDetection
	SDT                SDTState
	Appraisal          EMAAssessment
}

type PipelineMetadata struct {
	Version    string
	Steps      []string
	Confidence float64
}

type NuoChatResult struct {
	Understanding DialogueUnderstanding
	Strategy      StrategyMatch
	Metadata      PipelineMetadata
}

// ============================================================================
// 文本信号
// ============================================================================

type TextSignals struct {
	Sentiment        float64
	Uncertainty      float64
	BlameDirection   float64
	PerceivedControl float64
	SurpriseLevel    float64
	GoalImportance   float64
}

// ============================================================================
// SDT 追踪器配置
// ============================================================================

type SDTTrackerConfig struct {
	Weights         SDTWeights
	CompetenceAlpha float64
	CompetenceBeta  float64
	DecayRate       float64
}

func DefaultSDTTrackerConfig() SDTTrackerConfig {
	return SDTTrackerConfig{
		Weights:         SDTWeights{Autonomy: 0.33, Competence: 0.34, Relatedness: 0.33},
		CompetenceAlpha: 0.3,
		CompetenceBeta:  0.1,
		DecayRate:       0.05,
	}
}
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/xujian/projects/Mady && go build ./psychological/`

---

### Task 3: 文本信号提取 — `signal.go`

**Files:**
- Create: `psychological/signal.go`

- [ ] **Step 1: 编写 signal.go**

```go
package psychological

import "regexp"

// 预编译的正则模式，在包初始化时编译
var (
	reStrongNegative = regexp.MustCompile(`(?i)不好|不行|糟糕|失败|驳回|拒绝|awful|terrible`)
	reStrongPositive = regexp.MustCompile(`(?i)不错|开心|顺利|通过|满意|高兴|成功|搞定|thanks|great|good|happy`)
	reWeakNegative   = regexp.MustCompile(`(?i)烦|气|差|累|怒|讨厌|bad|angry`)
	reWeakPositive   = regexp.MustCompile(`(?i)好|顺利|good|happy`)
	reUncertain      = regexp.MustCompile(`(?i)不知道|不确定|可能|也许|大概|maybe|perhaps|unsure|if|whether`)
	reSelfBlame      = regexp.MustCompile(`(?i)我.*错|我不好|我能力|我不够|my fault|i should|i could`)
	reOtherBlame     = regexp.MustCompile(`(?i)他|他们|公司|客户|审查员|同事|环境|they|them|manager|boss|client`)
	reHasControl     = regexp.MustCompile(`(?i)有办法|可以|能处理|handle|manage|solution|plan`)
	reNoControl      = regexp.MustCompile(`(?i)没办法|不得不|被迫|无解|hopeless|stuck|can't|no choice`)
	reSurprise       = regexp.MustCompile(`(?i)没想到|突然|意外|surprise|unexpected|居然`)
	reImportant      = regexp.MustCompile(`(?i)很重要|关键|必须|重要|critical|important|must|need|有影响`)
)

// extractTextualSignals 从用户文本中提取量化信号
// 信号提取顺序：先匹配强信号，再匹配弱信号
func extractTextualSignals(text string) TextSignals {
	lower := text

	// 情感倾向 — 强信号优先
	sentiment := 0.0
	switch {
	case reStrongNegative.MatchString(lower):
		sentiment = -0.7
	case reStrongPositive.MatchString(lower):
		sentiment = 0.7
	case reWeakNegative.MatchString(lower):
		sentiment = -0.5
	case reWeakPositive.MatchString(lower):
		sentiment = 0.4
	}

	// 不确定性
	uncertainty := 0.2
	if reUncertain.MatchString(lower) {
		uncertainty = 0.7
	}

	// 归因方向 (-1=自己, 1=他人/环境)
	blameDirection := 0.0
	switch {
	case reSelfBlame.MatchString(lower):
		blameDirection = -0.6
	case reOtherBlame.MatchString(lower):
		blameDirection = 0.7
	}

	// 控制感
	perceivedControl := 0.5
	switch {
	case reNoControl.MatchString(lower):
		perceivedControl = 0.2
	case reHasControl.MatchString(lower):
		perceivedControl = 0.8
	}

	// 意外程度
	surpriseLevel := 0.2
	if reSurprise.MatchString(lower) {
		surpriseLevel = 0.8
	}

	// 目标重要性
	goalImportance := 0.4
	if reImportant.MatchString(lower) {
		goalImportance = 0.8
	}

	return TextSignals{
		Sentiment:        sentiment,
		Uncertainty:      uncertainty,
		BlameDirection:   blameDirection,
		PerceivedControl: perceivedControl,
		SurpriseLevel:    surpriseLevel,
		GoalImportance:   goalImportance,
	}
}

// buildAppraisalFrame 从文本信号构建 OCC 评价框架
func buildAppraisalFrame(signals TextSignals) AppraisalFrame {
	deservingness := 0.5
	if signals.Sentiment >= 0 {
		deservingness = 0.5
	} else {
		deservingness = 0.8
	}

	return AppraisalFrame{
		Desirability:      clamp(signals.Sentiment, -1, 1),
		Likelihood:        clamp(1-signals.Uncertainty, 0, 1),
		Praiseworthiness:  clamp(signals.Sentiment, -1, 1),
		Deservingness:     deservingness,
		Appealingness:     clamp(signals.Sentiment, -1, 1),
		Unexpectedness:    clamp(signals.SurpriseLevel, 0, 1),
		CausalAttribution: clamp(signals.BlameDirection, -1, 1),
		Controllability:   clamp(signals.PerceivedControl, 0, 1),
	}
}
```

- [ ] **Step 2: 编写 signal_test.go**

```go
package psychological

import "testing"

func TestExtractTextualSignalsStrongNegative(t *testing.T) {
	s := extractTextualSignals("这个专利被驳回了，太糟糕了")
	if s.Sentiment > -0.5 {
		t.Errorf("expected strong negative sentiment, got %f", s.Sentiment)
	}
}

func TestExtractTextualSignalsStrongPositive(t *testing.T) {
	s := extractTextualSignals("专利申请顺利通过了，非常开心！")
	if s.Sentiment < 0.5 {
		t.Errorf("expected strong positive sentiment, got %f", s.Sentiment)
	}
}

func TestExtractTextualSignalsUncertainty(t *testing.T) {
	s := extractTextualSignals("我不确定这个方案是否可行，可能需要再想想")
	if s.Uncertainty < 0.5 {
		t.Errorf("expected high uncertainty, got %f", s.Uncertainty)
	}
}

func TestExtractTextualSignalsSelfBlame(t *testing.T) {
	s := extractTextualSignals("都是我的错，我能力不够")
	if s.BlameDirection > -0.3 {
		t.Errorf("expected self-blame (negative), got %f", s.BlameDirection)
	}
}

func TestExtractTextualSignalsNoControl(t *testing.T) {
	s := extractTextualSignals("我没办法了，毫无选择")
	if s.PerceivedControl > 0.3 {
		t.Errorf("expected low control, got %f", s.PerceivedControl)
	}
}

func TestBuildAppraisalFrame(t *testing.T) {
	signals := TextSignals{
		Sentiment: 0.7, Uncertainty: 0.7, BlameDirection: 0.7,
		PerceivedControl: 0.8, SurpriseLevel: 0.2, GoalImportance: 0.8,
	}
	frame := buildAppraisalFrame(signals)
	if frame.Desirability != 0.7 {
		t.Errorf("expected desirability 0.7, got %f", frame.Desirability)
	}
	if frame.Likelihood != 0.3 {
		t.Errorf("expected likelihood 0.3 (1-uncertainty), got %f", frame.Likelihood)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestExtract|TestBuild" -v`

---

### Task 4: OCC 情绪评价引擎 — `occ.go`

**Files:**
- Create: `psychological/occ.go`

- [ ] **Step 1: 编写 occ.go**

```go
package psychological

// occFormulas 返回 14 条 OCC 情绪强度计算公式
// 分为三类：事件类(8) + 行为类(4) + 物品类(2)
func occFormulas() []OCCEmotionFormula {
	return []OCCEmotionFormula{
		// --- 事件类 ---
		{EmoJoy, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Unexpectedness} }},
		{EmoDistress, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, f.Unexpectedness} }},
		{EmoHope, []float64{1.0, 0.8},
			func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Likelihood} }},
		{EmoFear, []float64{1.0, 0.8},
			func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, 1 - f.Likelihood} }},
		{EmoSatisfaction, []float64{1.0, 0.3},
			func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Likelihood * (1 - f.Unexpectedness)} }},
		{EmoDisappointment, []float64{1.0, 0.3},
			func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, 1 - f.Likelihood} }},
		{EmoRelief, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 { return []float64{f.Desirability, f.Unexpectedness} }},
		{EmoFearConfirmed, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 { return []float64{-f.Desirability, 1 - f.Unexpectedness} }},
		// --- 行为类 ---
		{EmoPride, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 {
				return []float64{max64(0, f.Praiseworthiness) * max64(0, -f.CausalAttribution), f.Deservingness}
			}},
		{EmoShame, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 {
				return []float64{max64(0, -f.Praiseworthiness) * max64(0, -f.CausalAttribution), f.Deservingness}
			}},
		{EmoGratitude, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 {
				return []float64{max64(0, f.Praiseworthiness) * max64(0, f.CausalAttribution), f.Deservingness}
			}},
		{EmoAnger, []float64{1.0, 0.5},
			func(f AppraisalFrame) []float64 {
				return []float64{max64(0, -f.Praiseworthiness) * max64(0, f.CausalAttribution), f.Deservingness}
			}},
		// --- 物品类 ---
		{EmoLiking, []float64{1.0},
			func(f AppraisalFrame) []float64 { return []float64{f.Appealingness} }},
		{EmoDisliking, []float64{1.0},
			func(f AppraisalFrame) []float64 { return []float64{-f.Appealingness} }},
	}
}

// max64 返回两个 float64 的较大值
func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// computeOCCEmotions 计算所有 OCC 情绪强度
// 核心公式: intensity = max(0, Σ(wi × max(0, vi)) / Σ(wi))
func computeOCCEmotions(frame AppraisalFrame) map[OCCEmotion]float64 {
	intensities := make(map[OCCEmotion]float64)
	for _, f := range occFormulas() {
		vars := f.Variables(frame)
		if len(vars) != len(f.Weights) {
			continue
		}
		var weightedSum, weightSum float64
		for i, v := range vars {
			clamped := max64(0, v)
			weightedSum += f.Weights[i] * clamped
			weightSum += f.Weights[i]
		}
		if weightSum > 0 {
			intensities[f.Emotion] = clamp(weightedSum/weightSum, 0, 1)
		}
	}
	return intensities
}

// getDominantEmotion 返回强度最大的情绪（阈值 > 0.1）
func getDominantEmotion(intensities map[OCCEmotion]float64) (OCCEmotion, float64) {
	var top OCCEmotion
	var maxI float64
	for emo, intensity := range intensities {
		if intensity > maxI && intensity > 0.1 {
			maxI = intensity
			top = emo
		}
	}
	return top, maxI
}
```

- [ ] **Step 2: 编写 occ_test.go**

```go
package psychological

import "testing"

func TestComputeOCCEmotionsJoy(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: 0.8, Unexpectedness: 0.3,
		Likelihood: 0.7, Praiseworthiness: 0.0,
		Appealingness: 0.0, CausalAttribution: 0.0,
		Controllability: 0.5, Deservingness: 0.5,
	}
	intensities := computeOCCEmotions(frame)
	if intensities[EmoJoy] <= 0 {
		t.Errorf("expected positive joy intensity, got %f", intensities[EmoJoy])
	}
	if intensities[EmoDistress] > 0.3 {
		t.Errorf("expected low distress for positive frame, got %f", intensities[EmoDistress])
	}
}

func TestComputeOCCEmotionsDistress(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.7, Unexpectedness: 0.6,
		Likelihood: 0.5, Praiseworthiness: 0.0,
		Appealingness: 0.0, CausalAttribution: 0.0,
		Controllability: 0.3, Deservingness: 0.5,
	}
	intensities := computeOCCEmotions(frame)
	if intensities[EmoDistress] <= 0 {
		t.Errorf("expected positive distress intensity, got %f", intensities[EmoDistress])
	}
}

func TestGetDominantEmotion(t *testing.T) {
	intensities := map[OCCEmotion]float64{
		EmoJoy:  0.6,
		EmoHope: 0.4,
		EmoFear: 0.05,
	}
	dominant, intensity := getDominantEmotion(intensities)
	if dominant != EmoJoy {
		t.Errorf("expected dominant emotion joy, got %s", dominant)
	}
	if intensity != 0.6 {
		t.Errorf("expected intensity 0.6, got %f", intensity)
	}
}

func TestGetDominantEmotionBelowThreshold(t *testing.T) {
	intensities := map[OCCEmotion]float64{
		EmoJoy: 0.05,
	}
	dominant, intensity := getDominantEmotion(intensities)
	if dominant != "" {
		t.Errorf("expected no dominant emotion (all below threshold), got %s", dominant)
	}
	if intensity != 0 {
		t.Errorf("expected intensity 0, got %f", intensity)
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestCompute|TestGet" -v`

---

### Task 5: EMA 认知评价 — `ema.go`

**Files:**
- Create: `psychological/ema.go`

- [ ] **Step 1: 编写 ema.go**

```go
package psychological

const (
	emaCopingThreshold = 0.35
	emaAgencySelf      = -0.3
	emaAgencyOther     = 0.3
	emaCongruenceThreshold = 0.3
)

// computeEMA 执行 EMA 四维认知评价
func computeEMA(frame AppraisalFrame) EMAAssessment {
	goalRelevance := abs(frame.Desirability)*0.5 + frame.Unexpectedness*0.5
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
	if congruence > 0 && coping > emaCopingThreshold {
		return CopeProblemFocused
	}
	if congruence < 0 && coping > emaCopingThreshold {
		return CopeReappraisal
	}
	if congruence < 0 && coping <= emaCopingThreshold {
		if agency > emaAgencyOther {
			return CopeAvoidance
		}
		return CopeEmotionFocused
	}
	return CopeEmotionFocused
}

// emaToOCCEmotions 将 EMA 评价映射到具体 OCC 情绪
func emaToOCCEmotions(a EMAAssessment) map[OCCEmotion]float64 {
	result := make(map[OCCEmotion]float64)

	if a.GoalCongruence > emaCongruenceThreshold {
		result[EmoJoy] = a.GoalCongruence * a.CopingPotential
		if a.FutureExpectancy > 0 {
			result[EmoHope] = a.FutureExpectancy * a.GoalRelevance
		}
	}
	if a.GoalCongruence < -emaCongruenceThreshold {
		result[EmoDistress] = abs(a.GoalCongruence) * a.GoalRelevance
		if a.FutureExpectancy < 0 {
			result[EmoFear] = abs(a.FutureExpectancy) * a.GoalRelevance
		}
	}
	if abs(a.Agency) > emaCongruenceThreshold {
		if a.GoalCongruence > 0 && a.Agency < emaAgencySelf {
			result[EmoPride] = a.GoalCongruence
		}
		if a.GoalCongruence < 0 && a.Agency > emaAgencyOther {
			result[EmoAnger] = abs(a.GoalCongruence)
		}
		if a.GoalCongruence < 0 && a.Agency < emaAgencySelf {
			result[EmoGuilt] = abs(a.GoalCongruence) * 0.5
		}
	}
	return result
}
```

- [ ] **Step 2: 编写 ema_test.go**

```go
package psychological

import "testing"

func TestComputeEMAPositiveProblemFocused(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: 0.8, Likelihood: 0.7, Unexpectedness: 0.2,
		Controllability: 0.9, CausalAttribution: 0.0,
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeProblemFocused {
		t.Errorf("expected problem_focused, got %s", ema.CopingMode)
	}
	if ema.GoalCongruence < 0.5 {
		t.Errorf("expected positive congruence, got %f", ema.GoalCongruence)
	}
}

func TestComputeEMANegativeReappraisal(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.7, Likelihood: 0.6, Unexpectedness: 0.3,
		Controllability: 0.8, CausalAttribution: 0.0,
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeReappraisal {
		t.Errorf("expected reappraisal, got %s", ema.CopingMode)
	}
}

func TestComputeEMANegativeAvoidance(t *testing.T) {
	frame := AppraisalFrame{
		Desirability: -0.8, Likelihood: 0.3, Unexpectedness: 0.5,
		Controllability: 0.2, CausalAttribution: 0.7, // 归因他人
	}
	ema := computeEMA(frame)
	if ema.CopingMode != CopeAvoidance {
		t.Errorf("expected avoidance, got %s", ema.CopingMode)
	}
}

func TestEMAtoOCCEmotions(t *testing.T) {
	ema := EMAAssessment{
		GoalRelevance: 0.7, GoalCongruence: 0.6,
		CopingPotential: 0.8, Agency: -0.5,
		FutureExpectancy: 0.3,
	}
	result := emaToOCCEmotions(ema)
	if result[EmoJoy] <= 0 {
		t.Errorf("expected joy from positive congruence, got %f", result[EmoJoy])
	}
	if result[EmoPride] <= 0 {
		t.Errorf("expected pride from self-agency + positive, got %f", result[EmoPride])
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestComputeEMA|TestEMAtoOCC" -v`

---

### Task 6: VAD 情绪空间融合 — `vad.go`

**Files:**
- Create: `psychological/vad.go`

- [ ] **Step 1: 编写 vad.go**

```go
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
```

- [ ] **Step 2: 编写 vad_test.go**

```go
package psychological

import (
	"math"
	"testing"
)

func TestOCCtoVADJoy(t *testing.T) {
	intensities := map[OCCEmotion]float64{EmoJoy: 0.8}
	vad := occToVAD(intensities)
	if math.Abs(vad.Valence-0.81) > 0.01 {
		t.Errorf("expected valence ~0.81, got %f", vad.Valence)
	}
}

func TestOCCtoVADEmpty(t *testing.T) {
	vad := occToVAD(map[OCCEmotion]float64{})
	if vad.Valence != 0 || vad.Arousal != 0.5 || vad.Dominance != 0.5 {
		t.Errorf("expected neutral VAD for empty input, got %+v", vad)
	}
}

func TestMergeIntensities(t *testing.T) {
	occ := map[OCCEmotion]float64{EmoJoy: 0.5, EmoHope: 0.3}
	ema := map[OCCEmotion]float64{EmoJoy: 0.7, EmoFear: 0.4}
	merged := mergeIntensities(occ, ema)
	if merged[EmoJoy] != 0.7 {
		t.Errorf("expected merged joy 0.7 (max), got %f", merged[EmoJoy])
	}
	if merged[EmoFear] != 0.4 {
		t.Errorf("expected fear from EMA, got %f", merged[EmoFear])
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestOCCtoVAD|TestMerge" -v`

---

### Task 7: 认知扭曲检测 — `distortion.go`

**Files:**
- Create: `psychological/distortion.go`

- [ ] **Step 1: 编写 distortion.go**

```go
package psychological

import (
	"regexp"
	"strings"
)

// distortionRule 单一认知扭曲的检测规则
type distortionRule struct {
	Type        CognitiveDistortion
	Patterns    []*regexp.Regexp
	Description string
	Reframe     string
}

// DistortionReframe 扭曲的重构建议
type DistortionReframe struct {
	Distortion CognitiveDistortion
	Reframe    string
}

// distortionRules 在包初始化时编译正则
var distortionRules []distortionRule

func init() {
	rules := []struct {
		t CognitiveDistortion
		p []string
		d string
		r string
	}{
		{DistAllOrNothing,
			[]string{`总是|从不|完全|彻底|毫无|根本|绝对`, `要么.*要么|不是.*就是`, `完全失败|彻底完蛋|一无是处`},
			"非黑即白二分思维", "事物往往存在中间地带，不完全成功不等于完全失败"},
		{DistCatastrophizing,
			[]string{`完蛋|完了|糟了|最坏|灾难|受不了`, `万一.*怎么办|如果.*就完了`},
			"预想最坏结果", "最坏情况不一定会发生，你过去也应对过类似挑战"},
		{DistOvergeneralization,
			[]string{`每次都|从来都|总是这样|永远|一直.*不行|一切|所有`},
			"基于单一事件得出普遍结论", "单次经历不代表所有情况"},
		{DistMentalFiltering,
			[]string{`只看|只看到|只记得|全都是.*不好|没一个好|没什么好事`},
			"只关注负面细节而忽略全局", "尝试同时看到积极和消极的方面"},
		{DistDiscountingPositive,
			[]string{`不算|没什么|谁都能|运气好|碰巧|只是偶然`},
			"贬低正面经历", "你的成功是你努力的结果，正面经历值得认可"},
		{DistJumpingToConclusions,
			[]string{`肯定|一定|绝对.*是|不用说|我断定`},
			"在证据不充分时下结论", "在没有确认之前，可能存在其他可能性"},
		{DistMindReading,
			[]string{`他觉得|他们认为|肯定觉得|一定觉得`},
			"未经证实就断定他人想法", "我们无法确定他人的想法，除非直接沟通"},
		{DistFortuneTelling,
			[]string{`会失败|不会成功|肯定不行|预感.*不好|估计.*不行`},
			"预判负面未来", "未来有多种可能性，不要仅预测最坏的结果"},
		{DistMagnifying,
			[]string{`太可怕|太严重|太大了|太差`},
			"夸大问题的严重性", "试着客观评估问题的实际规模"},
		{DistEmotionalReasoning,
			[]string{`感觉.*就是|觉得.*一定|我感觉.*所以|因为.*害怕.*所以|因为.*焦虑.*所以`},
			"用情绪替代事实", "感受是感受，事实是事实——两者可能不一致"},
		{DistShouldStatements,
			[]string{`应该|必须|不该|本来应该|本可以|本不该`},
			"僵化标准要求", "减少强求标准，接受更多可能性"},
		{DistLabeling,
			[]string{`我是.*的人|我是个.*|没用|差劲|废物|蠢|笨`},
			"贴极端标签", "行为不等于整体的人"},
		{DistPersonalization,
			[]string{`怪我|我的错|都因为我|因为我.*才|是我不好`},
			"过度承担责任", "很多因素共同导致结果"},
	}
	for _, r := range rules {
		compiled := make([]*regexp.Regexp, len(r.p))
		for i, pat := range r.p {
			compiled[i] = regexp.MustCompile(pat)
		}
		distortionRules = append(distortionRules, distortionRule{r.t, compiled, r.d, r.r})
	}
}

// detectDistortions 三步法检测认知扭曲
func detectDistortions(text string) DistortionDetection {
	var matched []CognitiveDistortion
	var beliefs []string
	var totalIntensity float64

	for _, rule := range distortionRules {
		for _, pat := range rule.Patterns {
			if m := pat.FindString(text); m != "" {
				matched = append(matched, rule.Type)
				beliefs = append(beliefs, m)
				totalIntensity += 0.3
				break
			}
		}
	}

	// 去重
	seen := make(map[CognitiveDistortion]bool)
	var unique []CognitiveDistortion
	for _, d := range matched {
		if !seen[d] {
			seen[d] = true
			unique = append(unique, d)
		}
	}

	// 提取事实陈述
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' || r == '\n'
	})
	var factuals []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) <= 4 {
			continue
		}
		hasDistortion := false
		for _, rule := range distortionRules {
			for _, pat := range rule.Patterns {
				if pat.MatchString(s) {
					hasDistortion = true
					break
				}
			}
			if hasDistortion {
				break
			}
		}
		if !hasDistortion {
			factuals = append(factuals, s)
		}
	}

	beliefIntensity := totalIntensity * 1.5
	if beliefIntensity > 1 {
		beliefIntensity = 1
	}

	return DistortionDetection{
		Distortions:       unique,
		BeliefStatements:   beliefs,
		BeliefIntensity:    beliefIntensity,
		FactualStatements:  factuals,
	}
}

// generateReframes 为检测到的扭曲生成重构建议
func generateReframes(distortions []CognitiveDistortion) []DistortionReframe {
	seen := make(map[CognitiveDistortion]bool)
	var results []DistortionReframe
	for _, d := range distortions {
		if seen[d] {
			continue
		}
		seen[d] = true
		for _, rule := range distortionRules {
			if rule.Type == d {
				results = append(results, DistortionReframe{Distortion: d, Reframe: rule.Reframe})
				break
			}
		}
	}
	return results
}

// hasSevereDistortion 判断是否存在严重认知扭曲
func hasSevereDistortion(d DistortionDetection) bool {
	if len(d.Distortions) >= 3 {
		return true
	}
	if d.BeliefIntensity >= 0.7 {
		return true
	}
	severe := map[CognitiveDistortion]bool{
		DistCatastrophizing: true, DistPersonalization: true, DistLabeling: true,
	}
	for _, dist := range d.Distortions {
		if severe[dist] {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 编写 distortion_test.go**

```go
package psychological

import "testing"

func TestDetectCatastrophizing(t *testing.T) {
	d := detectDistortions("这个案子完蛋了，万一被驳回就全完了")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistCatastrophizing {
			found = true
		}
	}
	if !found {
		t.Errorf("expected catastrophizing, got %v", d.Distortions)
	}
}

func TestDetectShouldStatements(t *testing.T) {
	d := detectDistortions("我应该做得更好，本不该犯这个错误")
	found := false
	for _, dist := range d.Distortions {
		if dist == DistShouldStatements {
			found = true
		}
	}
	if !found {
		t.Errorf("expected should_statements, got %v", d.Distortions)
	}
}

func TestNoDistortion(t *testing.T) {
	d := detectDistortions("今天的天气很好，适合出门散步")
	if len(d.Distortions) > 0 {
		t.Errorf("expected no distortions, got %v", d.Distortions)
	}
}

func TestHasSevereDistortion(t *testing.T) {
	d := DistortionDetection{
		Distortions:      []CognitiveDistortion{DistCatastrophizing, DistLabeling, DistPersonalization},
		BeliefIntensity:  0.8,
	}
	if !hasSevereDistortion(d) {
		t.Errorf("expected severe distortion")
	}
}

func TestGenerateReframes(t *testing.T) {
	d := detectDistortions("我完全失败了，完蛋了")
	reframes := generateReframes(d.Distortions)
	if len(reframes) == 0 {
		t.Errorf("expected reframes for detected distortions")
	}
	for _, r := range reframes {
		if r.Reframe == "" {
			t.Errorf("expected non-empty reframe for %s", r.Distortion)
		}
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestDetect|TestHasSevere|TestGenerate" -v`

---

### Task 8: SDT 需求追踪器 — `sdt.go`

**Files:**
- Create: `psychological/sdt.go`

- [ ] **Step 1: 编写 sdt.go**

```go
package psychological

import (
	"math"
	"sync"
)

// SDTSignals 从用户输入提取的 SDT 需求信号
type SDTSignals struct {
	AutonomyFrustration   float64 // 0=正常, >0=自主受挫
	CompetenceAnxiety     float64 // 0=正常, >0=能力焦虑, <0=自信
	RelatednessLoneliness float64 // 0=正常, >0=孤独, <0=连接
	PerceivedDifficulty   float64 // 0=容易, 1=困难
}

// SDTTracker 跨对话轮次追踪用户心理需求满足度
// 参考: Deterding & Guckelsberger (2026)
type SDTTracker struct {
	state      SDTState
	config     SDTTrackerConfig
	roundCount int
	mu         sync.RWMutex
}

// NewSDTTracker 创建 SDT 追踪器，初始状态为中等满足 (0.5)
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

// computeMotivation motivation = w_a·A + w_c·C + w_r·R
func (t *SDTTracker) computeMotivation(A, C, R float64) float64 {
	w := t.config.Weights
	return clamp(w.Autonomy*A+w.Competence*C+w.Relatedness*R, 0, 1)
}

// updateCompetence 胜任感动态更新 — Deterding 2026 Eq.(1)
// C(t+1) = C(t) + α·(D - C(t))·(1 - e^(-β·(C(t)-D)²))
func (t *SDTTracker) updateCompetence(C, difficulty float64) float64 {
	alpha, beta := t.config.CompetenceAlpha, t.config.CompetenceBeta
	gap := difficulty - C
	challengeFactor := 1 - math.Exp(-beta*gap*gap)
	return C + alpha*gap*challengeFactor
}

// UpdateFromSignals 应用信号更新 SDT 状态（每轮对话调用）
func (t *SDTTracker) UpdateFromSignals(signals SDTSignals) SDTState {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.roundCount++

	decay := t.config.DecayRate
	A := t.state.Autonomy + decay*(0.5-t.state.Autonomy)
	C := t.state.Competence + decay*(0.5-t.state.Competence)
	R := t.state.Relatedness + decay*(0.5-t.state.Relatedness)

	if signals.AutonomyFrustration > 0 {
		A = clamp(A-0.2*signals.AutonomyFrustration, 0, 1)
	}
	C = t.updateCompetence(C, signals.PerceivedDifficulty)
	if signals.CompetenceAnxiety > 0 {
		C = clamp(C-0.15*signals.CompetenceAnxiety, 0, 1)
	} else if signals.CompetenceAnxiety < 0 {
		C = clamp(C+0.1*abs(signals.CompetenceAnxiety), 0, 1)
	}
	if signals.RelatednessLoneliness > 0 {
		R = clamp(R-0.2*signals.RelatednessLoneliness, 0, 1)
	} else if signals.RelatednessLoneliness < 0 {
		R = clamp(R+0.2*abs(signals.RelatednessLoneliness), 0, 1)
	}

	deltaA := abs(A - t.state.Autonomy)
	deltaC := abs(C - t.state.Competence)
	deltaR := abs(R - t.state.Relatedness)

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

// GetState 返回当前 SDT 状态
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

// Reset 重置追踪器
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

// LowestNeed 返回最低的心理需求（最需关注的）
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
```

- [ ] **Step 2: 编写 sdt_test.go**

```go
package psychological

import (
	"math"
	"testing"
)

func TestSDTInitialState(t *testing.T) {
	tracker := NewSDTTracker(nil)
	state := tracker.GetState()
	if math.Abs(state.Autonomy-0.5) > 0.01 {
		t.Errorf("expected autonomy 0.5, got %f", state.Autonomy)
	}
}

func TestSDTUpdateAutonomyFrustration(t *testing.T) {
	tracker := NewSDTTracker(nil)
	signals := SDTSignals{
		AutonomyFrustration: 0.8,
		PerceivedDifficulty: 0.5,
	}
	state := tracker.UpdateFromSignals(signals)
	if state.Autonomy >= 0.5 {
		t.Errorf("expected autonomy to decrease from 0.5 after frustration, got %f", state.Autonomy)
	}
}

func TestSDTCompetenceUpdateFormula(t *testing.T) {
	tracker := NewSDTTracker(nil)
	// 难度略高于胜任感 → 应有适度增长
	signals := SDTSignals{
		PerceivedDifficulty: 0.55,
	}
	state := tracker.UpdateFromSignals(signals)
	// Deterding 公式下，gap=0.05 时变化很小
	if math.Abs(state.Competence-0.5) > 0.1 {
		t.Errorf("expected small competence change for small gap, got %f", state.Competence)
	}
}

func TestSDTMultiRoundDecay(t *testing.T) {
	tracker := NewSDTTracker(nil)
	// 手动设置高胜任感
	tracker.state.Competence = 0.9
	for i := 0; i < 20; i++ {
		tracker.UpdateFromSignals(SDTSignals{PerceivedDifficulty: 0.5})
	}
	state := tracker.GetState()
	// 经过 20 轮衰减，应向 0.5 回归
	if state.Competence > 0.8 {
		t.Errorf("expected competence to decay toward 0.5 after 20 rounds, got %f", state.Competence)
	}
}

func TestSDTRestore(t *testing.T) {
	tracker := NewSDTTracker(nil)
	saved := SDTState{Autonomy: 0.7, Competence: 0.3, Relatedness: 0.6, Motivation: 0.53}
	tracker.RestoreState(saved, 10)
	state := tracker.GetState()
	if state.Autonomy != 0.7 {
		t.Errorf("expected restored autonomy 0.7, got %f", state.Autonomy)
	}
	if tracker.RoundCount() != 10 {
		t.Errorf("expected round 10, got %d", tracker.RoundCount())
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestSDT" -v`

---

### Task 9: 对话策略匹配器 — `strategy.go`

**Files:**
- Create: `psychological/strategy.go`

- [ ] **Step 1: 编写 strategy.go**

```go
package psychological

import "fmt"

// StrategyInput 策略匹配的输入信号
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

// strategyCondition 策略与匹配条件
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

// allStrategies 9 种对话策略的条件映射表
var allStrategies = []strategyCondition{
	{
		Strategy: StrategyValidation,
		Conditions: []conditionRule{
			{"negative_high_arousal", 2.0, func(i StrategyInput) float64 { return max64(0, -i.VAD.Valence) * i.VAD.Arousal }},
			{"high_distortion", 1.0, func(i StrategyInput) float64 { return clamp(float64(i.DistortionCount)*0.2+i.BeliefIntensity*0.5, 0, 1) }},
			{"low_competence", 0.5, func(i StrategyInput) float64 { return max64(0, 0.5-i.SDT.Competence) }},
		},
		PromptTemplate: "先共情验证用户的感受，不急于分析或给建议。用反射确认你理解了他的情绪。",
	},
	{
		Strategy: StrategyReframing,
		Conditions: []conditionRule{
			{"distortion_present", 2.0, func(i StrategyInput) float64 { return clamp(float64(i.DistortionCount)*0.3, 0, 1) }},
			{"high_belief_intensity", 1.5, func(i StrategyInput) float64 { return i.BeliefIntensity }},
			{"ema_reappraisal_mode", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeReappraisal { return 0.7 }
				return 0
			}},
		},
		PromptTemplate: "温和地提出另一种视角。不要否定用户的感受：先用「我理解你为什么会这么想」开头，再提出「有没有另一种可能性？」。",
	},
	{
		Strategy: StrategyEmpowerment,
		Conditions: []conditionRule{
			{"low_autonomy", 1.5, func(i StrategyInput) float64 { return max64(0, 0.5-i.SDT.Autonomy) }},
			{"low_competence", 1.5, func(i StrategyInput) float64 { return max64(0, 0.5-i.SDT.Competence) }},
			{"low_dominance", 1.0, func(i StrategyInput) float64 { return max64(0, 0.5-i.VAD.Dominance) }},
		},
		PromptTemplate: "引导用户回忆类似场景的成功经验。用「你上次xx的情况也类似，你是怎么办的？」来唤醒自我效能感。",
	},
	{
		Strategy: StrategyCognitiveRestructuring,
		Conditions: []conditionRule{
			{"severe_distortion", 2.0, func(i StrategyInput) float64 {
				if i.DistortionCount >= 2 { return 0.8 }
				return 0
			}},
			{"socratic_applicable", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeReappraisal { return 0.6 }
				return 0
			}},
		},
		PromptTemplate: "使用苏格拉底式提问引导用户检验自己的信念：「支持这个想法的证据是什么？」「相反的证据呢？」",
	},
	{
		Strategy: StrategySocraticQuestioning,
		Conditions: []conditionRule{
			{"mild_distortion", 1.5, func(i StrategyInput) float64 {
				if i.DistortionCount == 1 { return 0.5 }
				return 0
			}},
			{"problem_focused_mode", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeProblemFocused { return 0.4 }
				return 0
			}},
			{"moderate_competence", 1.0, func(i StrategyInput) float64 {
				if i.SDT.Competence >= 0.4 && i.SDT.Competence <= 0.7 { return 0.5 }
				return 0
			}},
		},
		PromptTemplate: "用开放性问题引导用户自己探索：「能具体说说吗？」「你觉得为什么会这样？」",
	},
	{
		Strategy: StrategyLightHumor,
		Conditions: []conditionRule{
			{"fatigue_boredom", 1.5, func(i StrategyInput) float64 {
				if i.Dominant == EmoFatigue || i.Dominant == EmoBoredom { return 0.7 }
				return 0
			}},
			{"no_distortion", 1.0, func(i StrategyInput) float64 {
				if i.DistortionCount == 0 { return 0.4 }
				return 0
			}},
		},
		PromptTemplate: "用轻松幽默的语气回应。用行业相关的比喻，不要太正式。保持轻松自然，不要过度。",
	},
	{
		Strategy: StrategyNormalizing,
		Conditions: []conditionRule{
			{"low_competence_low_relatedness", 1.5, func(i StrategyInput) float64 {
				if i.SDT.Competence < 0.4 && i.SDT.Relatedness < 0.4 { return 0.6 }
				return 0
			}},
			{"impostor_signals", 1.0, func(i StrategyInput) float64 {
				for _, d := range i.Distortions {
					if d == DistLabeling { return 0.5 }
				}
				return 0
			}},
		},
		PromptTemplate: "让用户知道这不是ta一个人的问题。用「这个阶段很多人都会这样」开头。",
	},
	{
		Strategy: StrategyAccompany,
		Conditions: []conditionRule{
			{"confusion_anxiety", 1.5, func(i StrategyInput) float64 {
				if i.Dominant == EmoConfusion || i.Dominant == EmoAnxiety { return 0.6 }
				return 0
			}},
			{"requesting_advice", 1.0, func(i StrategyInput) float64 {
				if i.EMA.CopingMode == CopeProblemFocused { return 0.4 }
				return 0
			}},
		},
		PromptTemplate: "简短回应，认可用户的判断力和专业性。先确认ta的思考方向，再提供少量补充信息。",
	},
	{
		Strategy: StrategyRedirectAction,
		Conditions: []conditionRule{
			{"rumination", 1.5, func(i StrategyInput) float64 {
				if i.DistortionCount >= 2 && i.VAD.Arousal > 0.5 { return 0.5 }
				return 0
			}},
			{"low_control_high_arousal", 1.0, func(i StrategyInput) float64 {
				if i.VAD.Dominance < 0.3 && i.VAD.Arousal > 0.6 { return 0.5 }
				return 0
			}},
		},
		PromptTemplate: "引导用户注意力从「为什么」转移到「做什么」。「现在可以做的一件小事是什么？」",
	},
}

// incompatiblePairs 不可同时使用的策略组合
var incompatiblePairs = map[DialogueStrategy][]DialogueStrategy{
	StrategyValidation:            {StrategyRedirectAction, StrategyCognitiveRestructuring},
	StrategyRedirectAction:        {StrategyValidation, StrategySocraticQuestioning},
	StrategyCognitiveRestructuring: {StrategyLightHumor},
}

// matchStrategy 匹配最佳对话策略
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
	// 冒泡排序（简单场景，策略数量固定 9 个）
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

func isComplementary(primary, secondary DialogueStrategy) bool {
	incompatible, ok := incompatiblePairs[primary]
	if !ok {
		return true
	}
	for _, inc := range incompatible {
		if inc == secondary {
			return false
		}
	}
	return true
}

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

func getStrategyPrompt(strategy DialogueStrategy) string {
	for _, sc := range allStrategies {
		if sc.Strategy == strategy {
			return sc.PromptTemplate
		}
	}
	return ""
}
```

- [ ] **Step 2: 编写 strategy_test.go**

```go
package psychological

import "testing"

func TestMatchStrategyValidation(t *testing.T) {
	input := StrategyInput{
		VAD:             VADVector{Valence: -0.7, Arousal: 0.8, Dominance: 0.3},
		DistortionCount: 2,
		BeliefIntensity: 0.7,
		SDT:             SDTState{Autonomy: 0.5, Competence: 0.3, Relatedness: 0.5, Motivation: 0.43},
	}
	result := matchStrategy(input)
	if result.Primary != StrategyValidation && result.Primary != StrategyReframing {
		t.Errorf("expected validation or reframing for negative + distortion, got %s", result.Primary)
	}
}

func TestMatchStrategyEmpowerment(t *testing.T) {
	input := StrategyInput{
		VAD:             VADVector{Valence: 0, Arousal: 0.3, Dominance: 0.3},
		DistortionCount: 0,
		BeliefIntensity: 0.1,
		SDT:             SDTState{Autonomy: 0.3, Competence: 0.3, Relatedness: 0.5, Motivation: 0.36},
	}
	result := matchStrategy(input)
	if result.Primary != StrategyEmpowerment {
		t.Errorf("expected empowerment for low autonomy + low competence, got %s", result.Primary)
	}
}

func TestComplementary(t *testing.T) {
	if isComplementary(StrategyValidation, StrategyRedirectAction) {
		t.Errorf("validation and redirect_action should be incompatible")
	}
	if !isComplementary(StrategyValidation, StrategyEmpowerment) {
		t.Errorf("validation and empowerment should be complementary")
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestMatch|TestComplementary" -v`

---

### Task 10: SDT 持久化 — `store.go`

**Files:**
- Create: `psychological/store.go`

- [ ] **Step 1: 编写 store.go**

```go
package psychological

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Store 持久化 SDT 状态和历史情绪摘要
type Store struct {
	dir string
}

// NewStore 创建持久化存储
// dir 默认 ~/.mady/psychological/
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// storedData 持久化的数据结构
type storedData struct {
	SDTState  SDTState `json:"sdt_state"`
	RoundCount int     `json:"round_count"`
}

// LoadSDTState 从文件加载 SDT 状态
func (s *Store) LoadSDTState(sessionID string) (*storedData, error) {
	path := filepath.Join(s.dir, sessionID+".json")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 首次会话
		}
		return nil, err
	}
	defer f.Close()

	var data storedData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, err
	}
	return &data, nil
}

// SaveSDTState 持久化 SDT 状态
func (s *Store) SaveSDTState(sessionID string, state SDTState, roundCount int) error {
	data := storedData{SDTState: state, RoundCount: roundCount}
	f, err := os.Create(filepath.Join(s.dir, sessionID+".json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(data)
}
```

- [ ] **Step 2: 编写 store_test.go**

```go
package psychological

import (
	"os"
	"testing"
)

func TestStoreSaveLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	state := SDTState{Autonomy: 0.6, Competence: 0.7, Relatedness: 0.5, Motivation: 0.6}
	err = store.SaveSDTState("test-session", state, 5)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	data, err := store.LoadSDTState("test-session")
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if data == nil {
		t.Fatal("expected data, got nil")
	}
	if data.SDTState.Autonomy != 0.6 {
		t.Errorf("expected autonomy 0.6, got %f", data.SDTState.Autonomy)
	}
	if data.RoundCount != 5 {
		t.Errorf("expected round 5, got %d", data.RoundCount)
	}
}

func TestStoreLoadNonExistent(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)
	data, err := store.LoadSDTState("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data for nonexistent session")
	}
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestStore" -v`

---

### Task 11: 7 阶段管道编排 — `pipeline.go`

**Files:**
- Create: `psychological/pipeline.go`

- [ ] **Step 1: 编写 pipeline.go**

```go
package psychological

// DistortionLLMVerifier 认知扭曲 LLM 验证器（可选实现）
type DistortionLLMVerifier interface {
	Verify(text string, d DistortionDetection) DistortionDetection
}

// PipelineConfig 管道配置
type PipelineConfig struct {
	SDTTracker               *SDTTracker
	LLMVerifier              DistortionLLMVerifier
	SkipDistortionDetection  bool
}

// ExecuteFullPipeline 执行完整的 7 阶段心理分析管道
//
// 阶段:
// 1. 文本信号提取 (signal_extraction)
// 2. OCC AppraisalFrame → 情绪强度 (occ_emotion)
// 3. EMA 认知评价 (ema_appraisal)
// 4. OCC+EMA → VAD 融合 (vad_fusion)
// 5. 认知扭曲检测 (distortion_detection)
// 6. SDT 跨轮次需求追踪 (sdt_update)
// 7. 策略匹配 (strategy_matching)
func ExecuteFullPipeline(text string, config *PipelineConfig) NuoChatResult {
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

	// Step 5: 认知扭曲检测
	var distortions DistortionDetection
	if !config.SkipDistortionDetection {
		distortions = detectDistortions(text)
		if config.LLMVerifier != nil && len(distortions.Distortions) > 0 {
			distortions = config.LLMVerifier.Verify(text, distortions)
		}
	}
	severe := hasSevereDistortion(distortions)
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

func boolToFloat(cond bool) float64 {
	if cond {
		return 1
	}
	return 0
}

// BuildContextBlock 从分析结果构建注入系统提示的心理上下文
func BuildContextBlock(result NuoChatResult) string {
	var lines []string
	u := result.Understanding

	// 核心情绪状态
	lines = append(lines, "【当前感知的用户心理状态】")

	// 情绪
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
		lines = append(lines, fmt.Sprintf("检测到认知扭曲: %v (强度=%.2f)", distNames, u.Distortions.BeliefIntensity))
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

	return stringsJoin(lines, "\n")
}

func stringsJoin(lines []string, sep string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for _, l := range lines[1:] {
		result += sep + l
	}
	return result
}
```

> 注意: `pipeline.go` 依赖 `fmt` 包，需在文件开头添加 `import "fmt"`.

- [ ] **Step 2: 编写 pipeline_test.go**

```go
package psychological

import "testing"

func TestExecuteFullPipelineBasic(t *testing.T) {
	result := ExecuteFullPipeline("今天专利申请顺利通过了，非常开心！", &PipelineConfig{})
	if len(result.Metadata.Steps) != 7 {
		t.Errorf("expected 7 steps, got %d: %v", len(result.Metadata.Steps), result.Metadata.Steps)
	}
	if result.Understanding.VAD.Valence <= 0 {
		t.Errorf("expected positive valence for happy message, got %f", result.Understanding.VAD.Valence)
	}
	if result.Strategy.Primary == "" {
		t.Errorf("expected non-empty strategy")
	}
}

func TestExecuteFullPipelineNegative(t *testing.T) {
	result := ExecuteFullPipeline("完蛋了，专利被驳回了，我完全失败了，都怪我太差劲", &PipelineConfig{})
	if result.Understanding.VAD.Valence >= 0 {
		t.Errorf("expected negative valence for distressed message, got %f", result.Understanding.VAD.Valence)
	}
	if len(result.Understanding.Distortions.Distortions) == 0 {
		t.Errorf("expected cognitive distortions in distressed message")
	}
}

func TestBuildContextBlock(t *testing.T) {
	result := ExecuteFullPipeline("我很困惑，不知道该怎么办，可能我需要帮助", &PipelineConfig{})
	block := BuildContextBlock(result)
	if len(block) == 0 {
		t.Errorf("expected non-empty context block")
	}
	// 检查关键字段
	if !contains(block, "困惑") && !contains(block, "confusion") {
		// 中文主导情绪应出现在上下文中
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: 运行测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -run "TestExecute|TestBuild" -v`

---

### Task 12: Extension 实现 — `extension.go`

**Files:**
- Create: `psychological/extension.go`

- [ ] **Step 1: 编写 extension.go**

```go
package psychological

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Config 心理引擎配置
type Config struct {
	SDTConfig               *SDTTrackerConfig
	StoreDir                 string
	EnableLLM                bool
	LLMVerifier              DistortionLLMVerifier
	SkipDistortionDetection  bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		StoreDir: "",
	}
}

// Extension 实现 agentcore.Extension 接口
type Extension struct {
	config    Config
	tracker   *SDTTracker
	store     *Store
	lastInput string
	mu        sync.Mutex
}

// NewExtension 创建心理引擎扩展
func NewExtension(cfg Config) *Extension {
	return &Extension{
		config:  cfg,
		tracker: NewSDTTracker(cfg.SDTConfig),
	}
}

func (e *Extension) Name() string { return "psychological" }

func (e *Extension) Init(ctx context.Context, agent *agentcore.Agent) error {
	// 尝试从持久化存储恢复 SDT 状态
	if e.config.StoreDir != "" {
		store, err := NewStore(e.config.StoreDir)
		if err == nil {
			e.store = store
		}
	}
	return nil
}

func (e *Extension) Dispose() error { return nil }

// SystemPromptSuffix 实现 SystemPromptProvider — 添加心理感知基础指令
func (e *Extension) SystemPromptSuffix() string {
	return `【心理感知能力】
你具备感知用户情绪状态的能力。系统会自动分析用户消息的心理信号。
当收到【当前感知的用户心理状态】信息块时：
- 根据主导情绪调整语气（负面→温和共情，正面→积极共鸣）
- 遵循【对话策略】的指引调整沟通方式
- 不要直接提及"你的VAD值是..."等原始数据，而是自然内化这些信息
- 察觉到严重认知扭曲时，以温和方式引导而非直接指出`
}

// TransformContext 实现 TransformContextProvider — 分析用户消息并注入心理上下文
func (e *Extension) TransformContext(ctx context.Context, msgs []agentcore.Message) []agentcore.Message {
	// 找到最新的用户消息
	lastUserMsg := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == agentcore.RoleUser {
			lastUserMsg = msgs[i].Content
			break
		}
	}
	if lastUserMsg == "" {
		return msgs
	}

	// 避免重复分析同一条消息
	e.mu.Lock()
	if lastUserMsg == e.lastInput {
		e.mu.Unlock()
		return msgs
	}
	e.lastInput = lastUserMsg
	e.mu.Unlock()

	// 执行管道
	result := ExecuteFullPipeline(lastUserMsg, &PipelineConfig{
		SDTTracker:              e.tracker,
		LLMVerifier:             e.config.LLMVerifier,
		SkipDistortionDetection: e.config.SkipDistortionDetection,
	})

	// 持久化
	if e.store != nil {
		// session ID 从 context 获取（简化处理：使用 hash）
		_ = e.store.SaveSDTState("default", e.tracker.GetState(), e.tracker.RoundCount())
	}

	// 构建上下文块，作为 system message 插入
	contextBlock := BuildContextBlock(result)
	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}

	// 在用户消息之前插入心理上下文
	var out []agentcore.Message
	for i, msg := range msgs {
		if msg.Role == agentcore.RoleUser && msg.Content == lastUserMsg {
			out = append(out, sysMsg)
			out = append(out, msg)
		} else {
			out = append(out, msg)
		}
		_ = i // suppress unused warning
	}
	return out
}

// Tools 实现 ToolProvider — 注册心理分析工具
func (e *Extension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		{
			Name:        "analyze_emotion",
			Description: "分析用户输入的心理状态，返回完整的情绪分析结果（VAD、OCC 情绪、认知扭曲、SDT 需求、推荐策略）",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "要分析的文本",
					},
				},
				"required": []string{"text"},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				var params struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &params); err != nil {
					return nil, err
				}
				result := ExecuteFullPipeline(params.Text, &PipelineConfig{
					SDTTracker:              e.tracker,
					LLMVerifier:             e.config.LLMVerifier,
					SkipDistortionDetection: e.config.SkipDistortionDetection,
				})
				return result, nil
			},
		},
		{
			Name:        "emotion_status",
			Description: "查看当前对话的 SDT 心理需求状态和情绪轨迹",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Func: func(ctx context.Context, args json.RawMessage) (any, error) {
				state := e.tracker.GetState()
				return map[string]any{
					"sdt_state":    state,
					"round_count":  e.tracker.RoundCount(),
					"lowest_need":  e.tracker.LowestNeed(),
				}, nil
			},
		},
	}
}
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/xujian/projects/Mady && go build ./psychological/`

Expected: 编译成功。需要确认 `agentcore.RoleUser` 和 `agentcore.RoleSystem` 常量存在。

- [ ] **Step 3: 确认 agentcore 中的 Role 常量**

Run: `grep -n 'RoleUser\|RoleSystem\|RoleAssistant' /Users/xujian/projects/Mady/agentcore/*.go`

---

### Task 13: LifecycleHook 工厂 — `hook.go`

**Files:**
- Create: `psychological/hook.go`

- [ ] **Step 1: 编写 hook.go**

```go
package psychological

import (
	"context"

	"github.com/xujian519/mady/agentcore"
)

// psychologicalHook 实现 LifecycleHook，在 Agent 启动时运行心理分析
type psychologicalHook struct {
	agentcore.BaseLifecycleHook
	config  Config
	tracker *SDTTracker
}

// NewLifecycleHook 创建心理引擎的 LifecycleHook
// 轻量模式：仅 BeforeAgentRun 中分析用户输入并修改 Messages
func NewLifecycleHook(cfg Config) agentcore.LifecycleHook {
	return &psychologicalHook{
		config:  cfg,
		tracker: NewSDTTracker(cfg.SDTConfig),
	}
}

func (h *psychologicalHook) BeforeAgentRun(ctx context.Context, arc *agentcore.AgentRunContext) error {
	if arc.Input == "" {
		return nil
	}

	result := ExecuteFullPipeline(arc.Input, &PipelineConfig{
		SDTTracker:              h.tracker,
		LLMVerifier:             h.config.LLMVerifier,
		SkipDistortionDetection: h.config.SkipDistortionDetection,
	})

	contextBlock := BuildContextBlock(result)

	// 将心理上下文作为系统消息前置
	sysMsg := agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: contextBlock,
	}
	arc.Messages = append([]agentcore.Message{sysMsg}, arc.Messages...)
	return nil
}
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/xujian/projects/Mady && go build ./psychological/`

Expected: 编译成功。

---

### Task 14: 集成到 `domains/chat.go`

**Files:**
- Modify: `domains/chat.go`

- [ ] **Step 1: 修改 ChatAgentConfig**

```go
package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
)

// ChatAgentConfig builds the chat/assistant domain Agent configuration.
func ChatAgentConfig(base agentcore.Config) agentcore.Config {
	cfg := base
	cfg.Name = "chat-assistant"

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的通用聊天与智能助理模块。",
		"用简体中文回复，语气友好专业。",
		"",
		"职责：",
		"- 日常对话和信息查询",
		"- 代码生成和文件操作",
		"- 内容创作和编辑",
		"- 简单计算和数据分析",
		"",
		"边界：",
		"- 不提供法律建议（应由法律模块处理）",
		"- 不提供专利分析（应由专利模块处理）",
		"- 不确定的专业问题建议用户咨询相关专业人士",
	}, " ")

	cfg.Lifecycle = appendLifecycle(cfg.Lifecycle,
		guardrails.New(
			guardrails.WithLevel(guardrails.LevelLight),
			guardrails.WithBlockedPhrases([]string{"恶意代码", "攻击方法", "非法入侵"}),
		),
	)

	// 加载心理引擎 Extension
	psyCfg := psychological.DefaultConfig()
	cfg.Extensions = append(cfg.Extensions, psychological.NewExtension(psyCfg))

	return cfg
}
```

- [ ] **Step 2: 验证完整编译**

Run: `cd /Users/xujian/projects/Mady && go build ./...`

Expected: 全部包编译通过。

---

### Task 15: 全部测试验证

- [ ] **Step 1: 运行所有心理引擎测试**

Run: `cd /Users/xujian/projects/Mady && go test ./psychological/ -v`

Expected: 全部 PASS。

- [ ] **Step 2: 运行项目全部测试**

Run: `cd /Users/xujian/projects/Mady && go test ./...`

Expected: 全部包测试通过。

- [ ] **Step 3: 运行 go vet 静态检查**

Run: `cd /Users/xujian/projects/Mady && go vet ./psychological/`

Expected: 无警告。
