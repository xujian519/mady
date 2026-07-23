package evidence

import (
	"encoding/json"
	"testing"
)

func TestLedger_RecordAndLen(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "read", Success: true})
	l.Record(Receipt{ToolName: "edit", Success: true})
	if l.Len() != 2 {
		t.Fatalf("Len()=%d want 2", l.Len())
	}
}

func TestLedger_Reset(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "read", Success: true})
	l.Reset()
	if l.Len() != 0 {
		t.Fatalf("Len() after reset=%d want 0", l.Len())
	}
}

func TestLedger_HasSuccessfulWrite(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{
		ToolName: "edit",
		Success:  true,
		Write:    true,
		Paths:    []string{"/a/b.go", "/c/d.go"},
	})
	if !l.HasSuccessfulWrite([]string{"/a/b.go"}) {
		t.Error("expected HasSuccessfulWrite for /a/b.go")
	}
	if l.HasSuccessfulWrite([]string{"/x/y.go"}) {
		t.Error("expected no write for /x/y.go")
	}
	if !l.HasSuccessfulWrite([]string{"/a/b.go", "/c/d.go"}) {
		t.Error("expected all paths covered")
	}
}

func TestLedger_HasSuccessfulCommand(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{
		ToolName: "bash",
		Success:  true,
		Command:  "go test ./...",
	})
	if !l.HasSuccessfulCommand("go test") {
		t.Error("expected HasSuccessfulCommand for 'go test'")
	}
	if l.HasSuccessfulCommand("npm test") {
		t.Error("expected no match for 'npm test'")
	}
}

func TestLedger_HasFailedCommand(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{
		ToolName: "bash",
		Success:  false,
		Command:  "go build ./...",
	})
	if !l.HasFailedCommand("go build") {
		t.Error("expected HasFailedCommand for 'go build'")
	}
	if l.HasSuccessfulCommand("go build") {
		t.Error("expected HasSuccessfulCommand=false for failed cmd")
	}
}

func TestLedger_TouchedPaths(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "read", Success: true, Read: true, Paths: []string{"/a"}})
	l.Record(Receipt{ToolName: "edit", Success: true, Write: true, Paths: []string{"/b"}})
	l.Record(Receipt{ToolName: "edit", Success: false, Write: true, Paths: []string{"/c"}})

	all := l.TouchedPaths(10, false)
	if len(all) != 2 {
		t.Fatalf("TouchedPaths(false)=%v want 2 items", all)
	}

	writes := l.TouchedPaths(10, true)
	if len(writes) != 1 || writes[0] != "/b" {
		t.Fatalf("TouchedPaths(true)=%v want [/b]", writes)
	}
}

func TestLedger_HasAnySuccessfulReceipt(t *testing.T) {
	l := NewLedger()
	if l.HasAnySuccessfulReceipt() {
		t.Error("expected false for empty ledger")
	}
	l.Record(Receipt{ToolName: "read", Success: false})
	if l.HasAnySuccessfulReceipt() {
		t.Error("expected false when only failed receipts")
	}
	l.Record(Receipt{ToolName: "read", Success: true})
	if !l.HasAnySuccessfulReceipt() {
		t.Error("expected true with at least one success")
	}
}

func TestLedger_HasWriteOrCommandSince(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "read", Success: true, Read: true})
	l.Record(Receipt{ToolName: "ask", Success: true})
	l.Record(Receipt{ToolName: "edit", Success: true, Write: true})
	if !l.HasWriteOrCommandSince(0) {
		t.Error("expected write at index 2")
	}
	if l.HasWriteOrCommandSince(3) {
		t.Error("expected false since index 3 (past end)")
	}
}

func TestLedger_SuccessfulCommands(t *testing.T) {
	l := NewLedger()
	l.Record(Receipt{ToolName: "bash", Success: true, Command: "go test"})
	l.Record(Receipt{ToolName: "bash", Success: false, Command: "go vet"})
	l.Record(Receipt{ToolName: "bash", Success: true, Command: "go build"})
	cmds := l.SuccessfulCommands(5)
	if len(cmds) != 2 {
		t.Fatalf("SuccessfulCommands=%v want 2 items", cmds)
	}
	if cmds[0] != "go build" {
		t.Errorf("most recent first: got %q want %q", cmds[0], "go build")
	}
}

func TestReceiptFromToolCall(t *testing.T) {
	args := json.RawMessage(`{"path":"/test.go","command":"ls"}`)
	r := ReceiptFromToolCall("edit", args, true, 42)
	if !r.Write {
		t.Error("expected Write=true for edit")
	}
	if r.Read {
		t.Error("expected Read=false for edit")
	}
	if r.DurationMs != 42 {
		t.Errorf("DurationMs=%d want 42", r.DurationMs)
	}
	if len(r.Paths) != 1 || r.Paths[0] != "/test.go" {
		t.Errorf("Paths=%v want [/test.go]", r.Paths)
	}
}

func TestReceiptFromToolCall_Bash(t *testing.T) {
	args := json.RawMessage(`{"command":"go test ./..."}`)
	r := ReceiptFromToolCall("bash", args, true, 100)
	if r.Command != "go test ./..." {
		t.Errorf("Command=%q want %q", r.Command, "go test ./...")
	}
}

func TestLedger_NilSafe(t *testing.T) {
	var l *Ledger
	l.Reset()
	l.Record(Receipt{})
	_ = l.Len()
	_ = l.HasSuccessfulWrite([]string{"/a"})
	_ = l.HasAnySuccessfulReceipt()
	_ = l.Snapshot()
}
