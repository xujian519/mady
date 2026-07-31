package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/mcp"
	"github.com/xujian519/mady/skill"
)

func TestServerStreamChatEmitsMCPEvents(t *testing.T) {
	ext := newServerMCPStdioExtension(t)
	defer func() { _ = ext.Dispose() }()

	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: &serverMCPToolProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})

	body := postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "refresh",
		Stream:   true,
		ThreadID: "thread-stream",
	})
	if !strings.Contains(body, "event: mcp_capabilities_updated") {
		t.Fatalf("missing capabilities event in stream: %s", body)
	}
	if !strings.Contains(body, "\"schema\":\"mcp.capabilities_updated.v1\"") {
		t.Fatalf("missing capabilities schema in stream: %s", body)
	}
	if !strings.Contains(body, "\"thread_id\":\"thread-stream\"") {
		t.Fatalf("missing thread id in stream payload: %s", body)
	}
	if !strings.Contains(body, "\"transport\":\"stdio\"") {
		t.Fatalf("missing stdio transport payload in stream: %s", body)
	}
	if !strings.Contains(body, "\"schema\":\"chat.done.v1\"") || !strings.Contains(body, "\"type\":\"done\"") {
		t.Fatalf("missing structured done payload in stream: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("missing done event in stream: %s", body)
	}
}

func TestServerStreamChatWrapsAgentEventsInEnvelope(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
	})

	body := postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "hello",
		Stream:   true,
		ThreadID: "thread-agent-stream",
	})
	if !strings.Contains(body, "event: agent_start") {
		t.Fatalf("missing agent_start event in stream: %s", body)
	}
	if !strings.Contains(body, "\"schema\":\"agent.event.v1\"") {
		t.Fatalf("missing agent event schema in stream: %s", body)
	}
	if !strings.Contains(body, "\"thread_id\":\"thread-agent-stream\"") {
		t.Fatalf("missing thread id in agent event stream: %s", body)
	}
	if !strings.Contains(body, "\"type\":\"agent_start\"") {
		t.Fatalf("missing agent_start envelope type in stream: %s", body)
	}
	if !strings.Contains(body, "\"payload\":{\"input\":\"hello\"}") {
		t.Fatalf("missing normalized agent_start payload in stream: %s", body)
	}
}

func TestServerStreamChatEmitsSkillLoadedEvent(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills: []skill.Skill{{
				Name:        "planner",
				Description: "Plans work",
				FilePath:    "/skills/planner/SKILL.md",
				BaseDir:     "/skills/planner",
				Body:        "Plan carefully.",
			}},
		},
	})

	body := postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "/skill:planner gather requirements",
		Stream:   true,
		ThreadID: "thread-skill-stream",
	})
	if !strings.Contains(body, "event: skill_loaded") {
		t.Fatalf("missing skill_loaded event in stream: %s", body)
	}
	if !strings.Contains(body, "\"schema\":\"agent.event.v1\"") {
		t.Fatalf("missing agent event schema in skill stream: %s", body)
	}
	if !strings.Contains(body, "\"type\":\"skill_loaded\"") {
		t.Fatalf("missing skill_loaded type in stream: %s", body)
	}
	if !strings.Contains(body, "\"payload\":{\"skill_name\":\"planner\",\"path\":\"/skills/planner/SKILL.md\",\"source\":\"explicit_command\",\"arguments\":\"gather requirements\"}") {
		t.Fatalf("missing normalized skill payload in stream: %s", body)
	}
}

func TestServerStreamChatStructuredEventsDecode(t *testing.T) {
	ext := newServerMCPStdioExtension(t)
	defer func() { _ = ext.Dispose() }()

	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: &serverMCPToolProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})

	events := parseSSEEvents(t, postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "refresh",
		Stream:   true,
		ThreadID: "thread-structured",
	}))

	var capabilitiesData json.RawMessage
	var agentStartData json.RawMessage
	var toolCallEndData json.RawMessage
	var doneData json.RawMessage
	for _, ev := range events {
		switch ev.Event {
		case "mcp_capabilities_updated":
			capabilitiesData = ev.Data
		case "agent_start":
			agentStartData = ev.Data
		case "tool_call_end":
			toolCallEndData = ev.Data
		case "done":
			doneData = ev.Data
		}
	}
	if len(capabilitiesData) == 0 || len(agentStartData) == 0 || len(toolCallEndData) == 0 || len(doneData) == 0 {
		t.Fatalf("missing structured events: %#v", events)
	}

	var capabilitiesEvent MCPStreamCapabilitiesEvent
	if err := json.Unmarshal(capabilitiesData, &capabilitiesEvent); err != nil {
		t.Fatalf("decode capabilities event: %v", err)
	}
	if capabilitiesEvent.Schema != streamSchemaMCPAbilitiesUpdated || capabilitiesEvent.ThreadID != "thread-structured" {
		t.Fatalf("capabilities event = %#v", capabilitiesEvent)
	}

	var agentEvent struct {
		Schema   string          `json:"schema"`
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id"`
		Payload  json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(agentStartData, &agentEvent); err != nil {
		t.Fatalf("decode agent event: %v", err)
	}
	if agentEvent.Schema != streamSchemaAgentEvent || agentEvent.Type != "agent_start" || agentEvent.ThreadID != "thread-structured" {
		t.Fatalf("agent event = %#v", agentEvent)
	}
	var agentStartPayload AgentStartStreamPayload
	if err := json.Unmarshal(agentEvent.Payload, &agentStartPayload); err != nil {
		t.Fatalf("decode agent_start payload: %v", err)
	}
	if agentStartPayload.Input != "refresh" {
		t.Fatalf("agent_start payload = %#v", agentStartPayload)
	}

	var toolCallEvent struct {
		Schema   string          `json:"schema"`
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id"`
		Payload  json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(toolCallEndData, &toolCallEvent); err != nil {
		t.Fatalf("decode tool_call_end event: %v", err)
	}
	if toolCallEvent.Schema != streamSchemaAgentEvent || toolCallEvent.Type != "tool_call_end" || toolCallEvent.ThreadID != "thread-structured" {
		t.Fatalf("tool_call_end event = %#v", toolCallEvent)
	}
	var toolCallPayload ToolCallEndStreamPayload
	if err := json.Unmarshal(toolCallEvent.Payload, &toolCallPayload); err != nil {
		t.Fatalf("decode tool_call_end payload: %v", err)
	}
	if toolCallPayload.ToolCallID != "call_1" || toolCallPayload.ToolName != "mcp.echo" || toolCallPayload.Result == "" {
		t.Fatalf("tool_call_end payload = %#v", toolCallPayload)
	}

	var doneEvent StreamDoneEvent
	if err := json.Unmarshal(doneData, &doneEvent); err != nil {
		t.Fatalf("decode done event: %v", err)
	}
	if doneEvent.Schema != streamSchemaChatDone || doneEvent.Type != "done" || doneEvent.ThreadID != "thread-structured" || doneEvent.Output != "done" {
		t.Fatalf("done event = %#v", doneEvent)
	}
}

func TestServerStreamChatNormalizesAgentErrorEvent(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: failingProvider{},
		},
	})

	events := parseSSEEvents(t, postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "explode",
		Stream:   true,
		ThreadID: "thread-error",
	}))

	var agentErrorData json.RawMessage
	var doneData json.RawMessage
	for _, ev := range events {
		switch ev.Event {
		case "agent_error":
			agentErrorData = ev.Data
		case "done":
			doneData = ev.Data
		}
	}
	if len(agentErrorData) == 0 || len(doneData) == 0 {
		t.Fatalf("missing error events: %#v", events)
	}

	var agentErrorEvent struct {
		Schema   string          `json:"schema"`
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id"`
		Payload  json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(agentErrorData, &agentErrorEvent); err != nil {
		t.Fatalf("decode agent_error event: %v", err)
	}
	if agentErrorEvent.Schema != streamSchemaAgentEvent || agentErrorEvent.Type != "agent_error" || agentErrorEvent.ThreadID != "thread-error" {
		t.Fatalf("agent_error event = %#v", agentErrorEvent)
	}

	var payload AgentErrorStreamPayload
	if err := json.Unmarshal(agentErrorEvent.Payload, &payload); err != nil {
		t.Fatalf("decode agent_error payload: %v", err)
	}
	if !strings.Contains(payload.Error, "provider boom") {
		t.Fatalf("agent_error payload = %#v", payload)
	}

	var doneEvent StreamDoneEvent
	if err := json.Unmarshal(doneData, &doneEvent); err != nil {
		t.Fatalf("decode done event: %v", err)
	}
	if !strings.Contains(doneEvent.Error, "provider boom") {
		t.Fatalf("done event = %#v", doneEvent)
	}
}

func TestStreamEventPayloadShapesMCPToolsRefreshedEvent(t *testing.T) {
	payload := streamEventPayload("thread-tools", mcp.ToolsRefreshedEvent{
		Extension: "mcp-server-test",
		Transport: "stdio",
		OldTools:  []string{"mcp.echo"},
		NewTools:  []string{"mcp.reverse"},
	})

	ev, ok := payload.(MCPStreamToolsRefreshedEvent)
	if !ok {
		t.Fatalf("payload type = %T", payload)
	}
	if ev.Schema != streamSchemaMCPToolsRefreshed || ev.Type != string(mcp.EventMCPToolsRefreshed) || ev.ThreadID != "thread-tools" {
		t.Fatalf("tools refreshed event = %#v", ev)
	}
	if len(ev.OldTools) != 1 || ev.OldTools[0] != "mcp.echo" || len(ev.NewTools) != 1 || ev.NewTools[0] != "mcp.reverse" {
		t.Fatalf("tools refreshed payload = %#v", ev)
	}
}

func TestStreamEventPayloadShapesMCPRuntimeEvents(t *testing.T) {
	transportPayload := streamEventPayload("thread-runtime", mcp.TransportErrorEvent{
		Extension:   "mcp-http-test",
		Transport:   "http",
		Operation:   "server_stream",
		Message:     "boom",
		Reason:      "server_stream_unsupported",
		Recoverable: false,
	})
	transportEvent, ok := transportPayload.(MCPStreamTransportErrorEvent)
	if !ok {
		t.Fatalf("transport payload type = %T", transportPayload)
	}
	if transportEvent.Schema != streamSchemaMCPTransportError || transportEvent.Type != string(mcp.EventMCPTransportError) || transportEvent.Operation != "server_stream" {
		t.Fatalf("transport event = %#v", transportEvent)
	}

	reconnectPayload := streamEventPayload("thread-runtime", mcp.ReconnectEvent{
		Extension:      "mcp-http-test",
		Transport:      "http",
		Phase:          mcp.ReconnectPhaseSucceeded,
		Reason:         mcp.ReconnectReasonSessionExpired,
		Attempt:        1,
		StaleSessionID: "sess-1",
		SessionID:      "sess-2",
	})
	reconnectEvent, ok := reconnectPayload.(MCPStreamReconnectEvent)
	if !ok {
		t.Fatalf("reconnect payload type = %T", reconnectPayload)
	}
	if reconnectEvent.Schema != streamSchemaMCPReconnect || reconnectEvent.Type != string(mcp.EventMCPReconnect) || reconnectEvent.SessionID != "sess-2" {
		t.Fatalf("reconnect event = %#v", reconnectEvent)
	}

	refreshPayload := streamEventPayload("thread-runtime", mcp.RefreshEvent{
		Extension: "mcp-http-test",
		Transport: "http",
		Phase:     mcp.RefreshPhaseCoalesced,
		Reason:    "in_flight",
		Coalesced: true,
		InFlight:  true,
	})
	refreshEvent, ok := refreshPayload.(MCPStreamRefreshEvent)
	if !ok {
		t.Fatalf("refresh payload type = %T", refreshPayload)
	}
	if refreshEvent.Schema != streamSchemaMCPRefresh || refreshEvent.Type != string(mcp.EventMCPRefresh) || !refreshEvent.Coalesced {
		t.Fatalf("refresh event = %#v", refreshEvent)
	}
}

func TestAgentEventPayloadShapesSkillLoadedEvent(t *testing.T) {
	payload := agentEventPayload(agentcore.NewSkillLoadedEvent(
		"planner",
		"/skills/planner/SKILL.md",
		"model_selection",
		"scope work",
	))
	skillPayload, ok := payload.(SkillLoadedStreamPayload)
	if !ok {
		t.Fatalf("payload type = %T", payload)
	}
	if skillPayload.SkillName != "planner" || skillPayload.Source != "model_selection" || skillPayload.Arguments != "scope work" {
		t.Fatalf("skill payload = %#v", skillPayload)
	}
}

// TestAgentEventPayloadPreservesDeltaKind is the regression guard for the
// DeepSeek v4 text-garbling bug on the Server SSE path: the delta payload must
// carry the Kind field so SSE consumers can separate thinking content from the
// visible text stream.
func TestAgentEventPayloadPreservesDeltaKind(t *testing.T) {
	textPayload := agentEventPayload(&agentcore.MessageDeltaEvent{
		Delta: "可见正文",
		Kind:  agentcore.BlockKindText,
	})
	textDelta, ok := textPayload.(MessageDeltaStreamPayload)
	if !ok {
		t.Fatalf("payload type = %T", textPayload)
	}
	if textDelta.Delta != "可见正文" || textDelta.Kind != "text" {
		t.Fatalf("text delta payload = %#v", textDelta)
	}

	thinkingPayload := agentEventPayload(&agentcore.MessageDeltaEvent{
		Delta: "内部思考过程",
		Kind:  agentcore.BlockKindThinking,
	})
	thinkingDelta, ok := thinkingPayload.(MessageDeltaStreamPayload)
	if !ok {
		t.Fatalf("payload type = %T", thinkingPayload)
	}
	if thinkingDelta.Delta != "内部思考过程" || thinkingDelta.Kind != "thinking" {
		t.Fatalf("thinking delta payload = %#v", thinkingDelta)
	}
}

func TestServerStreamChatEmitsHTTPMCPReconnectEvents(t *testing.T) {
	var mu sync.Mutex
	initCount := 0
	callCount := 0

	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case http.MethodPost:
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := req["method"].(string)
		switch method {
		case "initialize":
			mu.Lock()
			initCount++
			current := initCount
			mu.Unlock()
			sessionID := "sess-1"
			if current > 1 {
				sessionID = "sess-2"
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", sessionID)
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"protocolVersion": "2025-11-25",
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "echo", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		case "tools/call":
			sessionID := r.Header.Get("Mcp-Session-Id")
			mu.Lock()
			callCount++
			currentCall := callCount
			mu.Unlock()
			if sessionID == "sess-1" && currentCall == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "tool list updated"},
					},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer mcpServer.Close()

	ext, err := mcp.NewHTTPExtension(context.Background(), mcp.HTTPConfig{
		Name:       "mcp-http-server-test",
		Endpoint:   mcpServer.URL,
		ToolPrefix: "mcp.",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ext.Dispose() }()

	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: &serverMCPToolProvider{},
		},
		Extensions: []agentcore.Extension{ext},
	})

	events := parseSSEEvents(t, postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "refresh",
		Stream:   true,
		ThreadID: "thread-http-reconnect",
	}))

	var reconnectData json.RawMessage
	for _, ev := range events {
		if ev.Event == "mcp_reconnect" {
			reconnectData = ev.Data
			break
		}
	}
	if len(reconnectData) == 0 {
		t.Fatalf("missing reconnect event: %#v", events)
	}

	var reconnectEvent MCPStreamReconnectEvent
	if err := json.Unmarshal(reconnectData, &reconnectEvent); err != nil {
		t.Fatalf("decode reconnect event: %v", err)
	}
	if reconnectEvent.Schema != streamSchemaMCPReconnect || reconnectEvent.ThreadID != "thread-http-reconnect" || reconnectEvent.Transport != "http" {
		t.Fatalf("reconnect event = %#v", reconnectEvent)
	}
	if reconnectEvent.Reason != mcp.ReconnectReasonSessionExpired && reconnectEvent.Reason != mcp.ReconnectReasonServerStream404 {
		t.Fatalf("unexpected reconnect reason = %#v", reconnectEvent)
	}
}

func TestServerStreamChatEmitsHTTPMCPTransportErrorEvents(t *testing.T) {
	var mu sync.Mutex
	getCount := 0
	firstGETStarted := make(chan struct{})
	releaseFirstGET := make(chan struct{})
	secondGETWritten := make(chan struct{})

	mcpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case http.MethodPost:
		case http.MethodGet:
			mu.Lock()
			getCount++
			currentGET := getCount
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			switch currentGET {
			case 1:
				close(firstGETStarted)
				<-releaseFirstGET
				_, _ = w.Write([]byte("id: first\n"))
				_, _ = w.Write([]byte("retry: 1\n"))
				_, _ = w.Write([]byte("data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/ping\"}\n\n"))
			case 2:
				_, _ = w.Write([]byte("data: {not-json}\n\n"))
				close(secondGETWritten)
			default:
				<-r.Context().Done()
			}
			return
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		method, _ := req["method"].(string)
		switch method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "sess-transport")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"protocolVersion": "2025-11-25",
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "echo", "inputSchema": map[string]any{"type": "object"}},
					},
				},
			})
		default:
			t.Fatalf("unexpected method %q", method)
		}
	}))
	defer mcpServer.Close()

	ext, err := mcp.NewHTTPExtension(context.Background(), mcp.HTTPConfig{
		Name:               "mcp-http-transport-test",
		Endpoint:           mcpServer.URL,
		EnableServerStream: true,
		NotificationHandler: func(ctx context.Context, method string, params json.RawMessage) error {
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ext.Dispose() }()

	provider := &blockingProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: provider,
		},
		Extensions: []agentcore.Extension{ext},
	})

	bodyCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		reqBody, err := json.Marshal(ChatRequest{
			Message:  "hello",
			Stream:   true,
			ThreadID: "thread-http-transport",
		})
		if err != nil {
			errCh <- err
			return
		}
		req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(reqBody))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			errCh <- fmt.Errorf("status = %d body = %s", rec.Code, rec.Body.String())
			return
		}
		bodyCh <- rec.Body.String()
	}()

	select {
	case <-firstGETStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first server-stream GET")
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for provider to start")
	}

	close(releaseFirstGET)
	// 等待第二个 GET 写入无效 SSE 数据（触发 transport error），
	// 然后短时间 yield 让 runServerStream goroutine 完成 SSE 错误处理。
	select {
	case <-secondGETWritten:
		// 等待 SSE 错误处理完成（in-memory 处理通常 < 1ms）
		tmr := time.NewTimer(10 * time.Millisecond)
		<-tmr.C
		tmr.Stop()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second GET to write data")
	}
	close(provider.release)

	select {
	case err := <-errCh:
		t.Fatal(err)
	case body := <-bodyCh:
		events := parseSSEEvents(t, body)

		var transportData json.RawMessage
		for _, ev := range events {
			if ev.Event == "mcp_transport_error" {
				transportData = ev.Data
				break
			}
		}
		if len(transportData) == 0 {
			t.Fatalf("missing transport error event: %#v", events)
		}

		var transportEvent MCPStreamTransportErrorEvent
		if err := json.Unmarshal(transportData, &transportEvent); err != nil {
			t.Fatalf("decode transport error event: %v", err)
		}
		if transportEvent.Schema != streamSchemaMCPTransportError || transportEvent.ThreadID != "thread-http-transport" || transportEvent.Transport != "http" {
			t.Fatalf("transport error event = %#v", transportEvent)
		}
		if transportEvent.Operation != "server_stream" || transportEvent.Reason != mcp.ReconnectReasonServerStreamEOF {
			t.Fatalf("unexpected transport error payload = %#v", transportEvent)
		}
		if !strings.Contains(transportEvent.Message, "invalid server sse payload") {
			t.Fatalf("transport error message = %#v", transportEvent)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for chat stream body")
	}
}

type unknownStreamEvent struct{}

func (unknownStreamEvent) EventKind() agentcore.EventType { return "unknown_event" }
func (unknownStreamEvent) EventTime() time.Time           { return time.Unix(123, 0).UTC() }

func TestStreamEventPayloadFallsBackForUnknownEvent(t *testing.T) {
	payload := streamEventPayload("thread-unknown", unknownStreamEvent{})

	ev, ok := payload.(AgentStreamEvent)
	if !ok {
		t.Fatalf("payload type = %T", payload)
	}
	if ev.Schema != streamSchemaAgentEvent || ev.Type != "unknown_event" || ev.ThreadID != "thread-unknown" {
		t.Fatalf("unknown event envelope = %#v", ev)
	}
	if _, ok := ev.Payload.(unknownStreamEvent); !ok {
		t.Fatalf("unknown payload type = %T", ev.Payload)
	}
}
