// Command export-benchmark 将 Mady evaluate/benchmark 的专利代理人考试评测集
// 导出为 JSON fixture（每个 suite 一个文件，字段 camelCase），供 Sati 等项目复用。
//
// 用法:
//
//	go run ./cmd/export-benchmark -out /path/to/sati/tests/patent/benchmark/fixtures
//
// 输出: <suite>.json x N + index.json（含总用例数与各 suite 数量）。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xujian519/mady/evaluate"
	"github.com/xujian519/mady/evaluate/benchmark"
)

const sourceNote = "Mady evaluate/benchmark (github.com/xujian519/mady)"

// exportCase 与 Sati 端 TS 类型 tests/patent/benchmark/types.ts 一一对应。
type exportCase struct {
	ID                string   `json:"id"`
	Domain            string   `json:"domain"`
	Input             string   `json:"input"`
	Expected          string   `json:"expected"`
	RequiredCitations []string `json:"requiredCitations,omitempty"`
	Era               string   `json:"era,omitempty"`
	KnowledgeCutoff   string   `json:"knowledgeCutoff,omitempty"`
	Difficulty        string   `json:"difficulty,omitempty"`
}

type fixtureFile struct {
	Suite       string       `json:"suite"`
	Description string       `json:"description,omitempty"`
	Source      string       `json:"source"`
	GeneratedAt string       `json:"generatedAt"`
	CaseCount   int          `json:"caseCount"`
	Cases       []exportCase `json:"cases"`
}

type indexEntry struct {
	Suite     string `json:"suite"`
	CaseCount int    `json:"caseCount"`
}

type indexFile struct {
	GeneratedAt string       `json:"generatedAt"`
	Source      string       `json:"source"`
	TotalCases  int          `json:"totalCases"`
	Suites      []indexEntry `json:"suites"`
}

type suite struct {
	name, desc string
	cases      []evaluate.TestCase
}

func convert(cases []evaluate.TestCase) []exportCase {
	out := make([]exportCase, 0, len(cases))
	for _, c := range cases {
		out = append(out, exportCase{
			ID:                c.ID,
			Domain:            c.Domain,
			Input:             c.Input,
			Expected:          c.Expected,
			RequiredCitations: c.RequiredCitations,
			Era:               c.Era,
			KnowledgeCutoff:   c.KnowledgeCutoff,
			Difficulty:        c.Difficulty,
		})
	}
	return out
}

func main() {
	outDir := flag.String("out", "fixtures", "输出目录（Sati tests/patent/benchmark/fixtures）")
	flag.Parse()

	suites := []suite{
		{"patent-exam-mock", "结构化模拟题（新颖性/创造性/权项解释/OA答复/侵权等考点，Mady Golden Benchmark 第一层）", benchmark.PatentExamCases},
		{"patent-exam-real-a2", "2007-2019 全国专利代理人资格考试《专利代理实务》真题（专利法第2条考点）", benchmark.PatentExamRealA2Cases},
		{"patent-exam-real-a22", "2007-2019 全国专利代理人资格考试《专利代理实务》真题（专利法第22条考点）", benchmark.PatentExamRealA22Cases},
		{"patent-exam-real-a26", "2007-2019 全国专利代理人资格考试《专利代理实务》真题（专利法第26条第4款考点）", benchmark.PatentExamRealA26Cases},
		{"patent-exam-real-a26-3", "全国专利代理人资格考试《专利代理实务》真题及无效决定（专利法第26条第3款考点）", benchmark.PatentExamRealA26_3Cases},
		{"patent-exam-real-a31", "2007-2019 全国专利代理人资格考试《专利代理实务》真题（专利法第31条考点）", benchmark.PatentExamRealA31Cases},
		{"patent-exam-real-a33", "2007-2019 全国专利代理人资格考试《专利代理实务》真题（专利法第33条考点）", benchmark.PatentExamRealA33Cases},
		{"patent-exam-real-r42", "2007-2019 全国专利代理人资格考试《专利代理实务》真题（实施细则第42条考点）", benchmark.PatentExamRealR42Cases},
		{"patent-invalidation-decisions", "100 件真实 CNIPA 无效宣告请求审查决定书（选自宝宸知识库_Raw 数据集 31562 件）", benchmark.InvalidationDecisionCases},
		{"patent-design-invalidation", "外观设计无效宣告 5 场景（整体观察/综合判断）", benchmark.DesignInvalidationCases},
	}

	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
		os.Exit(1)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	idx := indexFile{GeneratedAt: now, Source: sourceNote}
	total := 0

	for _, s := range suites {
		ff := fixtureFile{
			Suite:       s.name,
			Description: s.desc,
			Source:      sourceNote,
			GeneratedAt: now,
			CaseCount:   len(s.cases),
			Cases:       convert(s.cases),
		}
		raw, err := json.MarshalIndent(ff, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "序列化失败:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join(*outDir, s.name+".json"), raw, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			os.Exit(1)
		}
		idx.Suites = append(idx.Suites, indexEntry{Suite: s.name, CaseCount: len(s.cases)})
		total += len(s.cases)
	}
	idx.TotalCases = total

	raw, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "序列化失败:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(*outDir, "index.json"), raw, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}

	fmt.Printf("导出完成: %d 个用例 / %d 个 suite → %s\n", total, len(suites), *outDir)
}
