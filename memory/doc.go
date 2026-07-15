// Package memory 实现三层长期记忆系统（工作记忆/ episodic / 语义记忆），
// 为 Agent 提供跨会话的知识保持和偏好学习能力。
//
// 三层存储模型：
//   - 工作记忆：当前会话的短期上下文
//   - Episodic 记忆：具体交互事件的记录（JSONL 持久化）
//   - 语义记忆：经过抽象和泛化的知识（SQLite + LLM 摘要）
//
// 主要功能：
//   - 记忆存储与检索（Store/Retriever）
//   - 记忆提取与结构化（Extractor）
//   - 偏好学习（PreferenceLearner）
//   - 预热加载（Preheat）
//   - 策略学习型编译（compiler 子包）
//
// 主要类型：
//   - Manager: 三层记忆的统一管理者
//   - Store: 记忆持久化存储
//   - Retriever: 相关性检索（基于 Recency/Relevance/Importance）
//   - Extractor: 从对话中提取结构化记忆项
//   - Tool: 供 Agent 调用的记忆操作工具
//
// 使用示例：
//
//	mgr := memory.NewManager(store)
//	items, _ := mgr.Retrieve(ctx, "专利审查意见答复策略")
package memory
