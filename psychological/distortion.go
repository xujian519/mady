package psychological

import (
	"regexp"
	"strings"
	"sync"
)

// distortionRule 单一认知扭曲的检测规则
type distortionRule struct {
	Type        CognitiveDistortion
	Patterns    []*regexp.Regexp
	Description string
	Reframe     string
}

// DistortionReframe 扭曲的重构建议
type DistortionReframe struct {
	Distortion CognitiveDistortion
	Reframe    string
}

// compiledDistortionRules 延迟编译的正则规则列表，首次访问时通过 sync.Once 编译
var (
	compileDistortionOnce sync.Once
	compiledDistortionRules []distortionRule
)

// getDistortionRules 返回编译后的认知扭曲检测规则，仅在首次调用时编译正则
func getDistortionRules() []distortionRule {
	compileDistortionOnce.Do(func() {
	rules := []struct {
		t CognitiveDistortion
		p []string
		d string
		r string
	}{
		{DistAllOrNothing,
			[]string{`总是|从不|完全|彻底|毫无|根本|绝对`, `要么.*要么|不是.*就是`, `完全失败|彻底完蛋|一无是处`},
			"非黑即白二分思维", "事物往往存在中间地带，不完全成功不等于完全失败"},
		{DistCatastrophizing,
			[]string{`完蛋|完了|糟了|最坏|灾难|受不了`, `万一.*怎么办|如果.*就完了`},
			"预想最坏结果", "最坏情况不一定会发生，你过去也应对过类似挑战"},
		{DistOvergeneralization,
			[]string{`每次都|从来都|总是这样|永远|一直.*不行|一切|所有`},
			"基于单一事件得出普遍结论", "单次经历不代表所有情况，过去也有过不同的经历"},
		{DistMentalFiltering,
			[]string{`只看|只看到|只记得|全都是.*不好|没一个好|没什么好事`},
			"只关注负面细节而忽略全局", "尝试同时看到积极和消极的方面，不要只聚焦于负面"},
		{DistDiscountingPositive,
			[]string{`不算|没什么|谁都能|运气好|碰巧|只是偶然`},
			"贬低正面经历", "你的成功是你努力的结果，正面经历值得认可"},
		{DistJumpingToConclusions,
			[]string{`肯定|一定|绝对.*是|不用说|我断定`},
			"在证据不充分时下结论", "在没有确认之前，可能存在其他可能性"},
		{DistMindReading,
			[]string{`他觉得|他们认为|肯定觉得|一定觉得`},
			"未经证实就断定他人想法", "我们无法确定他人的想法，除非直接沟通"},
		{DistFortuneTelling,
			[]string{`会失败|不会成功|肯定不行|预感.*不好|估计.*不行`},
			"预判负面未来", "未来有多种可能性，不要仅预测最坏的结果"},
		{DistMagnifying,
			[]string{`太可怕|太严重|太大了|太差`},
			"夸大问题的严重性", "试着客观评估问题的实际规模"},
		{DistEmotionalReasoning,
			[]string{`感觉.*就是|觉得.*一定|我感觉.*所以|因为.*害怕.*所以|因为.*焦虑.*所以`},
			"用情绪替代事实", "感受是感受，事实是事实——两者可能不一致"},
		{DistShouldStatements,
			[]string{`应该|必须|不该|本来应该|本可以|本不该`},
			"僵化标准要求", "减少强求标准，接受更多可能性"},
		{DistLabeling,
			[]string{`我是.*的人|我是个.*|没用|差劲|废物|蠢|笨`},
			"贴极端标签", "行为不等于整体的人——不要贴标签"},
		{DistPersonalization,
			[]string{`怪我|我的错|都因为我|因为我.*才|是我不好`},
			"过度承担责任", "很多因素共同导致结果，不要全部归咎于自己"},
	}
	for _, r := range rules {
		compiled := make([]*regexp.Regexp, len(r.p))
		for i, pat := range r.p {
			compiled[i] = regexp.MustCompile(pat)
		}
		compiledDistortionRules = append(compiledDistortionRules, distortionRule{r.t, compiled, r.d, r.r})
	}
	})
	return compiledDistortionRules
}

// detectDistortions 三步法检测认知扭曲
//
// 参考 Diagnosis-of-Thought:
// 1. 分离事实与想法：识别事实性陈述和主观信念
// 2. 匹配扭曲模式：用规则库检测扭曲类型
// 3. 提取信念陈述：提取具体的扭曲信念文本
func detectDistortions(text string) DistortionDetection {
	var matched []CognitiveDistortion
	var beliefs []string
	var totalIntensity float64

	for _, rule := range getDistortionRules() {
		for _, pat := range rule.Patterns {
			if m := pat.FindString(text); m != "" {
				matched = append(matched, rule.Type)
				beliefs = append(beliefs, m)
				totalIntensity += 0.3
				break // 每种扭曲只匹配一次
			}
		}
	}

	// 去重
	seen := make(map[CognitiveDistortion]bool)
	var unique []CognitiveDistortion
	for _, d := range matched {
		if !seen[d] {
			seen[d] = true
			unique = append(unique, d)
		}
	}

	// 提取事实陈述（不含扭曲关键词的句子）
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' || r == '\n'
	})
	var factuals []string
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) <= 4 {
			continue
		}
		hasDistortion := false
		for _, rule := range getDistortionRules() {
			for _, pat := range rule.Patterns {
				if pat.MatchString(s) {
					hasDistortion = true
					break
				}
			}
			if hasDistortion {
				break
			}
		}
		if !hasDistortion {
			factuals = append(factuals, s)
		}
	}

	beliefIntensity := totalIntensity * 1.5
	if beliefIntensity > 1 {
		beliefIntensity = 1
	}

	return DistortionDetection{
		Distortions:       unique,
		BeliefStatements:   beliefs,
		BeliefIntensity:    beliefIntensity,
		FactualStatements:  factuals,
	}
}

// generateReframes 为检测到的扭曲生成重构建议
func generateReframes(distortions []CognitiveDistortion) []DistortionReframe {
	seen := make(map[CognitiveDistortion]bool)
	var results []DistortionReframe
	for _, d := range distortions {
		if seen[d] {
			continue
		}
		seen[d] = true
		for _, rule := range getDistortionRules() {
			if rule.Type == d {
				results = append(results, DistortionReframe{Distortion: d, Reframe: rule.Reframe})
				break
			}
		}
	}
	return results
}

// hasSevereDistortion 判断是否存在严重认知扭曲
func hasSevereDistortion(d DistortionDetection) bool {
	if len(d.Distortions) >= 3 {
		return true
	}
	if d.BeliefIntensity >= 0.7 {
		return true
	}
	severe := map[CognitiveDistortion]bool{
		DistCatastrophizing: true, DistPersonalization: true, DistLabeling: true,
	}
	for _, dist := range d.Distortions {
		if severe[dist] {
			return true
		}
	}
	return false
}
