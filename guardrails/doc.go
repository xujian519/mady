// Package guardrails provides a domain-graded content safety and compliance
// infrastructure for the Mady agent framework.
//
// Guardrails operate at three levels:
//
//	LevelLight    — 通用对话，仅检查基本内容安全和敏感词
//	LevelStandard — 专业咨询，增加不确定性声明要求
//	LevelStrict   — 法律/专利，强制免责声明 + 人工审批门
//
// Each level is implemented as an iface.LifecycleHook that inspects
// model output in AfterModelCall and can inject warnings, block execution,
// or trigger human approval workflows.
//
// Usage:
//
//	// Light guardrail for chat domain
//	hook := guardrails.New(guardrails.LevelLight)
//
//	// Strict guardrail for patent domain with custom disclaimer
//	hook := guardrails.New(guardrails.LevelStrict,
//	    guardrails.WithDisclaimer(guardrails.Disclaimer(guardrails.LevelStrict)),
//	    guardrails.WithApproval([]string{"专利结论", "侵权判断"}),
//	)
//
//	cfg.Lifecycle = agentcore.LifecycleChain{hook}
package guardrails
