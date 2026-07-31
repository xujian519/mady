package theme

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAnyToColorString(t *testing.T) {
	cases := []struct {
		name    string
		in      any
		want    string
		wantErr bool
	}{
		{"string-trimmed", "  #ff0000  ", "#ff0000", false},
		{"empty-string", "", "", false},
		{"float64-int", float64(15), "15", false},
		{"float64-int-large", float64(123456789), "123456789", false},
		{"float64-fraction", 1.5, "", true},
		{"json-number-int", json.Number("42"), "42", false},
		{"json-number-fraction", json.Number("3.14"), "3.14", false},
		{"nil", nil, "", false},
		{"bool", true, "", true},
		{"map", map[string]string{}, "", true},
		{"slice", []int{1}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := anyToColorString(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("anyToColorString(%v) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("anyToColorString(%v): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("anyToColorString(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// applyColorKeyKeys maps JSON color keys to their SemanticTheme struct fields.
var applyColorKeyKeys = map[string]string{
	"accent": "Accent", "border": "Border", "borderAccent": "BorderAccent",
	"borderMuted": "BorderMuted", "success": "Success", "error": "Error",
	"warning": "Warning", "muted": "Muted", "dim": "Dim", "text": "Text",
	"system": "System", "assistantText": "AssistantText",
	"thinkingText": "ThinkingText", "userMessageText": "UserMessage",
	"selectedBg": "SelectedBg", "userMessageBg": "UserMessageBg",
	"toolPendingBg": "ToolPendingBg", "toolSuccessBg": "ToolSuccessBg",
	"toolErrorBg": "ToolErrorBg", "mdHeading": "MdHeading", "mdLink": "MdLink",
	"mdLinkUrl": "MdLinkURL", "mdCode": "MdCode", "mdCodeBlock": "MdCodeBlock",
	"mdCodeBlockBorder": "MdCodeBlockBorder", "mdQuote": "MdQuote",
	"mdQuoteBorder": "MdQuoteBorder", "mdHr": "MdHr",
	"mdListBullet": "MdListBullet", "syntaxComment": "SyntaxComment",
	"syntaxKeyword": "SyntaxKeyword", "syntaxFunction": "SyntaxFunction",
	"syntaxVariable": "SyntaxVariable", "syntaxString": "SyntaxString",
	"syntaxNumber": "SyntaxNumber", "syntaxType": "SyntaxType",
	"syntaxOperator": "SyntaxOperator", "syntaxPunctuation": "SyntaxPunctuation",
	"loaderSpinner": "LoaderSpinner", "progressBar": "ProgressBar",
	"background": "Background", "surface": "Surface",
	"surfaceRaised": "SurfaceRaised", "evidenceSupport": "EvidenceSupport",
	"evidenceCounter": "EvidenceCounter", "confidenceLow": "ConfidenceLow",
	"confidenceMedium": "ConfidenceMedium", "confidenceHigh": "ConfidenceHigh",
}

func TestApplyColorKeyAllFields(t *testing.T) {
	const col = "#aabbcc"
	for key, field := range applyColorKeyKeys {
		t.Run(key, func(t *testing.T) {
			th := &SemanticTheme{}
			applyColorKey(th, key, col)
			got := reflect.ValueOf(th).Elem().FieldByName(field).String()
			if got != col {
				t.Fatalf("applyColorKey(%q) set %s = %q, want %q", key, field, got, col)
			}
		})
	}
}

func TestApplyColorKeyUnknownKeyIgnored(t *testing.T) {
	th := DefaultMadyDark()
	// Must not panic and must not mutate anything.
	applyColorKey(th, "totally-unknown-key", "#ffffff")
	applyColorKey(th, "", "#ffffff")
	if th.Accent != DefaultMadyDark().Accent {
		t.Fatal("unknown key mutated the theme")
	}
}

func TestParseSemanticThemeJSONErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"invalid-json", `{not json`},
		{"bad-var-type", `{"vars":{"x":true}}`},
		{"bad-color-type", `{"colors":{"accent":true}}`},
		{"bad-color-fraction", `{"colors":{"accent":1.5}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseSemanticThemeJSON([]byte(tc.raw), DefaultSemanticLight()); err == nil {
				t.Fatalf("ParseSemanticThemeJSON(%q) should return error", tc.raw)
			}
		})
	}
}

func TestParseSemanticThemeJSONNumericColor(t *testing.T) {
	// float64 integer color value -> decimal string "15" -> resolved as-is.
	sem, err := ParseSemanticThemeJSON([]byte(`{"colors":{"accent":15}}`), DefaultSemanticLight())
	if err != nil {
		t.Fatal(err)
	}
	if sem.Accent != "15" {
		t.Fatalf("accent = %q, want 15", sem.Accent)
	}
}

func TestParseSemanticThemeJSONNullColorSkipped(t *testing.T) {
	sem, err := ParseSemanticThemeJSON([]byte(`{"colors":{"accent":null}}`), DefaultSemanticLight())
	if err != nil {
		t.Fatal(err)
	}
	// Empty resolved color -> skipped, base value preserved.
	if sem.Accent != DefaultSemanticLight().Accent {
		t.Fatalf("null color should be skipped, got %q", sem.Accent)
	}
}

func TestParseSemanticThemeJSONVarChain(t *testing.T) {
	raw := `{
		"name": "chain",
		"vars": {"base": "#123456", "derived": "base"},
		"colors": {"accent": "derived", "border": "base"}
	}`
	sem, err := ParseSemanticThemeJSON([]byte(raw), DefaultSemanticLight())
	if err != nil {
		t.Fatal(err)
	}
	if sem.Accent != "#123456" {
		t.Fatalf("chained var accent = %q, want #123456", sem.Accent)
	}
	if sem.Border != "#123456" {
		t.Fatalf("direct var border = %q, want #123456", sem.Border)
	}
}

func TestParseSemanticThemeJSONVarCycleReturnsEmpty(t *testing.T) {
	raw := `{"vars":{"a":"b","b":"a"},"colors":{"accent":"a"}}`
	sem, err := ParseSemanticThemeJSON([]byte(raw), DefaultSemanticLight())
	if err != nil {
		t.Fatal(err)
	}
	// Cyclic reference resolves to "" -> key skipped, base preserved.
	if sem.Accent != DefaultSemanticLight().Accent {
		t.Fatalf("cyclic var should be skipped, got %q", sem.Accent)
	}
}

func TestParseSemanticThemeJSONUnknownVarRef(t *testing.T) {
	raw := `{"vars":{"a":"missing-var"},"colors":{"accent":"a"}}`
	sem, err := ParseSemanticThemeJSON([]byte(raw), DefaultSemanticLight())
	if err != nil {
		t.Fatal(err)
	}
	if sem.Accent != DefaultSemanticLight().Accent {
		t.Fatalf("unknown var reference should be skipped, got %q", sem.Accent)
	}
}

func TestParseSemanticThemeJSONNilBase(t *testing.T) {
	sem, err := ParseSemanticThemeJSON([]byte(`{"name":"nb","colors":{"accent":"#010101"}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sem.Name != "nb" {
		t.Fatalf("name = %q, want nb", sem.Name)
	}
	if sem.Accent != "#010101" {
		t.Fatalf("accent = %q, want #010101", sem.Accent)
	}
}

func TestParseSemanticThemeJSONEmptyNameKeepsBase(t *testing.T) {
	sem, err := ParseSemanticThemeJSON([]byte(`{"colors":{"accent":"#020202"}}`), DefaultMadyDark())
	if err != nil {
		t.Fatal(err)
	}
	if sem.Name != "dark" {
		t.Fatalf("empty JSON name should keep base name, got %q", sem.Name)
	}
}
