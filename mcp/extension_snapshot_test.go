package mcp

// M69 补测：传输扩展的健康快照面（Name / SnapshotEvents）与 5 个 MCP 事件的
// EventKind/EventTime 映射此前为 0% 覆盖。Server 层把 SnapshotEvents 的结果
// 直接转成 SSE 的 mcp_capabilities_updated 帧，Transport 字段是前端区分
// stdio/http 的依据，故用断言锁定。

import (
	"errors"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

func TestSessionExpiredError(t *testing.T) {
	err := sessionExpiredError{sessionID: "sess-1"}
	if got := err.Error(); got != "mcp session expired" {
		t.Fatalf("Error() = %q，期望 mcp session expired", got)
	}
	if !errors.Is(err, errSessionExpired) {
		t.Fatal("sessionExpiredError 应可通过 errors.Is 命中 errSessionExpired")
	}
}

func TestExtensionSnapshotEvents(t *testing.T) {
	t.Run("HTTP 扩展", func(t *testing.T) {
		ext := &HTTPExtension{name: "mcp-http"}
		if ext.Name() != "mcp-http" {
			t.Fatalf("Name = %q", ext.Name())
		}
		if snap := ext.SnapshotEvents(); snap != nil {
			t.Fatalf("client 为 nil 时应返回 nil，实际 %#v", snap)
		}

		ext = &HTTPExtension{name: "mcp-http", client: &HTTPClient{}}
		snap := ext.SnapshotEvents()
		if len(snap) != 1 {
			t.Fatalf("快照事件数 = %d，期望 1", len(snap))
		}
		ev, ok := snap[0].(CapabilitiesUpdatedEvent)
		if !ok {
			t.Fatalf("事件类型 = %T，期望 CapabilitiesUpdatedEvent", snap[0])
		}
		if ev.Transport != "http" || ev.Extension != "mcp-http" {
			t.Fatalf("快照字段 = %#v", ev)
		}
		if ev.At.IsZero() {
			t.Fatal("快照时间戳不应为零值")
		}
		if ev.EventKind() != agentcore.EventType(EventMCPCapabilitiesUpdated) {
			t.Fatalf("EventKind = %q", ev.EventKind())
		}
	})

	t.Run("Stdio 扩展", func(t *testing.T) {
		ext := &StdioExtension{name: "mcp-stdio"}
		if ext.Name() != "mcp-stdio" {
			t.Fatalf("Name = %q", ext.Name())
		}
		if snap := ext.SnapshotEvents(); snap != nil {
			t.Fatalf("client 为 nil 时应返回 nil，实际 %#v", snap)
		}

		ext = &StdioExtension{name: "mcp-stdio", client: &Client{}}
		snap := ext.SnapshotEvents()
		if len(snap) != 1 {
			t.Fatalf("快照事件数 = %d，期望 1", len(snap))
		}
		ev, ok := snap[0].(CapabilitiesUpdatedEvent)
		if !ok {
			t.Fatalf("事件类型 = %T，期望 CapabilitiesUpdatedEvent", snap[0])
		}
		if ev.Transport != "stdio" {
			t.Fatalf("Transport = %q，契约要求 stdio", ev.Transport)
		}
	})
}

func TestMCPEventTimeAndKind(t *testing.T) {
	at := time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		event agentcore.Event
		want  agentcore.EventType
	}{
		{"能力快照", CapabilitiesUpdatedEvent{At: at}, agentcore.EventType(EventMCPCapabilitiesUpdated)},
		{"工具刷新", ToolsRefreshedEvent{At: at}, agentcore.EventType(EventMCPToolsRefreshed)},
		{"传输错误", TransportErrorEvent{At: at}, agentcore.EventType(EventMCPTransportError)},
		{"重连", ReconnectEvent{At: at}, agentcore.EventType(EventMCPReconnect)},
		{"刷新", RefreshEvent{At: at}, agentcore.EventType(EventMCPRefresh)},
	}
	if len(cases) != 5 {
		t.Fatalf("MCP 事件应有 5 个，当前 fixture 为 %d", len(cases))
	}
	for _, c := range cases {
		if got := c.event.EventTime(); !got.Equal(at) {
			t.Errorf("%s：EventTime = %v，期望透传 %v", c.name, got, at)
		}
		if got := c.event.EventKind(); got != c.want {
			t.Errorf("%s：EventKind = %q，期望 %q", c.name, got, c.want)
		}
	}
}
