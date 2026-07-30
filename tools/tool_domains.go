package tools

// =============================================================================
// ToolDomains — 工具域映射系统
//
// 借鉴 BCIP 的 13 工具域 (codex-patent-core types) 和按角色过滤工具的机制
// (filter_tools_by_role), 将每个工具归类到功能域。角色级配置通过域来声明
// 需要的工具范围，实现"职责所需即所见"的工具过滤。
//
// 域命名约定：小写英文字母 + 下划线（蛇形命名）。
// 域按功能分类，不考虑实现技术（如 "search" 包含 FTS、向量、外部 API 搜索）。
//
// 使用示例：
//
//	// 创建仅包含 search + analysis 域的工具列表
//	domains := tools.ToolDomainsForRole([]string{"search", "analysis"}, nil)
//	filtered := tools.FilterByDomains(allTools, domains)
//	agent.RegisterTools(filtered...)
// =============================================================================

// ToolDomains 将工具名称映射到功能域。
// 映射表在启动时固定，运行时通过 FilterByDomains 过滤。
//
// 每个工具只分配一个主域。如果一个工具跨多个域，选择最匹配的主域，
// 可在角色配置中通过 Secondary 域补充。
var ToolDomains = map[string]string{
	// ── 系统/内置工具 (system) ──
	ToolBash:        "system",
	ToolExecuteCode: "system",
	ToolProcess:     "system",
	"read":          "system",
	"edit":          "system",
	"write_file":    "system",
	"ls":            "system",
	"grep":          "system",
	"find":          "system",
	"glob":          "system",
	"delete":        "system",
	"move":          "system",
	"mkdir":         "system",
	"patch":         "system",
	"append":        "system",
	ToolPandoc:      "system",
	ToolOCR:         "system",

	// ── 版本控制 (git) ──
	ToolGitStatus: "git",
	ToolGitDiff:   "git",
	ToolGitLog:    "git",

	// ── 搜索 (search) ──
	"search_knowledge":     "search",
	"search_laws":          "search",
	"search_project_files": "search",

	// ── 网络搜索 (web_search) ──
	"web_search": "web_search",
	"web_fetch":  "web_search",

	// ── 浏览器自动化 (browser) ──
	ToolBrowser: "browser",

	// ── 视觉/OCR (vision) ──
	ToolVision: "vision",

	// ── 桌面控制 (desktop) ──
	ToolComputerUse: "desktop",

	// ── 专利搜索 (patent_search) ──
	"patent_search":        "patent_search",
	"google_patents":       "patent_search",
	"iterative_search":     "patent_search",
	"search_query_builder": "patent_search",

	// ── 专利分析 (patent_analysis) ──
	"patent_novelty":         "patent_analysis",
	"patent_inventiveness":   "patent_analysis",
	"novelty_analysis":       "patent_analysis",
	"inventiveness_analysis": "patent_analysis",
	"claim_compare":          "patent_analysis",
	"feature_extractor":      "patent_analysis",

	// ── 权利要求撰写 (drafting) ──
	"claim_draft":         "drafting",
	"claim_generator":     "drafting",
	"specification_draft": "drafting",
	"abstract_draft":      "drafting",

	// ── 审查意见响应 (oa) ──
	"oa_response":      "oa",
	"oa_parser":        "oa",
	"oa_strategist":    "oa",
	"patent_responder": "oa",

	// ── 法律分析 (legal) ──
	"legal_knowledge_search": "legal",
	"legal_comparison":       "legal",
	"claim_parse":            "legal",

	// ── 专利无效/复审 (council) ──
	"infringement_check":  "council",
	"invalidity_check":    "council",
	"reexamination":       "council",
	"design_invalidation": "council",

	// ── 评估 (evaluation) ──
	"patent_eval":          "evaluation",
	"quality_assess":       "evaluation",
	"formal_check":         "evaluation",
	"innovation_evaluator": "evaluation",

	// ── 交底书分析 (disclosure) ──
	"disclosure_analyze": "disclosure",

	// ── 证据分析 (evidence) ──
	"evidence_check": "evidence",

	// ── 充分公开 (enablement) ──
	"enablement_check": "enablement",

	// ── 知识管理 (knowledge) ──
	"add_document": "knowledge",

	// ── 撰写质量 (writing) ──
	"writing_eval": "writing",
	"style_check":  "writing",
}

// ToolDomain 返回工具所属的域名。如果工具未注册域，返回空字符串。
// 未注册域的工具在角色过滤时默认放行（不限制）。
func ToolDomain(name string) string {
	return ToolDomains[name]
}

// AllDomains 返回所有已注册的工具域列表（去重）。
func AllDomains() []string {
	seen := make(map[string]bool)
	for _, d := range ToolDomains {
		seen[d] = true
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	return out
}

// ---------------------------------------------------------------------------
// FilterByDomains — 按工具域过滤工具列表（名称版）
// ---------------------------------------------------------------------------

// FilterToolNames 从工具名称列表中筛选出域属于 allowedDomains 的工具。
// 未注册域的工具默认放行（视为"通用工具"）。
//
// 用途：按 Agent 角色可见性过滤工具名称列表，用于 DisableTools 配置。
//
// 示例：
//
//	roleDomains := agentconfig.ToolDomainSet{
//	    Primary:   []string{"search", "patent_search", "patent_analysis"},
//	}
//	cfg.DisableTools = tools.FilterToolNames(allToolNames, roleDomains.AllDomains())
func FilterToolNames(names []string, allowedDomains []string) []string {
	if len(allowedDomains) == 0 {
		return names
	}
	allowed := make(map[string]bool, len(allowedDomains))
	for _, d := range allowedDomains {
		allowed[d] = true
	}
	var filtered []string
	for _, name := range names {
		domain := ToolDomain(name)
		if domain == "" || allowed[domain] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}

// ToolHasDomain 检查工具名称是否属于指定的域列表。
// 未注册域的工具视为通用，始终返回 true。
func ToolHasDomain(toolName string, domains []string) bool {
	d := ToolDomain(toolName)
	if d == "" {
		return true
	}
	for _, allowed := range domains {
		if d == allowed {
			return true
		}
	}
	return false
}
