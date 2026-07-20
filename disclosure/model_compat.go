package disclosure

import (
	"os"
	"strings"
)

// supportsJSONSchemaResponseFormat reports whether the current provider setup
// should receive Chat Completions `response_format: {type: json_schema}`.
//
// DeepSeek's current chat-completions endpoint rejects this field for the
// models used by disclosure, so we fall back to prompt-constrained JSON output
// and parse the returned text locally.
func supportsJSONSchemaResponseFormat() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROVIDER"))) {
	case "", "deepseek":
		return false
	default:
		return true
	}
}
