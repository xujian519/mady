package piagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sky-valley/pi/ai"

	"github.com/xujian519/mady/agentcore"
)

func echoTool(name, out string) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        name,
		Description: "echo " + name,
		Parameters:  map[string]any{"type": "object"},
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return out, nil
		},
	}
}

func TestToAgentTool_Basic(t *testing.T) {
	at, err := ToAgentTool(echoTool("read", "hello"), BridgeConfig{})
	if err != nil {
		t.Fatalf("ToAgentTool: %v", err)
	}
	if at.Name != "read" || at.Description != "echo read" {
		t.Errorf("name/desc mismatch: %+v", at)
	}
	res, err := at.Execute(context.Background(), "call-1", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(res.Content))
	}
	text, ok := res.Content[0].(ai.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T", res.Content[0])
	}
	if text.Text != "hello" {
		t.Errorf("text = %q, want hello", text.Text)
	}
}

func TestToAgentTool_ReadOnlyRejectsWrite(t *testing.T) {
	at, err := ToAgentTool(echoTool("edit", "boom"), BridgeConfig{ReadOnly: true})
	if err != nil {
		t.Fatalf("ToAgentTool: %v", err)
	}
	_, err = at.Execute(context.Background(), "call-1", map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "只读子会话") {
		t.Fatalf("want read-only rejection, got %v", err)
	}
}

func TestToAgentTool_ReadOnlyAllowsRead(t *testing.T) {
	at, err := ToAgentTool(echoTool("read", "ok"), BridgeConfig{ReadOnly: true})
	if err != nil {
		t.Fatalf("ToAgentTool: %v", err)
	}
	if _, err := at.Execute(context.Background(), "call-1", map[string]any{}, nil); err != nil {
		t.Fatalf("read-only tool should pass: %v", err)
	}
}

func TestToAgentTool_PolicyDenies(t *testing.T) {
	denied := errors.New("权限拒绝: domain=patent")
	policy := func(ctx context.Context, name string, args json.RawMessage) error {
		if name == "read" {
			return denied
		}
		return nil
	}
	at, err := ToAgentTool(echoTool("read", "x"), BridgeConfig{Policy: policy})
	if err != nil {
		t.Fatalf("ToAgentTool: %v", err)
	}
	_, err = at.Execute(context.Background(), "call-1", map[string]any{}, nil)
	if !errors.Is(err, denied) {
		t.Fatalf("want policy denial, got %v", err)
	}
}

func TestToAgentTool_StructResultSerialized(t *testing.T) {
	rich := &agentcore.Tool{
		Name: "meta",
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			return map[string]any{"a": 1, "b": "x"}, nil
		},
	}
	at, err := ToAgentTool(rich, BridgeConfig{})
	if err != nil {
		t.Fatalf("ToAgentTool: %v", err)
	}
	res, err := at.Execute(context.Background(), "call-1", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	text, ok := res.Content[0].(ai.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T", res.Content[0])
	}
	if !strings.Contains(text.Text, `"a": 1`) {
		t.Errorf("serialized result missing fields: %s", text.Text)
	}
}

func TestToAgentTool_SchemaErrorSkips(t *testing.T) {
	bad := &agentcore.Tool{
		Name:       "bad",
		Parameters: map[string]any{"$ref": "#/definitions/X"},
	}
	_, err := ToAgentTool(bad, BridgeConfig{})
	if err == nil {
		t.Fatal("want schema conversion error")
	}
}

func TestToAgentTools_SkipUnsupported(t *testing.T) {
	bad := &agentcore.Tool{Name: "bad", Parameters: map[string]any{"$ref": "#"}}
	good := echoTool("read", "ok")
	got, skipped := ToAgentTools([]*agentcore.Tool{bad, good}, BridgeConfig{})
	if len(got) != 1 || got[0].Name != "read" {
		t.Errorf("tools = %v, want only read", got)
	}
	if len(skipped) != 1 || skipped[0] != "bad" {
		t.Errorf("skipped = %v, want [bad]", skipped)
	}
}
