package a2ui

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestEnvelopeRoundTripCreateSurface(t *testing.T) {
	env := NewCreateSurface("profile", BasicCatalogID)
	env.CreateSurface.Theme = map[string]any{"primaryColor": "#00BFFF"}
	env.CreateSurface.SendDataModel = true

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := ParseEnvelope(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Kind() != KindCreateSurface {
		t.Fatalf("kind = %v, want createSurface", got.Kind())
	}
	if got.CreateSurface.SurfaceID != "profile" || got.CreateSurface.CatalogID != BasicCatalogID {
		t.Fatalf("unexpected createSurface: %+v", got.CreateSurface)
	}
	if !got.CreateSurface.SendDataModel {
		t.Fatalf("sendDataModel lost in round trip")
	}
	if got.CreateSurface.Theme["primaryColor"] != "#00BFFF" {
		t.Fatalf("theme lost: %+v", got.CreateSurface.Theme)
	}
}

func TestEnvelopeVersionEmittedInWire(t *testing.T) {
	data, err := json.Marshal(NewDeleteSurface("s1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version":"v0.9.1"`) {
		t.Fatalf("version not present in wire form: %s", data)
	}
}

func TestParseEnvelopeRejectsMultipleBodies(t *testing.T) {
	raw := []byte(`{"version":"v0.9.1","deleteSurface":{"surfaceId":"a"},"createSurface":{"surfaceId":"b","catalogId":"c"}}`)
	if _, err := ParseEnvelope(raw); !errors.Is(err, ErrMultipleBodies) {
		t.Fatalf("err = %v, want ErrMultipleBodies", err)
	}
}

func TestParseEnvelopeRejectsNoBody(t *testing.T) {
	if _, err := ParseEnvelope([]byte(`{"version":"v0.9.1"}`)); !errors.Is(err, ErrNoBody) {
		t.Fatalf("err = %v, want ErrNoBody", err)
	}
}

func TestComponentFlatMarshaling(t *testing.T) {
	c := Text("greeting", "Hello")
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["id"] != "greeting" || m["component"] != "Text" || m["text"] != "Hello" {
		t.Fatalf("flat marshaling wrong: %v", m)
	}

	var back Component
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.ID != "greeting" || back.Type != "Text" || back.Props["text"] != "Hello" {
		t.Fatalf("unmarshal wrong: %+v", back)
	}
}

func TestDynamicMarshaling(t *testing.T) {
	cases := []struct {
		name string
		d    Dynamic
		want string
	}{
		{"literal", Lit("hi"), `"hi"`},
		{"path", Bind("/user/name"), `{"path":"/user/name"}`},
		{"function", Call("formatString", map[string]any{"value": "x"}), `{"call":"formatString","args":{"value":"x"}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.d)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.want {
				t.Fatalf("got %s want %s", data, tc.want)
			}
			var back Dynamic
			if err := json.Unmarshal(data, &back); err != nil {
				t.Fatal(err)
			}
			redata, _ := json.Marshal(back)
			if string(redata) != tc.want {
				t.Fatalf("round trip got %s want %s", redata, tc.want)
			}
		})
	}
}

func TestChildListMarshaling(t *testing.T) {
	static := StaticChildren("a", "b")
	data, _ := json.Marshal(static)
	if string(data) != `["a","b"]` {
		t.Fatalf("static children: %s", data)
	}

	tmpl := TemplateChildren("/users", "user_card")
	data, _ = json.Marshal(tmpl)
	if string(data) != `{"path":"/users","componentId":"user_card"}` {
		t.Fatalf("template children: %s", data)
	}

	var cl ChildList
	if err := json.Unmarshal([]byte(`["x","y"]`), &cl); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cl.Static, []string{"x", "y"}) {
		t.Fatalf("decoded static: %+v", cl)
	}
	if err := json.Unmarshal([]byte(`{"path":"/p","componentId":"c"}`), &cl); err != nil {
		t.Fatal(err)
	}
	if cl.Template == nil || cl.Template.ComponentID != "c" {
		t.Fatalf("decoded template: %+v", cl)
	}
}

func TestUpdateDataModelRemoveVsSet(t *testing.T) {
	set := NewUpdateDataModel("s", "/a", nil)
	data, _ := json.Marshal(set)
	if !strings.Contains(string(data), `"value":null`) {
		t.Fatalf("explicit null value should be present: %s", data)
	}

	rm := NewRemoveDataModel("s", "/a")
	data, _ = json.Marshal(rm)
	if strings.Contains(string(data), `"value"`) {
		t.Fatalf("remove must omit value: %s", data)
	}

	var back UpdateDataModel
	if err := json.Unmarshal([]byte(`{"surfaceId":"s","path":"/a"}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.ValueSet {
		t.Fatalf("ValueSet should be false when value omitted")
	}
}

func TestDataModelPointerEngine(t *testing.T) {
	model := any(map[string]any{})

	model, err := ApplyUpdate(model, "/user/name", "Alice", true)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := GetData(model, "/user/name"); !ok || v != "Alice" {
		t.Fatalf("get /user/name = %v %v", v, ok)
	}

	// Replace whole model.
	model, err = ApplyUpdate(model, "/", map[string]any{"items": []any{"a", "b"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := GetData(model, "/items/1"); !ok || v != "b" {
		t.Fatalf("get /items/1 = %v %v", v, ok)
	}

	// Append using "-".
	model, err = ApplyUpdate(model, "/items/-", "c", true)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := GetData(model, "/items/2"); !ok || v != "c" {
		t.Fatalf("append failed: %v %v", v, ok)
	}

	// Remove a key.
	model, err = ApplyUpdate(model, "/items", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := GetData(model, "/items"); ok {
		t.Fatalf("items should be removed")
	}
}

func TestPointerEscaping(t *testing.T) {
	model, err := ApplyUpdate(map[string]any{}, "/a~1b/c~0d", 42, true)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := GetData(model, "/a~1b/c~0d"); !ok || v != 42 {
		t.Fatalf("escaped pointer failed: %v %v", v, ok)
	}
	if JoinPointer("a/b", "c~d") != "/a~1b/c~0d" {
		t.Fatalf("JoinPointer escaping wrong: %s", JoinPointer("a/b", "c~d"))
	}
}

func TestSurfaceStoreLifecycle(t *testing.T) {
	store := NewSurfaceStore()

	if err := store.Apply(NewUpdateComponents("missing", Text("x", "y"))); !errors.Is(err, ErrSurfaceNotFound) {
		t.Fatalf("update before create: %v", err)
	}

	if err := store.Apply(NewCreateSurface("s", BasicCatalogID)); err != nil {
		t.Fatal(err)
	}
	if err := store.Apply(NewCreateSurface("s", BasicCatalogID)); !errors.Is(err, ErrSurfaceExists) {
		t.Fatalf("duplicate create: %v", err)
	}

	envs := []Envelope{
		NewUpdateComponents("s", Column("root", "name"), Text("name", Bind("/user/name"))),
		NewUpdateDataModel("s", "/user/name", "Ada"),
	}
	for _, e := range envs {
		if err := store.Apply(e); err != nil {
			t.Fatal(err)
		}
	}

	srf, ok := store.Surface("s")
	if !ok {
		t.Fatal("surface missing")
	}
	if _, ok := srf.Root(); !ok {
		t.Fatal("root missing")
	}
	if v, ok := srf.Get("/user/name"); !ok || v != "Ada" {
		t.Fatalf("data model: %v %v", v, ok)
	}

	// Delete is a no-op for unknown surfaces and removes known ones.
	if err := store.Apply(NewDeleteSurface("nope")); err != nil {
		t.Fatalf("delete unknown should be no-op: %v", err)
	}
	if err := store.Apply(NewDeleteSurface("s")); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Surface("s"); ok {
		t.Fatal("surface should be deleted")
	}
}

func TestClientDataModelCollection(t *testing.T) {
	store := NewSurfaceStore()
	cs := NewCreateSurface("s", BasicCatalogID)
	cs.CreateSurface.SendDataModel = true
	_ = store.Apply(cs)
	_ = store.Apply(NewCreateSurface("other", BasicCatalogID))
	_ = store.Apply(NewUpdateDataModel("s", "/k", "v"))
	_ = store.Apply(NewUpdateDataModel("other", "/k", "hidden"))

	payload := store.ClientDataModel()
	if _, ok := payload.Surfaces["s"]; !ok {
		t.Fatal("surface s should be included")
	}
	if _, ok := payload.Surfaces["other"]; ok {
		t.Fatal("surface without sendDataModel must be excluded")
	}
}

func TestValidateEnvelope(t *testing.T) {
	cat := BasicCatalog()

	errs := ValidateEnvelope(NewCreateSurface("", ""), cat)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Code != CodeValidationFailed {
			t.Fatalf("code = %s", e.Code)
		}
	}

	errs = ValidateEnvelope(NewUpdateComponents("s", NewComponent("a", "Nonexistent", nil)), cat)
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "unknown component") {
		t.Fatalf("expected unknown component error, got %v", errs)
	}

	if errs := ValidateEnvelope(NewCreateSurface("s", BasicCatalogID), cat); len(errs) != 0 {
		t.Fatalf("valid envelope produced errors: %v", errs)
	}
}

func TestValidateSurfaceTree(t *testing.T) {
	cat := BasicCatalog()
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("s", BasicCatalogID))

	// Missing root + dangling reference.
	_ = store.Apply(NewUpdateComponents("s", Card("card", "ghost")))
	srf, _ := store.Surface("s")
	errs := ValidateSurfaceTree(srf, cat)
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
	var hasRoot, hasDangling bool
	for _, e := range errs {
		if strings.Contains(e.Message, "no \"root\"") {
			hasRoot = true
		}
		if strings.Contains(e.Message, "undefined component") {
			hasDangling = true
		}
	}
	if !hasRoot || !hasDangling {
		t.Fatalf("missing expected errors: %v", errs)
	}

	// Now make it valid.
	store2 := NewSurfaceStore()
	_ = store2.Apply(NewCreateSurface("s", BasicCatalogID))
	_ = store2.Apply(NewUpdateComponents("s", Column("root", "t"), Text("t", "hi")))
	srf2, _ := store2.Surface("s")
	if errs := ValidateSurfaceTree(srf2, cat); len(errs) != 0 {
		t.Fatalf("valid tree produced errors: %v", errs)
	}
}

func TestValidateDetectsCycle(t *testing.T) {
	cat := BasicCatalog()
	store := NewSurfaceStore()
	_ = store.Apply(NewCreateSurface("s", BasicCatalogID))
	_ = store.Apply(NewUpdateComponents("s",
		Column("root", "a"),
		Card("a", "b"),
		Card("b", "a"),
	))
	srf, _ := store.Surface("s")
	errs := ValidateSurfaceTree(srf, cat)
	var cyc bool
	for _, e := range errs {
		if strings.Contains(e.Message, "circular reference") {
			cyc = true
		}
	}
	if !cyc {
		t.Fatalf("expected circular reference error, got %v", errs)
	}
}

func TestStreamEncodeDecodeJSONL(t *testing.T) {
	envs := NewSurface("s", BasicCatalogID).
		Add(Column("root", "t"), Text("t", "hi")).
		Data("/x", 1).
		Build()

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.EncodeAll(envs); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimRight(buf.String(), "\n"), "\n") + 1; lines != len(envs) {
		t.Fatalf("expected %d JSONL lines, got %d: %q", len(envs), lines, buf.String())
	}

	dec := NewDecoder(&buf)
	var decoded []Envelope
	for {
		env, err := dec.Decode()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		decoded = append(decoded, env)
	}
	if len(decoded) != len(envs) {
		t.Fatalf("decoded %d, want %d", len(decoded), len(envs))
	}
	if decoded[0].Kind() != KindCreateSurface || decoded[1].Kind() != KindUpdateComponents {
		t.Fatalf("unexpected decoded kinds: %v %v", decoded[0].Kind(), decoded[1].Kind())
	}
}
