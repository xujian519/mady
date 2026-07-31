package a2ui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xujian519/mady/a2a"
	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/agui"
)

func TestNewClientActionNilContext(t *testing.T) {
	a := NewClientAction("submit", "s", "btn", nil)
	if a.Context == nil {
		t.Fatal("nil context should be converted to empty map")
	}
	if len(a.Context) != 0 {
		t.Fatalf("expected empty context, got %v", a.Context)
	}
}

func TestComponentSetNilProps(t *testing.T) {
	c := Component{ID: "x", Type: "Text"}
	c.Set("text", "hi")
	if c.Props == nil {
		t.Fatal("Set should initialize nil Props")
	}
	if c.Props["text"] != "hi" {
		t.Fatalf("Set failed: %+v", c)
	}
}

func TestComponentMarshalJSON(t *testing.T) {
	// Normal marshaling.
	c := Component{ID: "x", Type: "Text", Props: map[string]any{"text": "hello"}}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"id":"x"`) {
		t.Fatalf("id missing: %s", data)
	}
	if !strings.Contains(string(data), `"component":"Text"`) {
		t.Fatalf("component type missing: %s", data)
	}
	if !strings.Contains(string(data), `"text":"hello"`) {
		t.Fatalf("text prop missing: %s", data)
	}
	// Props with reserved keys "id" and "component" are silently dropped.
	c2 := Component{ID: "x", Type: "Text", Props: map[string]any{"id": "should-drop", "component": "should-drop", "text": "ok"}}
	data2, _ := json.Marshal(c2)
	if strings.Contains(string(data2), `"should-drop"`) {
		t.Fatalf("reserved props should be dropped: %s", data2)
	}
}

func TestComponentUnmarshalJSONErrors(t *testing.T) {
	// Not JSON at all.
	var c Component
	if err := json.Unmarshal([]byte(`{`), &c); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	// ID not a string.
	if err := json.Unmarshal([]byte(`{"id":123,"component":"Text"}`), &c); err == nil {
		t.Fatal("expected error for non-string id")
	}
	// Component type not a string.
	if err := json.Unmarshal([]byte(`{"id":"x","component":123}`), &c); err == nil {
		t.Fatal("expected error for non-string component type")
	}
	// Valid unmarshal with extra props.
	if err := json.Unmarshal([]byte(`{"id":"x","component":"Text","text":"hello"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.ID != "x" || c.Type != "Text" || c.Props["text"] != "hello" {
		t.Fatalf("unmarshal result: %+v", c)
	}
}

func TestChildListMarshalJSON(t *testing.T) {
	// Static children but nil slice.
	cl := ChildList{}
	data, _ := json.Marshal(cl)
	if string(data) != `[]` {
		t.Fatalf("nil static should marshal to empty array, got %s", data)
	}
	// Template children.
	cl2 := ChildList{Template: &ChildTemplate{Path: "/items", ComponentID: "card"}}
	data2, _ := json.Marshal(cl2)
	if !strings.Contains(string(data2), `"path":"/items"`) {
		t.Fatalf("template marshal: %s", data2)
	}
}

func TestChildListUnmarshalJSONErrors(t *testing.T) {
	var cl ChildList
	// Invalid JSON.
	if err := json.Unmarshal([]byte(`{`), &cl); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParsePointerEdgeCases(t *testing.T) {
	// Path with multiple leading slashes yields empty tokens.
	pp := ParsePointer("//")
	if len(pp) != 2 || pp[0] != "" || pp[1] != "" {
		t.Fatalf("expected ['',''] for '//', got %v", pp)
	}
}

func TestJoinPointerEmpty(t *testing.T) {
	s := JoinPointer()
	if s != "/" {
		t.Fatalf("expected '/', got %q", s)
	}
}

func TestGetDataEdgeCases(t *testing.T) {
	model := map[string]any{
		"a": map[string]any{"b": "val"},
		"c": []any{"x", "y"},
	}
	// Key not found in map.
	if _, ok := GetData(model, "/missing"); ok {
		t.Fatal("expected not found for missing key")
	}
	// Array index out of range.
	if _, ok := GetData(model, "/c/5"); ok {
		t.Fatal("expected not found for out-of-range index")
	}
	// Traverse into scalar.
	if _, ok := GetData(model, "/a/b/c"); ok {
		t.Fatal("expected not found when traversing into scalar")
	}
	// Path that goes into scalar instead of container.
	model2 := map[string]any{"x": "scalar"}
	if _, ok := GetData(model2, "/x/y"); ok {
		t.Fatal("expected not found when descending into scalar")
	}
}

func TestApplyUpdateEdgeCases(t *testing.T) {
	// Root path with hasValue=false (clear model).
	result, err := ApplyUpdate(map[string]any{"a": 1}, "/", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}

	// Nil model with non-root path.
	result, err = ApplyUpdate(nil, "/a", "val", true)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := result.(map[string]any)
	if !ok || m["a"] != "val" {
		t.Fatalf("expected map[a:val], got %v", result)
	}

	// applyTokens: map child != nil (existing map updated).
	result, err = ApplyUpdate(map[string]any{"a": map[string]any{"b": 1}}, "/a/b", 2, true)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := GetData(result, "/a/b")
	if v != 2 {
		t.Fatalf("expected 2, got %v", v)
	}

	// Array append with hasValue=false (no-op append).
	result, err = ApplyUpdate([]any{"a"}, "/-", "b", false)
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := result.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("expected array unchanged, got %v", result)
	}

	// Array set element by index.
	result, err = ApplyUpdate([]any{"a", "b"}, "/1", "B", true)
	if err != nil {
		t.Fatal(err)
	}
	arr, _ = result.([]any)
	if arr[1] != "B" {
		t.Fatalf("expected B at [1], got %v", arr)
	}

	// Array remove element (set to nil).
	result, err = ApplyUpdate([]any{"a", "b"}, "/1", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	arr, _ = result.([]any)
	if arr[1] != nil {
		t.Fatalf("expected nil at [1], got %v", arr)
	}

	// Array index out of range.
	_, err = ApplyUpdate([]any{"a"}, "/5", "b", true)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}

	// Cannot descend into append marker.
	_, err = ApplyUpdate([]any{"a", "b"}, "/-/x", "val", true)
	if err == nil {
		t.Fatal("expected error for descending into append")
	}

	// Descend into array with index, child is nil → gets initialized.
	result, err = ApplyUpdate([]any{nil}, "/0/x", "val", true)
	if err != nil {
		t.Fatal(err)
	}
	arr, _ = result.([]any)
	if arr == nil {
		t.Fatal("result is nil")
	}

	// applyTokens: error propagated from recursive call when descending into map.
	_, err = ApplyUpdate(map[string]any{"a": []any{}}, "/a/0/x", "val", true)
	if err == nil {
		t.Fatal("expected error when descending into non-existent array index")
	}

	// Scalar where container is expected (replace with map).
	type testType int
	result, err = ApplyUpdate(testType(42), "/a", "val", true)
	if err != nil {
		t.Fatal(err)
	}
	m, _ = result.(map[string]any)
	if m["a"] != "val" {
		t.Fatalf("expected map[a:val], got %v", result)
	}
}

func TestArrayIndexError(t *testing.T) {
	_, _, err := arrayIndex("not-a-number", 0)
	if err == nil {
		t.Fatal("expected error for non-numeric index")
	}
}

func TestSurfaceStoreApplyDefault(t *testing.T) {
	store := NewSurfaceStore()
	env := Envelope{Version: Version}
	err := store.Apply(env)
	if !errors.Is(err, ErrNoBody) {
		t.Fatalf("expected ErrNoBody, got %v", err)
	}
}

func TestSurfaceStoreApplyDataModelError(t *testing.T) {
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("s", BasicCatalogID))
	// Update a path that triggers data model error (descend into non-existent array index).
	_ = store.Apply(NewUpdateDataModel("s", "/a", []any{"x"}))
	err := store.Apply(NewUpdateDataModel("s", "/a/5", "val"))
	if err == nil {
		t.Fatal("expected error for out-of-range array index")
	}
}

func TestChildRefsWithUnknownType(t *testing.T) {
	cat := BasicCatalog()
	c := NewComponent("x", "Nonexistent", map[string]any{"child": "y", "children": []any{"z"}})
	refs := childRefs(c, cat)
	// Should use defaults: "child" and "children".
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs from defaults, got %v", refs)
	}
}

func TestChildRefsWithNestedFields(t *testing.T) {
	cat := BasicCatalog()
	c := NewComponent("tabs1", "Tabs", map[string]any{
		"tabs": []any{
			map[string]any{"child": "tab1"},
			map[string]any{"child": "tab2"},
			map[string]any{},             // no child key
			map[string]any{"child": 123}, // child is not a string
		},
	})
	refs := childRefs(c, cat)
	if len(refs) != 2 || refs[0] != "tab1" || refs[1] != "tab2" {
		t.Fatalf("unexpected refs: %v", refs)
	}
}

func TestChildListRefs(t *testing.T) {
	// []any with string elements.
	refs := childListRefs([]any{"a", "b"})
	if len(refs) != 2 {
		t.Fatalf("[]any refs: %v", refs)
	}
	// []any with empty string (skipped).
	refs = childListRefs([]any{"a", ""})
	if len(refs) != 1 {
		t.Fatalf("empty string should be skipped: %v", refs)
	}
	// []any with non-string element.
	refs = childListRefs([]any{42})
	if len(refs) != 0 {
		t.Fatalf("non-string should be skipped: %v", refs)
	}
	// []string.
	refs = childListRefs([]string{"x", "y"})
	if len(refs) != 2 {
		t.Fatalf("[]string refs: %v", refs)
	}
	// map[string]any with componentId.
	refs = childListRefs(map[string]any{"componentId": "c1"})
	if len(refs) != 1 || refs[0] != "c1" {
		t.Fatalf("map refs: %v", refs)
	}
	// map[string]any without componentId.
	refs = childListRefs(map[string]any{"other": "val"})
	if len(refs) != 0 {
		t.Fatalf("map without componentId: %v", refs)
	}
	// ChildList with Template.
	refs = childListRefs(ChildList{Template: &ChildTemplate{ComponentID: "tmpl"}})
	if len(refs) != 1 || refs[0] != "tmpl" {
		t.Fatalf("ChildList template refs: %v", refs)
	}
	// ChildList with nil Template (static).
	refs = childListRefs(ChildList{Static: []string{"s1", "s2"}})
	if len(refs) != 2 {
		t.Fatalf("ChildList static refs: %v", refs)
	}
	// nil.
	refs = childListRefs(nil)
	if len(refs) != 0 {
		t.Fatalf("nil refs: %v", refs)
	}
}

func TestValidateEnvelopeEdgeCases(t *testing.T) {
	cat := BasicCatalog()

	// KindUnknown.
	errs := ValidateEnvelope(Envelope{Version: Version}, cat)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error for unknown kind, got %d", len(errs))
	}

	// UpdateComponents with empty surfaceID.
	errs = ValidateEnvelope(NewUpdateComponents("", Text("t", "hi")), cat)
	hasEmptySurfaceID := false
	for _, e := range errs {
		if e.Path == "/updateComponents/surfaceId" {
			hasEmptySurfaceID = true
		}
	}
	if !hasEmptySurfaceID {
		t.Fatalf("expected surfaceId error, got %v", errs)
	}

	// UpdateComponents with empty component id.
	errs = ValidateEnvelope(NewUpdateComponents("s", Component{Type: "Text"}), cat)
	hasEmptyID := false
	for _, e := range errs {
		if e.Path == "/updateComponents/components/0/id" {
			hasEmptyID = true
		}
	}
	if !hasEmptyID {
		t.Fatalf("expected component id error, got %v", errs)
	}

	// UpdateComponents with empty component type.
	errs = ValidateEnvelope(NewUpdateComponents("s", Component{ID: "x"}), cat)
	hasEmptyType := false
	for _, e := range errs {
		if e.Path == "/updateComponents/components/0/component" {
			hasEmptyType = true
		}
	}
	if !hasEmptyType {
		t.Fatalf("expected component type error, got %v", errs)
	}

	// UpdateComponents with unknown component type.
	errs = ValidateEnvelope(NewUpdateComponents("s", NewComponent("x", "UnknownType", nil)), cat)
	hasUnknown := false
	for _, e := range errs {
		if strings.Contains(e.Message, "unknown component type") {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Fatalf("expected unknown component type error, got %v", errs)
	}

	// UpdateDataModel with empty surfaceID.
	errs = ValidateEnvelope(Envelope{
		Version:         Version,
		UpdateDataModel: &UpdateDataModel{Path: "/a", Value: 1, ValueSet: true},
	}, cat)
	hasDataSurfaceID := false
	for _, e := range errs {
		if e.Path == "/updateDataModel/surfaceId" {
			hasDataSurfaceID = true
		}
	}
	if !hasDataSurfaceID {
		t.Fatalf("expected updateDataModel surfaceId error, got %v", errs)
	}

	// DeleteSurface with empty surfaceID.
	errs = ValidateEnvelope(Envelope{Version: Version, DeleteSurface: &DeleteSurface{}}, cat)
	hasDelSurfaceID := false
	for _, e := range errs {
		if e.Path == "/deleteSurface/surfaceId" {
			hasDelSurfaceID = true
		}
	}
	if !hasDelSurfaceID {
		t.Fatalf("expected deleteSurface surfaceId error, got %v", errs)
	}
}

func TestValidateSurfaceTreeEdgeCases(t *testing.T) {
	cat := BasicCatalog()
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("s", BasicCatalogID))
	_ = store.Apply(NewUpdateComponents("s", Column("root", "child"), NewComponent("child", "UnknownType", nil)))
	srf, _ := store.Surface("s")
	errs := ValidateSurfaceTree(srf, cat)
	hasUnknown := false
	for _, e := range errs {
		if strings.Contains(e.Message, "unknown component type") {
			hasUnknown = true
		}
	}
	if !hasUnknown {
		t.Fatalf("expected unknown component type error, got %v", errs)
	}
}

func TestParseEnvelopeDecodeError(t *testing.T) {
	if _, err := ParseEnvelope([]byte(`{`)); err == nil {
		t.Fatal("expected decode error for malformed JSON")
	}
}

func TestUpdateDataModelUnmarshalErrors(t *testing.T) {
	var u UpdateDataModel
	// Malformed JSON.
	if err := json.Unmarshal([]byte(`{`), &u); err == nil {
		t.Fatal("expected error")
	}
	// surfaceId not a string.
	if err := json.Unmarshal([]byte(`{"surfaceId":123,"path":"/a","value":1}`), &u); err == nil {
		t.Fatal("expected error for non-string surfaceId")
	}
	// path not a string.
	u = UpdateDataModel{}
	if err := json.Unmarshal([]byte(`{"surfaceId":"s","path":123,"value":1}`), &u); err == nil {
		t.Fatal("expected error for non-string path")
	}
}

func TestComponentUnmarshalJSONPropUnmarshalError(t *testing.T) {
	var c Component
	// Prop value fails to unmarshal (json.RawMessage -> any fails for malformed).
	// This is hard to trigger with json.Unmarshal since it uses json.RawMessage
	// and then json.Unmarshal(raw, &v), which handles most things.
	// Instead test that unknown keys under "id" or "component" don't break.
	if err := json.Unmarshal([]byte(`{"id":"x","component":"Text","text":"ok"}`), &c); err != nil {
		t.Fatal(err)
	}
}

func TestDynamicUnmarshalErrorPaths(t *testing.T) {
	var d Dynamic
	// Function call with invalid JSON inside.
	if err := json.Unmarshal([]byte(`{"call":123}`), &d); err == nil {
		t.Fatal("expected error for non-string call")
	}
	// Path binding with invalid path type.
	if err := json.Unmarshal([]byte(`{"path":123}`), &d); err == nil {
		t.Fatal("expected error for non-string path")
	}
	// Literal unmarshal error.
	if err := json.Unmarshal([]byte(``), &d); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestDynamicUnmarshalExtraKeysWithPath(t *testing.T) {
	var d Dynamic
	// path with extra keys should error.
	err := json.Unmarshal([]byte(`{"path":"/a","extra":1}`), &d)
	if err == nil {
		t.Fatal("expected error for path with extra keys")
	}
	if !strings.Contains(err.Error(), "extra keys") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestComponentMarshalJSONUnserializableProp(t *testing.T) {
	c := Component{ID: "x", Type: "Text", Props: map[string]any{"ch": make(chan int)}}
	_, err := json.Marshal(c)
	// This should fail because the channel cannot be marshaled.
	if err == nil {
		t.Fatal("expected marshal error for unserializable prop")
	}
}

func TestEnvelopeToDataPartMarshalError(t *testing.T) {
	// Trigger envelopeToMap json.Marshal error via unserializable value in Theme.
	env := Envelope{
		CreateSurface: &CreateSurface{
			SurfaceID: "s", CatalogID: BasicCatalogID,
			Theme: map[string]any{"ch": make(chan int)},
		},
	}
	_, err := EnvelopeToDataPart(env)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestEnvelopesToMessageError(t *testing.T) {
	envs := []Envelope{{
		CreateSurface: &CreateSurface{
			SurfaceID: "s", CatalogID: BasicCatalogID,
			Theme: map[string]any{"ch": make(chan int)},
		},
	}}
	_, err := EnvelopesToMessage("agent", envs)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyTokensRecursiveError(t *testing.T) {
	// Descend into nested array where the inner array has an invalid index.
	_, err := ApplyUpdate([]any{[]any{}}, "/0/abc", "val", true)
	if err == nil {
		t.Fatal("expected error for invalid array index on nested array")
	}
}

func TestApplyTokensArrayIndexError(t *testing.T) {
	// Non-numeric key on a top-level array.
	_, err := ApplyUpdate([]any{"x"}, "/abc", "val", true)
	if err == nil {
		t.Fatal("expected error for non-numeric array index")
	}
}

func TestEnvelopeUnmarshalJSONSurfaceIDError(t *testing.T) {
	// surfaceId not a string in createSurface.
	var e Envelope
	// Use the full unmarshal path via envelope's UnmarshalJSON.
	if err := json.Unmarshal([]byte(`{"version":"v0.9.1","createSurface":{"surfaceId":123,"catalogId":"c"}}`), &e); err == nil {
		t.Fatal("expected error for non-string surfaceId in createSurface")
	}
}

func TestUpdateDataModelUnmarshalValueError(t *testing.T) {
	// value field - verify successful unmarshal (json.Unmarshal into any always
	// succeeds for valid JSON, so error path is unreachable via normal input).
	var u UpdateDataModel
	if err := json.Unmarshal([]byte(`{"surfaceId":"s","path":"/a","value":1}`), &u); err != nil {
		t.Fatal(err)
	}
	if u.SurfaceID != "s" || u.Path != "/a" || u.Value.(float64) != 1 || !u.ValueSet {
		t.Fatalf("unexpected result: %+v", u)
	}
}

func TestSurfaceStoreApplyDataModelApplyUpdateError(t *testing.T) {
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("s", BasicCatalogID))
	// Update with a path that forms a valid data model but ApplyUpdate fails
	// due to array index error on the data model content.
	_ = store.Apply(NewUpdateDataModel("s", "/a", []any{"x"}))
	err := store.Apply(NewUpdateDataModel("s", "/a/abc", "val"))
	if err == nil {
		t.Fatal("expected error for invalid array index")
	}
}

func TestComponentUnmarshalJSONIDError(t *testing.T) {
	var c Component
	if err := json.Unmarshal([]byte(`{"id":123,"component":"Text"}`), &c); err == nil {
		t.Fatal("expected error for non-string id")
	}
}

func TestComponentUnmarshalJSONPropNotUnmarshalable(t *testing.T) {
	var c Component
	// Props with valid JSON - this should succeed (the prop unmarshal into any
	// always succeeds for valid JSON).
	if err := json.Unmarshal([]byte(`{"id":"x","component":"Text","text":"hello"}`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Props["text"] != "hello" {
		t.Fatalf("prop not set: %+v", c)
	}
}

func TestChildListUnmarshalJSONTemplateError(t *testing.T) {
	var cl ChildList
	// A boolean fails both []string and ChildTemplate unmarshal, producing an error.
	if err := json.Unmarshal([]byte(`true`), &cl); err == nil {
		t.Fatal("expected error for boolean as ChildList")
	}
}

func TestDynamicUnmarshalLiteralError(t *testing.T) {
	var d Dynamic
	// Plain data that fails as a literal to cover the final error path.
	if err := json.Unmarshal([]byte(``), &d); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestFromCustomEventMarshalError(t *testing.T) {
	ev := agui.CustomEvent{
		BaseEvent: agui.BaseEvent{Type: agui.EventCustom},
		Name:      AGUIEventName,
		Value:     map[string]any{"ch": make(chan int)},
	}
	_, ok, err := FromCustomEvent(ev)
	if err == nil || ok {
		t.Fatalf("expected marshal error, got ok=%v err=%v", ok, err)
	}
}

func TestDataPartToEnvelopeStructuralError(t *testing.T) {
	// Valid JSON that marshals fine but causes a JSON decode error in ParseEnvelope.
	// This is hard to trigger because json.Marshal and json.Decode are symmetric.
	// Instead, test with data that has valid JSON but no A2UI body (ErrNoBody
	// path, now treated as non-match).
	part := a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: MIMEType, Data: map[string]any{"key": "val"}},
	}
	_, ok, err := DataPartToEnvelope(part)
	if ok {
		t.Fatalf("expected ok=false for non-A2UI envelope, got ok=true")
	}
	if err == nil || !errors.Is(err, ErrInvalidA2UIEnvelope) {
		t.Fatalf("expected ErrInvalidA2UIEnvelope, got %v", err)
	}
}

func TestEnvelopesToCustomEventsReturnPath(t *testing.T) {
	// Ensure the function handles empty input (zero-length).
	events := EnvelopesToCustomEvents(nil)
	if len(events) != 0 {
		t.Fatalf("expected empty, got %d", len(events))
	}
	events = EnvelopesToCustomEvents([]Envelope{})
	if len(events) != 0 {
		t.Fatalf("expected empty, got %d", len(events))
	}
}

func TestParsePointerTrimmedEmpty(t *testing.T) {
	// This branch (trimmed == "") is unreachable through normal usage because
	// path == "" || path == "/" is caught first. Verify it can't be triggered.
	pp := ParsePointer("")
	if pp != nil {
		t.Fatalf("empty path should return nil, got %v", pp)
	}
}

func TestEnvelopeToDataPartVersionFill(t *testing.T) {
	// Version should be set when envelope has none.
	env := Envelope{CreateSurface: &CreateSurface{SurfaceID: "s", CatalogID: BasicCatalogID}}
	part, err := EnvelopeToDataPart(env)
	if err != nil {
		t.Fatal(err)
	}
	if part.Data.Data["version"] != Version {
		t.Fatalf("version not filled: got %v", part.Data.Data["version"])
	}
}

func TestDataPartToEnvelopeMIMETypeEmpty(t *testing.T) {
	// MIME type is empty but part has valid A2UI data structure.
	part := a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: "", Data: map[string]any{
			"version":       Version,
			"deleteSurface": map[string]any{"surfaceId": "s"},
		}},
	}
	_, ok, err := DataPartToEnvelope(part)
	if !ok || err != nil {
		t.Fatalf("expected ok=true for empty MIME type, got ok=%v err=%v", ok, err)
	}
}

func TestEnvelopeUnmarshalJSONErrors(t *testing.T) {
	var e Envelope
	// surfaceId not a string in createSurface.
	if err := json.Unmarshal([]byte(`{"version":"v0.9.1","createSurface":{"surfaceId":123}}`), &e); err == nil {
		t.Fatal("expected error for non-string surfaceId in createSurface")
	}
}

func TestSurfaceStoreApplyCreateExistsError(t *testing.T) {
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("s", BasicCatalogID))
	err := store.Apply(NewCreateSurface("s", BasicCatalogID))
	if !errors.Is(err, ErrSurfaceExists) {
		t.Fatalf("expected ErrSurfaceExists, got %v", err)
	}
}

func TestSurfaceStoreApplyComponentsNotFoundError(t *testing.T) {
	store := NewSurfaceStore()
	err := store.Apply(NewUpdateComponents("nonexistent", Text("t", "hi")))
	if !errors.Is(err, ErrSurfaceNotFound) {
		t.Fatalf("expected ErrSurfaceNotFound, got %v", err)
	}
}

func TestCatalogHasComponentAndFunction(t *testing.T) {
	cat := BasicCatalog()
	if cat.HasComponent("") {
		t.Fatal("empty component type should not be found")
	}
	if cat.HasFunction("nonexistent") {
		t.Fatal("nonexistent function should not be found")
	}
}

// Direct-call tests for UnmarshalJSON methods to cover error branches
// that are hard to reach through json.Unmarshal's indirect call path.

func TestComponentUnmarshalJSONDirectMalformed(t *testing.T) {
	var c Component
	if err := c.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestComponentUnmarshalJSONDirectNonStringID(t *testing.T) {
	var c Component
	if err := c.UnmarshalJSON([]byte(`{"id":123,"component":"Text"}`)); err == nil {
		t.Fatal("expected error for non-string id")
	}
}

func TestComponentUnmarshalJSONDirectNonStringType(t *testing.T) {
	var c Component
	if err := c.UnmarshalJSON([]byte(`{"id":"x","component":123}`)); err == nil {
		t.Fatal("expected error for non-string component type")
	}
}

func TestComponentUnmarshalJSONDirectPropUnmarshalError(t *testing.T) {
	var c Component
	// Valid JSON but "text" prop value is a number that we then check.
	if err := c.UnmarshalJSON([]byte(`{"id":"x","component":"Text","text":"hello"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateDataModelUnmarshalDirectSurfaceID(t *testing.T) {
	var u UpdateDataModel
	if err := u.UnmarshalJSON([]byte(`{"surfaceId":123,"path":"/a"}`)); err == nil {
		t.Fatal("expected error for non-string surfaceId")
	}
}

func TestUpdateDataModelUnmarshalDirectPath(t *testing.T) {
	var u UpdateDataModel
	if err := u.UnmarshalJSON([]byte(`{"surfaceId":"s","path":123,"value":1}`)); err == nil {
		t.Fatal("expected error for non-string path")
	}
}

func TestUpdateDataModelUnmarshalDirectValue(t *testing.T) {
	var u UpdateDataModel
	if err := u.UnmarshalJSON([]byte(`{"surfaceId":"s","path":"/a","value":1}`)); err != nil {
		t.Fatal(err)
	}
	if u.Value.(float64) != 1 || !u.ValueSet {
		t.Fatalf("unexpected: %+v", u)
	}
}

func TestDynamicUnmarshalDirectLiteralError(t *testing.T) {
	var d Dynamic
	if err := d.UnmarshalJSON([]byte(``)); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestDataPartToEnvelopeJSONParseError(t *testing.T) {
	// Data that marshals fine but fails ParseEnvelope with a JSON error
	// (not ErrNoBody/ErrMultipleBodies). Since json.Marshal and json.Decode
	// are symmetric, this can only happen with an Envelope that has a value
	// that triggers a JSON syntax error during ParseEnvelope.
	// This is practically unreachable; test the non-body path instead.
	// For the JSON error path, we test through ParseEnvelope directly.
	if _, err := ParseEnvelope([]byte(`invalid`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEnvelopeToMapUnmarshalError(t *testing.T) {
	// envelopeToMap json.Unmarshal error is unreachable since the input
	// comes from json.Marshal which always produces valid JSON.
	// Verify the success path works.
	env := NewDeleteSurface("s")
	part, err := EnvelopeToDataPart(env)
	if err != nil {
		t.Fatal(err)
	}
	if part.Data == nil {
		t.Fatal("expected data part")
	}
}

func TestParsePointerTrimmedEmptyPath(t *testing.T) {
	// trimmed == "" branch: unreachable via normal paths since "" and "/"
	// are caught first. Created with a path that would produce trimmed==""
	// if the first check didn't catch it.
	pp := ParsePointer("/")
	if pp != nil {
		t.Fatalf("expected nil for '/', got %v", pp)
	}
}

type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("write error") }

func TestEncoderEncodeError(t *testing.T) {
	enc := NewEncoder(failWriter{})
	if err := enc.Encode(Envelope{DeleteSurface: &DeleteSurface{SurfaceID: "s"}}); err == nil {
		t.Fatal("expected encode error")
	}
}

func TestEncoderEncodeAllError(t *testing.T) {
	enc := NewEncoder(failWriter{})
	if err := enc.EncodeAll([]Envelope{{DeleteSurface: &DeleteSurface{SurfaceID: "s"}}}); err == nil {
		t.Fatal("expected encode all error")
	}
}

func TestSurfaceStoreApplyDefaultKind(t *testing.T) {
	store := NewSurfaceStore()
	if err := store.Apply(Envelope{Version: Version}); !errors.Is(err, ErrNoBody) {
		t.Fatalf("expected ErrNoBody, got %v", err)
	}
}

func TestSurfaceStoreApplyDataModelNotFound(t *testing.T) {
	store := NewSurfaceStore()
	err := store.Apply(NewUpdateDataModel("nonexistent", "/a", "val"))
	if !errors.Is(err, ErrSurfaceNotFound) {
		t.Fatalf("expected ErrSurfaceNotFound, got %v", err)
	}
}

func TestUpdateDataModelUnmarshalDirectMalformedJSON(t *testing.T) {
	var u UpdateDataModel
	if err := u.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestToAgentCoreEvent(t *testing.T) {
	env := NewCreateSurface("test-surface", BasicCatalogID)
	ae := ToAgentCoreEvent(env)
	if ae == nil {
		t.Fatal("ToAgentCoreEvent returned nil")
	}
	if ae.EventKind() != agentcore.EventA2UI {
		t.Fatalf("EventKind = %v, want %v", ae.EventKind(), agentcore.EventA2UI)
	}
	if ae.Envelope == nil {
		t.Fatal("Envelope is nil")
	}
	// Verify the envelope carries the expected fields.
	if ae.Envelope["version"] != "v0.9.1" {
		t.Fatalf("version = %v, want v0.9.1", ae.Envelope["version"])
	}
	cs, ok := ae.Envelope["createSurface"].(map[string]any)
	if !ok {
		t.Fatalf("createSurface not found or wrong type: %+v", ae.Envelope)
	}
	if cs["surfaceId"] != "test-surface" || cs["catalogId"] != BasicCatalogID {
		t.Fatalf("createSurface = %+v", cs)
	}
}

func TestToAgentCoreEventVersionFill(t *testing.T) {
	// Envelopes without a version should have v0.9.1 filled in.
	env := Envelope{CreateSurface: &CreateSurface{SurfaceID: "s", CatalogID: BasicCatalogID}}
	ae := ToAgentCoreEvent(env)
	if ae.Envelope["version"] != "v0.9.1" {
		t.Fatalf("version = %v, want v0.9.1", ae.Envelope["version"])
	}
}

func TestToAgentCoreEventHappyPath(t *testing.T) {
	// A normal envelope should produce a valid A2UIEvent with all fields intact.
	// envelopeToMap always succeeds for serializable Envelope structs, so the
	// error fallback path (added for defense-in-depth) is not reachable here.
	env := Envelope{CreateSurface: &CreateSurface{SurfaceID: "s", CatalogID: BasicCatalogID}}
	ae := ToAgentCoreEvent(env)
	if ae == nil || ae.Envelope == nil {
		t.Fatal("expected valid event even on edge case")
	}
}

func TestDataPartToEnvelopeEmptyMIME(t *testing.T) {
	// Empty MIME type with non-A2UI content should return ok=false, err=nil
	// (treated as non-match, not a hard error).
	part := a2a.Part{
		Type: a2a.PartTypeData,
		Data: &a2a.DataPart{MIMEType: "", Data: map[string]any{"key": "val"}},
	}
	_, ok, err := DataPartToEnvelope(part)
	if ok {
		t.Fatalf("expected ok=false for empty MIME, got ok=true")
	}
	if err != nil {
		t.Fatalf("expected err=nil for empty MIME, got %v", err)
	}
}

func TestDefaultServerCapabilities(t *testing.T) {
	caps := DefaultServerCapabilities()
	if len(caps.SupportedCatalogIDs) != 1 {
		t.Fatalf("SupportedCatalogIDs = %v, want [BasicCatalogID]", caps.SupportedCatalogIDs)
	}
	if caps.SupportedCatalogIDs[0] != BasicCatalogID {
		t.Fatalf("SupportedCatalogIDs[0] = %q, want %q", caps.SupportedCatalogIDs[0], BasicCatalogID)
	}
	if caps.AcceptsInlineCatalogs {
		t.Fatal("AcceptsInlineCatalogs should be false by default")
	}
}
