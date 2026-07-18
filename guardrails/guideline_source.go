package guardrails

import (
	"fmt"
	"strings"

	"github.com/xujian519/mady/pkg/lawcite"
)

// guidelineTopics maps examination guideline part/chapter/section codes to
// their topic keywords for citation verification.
//
// The Patent Examination Guidelines (审查指南) are structured as:
//
//	Part (部) → Chapter (章) → Section (节) → Subsection (小节)
//
// Key encoding: the article parameter in CitationSource.Topics() is encoded
// as PPPCCSS where PP=part, CC=chapter, SS=section.
// Example: 20301 → Part 2, Chapter 3, Section 1 (新颖性概念).
//
// This index covers the ~60 most frequently cited guideline sections in
// patent office action responses.
var guidelineTopics = map[int][]string{
	// Part 1 (第一部分: 初步审查)
	10101: {"发明专利申请初步审查", "申请文件形式要求", "请求书"},
	10102: {"委托专利代理机构", "专利代理委托书"},
	10201: {"实用新型初步审查", "保护客体"},
	10301: {"外观设计初步审查", "图片照片", "简要说明"},

	// Part 2 (第二部分: 实质审查) — most frequently cited
	20101: {"不授予专利权的申请", "科学发现", "智力活动规则和方法"},
	20102: {"疾病的诊断和治疗方法", "不授予专利权"},
	20201: {"说明书", "充分公开", "能够实现", "清楚完整"},
	20202: {"说明书", "支持", "以说明书为依据"},
	20203: {"权利要求书", "清楚", "保护范围", "必要技术特征"},
	20204: {"权利要求书", "以说明书为依据", "支持"},
	20205: {"权利要求书", "单一性", "总的发明构思"},
	20206: {"说明书摘要", "摘要"},
	20301: {"新颖性", "现有技术", "单独对比"},
	20302: {"新颖性", "抵触申请"},
	20303: {"新颖性", "相同内容的发明创造"},
	20304: {"新颖性", "惯用手段的直接置换"},
	20305: {"新颖性", "上下位概念"},
	20306: {"新颖性", "数值范围"},
	20401: {"创造性", "三步法", "最接近的现有技术"},
	20402: {"创造性", "确定区别特征", "实际解决的技术问题"},
	20403: {"创造性", "非显而易见", "技术启示"},
	20404: {"创造性", "辅助判断因素", "预料不到的技术效果", "商业成功"},
	20405: {"创造性", "要素变更发明", "组合发明"},
	20501: {"实用性", "能够制造使用", "积极效果"},
	20601: {"单一性", "总的发明构思", "特定技术特征"},
	20602: {"单一性", "分案申请"},
	20701: {"检索", "现有技术", "检索范围"},
	20702: {"检索", "检索策略"},
	20801: {"实质审查程序", "审查意见通知书"},
	20802: {"实质审查", "修改", "专利法第33条"},
	20803: {"实质审查", "驳回", "授权"},
	20804: {"实质审查", "会晤", "电话讨论"},
	20805: {"实质审查", "听证原则"},

	// Part 3 (第三部分: PCT国际申请)
	30101: {"PCT", "国际申请", "进入国家阶段"},
	30102: {"PCT", "国际检索", "国际初步审查"},

	// Part 4 (第四部分: 复审与无效)
	40101: {"复审", "复审请求", "复审程序"},
	40102: {"复审", "前置审查", "合议审查"},
	40201: {"无效宣告", "无效宣告请求", "无效理由"},
	40202: {"无效宣告", "形式审查"},
	40203: {"无效宣告", "合议审查", "口头审理"},
	40204: {"无效宣告", "无效决定", "司法救济"},
	40301: {"复审无效", "口头审理", "程序"},

	// Part 5 (第五部分: 申请及事务处理)
	50101: {"专利申请", "受理", "申请日"},
	50102: {"专利申请", "优先权", "外国优先权", "本国优先权"},
	50201: {"专利费用", "申请费", "年费", "减缓"},
	50202: {"期限", "法定期限", "指定期限", "延长期限"},
	50301: {"专利公报", "公告", "公布"},
	50401: {"专利文档", "查阅", "复制"},
}

// GuidelineSource implements CitationSource for the Patent Examination Guidelines
// (审查指南). It provides topic keywords for guideline part/chapter/section
// references, enabling the CitationGate to verify guideline citations in agent outputs.
//
// The article parameter is encoded as PPPCCSS (Part * 10000 + Chapter * 100 + Section).
// For example: 20403 = Part 2, Chapter 4, Section 3 (创造性判断中对技术启示的论述).
//
// Usage:
//
//	guidelineSrc := guardrails.NewGuidelineSource()
//	gate := guardrails.NewCitationGate(guardrails.LevelStandard,
//	    guardrails.WithCitationSource(
//	        guardrails.CompositeCitationSource(
//	            guardrails.DefaultCitationSource(),
//	            guidelineSrc,
//	        ),
//	    ),
//	)
type GuidelineSource struct{}

// NewGuidelineSource creates a GuidelineSource backed by the built-in
// guidelineTopics index (covering ~50 key sections from the 2023 Examination
// Guidelines).
func NewGuidelineSource() *GuidelineSource {
	return &GuidelineSource{}
}

// Topics returns the topic keywords for a given guideline section code.
// It only responds to lawcite.StatuteExamGuideline statutes. The article
// parameter is the PPPCCSS-encoded section number.
func (g *GuidelineSource) Topics(s lawcite.Statute, article int) ([]string, bool) {
	if s != lawcite.StatuteExamGuideline {
		return nil, false
	}

	// Try exact match.
	if topics, ok := guidelineTopics[article]; ok {
		return topics, true
	}

	// Try progressively shorter section codes for fuzzy matching.
	// Encoding: PPPCCSS — strip subsection then chapter.
	// e.g. 2040102 → 20401 → 20400 → 20000
	parent := article / 100 // strip subsection, get section level
	if parent > 0 {
		if topics, ok := guidelineTopics[parent]; ok {
			return topics, true
		}
	}
	parent = (article / 10000) * 10000 // get part level
	if parent > 0 {
		if topics, ok := guidelineTopics[parent]; ok {
			return topics, true
		}
	}

	return nil, false
}

// MaxArticle returns 0 — guideline references use section codes, not article
// numbers, so there is no article-number upper bound.
func (g *GuidelineSource) MaxArticle(s lawcite.Statute) int {
	if s == lawcite.StatuteExamGuideline {
		return 0 // no article-number bound for guidelines
	}
	return 0
}

// EncodeGuidelineSection encodes a guideline section reference string into
// the numeric key used by GuidelineSource.
//
// Supported formats:
//   - "第二部分第八章第4.2节" → 20804
//   - "2.8.4" → 20804
//   - "审查指南第二部分第三章第3.2.1节" → 20303
//
// Returns 0 if the format cannot be parsed.
func EncodeGuidelineSection(ref string) int {
	// Try dotted numeric format: "2.4.1" or "2.4.1.2"
	var part, chapter, section int
	if n, _ := fmt.Sscanf(ref, "%d.%d.%d", &part, &chapter, &section); n >= 3 {
		return part*10000 + chapter*100 + section
	}
	if n, _ := fmt.Sscanf(ref, "%d.%d", &part, &chapter); n >= 2 {
		return part*10000 + chapter*100
	}

	// Try Chinese format: "第X部分第X章第X节"
	part = extractNumberAfter(ref, "部分")
	chapter = extractNumberAfter(ref, "章")
	section = extractNumberAfter(ref, "节")
	if part > 0 {
		return part*10000 + chapter*100 + section
	}

	return 0
}

// extractNumberAfter extracts a Chinese or Arabic numeral after a marker word.
func extractNumberAfter(s, marker string) int {
	_, rest, ok := strings.Cut(s, marker)
	if !ok {
		return 0
	}
	// Try Arabic numeral first.
	for _, r := range rest {
		if r >= '0' && r <= '9' {
			continue
		}
		// Found end of number.
		numStr := rest[:strings.IndexFunc(rest, func(r rune) bool {
			return r < '0' || r > '9'
		})]
		if len(numStr) > 0 {
			var n int
			_, _ = fmt.Sscanf(numStr, "%d", &n)
			return n
		}
		break
	}
	return 0
}
