// Package tracing 提供基于 OpenTelemetry 的分布式追踪能力，用于监控
// Agent 执行链路、LLM 调用延迟和工具调用性能。
//
// 主要功能：
//   - Span 创建与管理（根 Span / 子 Span）
//   - LLM 调用追踪（请求/响应/Token 计数）
//   - 工具调用追踪（入参/出参/延迟）
//   - 上下文传播（Context propagation）
//
// 主要类型：
//   - Tracer: 追踪器，管理 Span 生命周期
//   - Span: 追踪跨度，记录操作起止时间和属性
//   - Config: OTel 导出配置（端点、采样率等）
//
// 使用示例：
//
//	tracer := tracing.NewTracer(cfg)
//	ctx, span := tracer.Start(ctx, "llm-call")
//	defer span.End()
package tracing
