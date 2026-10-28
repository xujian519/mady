package teams

import (
	"errors"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// MemberStatus 是成员状态；retired 之后不再接收新任务与新邮件。
type MemberStatus string

// 成员状态取值。
const (
	MemberIdle        MemberStatus = "idle"
	MemberRunning     MemberStatus = "running"
	MemberInterrupted MemberStatus = "interrupted"
	MemberRetired     MemberStatus = "retired"
)

// 成员角色。
const (
	RoleLeader = "leader"
	RoleMember = "member"
)

// 团队错误。
var (
	ErrMemberExists   = errors.New("团队成员已存在")
	ErrMemberNotFound = errors.New("团队成员不存在")
	ErrMemberRetired  = errors.New("团队成员已退役")
	ErrLocked         = errors.New("锁已被占用")
)

// Member 是一名团队成员。
type Member struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Role      string       `json:"role"`
	Status    MemberStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Message 是一封团队成员之间的邮件。
type Message struct {
	ID      string    `json:"id"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
	At      time.Time `json:"at"`
}

// TaskCounts 是任务状态计数。
type TaskCounts struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Archived   int `json:"archived"`
}

// View 是团队状态视图：成员、任务计数、各成员未读邮件数与当前锁。
type View struct {
	TeamID          string         `json:"team_id"`
	Dir             string         `json:"dir"`
	Members         []Member       `json:"members"`
	Tasks           TaskCounts     `json:"tasks"`
	PendingMessages map[string]int `json:"pending_messages"`
	Locks           []string       `json:"locks,omitempty"`
	TaskList        []TaskSummary  `json:"task_list,omitempty"`
}

// TaskSummary 是状态视图中的任务概要。
type TaskSummary struct {
	ID      string               `json:"id"`
	Subject string               `json:"subject"`
	Status  agentcore.TaskStatus `json:"status"`
	Owner   string               `json:"owner,omitempty"`
}
