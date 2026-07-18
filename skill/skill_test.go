package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_RecursiveDiscoveryAndCollisions(t *testing.T) {
	root := t.TempDir()
	mustWriteSkill(t, filepath.Join(root, "alpha", "SKILL.md"), `---
name: alpha
description: First skill
---
Alpha body`)
	mustWriteSkill(t, filepath.Join(root, "nested", "beta", "SKILL.md"), `---
name: beta
description: Beta skill
disable-model-invocation: true
---
Beta body`)
	mustWriteSkill(t, filepath.Join(root, "collision", "SKILL.md"), `---
name: alpha
description: Duplicate alpha
---
Other body`)
	mustWriteSkill(t, filepath.Join(root, "skip", "SKILL.md"), `---
name: skip
---
No description`)
	mustWriteSkill(t, filepath.Join(root, "alpha", "child", "SKILL.md"), `---
name: child
description: Should not be discovered because parent is a skill root
---
child`)

	skills, diagnostics, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("skills len = %d", len(skills))
	}
	if _, found := FindByName(skills, "child"); found {
		t.Fatal("unexpected nested child skill under existing root")
	}
	var collisionWarn, missingDescWarn bool
	for _, diag := range diagnostics {
		if strings.Contains(diag.Message, "collides") {
			collisionWarn = true
		}
		if strings.Contains(diag.Message, "missing required description") {
			missingDescWarn = true
		}
	}
	if !collisionWarn {
		t.Fatal("expected collision diagnostic")
	}
	if !missingDescWarn {
		t.Fatal("expected missing description diagnostic")
	}
}

func TestIndexAndExplicitInvocation(t *testing.T) {
	skills := []Skill{
		{Name: "visible", Description: "Shown", FilePath: "/tmp/visible/SKILL.md", BaseDir: "/tmp/visible", Body: "Visible body"},
		{Name: "hidden", Description: "Hidden", FilePath: "/tmp/hidden/SKILL.md", BaseDir: "/tmp/hidden", Body: "Hidden body", DisableModelInvocation: true},
	}
	index := Index(skills)
	if !strings.Contains(index, "<name>visible</name>") {
		t.Fatalf("index = %q", index)
	}
	if strings.Contains(index, "hidden") {
		t.Fatalf("hidden skill should not appear in index: %q", index)
	}
	invocation := ExplicitInvocation(skills[1], "run it now")
	if !strings.Contains(invocation, `<skill name="hidden"`) {
		t.Fatalf("invocation = %q", invocation)
	}
	if !strings.Contains(invocation, "User: run it now") {
		t.Fatalf("invocation args = %q", invocation)
	}
}

func TestParseCommand(t *testing.T) {
	cmd, ok := ParseCommand(" /skill:debug investigate flaky test ")
	if !ok {
		t.Fatal("expected command to parse")
	}
	if cmd.Name != "debug" || cmd.Args != "investigate flaky test" {
		t.Fatalf("command = %#v", cmd)
	}
	if _, ok := ParseCommand("/thinking summarized"); ok {
		t.Fatal("unexpected non-skill command parse")
	}
}

func TestLoad_WarnsOnSpecValidationIssues(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "MismatchDir")
	longDescription := strings.Repeat("x", maxSkillDescriptionLength+1)
	mustWriteSkill(t, filepath.Join(dir, "SKILL.md"), `---
name: Invalid--Name-
description: `+longDescription+`
---
Body`)

	skills, diagnostics, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills len = %d", len(skills))
	}
	var parentWarn, charsetWarn, hyphenWarn, longDescWarn bool
	for _, diag := range diagnostics {
		switch {
		case strings.Contains(diag.Message, "does not match parent directory"):
			parentWarn = true
		case strings.Contains(diag.Message, "invalid characters"):
			charsetWarn = true
		case strings.Contains(diag.Message, "start or end with a hyphen"),
			strings.Contains(diag.Message, "consecutive hyphens"):
			hyphenWarn = true
		case strings.Contains(diag.Message, "description exceeds"):
			longDescWarn = true
		}
	}
	if !parentWarn || !charsetWarn || !hyphenWarn || !longDescWarn {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestActivePrompt_ReadsBodyOnDemand(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "planner", "SKILL.md")
	mustWriteSkill(t, path, `---
name: planner
description: Planning skill
---
Plan carefully.
Use references/guide.md when needed.`)

	skills, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills len = %d", len(skills))
	}
	item := skills[0]
	item.Body = ""

	prompt := ActivePrompt([]Skill{item})
	if !strings.Contains(prompt, "Plan carefully.") || !strings.Contains(prompt, "references/guide.md") {
		t.Fatalf("prompt = %q", prompt)
	}

	invocation := ExplicitInvocation(item, "do it")
	if !strings.Contains(invocation, "Plan carefully.") || !strings.Contains(invocation, "User: do it") {
		t.Fatalf("invocation = %q", invocation)
	}
}

func mustWriteSkill(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_DotAgentDirectoryNotSkipped(t *testing.T) {
	// .agent directories should be scanned for SKILL.md files.
	// Other dot-directories should still be skipped.
	root := t.TempDir()

	// .agent directory with SKILL.md — should be discovered
	mustWriteSkill(t, filepath.Join(root, ".agent", "my-agent", "SKILL.md"), `---
name: my-agent
description: A skill inside .agent directory
---
Agent body`)

	// .hidden directory with SKILL.md — should be skipped
	mustWriteSkill(t, filepath.Join(root, ".hidden", "secret", "SKILL.md"), `---
name: secret
description: Should not be discovered
---
Secret body`)

	skills, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, found := FindByName(skills, "my-agent"); !found {
		t.Fatal("expected skill from .agent directory to be discovered")
	}
	if _, found := FindByName(skills, "secret"); found {
		t.Fatal("skill from .hidden directory should NOT be discovered")
	}
}

func TestLoad_MadyExtension(t *testing.T) {
	root := t.TempDir()
	mustWriteSkill(t, filepath.Join(root, "patent-analysis", "SKILL.md"), `---
name: patent-analysis
description: Patent deep analysis skill
allowed-tools:
  - web_search
  - patent_search

mady:
  mode: patent
  guardrail_level: standard
  approval_required: true
  inputs:
    - name: patent_number
      type: string
      required: true
      label: 专利号
    - name: analysis_type
      type: enum
      values:
        - novelty
        - infringement
        - validity
      default: novelty
      label: 分析类型
  example_prompt: "Analyze patent CN109690000A for novelty"
  example_prompt_zh: "分析专利 CN109690000A 的权利要求新颖性"
  capabilities:
    - patent_search
    - reasoning
    - approval_gate
  handoff_allowed: true
---
Perform step-by-step patent analysis.`)

	skills, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	item := skills[0]
	if item.Mady == nil {
		t.Fatal("expected Mady extension to be parsed")
	}
	ext := item.Mady
	if ext.Mode != "patent" {
		t.Fatalf("mode = %q", ext.Mode)
	}
	if ext.GuardrailLevel != "standard" {
		t.Fatalf("guardrail_level = %q", ext.GuardrailLevel)
	}
	if !ext.ApprovalRequired {
		t.Fatal("approval_required should be true")
	}
	if len(ext.Inputs) != 2 {
		t.Fatalf("inputs len = %d", len(ext.Inputs))
	}
	if ext.Inputs[0].Name != "patent_number" || ext.Inputs[0].Type != "string" || !ext.Inputs[0].Required {
		t.Fatalf("input[0] = %+v", ext.Inputs[0])
	}
	if ext.Inputs[1].Name != "analysis_type" || ext.Inputs[1].Type != "enum" || len(ext.Inputs[1].Values) != 3 {
		t.Fatalf("input[1] = %+v", ext.Inputs[1])
	}
	if ext.ExamplePrompt != "Analyze patent CN109690000A for novelty" {
		t.Fatalf("example_prompt = %q", ext.ExamplePrompt)
	}
	if ext.ExamplePromptZh != "分析专利 CN109690000A 的权利要求新颖性" {
		t.Fatalf("example_prompt_zh = %q", ext.ExamplePromptZh)
	}
	if len(ext.Capabilities) != 3 {
		t.Fatalf("capabilities len = %d", len(ext.Capabilities))
	}
	if !ext.HandoffAllowed {
		t.Fatal("handoff_allowed should be true")
	}
}

func TestLoad_MadyExtensionAbsent(t *testing.T) {
	root := t.TempDir()
	mustWriteSkill(t, filepath.Join(root, "plain-skill", "SKILL.md"), `---
name: plain-skill
description: Skill without Mady extensions
---
Just a regular skill.`)

	skills, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Mady != nil {
		t.Fatal("expected nil Mady extension for plain skill")
	}
}

func TestLoad_MadyExtensionPartialFields(t *testing.T) {
	root := t.TempDir()
	mustWriteSkill(t, filepath.Join(root, "minimal", "SKILL.md"), `---
name: minimal
description: Minimal skill with partial Mady extension
mady:
  mode: chat
---
Minimal body.`)

	skills, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	item := skills[0]
	if item.Mady == nil {
		t.Fatal("expected Mady extension")
	}
	if item.Mady.Mode != "chat" {
		t.Fatalf("mode = %q", item.Mady.Mode)
	}
	// Other fields should be zero values.
	if item.Mady.ApprovalRequired {
		t.Fatal("approval_required should default to false")
	}
}
