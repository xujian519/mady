package provisions

import (
	"log/slog"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// 注册函数
// =============================================================================

// RegisterProvisionHandoffs 从 manifest 加载条款智能体 Handoff 并追加到配置中。
// 这是 PatentAgentConfig 调用的入口函数。
//
// manifestPath 可以为空（使用默认路径），也可以指定自定义路径。
// cfg 会被修改（Handoffs 字段追加条目）。
func RegisterProvisionHandoffs(cfg *agentcore.Config, manifestPath string) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		// fail-open：manifest 缺失只跳过条款智能体注册，不阻断专利 Agent 启动。
		slog.Warn("provisions: 加载 Manifest 失败，跳过条款智能体注册", "err", err)
		return
	}
	RegisterProvisionHandoffsFromManifest(cfg, manifest)
}

// RegisterProvisionHandoffsFromManifest 从已加载的 Manifest 注册 Handoff。
// 避免重复加载 manifest 文件（与 OrchestratorHandoffConfig 共享同一 manifest）。
func RegisterProvisionHandoffsFromManifest(cfg *agentcore.Config, manifest *PatentManifest) {
	if manifest == nil {
		slog.Warn("provisions: Manifest 为 nil，跳过条款智能体注册")
		return
	}

	provisionCount := 0
	for _, entry := range manifest.Provisions {
		if !entry.PreRegister {
			continue
		}
		cfg.Handoffs = append(cfg.Handoffs, ProvisionToHandoff(&entry, *cfg))
		provisionCount++
	}

	reasoningCount := 0
	for _, entry := range manifest.Reasoning {
		if !entry.PreRegister {
			continue
		}
		cfg.Handoffs = append(cfg.Handoffs, ReasoningToHandoff(&entry, *cfg))
		reasoningCount++
	}

	slog.Info("provisions: 已注册条款智能体",
		"provision_count", provisionCount,
		"reasoning_count", reasoningCount,
	)

	// 校验最小完备集，记录警告而非阻塞
	if ok, missing := ValidateManifest(manifest); !ok {
		slog.Warn("provisions: Manifest 未覆盖全部 Tier A 条款簇",
			"defined", provisionCount+reasoningCount,
			"missing", missing,
		)
	}
}

// ListRegisteredProvisions 返回当前 manifest 中已注册的条款智能体摘要列表。
func ListRegisteredProvisions(manifestPath string) []string {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return []string{"无法加载 Manifest: " + err.Error()}
	}

	var lines []string
	for _, e := range manifest.Provisions {
		status := " 按需"
		if e.PreRegister {
			status = " 已注册"
		}
		subgraph := ""
		if e.ExistingSubgraph != "" {
			subgraph = " [子图:" + e.ExistingSubgraph + "]"
		}
		lines = append(lines, e.ID+status+": "+e.Worker+" — "+e.Name+subgraph)
	}
	for _, e := range manifest.Reasoning {
		status := " 按需"
		if e.PreRegister {
			status = " 已注册"
		}
		lines = append(lines, e.ID+status+": "+e.Worker+" — "+e.Name)
	}
	return lines
}
