# Context Builder

统一上下文组装器，将 System Prompt、Knowledge、Memory、History 等多层上下文组装为最终的 LLM 请求。

借鉴 **Claude Code** 的 Static/Dynamic 边界分离、**LangChain RunnableSequence** 的管线组合、
**LlamaIndex** 的 Token 预算分配、**CrewAI** 的可配置注入策略。

## 架构

```
BuildInput → DefaultContextBuilder.Build() → BuildOutput
                 │
    ┌────────────┼────────────┬──────────────┐
    ▼            ▼            ▼              ▼
 LayerSystem  LayerKnowledge  LayerMemory  LayerHistory
 (Provider)   (Provider)      (Provider)   (pass-through)
```

## 分层设计

| 层 | Provider | 注入策略 | Token 配额 | 优先级 |
|----|----------|---------|-----------|-------|
| LayerSystem | 内建 | always | 无限制 | 1（最高） |
| LayerTools | 内建 | per_turn | 无限制 | 2 |
| LayerKnowledge | KnowledgeExtension | smart | 2000 | 4 |
| LayerMemory | MemoryExtension | always | 1000 | 5 |
| LayerHistory | 内建 | always | 剩余 | 6（最低） |

## LayerProvider 接口

各模块通过 `LayerProvider` 接口参与上下文组装：

```go
type LayerProvider interface {
    Layer() ContextLayer
    Provide(ctx context.Context, input BuildInput, layerCfg LayerConfig) ([]Message, error)
}
```

Memory 和 Knowledge 扩展都实现了此接口。

## SystemPrompt 分段设计

借鉴 Claude Code 的 prefix caching 模式，System Prompt 分离为三段：

```go
cfg := SystemPromptConfig{
    StaticPrefix: "角色定义、安全规则（可缓存）",
    ToolIndex:    "可用工具列表",
    DynamicSuffix: "当前日期、环境变量",
}
```

## 配置

```go
// Agent 配置
cfg := agentcore.NewConfig(
    agentcore.WithContextBuilder(builder),
    agentcore.WithLayerConfig(agentcore.LayerMemory, agentcore.LayerConfig{
        Enabled:    true,
        MaxTokens:  2000,
        InjectMode: agentcore.InjectAlways,
    }),
)
```

## 向后兼容

`ContextBuilder.Enabled = false`（默认）时，Agent 回退到原有的 `TransformContext → ConvertToLLM` 路径。
启用 `true` 后，Build() 替换 TransformContext 作为主要上下文组装点。
