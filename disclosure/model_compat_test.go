package disclosure

import "testing"

func TestSupportsJSONSchemaResponseFormat(t *testing.T) {
	t.Setenv("PROVIDER", "")
	if supportsJSONSchemaResponseFormat() {
		t.Fatal("empty PROVIDER should default to deepseek-compatible false")
	}

	t.Setenv("PROVIDER", "deepseek")
	if supportsJSONSchemaResponseFormat() {
		t.Fatal("deepseek should not use json_schema response_format")
	}

	t.Setenv("PROVIDER", "generic")
	if !supportsJSONSchemaResponseFormat() {
		t.Fatal("generic should allow json_schema response_format")
	}
}
