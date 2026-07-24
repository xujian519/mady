package agentconfig_test

import (
	"testing"

	"github.com/xujian519/mady/pkg/agentconfig"
	"github.com/xujian519/mady/prompt"
)

type fakePromptStore struct {
	data map[string]prompt.ResolvedPrompt
}

func (f *fakePromptStore) Resolve(name string, vars map[string]string) (prompt.ResolvedPrompt, bool) {
	r, ok := f.data[name]
	return r, ok
}

func TestResolveSystemPrompt_InlineUnchanged(t *testing.T) {
	store := &fakePromptStore{}
	got, ok := agentconfig.ResolveSystemPrompt("you are helpful", store)
	if !ok {
		t.Fatal("expected ok for inline prompt")
	}
	if got != "you are helpful" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSystemPrompt_TemplateReference(t *testing.T) {
	store := &fakePromptStore{
		data: map[string]prompt.ResolvedPrompt{
			"patent-expert": {
				SystemPrompt: "你是资深专利代理师",
				UserPrompt:   "用户问题",
			},
		},
	}
	got, ok := agentconfig.ResolveSystemPrompt("prompt://patent-expert", store)
	if !ok {
		t.Fatal("expected ok for existing template")
	}
	if got != "你是资深专利代理师" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSystemPrompt_MissingTemplate(t *testing.T) {
	store := &fakePromptStore{data: map[string]prompt.ResolvedPrompt{}}
	raw := "prompt://missing"
	got, ok := agentconfig.ResolveSystemPrompt(raw, store)
	if ok {
		t.Fatal("expected not ok for missing template")
	}
	if got != raw {
		t.Fatalf("expected raw fallback %q, got %q", raw, got)
	}
}

func TestResolveSystemPrompt_NilStore(t *testing.T) {
	got, ok := agentconfig.ResolveSystemPrompt("prompt://anything", nil)
	if !ok {
		t.Fatal("expected ok when store is nil")
	}
	if got != "prompt://anything" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSystemPrompt_EmptyName(t *testing.T) {
	store := &fakePromptStore{}
	got, ok := agentconfig.ResolveSystemPrompt("prompt://   ", store)
	if ok {
		t.Fatal("expected not ok for empty name")
	}
	if got != "prompt://   " {
		t.Fatalf("got %q", got)
	}
}

func TestResolveSystemPromptStrict_ErrorOnMissing(t *testing.T) {
	store := &fakePromptStore{data: map[string]prompt.ResolvedPrompt{}}
	_, err := agentconfig.ResolveSystemPromptStrict("prompt://missing", store)
	if err == nil {
		t.Fatal("expected error for missing template in strict mode")
	}
}

func TestResolveSystemPromptStrict_OK(t *testing.T) {
	store := &fakePromptStore{
		data: map[string]prompt.ResolvedPrompt{
			"ok": {SystemPrompt: "ok system"},
		},
	}
	got, err := agentconfig.ResolveSystemPromptStrict("prompt://ok", store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok system" {
		t.Fatalf("got %q", got)
	}
}
