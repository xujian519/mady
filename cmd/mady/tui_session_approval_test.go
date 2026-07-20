package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains"
)

type approvalTestProvider struct {
	response *agentcore.ProviderResponse
}

func (p *approvalTestProvider) Complete(context.Context, *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	if p.response == nil {
		return &agentcore.ProviderResponse{Content: "ok"}, nil
	}
	return p.response, nil
}

func (p *approvalTestProvider) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan agentcore.StreamDelta, 2)
	if resp.Content != "" {
		ch <- agentcore.StreamDelta{Content: resp.Content}
	}
	if len(resp.ToolCalls) > 0 {
		deltas := make([]agentcore.ToolCallDelta, 0, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			deltas = append(deltas, agentcore.ToolCallDelta{
				Index:     int64(i),
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
		}
		ch <- agentcore.StreamDelta{ToolCalls: deltas}
	}
	ch <- agentcore.StreamDelta{Done: true}
	close(ch)
	return ch, nil
}

func newApprovalTestSession(store domains.ApprovalStore) *tuiSession {
	s := &tuiSession{
		ctx:             context.Background(),
		currentThreadID: "sess-tui",
		approvalGate: domains.NewApprovalGate(
			domains.DefaultApprovalConfig(),
			domains.WithApprovalStore(store),
		),
	}
	return s
}

func TestTUISessionRecordApprovalDecision_SoftInterruptUsesReviewTrigger(t *testing.T) {
	store := domains.NewMemoryApprovalStore()
	s := newApprovalTestSession(store)
	s.currentProject = &domains.ProjectRecord{ProjectID: "case-soft"}

	agent := agentcore.New(agentcore.StubConfig(&approvalTestProvider{}))
	defer agent.Close()
	s.currentAgent = agent

	arc := &agentcore.AgentRunContext{Agent: agent}
	mcc := &agentcore.ModelCallContext{
		Response: &agentcore.ProviderResponse{
			Content: "最终建议：建议继续撰写申请文件并进入人工复核。",
		},
	}
	s.approvalGate.AfterModelCall(context.Background(), arc, mcc)

	s.recordApprovalDecision(domains.DecisionModified, "最终建议：先补一轮从权。", "补充从权建议")

	records, err := store.List(context.Background(), "sess-tui")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.TriggerKeyword != "review" {
		t.Fatalf("trigger = %q, want review", r.TriggerKeyword)
	}
	if r.CaseID != "case-soft" {
		t.Fatalf("case id = %q, want case-soft", r.CaseID)
	}
	if r.Decision != domains.DecisionModified || r.State != domains.StateModified {
		t.Fatalf("decision/state = (%q, %q), want (modified, modified)", r.Decision, r.State)
	}
	if !strings.Contains(r.OriginalOutput, "最终建议") {
		t.Fatalf("original output should come from ApprovalGate preview, got %q", r.OriginalOutput)
	}
	if r.ModifiedOutput == "" || r.Feedback == "" {
		t.Fatal("modified output and feedback should be persisted")
	}
}

func TestTUISessionRecordApprovalDecision_HardInterruptUsesGateData(t *testing.T) {
	store := domains.NewMemoryApprovalStore()
	s := newApprovalTestSession(store)
	s.currentProject = &domains.ProjectRecord{ProjectID: "case-hard"}

	provider := &approvalTestProvider{
		response: &agentcore.ProviderResponse{
			ToolCalls: []agentcore.ToolCall{
				{ID: "call-1", Name: "review_gate", Arguments: `{}`},
			},
		},
	}
	tool := &agentcore.Tool{
		Name:        "review_gate",
		Description: "interrupt for human review",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Func: func(context.Context, json.RawMessage) (any, error) {
			return "paused", agentcore.NewInterruptErrorWithData("需要人工复核 disclosure 报告", map[string]any{
				"gate":      "disclosure_review",
				"report_id": "report-42",
			})
		},
	}
	agent := agentcore.New(agentcore.StubConfig(provider, agentcore.WithTools(tool)))
	defer agent.Close()
	s.currentAgent = agent

	if _, err := agent.Run(context.Background(), "start review"); err != nil {
		t.Fatalf("expected nil error with interrupted agent state, got %v", err)
	}
	if agent.Interrupted() == nil {
		t.Fatal("expected agent.Interrupted() to be populated")
	}

	s.recordApprovalDecision(domains.DecisionRejected, "", "报告结论需要补证据")

	records, err := store.List(context.Background(), "sess-tui")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.TriggerKeyword != "disclosure_review" {
		t.Fatalf("trigger = %q, want disclosure_review", r.TriggerKeyword)
	}
	if r.CaseID != "case-hard" {
		t.Fatalf("case id = %q, want case-hard", r.CaseID)
	}
	if r.Decision != domains.DecisionRejected || r.State != domains.StateRejected {
		t.Fatalf("decision/state = (%q, %q), want (rejected, rejected)", r.Decision, r.State)
	}
	if !strings.Contains(r.OriginalOutput, "需要人工复核 disclosure 报告") {
		t.Fatalf("original output should include interrupt reason, got %q", r.OriginalOutput)
	}
	if !strings.Contains(r.OriginalOutput, `"gate":"disclosure_review"`) {
		t.Fatalf("original output should include interrupt data json, got %q", r.OriginalOutput)
	}
	if r.Feedback != "报告结论需要补证据" {
		t.Fatalf("feedback = %q, want 报告结论需要补证据", r.Feedback)
	}
}
