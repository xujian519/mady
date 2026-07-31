package a2ui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/a2a"
	"github.com/xujian519/mady/agui"
)

func TestBindingA2AErrorPaths(t *testing.T) {
	// EnvelopeToDataPart with an unserializable value.

	// DataPartToEnvelope with nil data.
	_, ok, err := DataPartToEnvelope(a2a.Part{Type: a2a.PartTypeData})
	if ok || err != nil {
		t.Fatalf("expected ok=false, err=nil for nil data part, got ok=%v err=%v", ok, err)
	}

	// DataPartToEnvelope with wrong MIME type.
	_, ok, err = DataPartToEnvelope(a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: "text/plain", Data: map[string]any{"key": "val"}},
	})
	if ok || err != nil {
		t.Fatalf("expected ok=false for wrong mime type, got ok=%v", ok)
	}

	// DataPartToEnvelope with unmarshalable data (channel in data).
	_, _, err = DataPartToEnvelope(a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: MIMEType, Data: map[string]any{"ch": make(chan int)}},
	})
	if err == nil {
		t.Fatal("expected error from unmarshalable data")
	}

	// DataPartToEnvelope with body-level parse error (no body, no version)
	// but explicit A2UI MIME type — should report ErrInvalidA2UIEnvelope.
	_, ok, err = DataPartToEnvelope(a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: MIMEType, Data: map[string]any{"invalid": "data"}},
	})
	if ok {
		t.Fatalf("expected ok=false for invalid A2UI envelope, got ok=true")
	}
	if err == nil || !errors.Is(err, ErrInvalidA2UIEnvelope) {
		t.Fatalf("expected ErrInvalidA2UIEnvelope, got %v", err)
	}

	// EnvelopesToMessage error (unserializable value in component).
	envs := []Envelope{{
		Version:       Version,
		DeleteSurface: &DeleteSurface{SurfaceID: "s"},
	}}
	msg, err := EnvelopesToMessage("agent", envs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := MessageEnvelopes(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(got))
	}

	// MessageEnvelopes with a malformed part (data that errors on marshal).
	msg.Parts = append(msg.Parts, a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: MIMEType, Data: map[string]any{"ch": make(chan int)}},
	})
	if _, err := MessageEnvelopes(msg); err == nil {
		t.Fatal("expected error from malformed part")
	}
}

func TestBindingA2AEnvelopeToMap(t *testing.T) {
	// This exercises the internal envelopeToMap function indirectly.
	// Non-versioned envelope triggers the version-set branch in EnvelopeToDataPart.
	env := Envelope{
		DeleteSurface: &DeleteSurface{SurfaceID: "s"},
	}
	part, err := EnvelopeToDataPart(env)
	if err != nil {
		t.Fatal(err)
	}
	if part.Data == nil {
		t.Fatal("data part missing")
	}
	if part.Data.Data["version"] != Version {
		t.Fatalf("version not set: %v", part.Data.Data["version"])
	}
}

func TestAGUIBindingToCustomEventVersionFill(t *testing.T) {
	env := Envelope{DeleteSurface: &DeleteSurface{SurfaceID: "s"}}
	ev := ToCustomEvent(env)
	raw, _ := json.Marshal(ev.Value)
	if !strings.Contains(string(raw), `"version":"v0.9.1"`) {
		t.Fatal("version not set by ToCustomEvent")
	}
}

func TestAGUIBindingFromCustomEventErrors(t *testing.T) {
	// Unmarshalable value (channel).
	ev := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      AGUIEventName,
		Value:     map[string]any{"ch": make(chan int)},
	}
	if _, ok, err := FromCustomEvent(ev); err == nil || ok {
		t.Fatalf("expected error from unmarshalable value, got ok=%v err=%v", ok, err)
	}

	// Parse error (valid JSON but not an envelope).
	ev2 := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      AGUIEventName,
		Value:     map[string]any{"invalid": "data"},
	}
	if _, ok, err := FromCustomEvent(ev2); ok || err != nil {
		t.Fatalf("expected ok=false, err=nil for non-A2UI, got ok=%v err=%v", ok, err)
	}
}
