package domains

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/pkg/csync"
)

// ApprovalGate is a LifecycleHook that pauses Agent execution at critical
// decision points, waiting for human confirmation before proceeding.
//
// It implements the "重点节点人机协作" principle from Mady's guiding
// philosophy. When the Agent reaches a high-stakes decision (e.g., final
// patent claim drafting, legal conclusion, risk assessment), this hook
// interrupts execution and waits for a human to review and approve.
//
// Usage:
//
//	gate := domains.NewApprovalGate(domains.ApprovalConfig{
//	    RequireApprovalFor: []string{"专利结论", "法律意见", "风险评估"},
//	    TimeoutMsg:         "等待人工审核中...",
//	})
//	cfg.Lifecycle = agentcore.LifecycleChain{gate}
//
// The human operator reviews the paused output and calls agent.Resume()
// to continue, or provides feedback through agent.FollowUp().
type ApprovalGate struct {
	agentcore.BaseLifecycleHook
	config ApprovalConfig
	store  ApprovalStore // optional; set via WithApprovalStore

	// lastTriggeredOutput holds the most recent Agent output that triggered
	// the approval gate. It is consumed (and cleared) by RecordDecision so
	// that /approve or /reject records exactly the output the human reviewed.
	lastTriggeredOutput string
}

// ApprovalConfig controls when and how the approval gate triggers.
type ApprovalConfig struct {
	// RequireApprovalFor is a list of keywords. If the model's output
	// contains any of these, execution is paused for human approval.
	RequireApprovalFor []string

	// TimeoutMsg is the message shown to the human operator while waiting.
	// Default: "此步骤需要人工审核确认，请检查以下内容后回复'确认'继续，或提供修改意见。"
	TimeoutMsg string

	// SkipIfNoTools indicates whether to skip the gate if the output
	// doesn't involve tool calls (purely informational).
	SkipIfNoTools bool
}

// DefaultApprovalConfig returns a sensible default for patent/legal domains.
func DefaultApprovalConfig() ApprovalConfig {
	return ApprovalConfig{
		RequireApprovalFor: []string{
			"专利结论", "侵权判断", "有效性结论",
			"法律意见", "诉讼策略", "判决预测",
			"风险评估", "最终建议",
		},
		TimeoutMsg:    "此步骤需要人工审核确认。请检查以下内容后回复'确认'继续，或提供修改意见。",
		SkipIfNoTools: false,
	}
}

// NewApprovalGate creates an ApprovalGate with the given configuration and opts.
func NewApprovalGate(config ApprovalConfig, opts ...func(*ApprovalGate)) *ApprovalGate {
	if len(config.RequireApprovalFor) == 0 {
		config = DefaultApprovalConfig()
	}
	if config.TimeoutMsg == "" {
		config.TimeoutMsg = "此步骤需要人工审核确认。请检查以下内容后回复'确认'继续，或提供修改意见。"
	}
	g := &ApprovalGate{config: config}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// AfterModelCall implements LifecycleHook.AfterModelCall.
// It checks if the model's output triggers human approval and, if so,
// interrupts execution to wait for confirmation.
func (g *ApprovalGate) AfterModelCall(_ context.Context, arc *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if mcc == nil || mcc.Response == nil || mcc.Err != nil {
		return
	}

	// Skip if no tool calls and SkipIfNoTools is set.
	if g.config.SkipIfNoTools && len(mcc.Response.ToolCalls) == 0 {
		return
	}

	// Check if output contains any trigger keywords.
	if !g.needsApproval(mcc.Response.Content) {
		return
	}

	// Save the triggered output so RecordDecision can persist it when the
	// human operator issues /approve or /reject.
	g.lastTriggeredOutput = mcc.Response.Content

	// Interrupt and wait for human approval.
	// The interrupted output is preserved, and the human can review it
	// before calling Resume() to continue.
	arc.Agent.Steer(agentcore.Message{
		Role:    agentcore.RoleSystem,
		Content: g.buildApprovalMessage(mcc.Response.Content),
	})
}

// needsApproval checks if the content triggers the approval requirement.
func (g *ApprovalGate) needsApproval(content string) bool {
	for _, keyword := range g.config.RequireApprovalFor {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}

// buildApprovalMessage constructs the human-readable approval prompt.
func (g *ApprovalGate) buildApprovalMessage(content string) string {
	// Truncate content for display.
	preview := content
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}

	return strings.Join([]string{
		"═══════════════════════════════════════",
		"⚠️  人 工 审 核 关 卡",
		"═══════════════════════════════════════",
		"",
		g.config.TimeoutMsg,
		"",
		"--- AI 生成内容预览 ---",
		preview,
		"",
		"操作方式：",
		"  • 回复「确认」→ 继续执行",
		"  • 回复修改意见 → AI 将根据您的意见调整",
		"  • 回复「取消」→ 终止当前任务",
		"═══════════════════════════════════════",
	}, "\n")
}

// RequireApproval is a helper function that domain code can call to
// explicitly mark a tool result as requiring human approval.
// It returns an InterruptError that pauses the Agent loop.
func RequireApproval(reason string, data map[string]any) error {
	return agentcore.NewInterruptErrorWithData(
		fmt.Sprintf("需要人工审核: %s", reason),
		data,
	)
}

// ---------------------------------------------------------------------------
// Structured Approval Records (B1 — ApprovalGate 留痕机制)
// ---------------------------------------------------------------------------

// ApprovalDecision records the human operator's verdict on a gated output.
type ApprovalDecision string

const (
	// DecisionAdopted means the human accepted the AI output verbatim.
	DecisionAdopted ApprovalDecision = "adopted"
	// DecisionModified means the human accepted with edits.
	DecisionModified ApprovalDecision = "modified"
	// DecisionRejected means the human rejected the output entirely.
	DecisionRejected ApprovalDecision = "rejected"
)

// ApprovalRecord captures a single approval-gate interaction: what the AI
// generated, which keyword triggered the gate, and how the human responded.
// These records feed the AdoptionRate metric (A3) and the second-layer
// Golden Benchmark conversion pipeline (C1).
type ApprovalRecord struct {
	ID             string           // unique record ID
	SessionID      string           // agent session that triggered the gate
	CaseID         string           // optional case/project identifier
	Timestamp      time.Time        // when the decision was made
	TriggerKeyword string           // keyword that activated the gate
	OriginalOutput string           // AI-generated content before review
	Decision       ApprovalDecision // adopted / modified / rejected
	ModifiedOutput string           // human-edited content (non-empty only when Decision == modified)
	Feedback       string           // human's modification notes or rejection reason
	State          ApprovalState    // 审批状态机的当前状态（持久化后用于正确重建）
}

// ApprovalStore persists ApprovalRecords. Implementations must be safe for
// concurrent use. The domain layer defines the interface; concrete SQLite
// backends live in a sub-package (e.g. domains/sqlite) to respect the
// dependency-inversion rule.
type ApprovalStore interface {
	Save(ctx context.Context, record ApprovalRecord) error
	List(ctx context.Context, sessionID string) ([]ApprovalRecord, error)
	ListByCase(ctx context.Context, caseID string) ([]ApprovalRecord, error)
}

// MemoryApprovalStore is an in-memory ApprovalStore for testing and
// single-session use. Data is lost on process exit.
type MemoryApprovalStore struct {
	records csync.Slice[ApprovalRecord]
}

// NewMemoryApprovalStore creates an empty MemoryApprovalStore.
func NewMemoryApprovalStore() *MemoryApprovalStore {
	return &MemoryApprovalStore{}
}

// Save appends a record.
func (s *MemoryApprovalStore) Save(_ context.Context, record ApprovalRecord) error {
	s.records.Append(record)
	return nil
}

// List returns all records for the given session, oldest first.
func (s *MemoryApprovalStore) List(_ context.Context, sessionID string) ([]ApprovalRecord, error) {
	var out []ApprovalRecord
	for _, r := range s.records.Copy() {
		if r.SessionID == sessionID {
			out = append(out, r)
		}
	}
	return out, nil
}

// ListByCase returns all records for the given case ID, oldest first.
func (s *MemoryApprovalStore) ListByCase(_ context.Context, caseID string) ([]ApprovalRecord, error) {
	var out []ApprovalRecord
	for _, r := range s.records.Copy() {
		if r.CaseID == caseID {
			out = append(out, r)
		}
	}
	return out, nil
}

// WithApprovalStore attaches an ApprovalStore to the gate so that
// RecordDecision calls are persisted. Without a store, RecordDecision
// is a no-op.
func WithApprovalStore(store ApprovalStore) func(*ApprovalGate) {
	return func(g *ApprovalGate) { g.store = store }
}

// RecordDecision creates and persists an ApprovalRecord. It should be
// called by the TUI /review handler (or any human-in-the-loop entry point)
// after the operator has decided on a gated output. If no store is
// configured the call is silently ignored.
func (g *ApprovalGate) RecordDecision(
	ctx context.Context,
	sessionID, caseID, triggerKeyword, originalOutput string,
	decision ApprovalDecision,
	modifiedOutput, feedback string,
) error {
	if g.store == nil {
		return nil
	}
	// Use the saved triggered output if the caller didn't provide one.
	if originalOutput == "" {
		originalOutput = g.lastTriggeredOutput
	}
	g.lastTriggeredOutput = "" // consume
	record := ApprovalRecord{
		ID:             fmt.Sprintf("appr_%d_%s", time.Now().UnixNano(), sessionID),
		SessionID:      sessionID,
		CaseID:         caseID,
		Timestamp:      time.Now(),
		TriggerKeyword: triggerKeyword,
		OriginalOutput: originalOutput,
		Decision:       decision,
		ModifiedOutput: modifiedOutput,
		Feedback:       feedback,
		State:          decisionToState(decision),
	}
	return g.store.Save(ctx, record)
}

// decisionToState maps a human approval decision to the corresponding state
// machine state so that persisted records can be reconstructed correctly.
func decisionToState(d ApprovalDecision) ApprovalState {
	switch d {
	case DecisionAdopted:
		return StateApproved
	case DecisionModified:
		return StateModified
	case DecisionRejected:
		return StateRejected
	default:
		return StateNone
	}
}
