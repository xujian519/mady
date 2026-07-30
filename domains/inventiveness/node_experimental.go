package inventiveness

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// evaluateExperimentalDataNode 实验数据核验节点。
// 评估实验数据充分性、补充数据可接受性、对比试验代表性。
// 输出供 generateConclusionNode 的辅助因素分析使用。
// 位置：Step4 之后、generate_conclusion 之前。
// 注意：当 InventivenessInput 中未提供 ExperimentalData 时，节点正常跳过。
func evaluateExperimentalDataNode() graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		if stateHasSkip(state) {
			return state, nil
		}

		raw, ok := state[StateKeyInput]
		if !ok {
			return state, nil
		}
		input, ok := raw.(*InventivenessInput)
		if !ok || input == nil || input.ExperimentalData == nil {
			// 无实验数据输入，正常跳过
			state[stateKeyExperiment] = ""
			return state, nil
		}

		ed := input.ExperimentalData

		var sb strings.Builder
		sb.WriteString("## 实验数据核验报告\n\n")

		// 1. 原始数据审查
		sb.WriteString("### 1. 原始实验数据审查\n")
		if ed.HasOriginalData {
			sb.WriteString("- ✅ 原始申请文件中有实验数据记载\n")
		} else {
			sb.WriteString("- ❌ 原始申请文件中无实验数据记载\n")
		}
		if ed.DataSummary != "" {
			fmt.Fprintf(&sb, "- 数据摘要：%s\n", ed.DataSummary)
		}

		// 2. 补充数据审查
		sb.WriteString("\n### 2. 补充实验数据审查\n")
		if ed.HasSupplementData {
			sb.WriteString("- ⚠️ 存在申请日后补充实验数据\n")
			sb.WriteString("- 需审查补充数据证明的技术效果是否可从原始申请文件公开内容中得到\n")
			sb.WriteString("- 原则：先申请制下，补充数据不得为专利申请文件引入新信息\n")
		} else {
			sb.WriteString("- 无补充数据\n")
		}

		// 3. 对比试验代表性
		sb.WriteString("\n### 3. 对比试验代表性评估\n")
		if ed.ComparisonType != "" {
			fmt.Fprintf(&sb, "- 对比试验类型：%s\n", ed.ComparisonType)
		}
		sb.WriteString("- 注意：不能仅以个别效果最好的实施例代表权利要求的整体保护范围\n")
		sb.WriteString("- 注意：不能仅选择现有技术中效果最差的方案进行对比\n\n")

		state[stateKeyExperiment] = sb.String()
		return state, nil
	}
}
