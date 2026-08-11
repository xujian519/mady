package domains

import (
	"strings"

	"github.com/xujian519/mady/agentcore"
	"github.com/xujian519/mady/domains/workflows/legal"
	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/psychological"
)

// LegalAgentConfig builds the legal domain Agent configuration.
//
// toolExt 是调用方已装配好的工具扩展，通过被动注入传入。
func LegalAgentConfig(base agentcore.Config, toolExt agentcore.Extension) agentcore.Config {
	cfg := base
	cfg.Name = "legal-advisor"

	cfg.SystemPrompt = strings.Join([]string{
		"你是 Mady 的法律咨询与研究模块。",
		"用简体中文回复，专业严谨。",
		styleInjection("legal"),
		"",
		"五步工作法：",
		"1. 发现事实 — 了解案件背景、当事人信息、法律诉求",
		"2. 获取规则 — 使用 web_search / web_fetch 检索相关法律法规、司法解释、指导性案例；使用 read 工具读取案卷材料",
		"3. 规划 — 确定法律分析框架和论证逻辑",
		"4. 执行 — 进行法条匹配、判例比对、法律推理",
		"5. 检查 — 验证法条引用准确性、判例相关性、论证完整性",
		"",
		"可用工具：",
		"- web_search / web_fetch：检索法律法规、司法解释、裁判文书",
		"- scholar_search：检索学术论文和法学文献",
		"- search_knowledge / search_laws：检索本地知识库中的法律法规、司法解释全文（9121 部法律）",
		"- read / grep / find / glob / ls：读取和分析案件文件、证据材料",
		"- write_file / edit：生成法律文书、合同、分析报告",
		"",
		"使用工具前，先简要说明你要做什么，执行完给出结构化结果。",
		"",
		"免责声明：所有涉及法律判断的输出必须附带：",
		"「本分析由 AI 辅助生成，不构成正式法律意见。」",
		"",
		"输出格式：完成任务后，用以下 JSON 格式返回结果（便于 Chat Agent 解释给用户）：",
		`{"action":"做了什么","result":"结果摘要","success":true}`,
		"- action: 你做了什么操作",
		"- result: 结果的简洁摘要",
		"- success: 是否成功完成",
	}, "\n")

	// 被动注入：调用方已装配好的工具扩展。
	// 法律领域额外工具（legal comparison）追加到 cfg.Tools。
	cfg.Tools = append(cfg.Tools, legal.NewLegalComparisonTool())

	cfg.Extensions = append(cfg.Extensions, toolExt,
		// 心理引擎 — 法律领域：VAD/OCC 语气调整 + 认知扭曲诊断（法律分析需要完整心理评估）。
		psychological.NewExtension(LegalPsychConfig()),
	)

	// Chunked context engine for long legal documents.
	cfg.Engine = "chunked"

	// Reasoning engine injection: same five-step workflow + legal case
	// comparison tools as PatentAgent, via the shared injectDraftingTool.
	injectDomainSharedExtensions(&cfg)

	// DoomLoop + Gateway 统一决策入口（见 assemble.go appendGatewayLifecycle）。
	appendGatewayLifecycle(&cfg, nil)

	// 法条引用核验 Gate（P1b）：R1 存在性 + R2 交叉匹配，命中疑点追加存疑提示。
	// P1b 阶段统一按 Standard 处置；Strict 的 SuppressPersist + ApprovalGate 联动留待 P2。
	appendStandardCitationGate(&cfg)

	// Guardrail: LevelStrict with legal disclaimer + approval gate（共享段落）。
	appendStrictGuardrails(&cfg, DomainLegal, guardrails.DisclaimerLegal, true)

	return cfg
}
