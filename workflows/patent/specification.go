package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// State keys for the specification drafting workflow.
const (
	StateSpecDisclosure = "spec_disclosure" // original technical disclosure
	StateSpecClaims     = "spec_claims"     // claim set text
	StateSpecTechField  = "spec_tech_field" // section 1: 技术领域
	StateSpecBackground = "spec_background" // section 2: 背景技术
	StateSpecInvention  = "spec_invention"  // section 3: 发明内容
	StateSpecDrawings   = "spec_drawings"   // section 4: 附图说明
	StateSpecEmbodiment = "spec_embodiment" // section 5: 具体实施方式
	StateSpecOutput     = "spec_output"     // complete specification
)

// specTechFieldNode generates the "技术领域" section.
// Extracts the technical domain from the disclosure.
func specTechFieldNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	disclosure := state.GetString(StateSpecDisclosure)
	if disclosure == "" {
		return nil, fmt.Errorf("specification: disclosure is empty")
	}

	// Extract the technical field from the disclosure.
	// Look for standard Chinese patent disclosure patterns.
	techField := extractTechField(disclosure)

	return graph.PregelState{
		StateSpecTechField:  techField,
		StateSpecDisclosure: disclosure,
	}, nil
}

// extractTechField identifies the technical field from disclosure text.
func extractTechField(disclosure string) string {
	var field strings.Builder
	field.WriteString("本发明涉及")

	// Look for key domain-indicating patterns.
	patterns := []string{"技术领域", "应用领域", "属于", "涉及"}
	for _, p := range patterns {
		if idx := strings.Index(disclosure, p); idx >= 0 {
			rest := disclosure[idx+len(p):]
			// Take up to the first sentence ending.
			if end := strings.IndexAny(rest, "。；\n"); end > 0 {
				field.WriteString(strings.TrimSpace(rest[:end]))
			} else if len(rest) > 5 {
				field.WriteString(strings.TrimSpace(rest[:min(len(rest), 60)]))
			}
			break
		}
	}

	field.WriteString("技术领域。")
	return field.String()
}

// specBackgroundNode generates the "背景技术" section.
// Describes the technical problem and existing solutions.
func specBackgroundNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	disclosure := state.GetString(StateSpecDisclosure)

	var bg strings.Builder
	bg.WriteString("在现有技术中，")

	// Extract problem statement.
	if idx := strings.Index(disclosure, "技术问题"); idx >= 0 {
		rest := disclosure[idx+len("技术问题"):]
		if end := strings.IndexAny(rest, "。；\n"); end > 0 {
			bg.WriteString(strings.TrimSpace(rest[:end]))
		} else {
			bg.WriteString("存在需要解决的技术问题")
		}
	} else if idx := strings.Index(disclosure, "缺点"); idx >= 0 {
		rest := disclosure[idx:]
		if end := strings.IndexAny(rest, "。；\n"); end > 0 {
			bg.WriteString(strings.TrimSpace(rest[:end]))
		}
	} else {
		bg.WriteString("相关技术存在改进空间")
	}

	bg.WriteString("。\n\n目前，")
	// Extract existing solutions if mentioned.
	if idx := strings.Index(disclosure, "现有"); idx >= 0 {
		rest := disclosure[idx:]
		if end := strings.IndexAny(rest, "。；\n"); end > 0 {
			bg.WriteString(strings.TrimSpace(rest[:end]))
		}
	} else {
		bg.WriteString("尚未有公开的技术方案能够有效解决上述问题")
	}
	bg.WriteString("。")

	return graph.PregelState{
		StateSpecBackground: bg.String(),
		StateSpecTechField:  state[StateSpecTechField],
		StateSpecDisclosure: state[StateSpecDisclosure],
	}, nil
}

// specInventionNode generates the "发明内容" section.
// Outlines the technical problem, solution, and beneficial effects.
func specInventionNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	disclosure := state.GetString(StateSpecDisclosure)

	var inv strings.Builder
	inv.WriteString("本发明要解决的技术问题是：")

	// Problem
	if idx := strings.Index(disclosure, "技术问题"); idx >= 0 {
		rest := disclosure[idx+len("技术问题"):]
		if end := strings.IndexAny(rest, "。；\n"); end > 0 {
			inv.WriteString(strings.TrimSpace(rest[:end]))
		}
	} else {
		inv.WriteString("提供一种改进的技术方案")
	}
	inv.WriteString("。\n\n")
	inv.WriteString("为解决上述技术问题，本发明采用如下技术方案：\n\n")

	// Solution — extract from disclosure or use claims.
	claims := state.GetString(StateSpecClaims)
	if claims != "" {
		inv.WriteString(claims)
	} else {
		// Fallback: extract solution from disclosure.
		if idx := strings.Index(disclosure, "技术方案"); idx >= 0 {
			rest := disclosure[idx+len("技术方案"):]
			if end := strings.Index(rest, "有益效果"); end > 0 {
				inv.WriteString(strings.TrimSpace(rest[:end]))
			} else if len(rest) > 10 {
				inv.WriteString(strings.TrimSpace(rest[:min(len(rest), 500)]))
			}
		} else {
			inv.WriteString(disclosure[:min(len(disclosure), 500)])
		}
	}
	inv.WriteString("\n\n")

	// Beneficial effects
	inv.WriteString("与现有技术相比，本发明具有以下有益效果：\n\n")
	if idx := strings.Index(disclosure, "有益效果"); idx >= 0 {
		rest := disclosure[idx+len("有益效果"):]
		if end := strings.Index(rest, "附图说明"); end > 0 {
			inv.WriteString(strings.TrimSpace(rest[:end]))
		} else {
			inv.WriteString(strings.TrimSpace(rest[:min(len(rest), 300)]))
		}
	} else {
		inv.WriteString("1. 提供了一种改进的技术实现方案。\n")
		inv.WriteString("2. 提升了系统的性能和可靠性。\n")
		inv.WriteString("3. 降低了实施成本，便于推广应用。\n")
	}

	return graph.PregelState{
		StateSpecInvention:  inv.String(),
		StateSpecTechField:  state[StateSpecTechField],
		StateSpecBackground: state[StateSpecBackground],
		StateSpecDisclosure: state[StateSpecDisclosure],
	}, nil
}

// specDrawingsNode generates the "附图说明" section.
func specDrawingsNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	var dwg strings.Builder
	dwg.WriteString("图1为本发明实施例的方法流程图。\n")
	dwg.WriteString("图2为本发明实施例的系统结构示意图。\n")
	dwg.WriteString("图3为本发明实施例的模块交互图。\n")

	return graph.PregelState{
		StateSpecDrawings:   dwg.String(),
		StateSpecTechField:  state[StateSpecTechField],
		StateSpecBackground: state[StateSpecBackground],
		StateSpecInvention:  state[StateSpecInvention],
		StateSpecDisclosure: state[StateSpecDisclosure],
	}, nil
}

// specEmbodimentNode generates the "具体实施方式" section.
// Provides detailed implementation examples.
func specEmbodimentNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	disclosure := state.GetString(StateSpecDisclosure)

	var emb strings.Builder
	emb.WriteString("为使本发明的目的、技术方案和优点更加清楚，下面将结合附图和具体实施例，")
	emb.WriteString("对本发明的技术方案进行清楚、完整的描述。\n\n")

	emb.WriteString("实施例1\n\n")
	emb.WriteString("本实施例提供一种实现方式。")
	if idx := strings.Index(disclosure, "实施方式"); idx >= 0 {
		rest := disclosure[idx+len("实施方式"):]
		emb.WriteString(strings.TrimSpace(rest[:min(len(rest), 400)]))
	} else if idx := strings.Index(disclosure, "实施例"); idx >= 0 {
		rest := disclosure[idx+len("实施例"):]
		emb.WriteString(strings.TrimSpace(rest[:min(len(rest), 400)]))
	} else {
		emb.WriteString("具体包括以下步骤：\n")
		emb.WriteString("1. 接收输入数据；\n")
		emb.WriteString("2. 对输入数据进行预处理；\n")
		emb.WriteString("3. 执行核心处理算法；\n")
		emb.WriteString("4. 输出处理结果。\n")
	}
	emb.WriteString("\n\n")

	emb.WriteString("实施例2\n\n")
	emb.WriteString("本实施例在实施例1的基础上进一步优化。")
	emb.WriteString("所属技术领域的技术人员应当理解，")
	emb.WriteString("上述实施例仅为本发明的优选实施例，并非用于限制本发明的保护范围。")
	emb.WriteString("凡是利用本发明说明书及附图内容所作的等效结构或等效流程变换，")
	emb.WriteString("均同理包括在本发明的专利保护范围内。\n")

	return graph.PregelState{
		StateSpecEmbodiment: emb.String(),
		StateSpecTechField:  state[StateSpecTechField],
		StateSpecBackground: state[StateSpecBackground],
		StateSpecInvention:  state[StateSpecInvention],
		StateSpecDrawings:   state[StateSpecDrawings],
		StateSpecDisclosure: state[StateSpecDisclosure],
	}, nil
}

// specAssembleNode assembles all sections into the complete specification document.
func specAssembleNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	var doc strings.Builder

	doc.WriteString("# 说明书\n\n")
	doc.WriteString("## 技术领域\n\n")
	doc.WriteString(state.GetString(StateSpecTechField))
	doc.WriteString("\n\n## 背景技术\n\n")
	doc.WriteString(state.GetString(StateSpecBackground))
	doc.WriteString("\n\n## 发明内容\n\n")
	doc.WriteString(state.GetString(StateSpecInvention))
	doc.WriteString("\n\n## 附图说明\n\n")
	doc.WriteString(state.GetString(StateSpecDrawings))
	doc.WriteString("\n\n## 具体实施方式\n\n")
	doc.WriteString(state.GetString(StateSpecEmbodiment))
	doc.WriteString("\n\n---\n")
	doc.WriteString("> ⚠️ 本说明书由 AI 辅助生成，不构成正式专利申请文件。")
	doc.WriteString("专利申请应由具备资质的专利代理师撰写和提交。\n")

	return graph.PregelState{
		StateSpecOutput: doc.String(),
	}, nil
}

// BuildSpecificationGraph constructs a Pregel graph for patent specification drafting.
//
// Graph structure:
//
//	parse_spec → tech_field → background → invention → drawings → embodiment → assemble → __end__
func BuildSpecificationGraph() (*graph.CompiledPregelGraph, error) {
	g := graph.NewPregelGraph()

	g.AddNode("tech_field", specTechFieldNode)
	g.AddNode("background", specBackgroundNode)
	g.AddNode("invention", specInventionNode)
	g.AddNode("drawings", specDrawingsNode)
	g.AddNode("embodiment", specEmbodimentNode)
	g.AddNode("assemble", specAssembleNode)

	// Linear flow through all 5 sections + assembly.
	g.AddEdge("tech_field", "background")
	g.AddEdge("background", "invention")
	g.AddEdge("invention", "drawings")
	g.AddEdge("drawings", "embodiment")
	g.AddEdge("embodiment", "assemble")
	g.AddEdge("assemble", graph.PregelEnd)

	return g.Compile("tech_field", 10)
}
