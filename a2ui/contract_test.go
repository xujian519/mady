package a2ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// 本文件是协议面契约护栏（M67-1）。契约清单见
// docs/code-health/protocol-contract-baseline.md §3.2。
//
// A2UI 是跨端声明式 UI 协议：版本号、目录 ID、四个信封字段名与 JSONL 分帧
// 都是客户端解析入口，改动属破坏性变更，须走 M70 的契约变更流程。

// TestContractA2UIVersionAndCatalog 锁定协议版本与基础组件目录 ID。
func TestContractA2UIVersionAndCatalog(t *testing.T) {
	if Version != "v0.9.1" {
		t.Errorf("A2UI 版本漂移：%q", Version)
	}
	const wantCatalog = "https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json"
	if BasicCatalogID != wantCatalog {
		t.Errorf("基础目录 ID 漂移\n期望：%s\n实际：%s", wantCatalog, BasicCatalogID)
	}
}

// TestContractA2UIEnvelopeJSON 锁定四个信封的字段名与序列化形态。
func TestContractA2UIEnvelopeJSON(t *testing.T) {
	cases := []struct {
		name string
		env  Envelope
		want string
	}{
		{
			"createSurface",
			NewCreateSurface("s1", BasicCatalogID),
			`{"version":"v0.9.1","createSurface":{"surfaceId":"s1","catalogId":"https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json"}}`,
		},
		{
			"updateComponents",
			NewUpdateComponents("s1", NewComponent("root", "Column", nil)),
			`{"version":"v0.9.1","updateComponents":{"surfaceId":"s1","components":[{"component":"Column","id":"root"}]}}`,
		},
		{
			"updateDataModel",
			NewUpdateDataModel("s1", "/title", "hi"),
			`{"version":"v0.9.1","updateDataModel":{"path":"/title","surfaceId":"s1","value":"hi"}}`,
		},
		{
			"removeDataModel",
			NewRemoveDataModel("s1", "/title"),
			`{"version":"v0.9.1","updateDataModel":{"path":"/title","surfaceId":"s1"}}`,
		},
		{
			"deleteSurface",
			NewDeleteSurface("s1"),
			`{"version":"v0.9.1","deleteSurface":{"surfaceId":"s1"}}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := json.Marshal(c.env)
			if err != nil {
				t.Fatalf("marshal 失败：%v", err)
			}
			if string(got) != c.want {
				t.Errorf("信封 JSON 契约漂移\n期望：%s\n实际：%s", c.want, got)
			}
		})
	}
}

// TestContractA2UIJSONLFraming 锁定 JSONL 分帧：一行一个紧凑信封，行尾换行。
func TestContractA2UIJSONLFraming(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.EncodeAll([]Envelope{
		NewCreateSurface("s1", BasicCatalogID),
		NewDeleteSurface("s1"),
	}); err != nil {
		t.Fatalf("编码失败：%v", err)
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSONL 应为 2 行，实际 %d 行：%q", len(lines), buf.String())
	}
	for i, line := range lines {
		if strings.Contains(line, "\n") || line == "" {
			t.Errorf("第 %d 行不是单行紧凑 JSON：%q", i+1, line)
		}
		if _, err := ParseEnvelope([]byte(line)); err != nil {
			t.Errorf("第 %d 行无法按信封解析：%v", i+1, err)
		}
	}
}

// TestContractA2UIAguiBinding 锁定 A2UI 经 AGUI CUSTOM 通道传输的约定：
// name 固定为 "a2ui"，版本缺省时补 v0.9.1。
func TestContractA2UIAguiBinding(t *testing.T) {
	if AGUIEventName != "a2ui" {
		t.Errorf("AGUI 自定义事件名漂移：%q", AGUIEventName)
	}

	ev := ToCustomEvent(Envelope{})
	if string(ev.Type) != "CUSTOM" {
		t.Errorf("事件类型应为 CUSTOM，实际 %q", ev.Type)
	}
	if ev.Name != "a2ui" {
		t.Errorf("事件 name 应为 a2ui，实际 %q", ev.Name)
	}
	got, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal 失败：%v", err)
	}
	want := `{"type":"CUSTOM","name":"a2ui","value":{"version":"v0.9.1"}}`
	if string(got) != want {
		t.Errorf("AGUI 绑定载荷漂移\n期望：%s\n实际：%s", want, got)
	}

	// 回读：带正文的信封应可往返；非 a2ui 事件必须被明确判为非匹配。
	roundTrip := ToCustomEvent(NewCreateSurface("s1", BasicCatalogID))
	parsed, ok, err := FromCustomEvent(roundTrip)
	if err != nil || !ok {
		t.Fatalf("往返解析应成功，ok=%v err=%v", ok, err)
	}
	if parsed.CreateSurface == nil || parsed.CreateSurface.SurfaceID != "s1" {
		t.Errorf("往返后 createSurface 丢失：%+v", parsed)
	}
	other := roundTrip
	other.Name = "not-a2ui"
	if _, ok, err := FromCustomEvent(other); err != nil || ok {
		t.Errorf("非 a2ui 事件应返回 ok=false，实际 ok=%v err=%v", ok, err)
	}
}
