package agentconfig

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/prompt"
)

const promptTemplatePrefix = "prompt://"

// PromptTemplateStore is the subset of prompt.PromptStore needed to resolve
// system prompt template references. Using an interface keeps unit tests
// lightweight and avoids forcing a full PromptStore in contexts that only
// need name lookup.
type PromptTemplateStore interface {
	// Resolve fills template variables in the named template and returns the
	// resolved prompts. The second return value is false when not found.
	Resolve(name string, vars map[string]string) (prompt.ResolvedPrompt, bool)
}

// ResolveSystemPrompt interprets a system_prompt value. If it starts with
// "prompt://<name>", the named template's system_prompt field is returned.
// Otherwise the raw value is returned unchanged.
//
// When the template is not found, it returns the raw value and false so
// callers can decide whether to fall back or fail.
func ResolveSystemPrompt(raw string, store PromptTemplateStore) (string, bool) {
	if store == nil || !strings.HasPrefix(raw, promptTemplatePrefix) {
		return raw, true
	}

	name := strings.TrimSpace(strings.TrimPrefix(raw, promptTemplatePrefix))
	if name == "" {
		return raw, false
	}

	resolved, ok := store.Resolve(name, nil)
	if !ok {
		return raw, false
	}
	return resolved.SystemPrompt, true
}

// ResolveSystemPromptStrict is like ResolveSystemPrompt but returns an error
// when a prompt:// reference cannot be resolved. Use this when the caller
// wants to fail fast on missing templates.
func ResolveSystemPromptStrict(raw string, store PromptTemplateStore) (string, error) {
	resolved, ok := ResolveSystemPrompt(raw, store)
	if !ok {
		return raw, fmt.Errorf("agentconfig: prompt template %q not found", raw)
	}
	return resolved, nil
}
