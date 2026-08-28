package provisions

import (
	"strings"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

func TestLoadManifest(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatalf("LoadManifest() 失败: %v", err)
	}
	if m == nil {
		t.Fatal("LoadManifest() 返回 nil")
	}
	if len(m.Provisions) == 0 {
		t.Fatal("manifest 中 provisions 为空")
	}
	if len(m.Reasoning) == 0 {
		t.Fatal("manifest 中 reasoning 为空")
	}
	t.Logf("已加载 %d 个条款智能体, %d 个推理模式", len(m.Provisions), len(m.Reasoning))
}

func TestValidateManifest(t *testing.T) {
	m := LoadManifestOrDefault("")
	ok, missing := ValidateManifest(m)
	if !ok {
		t.Logf("Manifest 未覆盖全部条款簇，缺失: %v（非阻塞警告——仅高频 9 条已注册）", missing)
	} else {
		t.Log("Manifest 覆盖全部 22 个条款簇")
	}
}

func TestPreRegisteredProvisions(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, p := range m.Provisions {
		if p.PreRegister {
			count++
		}
	}
	t.Logf("预注册的条款智能体: %d 个", count)
	if count < 9 {
		t.Errorf("预期至少 9 个预注册条款，实际 %d", count)
	}
}

func TestPreRegisteredReasoning(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, r := range m.Reasoning {
		if r.PreRegister {
			count++
		}
	}
	t.Logf("预注册的推理模式: %d 个", count)
	if count < 6 {
		t.Errorf("预期至少 6 个预注册推理模式，实际 %d", count)
	}
}

func TestBuildProvisionSystemPrompt(t *testing.T) {
	entry := &ProvisionManifestEntry{
		ID:     "P-A01",
		Worker: "provision-novelty",
		Name:   "新颖性条款智能体",
		LegalBasis: []string{
			"专利法第22条第2款",
		},
		ConceptIDs: []string{"新颖性", "单独对比", "抵触申请"},
		MethodologySteps: []string{
			"Step 1: 确定申请日/优先权日",
			"Step 2: 单独对比分析",
		},
	}

	prompt := BuildProvisionSystemPrompt(entry)
	if prompt == "" {
		t.Fatal("System Prompt 不应为空")
	}
	if !strings.Contains(prompt, "专利法第22条第2款") {
		t.Error("System Prompt 应包含法律依据")
	}
	if !strings.Contains(prompt, "新颖性条款智能体") {
		t.Error("System Prompt 应包含名称")
	}
}

func TestBuildReasoningSystemPrompt(t *testing.T) {
	entry := &ReasoningManifestEntry{
		ID:     "R-01",
		Worker: "reasoning-prior-art-identification",
		Name:   "现有技术认定推理",
		Serves: []string{"P-A01", "P-C04"},
		MethodologySteps: []string{
			"确定文献公开日与申请日/优先权日的时间关系",
			"判断是否属于公众可得知的状态",
		},
	}

	prompt := BuildReasoningSystemPrompt(entry)
	if prompt == "" {
		t.Fatal("System Prompt 不应为空")
	}
	if !strings.Contains(prompt, "现有技术认定推理") {
		t.Error("System Prompt 应包含名称")
	}
	if !strings.Contains(prompt, "P-A01") {
		t.Error("System Prompt 应包含服务的条款簇")
	}
}

func TestProvisionHandoffs(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatal(err)
	}

	// 使用空 Config 测试 Handoff 生成（只需要验证不 panic）
	handoffs := ProvisionHandoffs(m, agentcore.Config{})
	if len(handoffs) == 0 {
		t.Fatal("ProvisionHandoffs() 不应返回空")
	}

	for _, h := range handoffs {
		if h.Name == "" {
			t.Error("Handoff Name 不应为空")
		}
		if h.Description == "" {
			t.Error("Handoff Description 不应为空")
		}
		if h.AgentConfig.Name == "" {
			t.Error("Handoff AgentConfig.Name 不应为空")
		}
		if len(h.AllowedSources) == 0 {
			t.Error("Handoff AllowedSources 不应为空")
		}
	}
}

func TestReasoningHandoffs(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatal(err)
	}

	handoffs := ReasoningHandoffs(m, agentcore.Config{})
	if len(handoffs) == 0 {
		t.Fatal("ReasoningHandoffs() 不应返回空")
	}

	for _, h := range handoffs {
		if h.Name == "" {
			t.Error("Handoff Name 不应为空")
		}
		if h.AgentConfig.SystemPrompt == "" {
			t.Error("Reasoning Handoff SystemPrompt 不应为空")
		}
	}
}

func TestRegisterProvisionHandoffs(t *testing.T) {
	cfg := &agentcore.Config{}
	RegisterProvisionHandoffs(cfg, "")
	if len(cfg.Handoffs) == 0 {
		t.Fatal("RegisterProvisionHandoffs() 后 cfg.Handoffs 不应为空")
	}
	t.Logf("已注册 %d 个 Handoff（含条款和推理模式）", len(cfg.Handoffs))
}

func TestListRegisteredProvisions(t *testing.T) {
	lines := ListRegisteredProvisions("")
	if len(lines) == 0 {
		t.Fatal("ListRegisteredProvisions() 不应为空")
	}
	for _, line := range lines {
		t.Log(line)
	}
}

// =============================================================================
// Orchestrator 测试
// =============================================================================

func TestOrchestratorSystemPrompt(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatal(err)
	}

	prompt := OrchestratorSystemPrompt(m)
	if prompt == "" {
		t.Fatal("OrchestratorSystemPrompt() 不应为空")
	}
	if !strings.Contains(prompt, "patent-orchestrator") {
		t.Error("System Prompt 应包含编排器标识")
	}
	if !strings.Contains(prompt, "transfer_to_") {
		t.Error("System Prompt 应列出 transfer_to 工具")
	}
	if !strings.Contains(prompt, "suggest_checkers") {
		t.Error("System Prompt 应提及质量复核")
	}
}

func TestOrchestratorHandoffConfig(t *testing.T) {
	m, err := LoadManifest("")
	if err != nil {
		t.Fatal(err)
	}

	h := OrchestratorHandoffConfig(m, agentcore.Config{})
	if h.Name != "patent-orchestrator" {
		t.Errorf("预期 Name = patent-orchestrator，实际 = %s", h.Name)
	}
	if h.Description == "" {
		t.Error("Description 不应为空")
	}
	if h.Mode != agentcore.HandoffDelegate {
		t.Errorf("预期 Mode = delegate，实际 = %s", h.Mode)
	}
	if len(h.AllowedSources) == 0 {
		t.Error("AllowedSources 不应为空")
	}
	if h.AgentConfig.Name != "patent-orchestrator" {
		t.Errorf("预期 AgentConfig.Name = patent-orchestrator，实际 = %s", h.AgentConfig.Name)
	}
	if h.AgentConfig.SystemPrompt == "" {
		t.Error("AgentConfig.SystemPrompt 不应为空")
	}
}

// contains 辅助函数

// =============================================================================
// IPC 领域专家（Tier C）测试
// =============================================================================

func TestResolveDomainWorkerName(t *testing.T) {
	name := ResolveDomainWorkerName("A61", "novelty")
	if name != "domain-A61-novelty" {
		t.Errorf("预期 domain-A61-novelty，实际 %s", name)
	}
	name = ResolveDomainWorkerName("g06", "inventiveness")
	if name != "domain-G06-inventiveness" {
		t.Errorf("预期 domain-G06-inventiveness，实际 %s", name)
	}
}

func TestLoadIpcDomainMap(t *testing.T) {
	m, err := LoadIpcDomainMap("")
	if err != nil {
		t.Fatalf("LoadIpcDomainMap() 失败: %v", err)
	}
	if m == nil {
		t.Fatal("LoadIpcDomainMap() 返回 nil")
	}
	if len(m.IpcSections) == 0 {
		t.Fatal("IPC 映射表为空")
	}
	t.Logf("已加载 %d 个 IPC 领域", len(m.IpcSections))
}

func TestBuildDomainSystemPrompt(t *testing.T) {
	section := &IpcSectionEntry{
		Section:     "A61",
		Name:        "医学/卫生学",
		Description: "医疗器械、药物制剂、诊断方法等领域",
	}
	prompt := BuildDomainSystemPrompt(section, "新颖性分析")
	if prompt == "" {
		t.Fatal("System Prompt 不应为空")
	}
	if !strings.Contains(prompt, "医学/卫生学") {
		t.Error("应包含领域名称")
	}
	if !strings.Contains(prompt, "A61") {
		t.Error("应包含 IPC 段")
	}
}

func TestDomainAgentHandoffConfig(t *testing.T) {
	h := DomainAgentHandoffConfig("A61", "novelty", "新颖性分析", agentcore.Config{})
	if h.Name != "domain-A61-novelty" {
		t.Errorf("预期 domain-A61-novelty，实际 %s", h.Name)
	}
	if h.Description == "" {
		t.Error("Description 不应为空")
	}
	if h.AgentConfig.SystemPrompt == "" {
		t.Error("SystemPrompt 不应为空")
	}
	if len(h.AllowedSources) == 0 {
		t.Error("AllowedSources 不应为空")
	}
}

func TestListDomainWorkerNames(t *testing.T) {
	names := ListDomainWorkerNames([]string{"A61", "G06"}, "")
	if len(names) == 0 {
		t.Fatal("ListDomainWorkerNames() 不应为空")
	}
	t.Logf("IPC A61 + G06 的 domain worker: %v", names)
	for _, name := range names {
		if !strings.HasPrefix(name, "domain-") {
			t.Errorf("domain worker 名应以 domain- 开头: %s", name)
		}
	}
}

func TestListDomainWorkerNamesUnknownIPC(t *testing.T) {
	names := ListDomainWorkerNames([]string{"ZZ99"}, "")
	if len(names) != 0 {
		t.Errorf("未知 IPC 应返回空，实际得到 %d 个", len(names))
	}
}

func TestRegisterDomainExpertHandoffs(t *testing.T) {
	m, err := LoadIpcDomainMap("")
	if err != nil {
		t.Fatalf("LoadIpcDomainMap() 失败: %v", err)
	}
	want := 0
	for _, sec := range m.IpcSections {
		if sec.PreRegister {
			want++
		}
	}
	want *= len(m.ProvisionSuffixes)
	if want == 0 {
		t.Fatal("映射表无 pre_register 段，预注册无从验证")
	}

	cfg := &agentcore.Config{}
	got := RegisterDomainExpertHandoffs(cfg, "")
	if got != want {
		t.Fatalf("RegisterDomainExpertHandoffs() = %d, want %d", got, want)
	}
	if len(cfg.Handoffs) != want {
		t.Fatalf("Handoffs = %d, want %d", len(cfg.Handoffs), want)
	}

	seen := make(map[string]bool, want)
	for _, h := range cfg.Handoffs {
		if !strings.HasPrefix(h.Name, "domain-") {
			t.Errorf("Handoff 名 %q 应以 domain- 开头", h.Name)
		}
		if seen[h.Name] {
			t.Errorf("Handoff 名重复: %s", h.Name)
		}
		seen[h.Name] = true
		if h.AgentConfig.SystemPrompt == "" {
			t.Errorf("%s 的 SystemPrompt 不应为空", h.Name)
		}
		if h.AllowedSources == nil {
			t.Errorf("%s 的 AllowedSources 不应为空", h.Name)
		}
	}
	if !seen["domain-A61-novelty"] {
		t.Error("缺少预注册 Handoff domain-A61-novelty")
	}
}

func TestListDomainWorkerNamesSubsetOfRegistered(t *testing.T) {
	cfg := &agentcore.Config{}
	if n := RegisterDomainExpertHandoffs(cfg, ""); n == 0 {
		t.Fatal("预注册返回 0，无法验证一致性")
	}
	registered := make(map[string]bool, len(cfg.Handoffs))
	for _, h := range cfg.Handoffs {
		registered[h.Name] = true
	}

	names := ListDomainWorkerNames([]string{"A61", "G06", "C12"}, "")
	if len(names) == 0 {
		t.Fatal("ListDomainWorkerNames() 不应返回空")
	}
	for _, n := range names {
		if !registered[n] {
			t.Errorf("发现工具广告 %s 但未注册，广告与注册不一致", n)
		}
	}
}
