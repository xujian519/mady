package a2ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuilderProducesOrderedEnvelopes(t *testing.T) {
	envs := NewSurface("s", BasicCatalogID).
		SendDataModel(true).
		Add(Column("root", "t")).
		Add(Text("t", "hi")).
		Data("/a", 1).
		RemoveData("/b").
		Build()

	if len(envs) != 4 {
		t.Fatalf("expected 4 envelopes, got %d", len(envs))
	}
	wantKinds := []MessageKind{KindCreateSurface, KindUpdateComponents, KindUpdateDataModel, KindUpdateDataModel}
	for i, want := range wantKinds {
		if envs[i].Kind() != want {
			t.Fatalf("env %d kind = %v, want %v", i, envs[i].Kind(), want)
		}
	}
	if !envs[0].CreateSurface.SendDataModel {
		t.Fatal("sendDataModel not propagated")
	}
}

func TestClientActionTimestamp(t *testing.T) {
	a := NewClientAction("submit", "s", "btn", map[string]any{"x": 1})
	if a.Timestamp == "" {
		t.Fatal("timestamp not set")
	}
	data, _ := json.Marshal(ClientMessage{Action: a})
	if !strings.Contains(string(data), `"sourceComponentId":"btn"`) {
		t.Fatalf("client action wire form wrong: %s", data)
	}
}

func TestClientError(t *testing.T) {
	err := ClientError{Code: CodeValidationFailed, SurfaceID: "s", Path: "/a", Message: "bad"}
	if s := err.Error(); s != "VALIDATION_FAILED at /a: bad" {
		t.Fatalf("unexpected error string with path: %q", s)
	}
	err2 := ClientError{Code: "ERR", Message: "something"}
	if s := err2.Error(); s != "ERR: something" {
		t.Fatalf("unexpected error string without path: %q", s)
	}
}

func TestValidationErrorToClientError(t *testing.T) {
	ve := ValidationError{Code: CodeValidationFailed, SurfaceID: "s", Path: "/x", Message: "invalid"}
	ce := ve.ToClientError()
	if ce.Code != ve.Code || ce.SurfaceID != ve.SurfaceID || ce.Path != ve.Path || ce.Message != ve.Message {
		t.Fatalf("ToClientError mismatch: %+v vs %+v", ve, ce)
	}
}

func TestEnvelopeSurfaceID(t *testing.T) {
	if id := NewCreateSurface("s1", BasicCatalogID).SurfaceID(); id != "s1" {
		t.Fatalf("createSurface SurfaceID: %q", id)
	}
	if id := NewUpdateComponents("s2", Text("t", "hi")).SurfaceID(); id != "s2" {
		t.Fatalf("updateComponents SurfaceID: %q", id)
	}
	if id := NewUpdateDataModel("s3", "/a", 1).SurfaceID(); id != "s3" {
		t.Fatalf("updateDataModel SurfaceID: %q", id)
	}
	if id := NewDeleteSurface("s4").SurfaceID(); id != "s4" {
		t.Fatalf("deleteSurface SurfaceID: %q", id)
	}
	var empty Envelope
	if id := empty.SurfaceID(); id != "" {
		t.Fatalf("empty SurfaceID: %q", id)
	}
}

func TestUpdateDataModelSetValue(t *testing.T) {
	u := &UpdateDataModel{SurfaceID: "s", Path: "/a"}
	u.SetValue(42)
	if !u.ValueSet {
		t.Fatal("ValueSet should be true after SetValue")
	}
	if u.Value != 42 {
		t.Fatalf("Value = %v, want 42", u.Value)
	}
	data, _ := json.Marshal(Envelope{Version: Version, UpdateDataModel: u})
	if !strings.Contains(string(data), `"value":42`) {
		t.Fatalf("value not in wire form: %s", data)
	}
}

func TestCatalogRegistry(t *testing.T) {
	r := NewCatalogRegistry()
	cat, ok := r.Lookup(BasicCatalogID)
	if !ok {
		t.Fatal("basic catalog not found in registry")
	}
	if cat.ID != BasicCatalogID {
		t.Fatalf("unexpected catalog ID: %q", cat.ID)
	}

	custom := &Catalog{
		ID: "urn:custom",
		Components: map[string]ComponentDef{
			"MyWidget": {Name: "MyWidget", ChildFields: []string{"child"}},
		},
		Functions: map[string]struct{}{"myFunc": {}},
	}
	r.Register(custom)
	got, ok := r.Lookup("urn:custom")
	if !ok {
		t.Fatal("custom catalog not found")
	}
	if got.ID != "urn:custom" {
		t.Fatalf("unexpected catalog ID: %q", got.ID)
	}
	if !got.HasComponent("MyWidget") {
		t.Fatal("MyWidget not found in custom catalog")
	}
	if !got.HasFunction("myFunc") {
		t.Fatal("myFunc not found in custom catalog")
	}

	// Register nil is a no-op.
	r.Register(nil)
	// Verify no panic and existing catalogs are still there.
	if _, ok := r.Lookup(BasicCatalogID); !ok {
		t.Fatal("basic catalog disappeared after nil register")
	}
}

func TestBuilderConstructors(t *testing.T) {
	t.Run("Text", func(t *testing.T) {
		c := Text("t1", "hello")
		if c.ID != "t1" || c.Type != "Text" || c.Props["text"] != "hello" {
			t.Fatalf("Text: %+v", c)
		}
	})
	t.Run("Image", func(t *testing.T) {
		c := Image("i1", "https://example.com/img.png")
		if c.ID != "i1" || c.Type != "Image" || c.Props["url"] != "https://example.com/img.png" {
			t.Fatalf("Image: %+v", c)
		}
	})
	t.Run("Icon", func(t *testing.T) {
		c := Icon("ic1", "star")
		if c.ID != "ic1" || c.Type != "Icon" || c.Props["name"] != "star" {
			t.Fatalf("Icon: %+v", c)
		}
	})
	t.Run("Row", func(t *testing.T) {
		c := Row("r1", "a", "b")
		if c.ID != "r1" || c.Type != "Row" {
			t.Fatalf("Row: %+v", c)
		}
		cl, ok := c.Props["children"].(ChildList)
		if !ok || len(cl.Static) != 2 || cl.Static[0] != "a" {
			t.Fatalf("Row children: %+v", c.Props["children"])
		}
	})
	t.Run("List", func(t *testing.T) {
		c := List("l1", "x", "y")
		if c.ID != "l1" || c.Type != "List" {
			t.Fatalf("List: %+v", c)
		}
	})
	t.Run("TemplateList", func(t *testing.T) {
		c := TemplateList("tl1", "/items", "card")
		if c.ID != "tl1" || c.Type != "List" {
			t.Fatalf("TemplateList: %+v", c)
		}
		cl, ok := c.Props["children"].(ChildList)
		if !ok || cl.Template == nil || cl.Template.Path != "/items" || cl.Template.ComponentID != "card" {
			t.Fatalf("TemplateList children: %+v", c.Props["children"])
		}
	})
	t.Run("Divider", func(t *testing.T) {
		c := Divider("d1")
		if c.ID != "d1" || c.Type != "Divider" {
			t.Fatalf("Divider: %+v", c)
		}
	})
	t.Run("Button", func(t *testing.T) {
		action := EventAction("submit", map[string]any{"k": 1})
		c := Button("b1", "Click", action)
		if c.ID != "b1" || c.Type != "Button" || c.Props["text"] != "Click" {
			t.Fatalf("Button: %+v", c)
		}
	})
	t.Run("TextField", func(t *testing.T) {
		c := TextField("tf1", "/user/name")
		if c.ID != "tf1" || c.Type != "TextField" {
			t.Fatalf("TextField: %+v", c)
		}
		d, ok := c.Props["value"].(Dynamic)
		if !ok || !d.IsPath || d.Path != "/user/name" {
			t.Fatalf("TextField value binding: %+v", c.Props["value"])
		}
	})
	t.Run("CheckBox", func(t *testing.T) {
		c := CheckBox("cb1", "Accept", "/terms")
		if c.ID != "cb1" || c.Type != "CheckBox" {
			t.Fatalf("CheckBox: %+v", c)
		}
	})
	t.Run("Slider", func(t *testing.T) {
		c := Slider("s1", "/volume")
		if c.ID != "s1" || c.Type != "Slider" {
			t.Fatalf("Slider: %+v", c)
		}
	})
	t.Run("Video", func(t *testing.T) {
		c := Video("v1", "https://example.com/video.mp4")
		if c.ID != "v1" || c.Type != "Video" || c.Props["url"] != "https://example.com/video.mp4" {
			t.Fatalf("Video: %+v", c)
		}
	})
	t.Run("AudioPlayer", func(t *testing.T) {
		c := AudioPlayer("a1", "https://example.com/audio.mp3")
		if c.ID != "a1" || c.Type != "AudioPlayer" || c.Props["url"] != "https://example.com/audio.mp3" {
			t.Fatalf("AudioPlayer: %+v", c)
		}
	})
	t.Run("Tabs", func(t *testing.T) {
		c := Tabs("tab1", []TabItem{{Child: "info", Label: "info"}, {Child: "settings", Label: "settings"}})
		if c.ID != "tab1" || c.Type != "Tabs" {
			t.Fatalf("Tabs: %+v", c)
		}
		tabs, ok := c.Props["tabs"].([]any)
		if !ok || len(tabs) != 2 {
			t.Fatalf("Tabs tabs: %+v", c.Props["tabs"])
		}
	})
	t.Run("Modal", func(t *testing.T) {
		c := Modal("m1", "body", "trigger")
		if c.ID != "m1" || c.Type != "Modal" {
			t.Fatalf("Modal: %+v", c)
		}
		if c.Props["child"] != "body" || c.Props["entryPointChild"] != "trigger" {
			t.Fatalf("Modal props: %+v", c.Props)
		}
	})
	t.Run("DateTimeInput", func(t *testing.T) {
		c := DateTimeInput("dt1", "/appointment")
		if c.ID != "dt1" || c.Type != "DateTimeInput" {
			t.Fatalf("DateTimeInput: %+v", c)
		}
		d, ok := c.Props["value"].(Dynamic)
		if !ok || !d.IsPath || d.Path != "/appointment" {
			t.Fatalf("DateTimeInput value binding: %+v", c.Props["value"])
		}
	})
	t.Run("ChoicePicker", func(t *testing.T) {
		opts := []map[string]any{{"label": "A", "value": "a"}, {"label": "B", "value": "b"}}
		c := ChoicePicker("cp1", "/choice", opts)
		if c.ID != "cp1" || c.Type != "ChoicePicker" {
			t.Fatalf("ChoicePicker: %+v", c)
		}
		d, ok := c.Props["value"].(Dynamic)
		if !ok || !d.IsPath || d.Path != "/choice" {
			t.Fatalf("ChoicePicker value binding: %+v", c.Props["value"])
		}
	})
	t.Run("Set", func(t *testing.T) {
		c := NewComponent("x", "Text", nil)
		c.Set("text", "hi")
		if c.Props["text"] != "hi" {
			t.Fatalf("Set after pointer receiver: %+v", c)
		}
	})
	t.Run("FunctionAction", func(t *testing.T) {
		a := FunctionAction("validate", map[string]any{"val": 1})
		if a.FunctionCall == nil || a.FunctionCall.CallName != "validate" {
			t.Fatalf("FunctionAction: %+v", a)
		}
	})
}

func TestNewCheck(t *testing.T) {
	chk := NewCheck("required", "此项必填")
	if chk.Call != "required" || chk.Message != "此项必填" {
		t.Fatalf("NewCheck: %+v", chk)
	}
	if chk.Args != nil {
		t.Fatalf("NewCheck: Args should be nil")
	}
	if chk.Condition != nil {
		t.Fatalf("NewCheck: Condition should be nil")
	}
}

func TestBuilderThemeAndDelete(t *testing.T) {
	b := NewSurface("s", BasicCatalogID)
	b.Theme(map[string]any{"primaryColor": "red"})
	b.SendDataModel(true)
	_ = b.Add(Column("root", "t"), Text("t", "hi"))
	_ = b.Data("/x", 1)

	envs := b.Build()
	if len(envs) != 3 {
		t.Fatalf("expected 3 envelopes, got %d", len(envs))
	}
	if envs[0].Kind() != KindCreateSurface {
		t.Fatalf("first should be createSurface, got %v", envs[0].Kind())
	}
	if envs[0].CreateSurface.Theme["primaryColor"] != "red" {
		t.Fatalf("theme not propagated: %+v", envs[0].CreateSurface.Theme)
	}
	if !envs[0].CreateSurface.SendDataModel {
		t.Fatal("sendDataModel not propagated")
	}

	del := b.Delete()
	if del.Kind() != KindDeleteSurface || del.DeleteSurface.SurfaceID != "s" {
		t.Fatalf("Delete: %+v", del)
	}
}
