package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock AgentHandler
// ---------------------------------------------------------------------------

type mockHandler struct {
	card      AgentCard
	tasks     map[string]*Task
	pushCfg   map[string]*PushNotificationConfig
	mu        sync.Mutex
	onSend    func(SendTaskRequest)
	onCancel  func(string)
	publisher TaskUpdatePublisher
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		tasks:   make(map[string]*Task),
		pushCfg: make(map[string]*PushNotificationConfig),
		card: AgentCard{
			Name:        "test-agent",
			Description: "A test agent",
			URL:         "http://localhost:8080",
			Version:     "1.0.0",
			Capabilities: AgentCapabilities{
				Streaming:         true,
				PushNotifications: true,
			},
			Skills: []AgentSkill{
				{ID: "greet", Name: "Greeting", Description: "Say hello"},
			},
		},
	}
}

func (m *mockHandler) Card() AgentCard { return m.card }

func (m *mockHandler) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	if m.onSend != nil {
		m.onSend(req)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.tasks[req.ID]; ok {
		if isTerminalState(existing.State) {
			return nil, fmt.Errorf("task %q is already in terminal state", req.ID)
		}
		existing.Messages = append(existing.Messages, req.Message)
		existing.State = TaskStateCompleted
		existing.History = append(existing.History, TaskStatus{State: TaskStateCompleted, Timestamp: time.Now()})
		return existing, nil
	}

	task := &Task{
		ID:        req.ID,
		SessionID: req.SessionID,
		State:     TaskStateWorking,
		Messages:  []Message{req.Message},
		Metadata:  req.Metadata,
		History: []TaskStatus{
			{State: TaskStateWorking, Timestamp: time.Now()},
		},
	}
	m.tasks[req.ID] = task
	return task, nil
}

func (m *mockHandler) SetUpdatePublisher(p TaskUpdatePublisher) {
	m.publisher = p
}

func (m *mockHandler) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}

	if req.HistoryLength > 0 && len(task.History) > req.HistoryLength {
		t := *task
		offset := len(task.History) - req.HistoryLength
		t.History = task.History[offset:]
		return &t, nil
	}

	return task, nil
}

func (m *mockHandler) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	if m.onCancel != nil {
		m.onCancel(req.ID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}

	if task.State == TaskStateCompleted {
		return nil, fmt.Errorf("task already completed")
	}

	task.State = TaskStateCanceled
	task.History = append(task.History, TaskStatus{State: TaskStateCanceled, Timestamp: time.Now()})
	return task, nil
}

func (m *mockHandler) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pushCfg[req.ID] = &req.Config
	return nil
}

func (m *mockHandler) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, ok := m.pushCfg[taskID]
	if !ok {
		return nil, fmt.Errorf("no push config")
	}
	return cfg, nil
}

func (m *mockHandler) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Task
	for _, task := range m.tasks {
		if req.SessionID != "" && task.SessionID != req.SessionID {
			continue
		}
		if req.State != "" && task.State != req.State {
			continue
		}
		result = append(result, task)
	}
	if req.Limit > 0 && len(result) > req.Limit {
		result = result[:req.Limit]
	}
	if result == nil {
		result = []*Task{}
	}
	return &QueryTasksResult{Tasks: result}, nil
}

// ---------------------------------------------------------------------------
// Server Tests
// ---------------------------------------------------------------------------

func TestServer_AgentCard(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/agent.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatal(err)
	}

	if card.Name != "test-agent" {
		t.Fatalf("expected name test-agent, got %s", card.Name)
	}
	if !card.Capabilities.Streaming {
		t.Fatal("expected streaming capability")
	}
	if len(card.Skills) != 1 || card.Skills[0].ID != "greet" {
		t.Fatalf("unexpected skills: %v", card.Skills)
	}
}

func TestServer_SendTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "task-1",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	}

	resp, err := postJSON(ts.URL+"/", req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}

	if task.ID != "task-1" {
		t.Fatalf("expected task-1, got %s", task.ID)
	}
	if task.State != TaskStateWorking {
		t.Fatalf("expected working, got %s", task.State)
	}
	if len(task.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(task.Messages))
	}
}

func TestServer_GetTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// First send a task
	sendReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "task-get",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	}
	_, _ = postJSON(ts.URL+"/", sendReq)

	// Then get it
	getReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/get",
		Params:  mustJSON(GetTaskRequest{ID: "task-get"}),
	}

	resp, err := postJSON(ts.URL+"/", getReq)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}

	if task.ID != "task-get" {
		t.Fatalf("expected task-get, got %s", task.ID)
	}
}

func TestServer_CancelTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Send a task
	sendReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "task-cancel",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	}
	_, _ = postJSON(ts.URL+"/", sendReq)

	// Cancel it
	cancelReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/cancel",
		Params:  mustJSON(CancelTaskRequest{ID: "task-cancel"}),
	}

	resp, err := postJSON(ts.URL+"/", cancelReq)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		t.Fatal(err)
	}

	if task.State != TaskStateCanceled {
		t.Fatalf("expected canceled, got %s", task.State)
	}
}

func TestServer_MethodNotFound(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/unknown",
	}

	resp, err := postJSON(ts.URL+"/", req)
	if err != nil {
		t.Fatal(err)
	}

	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != JSONRPCMethodNotFound {
		t.Fatalf("expected method not found, got %d", resp.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// Client Tests
// ---------------------------------------------------------------------------

func TestClient_GetAgentCard(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)
	card, err := client.GetAgentCard(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if card.Name != "test-agent" {
		t.Fatalf("expected test-agent, got %s", card.Name)
	}
}

func TestClient_SendTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)
	task, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:      "client-task-1",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello from client")}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if task.ID != "client-task-1" {
		t.Fatalf("expected client-task-1, got %s", task.ID)
	}
	if task.State != TaskStateWorking {
		t.Fatalf("expected working, got %s", task.State)
	}
}

func TestClient_GetTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	// Send first
	_, _ = client.SendTask(context.Background(), SendTaskRequest{
		ID:      "client-get-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})

	// Get
	task, err := client.GetTask(context.Background(), GetTaskRequest{ID: "client-get-task"})
	if err != nil {
		t.Fatal(err)
	}

	if task.ID != "client-get-task" {
		t.Fatalf("expected client-get-task, got %s", task.ID)
	}
}

func TestClient_CancelTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	// Send first
	_, _ = client.SendTask(context.Background(), SendTaskRequest{
		ID:      "client-cancel-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})

	// Cancel
	task, err := client.CancelTask(context.Background(), CancelTaskRequest{ID: "client-cancel-task"})
	if err != nil {
		t.Fatal(err)
	}

	if task.State != TaskStateCanceled {
		t.Fatalf("expected canceled, got %s", task.State)
	}
}

// ---------------------------------------------------------------------------
// Type Tests
// ---------------------------------------------------------------------------

func TestPartHelpers(t *testing.T) {
	textPart := NewTextPart("hello")
	if textPart.Type != PartTypeText || textPart.Text != "hello" {
		t.Fatal("text part mismatch")
	}

	dataPart := NewDataPart(map[string]any{"key": "value"})
	if dataPart.Type != PartTypeData || dataPart.Data == nil {
		t.Fatal("data part mismatch")
	}

	filePart := NewFilePartBytes("test.txt", "text/plain", "base64data")
	if filePart.Type != PartTypeFile || filePart.File.Name != "test.txt" {
		t.Fatal("file part mismatch")
	}

	uriPart := NewFilePartURI("test.txt", "text/plain", "http://example.com/file.txt")
	if uriPart.Type != PartTypeFile || uriPart.File.URI != "http://example.com/file.txt" {
		t.Fatal("uri part mismatch")
	}
}

func TestJSONRPCError(t *testing.T) {
	err := &JSONRPCError{Code: JSONRPCInternalError, Message: "something went wrong"}
	if err.Error() != "jsonrpc error -32603: something went wrong" {
		t.Fatalf("unexpected error string: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// SSE Decoder Tests
// ---------------------------------------------------------------------------

func TestSSEDecoder(t *testing.T) {
	input := `data: {"id":1,"result":{"id":"task-1"}}

data: {"id":2,"final":true}

`
	decoder := NewSSEDecoder(strings.NewReader(input))

	ev1, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if ev1 == nil {
		t.Fatal("expected event")
	}
	if !strings.Contains(ev1.Data, `"id":"task-1"`) {
		t.Fatalf("unexpected data: %s", ev1.Data)
	}

	ev2, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if ev2 == nil {
		t.Fatal("expected event")
	}
	if !strings.Contains(ev2.Data, `"final":true`) {
		t.Fatalf("unexpected data: %s", ev2.Data)
	}

	ev3, err := decoder.Next()
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if ev3 != nil {
		t.Fatal("expected nil event after EOF")
	}
}

func TestSSEDecoder_WithIDAndEvent(t *testing.T) {
	input := `id: msg-1
event: update
data: hello

`
	decoder := NewSSEDecoder(strings.NewReader(input))

	ev, err := decoder.Next()
	if err != nil {
		t.Fatal(err)
	}
	if ev.ID != "msg-1" {
		t.Fatalf("expected id msg-1, got %s", ev.ID)
	}
	if ev.Event != "update" {
		t.Fatalf("expected event update, got %s", ev.Event)
	}
	if ev.Data != "hello" {
		t.Fatalf("expected data hello, got %s", ev.Data)
	}
}

// ---------------------------------------------------------------------------
// Integration Test: Client -> Server round-trip
// ---------------------------------------------------------------------------

func TestIntegration_ClientServer(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	// Get agent card
	card, err := client.GetAgentCard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "test-agent" {
		t.Fatalf("unexpected card name: %s", card.Name)
	}

	// Send task
	task, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:       "integration-task",
		Message:  Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Test message")}},
		Metadata: map[string]any{"source": "integration_test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "integration-task" {
		t.Fatalf("unexpected task id: %s", task.ID)
	}
	if task.State != TaskStateWorking {
		t.Fatalf("unexpected task state: %s", task.State)
	}

	// Get task
	fetched, err := client.GetTask(context.Background(), GetTaskRequest{ID: "integration-task"})
	if err != nil {
		t.Fatal(err)
	}
	if fetched.ID != "integration-task" {
		t.Fatalf("unexpected fetched id: %s", fetched.ID)
	}

	// Set push notification
	err = client.SetPushNotification(context.Background(), SetPushNotificationRequest{
		ID: "integration-task",
		Config: PushNotificationConfig{
			URL:   "http://example.com/webhook",
			Token: "secret-token",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Get push notification
	cfg, err := client.GetPushNotification(context.Background(), "integration-task")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL != "http://example.com/webhook" {
		t.Fatalf("unexpected webhook url: %s", cfg.URL)
	}
	if cfg.Token != "secret-token" {
		t.Fatalf("unexpected token: %s", cfg.Token)
	}
}

// ---------------------------------------------------------------------------
// Handoff Integration Tests
// ---------------------------------------------------------------------------

func TestExtractTaskResult(t *testing.T) {
	task := &Task{
		Artifacts: []Artifact{{
			Parts: []Part{NewTextPart("artifact result")},
		}},
		Messages: []Message{
			{Role: string(RoleUser), Parts: []Part{NewTextPart("input")}},
			{Role: string(RoleAgent), Parts: []Part{NewTextPart("message result")}},
		},
	}

	// Should prefer artifact
	result := extractTaskResult(task, nil)
	if result != "artifact result" {
		t.Fatalf("expected artifact result, got %s", result)
	}

	// Without artifacts, should use last agent message
	task.Artifacts = nil
	result = extractTaskResult(task, nil)
	if result != "message result" {
		t.Fatalf("expected message result, got %s", result)
	}

	// Empty task
	result = extractTaskResult(nil, nil)
	if result != "" {
		t.Fatalf("expected empty, got %s", result)
	}
}

func TestExtractMessageText(t *testing.T) {
	msg := Message{
		Parts: []Part{
			NewTextPart("Hello "),
			NewTextPart("world"),
		},
	}
	text := extractMessageText(msg)
	if text != "Hello world" {
		t.Fatalf("expected 'Hello world', got %s", text)
	}
}
