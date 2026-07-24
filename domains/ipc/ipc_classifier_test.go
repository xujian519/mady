package ipc

import (
	"strings"
	"testing"
)

// =============================================================================
// Section 1: Classify — end-to-end classification tests
// =============================================================================

func TestClassify_Pharmaceutical(t *testing.T) {
	text := "本发明涉及一种新的药物组合物，包含阿司匹林和雷尼替丁，用于治疗心血管疾病和胃溃疡。"
	section, confidence := Classify(text)
	if section != IPCA {
		t.Errorf("pharmaceutical text should be classified as A (HumanNecessities), got %s", section)
	}
	if confidence < minConfidenceForKeyword {
		t.Errorf("expected confidence >= %.2f for pharmaceutical text, got %.2f", minConfidenceForKeyword, confidence)
	}
}

func TestClassify_Mechanical(t *testing.T) {
	text := "一种高效的离心分离装置，包括转子、进料管和多个分离室，通过高速旋转实现固液分离。"
	section, confidence := Classify(text)
	if section != IPCB {
		t.Errorf("mechanical separation text should be classified as B (OperationsTransport), got %s", section)
	}
	if confidence < minConfidenceForKeyword {
		t.Errorf("expected confidence >= %.2f for mechanical text, got %.2f", minConfidenceForKeyword, confidence)
	}
}

func TestClassify_Chemistry(t *testing.T) {
	text := "一种新型高分子聚合物催化剂，由过渡金属配合物和有机配体组成，用于烯烃聚合反应。"
	section, confidence := Classify(text)
	if section != IPCC {
		t.Errorf("chemistry text should be classified as C (Chemistry), got %s", section)
	}
	if confidence < minConfidenceForKeyword {
		t.Errorf("expected confidence >= %.2f for chemistry text, got %.2f", minConfidenceForKeyword, confidence)
	}
}

func TestClassify_Electricity(t *testing.T) {
	text := "一种无线通信方法，在基站和移动终端之间通过OFDM调制方式传输数据，包含信道估计和均衡步骤。"
	section, confidence := Classify(text)
	if section != IPCH {
		t.Errorf("electricity/communication text should be classified as H (Electricity), got %s", section)
	}
	if confidence < minConfidenceForKeyword {
		t.Errorf("expected confidence >= %.2f for electricity text, got %.2f", minConfidenceForKeyword, confidence)
	}
}

func TestClassify_Physics(t *testing.T) {
	text := "一种高精度光学测量装置，利用激光干涉原理检测物体的微小位移，包括光源、分束器和光电探测器。"
	section, confidence := Classify(text)
	if section != IPCG {
		t.Errorf("optical measurement text should be classified as G (Physics), got %s", section)
	}
	if confidence < minConfidenceForKeyword {
		t.Errorf("expected confidence >= %.2f for optics text, got %.2f", minConfidenceForKeyword, confidence)
	}
}

func TestClassify_Construction(t *testing.T) {
	text := "一种抗震建筑结构，包括框架柱、剪力墙和耗能支撑构件，能够有效吸收地震能量。"
	section, _ := Classify(text)
	if section != IPCE {
		t.Errorf("construction text should be classified as E (FixedConstructions), got %s", section)
	}
}

func TestClassify_Textile(t *testing.T) {
	text := "一种防水透气织物的制备方法，包括纤维编织、涂层整理和防水处理步骤。"
	section, _ := Classify(text)
	if section != IPCD {
		t.Errorf("textile text should be classified as D (Textiles), got %s", section)
	}
}

func TestClassify_Engine(t *testing.T) {
	text := "一种涡轮增压内燃机，包括气缸、活塞、涡轮增压器和进气冷却系统。"
	section, _ := Classify(text)
	if section != IPCF {
		t.Errorf("engine text should be classified as F (MechanicalEngine), got %s", section)
	}
}

// =============================================================================
// Section 2: Fallback behavior (no matching keywords)
// =============================================================================

func TestClassify_EmptyText(t *testing.T) {
	section, confidence := Classify("")
	if section != IPCB {
		t.Errorf("empty text should default to B, got %s", section)
	}
	if confidence != 0.15 {
		t.Errorf("empty text confidence should be 0.15, got %.2f", confidence)
	}
}

func TestClassify_NoKeywords(t *testing.T) {
	// Text with no IPC keywords should fall back to B with low confidence.
	section, confidence := Classify("今天天气很好，适合出去散步。")
	if section != IPCB {
		t.Errorf("no-match text should default to B, got %s", section)
	}
	if confidence != 0.15 {
		t.Errorf("no-match confidence should be 0.15, got %.2f", confidence)
	}
}

// =============================================================================
// Section 3: ClassifyDetailed
// =============================================================================

func TestClassifyDetailed_ReturnsMatchedKeywords(t *testing.T) {
	text := "一种新型化合物催化剂用于化学反应"
	result := ClassifyDetailed(text)
	if result.Section != IPCC {
		t.Errorf("expected C (Chemistry), got %s", result.Section)
	}
	if len(result.MatchedKeywords) == 0 {
		t.Error("expected matched keywords to be non-empty")
	}
	found := false
	for _, kw := range result.MatchedKeywords {
		if strings.Contains(kw, "化合物") || strings.Contains(kw, "催化") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected keywords containing '化合物' or '催化', got %v", result.MatchedKeywords)
	}
}

func TestClassifyDetailed_AmbiguousText(t *testing.T) {
	// Text mixing chemistry and pharmaceuticals.
	text := "一种药物化合物及其制备方法，包含催化剂和反应步骤。"
	result := ClassifyDetailed(text)
	// Both IPCA (pharmaceutical) and IPCC (chemistry) should have matched.
	// IPCA should win due to "药物" being a strong match.
	if result.Section != IPCA && result.Section != IPCC {
		t.Errorf("ambiguous text should match A or C, got %s", result.Section)
	}
}

func TestClassifyDetailed_NoMatch(t *testing.T) {
	result := ClassifyDetailed("这是一个没有技术特征的自然语言描述。")
	if result.Section != IPCB {
		t.Errorf("expected default B, got %s", result.Section)
	}
	if len(result.MatchedKeywords) != 0 {
		t.Errorf("expected no matched keywords for no-match text, got %v", result.MatchedKeywords)
	}
}

// =============================================================================
// Section 4: IsHighConfidence
// =============================================================================

func TestIsHighConfidence(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
		want       bool
	}{
		{"above threshold", 0.85, true},
		{"exactly threshold", 0.80, true},
		{"slightly below", 0.79, false},
		{"low", 0.15, false},
		{"zero", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHighConfidence(tt.confidence)
			if got != tt.want {
				t.Errorf("IsHighConfidence(%.2f) = %v, want %v", tt.confidence, got, tt.want)
			}
		})
	}
}

// =============================================================================
// Section 5: AllDomains completeness
// =============================================================================

func TestAllDomains_ContainsAllSections(t *testing.T) {
	requiredSections := []IPCSection{IPCA, IPCB, IPCC, IPCD, IPCE, IPCF, IPCG, IPCH}
	for _, s := range requiredSections {
		if _, ok := AllDomains[s]; !ok {
			t.Errorf("AllDomains missing section %s", s)
		}
	}
}

func TestAllDomains_AllHaveRequiredFields(t *testing.T) {
	for section, domain := range AllDomains {
		if domain.Name == "" {
			t.Errorf("section %s has empty Name", section)
		}
		if len(domain.Keywords) == 0 {
			t.Errorf("section %s has empty Keywords", section)
		}
		if len(domain.InventivenessFocus) == 0 {
			t.Errorf("section %s has empty InventivenessFocus", section)
		}
	}
}

// =============================================================================
// Section 6: IPCSection methods
// =============================================================================

func TestIPCSection_SectionOf(t *testing.T) {
	tests := []struct {
		section IPCSection
		want    string
	}{
		{IPCA, "人类生活必需"},
		{IPCB, "作业/运输"},
		{IPCC, "化学/冶金"},
		{IPCD, "纺织/造纸"},
		{IPCE, "固定建筑物"},
		{IPCF, "机械工程"},
		{IPCG, "物理"},
		{IPCH, "电学"},
		{IPCSection("Z"), "未知"},
	}
	for _, tt := range tests {
		t.Run(string(tt.section), func(t *testing.T) {
			got := tt.section.SectionOf()
			if got != tt.want {
				t.Errorf("SectionOf(%s) = %q, want %q", tt.section, got, tt.want)
			}
		})
	}
}

func TestIPCSection_String(t *testing.T) {
	if string(IPCA) != "A" {
		t.Errorf("IPCA.String() = %s, want A", IPCA)
	}
	if string(IPCH) != "H" {
		t.Errorf("IPCH.String() = %s, want H", IPCH)
	}
}

// =============================================================================
// Section 7: GetInventivenessHints
// =============================================================================

func TestGetInventivenessHints_AllSections(t *testing.T) {
	sections := []IPCSection{IPCA, IPCB, IPCC, IPCD, IPCE, IPCF, IPCG, IPCH}
	for _, s := range sections {
		hints := GetInventivenessHints(s)
		if hints == "" {
			t.Errorf("GetInventivenessHints(%s) returned empty", s)
		}
		if !strings.Contains(hints, "创造性") {
			t.Errorf("GetInventivenessHints(%s) should mention 创造性", s)
		}
		if !strings.Contains(hints, "本领域的技术人员") {
			t.Errorf("GetInventivenessHints(%s) should mention 本领域的技术人员", s)
		}
	}
}

func TestGetInventivenessHints_UnknownSection(t *testing.T) {
	hints := GetInventivenessHints(IPCSection("Z"))
	if hints != "" {
		t.Errorf("expected empty for unknown section, got %q", hints)
	}
}

// =============================================================================
// Section 8: GetNoveltyHints
// =============================================================================

func TestGetNoveltyHints_AllSections(t *testing.T) {
	sections := []IPCSection{IPCA, IPCB, IPCC, IPCD, IPCE, IPCF, IPCG, IPCH}
	for _, s := range sections {
		hints := GetNoveltyHints(s)
		if hints == "" {
			t.Errorf("GetNoveltyHints(%s) returned empty", s)
		}
		if !strings.Contains(hints, "新颖性") {
			t.Errorf("GetNoveltyHints(%s) should mention 新颖性", s)
		}
	}
}

func TestGetNoveltyHints_ChemistryHasSpecialRules(t *testing.T) {
	hints := GetNoveltyHints(IPCC)
	requiredTerms := []string{
		"通式化合物", "异构体", "晶体", "组合物",
		"制药用途", "Markush",
	}
	for _, term := range requiredTerms {
		if !strings.Contains(hints, term) {
			t.Errorf("chemistry novelty hints missing term: %q", term)
		}
	}
}

func TestGetNoveltyHints_ElectricityHasSpecialRules(t *testing.T) {
	hints := GetNoveltyHints(IPCH)
	requiredTerms := []string{"电路结构", "通信协议", "信号处理", "半导体"}
	for _, term := range requiredTerms {
		if !strings.Contains(hints, term) {
			t.Errorf("electricity novelty hints missing term: %q", term)
		}
	}
}

func TestGetNoveltyHints_UnknownSection(t *testing.T) {
	hints := GetNoveltyHints(IPCSection("Z"))
	if hints != "" {
		t.Errorf("expected empty for unknown section, got %q", hints)
	}
}

// =============================================================================
// Section 9: GetCommonKnowledge
// =============================================================================

func TestGetCommonKnowledge_AllSections(t *testing.T) {
	sections := []IPCSection{IPCA, IPCB, IPCC, IPCD, IPCE, IPCF, IPCG, IPCH}
	for _, s := range sections {
		knowledge := GetCommonKnowledge(s)
		if len(knowledge) == 0 {
			t.Errorf("GetCommonKnowledge(%s) returned empty", s)
		}
	}
}

func TestGetCommonKnowledge_ReturnsCopy(t *testing.T) {
	// Verify that modifying the returned slice doesn't affect the original.
	knowledge := GetCommonKnowledge(IPCA)
	if len(knowledge) == 0 {
		t.Fatal("expected non-empty knowledge")
	}
	originalLen := len(AllDomains[IPCA].CommonKnowledge)
	// Modify the returned copy.
	knowledge[0] = "modified"
	if AllDomains[IPCA].CommonKnowledge[0] == "modified" {
		t.Error("GetCommonKnowledge should return a copy, not a reference")
	}
	if len(AllDomains[IPCA].CommonKnowledge) != originalLen {
		t.Error("original data should not be affected")
	}
}

func TestGetCommonKnowledge_UnknownSection(t *testing.T) {
	knowledge := GetCommonKnowledge(IPCSection("Z"))
	if knowledge != nil {
		t.Errorf("expected nil for unknown section, got %v", knowledge)
	}
}

// =============================================================================
// Section 10: matchKeywordsInText
// =============================================================================

func TestMatchKeywordsInText(t *testing.T) {
	keywords := []string{"药物", "化合物", "催化"}
	matched := matchKeywordsInText("一种新型化合物", keywords)
	if len(matched) != 1 {
		t.Errorf("expected 1 match, got %d: %v", len(matched), matched)
	}
	if matched[0] != "化合物" {
		t.Errorf("expected '化合物', got %q", matched[0])
	}
}

func TestMatchKeywordsInText_CaseInsensitive(t *testing.T) {
	matched := matchKeywordsInText("DRUG compound", []string{"drug", "compound"})
	if len(matched) != 2 {
		t.Errorf("expected 2 matches (case-insensitive), got %d: %v", len(matched), matched)
	}
}

func TestMatchKeywordsInText_NoMatch(t *testing.T) {
	matched := matchKeywordsInText("今天天气很好", AllDomains[IPCA].Keywords)
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d: %v", len(matched), matched)
	}
}
