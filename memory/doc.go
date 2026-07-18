// Package memory 实现三层长期记忆系统（User / Session / Long-term），
// 为 Agent 提供跨会话的知识保持和偏好学习能力。
//
// 三层存储模型：
//   - User 层：跨会话用户偏好/背景（UserID 隔离，持久化）
//   - Session 层：当前会话的关键上下文（SessionID 隔离，会话级）
//   - Long-term 层：跨会话持久事实/知识（UserID + AgentID 隔离，持久化）
//
// 主要功能：
//   - 记忆存储与检索（Store/Retriever）
//   - 记忆提取与结构化（Extractor）
//   - 偏好学习（Preference）
//   - 预热加载（Preheat）
//   - 策略学习型编译（compiler 子包）
//
// 主要类型：
//   - Manager: 三层记忆的统一管理者
//   - Store: 记忆持久化存储（InMemory / SQLite）
//   - Retriever: 相关性检索（复合评分：语义 + 新鲜度 + 重要性）
//   - Extractor: 从对话中提取结构化记忆项
//   - Tool: 供 Agent 调用的记忆操作工具（remember/recall/forget）
//
// 使用示例：
//
//	mgr := memory.NewManager(store)
//	items, _ := mgr.Retrieve(ctx, "专利审查意见答复策略")
package memory
