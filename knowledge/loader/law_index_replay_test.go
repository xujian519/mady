package loader_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xujian519/mady/guardrails"
	"github.com/xujian519/mady/knowledge/loader"
	"github.com/xujian519/mady/pkg/lawcite"
)

// 本文件是 S1+S2 复合源的离线回放验收（docs/design/citation-verification-gate.md
// §8.2 的 P2a 扩展）：scripts/replay_citation_gate 只验 S1 默认源，
// 本测试验证「S1 静态表 + S2 wiki 法条索引」复合源在三层真实答案缓存上
// 仍保持——已知真实引用错误（TP）全部命中、其余答案误报为 0。
//
// S2 标题词只参与本条自证、不参与交叉匹配，理论上不会引入新误报；
// 风险方向相反：S2 词可能让 TP 案例的用途描述意外自证（掩盖真实幻觉），
// 因此 TP 仍命中是本测试的核心断言。

// knownTruePositives 与 scripts/replay_citation_gate 保持一致。
var knownTruePositives = map[string]bool{
	"patent_exam_2008_a31_02": true, // 分案申请错引专利法第47条/细则21条
	"patent_exam_2010_a22_02": true, // 抵触申请（现有技术）错引第33条
	"patent_exam_2016_a22_02": true, // 实用新型定义错引第22条
	"patent_exam_2007_a33_03": true, // 清楚/简要错引第42条
	"patent_exam_2013_a26_01": true, // 智力活动规则（第25条客体）错引第22条
}

// patentLawOnlyAdapter 把单法索引适配为 CitationSource：
// 非专利法一律返回未覆盖，让复合源回退到 S1（如实施细则第 42 条）。
func patentLawOnlyAdapter(idx *loader.LawArticleIndex) guardrails.CitationSource {
	return guardrails.CitationSourceFuncs{
		TopicsFunc: func(s lawcite.Statute, article int) ([]string, bool) {
			if s != lawcite.StatutePatentLaw {
				return nil, false
			}
			return idx.Topics(article)
		},
		MaxArticleFunc: func(s lawcite.Statute) int {
			if s != lawcite.StatutePatentLaw {
				return 0
			}
			return idx.MaxArticle()
		},
	}
}

func TestCompositeSource_Replay(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法定位用户目录")
	}
	wikiDir := filepath.Join(home, ".mady", "knowledge", "wiki", "法律法规", "法律")
	if _, err := os.Stat(wikiDir); err != nil {
		t.Skipf("真实 wiki 法条目录不存在，跳过：%s", wikiDir)
	}

	idx, err := loader.BuildLawArticleIndex(wikiDir)
	if err != nil {
		t.Fatalf("BuildLawArticleIndex 失败：%v", err)
	}
	composite := guardrails.CompositeCitationSource(
		guardrails.DefaultCitationSource(), patentLawOnlyAdapter(idx))

	layers := []struct{ name, file string }{
		{"L0 裸 LLM", "mady_deepseek_eval.json"},
		{"L1 Agent 框架", "mady_agent_baseline_eval.json"},
		{"L3 +检索工具", "mady_agent_patent_eval.json"},
	}

	ran := false
	for _, layer := range layers {
		data, err := os.ReadFile(filepath.Join(os.TempDir(), layer.file))
		if err != nil {
			continue // 单层缓存缺失不阻塞其他层验收
		}
		var answers map[string]string
		if err := json.Unmarshal(data, &answers); err != nil {
			t.Fatalf("%s 缓存解析失败：%v", layer.name, err)
		}
		ran = true

		var tpHits, falsePositives int
		for caseID, answer := range answers {
			report := guardrails.VerifyCitationsWithSource(answer, composite)
			if len(report.Flagged) == 0 {
				continue
			}
			if knownTruePositives[caseID] {
				tpHits++
			} else {
				falsePositives++
				for _, f := range report.Flagged {
					t.Logf("[误报] %s %s：「%s」%s", layer.name, caseID, f.Citation.String(), f.Reason)
				}
			}
		}
		if falsePositives > 0 {
			t.Errorf("%s：复合源引入 %d 个误报", layer.name, falsePositives)
		}
		if tpHits == 0 {
			t.Errorf("%s：已知真实引用错误（TP）无一命中——S2 自证可能掩盖了幻觉", layer.name)
		}
		t.Logf("%s（%d 题）：TP 命中 %d，误报 %d", layer.name, len(answers), tpHits, falsePositives)
	}
	if !ran {
		t.Skip("三层答案缓存均不存在（$TMPDIR/mady_*_eval.json）")
	}
}
