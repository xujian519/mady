package benchmark

import (
	"strings"
	"testing"
)

// 说明：本文件是 evaluate/benchmark 数据集的特征测试。
// patent_exam_real_a22.go（619 行）是纯数据文件，拆分时最大的风险是「用例静默丢失」
// ——suite_test.go 的 CaseIntegrity 只能发现空字段与重复 ID，丢整条用例仍会通过。
// 这里用「精确条数 + ID 形态 + 领域一致」把当前数据集钉住。

func TestPatentExamRealA22Cases_DatasetShape(t *testing.T) {
	const wantCount = 15 // 拆分前的用例条数；新增用例须同步更新并说明来源
	if got := len(PatentExamRealA22Cases); got != wantCount {
		t.Fatalf("PatentExamRealA22Cases 条数 = %d, want %d（拆分数据文件时用例不得丢失）", got, wantCount)
	}

	seen := make(map[string]bool, len(PatentExamRealA22Cases))
	for i, c := range PatentExamRealA22Cases {
		if !strings.HasPrefix(c.ID, "patent_exam_") || !strings.Contains(c.ID, "_a22_") {
			t.Errorf("第 %d 条 ID = %q，应形如 patent_exam_<年份>_a22_<序号>", i, c.ID)
		}
		if seen[c.ID] {
			t.Errorf("ID 重复: %q", c.ID)
		}
		seen[c.ID] = true
		if c.Domain != "patent" {
			t.Errorf("用例 %s 的 Domain = %q, want \"patent\"", c.ID, c.Domain)
		}
		if strings.TrimSpace(c.Input) == "" || strings.TrimSpace(c.Expected) == "" {
			t.Errorf("用例 %s 的 Input/Expected 不应为空", c.ID)
		}
	}
}
