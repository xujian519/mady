# psychological/ 心理引擎审阅报告 — 2026-07-25

> Phase 2 子审阅（R9）｜依据：`Mady 全面审阅计划 v1.0` ｜执行者：AI（Grok）｜Human Owner：[NEEDS CLARIFICATION]
> 验证：本报告颠覆性结论（"仅 VAD 单模型"）已由主审阅员 grep 二次核实

## 摘要（3 条最关键发现）

1. **【P1｜认知鸿沟】任务描述的 OCC/EMA/SDT/CBT 五大模型在本模块根本不存在**——实际仅实现 VAD 一个模型（grep `OCCEmotion|EMAAssessment|SDTState|CognitiveDistortion` 全仓 0 命中）。phase5 历史发现的"EmoRelief=EmoJoy 同权重""SDTWeights 未归一化"问题**在代码中无从验证**——它们只活在未落地的设计稿（`docs/superpowers/specs/2026-07-10-...design.md`，状态"待审核"）里。这是 Phase 2 最重大的发现之一。
2. **【P1｜数学 bug】calming 策略置信度公式方向反了**（`engine.go:229-238`）：本应是"负面情绪越强，越该高置信地启动安抚"，当前公式却让极端愤怒（Valence→-1）的置信度塌缩到下限 0.5。属实质逻辑缺陷。
3. **【P1｜死代码 + 死开关】`hook.go` 整文件死代码**（`NewLifecycleHook` 全仓 0 调用）+ **`SkipDistortionDetection` 是无效死开关**（CBT 模型未实现却暴露配置，domains 层 4 个域的 true/false 差异 100% 无效）。这是"文档-代码"认知鸿沟的最差状态。

## 1. 审阅范围

| 文件 | 行数 | 性质 |
|------|------|------|
| `psychological/types.go` | 83 | VAD 相关类型 |
| `psychological/engine.go` | 290 | VAD 计算 + 策略匹配 |
| `psychological/hook.go` | 40 | LifecycleHook（**死代码**） |
| `psychological/extension.go` | 110 | Extension 装配 |
| `psychological/psychological_test.go` | ~102 | 测试 |
| **合计** | **非测试 523 行 + 测试 102 行** | |

**导出类型全部围绕 VAD**：VADVector/TextSignals/EmotionProfile/DialogueStrategy/StrategyMatch/NuoChatResult/PipelineMetadata/PipelineConfig/Config/Extension/psychologicalHook。无任何 OCC/EMA/SDT/CBT 类型。

## 2. 审阅维度执行情况（5 Lens 表格）

| Lens | 结论 |
|------|------|
| Lens-1 Go 规范 | 🟡 良好但有小问题：错误处理✓、无 panic✓、并发锁正确✓；TransformContext 去重可能误伤合法重复输入（F8） |
| Lens-2 架构分层 | 🔴 hook.go 整文件死代码（F4）；Extension 与 Hook 若同时启用会双重注入（F7） |
| Lens-3 数学模型 | 🔴 **仅 VAD 可核**：VAD 权重归一化正确✓；calming 置信度公式反向（F1）；词表"满意"重复双计（F5）；OCC/EMA/SDT/CBT **无法核验（未实现）** |
| Lens-4 测试 | 🔴 hook.go/extension.go 零测试（F3）；无任何数学边界/单调性/归一化专项测试 |
| Lens-5 核心理念 | 🔴 SkipDistortionDetection 死开关（F2）；设计稿（5 模型）与实现（1 模型）严重背离（F6）；但单文件行数克制✓ |

## 3. 发现清单

| ID | 风险等级 | 类别 | 证据(文件:行) | 规范条款 | 建议 |
|----|---------|------|--------------|---------|------|
| **F1** | **H** | Lens-3 数学 bug | `engine.go:229-238`：`Confidence: clip(0.5+(v.Arousal*(1+v.Valence)/2), 0.5, 0.95)`。Valence∈(-1,-0.4) 时 `(1+Valence)/2∈(0,0.3)`，**Valence→-1 置信度塌缩到 0.5**，Valence=-0.4 反而最高 0.65-0.8。反例：Valence=-0.99,Arousal=0.9→Conf=0.5045；Valence=-0.41→Conf=0.765 | 数学正确性 | 重写为正相关于 `(-Valence)+Arousal`，如 `clip(0.5+(-v.Valence)*0.3+(v.Arousal-0.5)*0.4, 0.5, 0.95)` |
| **F2** | **H** | Lens-5 死开关 | `engine.go:13` `_ = config // reserved for future`；`types.go:73,82` SkipDistortionDetection 字段；`domains/psychological_config.go` 给 4 域设 true/false 但**100% 无效**（CBT 未实现） | AGENTS.md"完成度约束——不允许带着已知错误/80% 代码结束"；"变更即记录" | 业务方确认 VAD-only 路线后删除字段 + domains 差异化配置；或真正实现 CBT |
| **F3** | **H** | Lens-4 测试空洞 | `psychological_test.go` 仅 3 函数，无 TestBeforeAgentRun/TestTransformContext/TestTools；无 VAD 边界/单调性/归一化测试 | GO-DEVELOPMENT-STANDARDS §7.2 表格驱动 | 补装配层测试 + 数学专项（权重和=1、calming 单调性反例、策略互斥） |
| **F4** | **H** | Lens-2 死代码 | `hook.go` 全文 40 行；`NewLifecycleHook`（hook.go:18）全仓 grep 0 调用；唯一装配的是 `NewExtension`（domains/unified.go:99-100） | "去繁就简" | 删除 hook.go；或在 Extension 上补 LifecycleHook() 方法二选一 |
| **F5** | **M** | Lens-3 词表重复 | `engine.go:91` 与 `engine.go:93` 的 positive 词表均含"满意"；countMatches（engine.go:160）每词独立计数→"满意"单次匹配 pos+=2 | 数据质量 | 去重词表；加"词表无重复"测试断言 |
| **F6** | **M** | Lens-5 文档-代码背离 | design.md 规划 occ.go/ema.go/sdt.go/distortion.go 等 9 文件 + 5 模型，实际仅 4 文件 + VAD；状态仍"待审核" | AGENTS.md"变更即记录" | design.md 状态改"已简化实现/VAD-only"；补 ADR 记录"5 模型→1 模型"决策 |
| **F7** | **M** | Lens-2 双重注入风险 | hook.go:33 在 messages 头部插入；extension.go:74 在最后一条 user 消息前插入。当前仅 Extension 启用未触发，但代码保留同时启用可能 | 契约清晰 | 明确单一注入路径（推荐 TransformContext，按轮次精准） |
| F8 | L | Lens-1 去重误判 | extension.go:58 当 lastUserMsg==lastInput 直接返回不注入；合法重复输入会丢失心理上下文 | — | 注释说明意图或改 (sessionID,msgHash) 键 |
| F9 | L | Lens-3 取值范围 | Valence 范围非对称 [-0.8,1.0]（PerceivedControl∈[0,1]）；Arousal 永不低于 0.2（常数基线） | — | 注释标注设计取舍 |
| F10 | L | Lens-3 信号压缩 | countMatches 每词最多 +1 不计频次，长文本失真 | — | 观察项，可选改 strings.Count |

## 4. 已验证合规项

- ✅ **VAD 三维权重均归一化**（和=1.0）：Valence 0.7+0.2+0.1=1.0；Arousal 0.3+0.3+0.2+0.2=1.0；Dominance 0.6+0.3+0.1=1.0。**不存在历史 SDTWeights 那种"未归一化"问题**（因 SDT 根本未实现）
- ✅ LifecycleHook/Extension 接口契约与 agentcore 一致（psychologicalHook 嵌入 BaseLifecycleHook）
- ✅ 依赖方向单向（psychological → agentcore），无反向导入
- ✅ 无 panic / 无硬编码密钥 / 无 `./` 相对路径
- ✅ 单文件行数克制（≤290）
- ✅ Extension 用 sync.Mutex 保护 lastInput，锁内无 IO
- ✅ 中文注释 + 英文标识符

## 5. 数学正确性"待裁决项"（任务要求 vs 代码现实）

| 任务要求核验的模型 | 代码现状 | 结论 |
|--------------------|----------|------|
| **VAD** | ✅ 已实现（engine.go:176-188） | 权重归一化正确；calming 置信度公式有 H 级 bug（F1） |
| **OCC**（22 情绪） | ❌ **未实现** | EmoRelief=EmoJoy 历史问题**仅存在于设计稿 design.md:285-293**，代码无 OCC，**无从验证** |
| **EMA**（衰减 α·e^(-βt)） | ❌ **未实现** | grep `decay\|exp\(` 本模块 0 命中 |
| **SDT**（d'/c/β） | ❌ **未实现** | SDTWeights 归一化问题**不存在于代码**（无该符号） |
| **CBT**（13 扭曲） | ❌ **未实现** | 仅剩 SkipDistortionDetection 死开关（F2） |

**核心结论**：phase5 历史发现的"OCC EmoRelief=EmoJoy""SDTWeights 未归一化"在当前代码中**都不存在——不是因为修了，而是因为对应模型从未被写出来**。应标记为"设计稿层面问题，实现层 N/A"。

## 6. 架构合理性独立意见（[NEEDS CLARIFICATION: 业务方确认]）

**问题**：专利/法律领域 Agent 是否真的需要全部 5 个心理模型？

**我的判断：不需要，当前的 VAD-only 精简版反而是正确的方向。**

- **论据 1（产品定位差异）**：design.md 第 18-22 行明确动机是"沉浸式用户体验""读懂 ta""长期关系建立"——这是**消费级陪伴型 Agent**（nuochat）的目标。Mady 是**面向专利代理人/律师的 B2B 专业工具**，用户预期是"精准、克制、可信赖"，不是"情感共鸣"。把陪伴 Agent 的 5 模型照搬进专利工具，恰恰**违反"去繁就简"**。
- **论据 2（成本/收益）**：OCC 14 情绪、EMA 应对模式、SDT 跨轮追踪对"权利要求撰写/侵权比对/无效宣告"等专业任务**零业务增益**，却带来维护成本、上下文注入噪声、可解释性下降。
- **论据 3（现状矛盾）**：实际代码已做出"砍到 VAD-only"的正确决定，但文档和死开关还停在"5 模型"旧叙事——这种"做对了但没说"的半成品状态比"全做"或"全砍"都糟。

**建议**：业务方（@xujian）确认后，二选一：
- **(A) 正式确认 VAD-only 路线（推荐）**：删 design.md 的 5 模型规划、删 SkipDistortionDetection、把 NuoChatResult 改名为 Result（去供应商痕迹），补 ADR
- **(B) 若确需 CBT**（如法律咨询识别当事人"灾难化"思维），则单独评审 CBT 子模块必要性

## 7. 建议下一步

### 立即（P1）
1. **修 F1**：重写 engine.go:233 calming 置信度公式 + 加反例单测（Valence=-0.99 时 Confidence 应高于 Valence=-0.41）
2. **决断 F2/F6**：业务方确认 VAD-only 路线后删除 SkipDistortionDetection + domains 差异化配置；更新 design.md 状态
3. **补 F3**：hook.go/extension.go 测试 + VAD 边界/单调性/策略互斥测试
4. **清理 F4**：删除 hook.go 或使其成为 Extension 的 LifecycleHook() 实现

### 短期（P2）
5. **修 F5**：去重情感词表 + "词表无重复"测试断言
6. **补 ADR**（F6）：`docs/decisions/` 记录"5 模型→VAD-only"决策 + AI_CHANGELOG

### 复核命令
```bash
go test -race -cover ./psychological/
grep -rn "满意" psychological/engine.go      # 核实词表重复
grep -rn "NewLifecycleHook" .                # 核实死代码（应 0 命中）
```

> 审阅员备注：本模块最大风险不是数学 bug（F1 虽实质但易修），而是**"文档-代码"认知鸿沟**。建议优先解决 F2/F6 让三方（任务描述/设计稿/代码）对齐，否则后续审阅都会重复"找不到 OCC/SDT 代码"的无效劳动。
