package provisions

import (
	"strings"
	"testing"
)

func TestLoadRoles_AllParsed(t *testing.T) {
	roles, err := LoadRoles()
	if err != nil {
		t.Fatalf("LoadRoles: %v", err)
	}
	if len(roles) < 11 {
		t.Fatalf("expected >=11 roles, got %d", len(roles))
	}
	// 字段完备性：核心三字段在全部角色非空。
	for _, r := range roles {
		if r.RoleID == "" || r.Name == "" || r.Identity == "" {
			t.Errorf("role 字段不完整: %+v", r)
		}
	}
}

func TestLoadRoles_OrchestratorPresent(t *testing.T) {
	roles, err := LoadRoles()
	if err != nil {
		t.Fatalf("LoadRoles: %v", err)
	}
	found := false
	for _, r := range roles {
		if r.RoleID == "patent_orchestrator" {
			found = true
			if len(r.PrimaryTools) == 0 {
				t.Error("patent_orchestrator 应声明 primary_tools")
			}
			if len(r.Methodology) == 0 {
				t.Error("patent_orchestrator 应声明 methodology 步骤")
			}
		}
	}
	if !found {
		t.Error("patent_orchestrator 角色缺失")
	}
}

func TestBuildRoleListForSystemPrompt(t *testing.T) {
	roles := []Role{
		{RoleID: "writer", Name: "撰写员"},
		{RoleID: "reviewer", Name: "审核员"},
	}
	s := BuildRoleListForSystemPrompt(roles)
	if !strings.Contains(s, "撰写员") || !strings.Contains(s, "writer") {
		t.Errorf("角色目录应含名称与 role_id, got: %q", s)
	}
	if got := BuildRoleListForSystemPrompt(nil); got != "" {
		t.Errorf("空角色应返回空串, got %q", got)
	}
}
