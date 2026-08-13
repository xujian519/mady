// Package piagent 将 sky-valley/pi（纯 Go 智能体框架）作为辅助智能体运行时
// 嵌入 Mady 工具层，实现 Sati 式动态子会话（spawn_agent）。
//
// 职责边界：
//   - 父 Agent（mady-agent）仍由 agentcore 驱动，本包不触碰主循环
//   - 子会话由 pi coding.Session 驱动，拥有独立上下文与 token 预算
//   - 子会话内所有工具调用经本包桥接层执行安全门控：
//     ① 只读预设拒绝写类工具（执行前判定，不产生副作用）
//     ② 底层 Mady 工具自身执行 WorkingDir 沙箱校验（resolvePathSandboxed）
//     ③ 嵌套禁用：子会话工具注册表不包含 spawn_agent
//
// 预设语义对齐 Sati builtinSubagentTypes（explore/verify/plan/general-purpose），
// 详见 presets.go。安全不变量见 03-design.md §3。
package piagent
