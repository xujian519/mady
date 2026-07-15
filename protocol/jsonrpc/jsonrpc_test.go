// Package jsonrpc_test 提供 JSON-RPC 2.0 协议类型的编解码测试。
package jsonrpc_test

import (
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/protocol/jsonrpc"
)

func TestRequestMarshalUnmarshal(t *testing.T) {
	req := jsonrpc.Request{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "test",
		Params:  json.RawMessage(`{"key":"value"}`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var decoded jsonrpc.Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Fatalf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
	}
	if decoded.Method != "test" {
		t.Fatalf("Method = %q, want %q", decoded.Method, "test")
	}
}

func TestRequestMarshalUnmarshal_NumericID(t *testing.T) {
	req := jsonrpc.Request{
		JSONRPC: "2.0",
		ID:      float64(42),
		Method:  "subtract",
		Params:  json.RawMessage(`[42,23]`),
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded jsonrpc.Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID.(float64) != float64(42) {
		t.Fatalf("ID = %v, want 42", decoded.ID)
	}
}

func TestRequestMarshalUnmarshal_NilID(t *testing.T) {
	req := jsonrpc.Request{
		JSONRPC: "2.0",
		Method:  "notify",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded jsonrpc.Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.ID != nil {
		t.Fatalf("ID should be nil for notification, got %v", decoded.ID)
	}
}

func TestResponseMarshalUnmarshal(t *testing.T) {
	resp := jsonrpc.Response{
		JSONRPC: "2.0",
		ID:      "1",
		Result:  json.RawMessage(`"ok"`),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var decoded jsonrpc.Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if decoded.ID.(string) != "1" {
		t.Fatalf("ID = %v, want '1'", decoded.ID)
	}
}

func TestErrorMarshalUnmarshal(t *testing.T) {
	resp := jsonrpc.Response{
		JSONRPC: "2.0",
		ID:      "1",
		Result:  nil,
		Error: &jsonrpc.Error{
			Code:    -32601,
			Message: "Method not found",
			Data:    nil,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	var decoded jsonrpc.Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if decoded.Error == nil {
		t.Fatal("Error field is nil after unmarshal")
	}
	if decoded.Error.Code != -32601 {
		t.Fatalf("Error.Code = %d, want -32601", decoded.Error.Code)
	}
	if decoded.Error.Message != "Method not found" {
		t.Fatalf("Error.Message = %q, want 'Method not found'", decoded.Error.Message)
	}
}

func TestNotificationMarshalUnmarshal(t *testing.T) {
	n := jsonrpc.Notification{
		JSONRPC: "2.0",
		Method:  "update",
		Params:  json.RawMessage(`[1,2,3]`),
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}

	var decoded jsonrpc.Notification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if decoded.Method != "update" {
		t.Fatalf("Method = %q, want 'update'", decoded.Method)
	}
}

func TestEmptyRequest(t *testing.T) {
	// Minimal request
	req := jsonrpc.Request{
		JSONRPC: "2.0",
		Method:  "ping",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded jsonrpc.Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Method != "ping" {
		t.Fatalf("Method = %q, want 'ping'", decoded.Method)
	}
}
