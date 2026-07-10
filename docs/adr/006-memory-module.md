# ADR-0006: Memory 模块设计

- **状态**：Accepted
- **日期**：2026-07-11
- **决策者**：Mady Authors
- **影响范围**：memory/ 包、agentcore.LayerProvider

## 上下文

Mady 缺少长期记忆系统。现有的 `AgentState.messages` 仅提供会话级短期记忆，
无法跨会话存储和检索用户偏好、关键事实。开源记忆系统（Mem0、Letta、CrewAI、LangChain、LlamaIndex）
提供了成熟的模式可以借鉴。

## 决策

### 1. 三层记忆模型（CrewAI + Mem0）

| 层 | 作用域 | 说明 |
|----|--------|------|
| User | user_id | 跨会话用户偏好/背景 |
| Session | user_id + session_id | 当前会话上下文 |
| Long-term | user_id + agent_id | 持久事实，由 Agent 主动管理 |

### 2. 四维隔离作用域（Mem0）

```go
type MemoryScope struct {
    UserID, AgentID, SessionID, ProjectID string
}
```

四维正交，任意组合检索。这是最小完备维度集。

### 3. 复合评分（CrewAI）

默认权重：语义 0.5、新鲜度 0.3、重要性 0.2。
避免纯向量搜索在数据增长后退化。

### 4. Token 预算感知（LlamaIndex）

`RecallWithBudget()` 在最大 token 预算内返回记忆。
默认：上下文窗口的 15%。防止记忆侵占对话空间。

### 5. 提取策略

- **隐式提取**: AfterModelCall 时 LLM 提取原子事实（Mem0 风格），通过 LifecycleHook 异步执行
- **显式管理**: Agent 通过 Tool Calling 管理记忆（Letta 风格）

Phase 1 实现隐式关闭（由 Manager.RememberFromTurn 托管），
Phase 2 启用 LLM 自动提取。

### 6. 集成方式

作为 `agentcore.Extension` 注册，通过 `TransformContextProvider` 注入记忆，
通过 `LayerProvider` 参与 ContextBuilder 组装。

### 排除方案

- **不**采用独立的向量数据库作为存储后端（Phase 1 保持零外部依赖）
- **不**采用 Graph DB 作为记忆后端（知识图谱已在 knowledge/graph 中处理）
- **不**做记忆合并/矛盾解决（Phase 1 简单的 latest-wins 冲突策略）

## 影响

- 新增 `memory/` 包，8 个文件，约 1500 行
- `agentcore.LayerProvider` 接口新增 `Layer()` 方法
- 新增 `ConfigOption`: `WithContextBuilder`, `WithLayerConfig`
