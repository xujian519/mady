package workercontract

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestRoleContract_Validate(t *testing.T) {
	valid := RoleContract{ID: "r1", Title: "评审员", Stance: StanceAttacker}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}

	noID := valid
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Error("missing id must be rejected")
	}

	badStance := valid
	badStance.Stance = "hacker"
	if err := badStance.Validate(); err == nil {
		t.Error("unknown stance must be rejected")
	}
}

func TestRoleContract_JudgeCannotDraftStrategy(t *testing.T) {
	judge := RoleContract{ID: "j1", Title: "裁判", Stance: StanceJudge, StrategyDrafting: true}
	if err := judge.Validate(); err == nil || !strings.Contains(err.Error(), "裁判") {
		t.Errorf("judge with strategy drafting must be rejected, got %v", err)
	}
	neutralJudge := judge
	neutralJudge.StrategyDrafting = false
	if err := neutralJudge.Validate(); err != nil {
		t.Fatalf("judge without strategy drafting should pass: %v", err)
	}
}

func TestRoleContract_PersonaSegment(t *testing.T) {
	judge := RoleContract{
		ID: "j1", Title: "合议组", Stance: StanceJudge, HITL: true,
		Duties:     []string{"独立裁定"},
		Boundaries: []string{"禁止参与策略起草"},
	}
	p := judge.PersonaSegment()
	for _, want := range []string{"中立裁判", "独立裁定", "禁止参与策略起草", "人工确认"} {
		if !strings.Contains(p, want) {
			t.Errorf("judge persona missing %q: %s", want, p)
		}
	}
}

func TestTeamComposition_JudgeRequiresPair(t *testing.T) {
	// 有裁判但只有一个对抗立场 → 配对不完整。
	team := TeamComposition{
		Scenario: "t",
		Roles: []RoleContract{
			{ID: "a", Title: "攻击方", Stance: StanceAttacker},
			{ID: "j", Title: "裁判", Stance: StanceJudge},
		},
	}
	if err := team.Validate(); err == nil || !strings.Contains(err.Error(), "对立双方") {
		t.Errorf("incomplete pairing must be rejected, got %v", err)
	}

	// 补齐第二个对抗立场（权利人）→ 通过。
	team.Roles = append(team.Roles, RoleContract{ID: "d", Title: "防御方", Stance: StanceApplicant})
	if err := team.Validate(); err != nil {
		t.Fatalf("complete pairing rejected: %v", err)
	}

	// 两个裁判 → 拒绝。
	team.Roles = append(team.Roles, RoleContract{ID: "j2", Title: "裁判2", Stance: StanceJudge})
	if err := team.Validate(); err == nil || !strings.Contains(err.Error(), "至多一个") {
		t.Errorf("two judges must be rejected, got %v", err)
	}
}

func TestResolveTeamComposition_BuiltinsValid(t *testing.T) {
	for _, scene := range TeamScenarios() {
		team, err := ResolveTeamComposition(scene)
		if err != nil {
			t.Fatalf("builtin scenario %s failed validation: %v", scene, err)
		}
		if len(team.Roles) == 0 {
			t.Errorf("scenario %s has no roles", scene)
		}
	}
}

func TestResolveTeamComposition_Unknown(t *testing.T) {
	if _, err := ResolveTeamComposition("no-such"); err == nil {
		t.Error("unknown scenario must fail")
	}
}

func TestTeamResolveTool(t *testing.T) {
	tool := NewPatentTeamResolveTool()
	result, err := tool.Func(context.Background(), json.RawMessage(`{"scenario":"invalidation"}`))
	if err != nil {
		t.Fatal(err)
	}
	s, ok := result.(string)
	if !ok || !strings.Contains(s, `"attacker"`) || !strings.Contains(s, `"judge"`) {
		t.Errorf("expected roles with attacker and judge, got %v", result)
	}

	// 未知场景 → HandoffResult 失败。
	bad, err := tool.Func(context.Background(), json.RawMessage(`{"scenario":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	hr, ok := bad.(agentcore.HandoffResult)
	if !ok || hr.Success {
		t.Errorf("unknown scenario must return failure, got %#v", bad)
	}
}
