# NuoChat 心理引擎 — Go 语言移植与集成设计

> 日期: 2026-07-10
> 状态: 待审核
> 来源: 从 XiaoNuo Agent 项目 (`@nuo/nuochat-agent`) 的 TypeScript 实现完整移植到 Go

---

## 一、动机与目标

### 1.1 业务价值

当前 Mady 的 chat 领域是一个标准的 LLM 对话助手，能回答问题但**不理解用户的情绪状态**。
引入 nuochat 的心理引擎后，Agent 获得以下能力：

- **情绪感知**：识别用户的愉悦度（valence）、唤醒度（arousal）、支配度（dominance）
- **认知健康监测**：检测 13 种 Beck CBT 认知扭曲（灾难化、非黑即白、过度泛化等）
- **自适应对话策略**：根据用户心理状态自动切换 9 种对话策略（共情验证、认知重构、赋能引导等）
- **长期关系建立**：通过 SDT 跨轮次追踪，感知用户的心理需求变化轨迹

这对**沉浸式用户体验**至关重要——用户不会感觉自己在对着一面墙说话，而是与一个能"读懂"ta 的智能体交流。

### 1.2 为什么选择移植而非直接引用

| 因素 | 说明 |
|------|------|
| **语言鸿沟** | nuochat 是 TypeScript，Mady 是 Go，无法直接引用 |
| **依赖简化** | nuochat 依赖 `@nuo/ai`, `@nuo/agent-core` 等 8+ 个 npm 包，Mady 仅需标准库 |
| **架构一致性** | Mady 追求极简外部依赖（仅 gorilla/websocket），心理引擎全部使用 Go 标准库 |
| **算法可控** | 核心算法（OCC/EMA/VAD/SDT/CBT）是纯数学模型，适合 Go 重写，无黑盒依赖 |

---

## 二、整体架构

### 2.1 包结构

```
mady/
  psychological/              # 新增包：心理引擎核心
    clamp.go                  # 工具函数 (clamp/abs/min/max)
    types.go                  # 所有类型定义 + 常量 + 映射表
    occ.go                    # OCC 情绪评价引擎 (14 种情绪 + AppraisalFrame)
    ema.go                    # EMA 认知评价 (四维评价 + 应对模式)
    vad.go                    # VAD 情绪空间融合 (OCC → VAD 加权映射)
    distortion.go             # 认知扭曲检测 (13 种 Beck CBT + 可选 LLM 验证)
    sdt.go                    # SDT 需求追踪器 (Deterding 2026 动态公式)
    strategy.go               # 对话策略匹配器 (9 种策略-条件映射)
    pipeline.go               # 7 阶段管道编排 (executeFullPipeline)
    signal.go                 # 文本信号提取 (关键词规则引擎)
    extension.go              # agentcore.Extension 实现
    hook.go                   # LifecycleHook 工厂函数
    store.go                  # SDT 状态持久化 (JSON 文件)
    *_test.go                 # 单元测试
```

### 2.2 依赖关系

```
psychological/
  ├── 标准库: regexp, math, sync, encoding/json, os
  ├── 读取/写入: store/          (SDT JSON 文件持久化)
  ├── 作为扩展注册: agentcore/    (Extension + LifecycleHook 接口)
  └── 可选 LLM 验证: provider/   (认知扭曲 LLM 二次验证，可选)
```

### 2.3 数据流

```
用户消息
    │
    ▼
LifecycleHook.BeforeAgentStart() / Extension.beforeAgentStart()
    │
    ▼
executeFullPipeline(userMessage):
    │
    ├──[1] extractTextualSignals(text)
    │       └── TextSignals{6 个量化维度}
    │
    ├──[2] buildAppraisalFrame(signals)
    │       └── AppraisalFrame{8 个评价维度}
    │       │
    │       ├── computeOCCEmotions(frame)
    │       │   └── 14 种情绪强度
    │       │
    │       └── computeEMA(frame)
    │           ├── EMAAssessment{5 个评价 + 应对模式}
    │           └── emaToOCCEmotions(ema)
    │               └── EMA 情绪映射
    │
    ├──[3] mergeIntensities(occ, emaMapped) → merged
    │       └── occToVAD(merged)
    │           └── VADVector{Valence, Arousal, Dominance}
    │
    ├──[4] detectDistortions(text)
    │       └── DistortionDetection{扭曲类型, 信念陈述, 强度}
    │
    ├──[5] sdtTracker.UpdateFromSignals(sdtSignals)
    │       └── SDTState{自主性, 胜任感, 归属感, 动机}
    │
    ├──[6] matchStrategy(StrategyInput{...})
    │       └── StrategyMatch{主策略, 辅策略, 置信度, prompt}
    │
    └── 输出: NuoChatResult → 注入心理上下文到系统提示
```

### 2.4 集成到 `domains/chat.go`

```go
func ChatAgentConfig(base agentcore.Config) agentcore.Config {
    cfg := base
    cfg.Name = "chat-assistant"
    // ... 现有 system prompt ...

    // 方式一：Extension（推荐，功能完整）
    cfg.Extensions = append(cfg.Extensions,
        psychological.NewExtension(psychological.DefaultConfig()),
    )

    // 方式二：LifecycleHook（轻量，仅需 BeforeAgentStart）
    // psyHook := psychological.NewLifecycleHook(psychological.DefaultConfig())
    // cfg.Lifecycle = appendLifecycle(cfg.Lifecycle, psyHook)

    return cfg
}
```

---

## 三、核心类型定义 (`types.go`)

### 3.1 VAD 三维情绪空间

```go
type VADVector struct {
    Valence   float64 // -1.0 ~ 1.0 (负面→正面)
    Arousal   float64 //  0.0 ~ 1.0 (平静→激动)
    Dominance float64 //  0.0 ~ 1.0 (失控→掌控)
}
```

每种 OCC 情绪有预定义的 VAD 中心坐标（22 种映射），加权平均后得到连续空间的精准位置。

### 3.2 OCC 情绪类型

```go
type OCCEmotion string

const (
    EmoJoy, EmoDistress, EmoHope, EmoFear,
    EmoSatisfaction, EmoDisappointment, EmoRelief, EmoFearConfirmed,
    EmoPride, EmoShame, EmoGratitude, EmoAnger, EmoGuilt,
    EmoAdmiration, EmoReproach, EmoLiking, EmoDisliking,
    EmoFrustration, EmoAnxiety, EmoFatigue, EmoBoredom, EmoConfusion
)
```

### 3.3 评价框架

```go
type AppraisalFrame struct {
    Desirability       float64 // 期望值 (-1~1)
    Likelihood         float64 // 可能性 (0~1)
    Praiseworthiness   float64 // 赞扬性 (-1~1)
    Deservingness      float64 // 应得性 (0~1)
    Appealingness      float64 // 吸引力 (-1~1)
    Unexpectedness     float64 // 出乎意料 (0~1)
    CausalAttribution  float64 // 因果归因 (-1=自己, 0=环境, 1=他人)
    Controllability    float64 // 可控性 (0~1)
}
```

### 3.4 EMA 认知评价

```go
type EMAAssessment struct {
    GoalRelevance    float64   // 目标相关性 (0~1)
    GoalCongruence   float64   // 目标一致性 (-1~1)
    CopingPotential  float64   // 应对能力 (0~1)
    Agency           float64   // 因果归因 (-1~1)
    FutureExpectancy float64   // 未来预期 (-1~1)
    CopingMode       CopingMode // 应对策略
}

type CopingMode string
const (
    CopeProblemFocused CopingMode = "problem_focused"
    CopeEmotionFocused CopingMode = "emotion_focused"
    CopeAvoidance      CopingMode = "avoidance"
    CopeReappraisal    CopingMode = "reappraisal"
)
```

### 3.5 认知扭曲

```go
type CognitiveDistortion string
const (
    DistAllOrNothing, DistCatastrophizing, DistOvergeneralization,
    DistMentalFiltering, DistDiscountingPositive, DistJumpingToConclusions,
    DistMindReading, DistFortuneTelling, DistMagnifying,
    DistEmotionalReasoning, DistShouldStatements, DistLabeling,
    DistPersonalization
)

type DistortionDetection struct {
    Distortions       []CognitiveDistortion
    BeliefStatements  []string
    BeliefIntensity   float64
    FactualStatements []string
}
```

### 3.6 SDT 状态

```go
type SDTState struct {
    Autonomy        float64 // 自主性 (0~1)
    Competence      float64 // 胜任感 (0~1)
    Relatedness     float64 // 归属感 (0~1)
    Motivation      float64 // 综合动机 (0~1)
    LastUpdatedNeed string  // 最近变化的需求
}
```

### 3.7 对话策略

```go
type DialogueStrategy string
const (
    StrategyValidation, StrategyReframing, StrategyEmpowerment,
    StrategyLightHumor, StrategyAccompany, StrategyNormalizing,
    StrategyRedirectAction, StrategyCognitiveRestructuring,
    StrategySocraticQuestioning
)

type StrategyMatch struct {
    Primary        DialogueStrategy
    Secondary      *DialogueStrategy // nil = 无辅策略
    Confidence     float64
    Rationale      string
    StrategyPrompt string
}
```

### 3.8 管道综合结果

```go
type NuoChatResult struct {
    Understanding DialogueUnderstanding
    Strategy      StrategyMatch
    Metadata      PipelineMetadata
}

type DialogueUnderstanding struct {
    VAD                VADVector
    DominantEmotion    OCCEmotion
    EmotionIntensities map[OCCEmotion]float64
    Distortions        DistortionDetection
    SDT                SDTState
    Appraisal          EMAAssessment
}
```

---

## 四、算法引擎

### 4.1 `occ.go` — OCC 情绪评价

**核心公式:**
```
intensity(emotion) = max(0, Σ(wi × max(0, vi)) / Σ(wi))
```

**14 种情绪公式分为三类:**

| 类别 | 情绪 | 评价变量 |
|------|------|---------|
| 事件类 | joy | desirability + unexpectedness |
| | distress | -desirability + unexpectedness |
| | hope | desirability + likelihood |
| | fear | -desirability + (1-likelihood) |
| | satisfaction | desirability + likelihood×(1-unexpectedness) |
| | disappointment | -desirability + (1-likelihood) |
| | relief | desirability + unexpectedness |
| | fear_confirmed | -desirability + (1-unexpectedness) |
| 行为类 | pride | praiseworthiness×self-attribution + deservingness |
| | shame | -praiseworthiness×self-attribution + deservingness |
| | gratitude | praiseworthiness×other-attribution + deservingness |
| | anger | -praiseworthiness×other-attribution + deservingness |
| 物品类 | liking | appealingness |
| | disliking | -appealingness |

**设计要点：**
- 公式通过闭包函数 (`Variables`) 提取评价维度
- `max(0, vi)` 确保负值变量不参与反向情绪计算
- 输出裁剪到 [0, 1]

### 4.2 `ema.go` — EMA 认知评价

**四维评价计算:**
```
goal_relevance    = |desirability| × 0.5 + unexpectedness × 0.5
goal_congruence   = desirability
coping_potential  = controllability × likelihood
agency            = causal_attribution
future_expectancy = desirability + likelihood - 1
```

**应对模式决策树:**
```
if congruence > 0  AND coping > 0.35 → problem_focused
if congruence < 0  AND coping > 0.35 → reappraisal
if congruence < 0  AND coping ≤ 0.35:
    if agency > 0.3 → avoidance   (因在他人 → 回避)
    else            → emotion_focused (因在自己/环境)
default → emotion_focused
```

**EMA → OCC 情绪映射:**
- goal_congruence > 0.3 → joy, hope
- goal_congruence < -0.3 → distress, fear
- |agency| > 0.3 时: pride (自己+正面), anger (他人+负面), guilt (自己+负面)

### 4.3 `vad.go` — VAD 融合

**加权平均公式:**
```
VAD = Σ(intensity_i × VAD_center_i) / Σ(intensity_i)
```

- 每种 OCC 情绪有预定义的 VAD 中心坐标 (22 种映射)
- 仅当 intensity > 0 时参与加权
- 总权重为 0 时返回中性默认值 {0, 0.5, 0.5}
- 输出 Valence 裁剪到 [-1, 1], Arousal/Dominance 裁剪到 [0, 1]

**OCC + EMA 合并策略:**
```
merged[emotion] = max(occIntensity[emotion], emaIntensity[emotion])
```
两套互补覆盖——OCC 完整覆盖 14 种情绪，EMA 捕捉 joy/distress/fear/pride/anger/guilt。

### 4.4 `signal.go` — 文本信号提取

基于中英文关键词正则规则，从自然语言文本中提取 6 个量化信号：

| 信号 | 范围 | 提取逻辑 |
|------|------|---------|
| Sentiment | -1 ~ 1 | 强负面词(-0.7) > 强正面词(0.7) > 弱负面(-0.5) > 弱正面(0.4) > 默认(0) |
| Uncertainty | 0 ~ 1 | 不确定词匹配 → 0.7, 否则 0.2 |
| BlameDirection | -1 ~ 1 | 自归因词 → -0.6, 他归因词 → 0.7, 否则 0 |
| PerceivedControl | 0 ~ 1 | 无助词 → 0.2, 掌控词 → 0.8, 否则 0.5 |
| SurpriseLevel | 0 ~ 1 | 意外词 → 0.8, 否则 0.2 |
| GoalImportance | 0 ~ 1 | 重要性词 → 0.8, 否则 0.4 |

### 4.5 `distortion.go` — 认知扭曲检测

**三步法 (参考 Diagnosis-of-Thought):**

1. **模式匹配**：13 个扭曲规则各含 2-3 个中英文正则，编译在 `init()` 中全局共享
2. **信念提取**：捕获每个匹配的原文片段
3. **事实分离**：拆分句子，标记不含扭曲关键词的为事实陈述

**严重扭曲判断:**
- 检测到 ≥3 种扭曲
- 信念强度 ≥ 0.7
- 包含灾难化/过度自责/贴标签

**可选 LLM 二次验证:**
- 输入 `DistortionVerifier` 接口
- LLM 返回【确认】/【否认】/【存疑】
- 仅保留【确认】条目，降低误报

### 4.6 `sdt.go` — SDT 需求追踪器

**核心公式 (Deterding 2026):**
```
C(t+1) = C(t) + α·(D - C(t))·(1 - e^(-β·(C(t)-D)²))
```

其中:
- C = 胜任感, D = 感知难度
- α = 学习率 (默认 0.3)
- β = 挑战-技能匹配敏感度 (默认 0.1)
- 当难度与胜任感匹配时 (C ≈ D)，学习率最大 → 最优挑战区

**动机综合:**
```
motivation = w_a·A + w_c·C + w_r·R
```
默认权重: {autonomy: 0.33, competence: 0.34, relatedness: 0.33}

**时间衰减:**
每轮对话，所有需求向 0.5 中性值衰减 5%：`X += decayRate × (0.5 - X)`

**并发安全:** SDTTracker 内部使用 `sync.RWMutex`

### 4.7 `strategy.go` — 策略匹配器

**9 种策略的条件映射:**

| 策略 | 理论基础 | 匹配条件 |
|------|---------|---------|
| validation | Rogers 人本主义 | VAD 负面+高唤醒, 高认知扭曲, 低胜任感 |
| reframing | Beck CBT | 认知扭曲存在, 高信念强度, reappraisal 模式 |
| empowerment | Bandura 自我效能 | 低自主性, 低胜任感, 低支配度 |
| cognitive_restructuring | CBT | 严重扭曲 (≥2), reappraisal 模式 |
| socratic_questioning | CBT/MultiAgentESC | 轻度扭曲 (1种), problem_focused 模式 |
| light_humor | - | fatigue/boredom 主导情绪, 无扭曲 |
| normalizing | 社会比较理论 | 低胜任感+低归属感, labeling 扭曲 |
| accompany | - | confusion/anxiety 主导, problem_focused |
| redirect_action | Flow Theory | 反刍 (≥2 扭曲+高唤醒), 低支配+高唤醒 |

**选择逻辑:**
1. 计算每个策略的每个条件的加权激活分数
2. 取加权平均作为策略得分
3. 排序 → 得分最高的为主策略 (阈值 > 0.15)
4. 次高分且互补的为辅策略 (阈值 > 0.2)
5. 不可组合对: validation↔redirect_action, validation↔cognitive_restructuring, cognitive_restructuring↔light_humor

---

## 五、集成层

### 5.1 `pipeline.go` — 7 阶段管道

```go
func executeFullPipeline(
    text        string,
    tracker     *SDTTracker,
    llmVerifier DistortionLLMVerifier,
) NuoChatResult
```

1. signal_extraction → 2. occ_emotion → 3. ema_appraisal → 4. vad_fusion
→ 5. distortion_detection → 6. sdt_update → 7. strategy_matching

### 5.2 `extension.go` — Extension 实现

实现 `agentcore.Extension` 接口:
- `Name()` → "psychological"
- `Register(ctx, agent)` → 注册 2 个工具 + `before_agent_start` 钩子
- `beforeAgentStart(ctx, input)` → 触发完整管道 + 构建心理上下文 + 持久化 SDT

**注册的工具:**
1. `analyze_emotion` — 手动触发情绪分析（返回完整 NuoChatResult JSON）
2. `emotion_status` — 查询当前 SDT 状态和情绪轨迹

### 5.3 `hook.go` — LifecycleHook 快捷方式

```go
func NewLifecycleHook(cfg Config) agentcore.LifecycleHook
```

轻量模式：仅实现 `BeforeAgentStart`，不注册工具，适合简单场景。

### 5.4 `store.go` — 持久化

- 单 JSON 文件路径: `~/.mady/psychological/{session_id}.json`
- 存储: SDTState + 最后 N 轮对话的情绪摘要
- 跨会话恢复: `LoadSDTState(sessionID)` → 恢复追踪器

---

## 六、配置

```go
type Config struct {
    // SDT 追踪器配置
    SDTConfig *SDTTrackerConfig

    // 可选 LLM 验证（认知扭曲二次验证）
    EnableLLM   bool
    LLMVerifier DistortionLLMVerifier

    // SDT 持久化
    StoreDir string // 默认 ~/.mady/psychological/

    // 管道控制
    SkipDistortionDetection bool // 跳过认知扭曲检测（性能优化）
    PipelineTimeout         time.Duration // 默认 2s
}

func DefaultConfig() Config
```

---

## 七、测试策略

| 文件 | 测试覆盖 |
|------|---------|
| `occ_test.go` | 14 种情绪公式等价性（与 TypeScript 版本交叉验证）、边界值（全0/全1/全负）、负值裁剪 |
| `ema_test.go` | 4 种应对模式分支覆盖、EMA → OCC 映射完整性、归一化验证 |
| `vad_test.go` | 单情绪映射、多情绪加权平均、空输入默认值、极值裁剪 |
| `distortion_test.go` | 每种扭曲独立检测、去重、事实分离、严重扭曲判定、mock LLM 验证 |
| `sdt_test.go` | Deterding 公式数学验证（手工计算预期值）、10 轮对话追踪、衰减收敛验证 |
| `strategy_test.go` | 9 种策略独立匹配、互补判断、组合禁忌、阈值边界 |
| `pipeline_test.go` | 端到端管道、中文/英文/混合输入、nil SDT 追踪器回退 |
| `signal_test.go` | 中英文关键词覆盖、优先级排序、边界输入 |

---

## 八、与 Mady 现有模块的关系

| Mady 模块 | 关系 | 说明 |
|-----------|------|------|
| `domains/chat.go` | **直接集成** | 通过 Extension 或 LifecycleHook 挂载 |
| `domains/patent.go` | **可选集成** | 专利代理也可受益于情绪感知（客户焦虑/沮丧） |
| `domains/legal.go` | **可选集成** | 法律咨询的情绪感知同理 |
| `guardrails/` | **互补** | 心理引擎可与护栏联动——严重负面情绪触发更高级别的干预 |
| `session/` | **关联** | SDT 状态按 session ID 持久化 |
| `agentcore/` | **实现接口** | Extension + LifecycleHook |
| `provider/` | **可选依赖** | 仅 LLM 二次验证时使用 |

---

## 九、风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 关键词规则覆盖不足 | 中文文本信号提取不准确 | 保留 LLM 验证扩展点，可后续升级为 LLM 驱动的信号提取 |
| SDT 跨轮次追踪偏差 | 长期对话中需求估计漂移 | 内置时间衰减 + 可重置 + 0.5 引力中心 |
| 认知扭曲对专业术语误报 | 专利/法律语言中"必须"/"应该"被误标 | 默认仅正则检测 + 可选 LLM 二次验证 |
| 性能影响 | 每次对话增加管道延迟 | 管道 < 2ms（纯正则 + 浮点运算）；LLM 验证异步可选 |

---

## 十、未来扩展方向

1. **LLM 驱动的信号提取** — 替代关键词规则，提升准确率
2. **情绪可视化仪表盘** — TUI 中展示 VAD 轨迹和 SDT 变化图
3. **人格模型集成** — Big Five (OCEAN) 人格维度
4. **多模态情绪感知** — 语音语调分析（需要额外服务）
5. **群体情绪分析** — 多用户会话的情绪聚合
