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

func TestDetectOaRejectionTypes(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []OaRejectionType
	}{
		{
			"single_novelty",
			"权利要求1不具备新颖性（专利法第22条第2款）",
			[]OaRejectionType{OaNovelty},
		},
		{
			"mixed_novelty_then_inventiveness",
			"权利要求1-3不具备新颖性（第22条第2款）。权利要求4-5相对于对比文件1和2的结合不具备创造性（第22条第3款）。",
			[]OaRejectionType{OaNovelty, OaInventiveness},
		},
		{
			"mixed_inventiveness_then_clarity",
			"权利要求1不具备创造性。权利要求2不清楚，不符合26条第4款。",
			[]OaRejectionType{OaInventiveness, OaClarity},
		},
		{
			"order_follows_text",
			"权利要求5不清楚。权利要求1不具备新颖性。",
			[]OaRejectionType{OaClarity, OaNovelty},
		},
		{
			"dedup_same_category",
			"权利要求1不具备新颖性，权利要求3也不具备新颖性（第22条第2款）。",
			[]OaRejectionType{OaNovelty},
		},
		{
			"none",
			"这是一段普通文字",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectOaRejectionTypes(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("DetectOaRejectionTypes(%q) = %v, want %v", tt.text, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("DetectOaRejectionTypes(%q)[%d] = %v, want %v", tt.text, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDetectOaRejectionType_BackwardCompat(t *testing.T) {
	// 单类型文本仍返回该类型；多类型文本返回第一个（按文本位置）。
	if got := DetectOaRejectionType("该权利要求不具备创造性"); got != OaInventiveness {
		t.Errorf("got %v, want inventiveness", got)
	}
	if got := DetectOaRejectionType("权利要求1不清楚，权利要求2不具备新颖性"); got != OaClarity {
		t.Errorf("got %v, want clarity (first in text order)", got)
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
		{"claim_range_no_prefix", "权利要求1-3不具备新颖性", []int{1, 2, 3}},
		{"claim_range_zh_no_prefix", "权利要求1至3不具备创造性", []int{1, 2, 3}},
		{"mixed", "权利要求2不符合规定，第4-6项也不符合", []int{2, 4, 5, 6}},
		{"claim_range_and_single", "权利要求1-2和权利要求5均被对比文件公开", []int{1, 2, 5}},
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
	if len(oa.RejectionTypes) != 1 || oa.RejectionTypes[0] != OaInventiveness {
		t.Errorf("rejection types = %v, want [inventiveness]", oa.RejectionTypes)
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

func TestParseOfficeAction_MixedRejections(t *testing.T) {
	text := `权利要求1-3相对于对比文件1（CN123456A）不具备新颖性，不符合专利法第22条第2款的规定。
权利要求4相对于对比文件1和对比文件2（US789012B）的结合不具备创造性，不符合专利法第22条第3款的规定。
权利要求5不清楚，不符合专利法第26条第4款的规定。`

	oa := ParseOfficeAction(text)

	if len(oa.RejectionTypes) != 3 {
		t.Fatalf("rejection types = %v, want 3 distinct types", oa.RejectionTypes)
	}
	want := []OaRejectionType{OaNovelty, OaInventiveness, OaClarity}
	for i := range want {
		if oa.RejectionTypes[i] != want[i] {
			t.Errorf("rejection types[%d] = %v, want %v", i, oa.RejectionTypes[i], want[i])
		}
	}
	if oa.RejectionType != OaNovelty {
		t.Errorf("primary rejection type = %v, want novelty", oa.RejectionType)
	}
	wantClaims := []int{1, 2, 3, 4, 5}
	if len(oa.AffectedClaims) != len(wantClaims) {
		t.Fatalf("affected claims = %v, want %v", oa.AffectedClaims, wantClaims)
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
