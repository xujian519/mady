package amendment

import (
	"testing"
)

func TestChecker_NoChanges(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		OriginalClaims: "一种装置，其特征在于，包括A。",
		AmendedClaims:  "一种装置，其特征在于，包括A。",
	})
	if !result.IsCompliant {
		t.Errorf("无修改应合规，但 IsCompliant=%v", result.IsCompliant)
	}
}

func TestChecker_WithChanges(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		OriginalClaims: "一种装置，其特征在于，包括A。",
		AmendedClaims:  "一种装置，其特征在于，包括A和B。",
	})
	if !result.HasClaimChanges {
		t.Error("应检测到权利要求修改")
	}
}

func TestChecker_PassiveWithoutOA(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		OriginalClaims:   "一种装置，其特征在于，包括A。",
		AmendedClaims:    "一种装置，其特征在于，包括A和B。",
		ModificationType: ModPassive,
	})
	if result.IsCompliant {
		t.Error("被动修改未提供OA文本应标记为不合规")
	}
	if !containsViolation(result.Violations, "amendment-passive-oa-required") {
		t.Error("应检测到 amendment-passive-oa-required 违规")
	}
}

func TestChecker_PassiveWithOA(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		OriginalClaims:   "一种装置，其特征在于，包括A。",
		AmendedClaims:    "一种装置，其特征在于，包括A和B。",
		ModificationType: ModPassive,
		OfficeActionText: "权利要求1不具备创造性，不符合专利法第22条第3款的规定。",
	})
	if len(result.Violations) != 0 {
		t.Errorf("提供有效OA文本时不应有违规，got %d violations", len(result.Violations))
	}
}

func TestChecker_NoOriginal(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		AmendedClaims: "一种装置，其特征在于，包括A。",
	})
	if result.IsCompliant {
		t.Error("未提供原始文件时应标记为不合规")
	}
	if !containsViolation(result.Violations, "amendment-basic-input") {
		t.Error("应检测到 amendment-basic-input 违规")
	}
}

func TestChecker_EmptyInput(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{})
	if !result.IsCompliant {
		t.Error("空输入应合规")
	}
	if result.Note == "" {
		t.Error("空输入应有说明性 Note")
	}
}

func TestChecker_HasSpecChanges(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		OriginalSpec: "本发明涉及一种装置。",
		AmendedSpec:  "本发明涉及一种改进的装置。",
	})
	if !result.HasSpecChanges {
		t.Error("应检测到说明书修改")
	}
}

func TestChecker_PassiveWithMalformedOA(t *testing.T) {
	c := NewChecker()
	result := c.Check(CheckInput{
		OriginalClaims:   "一种装置，其特征在于，包括A。",
		AmendedClaims:    "一种装置，其特征在于，包括A和B。",
		ModificationType: ModPassive,
		OfficeActionText: "审查员认为该申请存在一些问题。",
	})
	// 文本不包含驳回关键词，应告警 info
	if !containsViolation(result.Violations, "amendment-passive-oa-content") {
		t.Error("OA文本不含驳回关键词时应告警")
	}
}

func containsViolation(vv []Violation, name string) bool {
	for _, v := range vv {
		if v.RuleName == name {
			return true
		}
	}
	return false
}
