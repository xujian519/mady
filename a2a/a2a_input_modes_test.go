package a2a

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestValidateInputModes(t *testing.T) {
	// Test supported modes
	err := ValidateInputModes([]string{"text", "image/png"}, []string{"text", "image/png", "application/json"})
	if err != nil {
		t.Fatalf("expected no error for supported modes, got %v", err)
	}

	// Test unsupported mode
	err = ValidateInputModes([]string{"text", "video/mp4"}, []string{"text", "image/png"})
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}
	if err.Error() != `unsupported input mode: "video/mp4"` {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Test empty requested modes (should pass)
	err = ValidateInputModes([]string{}, []string{"text"})
	if err != nil {
		t.Fatalf("expected no error for empty requested modes, got %v", err)
	}

	// Test empty supported modes (should pass)
	err = ValidateInputModes([]string{"text"}, []string{})
	if err != nil {
		t.Fatalf("expected no error for empty supported modes, got %v", err)
	}
}

func TestValidateOutputModes(t *testing.T) {
	// Test supported modes
	err := ValidateOutputModes([]string{"text", "application/json"}, []string{"text", "image/png", "application/json"})
	if err != nil {
		t.Fatalf("expected no error for supported modes, got %v", err)
	}

	// Test unsupported mode
	err = ValidateOutputModes([]string{"video/mp4"}, []string{"text"})
	if err == nil {
		t.Fatal("expected error for unsupported mode")
	}
}

func TestExtractInputModes(t *testing.T) {
	// Test text mode
	msg := Message{Parts: []Part{NewTextPart("Hello")}}
	modes := ExtractInputModes(msg)
	if len(modes) != 1 || modes[0] != "text" {
		t.Fatalf("expected [text], got %v", modes)
	}

	// Test file mode with MIME type
	msg = Message{Parts: []Part{NewFilePartBytes("test.png", "image/png", "data")}}
	modes = ExtractInputModes(msg)
	if len(modes) != 1 || modes[0] != "image/png" {
		t.Fatalf("expected [image/png], got %v", modes)
	}

	// Test data mode
	msg = Message{Parts: []Part{NewDataPart(map[string]any{"key": "value"})}}
	modes = ExtractInputModes(msg)
	if len(modes) != 1 || modes[0] != "data" {
		t.Fatalf("expected [data], got %v", modes)
	}

	// Test mixed modes
	msg = Message{Parts: []Part{
		NewTextPart("Hello"),
		NewFilePartBytes("test.png", "image/png", "data"),
		NewDataPart(map[string]any{"key": "value"}),
	}}
	modes = ExtractInputModes(msg)
	if len(modes) != 3 {
		t.Fatalf("expected 3 modes, got %d", len(modes))
	}
	modeMap := make(map[string]bool)
	for _, m := range modes {
		modeMap[m] = true
	}
	if !modeMap["text"] || !modeMap["image/png"] || !modeMap["data"] {
		t.Fatalf("expected text, image/png, and data modes, got %v", modes)
	}
}

func TestServer_InputModeValidation(t *testing.T) {
	handler := newMockHandler()
	// Set supported input modes
	handler.card.DefaultInputModes = []string{"text", "image/png"}
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Test supported mode
	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "valid-mode-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("expected no error for supported mode, got %v", resp.Error)
	}

	// Test unsupported mode
	resp, err = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "invalid-mode-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewFilePartBytes("test.mp4", "video/mp4", "data")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unsupported input mode")
	}
	if resp.Error.Code != A2AErrorContentTypeNotSupported {
		t.Fatalf("expected content type not supported error, got %d", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// Input-Required State Tests
// ---------------------------------------------------------------------------

func TestDefaultAgentHandler_InputRequiredState(t *testing.T) {
	handler := &inputRequiredHandler{
		card: AgentCard{
			Name: "test-agent",
			URL:  "http://localhost:8080",
			Capabilities: AgentCapabilities{
				Streaming:              true,
				PushNotifications:      true,
				StateTransitionHistory: true,
			},
		},
		tasks: make(map[string]*Task),
	}

	ctx := context.Background()

	task, err := handler.SendTask(ctx, SendTaskRequest{
		ID:      "input-req-1",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if task.State != TaskStateInputRequired {
		t.Fatalf("expected input-required, got %s", task.State)
	}
	if len(task.Messages) < 1 {
		t.Fatalf("expected at least 1 message, got %d", len(task.Messages))
	}
}

func TestDefaultAgentHandler_InputRequiredThenContinue(t *testing.T) {
	handler := &appendableHandler{
		card: AgentCard{
			Name: "test-agent",
			URL:  "http://localhost:8080",
			Capabilities: AgentCapabilities{
				Streaming:              true,
				PushNotifications:      true,
				StateTransitionHistory: true,
			},
		},
		tasks: make(map[string]*Task),
	}

	ctx := context.Background()

	task1, err := handler.SendTask(ctx, SendTaskRequest{
		ID:      "multi-turn-1",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if task1.State != TaskStateInputRequired {
		t.Fatalf("expected input-required after first send, got %s", task1.State)
	}

	task2, err := handler.SendTask(ctx, SendTaskRequest{
		ID:      "multi-turn-1",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("More info")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if task2.State != TaskStateCompleted {
		t.Fatalf("expected completed after second send, got %s", task2.State)
	}

	if len(task2.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(task2.Messages))
	}
}

func TestDefaultAgentHandler_AppendToTerminalTaskFails(t *testing.T) {
	handler := &terminalHandler{
		card: AgentCard{
			Name: "test-agent",
			URL:  "http://localhost:8080",
			Capabilities: AgentCapabilities{
				Streaming: true,
			},
		},
		tasks: make(map[string]*Task),
	}

	ctx := context.Background()

	task, err := handler.SendTask(ctx, SendTaskRequest{
		ID:      "terminal-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if task.State != TaskStateCompleted {
		t.Fatalf("expected completed, got %s", task.State)
	}

	_, err = handler.SendTask(ctx, SendTaskRequest{
		ID:      "terminal-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("More info")}},
	})
	if err == nil {
		t.Fatal("expected error when appending to terminal task")
	}
}

// ---------------------------------------------------------------------------
// SSE Streaming Tests
// ---------------------------------------------------------------------------
