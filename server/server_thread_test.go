package server

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/session"
	"github.com/xujian519/mady/skill"
)

func TestServerChatThreadPersistsConversationState(t *testing.T) {
	store := newMemoryStore()
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: store,
	})

	first := postChat(t, srv.Handler(), ChatRequest{Message: "hello", ThreadID: "thread-1"})
	if first.Output != "users:1 last:hello" {
		t.Fatalf("first output = %q", first.Output)
	}
	if first.ThreadID != "thread-1" {
		t.Fatalf("first thread_id = %q", first.ThreadID)
	}

	second := postChat(t, srv.Handler(), ChatRequest{Message: "again", ThreadID: "thread-1"})
	if second.Output != "users:2 last:again" {
		t.Fatalf("second output = %q", second.Output)
	}
}

func TestServerChatWithoutThreadRemainsStateless(t *testing.T) {
	store := newMemoryStore()
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: store,
	})

	first := postChat(t, srv.Handler(), ChatRequest{Message: "hello"})
	second := postChat(t, srv.Handler(), ChatRequest{Message: "again"})

	if first.Output != "users:1 last:hello" {
		t.Fatalf("first output = %q", first.Output)
	}
	if second.Output != "users:1 last:again" {
		t.Fatalf("second output = %q", second.Output)
	}
}

func TestServerChatThreadOverridesCheckpointThreadID(t *testing.T) {
	store := newMemoryStore()
	checkpoints := agentcore.NewMemoryCheckpointSaver()
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: historyProvider{},
		},
		Store: store,
		Checkpoint: &agentcore.CheckpointSettings{
			Saver:    checkpoints,
			ThreadID: "default-thread",
		},
	})

	resp := postChat(t, srv.Handler(), ChatRequest{Message: "hello", ThreadID: "thread-override"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if len(checkpoints.All("thread-override")) == 0 {
		t.Fatal("expected checkpoints for request thread")
	}
	if len(checkpoints.All("default-thread")) != 0 {
		t.Fatal("did not expect checkpoints for default thread")
	}
}

func TestServerChatUsesDefaultThinkingConfig(t *testing.T) {
	provider := &captureThinkingProvider{}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: provider,
			Thinking: &agentcore.ThinkingConfig{
				Display: agentcore.ThinkingDisplaySummarized,
				Effort:  agentcore.ThinkingEffortMedium,
				Budget:  1024,
			},
		},
	})

	resp := postChat(t, srv.Handler(), ChatRequest{Message: "hello"})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if provider.lastThinking == nil {
		t.Fatal("expected default thinking config to reach provider")
	}
	if provider.lastThinking.Display != agentcore.ThinkingDisplaySummarized {
		t.Fatalf("display = %q", provider.lastThinking.Display)
	}
	if provider.lastThinking.Effort != agentcore.ThinkingEffortMedium {
		t.Fatalf("effort = %q", provider.lastThinking.Effort)
	}
	if provider.lastThinking.Budget != 1024 {
		t.Fatalf("budget = %d", provider.lastThinking.Budget)
	}
}

func TestServerChatRequestThinkingOverridesDefault(t *testing.T) {
	provider := &captureThinkingProvider{}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: provider,
			Thinking: &agentcore.ThinkingConfig{
				Display: agentcore.ThinkingDisplaySummarized,
				Effort:  agentcore.ThinkingEffortHigh,
				Budget:  4096,
			},
		},
	})

	resp := postChat(t, srv.Handler(), ChatRequest{
		Message: "hello",
		Thinking: &agentcore.ThinkingConfig{
			Display: agentcore.ThinkingDisplayOmitted,
			Budget:  -1,
		},
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if provider.lastThinking == nil {
		t.Fatal("expected request thinking config to reach provider")
	}
	if provider.lastThinking.Display != agentcore.ThinkingDisplayOmitted {
		t.Fatalf("display = %q", provider.lastThinking.Display)
	}
	if provider.lastThinking.Effort != agentcore.ThinkingEffortDefault {
		t.Fatalf("effort = %q", provider.lastThinking.Effort)
	}
	if provider.lastThinking.Budget != -1 {
		t.Fatalf("budget = %d", provider.lastThinking.Budget)
	}
}

func TestServerThreadThinkingEndpointsAndChatInheritance(t *testing.T) {
	sessionFS, err := session.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	threadStore := session.NewAgentStore(sessionFS, "/project")
	provider := &captureThinkingProvider{}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "stub",
			Provider: provider,
			Thinking: &agentcore.ThinkingConfig{
				Display: agentcore.ThinkingDisplayOmitted,
				Effort:  agentcore.ThinkingEffortLow,
			},
		},
		Store: threadStore,
	})

	thread := postChat(t, srv.Handler(), ChatRequest{Message: "hello"})
	if thread.ThreadID == "" {
		t.Fatal("expected thread id")
	}

	var putResp ThreadThinkingResponse
	putJSON(t, srv.Handler(), "/api/threads/"+thread.ThreadID+"/thinking", ThreadThinkingRequest{
		Thinking: &agentcore.ThinkingConfig{
			Display: agentcore.ThinkingDisplaySummarized,
			Effort:  agentcore.ThinkingEffortHigh,
			Budget:  2048,
		},
	}, &putResp, http.StatusOK)
	if putResp.Thinking == nil || putResp.Thinking.Display != agentcore.ThinkingDisplaySummarized {
		t.Fatalf("put response = %#v", putResp)
	}

	var getResp ThreadThinkingResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads/"+thread.ThreadID+"/thinking", &getResp, http.StatusOK)
	if getResp.Thinking == nil || getResp.Thinking.Effort != agentcore.ThinkingEffortHigh {
		t.Fatalf("get response = %#v", getResp)
	}

	resp := postChat(t, srv.Handler(), ChatRequest{Message: "again", ThreadID: thread.ThreadID})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if provider.lastThinking == nil {
		t.Fatal("expected thread thinking config to reach provider")
	}
	if provider.lastThinking.Display != agentcore.ThinkingDisplaySummarized {
		t.Fatalf("display = %q", provider.lastThinking.Display)
	}
	if provider.lastThinking.Effort != agentcore.ThinkingEffortHigh {
		t.Fatalf("effort = %q", provider.lastThinking.Effort)
	}
	if provider.lastThinking.Budget != 2048 {
		t.Fatalf("budget = %d", provider.lastThinking.Budget)
	}

	var threadResp session.ThreadSnapshot
	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads/"+thread.ThreadID, &threadResp, http.StatusOK)
	if threadResp.Thinking == nil || threadResp.Thinking.Display != agentcore.ThinkingDisplaySummarized {
		t.Fatalf("thread thinking = %#v", threadResp.Thinking)
	}
}

func TestServerThreadConfigEndpointsAndRequestOverride(t *testing.T) {
	sessionFS, err := session.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	threadStore := session.NewAgentStore(sessionFS, "/project")
	provider := &captureThinkingProvider{}
	srv := New(agentcore.Config{
		ModelConfig: agentcore.ModelConfig{
			Model:    "server-model",
			Provider: provider,
		},
		SkillConfig: agentcore.SkillConfig{
			AvailableSkills: []skill.Skill{
				{
					Name:        "thread-skill",
					Description: "Thread selected skill",
					FilePath:    "/skills/thread/SKILL.md",
					BaseDir:     "/skills/thread",
					Body:        "Thread skill body",
				},
				{
					Name:        "request-skill",
					Description: "Request selected skill",
					FilePath:    "/skills/request/SKILL.md",
					BaseDir:     "/skills/request",
					Body:        "Request skill body",
				},
			},
		},
		Store: threadStore,
	})

	thread := postChat(t, srv.Handler(), ChatRequest{Message: "hello"})
	if thread.ThreadID == "" {
		t.Fatal("expected thread id")
	}

	var putResp ThreadConfigResponse
	putJSON(t, srv.Handler(), "/api/threads/"+thread.ThreadID+"/config", ThreadConfigRequest{
		Config: &agentcore.CallConfig{
			Model:          "thread-model",
			Skills:         []string{"thread-skill"},
			ResponseFormat: agentcore.NewJSONObjectResponseFormat(),
			Thinking: &agentcore.ThinkingConfig{
				Display: agentcore.ThinkingDisplaySummarized,
			},
		},
	}, &putResp, http.StatusOK)
	if putResp.Config == nil || putResp.Config.Model != "thread-model" {
		t.Fatalf("put response = %#v", putResp)
	}

	var getResp ThreadConfigResponse
	getJSON(t, srv.Handler(), http.MethodGet, "/api/threads/"+thread.ThreadID+"/config", &getResp, http.StatusOK)
	if getResp.Config == nil || getResp.Config.ResponseFormat == nil {
		t.Fatalf("get response = %#v", getResp)
	}
	if len(getResp.Config.Skills) != 1 || getResp.Config.Skills[0] != "thread-skill" {
		t.Fatalf("thread skills = %#v", getResp.Config.Skills)
	}

	resp := postChat(t, srv.Handler(), ChatRequest{Message: "again", ThreadID: thread.ThreadID})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if provider.lastModel != "thread-model" {
		t.Fatalf("model = %q", provider.lastModel)
	}
	if provider.lastResponseFormat == nil || provider.lastResponseFormat.Type != agentcore.ResponseFormatJSONObject {
		t.Fatalf("response format = %#v", provider.lastResponseFormat)
	}
	if provider.lastThinking == nil || provider.lastThinking.Display != agentcore.ThinkingDisplaySummarized {
		t.Fatalf("thinking = %#v", provider.lastThinking)
	}
	if !messagesContain(t, provider.lastMessages, "Thread skill body") {
		t.Fatalf("expected thread skill prompt in messages: %#v", provider.lastMessages)
	}

	resp = postChat(t, srv.Handler(), ChatRequest{
		Message:        "override",
		ThreadID:       thread.ThreadID,
		Model:          "request-model",
		Skills:         []string{"request-skill"},
		ResponseFormat: agentcore.NewJSONSchemaResponseFormat("answer", map[string]any{"type": "object"}),
	})
	if resp.Error != "" {
		t.Fatalf("unexpected error: %s", resp.Error)
	}
	if provider.lastModel != "request-model" {
		t.Fatalf("override model = %q", provider.lastModel)
	}
	if provider.lastResponseFormat == nil || provider.lastResponseFormat.Type != agentcore.ResponseFormatJSONSchema {
		t.Fatalf("override response format = %#v", provider.lastResponseFormat)
	}
	if provider.lastThinking == nil || provider.lastThinking.Display != agentcore.ThinkingDisplaySummarized {
		t.Fatalf("override thinking = %#v", provider.lastThinking)
	}
	if !messagesContain(t, provider.lastMessages, "Request skill body") {
		t.Fatalf("expected request skill prompt in messages: %#v", provider.lastMessages)
	}
	if messagesContain(t, provider.lastMessages, "Thread skill body") {
		t.Fatalf("request skill override should replace thread selection: %#v", provider.lastMessages)
	}
}
