// Package plantask provides Sati-aligned patent plan task tools.
//
// Mady already has a full PlanTask HCL extension in agentcore/plantask for
// production HITL workflows. The tools here are lightweight, Sati-schema
// compatible wrappers that give patent agents a unified plan-state interface
// without requiring deep integration with the runtime extension.
package plantask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/provenance"
)

// provenanceLog 是包级溯源日志器；nil 时静默（Log 自身 nil-safe）。
var provenanceLog *provenance.ProvenanceLogger

// SetProvenance 注入溯源日志器（bootstrap 装配）；传 nil 时溯源静默关闭。
func SetProvenance(l *provenance.ProvenanceLogger) { provenanceLog = l }

// State is the whitelist-checked state for patent_plan_task.
type State string

// State values for the plan state machine.
const (
	StatePlanning         State = "planning"
	StateAwaitingApproval State = "awaiting_approval"
	StateExecuting        State = "executing"
	StateAwaitingFeedback State = "awaiting_feedback"
	StateReplanning       State = "replanning"
	StateFinished         State = "finished"
)

// transitions defines the legal state machine edges.
var transitions = map[State][]State{
	StatePlanning:         {StateAwaitingApproval},
	StateAwaitingApproval: {StateExecuting, StateReplanning},
	StateExecuting:        {StateAwaitingFeedback, StateFinished},
	StateAwaitingFeedback: {StateReplanning, StateFinished},
	StateReplanning:       {StateAwaitingApproval, StateExecuting},
}

// isKnownState reports whether s appears anywhere in the state machine — as a
// source (map key) OR a transition target (edge value). StateFinished is a
// terminal state that only appears as a target, so it must be recognized here
// or to="finished" would be rejected as 非法状态.
func isKnownState(s State) bool {
	if _, ok := transitions[s]; ok {
		return true
	}
	for _, targets := range transitions {
		for _, t := range targets {
			if t == s {
				return true
			}
		}
	}
	return false
}

// isSourceState reports whether s has outgoing transitions (is a map key).
// Terminal states like StateFinished have no outgoing edges and cannot be a
// transition source.
func isSourceState(s State) bool {
	_, ok := transitions[s]
	return ok
}

// allStates returns every state in declaration order regardless of whether it
// appears as a key or only as a transition target. Used for the error message's
// 合法状态 list.
func allStates() []State {
	return []State{
		StatePlanning, StateAwaitingApproval, StateExecuting,
		StateAwaitingFeedback, StateReplanning, StateFinished,
	}
}

// PlanTask is a single task derived from a plan step.
type PlanTask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	BlockedBy   []string `json:"blockedBy,omitempty"`
	Hash        string   `json:"hash"`
}

// Input is the tool input shape.
type Input struct {
	Action        string     `json:"action"`
	CurrentState  string     `json:"current_state,omitempty"`
	To            string     `json:"to,omitempty"`
	PlanSteps     []string   `json:"plan_steps,omitempty"`
	PreviousTasks []PlanTask `json:"previous_tasks,omitempty"`
	Tasks         []PlanTask `json:"tasks,omitempty"`
	Feedback      string     `json:"feedback,omitempty"`
}

// NewPatentPlanTaskTool creates the patent_plan_task tool.
func NewPatentPlanTaskTool() *agentcore.Tool {
	return &agentcore.Tool{
		Name:        "patent_plan_task",
		Description: "人机协作计划状态机工具（HITL 闭环）：transition 白名单状态迁移、sync 计划步骤 → 任务、replan 哈希比对已完成步骤增量续跑。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":         map[string]any{"type": "string", "enum": []string{"transition", "sync", "replan"}, "description": "操作类型"},
				"current_state":  map[string]any{"type": "string", "description": "当前状态（transition 必需）"},
				"to":             map[string]any{"type": "string", "description": "目标状态（transition 必需）"},
				"plan_steps":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "计划步骤（sync/replan 必需）"},
				"previous_tasks": map[string]any{"type": "array", "description": "之前已同步的任务（replan 可选）"},
				"tasks":          map[string]any{"type": "array", "description": "当前任务列表（transition 到 executing 必需）"},
				"feedback":       map[string]any{"type": "string", "description": "重规划反馈（transition 到 replanning 必需）"},
			},
			"required": []string{"action"},
		},
		ReadOnly: true,
		Func:     handlePlanTask,
	}
}

func handlePlanTask(_ context.Context, args json.RawMessage) (any, error) {
	var p Input
	if err := json.Unmarshal(args, &p); err != nil {
		return agentcore.NewFailureResult("参数解析失败", "计划任务参数格式错误"), nil
	}

	switch p.Action {
	case "transition":
		return handleTransition(p)
	case "sync":
		return handleSync(p)
	case "replan":
		return handleReplan(p)
	default:
		return fmt.Sprintf("patent_plan_task: 未知操作 %q（可选: transition / sync / replan）", p.Action), nil
	}
}

func handleTransition(p Input) (any, error) {
	from := State(p.CurrentState)
	to := State(p.To)
	if from == "" {
		from = StatePlanning
	}

	validFrom := isSourceState(from)
	validTo := isKnownState(to)
	if !validFrom || !validTo {
		states := []string{}
		for _, s := range allStates() {
			states = append(states, string(s))
		}
		return agentcore.NewFailureResult("非法状态", fmt.Sprintf("状态 %q 或 %q 不合法（合法: %s）", from, to, strings.Join(states, ", "))), nil
	}

	allowed := transitions[from]
	if !slices.Contains(allowed, to) {
		return agentcore.NewFailureResult("非法迁移", fmt.Sprintf("不允许 %q → %q", from, to)), nil
	}

	// Semantic guards.
	if to == StateExecuting && len(p.Tasks) == 0 {
		return agentcore.NewFailureResult("语义错误", "进入 executing 必须先 sync 出非空任务列表"), nil
	}
	if to == StateReplanning && p.Feedback == "" {
		return agentcore.NewFailureResult("语义错误", "进入 replanning 必须提供 feedback"), nil
	}

	_ = provenanceLog.Log(provenance.ProvenanceEvent{
		Kind:    provenance.KindPlanLifecycle,
		Tool:    "patent_plan_task",
		Details: fmt.Sprintf("状态迁移 %s → %s", from, to),
	})

	return fmt.Sprintf("patent_plan_task: %s → %s ✅", from, to), nil
}

func handleSync(p Input) (any, error) {
	if len(p.PlanSteps) == 0 {
		return agentcore.NewFailureResult("空计划", "sync 需要 plan_steps 非空"), nil
	}
	tasks := make([]PlanTask, len(p.PlanSteps))
	toRun := []string{}
	for i, step := range p.PlanSteps {
		id := fmt.Sprintf("t%d", i+1)
		tasks[i] = PlanTask{
			ID:          id,
			Description: step,
			Status:      "pending",
			Hash:        stepHash(step),
		}
		if i == 0 {
			toRun = append(toRun, id)
		} else {
			tasks[i].BlockedBy = []string{fmt.Sprintf("t%d", i)}
		}
	}

	data, err := json.Marshal(map[string]any{
		"tasks":      tasks,
		"to_run":     toRun,
		"task_count": len(tasks),
	})
	if err != nil {
		return agentcore.NewFailureResult("序列化失败", err.Error()), nil
	}
	return string(data), nil
}

func handleReplan(p Input) (any, error) {
	if len(p.PlanSteps) == 0 {
		return agentcore.NewFailureResult("空计划", "replan 需要 plan_steps 非空"), nil
	}
	preserved := []string{}
	previousHashes := make(map[string]bool)
	for _, t := range p.PreviousTasks {
		if t.Status == "completed" {
			previousHashes[t.Hash] = true
			preserved = append(preserved, t.ID)
		}
	}

	tasks := make([]PlanTask, len(p.PlanSteps))
	toRun := []string{}
	for i, step := range p.PlanSteps {
		h := stepHash(step)
		id := fmt.Sprintf("t%d", i+1)
		status := "pending"
		// Preserve completed steps that still appear in the new plan.
		if previousHashes[h] {
			status = "completed"
		} else {
			toRun = append(toRun, id)
		}
		tasks[i] = PlanTask{
			ID:          id,
			Description: step,
			Status:      status,
			Hash:        h,
		}
		if i > 0 {
			tasks[i].BlockedBy = []string{fmt.Sprintf("t%d", i)}
		}
	}

	data, err := json.Marshal(map[string]any{
		"tasks":      tasks,
		"preserved":  preserved,
		"to_run":     toRun,
		"task_count": len(tasks),
	})
	if err != nil {
		return agentcore.NewFailureResult("序列化失败", err.Error()), nil
	}

	_ = provenanceLog.Log(provenance.ProvenanceEvent{
		Kind:    provenance.KindPlanLifecycle,
		Tool:    "patent_plan_task",
		Details: fmt.Sprintf("replan 续跑：%d 任务 / 保留 %d 已完成", len(tasks), len(preserved)),
	})

	return string(data), nil
}

func stepHash(step string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(step)))
	return hex.EncodeToString(sum[:])[:16]
}
