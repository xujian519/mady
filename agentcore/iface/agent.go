package iface

import "context"

// =============================================================================
// Agent 运行状态与核心接口
// =============================================================================

// AgentStatus represents the runtime status of an agent.
type AgentStatus string

const (
	// StatusIdle indicates the agent is not currently running.
	StatusIdle AgentStatus = "idle"
	// StatusRunning indicates the agent is currently executing.
	StatusRunning AgentStatus = "running"
	// StatusFinished indicates the agent has completed execution successfully.
	StatusFinished AgentStatus = "finished"
	// StatusError indicates the agent encountered an error during execution.
	StatusError AgentStatus = "error"
	// StatusInterrupted indicates the agent was interrupted before completion.
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
