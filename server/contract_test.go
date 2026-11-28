package server

// 本文件是协议面契约护栏（M67-2）。契约清单见
// docs/code-health/protocol-contract-baseline.md §3.5。
//
// Server 层对外暴露的是 HTTP 路由表、/api/v1 版本化响应头、SSE 帧格式与
// 流事件 schema 名。桌面端与第三方前端直接消费这些字符串，改动须走协议
// 变更流程，不能在重构卡里顺手改。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// TestContractServerStreamSchemas 锁定 SSE 流载荷的 schema 名。
func TestContractServerStreamSchemas(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{streamSchemaAgentEvent, "agent.event.v1"},
		{streamSchemaMCPAbilitiesUpdated, "mcp.capabilities_updated.v1"},
		{streamSchemaMCPToolsRefreshed, "mcp.tools_refreshed.v1"},
		{streamSchemaMCPTransportError, "mcp.transport_error.v1"},
		{streamSchemaMCPReconnect, "mcp.reconnect.v1"},
		{streamSchemaMCPRefresh, "mcp.refresh.v1"},
		{streamSchemaSkillsSnapshot, "skills.snapshot.v1"},
		{streamSchemaChatDone, "chat.done.v1"},
	}
	if len(cases) != 8 {
		t.Fatalf("SSE 流 schema 应有 8 个，当前 fixture 为 %d", len(cases))
	}
	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("schema 漂移：当前 %q，契约要求 %q", c.got, c.want)
		}
		if seen[c.want] {
			t.Errorf("schema 重复：%q", c.want)
		}
		seen[c.want] = true
	}
}

// TestContractServerAPIRouteVersioning 锁定 /api/* 与 /api/v1/* 双注册及响应头。
func TestContractServerAPIRouteVersioning(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	registerAPIRoute(mux, "GET /api/probe", okHandler)

	t.Run("v1 路径附版本头且无废弃头", func(t *testing.T) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 = %d，期望 200（/api/v1/* 应镜像注册）", w.Code)
		}
		if got := w.Header().Get("X-API-Version"); got != "v1" {
			t.Fatalf("X-API-Version = %q，契约要求 v1", got)
		}
		if got := w.Header().Get("X-API-Deprecated"); got != "" {
			t.Fatalf("v1 路径不应带 X-API-Deprecated，实际 %q", got)
		}
	})

	t.Run("旧路径附废弃头", func(t *testing.T) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/probe", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 = %d，期望 200", w.Code)
		}
		if got := w.Header().Get("X-API-Deprecated"); got != "true" {
			t.Fatalf("X-API-Deprecated = %q，契约要求 true", got)
		}
		if got := w.Header().Get("X-API-Deprecated-Date"); got == "" {
			t.Fatal("X-API-Deprecated-Date 不应为空")
		}
		if got := w.Header().Get("X-API-Version"); got != "" {
			t.Fatalf("旧路径不应带 X-API-Version，实际 %q", got)
		}
	})

	t.Run("非 /api/ 前缀不做版本镜像", func(t *testing.T) {
		other := http.NewServeMux()
		registerAPIRoute(other, "GET /v1/probe", okHandler)

		w := httptest.NewRecorder()
		other.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/probe", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 = %d，期望 200", w.Code)
		}
		if got := w.Header().Get("X-API-Version"); got != "" {
			t.Fatalf("非 /api/ 路径不应加版本头，实际 %q", got)
		}

		w = httptest.NewRecorder()
		other.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("状态码 = %d，非 /api/ 路径不应生成 /api/v1 镜像", w.Code)
		}
	})
}

// TestContractServerRouteTable 锁定路由表：表内路径均已注册（错误方法应得 405），
// 表外路径为 404。
func TestContractServerRouteTable(t *testing.T) {
	handler := New(agentcore.Config{}).Handler()

	paths := []string{
		"/health",
		"/ready",
		"/api/chat", "/api/v1/chat",
		"/api/skills", "/api/v1/skills",
		"/api/skills/diagnostics", "/api/v1/skills/diagnostics",
		"/api/skills/events", "/api/v1/skills/events",
		"/api/skills/status", "/api/v1/skills/status",
		"/api/skills/reload", "/api/v1/skills/reload",
		"/api/threads", "/api/v1/threads",
		"/api/threads/t1", "/api/v1/threads/t1",
		"/api/threads/t1/config", "/api/v1/threads/t1/config",
		"/api/threads/t1/thinking", "/api/v1/threads/t1/thinking",
		"/api/threads/t1/branch", "/api/v1/threads/t1/branch",
		"/api/states", "/api/v1/states",
		"/api/states/t1", "/api/v1/states/t1",
		"/v1/disclosure/analyze",
		"/v1/disclosure/analyze/t1",
		"/v1/disclosure/analyze/t1/stream",
		"/v1/disclosure/analyze/t1/review",
		"/v1/evidence/judge",
		"/v1/evidence/judge/batch",
		"/v1/evidence/judge/t1",
		"/v1/evidence/burden/scenario",
		"/v1/evidence/standard/standard",
		"/v1/evidence/conflict",
		"/agui/chat",
	}

	for _, path := range paths {
		// PATCH 未被任何路由登记，命中已注册路径时 ServeMux 返回 405。
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, path, nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("路径 %s 未注册：PATCH 期望 405，实际 %d", path, w.Code)
		}
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/unknown-probe", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("未注册路径应返回 404，实际 %d", w.Code)
	}
}

// TestContractServerChatSSEFrames 锁定 /api/chat 流式响应的帧格式：
// 每帧 = event 行 + data 行 + 空行，末帧事件名为 done。
func TestContractServerChatSSEFrames(t *testing.T) {
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
	})
	body := postChatStreamRaw(t, srv.Handler(), ChatRequest{
		Message:  "hi",
		Stream:   true,
		ThreadID: "thread-contract",
	})

	frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
	if len(frames) < 2 {
		t.Fatalf("流式响应帧数过少：%q", body)
	}
	lastEvent := ""
	for i, frame := range frames {
		lines := strings.Split(frame, "\n")
		if len(lines) == 1 && strings.HasPrefix(lines[0], ": ") {
			continue // 保活注释帧
		}
		if len(lines) != 2 {
			t.Fatalf("第 %d 帧应为 event+data 两行，实际 %q", i, frame)
		}
		if !strings.HasPrefix(lines[0], "event: ") {
			t.Fatalf("第 %d 帧首行不是 event 行：%q", i, lines[0])
		}
		if !strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("第 %d 帧次行不是 data 行：%q", i, lines[1])
		}
		if !json.Valid([]byte(strings.TrimPrefix(lines[1], "data: "))) {
			t.Fatalf("第 %d 帧 data 不是合法 JSON：%q", i, lines[1])
		}
		lastEvent = strings.TrimPrefix(lines[0], "event: ")
	}
	if lastEvent != "done" {
		t.Fatalf("末帧事件名 = %q，契约要求 done", lastEvent)
	}
	if !strings.Contains(body, `"schema":"chat.done.v1"`) {
		t.Fatalf("done 帧缺少 chat.done.v1 schema：%s", body)
	}
}
