package tools

import (
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestEgoLiteExtensionDisabled(t *testing.T) {
	ext, err := NewEgoLiteExtension(EgoLiteConfig{Enabled: false})
	if err == nil {
		t.Error("expected error for disabled extension")
	}
	if ext != nil {
		t.Error("expected nil extension for disabled config")
	}
}

func TestEgoLiteHandoffToolSchema(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteHandoffTool(mgr)

	if tool.Name != EgoLiteHandoffToolName {
		t.Errorf("tool name = %q, want %q", tool.Name, EgoLiteHandoffToolName)
	}
	def := tool.Definition()
	if def.Name != EgoLiteHandoffToolName {
		t.Errorf("definition name = %q", def.Name)
	}
}

func TestEgoLiteTaskSpacesToolSchema(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteTaskSpacesTool(mgr)

	if tool.Name != EgoLiteTaskSpacesToolName {
		t.Errorf("tool name = %q, want %q", tool.Name, EgoLiteTaskSpacesToolName)
	}
	def := tool.Definition()
	if def.Name != EgoLiteTaskSpacesToolName {
		t.Errorf("definition name = %q", def.Name)
	}
}

func TestEgoLiteHandoffUnknownAction(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteHandoffTool(mgr)

	args, _ := json.Marshal(map[string]any{"action": "unknown_action"})
	_, err := tool.Func(nil, args)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestEgoLiteTaskSpacesMissingName(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteTaskSpacesTool(mgr)

	args, _ := json.Marshal(map[string]any{"action": "create"})
	_, err := tool.Func(nil, args)
	if err == nil {
		t.Error("expected error for create without name")
	}
}

func TestEgoLiteExtensionImplementsInterface(t *testing.T) {
	var _ agentcore.Extension = (*EgoLiteExtension)(nil)
}
