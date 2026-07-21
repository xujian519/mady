// Package domainconfig 提供领域 Agent 的声明式配置系统。
//
// # 概述
//
// domainconfig 允许通过 YAML/JSON 文件声明领域 Agent 的运行时配置，
// 无需编写 Go 代码即可注册新领域。与 manifest（agentcore.AgentManifest）
// 的分工明确：
//
//   - Manifest 是轻量元数据，描述 Agent 的身份和路由信息
//   - DomainConfig 是完整运行时配置，包含模型参数、工具列表、技能路径等
//
// # 使用示例
//
//	// 加载单个配置
//	cfg, err := domainconfig.LoadConfig("patent.yaml")
//
//	// 批量加载目录下的所有配置
//	configs, err := domainconfig.LoadConfigs("/etc/mady/domains/")
//
//	// 使用默认目录
//	dir := domainconfig.DefaultConfigDir()
//	configs, err := domainconfig.LoadConfigs(dir)
//
// # 配置文件格式
//
//	# patent.yaml
//	name: patent-agent
//	domain: patent
//	description: "专利代理与知识产权分析"
//	guardrail_level: strict
//	knowledge_domain: patent
//	handoff_targets:
//	  - chat-agent
//	  - assistant-agent
//	model: gpt-4o
//	temperature: 0.1
//	max_tokens: 4096
//	engine: chunked
//	max_turns: 50
//	system_prompt: "你是专利代理人..."
//	tools:
//	  - patent_search
//	  - drafting
//	skill_paths:
//	  - skills/patent
//
// # 与现有工厂函数的关系
//
// 本包只提供声明式配置的加载和校验，不修改现有的 Go 工厂函数
// （如 domains.PatentAgentConfig、domains.ChatAgentConfig）。
// 保持向后兼容，调用方可自行决定使用声明式配置还是工厂函数。
package domainconfig
