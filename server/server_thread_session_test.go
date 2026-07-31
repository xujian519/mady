package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/session"
)

func TestServerThreadEndpointsWithSessionStore(t *testing.T) {
	sessionFS, err := session.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	threadStore := session.NewAgentStore(sessionFS, "/project")
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: threadStore,
	})

	postChat(t, srv.Handler(), ChatRequest{Message: "hello", ThreadID: "thread-a"})
	postChat(t, srv.Handler(), ChatRequest{Message: "follow up", ThreadID: "thread-a"})
	postChat(t, srv.Handler(), ChatRequest{Message: "other", ThreadID: "thread-b"})

	var listResp struct {
		Threads []session.Info `json:"threads"`
	}
	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads", &listResp, http.StatusOK)
	if len(listResp.Threads) != 2 {
		t.Fatalf("threads len = %d", len(listResp.Threads))
	}

	var threadResp session.ThreadSnapshot
	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads/thread-a", &threadResp, http.StatusOK)
	if threadResp.Info.ID != "thread-a" {
		t.Fatalf("thread id = %q", threadResp.Info.ID)
	}
	if len(threadResp.Transcript) != 4 {
		t.Fatalf("transcript len = %d", len(threadResp.Transcript))
	}
	for i, item := range threadResp.Transcript {
		if item.EntryID == "" {
			t.Fatalf("transcript[%d] missing entry_id", i)
		}
	}
	if len(threadResp.Messages) != 4 {
		t.Fatalf("messages len = %d", len(threadResp.Messages))
	}
	if threadResp.Turn != 2 {
		t.Fatalf("turn = %d", threadResp.Turn)
	}

	var branchResp session.ThreadSnapshot
	postJSON(t, srv.Handler(), "/api/threads/thread-a/branch", nil, &branchResp, http.StatusOK)
	if branchResp.Info.ID == "thread-a" {
		t.Fatal("expected branch to create a new thread id")
	}
	if branchResp.Info.ParentSession != "thread-a" {
		t.Fatalf("parent_session = %q", branchResp.Info.ParentSession)
	}
	if len(branchResp.Messages) != 4 {
		t.Fatalf("branch messages len = %d", len(branchResp.Messages))
	}

	var historicalBranchResp session.ThreadSnapshot
	postJSON(t, srv.Handler(), "/api/threads/thread-a/branch", BranchThreadRequest{
		EntryID: threadResp.Transcript[1].EntryID,
	}, &historicalBranchResp, http.StatusOK)
	if historicalBranchResp.Info.ID == "thread-a" || historicalBranchResp.Info.ID == branchResp.Info.ID {
		t.Fatal("expected historical branch to create a distinct thread id")
	}
	if historicalBranchResp.Info.ParentSession != "thread-a" {
		t.Fatalf("historical parent_session = %q", historicalBranchResp.Info.ParentSession)
	}
	if len(historicalBranchResp.Messages) != 2 {
		t.Fatalf("historical branch messages len = %d", len(historicalBranchResp.Messages))
	}
	if historicalBranchResp.Messages[1].Content != "users:1 last:hello" {
		t.Fatalf("historical branch last message = %#v", historicalBranchResp.Messages[1])
	}

	deleteRequest(t, srv.Handler(), "/api/threads/thread-b", http.StatusNoContent)

	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads", &listResp, http.StatusOK)
	if len(listResp.Threads) != 3 {
		t.Fatalf("threads len after branch/delete = %d", len(listResp.Threads))
	}
	foundOriginal := false
	foundBranch := false
	foundHistoricalBranch := false
	for _, thread := range listResp.Threads {
		if thread.ID == "thread-a" {
			foundOriginal = true
		}
		if thread.ID == branchResp.Info.ID {
			foundBranch = true
		}
		if thread.ID == historicalBranchResp.Info.ID {
			foundHistoricalBranch = true
		}
	}
	if !foundOriginal || !foundBranch || !foundHistoricalBranch {
		t.Fatalf("threads = %#v", listResp.Threads)
	}
}

func TestServerChatAutoCreatesThreadForSessionStore(t *testing.T) {
	sessionFS, err := session.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	threadStore := session.NewAgentStore(sessionFS, "/project")
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: threadStore,
	})

	first := postChat(t, srv.Handler(), ChatRequest{Message: "hello"})
	if first.ThreadID == "" {
		t.Fatal("expected server-generated thread_id")
	}
	if first.Output != "users:1 last:hello" {
		t.Fatalf("first output = %q", first.Output)
	}

	second := postChat(t, srv.Handler(), ChatRequest{Message: "again", ThreadID: first.ThreadID})
	if second.ThreadID != first.ThreadID {
		t.Fatalf("second thread_id = %q want %q", second.ThreadID, first.ThreadID)
	}
	if second.Output != "users:2 last:again" {
		t.Fatalf("second output = %q", second.Output)
	}
}

func TestServerCreateThreadEndpoint(t *testing.T) {
	sessionFS, err := session.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	threadStore := session.NewAgentStore(sessionFS, "/project")
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: threadStore,
	})

	var threadResp session.ThreadSnapshot
	postJSON(t, srv.Handler(), "/api/threads", nil, &threadResp, http.StatusOK)
	if threadResp.Info.ID == "" {
		t.Fatal("expected created thread id")
	}
	if threadResp.Status != agentcore.StatusIdle {
		t.Fatalf("status = %q", threadResp.Status)
	}
	if len(threadResp.Messages) != 0 {
		t.Fatalf("messages len = %d", len(threadResp.Messages))
	}

	var loaded session.ThreadSnapshot
	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads/"+threadResp.Info.ID, &loaded, http.StatusOK)
	if loaded.Info.ID != threadResp.Info.ID {
		t.Fatalf("loaded id = %q want %q", loaded.Info.ID, threadResp.Info.ID)
	}
}

func TestServerThreadEndpointsRequireThreadCapableStore(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: newMemoryStore(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/threads", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/threads", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func postChat(t *testing.T, handler http.Handler, req ChatRequest) ChatResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}

	var resp ChatResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func getJSON(t *testing.T, handler http.Handler, method, path string, out any, wantStatus int) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if out == nil {
		return
	}
	if err := json.NewDecoder(w.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func getRaw(t *testing.T, handler http.Handler, method, path string, wantStatus int) string {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
		reader = &buf
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

type sseStreamHandle struct {
	ready  chan struct{}
	events chan sseEventRecord
	errs   chan error
	cancel context.CancelFunc
}

func openSSEStream(t *testing.T, url string, headers map[string]string) sseStreamHandle {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	handle := sseStreamHandle{
		ready:  make(chan struct{}),
		events: make(chan sseEventRecord, 4),
		errs:   make(chan error, 1),
		cancel: cancel,
	}
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			handle.errs <- err
			return
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			handle.errs <- err
			return
		}
		defer resp.Body.Close()
		close(handle.ready)

		scanner := bufio.NewScanner(resp.Body)
		var eventName string
		var dataLines []string
		flush := func() bool {
			if eventName == "" && len(dataLines) == 0 {
				return false
			}
			handle.events <- sseEventRecord{
				Event: eventName,
				Data:  json.RawMessage(strings.Join(dataLines, "\n")),
			}
			eventName = ""
			dataLines = nil
			return true
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, ":") {
				continue
			}
			if line == "" {
				flush()
				continue
			}
			if strings.HasPrefix(line, "event: ") {
				eventName = strings.TrimPrefix(line, "event: ")
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			handle.errs <- err
		}
	}()
	return handle
}

func nextSSEEvent(t *testing.T, stream sseStreamHandle, timeout time.Duration) sseEventRecord {
	t.Helper()
	select {
	case err := <-stream.errs:
		t.Fatalf("stream error: %v", err)
	case ev := <-stream.events:
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for sse event")
	}
	return sseEventRecord{}
}

func mustWriteSkillFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body any, out any, wantStatus int) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if out == nil {
		return
	}
	if err := json.NewDecoder(w.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func putJSON(t *testing.T, handler http.Handler, path string, body any, out any, wantStatus int) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPut, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if out == nil {
		return
	}
	if err := json.NewDecoder(w.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func deleteRequest(t *testing.T, handler http.Handler, path string, wantStatus int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != wantStatus {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func postChatStreamRaw(t *testing.T, handler http.Handler, req ChatRequest) string {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func newServerMCPStdioExtension(t *testing.T) *mcp.StdioExtension {
	t.Helper()
	serverMCPSetToolVersion(1)
	ext, err := mcp.NewStdioExtension(context.Background(), mcp.StdioConfig{
		Name:          "mcp-server-test",
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestServerMCPHelperProcess", "--"},
		Env:           []string{"GO_WANT_SERVER_MCP_HELPER_PROCESS=1"},
		ToolPrefix:    "mcp.",
		ClientName:    "mady-server-test",
		ClientVersion: "0.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ext
}

func TestServerMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_SERVER_MCP_HELPER_PROCESS") != "1" {
		return
	}
	runServerMCPHelper()
	os.Exit(0)
}

var serverMCPToolVersion = 1

func serverMCPSetToolVersion(v int) { serverMCPToolVersion = v }

func runServerMCPHelper() {
	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		id := msg["id"]
		switch method {
		case "initialize":
			writeServerMCPResponse(writer, id, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities": map[string]any{
					"tools": map[string]any{"listChanged": true},
				},
				"serverInfo": map[string]any{
					"name":    "fake-server-mcp",
					"version": "1.0.0",
				},
			})
		case "notifications/initialized":
		case "tools/list":
			writeServerMCPResponse(writer, id, map[string]any{
				"tools": serverMCPTools(),
			})
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			name, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]any)
			text, _ := args["text"].(string)
			if name == "echo" && text == "refresh-tools" {
				writeServerMCPResponse(writer, id, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "tool list updated"},
					},
				})
				serverMCPSetToolVersion(2)
				writeServerMCPNotification(writer, "notifications/tools/list_changed", nil)
				continue
			}
			if name == "reverse" {
				writeServerMCPResponse(writer, id, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": reverseServerMCPString(text)},
					},
				})
				continue
			}
			writeServerMCPResponse(writer, id, map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "echo: " + text},
				},
			})
		default:
			writeServerMCPError(writer, id, -32601, fmt.Sprintf("method not found: %s", method))
		}
	}
}

func serverMCPTools() []map[string]any {
	if serverMCPToolVersion == 2 {
		return []map[string]any{
			{
				"name":        "reverse",
				"description": "Reverse a string",
				"inputSchema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"text": map[string]any{"type": "string"},
					},
					"required": []string{"text"},
				},
			},
		}
	}
	return []map[string]any{
		{
			"name":        "echo",
			"description": "Echo back a string",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{"type": "string"},
				},
				"required": []string{"text"},
			},
		},
	}
}

func writeServerMCPResponse(w *bufio.Writer, id any, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	_ = w.Flush()
}

func writeServerMCPNotification(w *bufio.Writer, method string, params any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	_ = json.NewEncoder(w).Encode(msg)
	_ = w.Flush()
}

func writeServerMCPError(w *bufio.Writer, id any, code int64, message string) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	_ = w.Flush()
}

func reverseServerMCPString(v string) string {
	runes := []rune(v)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

type sseEventRecord struct {
	Event string
	Data  json.RawMessage
}

func parseSSEEvents(t *testing.T, body string) []sseEventRecord {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(body))
	var out []sseEventRecord
	var eventName string
	var dataLines []string
	flush := func() {
		if eventName == "" && len(dataLines) == 0 {
			return
		}
		out = append(out, sseEventRecord{
			Event: eventName,
			Data:  json.RawMessage(strings.Join(dataLines, "\n")),
		})
		eventName = ""
		dataLines = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan sse body: %v", err)
	}
	return out
}

func messagesContain(t *testing.T, msgs []agentcore.Message, needle string) bool {
	t.Helper()
	for _, msg := range msgs {
		if strings.Contains(msg.Content, needle) {
			return true
		}
	}
	return false
}
