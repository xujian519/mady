package permission

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

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
		{"glob path match", MustParseRule("Edit(docs/**)"), "Edit", editArgs, true},
		{"glob path no match", MustParseRule("Edit(src/**)"), "Edit", editArgs, false},
		{"bash command prefix", MustParseRule("Bash(go test:*)"), "Bash", bashArgs, true},
		{"bash wrong command", MustParseRule("Bash(rm:*)"), "Bash", bashArgs, false},
		{"nil args with specifier", MustParseRule("Edit(docs/**)"), "Edit", nil, false},
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
	denyRule := MustParseRule("Delete")
	allowReadRule := MustParseRule("Read")
	askBashRule := MustParseRule("Bash")
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
			DefaultPolicy(),
			"Read", true, readArgs, DecisionAllow,
		},
		{
			"writer fallback to ask (default mode)",
			DefaultPolicy(),
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
		// Allow: read-only tools
		{"read tool", "read", nil, true, DecisionAllow},
		{"ls tool", "ls", nil, true, DecisionAllow},
		{"grep tool", "grep", nil, true, DecisionAllow},
		{"find tool", "find", nil, true, DecisionAllow},
		{"glob tool", "glob", nil, true, DecisionAllow},
		{"view tool", "view", nil, true, DecisionAllow},

		// Ask: write tools
		{"edit tool", "edit", nil, false, DecisionAsk},
		{"write_file tool", "write_file", nil, false, DecisionAsk},

		// Deny: destructive tools
		{"bash tool", "bash", nil, false, DecisionDeny},
		{"delete tool", "delete", nil, false, DecisionDeny},
		{"move tool", "move", nil, false, DecisionDeny},
		{"process tool", "process", nil, false, DecisionDeny},
		{"execute_code tool", "execute_code", nil, false, DecisionDeny},

		// Fallback: unlisted read-only tool → Allow
		{"unlisted read-only", "web_search", nil, true, DecisionAllow},

		// Fallback: unlisted writer → Ask (Mode=DecisionAsk)
		{"unlisted writer", "browser", nil, false, DecisionAsk},

		// Deny is not sensitive to readOnly flag (deny overrides everything)
		{"deny even if readOnly", "bash", nil, true, DecisionDeny},
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
	var req *ApprovalRequest
	for i := 0; i < 200; i++ {
		req = a.PollPending()
		if req != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
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
	for i := 0; i < 200; i++ {
		req2 = a.PollPending()
		if req2 != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
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
	req2 = nil
	for i := 0; i < 200; i++ {
		req2 = a.PollPending()
		if req2 != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if req2 == nil {
		t.Fatal("expected pending request (cancel test), got nil")
	}
	cancel() // cancel the context
	d = <-done
	if d != DecisionDeny {
		t.Errorf("Approve() after cancel returned %v; want Deny", d)
	}
}
