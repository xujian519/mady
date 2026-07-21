// Package registry 提供 Agent 注册表，用于管理 A2A 协议中的 Agent 发现和查询。
//
// Registry 是线程安全的内存注册表，支持按名称、能力和技能进行 Agent 注册、
// 注销和查询。
//
// 使用示例：
//
//	reg := registry.New()
//
//	// 注册一个 Agent
//	err := reg.Register(&registry.Registration{
//	    Name:        "patent-agent",
//	    URL:         "http://localhost:8080",
//	    Version:     "1.0.0",
//	    Capabilities: []string{"streaming", "chat", "patent"},
//	    Skills:      []string{"patent-analysis", "novelty-check"},
//	})
//
//	// 查询 Agent
//	agent, ok := reg.Get("patent-agent")
//
//	// 按能力列表
//	agents := reg.ListByCapability("patent")
//
//	// 按技能列表
//	agents := reg.ListBySkill("patent-analysis")
package registry
