// Package autoresearch 实现面向专利/法律长周期研究任务的自动研究协议。
//
// 对齐 docs/decisions/reasonix-analysis.md §P2 AutoResearch Protocol：
// 状态机管理、任务契约、成功标准追踪、方向追踪、心跳监控。
//
// 架构定位：
//
//	本包属于 workflows/ 但与其他工作流（patent/、legal/）不同——
//	patent/legal 是"多步骤执行流水线"（graph.PregelState + agentcore.Tool），
//	而 autoresearch 是"多轮次元级状态管理器"（纯状态对象 + 内存契约）。
//	两者是层次关系而非平级关系：autoresearch 管理"研究过程"（几轮、用了什么策略、是否超时），
//	patent/invalidation 执行"分析逻辑"（文本解析、规则评估、结论生成）。
//	未来接入时，autoresearch 应包裹 patent/invalidation 工作流。
//
// 适用场景：专利无效宣告检索、法律条文体系性分析等需要多轮推进的长周期任务。
// 状态存储通过 ResearchStore 接口与进程生命周期解耦（默认 InMemoryResearchStore，
// 生产环境应接入 SQLite 或 JSON 文件实现）。
package autoresearch
