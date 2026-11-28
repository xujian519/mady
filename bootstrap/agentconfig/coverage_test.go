package agentconfig_test

// M69 补测：bootstrap/agentconfig 的纯配置解析面（Provider 目录、模型目录、
// 上下文窗口、思考配置、角色 YAML 与校验）。该包此前仅 46.2% 覆盖，
// 未覆盖部分全是「无副作用的环境变量/文件解析」，是低风险高收益的补测区。

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/xujian519/mady/bootstrap/agentconfig"
)

func TestProviderCatalog(t *testing.T) {
	catalog := agentconfig.ProviderCatalog()
	if len(catalog) != 4 {
		t.Fatalf("Provider 数量 = %d，期望 4", len(catalog))
	}
	wantNames := []string{"deepseek", "zhipu", "kimi", "generic"}
	seen := make(map[string]bool, len(catalog))
	for i, p := range catalog {
		if p.Name != wantNames[i] {
			t.Errorf("第 %d 个 Provider = %q，期望 %q", i, p.Name, wantNames[i])
		}
		if seen[p.Name] {
			t.Errorf("Provider 名称重复：%q", p.Name)
		}
		seen[p.Name] = true
		if p.Label == "" {
			t.Errorf("Provider %q 缺少中文 Label", p.Name)
		}
		if p.Name == "generic" {
			if p.Models != nil {
				t.Errorf("generic 的模型应由 MODEL 环境变量动态加载，不应静态声明")
			}
			continue
		}
		if len(p.Models) == 0 {
			t.Errorf("Provider %q 未声明任何模型", p.Name)
		}
		modelSeen := make(map[string]bool, len(p.Models))
		for _, m := range p.Models {
			if m.Name == "" || m.Label == "" || m.Description == "" {
				t.Errorf("Provider %q 的模型字段不完整：%#v", p.Name, m)
			}
			if m.Group != "recommended" && m.Group != "all" {
				t.Errorf("Provider %q 模型 %q 的 Group = %q，只允许 recommended/all", p.Name, m.Name, m.Group)
			}
			if modelSeen[m.Name] {
				t.Errorf("Provider %q 模型重复：%q", p.Name, m.Name)
			}
			modelSeen[m.Name] = true
		}
	}
}

func TestModelsForProvider(t *testing.T) {
	t.Run("固定目录 Provider", func(t *testing.T) {
		models := agentconfig.ModelsForProvider("deepseek")
		if len(models) != 2 || models[0].Name != "deepseek-v4-flash" {
			t.Fatalf("deepseek 模型 = %#v", models)
		}
		if got := agentconfig.ModelsForProvider("unknown-provider"); got != nil {
			t.Fatalf("未注册 Provider 应返回 nil，实际 %#v", got)
		}
	})

	t.Run("generic 读 MODEL 环境变量", func(t *testing.T) {
		t.Setenv("MODEL", "my-openai-model")
		models := agentconfig.ModelsForProvider("generic")
		if len(models) != 1 {
			t.Fatalf("generic 模型数量 = %d，期望 1", len(models))
		}
		if models[0].Name != "my-openai-model" || models[0].Group != "recommended" {
			t.Fatalf("generic 模型 = %#v", models[0])
		}
	})

	t.Run("generic 未配置 MODEL 时给出占位项", func(t *testing.T) {
		t.Setenv("MODEL", "")
		models := agentconfig.ModelsForProvider("generic")
		if len(models) != 1 {
			t.Fatalf("generic 模型数量 = %d，期望 1", len(models))
		}
		if models[0].Name != "(未配置)" || models[0].Group != "all" {
			t.Fatalf("generic 占位模型 = %#v", models[0])
		}
	})
}

func TestDefaultModelForProvider(t *testing.T) {
	cases := []struct{ provider, want string }{
		{"deepseek", "deepseek-v4-flash"},
		{"zhipu", "glm-5.2"},
		{"kimi", "kimi-k2.6"},
	}
	for _, c := range cases {
		if got := agentconfig.DefaultModelForProvider(c.provider); got != c.want {
			t.Errorf("DefaultModelForProvider(%q) = %q，期望 %q", c.provider, got, c.want)
		}
	}
	t.Run("generic 回退 MODEL 环境变量", func(t *testing.T) {
		t.Setenv("MODEL", "custom-model")
		if got := agentconfig.DefaultModelForProvider("generic"); got != "custom-model" {
			t.Fatalf("generic 默认模型 = %q，期望 custom-model", got)
		}
	})
}

func TestProviderNameLabelAndList(t *testing.T) {
	if got := agentconfig.ProviderNameLabel("deepseek"); got != "DeepSeek" {
		t.Errorf("ProviderNameLabel(deepseek) = %q", got)
	}
	if got := agentconfig.ProviderNameLabel("kimi"); got != "Kimi 月之暗面" {
		t.Errorf("ProviderNameLabel(kimi) = %q", got)
	}
	// 未注册名称原样返回，便于 TUI 直接展示用户输入。
	if got := agentconfig.ProviderNameLabel("unknown"); got != "unknown" {
		t.Errorf("未注册 Provider 应原样返回，实际 %q", got)
	}

	want := []string{"deepseek", "zhipu", "kimi", "generic"}
	if got := agentconfig.ProviderNameList(); !slices.Equal(got, want) {
		t.Fatalf("ProviderNameList = %v，期望 %v", got, want)
	}
}

func TestToolDomainSet(t *testing.T) {
	ts := agentconfig.ToolDomainSet{Primary: []string{"search", "document"}, Secondary: []string{"document", "analysis"}}

	domains := ts.AllDomains()
	sort.Strings(domains)
	if want := []string{"analysis", "document", "search"}; !slices.Equal(domains, want) {
		t.Fatalf("AllDomains = %v，期望去重后的 %v", domains, want)
	}
	if !ts.HasDomain("search") {
		t.Error("Primary 域应命中")
	}
	if !ts.HasDomain("analysis") {
		t.Error("Secondary 域应命中")
	}
	if ts.HasDomain("browser") {
		t.Error("未声明的域不应命中")
	}
}

func TestParseRoleYAML(t *testing.T) {
	t.Run("格式 A roles 命名空间", func(t *testing.T) {
		data := []byte(`
roles:
  retriever:
    description: "专利检索与分析员"
    temperature: 0.5
    max_tokens: 8192
    tool_domains:
      primary: [search]
      secondary: [document]
`)
		set, err := agentconfig.ParseRoleYAML(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(set) != 1 {
			t.Fatalf("角色数量 = %d，期望 1", len(set))
		}
		r, ok := set.Get("retriever")
		if !ok {
			t.Fatal("未解析出 retriever")
		}
		if r.ID != "retriever" {
			t.Errorf("ID = %q，期望回填为 map key", r.ID)
		}
		if r.Temperature != 0.5 || r.MaxTokens != 8192 {
			t.Errorf("数值字段解析错误：%#v", r)
		}
		if !r.ToolDomains.HasDomain("document") {
			t.Errorf("tool_domains.secondary 未解析：%#v", r.ToolDomains)
		}
	})

	t.Run("格式 B 扁平结构", func(t *testing.T) {
		set, err := agentconfig.ParseRoleYAML([]byte("novelty_checker:\n  description: \"新颖性审查员\"\n"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := set.Get("novelty_checker"); !ok {
			t.Fatalf("扁平格式未解析出角色：%#v", set)
		}
	})

	t.Run("非法 YAML 返回错误", func(t *testing.T) {
		if _, err := agentconfig.ParseRoleYAML([]byte("roles: [not-a-map")); err == nil {
			t.Fatal("期望解析错误")
		}
	})
}

func TestNewRoleSetFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roles.yaml")
	if err := os.WriteFile(path, []byte("roles:\n  writer:\n    description: \"撰写员\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	set, err := agentconfig.NewRoleSet(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := set.Get("writer"); !ok {
		t.Fatalf("未加载出 writer：%#v", set)
	}

	if _, err := agentconfig.NewRoleSet(filepath.Join(dir, "missing.yaml")); err == nil {
		t.Fatal("文件不存在时应返回错误")
	}
}

func TestRoleSetGetIDsMerge(t *testing.T) {
	var nilSet agentconfig.RoleSet
	if _, ok := nilSet.Get("x"); ok {
		t.Error("nil RoleSet 的 Get 应返回 false")
	}
	if ids := nilSet.IDs(); ids != nil {
		t.Errorf("nil RoleSet 的 IDs 应为 nil，实际 %v", ids)
	}
	nilSet.Merge(agentconfig.RoleSet{"x": {}}) // 空操作，不得 panic

	set := agentconfig.RoleSet{
		"writer":  {ID: "writer", Description: "旧"},
		"checker": {ID: "checker"},
	}
	if ids := set.IDs(); !slices.Equal(ids, []string{"checker", "writer"}) {
		t.Fatalf("IDs 未排序：%v", ids)
	}
	set.Merge(agentconfig.RoleSet{
		"writer": {ID: "writer", Description: "新"},
		"editor": {ID: "editor"},
	})
	if r, _ := set.Get("writer"); r.Description != "新" {
		t.Errorf("同 ID 角色未覆盖：%#v", r)
	}
	if len(set) != 3 {
		t.Errorf("Merge 后角色数量 = %d，期望 3", len(set))
	}
}

func TestRoleConfigValidate(t *testing.T) {
	valid := agentconfig.RoleConfig{
		ID:          "retriever",
		Temperature: 0.5,
		MaxTokens:   1024,
		ToolDomains: agentconfig.ToolDomainSet{Primary: []string{"search"}, Secondary: []string{"document"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("合法配置被拒：%v", err)
	}

	cases := []struct {
		name string
		cfg  agentconfig.RoleConfig
	}{
		{"ID 为空", agentconfig.RoleConfig{}},
		{"temperature 为负", agentconfig.RoleConfig{ID: "r", Temperature: -0.1}},
		{"temperature 超上限", agentconfig.RoleConfig{ID: "r", Temperature: 2.1}},
		{"max_tokens 为负", agentconfig.RoleConfig{ID: "r", MaxTokens: -1}},
		{"primary 域为空白", agentconfig.RoleConfig{ID: "r", ToolDomains: agentconfig.ToolDomainSet{Primary: []string{"  "}}}},
		{"secondary 域为空白", agentconfig.RoleConfig{ID: "r", ToolDomains: agentconfig.ToolDomainSet{Secondary: []string{""}}}},
	}
	for _, c := range cases {
		if err := c.cfg.Validate(); err == nil {
			t.Errorf("%s：期望校验失败", c.name)
		}
	}

	// ValidateAll 以 map key 回填 ID，键本身为空时校验失败。
	if err := (agentconfig.RoleSet{"": {}}).ValidateAll(); err == nil {
		t.Error("空 map key 应在 ValidateAll 中失败")
	}
	if err := (agentconfig.RoleSet{"ok": {Temperature: 0.2}}).ValidateAll(); err != nil {
		t.Errorf("合法集合被拒：%v", err)
	}
}

func TestResolveContextWindow(t *testing.T) {
	cases := []struct {
		model string
		want  int64
	}{
		{"deepseek-v4-flash", 1_000_000},
		{"DeepSeek-V4-Pro", 1_000_000},
		{"kimi-k2.6", 1_000_000},
		{"  glm-5.2  ", 1_000_000},
		{"glm-5v-turbo", 1_000_000},
		{"gpt-4o", 256_000},
		{"", 256_000},
		// 前缀必须带分隔符：避免 "glm-50" 这类误匹配。
		{"glm-50", 256_000},
		{"deepseek-v3", 256_000},
	}
	for _, c := range cases {
		if got := agentconfig.ResolveContextWindow(c.model); got != c.want {
			t.Errorf("ResolveContextWindow(%q) = %d，期望 %d", c.model, got, c.want)
		}
	}
}

func TestDefaultModel(t *testing.T) {
	cases := []struct{ provider, want string }{
		{"deepseek", "deepseek-v4-flash"},
		{"zhipu", "glm-5.2"},
		{"kimi", "kimi-k2.6"},
	}
	for _, c := range cases {
		t.Setenv("PROVIDER", c.provider)
		if got := agentconfig.DefaultModel(); got != c.want {
			t.Errorf("PROVIDER=%q 时 DefaultModel = %q，期望 %q", c.provider, got, c.want)
		}
	}

	t.Setenv("PROVIDER", "")
	if got := agentconfig.DefaultModel(); got != "deepseek-v4-flash" {
		t.Errorf("PROVIDER 未设置时应默认 deepseek，实际 %q", got)
	}

	t.Setenv("PROVIDER", "generic")
	t.Setenv("MODEL", "my-model")
	if got := agentconfig.DefaultModel(); got != "my-model" {
		t.Errorf("generic 应回退 MODEL，实际 %q", got)
	}
}

func TestThinkingFromEnv(t *testing.T) {
	t.Run("全未设置返回 nil", func(t *testing.T) {
		for _, k := range []string{"THINKING_INCLUDE_THOUGHTS", "THINKING_DISPLAY", "THINKING_EFFORT", "THINKING_BUDGET"} {
			t.Setenv(k, "")
		}
		if cfg := agentconfig.ThinkingFromEnv(); cfg != nil {
			t.Fatalf("未设置任何变量应返回 nil，实际 %#v", cfg)
		}
	})

	t.Run("全部设置时解析", func(t *testing.T) {
		t.Setenv("THINKING_INCLUDE_THOUGHTS", "true")
		t.Setenv("THINKING_DISPLAY", "SUMMARIZED")
		t.Setenv("THINKING_EFFORT", "High")
		t.Setenv("THINKING_BUDGET", "4096")
		cfg := agentconfig.ThinkingFromEnv()
		if cfg == nil {
			t.Fatal("期望解析出配置")
		}
		if !cfg.IncludeThoughts || cfg.Display != "summarized" || cfg.Effort != "high" || cfg.Budget != 4096 {
			t.Fatalf("解析结果 = %#v", cfg)
		}
	})

	t.Run("非法数值静默忽略但保留配置", func(t *testing.T) {
		t.Setenv("THINKING_INCLUDE_THOUGHTS", "not-a-bool")
		t.Setenv("THINKING_DISPLAY", "")
		t.Setenv("THINKING_EFFORT", "")
		t.Setenv("THINKING_BUDGET", "not-a-number")
		cfg := agentconfig.ThinkingFromEnv()
		if cfg == nil {
			t.Fatal("至少设置了变量时应返回非 nil")
		}
		if cfg.IncludeThoughts || cfg.Budget != 0 {
			t.Fatalf("非法值应回退零值，实际 %#v", cfg)
		}
	})
}
