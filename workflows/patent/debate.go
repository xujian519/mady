package patent

import (
	"context"
	"fmt"
	"strings"

	"github.com/xujian519/mady/graph"
)

// State keys for the examiner debate workflow.
const (
	StateDebateClaims     = "debate_claims"     // claim text under examination
	StateDebateDisclosure = "debate_disclosure" // technical disclosure
	StateDebateRounds     = "debate_rounds"     // accumulated debate rounds
	StateDebateSummary    = "debate_summary"    // final summary
	StateDebateOutput     = "debate_output"     // complete debate transcript
)

// DebateRound records a single exchange in the examiner-agent debate.
type DebateRound struct {
	Round   int    `json:"round"`
	Speaker string `json:"speaker"` // "examiner" or "agent"
	Content string `json:"content"`
}

// examinerObjections defines typical patent examination objections.
var examinerObjections = []struct {
	Topic   string
	Pattern string
}{
	{"新颖性", "与对比文件1相比，权利要求1的技术方案不具备新颖性，因为对比文件1公开了"},
	{"创造性", "权利要求1相对于对比文件1和对比文件2的结合不具备创造性，因为本领域技术人员有动机将"},
	{"清楚性", "权利要求中使用的术语未在说明书中明确定义，导致保护范围不清楚，违反第26条第4款"},
	{"支持性", "权利要求概括的范围得不到说明书的支持，说明书中仅公开了该方案的特定实施例"},
	{"修改超范围", "申请人对权利要求的修改超出了原申请文件记载的范围，违反第33条规定"},
	{"必要技术特征", "独立权利要求缺少解决技术问题的必要技术特征，不符合第21条第2款规定"},
}

// agentResponses provides standard agent rebuttal templates.
var agentResponses = []string{
	"申请人认为，本发明的区别技术特征不在于各个特征的简单组合，而在于特征之间的协同作用所产生的整体技术效果。对比文件均未公开或暗示这种协同关系。",
	"关于创造性的审查意见，申请人认为审查员采用的'事后诸葛亮'式分析不适用于本案。对比文件之间缺乏组合的技术启示，本领域技术人员没有动机将对比文件1的教导应用于对比文件2。",
	"关于权利要求不清楚的审查意见，申请人对权利要求进行了修改，增加了所述术语的定义，明确了保护范围。修改后的权利要求克服了审查员指出的缺陷。",
}

// buildDebateRounds creates 3 rounds of examiner-agent debate.
// Each round follows the standard examination pattern:
//   - Examiner raises objection → Agent rebuts
func buildDebateRounds(claims, disclosure string) []DebateRound {
	var rounds []DebateRound

	// Use 3 different objection topics for 3 rounds.
	objIndices := []int{0, 1, 3} // 新颖性, 创造性, 支持性
	if len(claims) > 200 {
		objIndices = []int{0, 1, 2} // 清楚性 for longer claims
	}

	for round := 0; round < 3; round++ {
		objIdx := objIndices[round%len(objIndices)]
		obj := examinerObjections[objIdx]

		// Examiner objection
		examinerContent := fmt.Sprintf("%s%s。", obj.Pattern, extractKeyTerm(claims, round))
		rounds = append(rounds, DebateRound{
			Round:   round + 1,
			Speaker: "examiner",
			Content: examinerContent,
		})

		// Agent rebuttal
		agentContent := agentResponses[round%len(agentResponses)]
		rounds = append(rounds, DebateRound{
			Round:   round + 1,
			Speaker: "agent",
			Content: agentContent,
		})
	}

	return rounds
}

// extractKeyTerm picks a distinguishing technical term from claims.
func extractKeyTerm(claims string, round int) string {
	terms := []string{"所述方法", "所述装置", "所述系统", "所述模块", "所述步骤"}
	for _, term := range terms {
		if strings.Contains(claims, term) {
			idx := strings.Index(claims, term)
			if idx < 0 {
				continue
			}
			rest := claims[idx:]
			if end := strings.IndexAny(rest, "，。；、"); end > 0 && end > len(term) {
				return strings.TrimSpace(rest[:end])
			}
			return term
		}
	}
	return "所述技术方案"
}

// =============================================================================
// Pregel graph nodes
// =============================================================================

// debateInitNode initializes the debate state with claims and disclosure.
func debateInitNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	claims := state.GetString(StateDebateClaims)
	if claims == "" {
		return nil, fmt.Errorf("debate: claims text is required")
	}

	rounds := buildDebateRounds(claims, state.GetString(StateDebateDisclosure))

	return graph.PregelState{
		StateDebateClaims:     claims,
		StateDebateDisclosure: state[StateDebateDisclosure],
		StateDebateRounds:     rounds,
	}, nil
}

// debateRoundNode processes a single debate round (one examiner-agent exchange).
// The round index is determined by the superstep counter.
func debateRoundNode(roundIdx int) graph.PregelNode {
	return func(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
		rounds, _ := state[StateDebateRounds].([]DebateRound)
		claims := state.GetString(StateDebateClaims)

		// If we've exhausted all pre-built rounds, no-op.
		start := roundIdx * 2
		if start >= len(rounds) {
			return state, nil
		}

		// The debate rounds are pre-built; this node just passes them through.
		// In a future LLM-enhanced version, each round would dynamically generate
		// examiner objections and agent responses via LLM calls.
		return graph.PregelState{
			StateDebateClaims:     claims,
			StateDebateDisclosure: state[StateDebateDisclosure],
			StateDebateRounds:     rounds,
		}, nil
	}
}

// debateSummaryNode assembles the complete debate transcript and outcome.
func debateSummaryNode(ctx context.Context, state graph.PregelState) (graph.PregelState, error) {
	rounds, _ := state[StateDebateRounds].([]DebateRound)

	var transcript strings.Builder
	transcript.WriteString("# 审查意见模拟辩论记录\n\n")
	transcript.WriteString("## 涉案权利要求\n\n")
	transcript.WriteString(state.GetString(StateDebateClaims))
	transcript.WriteString("\n\n## 辩论过程\n\n")

	for _, r := range rounds {
		switch r.Speaker {
		case "examiner":
			fmt.Fprintf(&transcript, "### 第%d轮 — 审查员意见\n\n", r.Round)
			transcript.WriteString("> **审查员：** ")
			transcript.WriteString(r.Content)
			transcript.WriteString("\n\n")
		case "agent":
			fmt.Fprintf(&transcript, "### 第%d轮 — 代理人答复\n\n", r.Round)
			transcript.WriteString("**代理人：** ")
			transcript.WriteString(r.Content)
			transcript.WriteString("\n\n")
		}
	}

	// Summary and strategic recommendation
	transcript.WriteString("## 综合分析\n\n")

	// Count objection types
	objCounts := map[string]int{}
	for _, r := range rounds {
		if r.Speaker == "examiner" {
			for _, obj := range examinerObjections {
				if strings.Contains(r.Content, obj.Pattern[:min(len(obj.Pattern), 10)]) {
					objCounts[obj.Topic]++
					break
				}
			}
		}
	}

	transcript.WriteString("### 审查意见类型统计\n\n")
	for topic, count := range objCounts {
		fmt.Fprintf(&transcript, "- **%s**：%d 项\n", topic, count)
	}

	transcript.WriteString("\n### 答复策略建议\n\n")
	transcript.WriteString("1. **优先处理新颖性问题**：对比文件1公开的内容与本发明技术方案之间的区别 ")
	transcript.WriteString("需明确界定，重点论述区别技术特征的存在及其技术贡献。\n")
	transcript.WriteString("2. **创造性答辩要点**：采用'问题-方案-效果'三步法论述，")
	transcript.WriteString("强调对比文件之间缺乏组合动机，本发明的技术效果不可预见。\n")
	transcript.WriteString("3. **形式缺陷修补**：根据审查意见对权利要求进行适应性修改，")
	transcript.WriteString("消除不清楚的表述，确保每个术语在说明书中有明确依据。\n\n")

	transcript.WriteString("---\n")
	transcript.WriteString("> ⚠️ 本辩论记录由 AI 模拟生成，仅供代理人参考，不构成正式答复意见。")
	transcript.WriteString("正式答复应结合案件具体情况和审查指南的规定，由执业专利代理师撰写。\n")

	return graph.PregelState{
		StateDebateSummary: transcript.String(),
		StateDebateOutput:  transcript.String(),
		StateDebateRounds:  rounds,
		StateDebateClaims:  state[StateDebateClaims],
	}, nil
}

// BuildDebateGraph constructs a Pregel graph for examiner-agent debate simulation.
//
// Graph structure:
//
//	init → round1 → round2 → round3 → summarize → __end__
//
// Each round node represents one examiner objection + one agent rebuttal exchange.
// Currently uses rule-based arguments; future enhancement will add LLM nodes.
func BuildDebateGraph() (*graph.CompiledPregelGraph, error) {
	g := graph.NewPregelGraph()

	g.AddNode("init", debateInitNode)
	g.AddNode("round1", debateRoundNode(0))
	g.AddNode("round2", debateRoundNode(1))
	g.AddNode("round3", debateRoundNode(2))
	g.AddNode("summarize", debateSummaryNode)

	g.AddEdge("init", "round1")
	g.AddEdge("round1", "round2")
	g.AddEdge("round2", "round3")
	g.AddEdge("round3", "summarize")
	g.AddEdge("summarize", graph.PregelEnd)

	return g.Compile("init", 8)
}
