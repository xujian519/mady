package mcp

// 本文件是协议面契约护栏（M67-2）。契约清单见
// docs/code-health/protocol-contract-baseline.md §3.6。
//
// MCP 的 RPC 方法名是与外部 MCP 服务端互操作的线协议；事件名经 Server SSE
// 透出给桌面端。改动须走协议变更流程，不能在重构卡里顺手改。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
)

// TestContractMCPRPCMethods 锁定客户端实际发出的 11 个 RPC 方法名。
// 用假 HTTP MCP 服务端记录收到的 method 字段，再驱动全部客户端 API。
func TestContractMCPRPCMethods(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]bool)

	recorder := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解码 MCP 请求失败：%v", err)
			return
		}
		method, _ := req["method"].(string)
		mu.Lock()
		seen[method] = true
		mu.Unlock()

		respond := func(result any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req["id"],
				"result":  result,
			})
		}

		switch method {
		case "initialize":
			w.Header().Set(headerSessionID, "sess-contract")
			respond(map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities": map[string]any{
					"tools":     map[string]any{"listChanged": true},
					"resources": map[string]any{"subscribe": true, "listChanged": true},
					"prompts":   map[string]any{"listChanged": true},
				},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			respond(map[string]any{"tools": []any{}})
		case "tools/call":
			respond(map[string]any{"content": []any{}})
		case "resources/list":
			respond(map[string]any{"resources": []any{}})
		case "resources/read":
			respond(map[string]any{"contents": []any{}})
		case "resources/templates/list":
			respond(map[string]any{"resourceTemplates": []any{}})
		case "resources/subscribe", "resources/unsubscribe":
			respond(map[string]any{})
		case "prompts/list":
			respond(map[string]any{"prompts": []any{}})
		case "prompts/get":
			respond(map[string]any{"messages": []any{}})
		default:
			t.Errorf("客户端发送了未登记的 MCP 方法：%q", method)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer recorder.Close()

	client, err := NewHTTPClient(context.Background(), HTTPConfig{
		Endpoint:      recorder.URL,
		ClientName:    "mady-contract",
		ClientVersion: "0.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	if _, err := client.ListTools(ctx); err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if _, err := client.CallTool(ctx, "echo", map[string]any{}); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if _, err := client.ListResources(ctx); err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if _, err := client.ReadResource(ctx, "file:///tmp/a.txt"); err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if _, err := client.ListResourceTemplates(ctx); err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if _, err := client.ListPrompts(ctx); err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if _, err := client.GetPrompt(ctx, "review", nil); err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if err := client.SubscribeResource(ctx, "file:///tmp/a.txt"); err != nil {
		t.Fatalf("SubscribeResource: %v", err)
	}
	if err := client.UnsubscribeResource(ctx, "file:///tmp/a.txt"); err != nil {
		t.Fatalf("UnsubscribeResource: %v", err)
	}

	want := []string{
		"initialize",
		"notifications/initialized",
		"tools/list",
		"tools/call",
		"resources/list",
		"resources/read",
		"resources/templates/list",
		"resources/subscribe",
		"resources/unsubscribe",
		"prompts/list",
		"prompts/get",
	}
	if len(want) != 11 {
		t.Fatalf("MCP 客户端应有 11 个 RPC 方法，当前 fixture 为 %d", len(want))
	}

	mu.Lock()
	got := make([]string, 0, len(seen))
	for method := range seen {
		got = append(got, method)
	}
	mu.Unlock()
	sort.Strings(got)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)

	if len(got) != len(sortedWant) {
		t.Fatalf("RPC 方法数量漂移：\n当前 %v\n契约 %v", got, sortedWant)
	}
	for i := range sortedWant {
		if got[i] != sortedWant[i] {
			t.Fatalf("RPC 方法漂移：\n当前 %v\n契约 %v", got, sortedWant)
		}
	}
}

// TestContractMCPEventNames 锁定经 Server SSE 透出的 5 个 MCP 事件名。
func TestContractMCPEventNames(t *testing.T) {
	cases := []struct {
		got  string
		want string
	}{
		{string(EventMCPCapabilitiesUpdated), "mcp_capabilities_updated"},
		{string(EventMCPToolsRefreshed), "mcp_tools_refreshed"},
		{string(EventMCPTransportError), "mcp_transport_error"},
		{string(EventMCPReconnect), "mcp_reconnect"},
		{string(EventMCPRefresh), "mcp_refresh"},
	}
	if len(cases) != 5 {
		t.Fatalf("MCP 事件应有 5 个，当前 fixture 为 %d", len(cases))
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("事件名漂移：当前 %q，契约要求 %q", c.got, c.want)
		}
	}
}
