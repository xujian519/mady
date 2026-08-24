package doctmpl

import (
	"strings"
	"testing"
)

func patentTemplateVars() map[string]string {
	return map[string]string{
		"firm_name":              "XX 知识产权代理所",
		"doc_no":                 "P-2026-001",
		"case_no":                "202610000001.0",
		"application_no":         "202610000001.0",
		"applicant":              "示例公司",
		"invention_title":        "一种智能传感器",
		"technical_field":        "物联网传感器技术领域",
		"prior_art":              "现有技术检索结果：CN1234567A、EP9876543B1。",
		"novelty_analysis":       "经比对，权利要求与现有技术存在以下区别特征…",
		"inventiveness_analysis": "区别特征结合技术启示，本方案具有创造性…",
		"conclusion":             "具备授权前景，建议进入实审。",
		"search_type":            "新颖性",
		"search_strategy":        "采用 IPC 分类 + 关键词组合检索。",
		"databases_covered":      "CNIPA、EPO、USPTO、WIPO。",
		"key_hits":               "D1: CN1234567A（高相关）；D2: US789456B（相关）。",
		"analysis":               "D1 披露了主要技术特征，D2 提供辅助启示。",
		"patent_no":              "CN1234567B",
		"patentee":               "被无效专利权人",
		"invalidator":            "请求人",
		"grounds":                "缺乏创造性（A22.3）与说明书不充分公开（A26.3）。",
		"evidence":               "证据1：对比文件D1；证据2：公开教科书。",
		"feature_comparison":     "权利要求特征与D1特征逐一比对…",
		"feasibility":            "创造性理由具备较强无效可行性。",
		"oa_type":                "创造性",
		"examiner_opinion":       "审查员认为区别特征为常规技术手段。",
		"amendment":              "将说明书中的特定数值范围写入从属权利要求。",
		"arguments":              "区别特征带来预料不到的技术效果。",
		"background":             "现有传感器存在能耗高、精度不足的问题。",
		"summary":                "本发明通过…（发明目的与技术方案）。",
		"claims":                 "1. 一种智能传感器，其特征在于，包括…。",
		"embodiments":            "实施例1：…；实施例2：…。",
		"disclaimer":             "本分析由 AI 辅助生成，不构成正式法律意见。",
	}
}

func TestPatentTemplates_Render(t *testing.T) {
	store, err := NewTemplateStore()
	if err != nil {
		t.Fatalf("NewTemplateStore: %v", err)
	}
	meta := RenderMeta{Style: &RenderStyle{Name: "patent-standard", Disclaimer: "本分析由 AI 辅助生成，不构成正式法律意见。"}, Title: "专利分析"}
	names := []string{
		"patentability-opinion", "search-report", "invalidation-opinion",
		"oa-response-sati", "claims-spec",
	}
	vars := patentTemplateVars()
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			out, err := store.Render(name, vars, FormatHTML, meta)
			if err != nil {
				t.Fatalf("render %s: %v", name, err)
			}
			html := string(out)
			if !strings.Contains(html, ".verdict-table") {
				t.Errorf("%s: expected patent stylesheet", name)
			}
			if !strings.Contains(html, "不构成正式法律意见") {
				t.Errorf("%s: expected disclaimer wording in output", name)
			}
		})
	}
}
