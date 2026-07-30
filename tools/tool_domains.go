package tools

// =============================================================================
// ToolDomains — 工具域映射系统
//
// 借鉴 BCIP 的 13 工具域 (codex-patent-core types) 和按角色过滤工具的机制
// (filter_tools_by_role), 将每个工具归类到功能域。角色级配置通过域来声明
// 需要的工具范围，实现"职责所需即所见"的工具过滤。
//
// 核心数据已迁移到 agentcore.ToolDomains，本文件维护向后兼容的别名。
// =============================================================================

import "github.com/xujian519/mady/agentcore"

// ToolDomains 将工具名称映射到功能域。
// Deprecated: 请使用 agentcore.ToolDomains。
var ToolDomains = agentcore.ToolDomains

// ToolDomain 返回工具所属的域名。
// Deprecated: 请使用 agentcore.ToolDomain。
func ToolDomain(name string) string {
	return agentcore.ToolDomain(name)
}

// AllDomains 返回所有已注册的工具域列表（去重）。
// Deprecated: 请使用 agentcore.AllDomains。
func AllDomains() []string {
	return agentcore.AllDomains()
}

// FilterToolNames 从工具名称列表中筛选出域属于 allowedDomains 的工具。
// Deprecated: 请使用 agentcore.FilterToolNames。
func FilterToolNames(names []string, allowedDomains []string) []string {
	return agentcore.FilterToolNames(names, allowedDomains)
}

// ToolHasDomain 检查工具名称是否属于指定的域列表。
// Deprecated: 请使用 agentcore.ToolHasDomain。
func ToolHasDomain(toolName string, domains []string) bool {
	return agentcore.ToolHasDomain(toolName, domains)
}
