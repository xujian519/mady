package agentcore

// =============================================================================
// ToolDomains — 工具域映射系统
//
// 借鉴 BCIP 的 13 工具域 (codex-patent-core types) 和按角色过滤工具的机制
// (filter_tools_by_role), 将每个工具归类到功能域。角色级配置通过域来声明
// 需要的工具范围，实现"职责所需即所见"的工具过滤。
//
// 域命名约定：小写英文字母 + 下划线（蛇形命名）。
// 域按功能分类，不考虑实现技术（如 "search" 包含 FTS、向量、外部 API 搜索）。
// =============================================================================

// ToolDomains 将工具名称映射到功能域。
// 映射表在启动时固定。注意：当前映射表尚未接线到角色工具过滤
// （FilterToolNames 等函数已删除，角色级工具过滤属未完成功能，接线需另立 spec）。
//
// 每个工具只分配一个主域。如果一个工具跨多个域，选择最匹配的主域。
var ToolDomains = map[string]string{
	// ── 系统/内置工具 (system) ──
	"bash":             "system",
	"execute_code":     "system",
	"process":          "system",
	"read":             "system",
	"edit":             "system",
	"write_file":       "system",
	"ls":               "system",
	"grep":             "system",
	"find":             "system",
	"glob":             "system",
	"delete":           "system",
	"move":             "system",
	"mkdir":            "system",
	"patch":            "system",
	"append":           "system",
	"convert_document": "system",
	"ocr":              "system",

	// ── 版本控制 (git) ──
	"git_status": "git",
	"git_diff":   "git",
	"git_log":    "git",

	// ── 搜索 (search) ──
	"search_knowledge":     "search",
	"search_laws":          "search",
	"search_project_files": "search",

	// ── 网络搜索 (web_search) ──
	"web_search": "web_search",
	"web_fetch":  "web_search",

	// ── 浏览器自动化 (browser) ──
	"browser": "browser",

	// ── 视觉/OCR (vision) ──
	"vision_analyze": "vision",

	// ── 桌面控制 (desktop) ──
	"computer_use": "desktop",

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
