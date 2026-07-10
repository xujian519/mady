# Knowledge 模块

知识库检索 + 图谱增强 + Agent 工具集成。

## 架构

```
KnowledgeExtension
  ├── RetrievalHook (LifecycleHook)  →  自动注入检索结果
  ├── GraphEnhancer    (可选)         →  图谱增强（相似案例 + 引用链）
  ├── search_knowledge (Tool)         →  按需检索
  └── LayerProvider   (ContextBuilder)→  分层上下文组装
```

## 触发策略 (TriggerPolicy)

| 策略 | 说明 | 适用场景 |
|------|------|---------|
| always（默认） | 每轮都检索 | 始终需要专业知识 |
| smart | 按复杂度分类，仅 Medium/High 时检索 | 减少简单对话开销 |
| first_n | 仅前 N 轮检索 | 初始上下文后无需知识 |
| on_demand | 仅通过 search_knowledge 工具触发 | Agent 自主决策 |

Smart Trigger 复用 `agentcore.ReasoningRouter` 的复杂度分类器：

- ComplexityLow（问候/闲聊）→ 跳过检索
- ComplexityMedium+（专业问题）→ 触发检索

## 三层上下文格式

当 `RetrievalHook` + `GraphEnhancer` 同时启用时，上下文格式如下：

```
### 相关文档片段 (Level 1)
--- 片段 1 (相关度: 0.92) ---
[法条内容]

### 知识图谱扩展：法条引用链 (Level 2)
1. 专利法第22条 [权威度: 1.0]

### 知识图谱扩展：相似案例 (Level 3)
1. Case-2023-001 [权威度: 0.75]
```

## 使用示例

```go
// 1. 创建知识库存储
store := knowledge.NewStore()
store.LoadPatentClaims("patent/claims.md")

// 2. 可选：创建图谱增强器
g := graph.NewGraphEnhancer(graphStore, graph.DefaultEnhanceConfig())

// 3. 创建 KnowledgeExtension
ext := knowledge.NewExtension(store, g, "patent", knowledge.DefaultKnowledgeExtConfig())

// 4. 注册到 Agent
agentCfg := agentcore.NewConfig(
    agentcore.WithLifecycle(ext.LifecycleHook()),
    agentcore.WithExtensions(ext),
)
```

## 评估钩子 (EvalHook)

RAGAS 风格的轻量评估（Phase 3 启发式，Phase 4 接入 LLM 评分）：

- **Faithfulness**: 答案是否忠实于检索上下文
- **AnswerRelevancy**: 答案是否针对问题
- **ContextPrecision**: 检索结果中是否有噪声
