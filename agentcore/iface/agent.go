package iface

import "context"

// =============================================================================
// Agent 运行状态与核心接口
// =============================================================================

// AgentStatus 表示 Agent 的运行状态。
type AgentStatus string

const (
	StatusIdle        AgentStatus = "idle"
	StatusRunning     AgentStatus = "running"
	StatusFinished    AgentStatus = "finished"
	StatusError       AgentStatus = "error"
	StatusInterrupted AgentStatus = "interrupted"
)

// AgentState 是 Agent 状态的轻量级快照。
type AgentState struct {
	Status    AgentStatus
	TurnCount int64
	LastError string
}

// AgentRunner 是 agent 运行时的核心接口。
// 由 agentcore.Agent 实现。
type AgentRunner interface {
	Run(ctx context.Context, input string) (string, error)
	Continue(ctx context.Context) (string, error)
	Resume(ctx context.Context, interruptData map[string]any) (string, error)
	Close()
	State() AgentState
}
