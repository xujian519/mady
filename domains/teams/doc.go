// Package teams 提供会话级多智能体协作层（ADR 0008）。
//
// 团队由队长与成员组成，成员是可跨轮唤醒的 Agent 实例。本包复用
// agentcore/tasklist 的任务存储与依赖语义，新增邮箱、事件驱动调度器、
// 团队锁、成员生命周期与状态视图；团队状态目录位于
// <工作区>/.teams/<teamId>/，事件写入会话日志，不新增 host 写 RPC。
package teams
