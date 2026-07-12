package rules

import (
	"testing"
)

func TestDetectOaRejectionType(t *testing.T) {
	tests := []struct {
		name string
		text string
		want OaRejectionType
	}{
		{"inventiveness", "该权利要求不具备创造性", OaInventiveness},
		{"inventiveness_obvious", "对本领域技术人员而言显而易见", OaInventiveness},
		{"inventiveness_article", "不符合专利法22条第3款的规定", OaInventiveness},
		{"novelty", "该技术方案不具备新颖性", OaNovelty},
		{"novelty_article", "不符合22条第2款", OaNovelty},
		{"clarity", "权利要求保护范围不清楚", OaClarity},
		{"clarity_article", "不符合26条第4款", OaClarity},
		{"disclosure", "说明书公开不充分", OaDisclosure},
		{"disclosure_article", "不符合26条第3款，无法实现", OaDisclosure},
		{"support", "权利要求得不到说明书支持", OaSupport},
		{"scope", "保护范围过宽，不符合33条", OaScope},
		{"scope_amend", "修改超范围，不符合33条", OaScope},
		{"formal", "存在明显的格式形式错误", OaFormal},
		{"other", "这是一段普通文字", OaOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectOaRejectionType(tt.text)
			if got != tt.want {
				t.Errorf("DetectOaRejectionType(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestExtractCitations(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			"single_cn",
			"对比文件CN101234567A公开了...",
			[]string{"CN101234567A"},
		},
		{
			"multiple_mixed",
			"CN101234567A和US2009012345A均公开了该技术",
			[]string{"CN101234567A", "US2009012345A"},
		},
		{
			"duplicate_dedup",
			"CN101234567A公开了... CN101234567A进一步揭示",
			[]string{"CN101234567A"},
		},
		{
			"none",
			"没有引用任何文献",
			nil,
		},
		{
			"wo_patent",
			"WO2015000123A公开",
			[]string{"WO2015000123A"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cits := ExtractCitations(tt.text)
			if len(cits) != len(tt.want) {
				t.Fatalf("got %d citations, want %d", len(cits), len(tt.want))
			}
			for i, c := range cits {
				if c.DocumentNumber != tt.want[i] {
					t.Errorf("[%d] got %q, want %q", i, c.DocumentNumber, tt.want[i])
				}
			}
		})
	}
}

func TestExtractAffectedClaims(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []int
	}{
		{"single", "权利要求1不具备新颖性", []int{1}},
		{"multiple", "权利要求1和权利要求3不具备创造性", []int{1, 3}},
		{"range", "第1-5项权利要求", []int{1, 2, 3, 4, 5}},
		{"range_zh", "第1至3项", []int{1, 2, 3}},
		{"mixed", "权利要求2不符合规定，第4-6项也不符合", []int{2, 4, 5, 6}},
		{"dedup", "权利要求1...权利要求1...权利要求1", []int{1}},
		{"none", "没有任何权利要求", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAffectedClaims(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("[%d] got %d, want %d", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestParseOfficeAction(t *testing.T) {
	text := `审查员认为第1-3项权利要求不具备创造性，不符合专利法22条第3款的规定。
对比文件CN101234567A（X类）公开了区别技术特征。`

	oa := ParseOfficeAction(text)

	if oa.RejectionType != OaInventiveness {
		t.Errorf("rejection type = %v, want inventiveness", oa.RejectionType)
	}
	if len(oa.Citations) != 1 || oa.Citations[0].DocumentNumber != "CN101234567A" {
		t.Errorf("citations = %v, want [CN101234567A]", oa.Citations)
	}
	want := []int{1, 2, 3}
	if len(oa.AffectedClaims) != len(want) {
		t.Fatalf("affected claims = %v, want %v", oa.AffectedClaims, want)
	}
	for i, c := range oa.AffectedClaims {
		if c != want[i] {
			t.Errorf("[%d] got %d, want %d", i, c, want[i])
		}
	}
}

func TestFormatOaSummary(t *testing.T) {
	oa := ParsedOfficeAction{
		RejectionType:  OaInventiveness,
		AffectedClaims: []int{1, 2, 3},
		Citations: []CitedReference{
			{DocumentNumber: "CN101234567A", Relevancy: "X"},
		},
	}
	s := FormatOaSummary(oa)
	if s == "" {
		t.Fatal("FormatOaSummary returned empty string")
	}
}
