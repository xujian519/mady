package a2a

// 本文件是协议面契约护栏（M67-2）。契约清单见
// docs/code-health/protocol-contract-baseline.md §3.1。
//
// A2A 的端点路径、JSON-RPC 方法名、错误码、TaskState/Role/PartType 枚举值、
// SSE 帧格式都是与第三方 Agent 客户端互操作的线协议。这些断言刻意写成
// 「逐字比对」，任何改动都属破坏性变更，必须走协议变更流程（改 fixture +
// 改文档 + 人工 review），不能在重构卡里顺手改。

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestContractA2AErrorCodes 锁定 JSON-RPC 标准码与 A2A 自定义码的数值。
// 客户端按数值分支处理（如 -32001 提示任务不存在），改数值即断约定。
func TestContractA2AErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"JSONRPCParseError", JSONRPCParseError, -32700},
		{"JSONRPCInvalidRequest", JSONRPCInvalidRequest, -32600},
		{"JSONRPCMethodNotFound", JSONRPCMethodNotFound, -32601},
		{"JSONRPCInvalidParams", JSONRPCInvalidParams, -32602},
		{"JSONRPCInternalError", JSONRPCInternalError, -32603},
		{"A2AErrorTaskNotFound", A2AErrorTaskNotFound, -32001},
		{"A2AErrorTaskNotCancelable", A2AErrorTaskNotCancelable, -32002},
		{"A2AErrorPushNotSupported", A2AErrorPushNotSupported, -32003},
		{"A2AErrorUnsupportedOperation", A2AErrorUnsupportedOperation, -32004},
		{"A2AErrorContentTypeNotSupported", A2AErrorContentTypeNotSupported, -32005},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("错误码 %s 漂移：当前 %d，契约要求 %d", c.name, c.got, c.want)
		}
	}
}

// TestContractA2AEnums 锁定任务状态、角色、Part 类型的线值。
func TestContractA2AEnums(t *testing.T) {
	states := []struct {
		got  TaskState
		want string
	}{
		{TaskStateSubmitted, "submitted"},
		{TaskStateWorking, "working"},
		{TaskStateInputRequired, "input-required"},
		{TaskStateCompleted, "completed"},
		{TaskStateFailed, "failed"},
		{TaskStateCanceled, "canceled"},
	}
	if len(states) != 6 {
		t.Fatalf("TaskState 应有 6 个取值，当前 fixture 为 %d", len(states))
	}
	for _, c := range states {
		if string(c.got) != c.want {
			t.Errorf("TaskState 漂移：当前 %q，契约要求 %q", c.got, c.want)
		}
	}

	roles := []struct {
		got  Role
		want string
	}{{RoleUser, "user"}, {RoleAgent, "agent"}}
	for _, c := range roles {
		if string(c.got) != c.want {
			t.Errorf("Role 漂移：当前 %q，契约要求 %q", c.got, c.want)
		}
	}

	partTypes := []struct {
		got  PartType
		want string
	}{{PartTypeText, "text"}, {PartTypeFile, "file"}, {PartTypeData, "data"}}
	for _, c := range partTypes {
		if string(c.got) != c.want {
			t.Errorf("PartType 漂移：当前 %q，契约要求 %q", c.got, c.want)
		}
	}
}

// TestContractA2AJSONRPCEnvelope 锁定 JSON-RPC 信封与 SSE 事件载荷的字段名。
func TestContractA2AJSONRPCEnvelope(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want string
	}{
		{
			"请求",
			JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tasks/get", Params: json.RawMessage(`{"id":"t1"}`)},
			`{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"id":"t1"}}`,
		},
		{
			"成功响应",
			JSONRPCResponse{JSONRPC: "2.0", ID: 1, Result: map[string]any{"id": "t1"}},
			`{"jsonrpc":"2.0","id":1,"result":{"id":"t1"}}`,
		},
		{
			"错误响应",
			JSONRPCResponse{JSONRPC: "2.0", ID: 1, Error: &JSONRPCError{Code: A2AErrorTaskNotFound, Message: "task not found"}},
			`{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"task not found"}}`,
		},
		{
			"SSE 任务更新载荷",
			TaskUpdateEvent{ID: "t1", Final: true},
			`{"id":"t1","final":true}`,
		},
	}
	for _, c := range cases {
		got, err := json.Marshal(c.val)
		if err != nil {
			t.Fatalf("%s 序列化失败：%v", c.name, err)
		}
		if string(got) != c.want {
			t.Errorf("%s 字段漂移：\n当前 %s\n契约 %s", c.name, got, c.want)
		}
	}
}

// TestContractA2AEndpoints 锁定传输端点与入口错误行为。
func TestContractA2AEndpoints(t *testing.T) {
	server := NewServer(newMockHandler())
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	t.Run("Agent Card", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/.well-known/agent.json")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("状态码 = %d，期望 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("Content-Type = %q，期望 application/json", ct)
		}
		var card AgentCard
		if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
			t.Fatalf("解析 Agent Card 失败：%v", err)
		}
		if card.Name != "test-agent" {
			t.Fatalf("Agent Card name = %q", card.Name)
		}
	})

	t.Run("健康检查", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/health")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("状态码 = %d，期望 200", resp.StatusCode)
		}
	})

	t.Run("联邦端点未启用时走 JSON-RPC 兜底", func(t *testing.T) {
		// 未注册 federationRegistry 时不挂载 /federation/agents，GET 会落到
		// "/" 的 JSON-RPC 入口，返回 -32600 而不是联邦 Agent 列表。
		resp, err := http.Get(ts.URL + "/federation/agents")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		rpcErr := decodeJSONRPCError(t, resp.Body)
		if rpcErr.Code != JSONRPCInvalidRequest {
			t.Fatalf("错误码 = %d，未启用联邦时期望兜底为 %d", rpcErr.Code, JSONRPCInvalidRequest)
		}
	})

	t.Run("非 POST 请求返回 invalid request", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		rpcErr := decodeJSONRPCError(t, resp.Body)
		if rpcErr.Code != JSONRPCInvalidRequest {
			t.Fatalf("错误码 = %d，期望 %d", rpcErr.Code, JSONRPCInvalidRequest)
		}
	})

	t.Run("非 JSON Content-Type 被拒", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/", "text/plain", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tasks/query","params":{}}`))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		rpcErr := decodeJSONRPCError(t, resp.Body)
		if rpcErr.Code != JSONRPCInvalidRequest {
			t.Fatalf("错误码 = %d，期望 %d", rpcErr.Code, JSONRPCInvalidRequest)
		}
		if !strings.Contains(rpcErr.Message, "Content-Type") {
			t.Fatalf("错误消息 = %q，期望包含 Content-Type", rpcErr.Message)
		}
	})

	t.Run("未知方法返回 method not found", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","id":1,"method":"tasks/nope","params":{}}`
		resp, err := http.Post(ts.URL+"/", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("状态码 = %d，JSON-RPC 错误应复用 HTTP 200", resp.StatusCode)
		}
		rpcErr := decodeJSONRPCError(t, resp.Body)
		if rpcErr.Code != JSONRPCMethodNotFound {
			t.Fatalf("错误码 = %d，期望 %d", rpcErr.Code, JSONRPCMethodNotFound)
		}
		if rpcErr.Message != `method "tasks/nope" not found` {
			t.Fatalf("错误消息 = %q，契约格式为 method %%q not found", rpcErr.Message)
		}
	})
}

// TestContractA2AHTTPMethodRegistry 锁定 HTTP 入口登记的 8 个 JSON-RPC 方法。
// 用非法参数触发业务层错误，只要不是 -32601 即说明方法已登记。
func TestContractA2AHTTPMethodRegistry(t *testing.T) {
	methods := []string{
		"tasks/send",
		"tasks/sendSubscribe",
		"tasks/get",
		"tasks/cancel",
		"tasks/query",
		"tasks/pushNotification/set",
		"tasks/pushNotification/get",
		"tasks/resubscribe",
	}
	if len(methods) != 8 {
		t.Fatalf("HTTP 入口应有 8 个方法，当前 fixture 为 %d", len(methods))
	}

	server := NewServer(newMockHandler())
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			// params 用非法类型，强制走各 handler 的参数解析分支：流式方法不会
			// 进入 SSE 推流，响应保持为单帧 JSON-RPC 错误。
			body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":"invalid"}`
			resp, err := http.Post(ts.URL+"/", "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			var rpcResp JSONRPCResponse
			if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
				t.Fatalf("解析响应失败：%v", err)
			}
			if rpcResp.Error != nil && rpcResp.Error.Code == JSONRPCMethodNotFound {
				t.Fatalf("方法 %q 被判为不存在（-32601），契约要求已登记", method)
			}
		})
	}
}

// TestContractA2AWSMethodRegistry 锁定 WebSocket 入口登记的方法集合。
// WS 额外支持 tasks/subscribe（HTTP 入口对应 sendSubscribe）。
func TestContractA2AWSMethodRegistry(t *testing.T) {
	methods := []string{
		"tasks/send",
		"tasks/get",
		"tasks/cancel",
		"tasks/query",
		"tasks/subscribe",
		"tasks/resubscribe",
		"tasks/pushNotification/set",
		"tasks/pushNotification/get",
	}
	if len(methods) != 8 {
		t.Fatalf("WebSocket 入口应有 8 个方法，当前 fixture 为 %d", len(methods))
	}

	server := NewServer(newMockHandler())
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	for i, method := range methods {
		// params 用非法类型，强制走各 handler 的参数解析分支；只要不是 -32601
		// 就说明该方法已在 wsReadLoop 的 switch 中登记。
		req := map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": method, "params": "invalid"}
		if err := conn.WriteJSON(req); err != nil {
			t.Fatalf("发送 %s 失败：%v", method, err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var resp JSONRPCResponse
		if err := conn.ReadJSON(&resp); err != nil {
			t.Fatalf("读取 %s 响应失败：%v", method, err)
		}
		if resp.Error == nil {
			continue
		}
		if resp.Error.Code == JSONRPCMethodNotFound {
			t.Fatalf("WS 方法 %q 被判为不存在（-32601），契约要求已登记", method)
		}
	}
}

// TestContractA2ASSEFrame 锁定 SSE 帧格式：A2A 不使用 event: 行，
// 事件类型在 JSON 载荷内部。
func TestContractA2ASSEFrame(t *testing.T) {
	srv := &Server{}
	ev := &TaskUpdateEvent{ID: "t1", Final: true}

	rec := httptest.NewRecorder()
	if !srv.writeSSEEvent(rec, rec, ev, 7) {
		t.Fatal("写入带 id 的 SSE 帧失败")
	}
	if got, want := rec.Body.String(), "id: 7\ndata: {\"id\":\"t1\",\"final\":true}\n\n"; got != want {
		t.Fatalf("带 id 帧格式漂移：\n当前 %q\n契约 %q", got, want)
	}

	rec = httptest.NewRecorder()
	if !srv.writeSSEEvent(rec, rec, ev, 0) {
		t.Fatal("写入无 id 的 SSE 帧失败")
	}
	if got, want := rec.Body.String(), "data: {\"id\":\"t1\",\"final\":true}\n\n"; got != want {
		t.Fatalf("无 id 帧格式漂移：\n当前 %q\n契约 %q", got, want)
	}

	rec = httptest.NewRecorder()
	if !srv.writeSSEComment(rec, rec, "ping") {
		t.Fatal("写入 SSE 注释帧失败")
	}
	if got, want := rec.Body.String(), ": ping\n\n"; got != want {
		t.Fatalf("注释帧格式漂移：\n当前 %q\n契约 %q", got, want)
	}
}

func decodeJSONRPCError(t *testing.T, r io.Reader) *JSONRPCError {
	t.Helper()
	var resp JSONRPCResponse
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		t.Fatalf("解析 JSON-RPC 响应失败：%v", err)
	}
	if resp.Error == nil {
		t.Fatal("期望错误响应，实际为成功响应")
	}
	return resp.Error
}
