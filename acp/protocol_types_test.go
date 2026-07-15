package acp_test

import (
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/acp"
)

func TestJSONRPCRequestMarshal(t *testing.T) {
	req := acp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "agent/initialize",
		Params:  json.RawMessage(`{}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded acp.JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Method != "agent/initialize" {
		t.Fatalf("Method = %q, want %q", decoded.Method, "agent/initialize")
	}
}

func TestJSONRPCError(t *testing.T) {
	e := &acp.JSONRPCError{
		Code:    -32601,
		Message: "Method not found",
	}
	want := "acp error -32601: Method not found"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestJSONRPCError_Nil(t *testing.T) {
	var e *acp.JSONRPCError
	if got := e.Error(); got != "" {
		t.Errorf("nil error should return empty, got %q", got)
	}
}

func TestInitializeParamsMarshal(t *testing.T) {
	p := acp.InitializeParams{
		ProtocolVersion: 1,
		ClientInfo: &acp.Implementation{
			Name:    "test-client",
			Version: "1.0",
		},
		ClientCapabilities: &acp.ClientCapabilities{
			FS: &acp.FileSystemCapability{
				ReadTextFile: true,
			},
		},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded acp.InitializeParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ProtocolVersion != 1 {
		t.Fatalf("ProtocolVersion = %d, want 1", decoded.ProtocolVersion)
	}
	if decoded.ClientInfo == nil {
		t.Fatal("ClientInfo is nil")
	}
}

func TestNewSessionParams(t *testing.T) {
	p := acp.NewSessionParams{
		CWD: "/home/user/project",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded acp.NewSessionParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.CWD != "/home/user/project" {
		t.Fatalf("CWD = %q, want %q", decoded.CWD, "/home/user/project")
	}
}

func TestSessionUpdateVariants(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"user message chunk", `{"sessionUpdate":"user_message_chunk","content":"hello"}`, "user_message_chunk"},
		{"agent message chunk", `{"sessionUpdate":"agent_message_chunk","content":"hi"}`, "agent_message_chunk"},
		{"tool call", `{"sessionUpdate":"tool_call","toolCallId":"tc1","title":"read","kind":"fs"}`, "tool_call"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var update acp.SessionUpdate
			if err := json.Unmarshal([]byte(tt.data), &update); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if update.SessionUpdate != tt.want {
				t.Errorf("SessionUpdate = %q, want %q", update.SessionUpdate, tt.want)
			}
		})
	}
}

func TestPermissionParamsRoundTrip(t *testing.T) {
	params := acp.RequestPermissionParams{
		SessionID: "session-1",
		ToolCall: acp.PermissionToolCall{
			ToolCallID: "tc-1",
			Title:      "read /etc/passwd",
			Kind:       "fs",
			Status:     "pending",
		},
		Options: []acp.PermissionOption{
			{OptionID: "allow_once", Name: "Allow once", Kind: "allow_once"},
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded acp.RequestPermissionParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.SessionID != "session-1" {
		t.Fatalf("SessionID = %q", decoded.SessionID)
	}
	if len(decoded.Options) != 1 {
		t.Fatalf("len(Options) = %d, want 1", len(decoded.Options))
	}
}
