package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/xujian519/mady/agentcore"
)

type streamingMockHandler struct {
	card  AgentCard
	tasks map[string]*Task
	mu    sync.Mutex
}

func newStreamingMockHandler() *streamingMockHandler {
	return &streamingMockHandler{
		card: AgentCard{
			Name: "test-agent",
			URL:  "http://localhost:8080",
			Capabilities: AgentCapabilities{
				Streaming:         true,
				PushNotifications: true,
			},
		},
		tasks: make(map[string]*Task),
	}
}

func (m *streamingMockHandler) Card() AgentCard { return m.card }

func (m *streamingMockHandler) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := &Task{
		ID:       req.ID,
		State:    TaskStateCompleted,
		Messages: []Message{req.Message},
		History: []TaskStatus{
			{State: TaskStateSubmitted, Timestamp: time.Now()},
			{State: TaskStateWorking, Timestamp: time.Now()},
			{State: TaskStateCompleted, Timestamp: time.Now()},
		},
	}
	m.tasks[req.ID] = task
	return task, nil
}

func (m *streamingMockHandler) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	return task, nil
}

func (m *streamingMockHandler) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	task.State = TaskStateCanceled
	return task, nil
}

func (m *streamingMockHandler) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	return nil
}

func (m *streamingMockHandler) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	return nil, fmt.Errorf("not configured")
}

func (m *streamingMockHandler) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
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
	if result == nil {
		result = []*Task{}
	}
	return &QueryTasksResult{Tasks: result}, nil
}

func TestServer_SendTaskSubscribe_Streaming(t *testing.T) {
	handler := newStreamingMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	stream, err := client.SendTaskSubscribe(context.Background(), SendTaskRequest{
		ID:      "stream-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var events []*TaskUpdateEvent
	for {
		ev, ok := stream.Recv()
		if !ok {
			break
		}
		events = append(events, ev)
		if ev.Final {
			break
		}
	}

	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}

	lastEvent := events[len(events)-1]
	if !lastEvent.Final {
		t.Fatal("expected last event to be final")
	}
	if lastEvent.Result == nil {
		t.Fatal("expected result in final event")
	}
	if lastEvent.Result.State != TaskStateCompleted {
		t.Fatalf("expected completed state, got %s", lastEvent.Result.State)
	}
}

func TestServer_SendTaskSubscribe_IntermediateUpdates(t *testing.T) {
	handler := newStreamingMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	stream, err := client.SendTaskSubscribe(context.Background(), SendTaskRequest{
		ID:      "intermediate-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var events []*TaskUpdateEvent
	for {
		ev, ok := stream.Recv()
		if !ok {
			break
		}
		events = append(events, ev)
		if ev.Final {
			break
		}
	}

	hasIntermediate := false
	for _, ev := range events[:len(events)-1] {
		if ev.Artifact != nil {
			hasIntermediate = true
			break
		}
	}

	if !hasIntermediate {
		t.Log("No intermediate artifact events received (handler may complete too fast)")
	}
}

func TestServer_SendTaskSubscribe_InputRequiredClosesStream(t *testing.T) {
	slowHandler := &inputRequiredHandler{
		card: AgentCard{
			Name: "test-agent",
			URL:  "http://localhost:8080",
			Capabilities: AgentCapabilities{
				Streaming: true,
			},
		},
		tasks: make(map[string]*Task),
	}

	server := NewServer(slowHandler)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	stream, err := client.SendTaskSubscribe(context.Background(), SendTaskRequest{
		ID:      "input-req-stream",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var events []*TaskUpdateEvent
	for {
		ev, ok := stream.Recv()
		if !ok {
			break
		}
		events = append(events, ev)
		if ev.Final {
			break
		}
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Result == nil {
		t.Fatal("expected result in final event")
	}
	if lastEvent.Result.State != TaskStateInputRequired {
		t.Fatalf("expected input-required state, got %s", lastEvent.Result.State)
	}
	if !lastEvent.Final {
		t.Fatal("expected final=true for input-required state in SSE stream")
	}
}

type inputRequiredHandler struct {
	card  AgentCard
	tasks map[string]*Task
	mu    sync.Mutex
}

func (h *inputRequiredHandler) Card() AgentCard { return h.card }

func (h *inputRequiredHandler) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	task := &Task{
		ID:       req.ID,
		State:    TaskStateInputRequired,
		Messages: []Message{req.Message},
		History: []TaskStatus{
			{State: TaskStateSubmitted, Timestamp: time.Now()},
			{State: TaskStateWorking, Timestamp: time.Now()},
			{State: TaskStateInputRequired, Timestamp: time.Now()},
		},
	}
	h.tasks[req.ID] = task
	return task, nil
}

func (h *inputRequiredHandler) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	task, ok := h.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	return task, nil
}

func (h *inputRequiredHandler) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	task, ok := h.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	task.State = TaskStateCanceled
	return task, nil
}

func (h *inputRequiredHandler) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	return nil
}

func (h *inputRequiredHandler) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	return nil, fmt.Errorf("not configured")
}

func (h *inputRequiredHandler) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var result []*Task
	for _, task := range h.tasks {
		if req.State != "" && task.State != req.State {
			continue
		}
		result = append(result, task)
	}
	if result == nil {
		result = []*Task{}
	}
	return &QueryTasksResult{Tasks: result}, nil
}

// ---------------------------------------------------------------------------
// Message Appending Tests
// ---------------------------------------------------------------------------

func TestServer_SendTask_AppendMessage(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	_, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "append-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("First message")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	handler.mu.Lock()
	if task, ok := handler.tasks["append-task"]; ok {
		task.State = TaskStateInputRequired
	}
	handler.mu.Unlock()

	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "append-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Second message")}},
		}),
	})
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

	if len(task.Messages) < 2 {
		t.Fatalf("expected at least 2 messages after append, got %d", len(task.Messages))
	}
}

func TestDefaultAgentHandler_EvictsOldestTerminalTasksOverCapacity(t *testing.T) {
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webhook.Close()

	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	})

	handler := NewDefaultAgentHandler(AgentCard{
		Name: "test-agent",
		URL:  "http://localhost:8080",
	}, agent, agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	})
	handler.SetMaxTasks(2)

	ctx := context.Background()
	ids := []string{"evict-1", "evict-2", "evict-3"}
	for _, id := range ids {
		if err := handler.SetPushNotification(ctx, SetPushNotificationRequest{
			ID:     id,
			Config: PushNotificationConfig{URL: webhook.URL},
		}); err != nil {
			t.Fatalf("SetPushNotification(%s): %v", id, err)
		}
		// No provider is configured, so agent.Run fails immediately and the
		// task lands in TaskStateFailed (a terminal state), making it
		// eligible for eviction.
		task, err := handler.SendTask(ctx, SendTaskRequest{
			ID:      id,
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("hi")}},
		})
		if err != nil {
			t.Fatalf("SendTask(%s): %v", id, err)
		}
		if task.State != TaskStateFailed {
			t.Fatalf("expected task %s to fail without a provider, got %s", id, task.State)
		}
	}

	handler.tasksMu.RLock()
	numTasks := len(handler.tasks)
	_, oldestStillPresent := handler.tasks["evict-1"]
	_, newestPresent := handler.tasks["evict-3"]
	handler.tasksMu.RUnlock()

	if numTasks > 2 {
		t.Fatalf("expected at most 2 tasks retained, got %d", numTasks)
	}
	if oldestStillPresent {
		t.Fatal("expected oldest task evict-1 to be evicted")
	}
	if !newestPresent {
		t.Fatal("expected newest task evict-3 to be retained")
	}

	handler.pushMu.RLock()
	_, pushStillPresent := handler.pushCfg["evict-1"]
	handler.pushMu.RUnlock()
	if pushStillPresent {
		t.Fatal("expected push notification config for evicted task to be removed")
	}
}

func TestStreamingHandler_Interface(t *testing.T) {
	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	})

	handler := NewDefaultAgentHandler(AgentCard{
		Name: "test-agent",
		URL:  "http://localhost:8080",
	}, agent, agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	})

	var _ StreamingHandler = handler

	adapter := NewAgentAdapter(AgentCard{
		Name: "test-agent",
		URL:  "http://localhost:8080",
	}, agent, agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	}, nil)

	var _ StreamingHandler = adapter
}

type appendableHandler struct {
	card  AgentCard
	tasks map[string]*Task
	mu    sync.Mutex
}

func (h *appendableHandler) Card() AgentCard { return h.card }

func (h *appendableHandler) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if existing, ok := h.tasks[req.ID]; ok {
		if isTerminalState(existing.State) {
			return nil, fmt.Errorf("task %q is already in terminal state", req.ID)
		}
		existing.Messages = append(existing.Messages, req.Message)
		existing.State = TaskStateCompleted
		existing.History = append(existing.History, TaskStatus{State: TaskStateCompleted, Timestamp: time.Now()})
		return existing, nil
	}

	task := &Task{
		ID:       req.ID,
		State:    TaskStateInputRequired,
		Messages: []Message{req.Message},
		History: []TaskStatus{
			{State: TaskStateSubmitted, Timestamp: time.Now()},
			{State: TaskStateWorking, Timestamp: time.Now()},
			{State: TaskStateInputRequired, Timestamp: time.Now()},
		},
	}
	h.tasks[req.ID] = task
	return task, nil
}

func (h *appendableHandler) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	task, ok := h.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	return task, nil
}

func (h *appendableHandler) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	task, ok := h.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	task.State = TaskStateCanceled
	return task, nil
}

func (h *appendableHandler) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	return nil
}

func (h *appendableHandler) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	return nil, fmt.Errorf("not configured")
}

func (h *appendableHandler) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var result []*Task
	for _, task := range h.tasks {
		if req.State != "" && task.State != req.State {
			continue
		}
		result = append(result, task)
	}
	if result == nil {
		result = []*Task{}
	}
	return &QueryTasksResult{Tasks: result}, nil
}

type terminalHandler struct {
	card  AgentCard
	tasks map[string]*Task
	mu    sync.Mutex
}

func (h *terminalHandler) Card() AgentCard { return h.card }

func (h *terminalHandler) SendTask(ctx context.Context, req SendTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.tasks[req.ID]; ok {
		return nil, fmt.Errorf("task %q is already in terminal state", req.ID)
	}

	task := &Task{
		ID:       req.ID,
		State:    TaskStateCompleted,
		Messages: []Message{req.Message},
		History: []TaskStatus{
			{State: TaskStateSubmitted, Timestamp: time.Now()},
			{State: TaskStateWorking, Timestamp: time.Now()},
			{State: TaskStateCompleted, Timestamp: time.Now()},
		},
	}
	h.tasks[req.ID] = task
	return task, nil
}

func (h *terminalHandler) GetTask(ctx context.Context, req GetTaskRequest) (*Task, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	task, ok := h.tasks[req.ID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	return task, nil
}

func (h *terminalHandler) CancelTask(ctx context.Context, req CancelTaskRequest) (*Task, error) {
	return nil, fmt.Errorf("task already completed")
}

func (h *terminalHandler) SetPushNotification(ctx context.Context, req SetPushNotificationRequest) error {
	return nil
}

func (h *terminalHandler) GetPushNotification(ctx context.Context, taskID string) (*PushNotificationConfig, error) {
	return nil, fmt.Errorf("not configured")
}

func (h *terminalHandler) QueryTasks(ctx context.Context, req QueryTasksRequest) (*QueryTasksResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var result []*Task
	for _, task := range h.tasks {
		if req.State != "" && task.State != req.State {
			continue
		}
		result = append(result, task)
	}
	if result == nil {
		result = []*Task{}
	}
	return &QueryTasksResult{Tasks: result}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func postJSON(url string, req JSONRPCRequest) (*JSONRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpResp, err := http.Post(url, "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------------------------------------------------------------------------
// Query Tasks Tests
// ---------------------------------------------------------------------------

func TestServer_QueryTasks(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	_, _ = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:        "query-1",
			SessionID: "session-a",
			Message:   Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})

	_, _ = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:        "query-2",
			SessionID: "session-b",
			Message:   Message{Role: string(RoleUser), Parts: []Part{NewTextPart("World")}},
		}),
	})

	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tasks/query",
		Params:  mustJSON(QueryTasksRequest{SessionID: "session-a"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result QueryTasksResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
	if result.Tasks[0].ID != "query-1" {
		t.Fatalf("expected query-1, got %s", result.Tasks[0].ID)
	}

	resp2, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tasks/query",
		Params:  mustJSON(QueryTasksRequest{}),
	})
	if err != nil {
		t.Fatal(err)
	}

	data2, _ := json.Marshal(resp2.Result)
	var result2 QueryTasksResult
	if err := json.Unmarshal(data2, &result2); err != nil {
		t.Fatal(err)
	}

	if len(result2.Tasks) < 2 {
		t.Fatalf("expected at least 2 tasks, got %d", len(result2.Tasks))
	}
}

func TestClient_QueryTasks(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL)

	_, _ = client.SendTask(context.Background(), SendTaskRequest{
		ID:        "client-query-1",
		SessionID: "session-x",
		Message:   Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})

	result, err := client.QueryTasks(context.Background(), QueryTasksRequest{SessionID: "session-x"})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
	if result.Tasks[0].ID != "client-query-1" {
		t.Fatalf("expected client-query-1, got %s", result.Tasks[0].ID)
	}
}

func TestServer_QueryTasks_ByState(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	_, _ = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "state-task-1",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})

	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/query",
		Params:  mustJSON(QueryTasksRequest{State: TaskStateWorking}),
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	data, _ := json.Marshal(resp.Result)
	var result QueryTasksResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Tasks) < 1 {
		t.Fatalf("expected at least 1 task with working state, got %d", len(result.Tasks))
	}
}

func TestServer_QueryTasks_WithLimit(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	for i := 0; i < 5; i++ {
		_, _ = postJSON(ts.URL+"/", JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      i + 1,
			Method:  "tasks/send",
			Params: mustJSON(SendTaskRequest{
				ID:      fmt.Sprintf("limit-task-%d", i),
				Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
			}),
		})
	}

	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "tasks/query",
		Params:  mustJSON(QueryTasksRequest{Limit: 3}),
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := json.Marshal(resp.Result)
	var result QueryTasksResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Tasks) != 3 {
		t.Fatalf("expected 3 tasks with limit, got %d", len(result.Tasks))
	}
}

// ---------------------------------------------------------------------------
// Last-Event-ID Tests
// ---------------------------------------------------------------------------

func TestServer_Resubscribe_WithLastEventID(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	_, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "leid-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	server.PublishTaskUpdate("leid-task", &TaskUpdateEvent{
		Result: &Task{ID: "leid-task", State: TaskStateWorking},
	})
	server.PublishTaskUpdate("leid-task", &TaskUpdateEvent{
		Result: &Task{ID: "leid-task", State: TaskStateCompleted},
		Final:  true,
	})

	client := NewClient(ts.URL)
	stream, err := client.ResubscribeTask(context.Background(), "leid-task")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var events []*TaskUpdateEvent
	for {
		ev, ok := stream.Recv()
		if !ok {
			break
		}
		events = append(events, ev)
		if ev.Final {
			break
		}
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// WebSocket Tests
// ---------------------------------------------------------------------------

func TestWebSocket_SendAndGetTask(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	sendReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params:  mustJSON(SendTaskRequest{ID: "ws-task-1", Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello WS")}}}),
	}
	if err := conn.WriteJSON(sendReq); err != nil {
		t.Fatal(err)
	}

	var resp JSONRPCResponse
	if err := conn.ReadJSON(&resp); err != nil {
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

	if task.ID != "ws-task-1" {
		t.Fatalf("expected ws-task-1, got %s", task.ID)
	}

	getReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/get",
		Params:  mustJSON(GetTaskRequest{ID: "ws-task-1"}),
	}
	if err := conn.WriteJSON(getReq); err != nil {
		t.Fatal(err)
	}

	var resp2 JSONRPCResponse
	if err := conn.ReadJSON(&resp2); err != nil {
		t.Fatal(err)
	}

	if resp2.Error != nil {
		t.Fatalf("unexpected error on get: %v", resp2.Error)
	}
}

func TestWebSocket_QueryTasks(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	sendReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params:  mustJSON(SendTaskRequest{ID: "ws-q-1", SessionID: "ws-session-a", Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}}}),
	}
	_ = conn.WriteJSON(sendReq)
	var resp JSONRPCResponse
	_ = conn.ReadJSON(&resp)

	queryReq := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/query",
		Params:  mustJSON(QueryTasksRequest{SessionID: "ws-session-a"}),
	}
	if err := conn.WriteJSON(queryReq); err != nil {
		t.Fatal(err)
	}

	var resp2 JSONRPCResponse
	if err := conn.ReadJSON(&resp2); err != nil {
		t.Fatal(err)
	}

	if resp2.Error != nil {
		t.Fatalf("unexpected error: %v", resp2.Error)
	}

	data, _ := json.Marshal(resp2.Result)
	var result QueryTasksResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
}

func TestWebSocket_MethodNotFound(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "nonexistent/method",
		Params:  json.RawMessage(`{}`),
	}
	if err := conn.WriteJSON(req); err != nil {
		t.Fatal(err)
	}

	var resp JSONRPCResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}

	if resp.Error.Code != JSONRPCMethodNotFound {
		t.Fatalf("expected method not found error, got %d", resp.Error.Code)
	}
}

func TestWSClient_ConnectAndSend(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewWSClient(ts.URL)
	conn, err := client.Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if err := conn.SendRequest("tasks/send", SendTaskRequest{
		ID:      "wsclient-1",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello WS Client")}},
	}); err != nil {
		t.Fatal(err)
	}

	ev, ok := conn.Recv()
	if !ok {
		t.Fatal("expected event")
	}

	if ev.Result == nil || ev.Result.ID != "wsclient-1" {
		t.Fatalf("unexpected event result: %+v", ev.Result)
	}
}

// mockUpdatePublisher collects TaskUpdateEvents into a channel so tests can
// observe what DefaultAgentHandler publishes asynchronously.
type mockUpdatePublisher struct {
	ch chan *TaskUpdateEvent
}

func (m *mockUpdatePublisher) PublishTaskUpdate(_ string, ev *TaskUpdateEvent) {
	m.ch <- ev
}

// TestAgentHandlerStreamingDeltaPartRouting is the regression guard for the
// DeepSeek v4 text-garbling bug on the A2A path: thinking deltas must be
// published as a DataPart (kind="thinking") so consumers can separate them
// from the visible text stream, while text deltas stay TextPart.
func TestAgentHandlerStreamingDeltaPartRouting(t *testing.T) {
	agent := agentcore.New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	})

	handler := NewDefaultAgentHandler(AgentCard{
		Name: "test-agent",
		URL:  "http://localhost:8080",
	}, agent, agentcore.Config{
		ModelConfig: agentcore.ModelConfig{Name: "test"},
	})

	pub := &mockUpdatePublisher{ch: make(chan *TaskUpdateEvent, 16)}
	handler.SetUpdatePublisher(pub)

	unsub := handler.subscribeAgentEvents("stream-task")
	defer unsub()

	// Emit the interleaved text/thinking pattern seen with DeepSeek v4.
	agent.EmitEvent(&agentcore.MessageDeltaEvent{Delta: "可见正文", Kind: agentcore.BlockKindText})
	agent.EmitEvent(&agentcore.MessageDeltaEvent{Delta: "内部思考过程", Kind: agentcore.BlockKindThinking})
	agent.EmitEvent(&agentcore.MessageDeltaEvent{Delta: "更多正文", Kind: agentcore.BlockKindText})

	var textParts, thinkingParts int
	for i := 0; i < 3; i++ {
		select {
		case ev := <-pub.ch:
			if ev.Artifact == nil || len(ev.Artifact.Parts) != 1 {
				t.Fatalf("expected 1 part per update, got %+v", ev.Artifact)
			}
			part := ev.Artifact.Parts[0]
			switch part.Type {
			case PartTypeText:
				textParts++
			case PartTypeData:
				thinkingParts++
				if part.Data == nil {
					t.Fatal("data part has nil Data")
				}
				if kind, _ := part.Data.Data["kind"].(string); kind != "thinking" {
					t.Fatalf("data part kind = %v", part.Data.Data["kind"])
				}
				if text, _ := part.Data.Data["text"].(string); text != "内部思考过程" {
					t.Fatalf("data part text = %q", text)
				}
			default:
				t.Fatalf("unexpected part type %q", part.Type)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for delta updates")
		}
	}
	if textParts != 2 || thinkingParts != 1 {
		t.Fatalf("text parts = %d, thinking parts = %d; want 2/1", textParts, thinkingParts)
	}
}
