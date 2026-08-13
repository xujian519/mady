package piagent

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func stubTool(name string) *agentcore.Tool {
	return &agentcore.Tool{Name: name, Description: name}
}

func TestPresets_AllDefined(t *testing.T) {
	for _, want := range []string{PresetExplore, PresetVerify, PresetPlan, PresetGeneralPurpose} {
		if p := FindPreset(want); p == nil {
			t.Errorf("preset %q missing", want)
		}
	}
	if FindPreset("nope") != nil {
		t.Error("unknown preset should be nil")
	}
}

func TestPresets_ReadOnlyExcludesWriteTools(t *testing.T) {
	for _, name := range []string{PresetExplore, PresetVerify, PresetPlan} {
		p := FindPreset(name)
		if !p.IsReadOnly {
			t.Errorf("preset %q should be read-only", name)
		}
		for _, tool := range p.AllowedTools {
			if IsWriteTool(tool) {
				t.Errorf("preset %q allows write tool %q", name, tool)
			}
		}
	}
	if p := FindPreset(PresetGeneralPurpose); p.IsReadOnly {
		t.Error("general-purpose should not be read-only")
	}
}

func TestIsWriteTool(t *testing.T) {
	for _, w := range []string{"bash", "edit", "write_file", "delete", "move", "patch", "execute_code", "computer_use"} {
		if !IsWriteTool(w) {
			t.Errorf("IsWriteTool(%q) = false, want true", w)
		}
	}
	for _, r := range []string{"read", "grep", "glob", "ls", "find", "view", "web_search"} {
		if IsWriteTool(r) {
			t.Errorf("IsWriteTool(%q) = true, want false", r)
		}
	}
}

func TestResolveAllowed_PresetPlusExtraMinusExclude(t *testing.T) {
	registered := []*agentcore.Tool{
		stubTool("read"), stubTool("grep"), stubTool("glob"), stubTool("ls"),
		stubTool("find"), stubTool("view"), stubTool("edit"), stubTool("bash"),
		stubTool("spawn_agent"),
	}
	preset := FindPreset(PresetExplore)

	got := ResolveAllowed(registered, preset, nil, nil)
	if len(got) != 6 {
		t.Fatalf("explore default whitelist = %d tools, want 6 (got %v)", len(got), namesOf(got))
	}
	if hasName(got, "edit") || hasName(got, "bash") || hasName(got, "spawn_agent") {
		t.Fatalf("explore whitelist leaks write/nested tools: %v", namesOf(got))
	}

	// tools 参数追加 + exclude 剔除
	got = ResolveAllowed(registered, preset, []string{"edit"}, []string{"grep"})
	if !hasName(got, "edit") {
		t.Errorf("extra tool not appended: %v", namesOf(got))
	}
	if hasName(got, "grep") {
		t.Errorf("exclude not applied: %v", namesOf(got))
	}

	// 未注册的工具名被静默忽略
	got = ResolveAllowed(registered, preset, []string{"no_such_tool"}, nil)
	if hasName(got, "no_such_tool") {
		t.Errorf("unregistered tool leaked: %v", namesOf(got))
	}
}

func TestResolveAllowed_GeneralPurposeInheritsAll(t *testing.T) {
	registered := []*agentcore.Tool{
		stubTool("read"), stubTool("edit"), stubTool("bash"), stubTool("spawn_agent"),
	}
	preset := FindPreset(PresetGeneralPurpose)
	got := ResolveAllowed(registered, preset, nil, nil)
	if len(got) != 3 {
		t.Fatalf("general-purpose = %d tools, want 3 (nested excluded), got %v", len(got), namesOf(got))
	}
	if hasName(got, "spawn_agent") {
		t.Error("nested spawn_agent must be excluded")
	}
}

func TestResolveAllowed_GeneralPurposeExcludeAll(t *testing.T) {
	// 回归：general-purpose + exclude_tools 显式排除后，白名单应为空，
	// 不得回落到「继承全量」分支（历史 bug：排除被忽略）。
	registered := []*agentcore.Tool{
		stubTool("read"), stubTool("edit"), stubTool("bash"), stubTool("spawn_agent"),
	}
	preset := FindPreset(PresetGeneralPurpose)
	got := ResolveAllowed(registered, preset, nil, []string{"read", "edit", "bash"})
	if len(got) != 0 {
		t.Fatalf("exclude all should empty the whitelist, got %v", namesOf(got))
	}
}

func TestResolveAllowed_EmptyWhitelistRejected(t *testing.T) {
	registered := []*agentcore.Tool{stubTool("read")}
	preset := FindPreset(PresetExplore)
	// exclude 全部白名单 → 空
	got := ResolveAllowed(registered, preset, nil, []string{"read", "grep", "glob", "ls", "find", "view"})
	if len(got) != 0 {
		t.Fatalf("expected empty whitelist, got %v", namesOf(got))
	}
}

func namesOf(tools []*agentcore.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

func hasName(tools []*agentcore.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}
