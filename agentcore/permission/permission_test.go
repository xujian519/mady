package permission

import (
	"context"
	"encoding/json"
	"testing"
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
