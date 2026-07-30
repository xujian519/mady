package domains

import (
	"testing"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/provisions"
)

// TestPatentAgentProvisionHandoffsIntegrated 验证 PatentAgentConfig 中
// 条款智能体注册的端到端完整性。
//
// 测试链路：ProfessionalHandoffConfigs → PatentAgentConfig
//
//	→ RegisterProvisionHandoffsFromManifest → 37 Handoff 注册
func TestPatentAgentProvisionHandoffsIntegrated(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}

	// 通过 ProfessionalHandoffConfigs 模拟 UnifiedAgent 创建 PatentAgent 的路径
	handoffs := ProfessionalHandoffConfigs(base)

	var patentHandoff *agentcore.HandoffConfig
	for i, h := range handoffs {
		if h.Name == DomainPatent {
			patentHandoff = &handoffs[i]
			break
		}
	}

	if patentHandoff == nil {
		t.Fatal("ProfessionalHandoffConfigs 应包含 patent 域的 Handoff")
	}

	// 验证 PatentAgent 配置包含 provisions
	cfg := patentHandoff.AgentConfig

	// 验证 PatentAgent 的 Name
	if cfg.Name != "patent-agent" {
		t.Errorf("PatentAgent Name = %q, 预期 %q", cfg.Name, "patent-agent")
	}

	// 验证专利 Agent 的 Handoff 中包含 provision 条目
	hasNovelty := false
	hasInventiveness := false
	hasOrchestrator := false
	for _, h := range cfg.Handoffs {
		switch h.Name {
		case "provision-novelty":
			hasNovelty = true
		case "provision-inventiveness":
			hasInventiveness = true
		case "patent-orchestrator":
			hasOrchestrator = true
		}
	}

	if !hasNovelty {
		t.Error("PatentAgent Handoff 中缺少 provision-novelty")
	}
	if !hasInventiveness {
		t.Error("PatentAgent Handoff 中缺少 provision-inventiveness")
	}
	if !hasOrchestrator {
		t.Error("PatentAgent Handoff 中缺少 patent-orchestrator")
	}

	// 验证被标记为 invisible 的 provision handoff
	for _, h := range cfg.Handoffs {
		if h.Name == "provision-novelty" && !h.Invisible {
			t.Error("provision Handoff 应标记为 Invisible")
		}
	}

	// 验证 orchestrator 对用户可见
	for _, h := range cfg.Handoffs {
		if h.Name == "patent-orchestrator" && h.Invisible {
			t.Error("patent-orchestrator Handoff 应对用户可见（Invisible=false）")
		}
	}

	t.Logf("PatentAgent 共有 %d 个 Handoff（含 provisions + orchestrator）", len(cfg.Handoffs))
}

// TestPatentAgentSystemPromptMentionsProvisions 验证 PatentAgent 的
// System Prompt 包含条款智能体委派指引。
func TestPatentAgentSystemPromptMentionsProvisions(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}

	cfg := PatentAgentConfig(base)

	checks := []struct {
		name    string
		keyword string
	}{
		{"编排器", "patent-orchestrator"},
		{"新颖性", "provision-novelty"},
		{"创造性", "provision-inventiveness"},
		{"实用性", "provision-utility"},
		{"保护客体", "provision-eligibility"},
		{"充分公开", "provision-disclosure"},
		{"清楚支持", "provision-claims-clarity"},
		{"修改超范围", "provision-amendment"},
		{"现有技术", "provision-prior-art"},
		{"权利要求书撰写", "provision-drafting-claims"},
		{"推理模式", "transfer_to_reasoning-"},
	}

	for _, c := range checks {
		if !containsString(cfg.SystemPrompt, c.keyword) {
			t.Errorf("System Prompt 缺少 %q（应包含 %q）", c.name, c.keyword)
		}
	}
}

// TestProvisionsManifestOnDisk 验证 manifest.yaml 文件存在于磁盘上，
// 且能被 provisions 包加载。
func TestProvisionsManifestOnDisk(t *testing.T) {
	manifest, err := provisions.LoadManifest("")
	if err != nil {
		t.Fatalf("加载 manifest 失败: %v", err)
	}
	if len(manifest.Provisions) == 0 {
		t.Fatal("manifest 中 provisions 为空")
	}
	if len(manifest.Reasoning) == 0 {
		t.Fatal("manifest 中 reasoning 为空")
	}
	t.Logf("Manifest: %d provisions, %d reasoning", len(manifest.Provisions), len(manifest.Reasoning))
}

// TestBuildProjectAgentHasProvisions 验证 BuildProjectAgent 也注册了 provisions。
func TestBuildProjectAgentHasProvisions(t *testing.T) {
	base := agentcore.Config{}
	base.Provider = &mockProvider{}

	rec := ProjectRecord{
		ProjectID: "test-case-001",
		RootPath:  "/tmp/test-case",
		Status:    StatusActive,
	}

	cfg := BuildProjectAgent(rec, base)

	hasNovelty := false
	for _, h := range cfg.Handoffs {
		if h.Name == "provision-novelty" {
			hasNovelty = true
			break
		}
	}

	if !hasNovelty {
		t.Error("BuildProjectAgent 生成的 Agent 中缺少 provision-novelty")
	}
}

// containsString 辅助函数
func containsString(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
