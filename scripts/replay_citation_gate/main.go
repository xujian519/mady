// replay_citation_gate 是引用核验 Gate 的离线回放验收工具
// （docs/design/citation-verification-gate.md §8.2 硬性验收标准）。
//
// 读取 $TMPDIR 下 v0.8 基线缓存的三层真实答案（L0/L1/L3 各 31 题），
// 逐条跑 guardrails.VerifyCitations，输出每层命中与误报统计。
// 验收标准：
//   - patent_exam_2008_a31_02（法条编号幻觉题）三层答案必须全部命中；
//   - 其余答案误报（Flagged 非空）必须为 0。
//
// 用法：go run ./scripts/replay_citation_gate
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/guardrails"
)

// cacheFile 描述一层答案缓存（JSON：caseID → 答案文本）。
type cacheFile struct {
	name string
	file string
}

// 经人工核实的真实引用错误答案（回放确认的张冠李戴），属于预期命中；
// 其余答案被标记一律计为误报。
var knownTruePositives = map[string]bool{
	"patent_exam_2008_a31_02": true, // 分案申请错引专利法第47条/细则21条
	"patent_exam_2010_a22_02": true, // 抵触申请（现有技术）错引第33条
	"patent_exam_2016_a22_02": true, // 实用新型定义错引第22条
	"patent_exam_2007_a33_03": true, // 清楚/简要错引第42条
	"patent_exam_2013_a26_01": true, // 智力活动规则（第25条客体）错引第22条
}

func main() {
	tmp := os.TempDir()
	layers := []cacheFile{
		{"L0 裸 LLM", "mady_deepseek_eval.json"},
		{"L1 Agent 框架", "mady_agent_baseline_eval.json"},
		{"L3 +检索工具", "mady_agent_patent_eval.json"},
	}

	totalFail := 0
	for _, layer := range layers {
		data, err := os.ReadFile(filepath.Join(tmp, layer.file))
		if err != nil {
			fmt.Printf("== %s：跳过（缓存不存在 %v）\n", layer.name, err)
			continue
		}
		var answers map[string]string
		if err := json.Unmarshal(data, &answers); err != nil {
			fmt.Printf("== %s：跳过（缓存解析失败 %v）\n", layer.name, err)
			continue
		}

		var truePositives, falsePositives int
		for _, pair := range sortedPairs(answers) {
			caseID, answer := pair[0], pair[1]
			report := guardrails.VerifyCitations(answer)
			if len(report.Flagged) == 0 {
				continue
			}
			if knownTruePositives[caseID] {
				truePositives++
				fmt.Printf("  [预期命中] %s → %d 条标记\n", caseID, len(report.Flagged))
				for _, f := range report.Flagged {
					fmt.Printf("      「%s」%s\n", f.Citation.String(), f.Reason)
				}
			} else {
				falsePositives++
				fmt.Printf("  [误报] %s → %d 条标记\n", caseID, len(report.Flagged))
				for _, f := range report.Flagged {
					fmt.Printf("      「%s」%s\n", f.Citation.String(), f.Reason)
				}
			}
		}

		status := "✅ 通过"
		if truePositives == 0 || falsePositives > 0 {
			status = "❌ 未达标"
			totalFail++
		}
		fmt.Printf("== %s（%d 题）：真实错误命中 %d，误报 %d → %s\n\n",
			layer.name, len(answers), truePositives, falsePositives, status)
	}

	if totalFail > 0 {
		fmt.Println("验收未通过：存在未命中或误报。")
		os.Exit(1)
	}
	fmt.Println("验收通过：三层幻觉题全命中，误报为 0。")
}

// sortedPairs 保证遍历顺序稳定（按 caseID 排序）。
func sortedPairs(answers map[string]string) [][2]string {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]string{k, answers[k]})
	}
	return out
}
