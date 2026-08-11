package claimdrafting

import "testing"

// =============================================================================
// 单一性评分引擎（unity.yaml 移植）
// =============================================================================

func TestCheckUnity_SingleIndependent(t *testing.T) {
	claims := []Claim{{Number: 1, Kind: "independent", Preamble: "一种加热装置", Characterized: "包括加热元件"}}
	verdict := CheckUnity(claims)
	if !verdict.HasUnity {
		t.Error("单一独立权利要求应天然满足单一性")
	}
	if verdict.Grade != UnityGradeGood {
		t.Errorf("评分应为 good，got %s", verdict.Grade)
	}
}

func TestCheckUnity_NoIndependent(t *testing.T) {
	claims := []Claim{{Number: 1, Kind: "dependent", DependsOn: []int{1}, Limitation: "还包括显示模块"}}
	verdict := CheckUnity(claims)
	if !verdict.HasUnity {
		t.Error("无独立权利要求应天然满足单一性")
	}
}

func TestCheckUnity_RelatedClaims(t *testing.T) {
	claims := []Claim{
		{Number: 1, Kind: "independent", Preamble: "一种智能门锁", Characterized: "包括锁体、指纹识别模块、控制模块和驱动电机，指纹识别模块采集指纹后由控制模块驱动电机开合锁体"},
		{Number: 2, Kind: "independent", Preamble: "一种智能门锁的指纹解锁方法", Characterized: "通过指纹识别模块采集指纹，由控制模块比对指纹特征后驱动电机开合锁体"},
	}
	verdict := CheckUnity(claims)
	if !verdict.HasUnity {
		t.Errorf("共享大量技术特征的独立权利要求应满足单一性，pair=%+v", verdict.PairScores)
	}
	if verdict.Score < unityScoreFair {
		t.Errorf("评分应不低于一般线 60，got %.1f", verdict.Score)
	}
}

func TestCheckUnity_UnrelatedClaims(t *testing.T) {
	claims := []Claim{
		{Number: 1, Kind: "independent", Preamble: "一种太阳能发电装置", Characterized: "包括光伏板、逆变器和支架"},
		{Number: 2, Kind: "independent", Preamble: "一种中药煎煮设备", Characterized: "包括药罐、加热盘和温控器"},
	}
	verdict := CheckUnity(claims)
	if verdict.HasUnity {
		t.Errorf("技术主题无关的独立权利要求不应满足单一性，pair=%+v", verdict.PairScores)
	}
	if verdict.Grade != UnityGradePoor {
		t.Errorf("评级应为 poor（建议分案），got %s", verdict.Grade)
	}
	if len(verdict.Diagnostics) == 0 {
		t.Error("应有单一性诊断说明")
	}
}

func TestCheckUnity_PairNumbers(t *testing.T) {
	claims := []Claim{
		{Number: 1, Kind: "independent", Preamble: "一种装置A"},
		{Number: 5, Kind: "independent", Preamble: "一种装置B"},
	}
	verdict := CheckUnity(claims)
	if len(verdict.PairScores) != 1 {
		t.Fatalf("应产生 1 个权利要求对，got %d", len(verdict.PairScores))
	}
	p := verdict.PairScores[0]
	if p.LeftNumber != 1 || p.RightNumber != 5 {
		t.Errorf("权利要求对编号错误，got %d-%d", p.LeftNumber, p.RightNumber)
	}
}

func TestUnitySimilarity_Identical(t *testing.T) {
	text := "一种智能门锁包括锁体、指纹识别模块和控制模块"
	if sim := unitySimilarity(text, text); sim < 0.999 {
		t.Errorf("相同文本相似度应接近 1，got %.4f", sim)
	}
}

func TestUnitySimilarity_Disjoint(t *testing.T) {
	a := "一种太阳能发电装置包括光伏板和逆变器"
	b := "一种中药煎煮设备包括药罐和加热盘"
	sim := unitySimilarity(a, b)
	if sim > unitySimThreshold {
		t.Errorf("不相关文本相似度应低于阈值 0.6，got %.4f", sim)
	}
}

// =============================================================================
// unityInventionRule 规则接线
// =============================================================================

func TestUnityInventionRule_RelatedClaims(t *testing.T) {
	rule := &unityInventionRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种智能门锁", Characterized: "包括锁体、指纹识别模块、控制模块和驱动电机，指纹识别模块采集指纹后由控制模块驱动电机开合锁体"},
		{Number: 2, ClaimType: ClaimTypeMethod, Kind: "independent", Preamble: "一种智能门锁的指纹解锁方法", Characterized: "通过指纹识别模块采集指纹，由控制模块比对指纹特征后驱动电机开合锁体"},
	}
	if v := rule.Check(claims, DraftInput{}); len(v) != 0 {
		t.Errorf("相关权利要求不应产生违规，got %v", v)
	}
}

func TestUnityInventionRule_UnrelatedClaims(t *testing.T) {
	rule := &unityInventionRule{}
	claims := []Claim{
		{Number: 1, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种太阳能发电装置", Characterized: "包括光伏板、逆变器和支架"},
		{Number: 2, ClaimType: ClaimTypeProduct, Kind: "independent", Preamble: "一种中药煎煮设备", Characterized: "包括药罐、加热盘和温控器"},
	}
	v := rule.Check(claims, DraftInput{})
	if len(v) != 1 {
		t.Fatalf("不相关权利要求应产生 1 条违规，got %d", len(v))
	}
	if v[0].Severity != SeverityWarning {
		t.Errorf("严重程度应为 warning，got %s", v[0].Severity)
	}
}

// =============================================================================
// 数值范围端点实施例支持规则（CLA-001 第3条）
// =============================================================================

func TestSupportRangeEndpointRule_NoRange(t *testing.T) {
	rule := &supportRangeEndpointRule{}
	claims := []Claim{{Number: 1, Kind: "independent", Preamble: "一种加热装置", Characterized: "包括加热元件"}}
	if v := rule.Check(claims, DraftInput{Description: "加热元件为电阻丝"}); len(v) != 0 {
		t.Errorf("无数值范围不应产生违规，got %v", v)
	}
}

func TestSupportRangeEndpointRule_MissingEndpoints(t *testing.T) {
	rule := &supportRangeEndpointRule{}
	claims := []Claim{{Number: 1, Kind: "independent", Preamble: "一种加热装置", Characterized: "工作温度范围为60-80℃"}}
	v := rule.Check(claims, DraftInput{Description: "加热装置通过电阻丝加热"})
	if len(v) != 1 {
		t.Fatalf("说明书缺少端点实施例应产生 1 条违规，got %d", len(v))
	}
	if v[0].Severity != SeverityWarning {
		t.Errorf("严重程度应为 warning，got %s", v[0].Severity)
	}
}

func TestSupportRangeEndpointRule_EndpointsCovered(t *testing.T) {
	rule := &supportRangeEndpointRule{}
	claims := []Claim{{Number: 1, Kind: "independent", Preamble: "一种加热装置", Characterized: "工作温度范围为60-80℃"}}
	if v := rule.Check(claims, DraftInput{Description: "实施例1温度为60℃，实施例2温度为80℃"}); len(v) != 0 {
		t.Errorf("说明书已覆盖端点不应产生违规，got %v", v)
	}
}

func TestSupportRangeEndpointRule_PartialCoverage(t *testing.T) {
	rule := &supportRangeEndpointRule{}
	claims := []Claim{{Number: 1, Kind: "independent", Preamble: "一种加热装置", Characterized: "工作温度范围为60-80℃"}}
	v := rule.Check(claims, DraftInput{Description: "实施例温度为60℃"})
	if len(v) != 1 {
		t.Fatalf("部分覆盖应产生 1 条 info 违规，got %d", len(v))
	}
	if v[0].Severity != SeverityInfo {
		t.Errorf("部分覆盖严重程度应为 info，got %s", v[0].Severity)
	}
}

func TestSupportRangeEndpointRule_EndpointBoundaryNoFalsePositive(t *testing.T) {
	// 端点 "10" 不得被说明书 "100mm" 等更长数值串误匹配（裸 Contains 的 false positive）。
	rule := &supportRangeEndpointRule{}
	claims := []Claim{{Number: 1, Kind: "independent", Preamble: "一种加热装置", Characterized: "壁厚范围为10-20mm"}}
	v := rule.Check(claims, DraftInput{Description: "实施例1壁厚为100mm"})
	if len(v) != 1 {
		t.Fatalf("端点 10/20 均未覆盖应产生 1 条 warning 违规，got %d（若端点被 '100mm' 误匹配则会降级/消失）", len(v))
	}
	if v[0].Severity != SeverityWarning {
		t.Errorf("全部端点缺失严重程度应为 warning，got %s", v[0].Severity)
	}
}
