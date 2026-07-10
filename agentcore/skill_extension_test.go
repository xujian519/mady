package agentcore

import (
	"context"
	"strings"
	"testing"

	"github.com/xujian519/mady/skill"
)

type modelSelectSkillProvider struct {
	requests []*ProviderRequest
}

func (p *modelSelectSkillProvider) Complete(ctx context.Context, req *ProviderRequest) (*ProviderResponse, error) {
	cp := *req
	cp.Messages = append([]Message(nil), req.Messages...)
	p.requests = append(p.requests, &cp)
	if len(p.requests) == 1 {
		return &ProviderResponse{Content: "/skill:planner gather requirements"}, nil
	}
	return &ProviderResponse{Content: "final answer"}, nil
}

func (p *modelSelectSkillProvider) Stream(ctx context.Context, req *ProviderRequest) (<-chan StreamDelta, error) {
	ch := make(chan StreamDelta)
	close(ch)
	return ch, nil
}

func TestMergeCallConfig_OverridesSkills(t *testing.T) {
	base := &CallConfig{
		Model:  "base",
		Skills: []string{"planner"},
	}
	override := &CallConfig{
		Model:  "override",
		Skills: []string{"debugger", "writer"},
	}
	merged := MergeCallConfig(base, override)
	if merged.Model != "override" {
		t.Fatalf("model = %q", merged.Model)
	}
	if len(merged.Skills) != 2 || merged.Skills[0] != "debugger" || merged.Skills[1] != "writer" {
		t.Fatalf("skills = %#v", merged.Skills)
	}
	override.Skills[0] = "changed"
	if merged.Skills[0] != "debugger" {
		t.Fatalf("expected cloned skills slice, got %#v", merged.Skills)
	}
}

func TestAgentRun_ExpandsSkillsInPromptAndExplicitInvocation(t *testing.T) {
	provider := &captureStructuredProvider{}
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:     "skills",
			Model:    "stub",
			Provider: provider,
		},
		SkillConfig: SkillConfig{
			AvailableSkills: []skill.Skill{
				{
					Name:        "planner",
					Description: "Plans work",
					FilePath:    "/skills/planner/SKILL.md",
					BaseDir:     "/skills/planner",
					Body:        "Plan carefully.",
				},
				{
					Name:                   "debugger",
					Description:            "Debugs failures",
					FilePath:               "/skills/debugger/SKILL.md",
					BaseDir:                "/skills/debugger",
					Body:                   "Inspect logs first.",
					DisableModelInvocation: true,
				},
			},
			SelectedSkills: []string{"planner"},
		},
	})
	var loaded []SkillLoadedEvent
	agent.On(EventSkillLoaded, func(e Event) {
		ev, ok := e.(SkillLoadedEvent)
		if ok {
			loaded = append(loaded, ev)
		}
		if ev, ok := e.(*SkillLoadedEvent); ok {
			loaded = append(loaded, *ev)
		}
	})

	if _, err := agent.Run(context.Background(), "/skill:debugger trace the outage"); err != nil {
		t.Fatal(err)
	}
	if provider.lastRequest == nil {
		t.Fatal("expected captured request")
	}
	var joined []string
	for _, msg := range provider.lastRequest.Messages {
		joined = append(joined, msg.Content)
	}
	body := strings.Join(joined, "\n---\n")
	if !strings.Contains(body, "<available_skills>") {
		t.Fatalf("missing skill index in request: %s", body)
	}
	if !strings.Contains(body, "<active_skills>") || !strings.Contains(body, "Plan carefully.") {
		t.Fatalf("missing active skill prompt: %s", body)
	}
	if !strings.Contains(body, `<skill name="debugger"`) || !strings.Contains(body, "User: trace the outage") {
		t.Fatalf("missing explicit skill expansion: %s", body)
	}
	if len(loaded) != 1 || loaded[0].SkillName != "debugger" || loaded[0].Source != "explicit_command" {
		t.Fatalf("loaded events = %#v", loaded)
	}
}

func TestSkillExtension_ToolFiltering(t *testing.T) {
	// Skills with AllowedTools restrictions should filter tool definitions
	// visible to the model.

	baseTools := []ToolDefinition{
		{Name: "web_search", Description: "Search the web"},
		{Name: "web_fetch", Description: "Fetch a URL"},
		{Name: "read", Description: "Read a file"},
		{Name: "write_file", Description: "Write a file"},
		{Name: "bash", Description: "Run shell commands"},
	}

	t.Run("no restrictions allows all tools", func(t *testing.T) {
		ext := &skillExtension{
			skills: []skill.Skill{
				{
					Name:         "chat-assistant",
					Description:  "Chat",
					AllowedTools: nil, // no restrictions
				},
			},
			selected: []string{"chat-assistant"},
		}

		mcc := &ModelCallContext{
			Request: &ProviderRequest{
				Tools: append([]ToolDefinition{}, baseTools...),
			},
		}

		err := ext.BeforeModelCall(context.Background(), nil, mcc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mcc.Request.Tools) != 5 {
			t.Errorf("no restrictions: expected 5 tools, got %d", len(mcc.Request.Tools))
		}
	})

	t.Run("allowed tools restrict visible tools", func(t *testing.T) {
		ext := &skillExtension{
			skills: []skill.Skill{
				{
					Name:         "chat-assistant",
					Description:  "Chat",
					AllowedTools: []string{"web_search", "web_fetch", "read", "write_file", "bash"},
				},
			},
			selected: []string{"chat-assistant"},
		}

		mcc := &ModelCallContext{
			Request: &ProviderRequest{
				Tools: append([]ToolDefinition{}, baseTools...),
			},
		}

		err := ext.BeforeModelCall(context.Background(), nil, mcc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mcc.Request.Tools) != 5 {
			t.Errorf("all allowed: expected 5 tools, got %d", len(mcc.Request.Tools))
		}
	})

	t.Run("restricted subset filters tools", func(t *testing.T) {
		ext := &skillExtension{
			skills: []skill.Skill{
				{
					Name:         "patent-agent",
					Description:  "Patent",
					AllowedTools: []string{"web_search", "read", "write_file"},
				},
			},
			selected: []string{"patent-agent"},
		}

		mcc := &ModelCallContext{
			Request: &ProviderRequest{
				Tools: append([]ToolDefinition{}, baseTools...),
			},
		}

		err := ext.BeforeModelCall(context.Background(), nil, mcc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mcc.Request.Tools) != 3 {
			t.Fatalf("expected 3 tools, got %d", len(mcc.Request.Tools))
		}

		names := make(map[string]bool)
		for _, td := range mcc.Request.Tools {
			names[td.Name] = true
		}
		if !names["web_search"] || !names["read"] || !names["write_file"] {
			t.Errorf("unexpected tool names: %v", names)
		}
		if names["web_fetch"] || names["bash"] {
			t.Errorf("disallowed tools present: %v", names)
		}
	})

	t.Run("multiple skills union allows tools", func(t *testing.T) {
		ext := &skillExtension{
			skills: []skill.Skill{
				{
					Name:         "skill-a",
					AllowedTools: []string{"web_search", "read"},
				},
				{
					Name:         "skill-b",
					AllowedTools: []string{"bash", "write_file"},
				},
			},
			selected: []string{"skill-a", "skill-b"},
		}

		mcc := &ModelCallContext{
			Request: &ProviderRequest{
				Tools: append([]ToolDefinition{}, baseTools...),
			},
		}

		err := ext.BeforeModelCall(context.Background(), nil, mcc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mcc.Request.Tools) != 4 {
			t.Fatalf("expected 4 tools (union), got %d", len(mcc.Request.Tools))
		}

		names := make(map[string]bool)
		for _, td := range mcc.Request.Tools {
			names[td.Name] = true
		}
		if !names["web_search"] || !names["read"] || !names["bash"] || !names["write_file"] {
			t.Errorf("expected union of tools, got: %v", names)
		}
	})

	t.Run("no active skills allows all tools", func(t *testing.T) {
		ext := &skillExtension{
			skills: []skill.Skill{
				{
					Name:         "planner",
					AllowedTools: []string{"read"},
				},
			},
			selected: nil, // no active selection
		}

		mcc := &ModelCallContext{
			Request: &ProviderRequest{
				Tools: append([]ToolDefinition{}, baseTools...),
			},
		}

		err := ext.BeforeModelCall(context.Background(), nil, mcc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mcc.Request.Tools) != 5 {
			t.Errorf("no active skills: expected all 5 tools, got %d", len(mcc.Request.Tools))
		}
	})

	t.Run("nil request is safe", func(t *testing.T) {
		ext := &skillExtension{
			skills: []skill.Skill{
				{Name: "x", AllowedTools: []string{"read"}},
			},
			selected: []string{"x"},
		}

		err := ext.BeforeModelCall(context.Background(), nil, &ModelCallContext{Request: nil})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
func TestAgentRun_ModelSelectedSkillTriggersSecondTurn(t *testing.T) {
	provider := &modelSelectSkillProvider{}
	agent := New(Config{
		ModelConfig: ModelConfig{
			Name:     "skills",
			Model:    "stub",
			Provider: provider,
		},
		SkillConfig: SkillConfig{
			AvailableSkills: []skill.Skill{
				{
					Name:        "planner",
					Description: "Plans work",
					FilePath:    "/skills/planner/SKILL.md",
					BaseDir:     "/skills/planner",
					Body:        "Plan carefully.",
				},
			},
		},
	})
	var loaded []SkillLoadedEvent
	agent.On(EventSkillLoaded, func(e Event) {
		ev, ok := e.(SkillLoadedEvent)
		if ok {
			loaded = append(loaded, ev)
		}
		if ev, ok := e.(*SkillLoadedEvent); ok {
			loaded = append(loaded, *ev)
		}
	})

	out, err := agent.Run(context.Background(), "help me plan")
	if err != nil {
		t.Fatal(err)
	}
	if out != "final answer" {
		t.Fatalf("output = %q", out)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("request count = %d", len(provider.requests))
	}
	var joined []string
	for _, msg := range provider.requests[1].Messages {
		joined = append(joined, msg.Content)
	}
	secondTurn := strings.Join(joined, "\n---\n")
	if !strings.Contains(secondTurn, "Plan carefully.") || !strings.Contains(secondTurn, "User: gather requirements") {
		t.Fatalf("second turn missing loaded skill: %s", secondTurn)
	}

	for _, msg := range agent.State().Messages() {
		if msg.Role == RoleAssistant && strings.Contains(msg.Content, "/skill:planner") {
			t.Fatalf("intermediate skill command should not persist: %#v", msg)
		}
	}
	if len(loaded) != 1 || loaded[0].SkillName != "planner" || loaded[0].Source != skillMetadataSourceModel {
		t.Fatalf("loaded events = %#v", loaded)
	}
}
