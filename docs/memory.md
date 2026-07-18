# Memory 模块

Mady 的长期记忆系统，参考 **Mem0**（四维隔离 + LLM 提取）、**CrewAI**（复合评分）、**Letta**（Agent 自我编辑记忆）和 **LlamaIndex**（Token 预算分配）等开源设计。

```
memory/
├── types.go      核心类型: MemoryScope, MemoryEntry, ScoredMemory, MemoryStore 接口
├── store.go      InMemoryStore 实现: 线程安全 map + CJK 分词检索 + CrewAI 复合评分
├── manager.go    MemoryManager 协调器: RememberFromTurn, SearchAllLayers
├── extractor.go  LLM 记忆提取器（预留）+ 规则 fallback
├── retriever.go  混合检索引擎: 复合评分 + Token 预算
├── extension.go  MemoryExtension: TransformContextProvider + ToolProvider + LayerProvider
├── tools.go      remember/recall/forget 工具定义（Letta 风格）
```

## 三层记忆模型

| 层级 | 作用域 | 存储 | 注入方式 |
|------|--------|------|---------|
| User | 跨会话用户偏好/背景 | UserID 隔离 | 每次 LLM 调用前自动注入 |
| Session | 当前会话上下文 | SessionID 隔离 | 始终在上下文中 |
| Long-term | 跨会话持久事实 | UserID + AgentID | Agent 通过 tool 主动管理 |

## MemoryScope（四维隔离）

借鉴 Mem0 的 `user_id / agent_id / app_id / run_id` 正交模型:

```go
scope := MemoryScope{
    UserID:    "alice",      // 用户标识
    AgentID:   "patent_bot", // Agent 标识
    SessionID: "sess_123",   // 会话标识
    ProjectID: "patent_app", // 项目标识
}
```

记忆检索时按维度组合过滤：

```go
// 只查 alice 在会话 sess_123 中的记忆
filter := MemoryFilter{
    UserID:    "alice",
    SessionID: "sess_123",
    TopK:      10,
}
```

## 复合评分（CrewAI 公式）

```
score = 0.5 × semantic_similarity + 0.3 × recency_decay + 0.2 × importance
```

- **语义相似度**: CJK 分词 + 关键词匹配（Phase 1）；向量余弦相似度（Phase 2）
- **新鲜度**: `0.5^(age_in_days / 30)` 指数衰减
- **重要性**: LLM 提取时标注（0~1），无 LLM 时基于关键词启发

## 记忆提取策略

### 隐式提取（Auto-extract）

每次 `AfterModelCall` 后由 `MemoryLifecycleHook` 异步提取：

```go
cfg := DefaultExtensionConfig()
cfg.AutoExtract = true // 启用（默认关闭）
```

### 显式管理（Tool-based, Letta 风格）

Agent 通过工具主动管理记忆：

```go
ToolCall("remember", {content: "用户偏好中文回答", importance: 0.9})
ToolCall("recall", {query: "编程偏好", limit: 5})
ToolCall("forget", {memory_id: "mem_123"})
```

### 预加热 (Preheat)

`Preheater` 在 Agent 启动时从 SQLite 或 JSONL 预加载用户/项目级记忆，
减少首次调用时的提取延迟：

```go
preheater := NewPreheater(store, preheatCfg)
preheater.Preheat(ctx, scope, target)
```

## 记忆编译器 (Compiler)

`memory/compiler/` 实现策略学习型记忆编译器：
- 从多轮对话中提取可复用的策略模式
- 按置信度分级（observed → suggested → established）
- 输出为结构化 `StrategyEntry` 供 Agent 跨会话复用

## 使用示例

```go
// 1. 创建存储引擎
store := memory.NewInMemoryStore()

// 2. 创建管理器
mgr := memory.NewManager(store, nil, nil, memory.DefaultManagerConfig())

// 3. 创建 Extension
scope := memory.MemoryScope{UserID: "alice"}
ext := memory.NewExtension(mgr, scope, memory.DefaultExtensionConfig())

// 4. 注册到 Agent
agentCfg := agentcore.NewConfig(
    agentcore.WithLifecycle(ext.LifecycleHook()),
    agentcore.WithExtensions(ext),
)
```

## 配置选项

| 参数 | 默认值 | 说明 |
|------|--------|------|
| Enabled | true | 是否启用记忆注入 |
| AutoExtract | false | 自动从对话提取记忆 |
| MaxMemoryTokens | 2000 | 注入的最大 token 数 |
| TopK | 5 | 每轮检索的最大记忆条数 |
| ExposeTools | true | 暴露 remember/recall/forget 工具 |
