// Package memory 实现 Mady Agent 的三层长期记忆系统（User / Session / Long-term），
// 为 Agent 提供跨会话的知识保持和偏好学习能力。
//
// 架构借鉴 Mem0、CrewAI、Letta、LangChain、LlamaIndex 等开源记忆系统：
//   - 四维隔离作用域（Mem0 风格）：user / agent / session / project
//   - 三层记忆模型（CrewAI + Mem0 风格）：User / Session / Long-term
//   - 复合评分检索（CrewAI 风格）：语义 + 新鲜度 + 重要性
//   - Token 预算感知（LlamaIndex 风格）：chat_history_token_ratio
//   - Agent 自我编辑记忆（Letta 风格）：通过 Tool Calling 管理
//
// 主要功能：
//   - 记忆存储与检索（Store / Retriever）
//   - 记忆提取与结构化（Extractor）
//   - 会话结束汇总（SessionSummarizer）
//   - 去重与冲突处理（Deduplicator / DedupDecider）
//   - 策略学习型编译（compiler 子包）
//
// 主要类型：
//   - Manager: 三层记忆的统一管理者
//   - Store: 记忆持久化存储（InMemoryStore / SQLiteMemoryStore）
//   - Retriever: 相关性检索（复合评分：语义 + 新鲜度 + 重要性）
//   - Extractor: 从对话中提取结构化记忆项
//   - MemoryExtension: 供 Agent 调用的记忆扩展（注入 / 工具 / 生命周期钩子）
//
// 集成方式：MemoryExtension 实现 agentcore.Extension 的 TransformContextProvider
// （注入记忆）+ ToolProvider（工具）+ LifecycleProvider（生命周期钩子），
// 复用 Mady 现有的 Extension 注册机制。
//
// 使用示例：
//
//	mgr := memory.NewManager(store, nil, nil, memory.DefaultManagerConfig())
//	items, err := mgr.Search(ctx, "专利审查意见答复策略", memory.MemoryFilter{TopK: 5})
package memory
