package evidence

import (
	"testing"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
)

// ── 互联网公开类型测试 ──────────────────────────────────────

func TestDefaultEngine_Judge_InternetPublication(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:          "test-internet-pub",
		SourceURI:   "web_pub:https://www.cnipa.gov.cn/notice/important-announcement",
		DocVersion:  "2023-06-15",
		Direction:   agentcore_evidence.DirectionSupporting,
		ContentHash: "abc123hash",
		Snippet:     "关于专利审查指南的最新修订通知...",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.EvidenceType != EvTypeInternetPublication {
		t.Errorf("EvidenceType = %q, 期望 %q", judgment.TypeSpecificJudgment.EvidenceType, EvTypeInternetPublication)
	}

	// 检查互联网公开特定字段
	if judgment.TypeSpecificJudgment.DateDetermination == nil {
		t.Error("互联网公开的 DateDetermination 不应为 nil")
	} else {
		if judgment.TypeSpecificJudgment.DateDetermination.Reliability != RelHigh {
			t.Errorf("精确日期可靠性应 = %q, 实际 = %q", RelHigh, judgment.TypeSpecificJudgment.DateDetermination.Reliability)
		}
	}

	if judgment.TypeSpecificJudgment.PlatformCredibility == nil {
		t.Error("互联网公开的 PlatformCredibility 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.ContentIntegrity != IntegrityVerified {
		t.Errorf("有内容哈希时 ContentIntegrity 应 = %q, 实际 = %q", IntegrityVerified, judgment.TypeSpecificJudgment.ContentIntegrity)
	}

	if judgment.TypeSpecificJudgment.PlatformCategory == "" {
		t.Error("互联网公开的 PlatformCategory 不应为空")
	}
}

func TestDefaultEngine_Judge_InternetPublication_Wayback(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:        "test-wayback",
		SourceURI: "web_pub:https://web.archive.org/web/20230615000000/https://example.com/doc",
		Direction: agentcore_evidence.DirectionSupporting,
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.EvidenceType != EvTypeInternetPublication {
		t.Errorf("EvidenceType = %q, 期望 %q", judgment.TypeSpecificJudgment.EvidenceType, EvTypeInternetPublication)
	}

	// Wayback Machine 存档应被识别
	if judgment.TypeSpecificJudgment.PlatformCategory != "网页存档平台" {
		t.Errorf("Wayback 平台分类 = %q, 期望 %q", judgment.TypeSpecificJudgment.PlatformCategory, "网页存档平台")
	}
}

func TestDefaultEngine_Judge_InternetPublication_RestrictedAccess(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:        "test-restricted",
		SourceURI: "web_pub:https://www.wsj.com/articles/tech-news-2023",
		Direction: agentcore_evidence.DirectionSupporting,
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	// WSJ 有付费墙
	if judgment.TypeSpecificJudgment.PublicIntent != IntentRestricted {
		t.Errorf("WSJ 公开意图应 = %q, 实际 = %q", IntentRestricted, judgment.TypeSpecificJudgment.PublicIntent)
	}
}

// ── 使用公开类型测试 ──────────────────────────────────────

func TestDefaultEngine_Judge_PublicUse(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:         "test-public-use",
		SourceURI:  "pub_use:sales-record-001",
		DocVersion: "2023-01-15",
		Direction:  agentcore_evidence.DirectionSupporting,
		Snippet:    "2023年1月15日在上海展会上公开销售了该产品，无保密要求",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.EvidenceType != EvTypePublicUse {
		t.Errorf("EvidenceType = %q, 期望 %q", judgment.TypeSpecificJudgment.EvidenceType, EvTypePublicUse)
	}

	// 检查四要件
	if judgment.TypeSpecificJudgment.FourElementsCheck == nil {
		t.Fatal("使用公开的 FourElementsCheck 不应为 nil")
	}

	// 销售 + 上海(国内) + 无保密 → 四要件应基本满足
	if !judgment.TypeSpecificJudgment.FourElementsCheck.TimeElement.Met {
		t.Error("时间要件应满足（有明确日期）")
	}
	if !judgment.TypeSpecificJudgment.FourElementsCheck.PlaceElement.Met {
		t.Error("地点要件应满足（提到上海）")
	}
	if !judgment.TypeSpecificJudgment.FourElementsCheck.MethodElement.Met {
		t.Error("方式要件应满足（提到销售）")
	}

	// 举证难度和证据链
	if judgment.TypeSpecificJudgment.BurdenDifficulty == "" {
		t.Error("BurdenDifficulty 不应为空")
	}
	if judgment.TypeSpecificJudgment.ChainIntegrity == "" {
		t.Error("ChainIntegrity 不应为空")
	}
}

func TestDefaultEngine_Judge_PublicUse_Confidential(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:         "test-confidential",
		SourceURI:  "pub_use:internal-test-001",
		DocVersion: "2023-06-01",
		Direction:  agentcore_evidence.DirectionSupporting,
		Snippet:    "在内部测试中使用了该技术方案，签有保密协议(NDA)",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.FourElementsCheck == nil {
		t.Fatal("FourElementsCheck 不应为 nil")
	}

	// 保密协议 → 可获取性不应满足
	if judgment.TypeSpecificJudgment.FourElementsCheck.Accessibility.Met {
		t.Error("存在保密协议时，公众可获取性不应满足")
	}

	// 四要件不应全部满足
	if judgment.TypeSpecificJudgment.FourElementsCheck.AllMet() {
		t.Error("存在保密协议时，四要件不应全部满足")
	}
}

func TestDefaultEngine_Judge_PublicUse_Exhibition(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:         "test-exhibition",
		SourceURI:  "pub_use:expo-record-2023",
		DocVersion: "2023-03-15",
		Direction:  agentcore_evidence.DirectionSupporting,
		Snippet:    "2023年3月在美国CES展会上公开演示了原型产品，对公众开放",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	// 检查方式要件：展览
	if !judgment.TypeSpecificJudgment.FourElementsCheck.MethodElement.Met {
		t.Error("方式要件应满足（提到展览/演示）")
	}

	// 检查地点要件：境外的美国
	if !judgment.TypeSpecificJudgment.FourElementsCheck.PlaceElement.Met {
		t.Error("地点要件应满足（提到美国CES展会）")
	}
}

// ── inferEvidenceType 测试 ─────────────────────────────────

func TestInferEvidenceType_NewTypes(t *testing.T) {
	tests := []struct {
		uri  string
		want EvidenceType
	}{
		{"", EvTypeGeneral},
		{"web_pub:https://example.com/doc", EvTypeInternetPublication},
		{"http_archive:https://archive.org/doc", EvTypeInternetPublication},
		{"pub_use:invoice-2023", EvTypePublicUse},
		{"public_use:exhibition-record", EvTypePublicUse},
		{"https://example.com/doc", EvTypeElectronic},
		{"web:https://example.com", EvTypeElectronic},
		{"witness:testimony-001", EvTypeWitness},
		{"patent:CN12345678A", EvTypePriorArtDate},
		{"prior_art:reference", EvTypePriorArtDate},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := inferEvidenceType(tt.uri)
			if got != tt.want {
				t.Errorf("inferEvidenceType(%q) = %q, 期望 %q", tt.uri, got, tt.want)
			}
		})
	}
}

// ── 分类和完整性辅助函数测试 ──────────────────────────────

func TestClassifyInternetPlatform(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"https://www.cnipa.gov.cn/notice/123", "政府/专利局官方平台"},
		{"https://www.cnki.net/article/123", "学术/教育平台"},
		{"https://www.xinhuanet.com/tech/123", "新闻媒体"},
		{"https://web.archive.org/web/20230615000000/doc", "网页存档平台"},
		{"https://mp.weixin.qq.com/s/doc", "内容平台"},
		{"https://weibo.com/u/123", "社交媒体"},
		{"https://github.com/user/repo", "代码托管平台"},
		{"https://www.google.com/search?q=patent", "搜索引擎"},
		{"", "未知"},
		{"https://unknown-site.com/page", "其他互联网平台"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			got := classifyInternetPlatform(tt.uri)
			if got != tt.want {
				t.Errorf("classifyInternetPlatform(%q) = %q, 期望 %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestEvaluateInternetContentIntegrity(t *testing.T) {
	tests := []struct {
		name string
		span agentcore_evidence.EvidenceSpan
		want ContentIntegrityStatus
	}{
		{
			"有内容哈希",
			agentcore_evidence.EvidenceSpan{ContentHash: "abc123", SourceURI: "https://example.com"},
			IntegrityVerified,
		},
		{
			"Wayback Machine存档",
			agentcore_evidence.EvidenceSpan{SourceURI: "https://web.archive.org/web/20230615000000/doc"},
			IntegrityPartial,
		},
		{
			"普通网页无哈希",
			agentcore_evidence.EvidenceSpan{SourceURI: "https://example.com"},
			IntegrityUnverified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateInternetContentIntegrity(tt.span)
			if got != tt.want {
				t.Errorf("evaluateInternetContentIntegrity = %q, 期望 %q", got, tt.want)
			}
		})
	}
}

func TestEvaluatePublicIntent(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want PublicIntent
	}{
		{"政府网站", "https://www.cnipa.gov.cn/notice/123", IntentPublic},
		{"WSJ付费墙", "https://www.wsj.com/articles/tech-news", IntentRestricted},
		{"Springer学术", "https://link.springer.com/article/123", IntentRestricted},
		{"空URI", "", IntentPublic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := agentcore_evidence.EvidenceSpan{SourceURI: tt.uri}
			got := evaluatePublicIntent(span)
			if got != tt.want {
				t.Errorf("evaluatePublicIntent(%q) = %q, 期望 %q", tt.uri, got, tt.want)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s        string
		keywords []string
		want     bool
	}{
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"world"}, true},
		{"hello world", []string{"foo", "bar"}, false},
		{"", []string{"hello"}, false},
		{"销售合同", []string{"销售", "合同"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := containsAny(tt.s, tt.keywords)
			if got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v, 期望 %v", tt.s, tt.keywords, got, tt.want)
			}
		})
	}
}

func TestFourElementsResult(t *testing.T) {
	// 测试 AllMet 和 OverallScore
	allMet := &FourElementsResult{
		TimeElement:   ElementResult{Met: true, Score: 0.9},
		PlaceElement:  ElementResult{Met: true, Score: 0.85},
		MethodElement: ElementResult{Met: true, Score: 0.9},
		Accessibility: ElementResult{Met: true, Score: 0.9},
	}

	if !allMet.AllMet() {
		t.Error("全部满足时 AllMet 应为 true")
	}

	score := allMet.OverallScore()
	if score <= 0 {
		t.Errorf("OverallScore 应 > 0, 实际 = %f", score)
	}
	if score > 1.0 {
		t.Errorf("OverallScore 应 <= 1.0, 实际 = %f", score)
	}

	// 部分满足
	partial := &FourElementsResult{
		TimeElement:   ElementResult{Met: true, Score: 0.9},
		PlaceElement:  ElementResult{Met: false, Score: 0.3},
		MethodElement: ElementResult{Met: true, Score: 0.9},
		Accessibility: ElementResult{Met: false, Score: 0.2},
	}

	if partial.AllMet() {
		t.Error("部分不满足时 AllMet 应为 false")
	}

	// nil 值
	var nilResult *FourElementsResult
	if nilResult.AllMet() {
		t.Error("nil 时 AllMet 应为 false")
	}
	if nilResult.OverallScore() != 0 {
		t.Error("nil 时 OverallScore 应为 0")
	}
}

func TestAssessPublicUseBurdenDifficulty(t *testing.T) {
	tests := []struct {
		name string
		four *FourElementsResult
		want string
	}{
		{"全部满足", &FourElementsResult{
			TimeElement:   ElementResult{Met: true},
			PlaceElement:  ElementResult{Met: true},
			MethodElement: ElementResult{Met: true},
			Accessibility: ElementResult{Met: true},
		}, "中"},
		{"部分满足", &FourElementsResult{
			TimeElement:   ElementResult{Met: true},
			PlaceElement:  ElementResult{Met: true},
			MethodElement: ElementResult{Met: false},
			Accessibility: ElementResult{Met: false},
		}, "高"},
		{"完全缺失", &FourElementsResult{
			TimeElement:   ElementResult{Met: false},
			PlaceElement:  ElementResult{Met: false},
			MethodElement: ElementResult{Met: false},
			Accessibility: ElementResult{Met: false},
		}, "极高"},
		{"nil", nil, "无法评估"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assessPublicUseBurdenDifficulty(tt.four)
			if got != tt.want {
				t.Errorf("assessPublicUseBurdenDifficulty = %q, 期望 %q", got, tt.want)
			}
		})
	}
}

func TestTypeSpecificJudgment_DateDeterminationString(t *testing.T) {
	ts := &TypeSpecificJudgment{
		DateDetermination: &DateDetermination{
			Determined:  "2023-06-15",
			Reliability: RelHigh,
			SourceType:  SrcExactPage,
		},
	}
	got := ts.DateDeterminationString()
	want := "2023-06-15(high/exact_page_date)"
	if got != want {
		t.Errorf("DateDeterminationString = %q, 期望 %q", got, want)
	}

	// nil DateDetermination
	tsNil := &TypeSpecificJudgment{}
	if tsNil.DateDeterminationString() != "未知" {
		t.Errorf("nil DateDetermination 时应返回 '未知', 实际 = %q", tsNil.DateDeterminationString())
	}
}

func TestTypeSpecificJudgment_PlatformCredibilityString(t *testing.T) {
	cred := CredHigh
	ts := &TypeSpecificJudgment{
		PlatformCredibility: &cred,
	}
	got := ts.PlatformCredibilityString()
	if got != "high" {
		t.Errorf("PlatformCredibilityString = %q, 期望 %q", got, "high")
	}

	// nil PlatformCredibility
	tsNil := &TypeSpecificJudgment{}
	if tsNil.PlatformCredibilityString() != "未知" {
		t.Errorf("nil PlatformCredibility 时应返回 '未知', 实际 = %q", tsNil.PlatformCredibilityString())
	}
}

func TestExtractDateFromText(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"2023-06-15", "2023-06-15"},
		{"在2023年6月15日公开销售", "2023年6月15日"},
		{"发布时间2023-06-15", "2023-06-15"},
		{"该产品于2023年1月上市", "2023年1月"},
		{"无日期信息", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := extractDateFromText(tt.text)
			if got != tt.want {
				t.Errorf("extractDateFromText(%q) = %q, 期望 %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestDefaultEngine_Judge_PublicUse_DateDetermination(t *testing.T) {
	engine := NewEngine(nil)

	span := agentcore_evidence.EvidenceSpan{
		ID:         "test-pubuse-date",
		SourceURI:  "pub_use:exhibition-record",
		DocVersion: "2023-06",
		Direction:  agentcore_evidence.DirectionSupporting,
		Snippet:    "2023年6月在展会上演示",
	}

	judgment, err := engine.Judge(span)
	if err != nil {
		t.Fatalf("Judge() 返回错误: %v", err)
	}

	if judgment.TypeSpecificJudgment == nil {
		t.Fatal("TypeSpecificJudgment 不应为 nil")
	}

	if judgment.TypeSpecificJudgment.DateDetermination == nil {
		t.Fatal("使用公开的 DateDetermination 不应为 nil")
	}

	// 月级日期应推定到月末
	if judgment.TypeSpecificJudgment.DateDetermination.Determined != "2023-06-30" {
		t.Errorf("月级日期应以月末推定, 实际 = %q", judgment.TypeSpecificJudgment.DateDetermination.Determined)
	}
}
