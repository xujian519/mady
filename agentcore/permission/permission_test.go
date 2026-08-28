package permission

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

// waitPending blocks until a TUIChannelApprover has a pending request or timeout elapses.
// Uses runtime.Gosched() to yield CPU while waiting — no time.Sleep involved.
func waitPending(a *TUIChannelApprover, timeout time.Duration) *ApprovalRequest {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			return a.PollPending()
		default:
		}
		if req := a.PollPending(); req != nil {
			return req
		}
		runtime.Gosched()
	}
}

func TestDecision_String(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{DecisionAllow, "allow"},
		{DecisionAsk, "ask"},
		{DecisionDeny, "deny"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Decision(%d).String()=%q want %q", tt.d, got, tt.want)
		}
	}
}

// mustParseRule is a test helper that panics on error, so test call sites
// remain concise.
func mustParseRule(t testing.TB, s string) Rule {
	t.Helper()
	r, err := MustParseRule(s)
	if err != nil {
		t.Fatalf("MustParseRule(%q) unexpected error: %v", s, err)
	}
	return r
}

func TestParseRule(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		tool    string
		spec    string
	}{
		{"Bash", false, "Bash", ""},
		{"Bash(go test:*)", false, "Bash", "go test:*"},
		{"Edit(docs/**)", false, "Edit", "docs/**"},
		{"Delete", false, "Delete", ""},
		{"", true, "", ""},
		{"Bash(go test", true, "", ""},
		{"(spec)", true, "", ""},
	}
	for _, tt := range tests {
		r, err := ParseRule(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseRule(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRule(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if r.Tool != tt.tool || r.Specifier != tt.spec {
			t.Errorf("ParseRule(%q)={Tool:%q,Spec:%q} want {Tool:%q,Spec:%q}",
				tt.input, r.Tool, r.Specifier, tt.tool, tt.spec)
		}
	}
}

func TestRule_Matches(t *testing.T) {
	editArgs, _ := json.Marshal(map[string]any{"path": "docs/readme.md"})
	bashArgs, _ := json.Marshal(map[string]any{"command": "go test ./..."})

	tests := []struct {
		name     string
		rule     Rule
		toolName string
		args     json.RawMessage
		want     bool
	}{
		{"no specifier matches all", Rule{Tool: "Edit"}, "Edit", editArgs, true},
		{"wrong tool", Rule{Tool: "Edit"}, "Delete", editArgs, false},
		{"case insensitive tool", Rule{Tool: "edit"}, "Edit", editArgs, true},
		{"glob path match", mustParseRule(t, "Edit(docs/**)"), "Edit", editArgs, true},
		{"glob path no match", mustParseRule(t, "Edit(src/**)"), "Edit", editArgs, false},
		{"bash command prefix", mustParseRule(t, "Bash(go test:*)"), "Bash", bashArgs, true},
		{"bash wrong command", mustParseRule(t, "Bash(rm:*)"), "Bash", bashArgs, false},
		{"nil args with specifier", mustParseRule(t, "Edit(docs/**)"), "Edit", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rule.Matches(tt.toolName, tt.args)
			if got != tt.want {
				t.Errorf("Matches()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestPolicy_Decide(t *testing.T) {
	denyRule := mustParseRule(t, "Delete")
	allowReadRule := mustParseRule(t, "Read")
	askBashRule := mustParseRule(t, "Bash")
	editArgs, _ := json.Marshal(map[string]any{"path": "/tmp/test.go"})
	readArgs, _ := json.Marshal(map[string]any{"path": "/tmp/test.go"})

	tests := []struct {
		name     string
		policy   Policy
		tool     string
		readOnly bool
		args     json.RawMessage
		want     Decision
	}{
		{
			"deny overrides everything",
			Policy{Deny: []Rule{denyRule}, Allow: []Rule{{Tool: "Delete"}}},
			"Delete", false, editArgs, DecisionDeny,
		},
		{
			"explicit allow for writer",
			Policy{Allow: []Rule{allowReadRule}},
			"Read", true, readArgs, DecisionAllow,
		},
		{
			"readOnly fallback to allow",
			Policy{Mode: DecisionAsk},
			"Read", true, readArgs, DecisionAllow,
		},
		{
			"writer fallback to ask (default mode)",
			Policy{Mode: DecisionAsk},
			"Edit", false, editArgs, DecisionAsk,
		},
		{
			"writer with Mode=Allow",
			Policy{Mode: DecisionAllow},
			"Edit", false, editArgs, DecisionAllow,
		},
		{
			"ask rule for bash",
			Policy{Ask: []Rule{askBashRule}},
			"Bash", false, editArgs, DecisionAsk,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.policy.Decide(tt.tool, tt.readOnly, tt.args)
			if got != tt.want {
				t.Errorf("Decide()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestApprovers(t *testing.T) {
	ctx := context.Background()

	if d := (NonInteractiveApprover{}).Approve(ctx, "Edit", nil); d != DecisionAllow {
		t.Errorf("NonInteractiveApprover=%v want Allow", d)
	}
	if d := (AlwaysDenyApprover{}).Approve(ctx, "Edit", nil); d != DecisionDeny {
		t.Errorf("AlwaysDenyApprover=%v want Deny", d)
	}

	called := false
	fn := FuncApprover(func(_ context.Context, _ string, _ json.RawMessage) Decision {
		called = true
		return DecisionAllow
	})
	if d := fn.Approve(ctx, "Edit", nil); d != DecisionAllow || !called {
		t.Errorf("FuncApprover: called=%v decision=%v", called, d)
	}
}

func TestGlobMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		{"docs/**", "docs/readme.md", true},
		{"docs/**", "docs/sub/deep/file.go", true},
		{"docs/**", "src/main.go", false},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "docs/readme.md", false},
		{"*", "anything", true},
		{"*.go", "main.go", true},
		{"*.go", "main.txt", false},
	}
	for _, tt := range tests {
		got := globMatch(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("globMatch(%q,%q)=%v want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestProjectAgentPolicy(t *testing.T) {
	policy := ProjectAgentPolicy()

	tests := []struct {
		name     string
		tool     string
		args     json.RawMessage
		readOnly bool
		want     Decision
	}{
		// Allow: any tool not in the Ask list auto-allows (Mode=Allow)
		{"read tool", "read", nil, true, DecisionAllow},
		{"ls tool", "ls", nil, true, DecisionAllow},
		{"grep tool", "grep", nil, true, DecisionAllow},
		{"find tool", "find", nil, true, DecisionAllow},
		{"glob tool", "glob", nil, true, DecisionAllow},
		{"view tool", "view", nil, true, DecisionAllow},
		{"edit tool", "edit", nil, false, DecisionAllow},
		{"write_file tool", "write_file", nil, false, DecisionAllow},
		{"delete tool", "delete", nil, false, DecisionAllow},
		{"move tool", "move", nil, false, DecisionAllow},
		{"process tool", "process", nil, false, DecisionAllow},
		{"browser tool", "browser", nil, false, DecisionAllow},

		// Ask: only truly dangerous operations
		{"bash tool", "bash", nil, false, DecisionAsk},
		{"execute_code tool", "execute_code", nil, false, DecisionAsk},
		{"computer_use tool", "computer_use", nil, false, DecisionAsk},

		// Allow: git tools and other non-listed tools (Mode=Allow)
		{"git_status tool", "git_status", nil, true, DecisionAllow},
		{"git_diff tool", "git_diff", nil, true, DecisionAllow},
		{"git_log tool", "git_log", nil, true, DecisionAllow},

		// Fallback: unlisted tool → Allow (Mode=DecisionAllow)
		{"unlisted read-only", "web_search", nil, true, DecisionAllow},
		{"unlisted non-readonly", "nonexistent_tool", nil, false, DecisionAllow},

		// Ask rules override readOnly default Allow
		{"bash even if readOnly", "bash", nil, true, DecisionAsk},
		{"computer_use readOnly", "computer_use", nil, true, DecisionAsk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := policy.Decide(tt.tool, tt.readOnly, tt.args)
			if got != tt.want {
				t.Errorf("ProjectAgentPolicy().Decide(%q, readOnly=%v) = %v; want %v",
					tt.tool, tt.readOnly, got, tt.want)
			}
		})
	}
}

func TestTUIChannelApprover(t *testing.T) {
	ctx := context.Background()
	a := NewTUIChannelApprover()

	// Initially no pending request.
	if req := a.PollPending(); req != nil {
		t.Fatalf("expected nil pending, got %+v", req)
	}

	// Launch Approve in a goroutine (it blocks).
	done := make(chan Decision, 1)
	args, _ := json.Marshal(map[string]any{"path": "/tmp/test.go"})
	go func() {
		done <- a.Approve(ctx, "Edit", args)
	}()

	// Wait for the pending request.
	req := waitPending(a, time.Second)
	if req == nil {
		t.Fatal("expected pending request, got nil")
	}
	if req.ToolName != "Edit" {
		t.Errorf("ToolName=%q want %q", req.ToolName, "Edit")
	}

	// Respond Allow and check the result.
	a.Respond(DecisionAllow)
	d := <-done
	if d != DecisionAllow {
		t.Errorf("Approve() returned %v; want Allow", d)
	}

	// After Respond, PollPending should be nil.
	if req := a.PollPending(); req != nil {
		t.Errorf("expected nil after respond, got %+v", req)
	}

	// Test Deny response.
	go func() {
		done <- a.Approve(ctx, "Delete", args)
	}()
	var req2 *ApprovalRequest
	req2 = waitPending(a, time.Second)
	if req2 == nil {
		t.Fatal("expected pending request (deny test), got nil")
	}
	a.Respond(DecisionDeny)
	d = <-done
	if d != DecisionDeny {
		t.Errorf("Approve() returned %v; want Deny", d)
	}

	// Test context cancellation.
	ctx2, cancel := context.WithCancel(context.Background())
	go func() {
		done <- a.Approve(ctx2, "Edit", args)
	}()
	req2 = waitPending(a, time.Second)
	if req2 == nil {
		t.Fatal("expected pending request (cancel test), got nil")
	}
	cancel() // cancel the context
	d = <-done
	if d != DecisionDeny {
		t.Errorf("Approve() after cancel returned %v; want Deny", d)
	}
}

// =============================================================================
// 单调拒绝层（monotonic deny）——不可覆盖性
// =============================================================================

func TestMonotonicDeny_RuleOverridesAllow(t *testing.T) {
	// 即便 Allow 规则完全匹配，单调拒绝仍胜出。
	p := Policy{
		Mode:          DecisionAllow,
		MonotonicDeny: []Rule{{Tool: "judge_type_specific"}},
		Allow:         []Rule{{Tool: "judge_type_specific"}},
	}
	got := p.Decide("judge_type_specific", true, json.RawMessage(`{}`))
	if got != DecisionDeny {
		t.Errorf("monotonic deny must override allow, got %s", got)
	}
	// 其他工具不受影响。
	if got := p.Decide("other_tool", true, json.RawMessage(`{}`)); got != DecisionAllow {
		t.Errorf("non-matching tool should be allowed, got %s", got)
	}
}

func TestMonotonicDeny_FnOverridesAskAndDeny(t *testing.T) {
	called := 0
	fn := func(toolName string, args json.RawMessage) (string, bool) {
		called++
		return "形式要件缺失", toolName == "evidence_tool"
	}
	p := Policy{
		Mode:             DecisionAllow,
		MonotonicDenyFns: []DenyCheck{fn},
		Ask:              []Rule{{Tool: "evidence_tool"}},
	}
	if got := p.Decide("evidence_tool", false, json.RawMessage(`{"a":1}`)); got != DecisionDeny {
		t.Errorf("fn deny must win over ask, got %s", got)
	}
	if called != 1 {
		t.Errorf("deny fn should be called once, got %d", called)
	}
	if got := p.Decide("other_tool", true, json.RawMessage(`{}`)); got != DecisionAllow {
		t.Errorf("non-matching tool should pass, got %s", got)
	}
}

func TestMonotonicDeny_EmptyLayerNoBehaviorChange(t *testing.T) {
	// 单调层为空时，Decide 行为与既有优先级完全一致。
	p := ProjectAgentPolicy()
	if got := p.Decide(ToolBash, false, json.RawMessage(`{"command":"ls"}`)); got != DecisionAsk {
		t.Errorf("bash should still ask, got %s", got)
	}
	if got := p.Decide("read_file", true, json.RawMessage(`{"path":"x"}`)); got != DecisionAllow {
		t.Errorf("read-only should still allow, got %s", got)
	}
}
