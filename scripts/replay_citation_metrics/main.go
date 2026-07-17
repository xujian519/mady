// replay_citation_metrics 是引用类评测指标的离线回放工具（P1c，
// docs/design/citation-verification-gate.md §9 验收：指标口径等价）。
//
// 读取 $TMPDIR 下 v0.8 基线缓存的三层真实答案（L0/L1/L3 各 31 题），
// 逐题重算 citation_completeness 与 citation_validity，输出 JSON：
// 每层每题得分 + 均值。用途有二：
//   - metrics.go 同源重构（CitationCompleteness 改调 pkg/lawcite）前后各跑一次，
//     diff 两层 per-case 得分必须为空（口径等价硬性验收）；
//   - citation_validity 新指标的首批真实数据由此产出（写入 v0.9 基线报告）。
//
// 用法：
//
//	go run ./scripts/replay_citation_metrics > /tmp/before.json   # 重构前
//	go run ./scripts/replay_citation_metrics > /tmp/after.json    # 重构后
//	diff <(jq -S . /tmp/before.json) <(jq -S . /tmp/after.json)      # 必须为空
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/xujian519/mady/agentcore/evaluate"
	"github.com/xujian519/mady/agentcore/evaluate/benchmark"
)

// caseScore 是单题引用类指标得分。
type caseScore struct {
	Completeness float64 `json:"completeness"`
	Validity     float64 `json:"validity"`
}

// layerReport 是一层答案的回放结果。
type layerReport struct {
	MeanCompleteness float64              `json:"mean_completeness"`
	MeanValidity     float64              `json:"mean_validity"`
	Cases            map[string]caseScore `json:"cases"`
}

func main() {
	// 题库 caseID → required citations。
	required := make(map[string][]string)
	for _, c := range benchmark.AllCases() {
		required[c.ID] = c.RequiredCitations
	}

	layers := []struct {
		key  string
		file string
	}{
		{"L0", "mady_deepseek_eval.json"},
		{"L1", "mady_agent_baseline_eval.json"},
		{"L3", "mady_agent_patent_eval.json"},
	}

	out := make(map[string]layerReport)
	for _, layer := range layers {
		data, err := os.ReadFile(filepath.Join(os.TempDir(), layer.file))
		if err != nil {
			fmt.Fprintf(os.Stderr, "跳过 %s：缓存不存在 %v\n", layer.key, err)
			continue
		}
		var answers map[string]string
		if err := json.Unmarshal(data, &answers); err != nil {
			fmt.Fprintf(os.Stderr, "跳过 %s：缓存解析失败 %v\n", layer.key, err)
			continue
		}

		rep := layerReport{Cases: make(map[string]caseScore, len(answers))}
		// 按 caseID 排序累加，保证均值浮点求和顺序确定（重构前后快照可逐字节 diff）。
		ids := make([]string, 0, len(answers))
		for id := range answers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			answer := answers[id]
			cc := evaluate.CitationCompleteness{Required: required[id]}.Compute(answer, "")
			cv := evaluate.CitationValidity{}.Compute(answer, "")
			rep.Cases[id] = caseScore{Completeness: cc, Validity: cv}
			rep.MeanCompleteness += cc
			rep.MeanValidity += cv
		}
		n := float64(len(answers))
		rep.MeanCompleteness /= n
		rep.MeanValidity /= n
		out[layer.key] = rep
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "编码输出失败:", err)
		os.Exit(1)
	}

	// 摘要走 stderr，不污染 stdout 的 JSON（供 diff）。
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(os.Stderr, "%s：mean completeness=%.3f validity=%.3f（%d 题）\n",
			k, out[k].MeanCompleteness, out[k].MeanValidity, len(out[k].Cases))
	}
}
