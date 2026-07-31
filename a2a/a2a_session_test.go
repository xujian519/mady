package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestClient_AuthAPIKey(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler, WithAuth(AuthConfig{APIKey: "client-api-key"}))

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL, WithAPIKey("client-api-key"))

	task, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:      "auth-client-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "auth-client-task" {
		t.Fatalf("expected auth-client-task, got %s", task.ID)
	}
}

func TestClient_AuthBearer(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler, WithAuth(AuthConfig{BearerToken: "client-bearer"}))

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	client := NewClient(ts.URL, WithBearerToken("client-bearer"))

	task, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:      "auth-bearer-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "auth-bearer-task" {
		t.Fatalf("expected auth-bearer-task, got %s", task.ID)
	}
}

func TestServer_TaskTTLCleanup(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler, WithTaskTTL(100*time.Millisecond))

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Send a task and cancel it to put it in terminal state
	_, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "ttl-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel the task to make it terminal
	_, err = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/cancel",
		Params:  mustJSON(CancelTaskRequest{ID: "ttl-task"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify task history exists
	server.taskStatesMu.RLock()
	_, exists := server.taskStates["ttl-task"]
	server.taskStatesMu.RUnlock()
	if !exists {
		t.Fatal("task history should exist immediately after creation")
	}

	// Wait for TTL cleanup to purge the task.
	deadline := time.After(2 * time.Second)
	for {
		server.taskStatesMu.RLock()
		_, exists = server.taskStates["ttl-task"]
		server.taskStatesMu.RUnlock()
		if !exists {
			break
		}
		select {
		case <-deadline:
			t.Fatal("task state was not purged within TTL timeout")
		default:
		}
		runtime.Gosched()
	}

	server.taskStatesMu.RLock()
	_, exists = server.taskStates["ttl-task"]
	server.taskStatesMu.RUnlock()
	if exists {
		t.Fatal("task history should have been purged after TTL")
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected status ok, got %s", result["status"])
	}
}

func TestClient_Retry(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(JSONRPCResponse{JSONRPC: "2.0", Result: &Task{ID: "retry-task", State: TaskStateWorking}})
	}))
	defer ts.Close()

	client := NewClient(ts.URL, WithRetry(3, 10*time.Millisecond))

	task, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:      "retry-task",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "retry-task" {
		t.Fatalf("expected retry-task, got %s", task.ID)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestClient_RetryExhausted(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, WithRetry(2, 5*time.Millisecond))

	_, err := client.SendTask(context.Background(), SendTaskRequest{
		ID:      "retry-fail",
		Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
	})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if attempts != 3 { // initial + 2 retries
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

// ---------------------------------------------------------------------------
// Medium Priority Tests
// ---------------------------------------------------------------------------

func TestHistoryLength(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Create a task with multiple history entries
	_, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:      "history-task",
			Message: Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add some history entries manually
	handler.mu.Lock()
	if task, ok := handler.tasks["history-task"]; ok {
		task.History = []TaskStatus{
			{State: TaskStateSubmitted, Timestamp: time.Now().Add(-3 * time.Minute)},
			{State: TaskStateWorking, Timestamp: time.Now().Add(-2 * time.Minute)},
			{State: TaskStateCompleted, Timestamp: time.Now().Add(-1 * time.Minute)},
		}
	}
	handler.mu.Unlock()

	// Get task without history length limit
	resp, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/get",
		Params:  mustJSON(GetTaskRequest{ID: "history-task"}),
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := json.Marshal(resp.Result)
	var fullTask Task
	if err := json.Unmarshal(data, &fullTask); err != nil {
		t.Fatal(err)
	}
	if len(fullTask.History) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(fullTask.History))
	}

	// Get task with history length limit of 1
	resp, err = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tasks/get",
		Params:  mustJSON(GetTaskRequest{ID: "history-task", HistoryLength: 1}),
	})
	if err != nil {
		t.Fatal(err)
	}

	data, _ = json.Marshal(resp.Result)
	var limitedTask Task
	if err := json.Unmarshal(data, &limitedTask); err != nil {
		t.Fatal(err)
	}
	if len(limitedTask.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(limitedTask.History))
	}
	if limitedTask.History[0].State != TaskStateCompleted {
		t.Fatalf("expected completed state, got %s", limitedTask.History[0].State)
	}
}

func TestSessionManager(t *testing.T) {
	sm := NewSessionManager()

	// Test GetOrCreate
	session1 := sm.GetOrCreate("session-1")
	if session1.ID != "session-1" {
		t.Fatalf("expected session-1, got %s", session1.ID)
	}
	if len(session1.Tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(session1.Tasks))
	}

	// Test AddTask
	sm.AddTask("session-1", "task-1")
	sm.AddTask("session-1", "task-2")

	tasks := sm.GetTasks("session-1")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Test duplicate task ID is not added twice
	sm.AddTask("session-1", "task-1")
	tasks = sm.GetTasks("session-1")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks after duplicate add, got %d", len(tasks))
	}

	// Test Get
	session := sm.Get("session-1")
	if session == nil {
		t.Fatal("expected session to exist")
	}
	if len(session.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(session.Tasks))
	}

	// Test Get for non-existent session
	session = sm.Get("non-existent")
	if session != nil {
		t.Fatal("expected nil for non-existent session")
	}

	// Test List
	ids := sm.List()
	if len(ids) != 1 || ids[0] != "session-1" {
		t.Fatalf("unexpected session list: %v", ids)
	}

	// Test Delete
	sm.Delete("session-1")
	if sm.Get("session-1") != nil {
		t.Fatal("expected session to be deleted")
	}
}

func TestSessionManager_PurgeStale(t *testing.T) {
	sm := NewSessionManager()

	// Create sessions with different update times
	session1 := sm.GetOrCreate("stale-session")
	session1.UpdatedAt = time.Now().Add(-2 * time.Hour)

	session2 := sm.GetOrCreate("fresh-session")
	session2.UpdatedAt = time.Now()

	// Purge sessions older than 1 hour
	cutoff := time.Now().Add(-1 * time.Hour)
	count := sm.PurgeStale(cutoff)

	if count != 1 {
		t.Fatalf("expected 1 purged session, got %d", count)
	}

	if sm.Get("stale-session") != nil {
		t.Fatal("expected stale session to be purged")
	}

	if sm.Get("fresh-session") == nil {
		t.Fatal("expected fresh session to remain")
	}
}

func TestServer_SessionTracking(t *testing.T) {
	handler := newMockHandler()
	server := NewServer(handler, WithSessionManager(time.Hour))

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Send a task with session ID
	_, err := postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:        "session-task-1",
			SessionID: "user-session-1",
			Message:   Message{Role: string(RoleUser), Parts: []Part{NewTextPart("Hello")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Send another task with same session ID
	_, err = postJSON(ts.URL+"/", JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tasks/send",
		Params: mustJSON(SendTaskRequest{
			ID:        "session-task-2",
			SessionID: "user-session-1",
			Message:   Message{Role: string(RoleUser), Parts: []Part{NewTextPart("World")}},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify session tracking
	tasks := server.sessionMgr.GetTasks("user-session-1")
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks in session, got %d", len(tasks))
	}

	// Verify task IDs
	taskMap := make(map[string]bool)
	for _, id := range tasks {
		taskMap[id] = true
	}
	if !taskMap["session-task-1"] || !taskMap["session-task-2"] {
		t.Fatalf("expected both tasks in session, got %v", tasks)
	}
}
