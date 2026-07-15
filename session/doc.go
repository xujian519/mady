// Package session 提供会话管理功能，以 JSONL 树形结构存储 Agent 对话历史，
// 支持多分支会话、快照恢复和上下文继承。
//
// 核心概念：
//   - 树形结构：每个会话节点代表一次消息交换，子节点形成分支
//   - JSONL 持久化：逐行追加写入，支持增量存储
//   - 快照：会话可在任意节点保存和恢复
//
// 主要类型：
//   - Tree: 会话树结构，管理节点的增删改查和分支漫游
//   - Session: 单次会话，包含消息列表和元数据
//   - FileStore: 基于 JSONL 文件的会话存储
//   - AgentStore: 与 agentcore 集成的 Agent 会话适配
//
// 使用示例：
//
//	tree := session.NewTree()
//	node := tree.AddMessage("user", "分析这件专利")
//	child := tree.Branch(node.ID, "assistant", "好的，我来分析")
package session
