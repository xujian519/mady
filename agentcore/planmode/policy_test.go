package planmode

import (
	"encoding/json"
	"strings"
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

// TestReadOnlyBash_InterpreterBypass covers C1/H8: general-purpose
// interpreters and awk must be fail-closed because they can execute
// arbitrary code (file deletion, network calls) without any shell
// redirection operator.
func TestReadOnlyBash_InterpreterBypass(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"python -c", `{"command":"python -c \"import os; os.remove('x')\"}"}`},
		{"python3 -c", `{"command":"python3 -c \"print(1)\"}"}`},
		{"node -e", `{"command":"node -e \"require('fs').unlinkSync('x')\"}"}`},
		{"ruby -e", `{"command":"ruby -e \"File.delete('x')\"}"}`},
		{"cargo run", `{"command":"cargo run"}`},
		{"cargo build", `{"command":"cargo build"}`},
		{"awk system", `{"command":"awk 'BEGIN{system(\"rm -rf /tmp/x\")}'"}`},
		{"awk redirect", `{"command":"awk 'BEGIN{print > \"/tmp/x\"}'"}`},
		{"sed -i", `{"command":"sed -i 's/a/b/' file.txt"}`},
	}
	for _, c := range cases {
		if isReadOnlyBashCommand(json.RawMessage(c.cmd)) {
			t.Errorf("%s: must be blocked (interpreter/awk can execute arbitrary code)", c.name)
		}
	}
}

// TestReadOnlyBash_GoSubcommands covers the go subcommand whitelist:
// read-only subcommands (test/vet/list) pass, code-executing or
// file-writing subcommands (run/build/install) are blocked.
func TestReadOnlyBash_GoSubcommands(t *testing.T) {
	allowed := []string{
		`{"command":"go test ./..."}`,
		`{"command":"go vet ./..."}`,
		`{"command":"go list -m all"}`,
		`{"command":"go doc strings"}`,
		`{"command":"go version"}`,
	}
	for _, c := range allowed {
		if !isReadOnlyBashCommand(json.RawMessage(c)) {
			t.Errorf("should be allowed: %s", c)
		}
	}
	blocked := []string{
		`{"command":"go run main.go"}`,
		`{"command":"go build ./..."}`,
		`{"command":"go install"}`,
		`{"command":"go get example.com/pkg"}`,
		`{"command":"go generate"}`,
	}
	for _, c := range blocked {
		if isReadOnlyBashCommand(json.RawMessage(c)) {
			t.Errorf("should be blocked: %s", c)
		}
	}
}

// TestReadOnlyBash_QuotedOperators covers M12: shell operators that
// appear inside single/double quotes must NOT be mistaken for chaining
// or redirection.
func TestReadOnlyBash_QuotedOperators(t *testing.T) {
	// Pipe inside quotes (regex alternation) must be allowed.
	cases := []struct {
		name string
		cmd  string
		want bool
	}{
		{"grep regex pipe", `{"command":"grep \"a|b\" file.txt"}`, true},
		{"grep single-quote pipe", `{"command":"grep 'a|b' file.txt"}`, true},
		{"echo gt in quotes", `{"command":"echo \"a>b\" file.txt"}`, true},
		{"echo with single-quote gt", `{"command":"echo 'x>y'"}`, true},
		// real pipe still splits, but both halves are read-only → allowed
		{"real pipe both readonly", `{"command":"ls | grep foo"}`, true},
		// real pipe with write second half → blocked
		{"real pipe with write", `{"command":"ls | rm -f x"}`, false},
		// real redirect outside quotes → blocked
		{"real redirect", `{"command":"echo hello > out.txt"}`, false},
	}
	for _, c := range cases {
		got := isReadOnlyBashCommand(json.RawMessage(c.cmd))
		if got != c.want {
			t.Errorf("%s: got allowed=%v want %v (cmd=%s)", c.name, got, c.want, c.cmd)
		}
	}
}

// TestDecide_CaseInsensitive covers L16: tool name matching must be
// case-insensitive so "Edit"/"BASH"/"DELETE" behave like their lowercase
// forms.
func TestDecide_CaseInsensitive(t *testing.T) {
	p := Policy{}
	for _, name := range []string{"Edit", "EDIT", "edit", "Write_File", "WRITE_FILE"} {
		if d := p.Decide(name, false, nil); !d.Blocked {
			t.Errorf("%s must be blocked (case-insensitive)", name)
		}
	}
	// "BASH" should reach the bash read-only path just like "bash"
	d := p.Decide("BASH", false, json.RawMessage(`{"command":"ls"}`))
	if d.Blocked {
		t.Error("BASH (uppercase) with read-only command should be allowed")
	}
}

func TestSplitCommandChain(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"ls -la", []string{"ls -la"}},
		{"ls && cat f", []string{"ls", "cat f"}},
		{"a; b ; c", []string{"a", "b", "c"}},
		{`grep "a|b" f`, []string{`grep "a|b" f`}},
		{`echo "x;y"`, []string{`echo "x;y"`}},
		{"a | b | c", []string{"a", "b", "c"}},
		{`echo 'a && b'`, []string{`echo 'a && b'`}},
	}
	for _, c := range cases {
		got := splitCommandChain(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCommandChain(%q) = %v (len %d), want %v (len %d)", c.in, got, len(got), c.want, len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCommandChain(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestStripQuoted(t *testing.T) {
	// stripQuoted preserves length and blanks out quoted regions so that
	// operators inside quotes disappear while unquoted text is kept.
	cases := []string{
		`grep "a|b" f`,
		`echo 'x>y'`,
		`echo "a\"b"`,
		`no quotes at all`,
		`find . -name "*.go" | xargs grep foo`,
	}
	for _, in := range cases {
		got := stripQuoted(in)
		if len(got) != len(in) {
			t.Errorf("stripQuoted(%q) changed length: %d → %d", in, len(in), len(got))
		}
		// No quote characters may survive.
		if strings.ContainsAny(got, `"'`) {
			t.Errorf("stripQuoted(%q) still contains a quote: %q", in, got)
		}
	}
	// Operators inside quotes must be gone; operators outside must remain.
	if out := stripQuoted(`grep "a|b" f`); strings.Contains(out, "|") {
		t.Errorf("quoted pipe should be removed, got %q", out)
	}
	if out := stripQuoted(`a | b`); !strings.Contains(out, "|") {
		t.Errorf("unquoted pipe should remain, got %q", out)
	}
}
