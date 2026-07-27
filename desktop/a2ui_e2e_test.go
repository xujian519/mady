package main

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
)

// ── A2UI 事件链端到端测试 ──────────────────────────
//
// 验证 agentcore.A2UIEvent → agui.Converter.Convert
// → mapAguiEventToWailsName → envelope 荷载保存
// 的完整链路。
//
// 四种 A2UI Envelope 类型全部覆盖：
//   - createSurface
//   - updateComponents
//   - updateDataModel
//   - deleteSurface

func TestA2UIE2E_createSurface(t *testing.T) {
	c := agui.NewConverter("th-e2e", "run-e2e")
	payload := map[string]any{
		"version": "v0.9.1",
		"createSurface": map[string]any{
			"surfaceId":     "demo",
			"catalogId":     "https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json",
			"theme":         map[string]any{"primaryColor": "#5856D6"},
			"sendDataModel": true,
		},
	}

	ev := agentcore.NewA2UIEvent(payload)
	events := c.Convert(ev)

	if len(events) < 1 {
		t.Fatal("expected at least 1 AGUI event from A2UIEvent")
	}

	ce, ok := events[0].(agui.CustomEvent)
	if !ok {
		t.Fatalf("event type = %T, want agui.CustomEvent", events[0])
	}

	if ce.Name != "a2ui" {
		t.Errorf("CustomEvent.Name = %q, want %q", ce.Name, "a2ui")
	}

	// 验证 Wails 事件名
	wailsName := mapAguiEventToWailsName(ce)
	if wailsName != "agui:a2ui" {
		t.Errorf("Wails event name = %q, want %q", wailsName, "agui:a2ui")
	}

	// 验证面荷载
	val, ok := ce.Value.(map[string]any)
	if !ok {
		t.Fatalf("CustomEvent.Value type = %T, want map[string]any", ce.Value)
	}

	cs := val["createSurface"].(map[string]any)
	if cs == nil {
		t.Fatal("createSurface missing from envelope")
	}
	if cs["surfaceId"] != "demo" {
		t.Errorf("surfaceId = %v, want demo", cs["surfaceId"])
	}
	theme := cs["theme"].(map[string]any)
	if theme["primaryColor"] != "#5856D6" {
		t.Errorf("primaryColor = %v, want #5856D6", theme["primaryColor"])
	}
}

func TestA2UIE2E_updateComponents(t *testing.T) {
	c := agui.NewConverter("th-e2e", "run-e2e")
	payload := map[string]any{
		"version": "v0.9.1",
		"updateComponents": map[string]any{
			"surfaceId": "demo",
			"components": []map[string]any{
				{
					"id":        "root",
					"component": "Column",
					"children":  []string{"hello"},
				},
				{
					"id":        "hello",
					"component": "Text",
					"content":   "Hello A2UI",
				},
			},
		},
	}

	ev := agentcore.NewA2UIEvent(payload)
	events := c.Convert(ev)

	if len(events) < 1 {
		t.Fatal("expected at least 1 AGUI event")
	}

	ce := events[0].(agui.CustomEvent)
	if ce.Name != "a2ui" {
		t.Errorf("Name = %q, want a2ui", ce.Name)
	}

	val := ce.Value.(map[string]any)
	uc := val["updateComponents"].(map[string]any)
	if uc["surfaceId"] != "demo" {
		t.Errorf("surfaceId = %v, want demo", uc["surfaceId"])
	}

	// 验证组件存在
	comps, ok := uc["components"]
	if !ok || comps == nil {
		t.Fatal("components missing from updateComponents")
	}
}

func TestA2UIE2E_updateDataModel(t *testing.T) {
	c := agui.NewConverter("th-e2e", "run-e2e")
	payload := map[string]any{
		"version": "v0.9.1",
		"updateDataModel": map[string]any{
			"surfaceId": "demo",
			"path":      "/user/name",
			"value":     "Alice",
		},
	}

	ev := agentcore.NewA2UIEvent(payload)
	events := c.Convert(ev)
	if len(events) < 1 {
		t.Fatal("expected at least 1 AGUI event")
	}

	ce := events[0].(agui.CustomEvent)
	val := ce.Value.(map[string]any)
	ud := val["updateDataModel"].(map[string]any)

	if ud["path"] != "/user/name" {
		t.Errorf("path = %v, want /user/name", ud["path"])
	}
	if ud["value"] != "Alice" {
		t.Errorf("value = %v, want Alice", ud["value"])
	}
}

func TestA2UIE2E_deleteSurface(t *testing.T) {
	c := agui.NewConverter("th-e2e", "run-e2e")
	payload := map[string]any{
		"version": "v0.9.1",
		"deleteSurface": map[string]any{
			"surfaceId": "demo",
		},
	}

	ev := agentcore.NewA2UIEvent(payload)
	events := c.Convert(ev)
	if len(events) < 1 {
		t.Fatal("expected at least 1 AGUI event")
	}

	ce := events[0].(agui.CustomEvent)
	val := ce.Value.(map[string]any)
	ds := val["deleteSurface"].(map[string]any)
	if ds["surfaceId"] != "demo" {
		t.Errorf("surfaceId = %v, want demo", ds["surfaceId"])
	}

	wailsName := mapAguiEventToWailsName(ce)
	if wailsName != "agui:a2ui" {
		t.Errorf("Wails event name = %q, want agui:a2ui", wailsName)
	}
}

func TestA2UIE2E_componentSequence(t *testing.T) {
	// 模拟 A2UI surface 的完整生命周期：
	// createSurface → updateComponents (root + text) → updateDataModel → deleteSurface
	c := agui.NewConverter("th-e2e", "run-e2e")

	// 步骤 1: createSurface
	createPayload := map[string]any{
		"version": "v0.9.1",
		"createSurface": map[string]any{
			"surfaceId": "demo",
			"catalogId": "https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json",
		},
	}
	createEvents := c.Convert(agentcore.NewA2UIEvent(createPayload))
	if len(createEvents) != 1 {
		t.Fatalf("createSurface: expected 1 event, got %d", len(createEvents))
	}

	// 步骤 2: updateComponents
	compPayload := map[string]any{
		"version": "v0.9.1",
		"updateComponents": map[string]any{
			"surfaceId": "demo",
			"components": []any{
				map[string]any{"id": "root", "component": "Column", "children": []string{"txt1"}},
				map[string]any{"id": "txt1", "component": "Text", "content": "Hello"},
			},
		},
	}
	compEvents := c.Convert(agentcore.NewA2UIEvent(compPayload))
	if len(compEvents) != 1 {
		t.Fatalf("updateComponents: expected 1 event, got %d", len(compEvents))
	}

	// 步骤 3: updateDataModel
	dmPayload := map[string]any{
		"version": "v0.9.1",
		"updateDataModel": map[string]any{
			"surfaceId": "demo",
			"path":      "/user/name",
			"value":     "Bob",
		},
	}
	dmEvents := c.Convert(agentcore.NewA2UIEvent(dmPayload))
	if len(dmEvents) != 1 {
		t.Fatalf("updateDataModel: expected 1 event, got %d", len(dmEvents))
	}
	dmCe := dmEvents[0].(agui.CustomEvent)
	dmVal := dmCe.Value.(map[string]any)
	dmBody := dmVal["updateDataModel"].(map[string]any)
	if dmBody["path"] != "/user/name" {
		t.Errorf("updateDataModel path = %v, want /user/name", dmBody["path"])
	}
	if dmBody["value"] != "Bob" {
		t.Errorf("updateDataModel value = %v, want Bob", dmBody["value"])
	}

	// 步骤 4: deleteSurface
	delPayload := map[string]any{
		"version": "v0.9.1",
		"deleteSurface": map[string]any{
			"surfaceId": "demo",
		},
	}
	delEvents := c.Convert(agentcore.NewA2UIEvent(delPayload))
	if len(delEvents) != 1 {
		t.Fatalf("deleteSurface: expected 1 event, got %d", len(delEvents))
	}
	delCe := delEvents[0].(agui.CustomEvent)
	delVal := delCe.Value.(map[string]any)
	delBody := delVal["deleteSurface"].(map[string]any)
	if delBody["surfaceId"] != "demo" {
		t.Errorf("deleteSurface surfaceId = %v, want demo", delBody["surfaceId"])
	}

	// 验证所有事件都映射到 agui:a2ui
	allEvents := append(createEvents, compEvents...)
	allEvents = append(allEvents, dmEvents...)
	allEvents = append(allEvents, delEvents...)
	for _, ev := range allEvents {
		ce := ev.(agui.CustomEvent)
		if ce.Name != "a2ui" {
			t.Errorf("expected a2ui event, got %q", ce.Name)
		}
		wn := mapAguiEventToWailsName(ce)
		if wn != "agui:a2ui" {
			t.Errorf("Wails name = %q, want agui:a2ui", wn)
		}
	}
}

func TestA2UIE2E_envelopeContentPreserved(t *testing.T) {
	// 验证 agentcore.A2UIEvent 的原始 envelope 内容在经过
	// agui.Converter 后是否无损传递到 CustomEvent.Value 中。
	envelope := map[string]any{
		"version": "v0.9.1",
		"createSurface": map[string]any{
			"surfaceId":     "test-surface",
			"catalogId":     "https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json",
			"sendDataModel": true,
			"theme": map[string]any{
				"primaryColor":     "#5856D6",
				"agentDisplayName": "Patent Assistant",
			},
		},
	}

	c := agui.NewConverter("th-preserve", "run-preserve")
	events := c.Convert(agentcore.NewA2UIEvent(envelope))
	if len(events) == 0 {
		t.Fatal("expected events")
	}

	ce := events[0].(agui.CustomEvent)
	val, ok := ce.Value.(map[string]any)
	if !ok {
		t.Fatalf("Value type = %T", ce.Value)
	}

	cs, ok := val["createSurface"].(map[string]any)
	if !ok {
		t.Fatal("createSurface missing from value")
	}
	if cs["surfaceId"] != "test-surface" {
		t.Errorf("surfaceId = %v, want test-surface", cs["surfaceId"])
	}

	theme, ok := cs["theme"].(map[string]any)
	if !ok {
		t.Fatal("theme missing from createSurface")
	}
	if theme["primaryColor"] != "#5856D6" {
		t.Errorf("primaryColor = %v, want #5856D6", theme["primaryColor"])
	}
	if theme["agentDisplayName"] != "Patent Assistant" {
		t.Errorf("agentDisplayName = %v, want Patent Assistant", theme["agentDisplayName"])
	}
}
