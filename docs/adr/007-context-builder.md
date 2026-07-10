# ADR-0007: ContextBuilder — 统一上下文组装

- **状态**：Accepted
- **日期**：2026-07-11
- **决策者**：Mady Authors
- **影响范围**：agentcore/ 包、所有 LayerProvider 实现

## 上下文

现有 Agent 的上下文组装分散在多个点：
1. `SystemPrompt` 在 Agent 初始化时注入（一次性）
2. `TransformContext` 在每次 LLM 调用前执行（可组合但功能弱）
3. `LifecycleHook.BeforeModelCall` 被用于注入检索结果（扩展使用）
4. `ConvertToLLM` 转换消息格式（无上下文添加能力）

这种分散导致没有统一的 Token 预算管理、
各上下文来源的优先级无法协调、无法实现智能的 Static/Dynamic 缓存分离。

## 决策

### 1. ContextBuilder 接口（分层组装）

`Build(BuildInput) BuildOutput` 作为单一入口，替代 `TransformContext`。

```go
type BuildInput struct {
    Messages, ToolDefs, SystemPrompt string
    ContextWindow, ReserveTokens     int64
    LayerConfigs                     map[ContextLayer]LayerConfig
}

type BuildOutput struct {
    Messages []Message
    ToolDefs []ToolDefinition
    Usage    BuildUsage
}
```

### 2. 分层设计

5 个标准层，按 `Priority` 排序（数字小优先保留）：

| 层 | 内容 | Provider | 默认 Token |
|----|------|----------|-----------|
| LayerSystem | 角色定义、安全规则 | 内建 | 无限制 |
| LayerTools | 工具描述 | 内建 | 无限制 |
| LayerKnowledge | 检索上下文 | KnowledgeExtension | 2000 |
| LayerMemory | 长期记忆 | MemoryExtension | 1000 |
| LayerHistory | 对话历史 | 内建 | 剩余 |

### 3. Static/Dynamic 边界分离（Claude Code 模式）

`SystemPromptConfig` 将系统提示分为三段：
- `StaticPrefix`: 角色、规则、安全（可标记 CacheControl）
- `ToolIndex`: 工具描述（变化时重算）
- `DynamicSuffix`: 环境、日期（每会话变化）

### 4. 向后兼容

`ContextBuilder.Enabled = false`（默认）时，Agent 回退到 `TransformContext → ConvertToLLM` 路径。
启用后逐步迁移。

### 5. 注入策略

| 策略 | 说明 |
|------|------|
| always | 每轮注入（Memory、History） |
| per_turn | 每轮重新生成（Tools） |
| on_demand | 仅工具触发 |
| by_trigger | 复杂度门控（Knowledge 的 SmartTrigger） |

## 排除方案

- **不**创建独立的 `ContextBuilderConfig` 子配置（避免碎片化）
- **不**支持 `LayerProvider` 的链式组合（只有简单的列表）
- **不**在 Phase 2 实现 CacheControl 的实际缓存机制（仅标记，由上游 Provider 处理）

## 影响

- 新增 5 个 agentcore 文件
- `Config` 新增 `ContextBuilder` 和 `LayerConfigs` 字段
- `LayerProvider` 接口让 Memory、Knowledge 可参与上下文组装
- 无运行时行为改变（默认关闭）
