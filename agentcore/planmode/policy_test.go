package planmode

import (
	"encoding/json"
	"testing"
)

func TestPolicy_BlockedTools(t *testing.T) {
	p := Policy{}
	for _, tool := range []string{"edit", "write_file", "delete", "move", "patch"} {
		d := p.Decide(tool, false, nil)
		if !d.Blocked {
			t.Errorf("%s should be blocked in plan mode", tool)
		}
	}
}

func TestPolicy_AlwaysAllowed(t *testing.T) {
	p := Policy{}
	for _, tool := range []string{"ask", "todo"} {
		d := p.Decide(tool, false, nil)
		if d.Blocked {
			t.Errorf("%s should always be allowed", tool)
		}
	}
}

func TestPolicy_ReadOnlyAllowed(t *testing.T) {
	p := Policy{}
	d := p.Decide("read", true, nil)
	if d.Blocked {
		t.Error("read-only tools should be allowed")
	}
}

func TestPolicy_UnknownWriterBlocked(t *testing.T) {
	p := Policy{}
	d := p.Decide("custom_tool", false, nil)
	if !d.Blocked {
		t.Error("unknown non-readonly tools should be blocked (fail-closed)")
	}
}

func TestPolicy_WhitelistedTool(t *testing.T) {
	p := Policy{AllowedTools: []string{"custom_reader"}}
	d := p.Decide("custom_reader", false, nil)
	if d.Blocked {
		t.Error("whitelisted tools should be allowed")
	}
}

func TestPolicy_BashReadOnly(t *testing.T) {
	p := Policy{}
	tests := []struct {
		cmd  string
		want bool // true = allowed (not blocked)
	}{
		{`{"command":"ls -la"}`, true},
		{`{"command":"cat README.md"}`, true},
		{`{"command":"grep foo *.go"}`, true},
		{`{"command":"git status"}`, true},
		{`{"command":"git diff"}`, true},
		{`{"command":"git log --oneline"}`, true},
		{`{"command":"go test ./..."}`, true},
		{`{"command":"echo hello"}`, true},
		{`{"command":"rm -rf /tmp/test"}`, false},
		{`{"command":"mkdir newdir"}`, false},
		{`{"command":"npm install"}`, false},
		{`{"command":"git push origin main"}`, false},
		{`{"command":"git commit -m test"}`, false},
		{`{"command":"echo hello > file.txt"}`, false},
		{`{"command":"curl http://example.com"}`, false},
	}

	for _, tt := range tests {
		d := p.Decide("bash", false, json.RawMessage(tt.cmd))
		blocked := d.Blocked
		if blocked == tt.want {
			t.Errorf("bash %s: blocked=%v want blocked=%v", tt.cmd, blocked, !tt.want)
		}
	}
}

func TestPolicy_BashChainedCommands(t *testing.T) {
	p := Policy{}
	// All read-only chained commands should be allowed
	d := p.Decide("bash", false, json.RawMessage(`{"command":"ls && cat README.md"}`))
	if d.Blocked {
		t.Error("chained read-only commands should be allowed")
	}

	// Mixed chain with a write command should be blocked
	d = p.Decide("bash", false, json.RawMessage(`{"command":"ls && rm test.go"}`))
	if !d.Blocked {
		t.Error("chained command with write should be blocked")
	}
}

func TestExtension_ActivateDeactivate(t *testing.T) {
	ext := NewExtension(Policy{})
	if ext.IsActive() {
		t.Error("should start inactive")
	}

	ext.Activate()
	if !ext.IsActive() {
		t.Error("should be active after Activate()")
	}

	ext.Deactivate()
	if ext.IsActive() {
		t.Error("should be inactive after Deactivate()")
	}
}

func TestExtension_Name(t *testing.T) {
	ext := NewExtension(Policy{})
	if ext.Name() != ExtensionName {
		t.Errorf("Name()=%q want %q", ext.Name(), ExtensionName)
	}
}

func TestReadOnlyBash_NoCommand(t *testing.T) {
	if isReadOnlyBashCommand(nil) {
		t.Error("nil args should not be read-only")
	}
	if isReadOnlyBashCommand(json.RawMessage(`{}`)) {
		t.Error("empty command should not be read-only")
	}
}
