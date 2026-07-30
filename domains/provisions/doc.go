// Package provisions 提供专利条款智能体的 Manifest 定义与注册机制。
//
// 本包实现"条款智能体体系"（Patent Provision Agent Architecture），
// 参考 XiaoNuo Agent 的三层智能体模型设计，将专利法各核心条款封装为
// 可独立调用的 Handoff 子 Agent：
//
//   - Tier A (Provision): 围绕单一法条簇完成分析/意见/结论，共 22 个条款簇
//   - Tier B (Reasoning): 封装跨条款的认定步骤，供 Tier A 作为子步骤调用
//   - Tier C (Domain): 按 IPC 技术领域注入领域审查标准（lazy 加载）
//
// # 使用方式
//
// 在 PatentAgentConfig 中调用 register.LoadPatentProvisionHandoffs(&cfg) 即可。
// Manifest 定义在 domains/rules/data/provisions/manifest.yaml。
//
// # 文件结构
//
//	domains/rules/data/provisions/manifest.yaml  — YAML manifest 定义
//	domains/provisions/
//	  doc.go               — 包文档
//	  types.go             — manifest 类型定义 + 加载/校验
//	  provision_agents.go  — Tier A 条款智能体工厂
//	  reasoning_agents.go  — Tier B 推理模式工厂
//	  orchestrator.go      — 专利编排器
//	  domain_agents.go     — Tier C IPC 领域专家工厂
//	  ipc_tool.go          — resolve_domain_workers 工具
//	  register.go          — 注册函数
//	  provision_agents_test.go — 测试
package provisions
