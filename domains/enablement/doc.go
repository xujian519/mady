// Package enablement 提供专利法第26条第3款（充分公开/可实现性）判断的独立 Pregel 子图。
//
// 本模块对标 domains/inventiveness/（创造性三步法评估），通过 EventBus 接收
// disclosure 管线完成事件，自动评估说明书是否满足 26.3 的充分公开要求。
//
// # 法条依据
//
//	《中华人民共和国专利法》（2020 年修正）第 26 条第 3 款（简称 A26.3 / 26.3）：
//	"说明书应当对发明或者实用新型作出清楚、完整的说明，以所属技术领域的技术人员能够实现为准。"
//
// # 评估流程（三步骤 + 领域自适应）
//
//	Step 1: 完整性检查 — 说明书是否包含 5 项必要章节，评估内容充分性
//	Step 2: 清楚性检查 — 歧义术语/自造词/明显错误检测，PFE 三元组是否闭环
//	Step 3: 能够实现性检查 — 逐一检测六种公开不充分情形，附加领域特殊规则
//	领域检测: 自动识别 chemical/biotech/tcm/computer/mechanical/electronic，附加领域规则
//
// # 使用方式
//
//  1. EventBus 自动触发：disclosure 管线完成后通过 EnablementTrigger 自动运行
//  2. Agent 工具调用：通过 NewEnablementTool() 注册为 Patent Agent 工具
//
// # 关键词
//
//	专利法第26条第3款、26.3、A26.3、充分公开、公开不充分、能够实现、enablement、
//	sufficient disclosure、清楚完整、说明书公开
package enablement
