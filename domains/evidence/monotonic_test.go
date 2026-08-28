package evidence

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormRequirementDenyCheck_OverseasWithoutNotarization(t *testing.T) {
	check := FormRequirementDenyCheck()
	args := json.RawMessage(`{"source_uri":"https://example.com","evidence_type_hint":"overseas"}`)
	reason, ok := check(TypeSpecificToolName, args)
	if !ok {
		t.Fatal("overseas evidence without notarization must be denied")
	}
	for _, want := range []string{"EVI-011", "notarization_status"} {
		if !strings.Contains(reason, want) {
			t.Errorf("deny reason should mention %q, got %s", want, reason)
		}
	}
}

func TestFormRequirementDenyCheck_OverseasWithNotarization(t *testing.T) {
	check := FormRequirementDenyCheck()
	args := json.RawMessage(`{"source_uri":"https://example.com","evidence_type_hint":"overseas","notarization_status":"已认证"}`)
	if _, ok := check(TypeSpecificToolName, args); ok {
		t.Fatal("notarized overseas evidence must pass")
	}
}

func TestFormRequirementDenyCheck_ForeignWithoutTranslation(t *testing.T) {
	check := FormRequirementDenyCheck()
	args := json.RawMessage(`{"source_uri":"https://example.com/ja","evidence_type_hint":"foreign_language"}`)
	if _, ok := check(TypeSpecificToolName, args); !ok {
		t.Fatal("foreign evidence without translation must be denied")
	}

	// 补齐译本声明后放行。
	args2 := json.RawMessage(`{"source_uri":"https://example.com/ja","evidence_type_hint":"foreign_language","translation_status":"completed"}`)
	if _, ok := check(TypeSpecificToolName, args2); ok {
		t.Fatal("translated foreign evidence must pass")
	}
}

func TestFormRequirementDenyCheck_GuardsOnlyKnownTools(t *testing.T) {
	check := FormRequirementDenyCheck()
	args := json.RawMessage(`{"evidence_type_hint":"overseas"}`)
	// 同样的参数落在非守卫工具上不拦截。
	if _, ok := check("some_other_tool", args); ok {
		t.Fatal("unguarded tool must not be denied")
	}
}

func TestFormRequirementDenyCheck_NoHintNoDeny(t *testing.T) {
	check := FormRequirementDenyCheck()
	args := json.RawMessage(`{"source_uri":"https://example.com"}`)
	if _, ok := check(TypeSpecificToolName, args); ok {
		t.Fatal("no type hint must not be denied (engine infers type)")
	}
	// 坏参数不拦截（交给工具自身参数校验）。
	if _, ok := check(TypeSpecificToolName, json.RawMessage(`{broken`)); ok {
		t.Fatal("broken args must not be denied here")
	}
}
