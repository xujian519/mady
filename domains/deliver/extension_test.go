package deliver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// 登记完成后必须发 deliver/registered 事件（会话日志靠它回放交付结论），
// 且调用方传入的 OnRegistered 不能被包装层吞掉。
func TestExtensionEmitsRegisteredEvent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.docx")
	writeFile(t, file, "v1")

	agent := agentcore.New(agentcore.Config{})
	defer agent.Close()

	called := 0
	ext := NewExtension(Config{OnRegistered: func(Deliverable) { called++ }})
	if err := ext.Init(context.Background(), agent); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got := make(chan *agentcore.DeliverRegisteredEvent, 1)
	agent.EventBus().On(agentcore.EventDeliverRegistered, func(e agentcore.Event) {
		if ev, ok := e.(*agentcore.DeliverRegisteredEvent); ok {
			got <- ev
		}
	})

	args, err := json.Marshal(map[string]any{
		"template_id": "claims-spec",
		"artifacts": []map[string]string{{
			"role": "a", "canonical_path": file, "delivered_path": file,
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := ext.Tools()[0].Func(context.Background(), args); err != nil {
		t.Fatalf("tool: %v", err)
	}

	select {
	case ev := <-got:
		if ev.Verdict != VerdictPass || ev.TemplateID != "claims-spec" || ev.Files != 2 {
			t.Fatalf("event = %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("未收到 deliver/registered 事件")
	}
	if called != 1 {
		t.Fatalf("OnRegistered 调用 %d 次", called)
	}
}
