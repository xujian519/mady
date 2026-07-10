package psychological

import "regexp"

// 预编译的正则模式，在包初始化时编译
var (
	reStrongNegative = regexp.MustCompile(`(?i)不好|不行|糟糕|失败|驳回|拒绝|awful|terrible`)
	reStrongPositive = regexp.MustCompile(`(?i)不错|开心|顺利|通过|满意|高兴|成功|搞定|thanks|great|good|happy`)
	reWeakNegative   = regexp.MustCompile(`(?i)烦|气|差|累|怒|讨厌|bad|angry`)
	reWeakPositive   = regexp.MustCompile(`(?i)好|顺利|good|happy`)
	reUncertain      = regexp.MustCompile(`(?i)不知道|不确定|可能|也许|大概|maybe|perhaps|unsure|if|whether`)
	reSelfBlame      = regexp.MustCompile(`(?i)是我.*错|我错了|都怪我|我的错|我不好|我能力|我不够|my fault|i should|i could`)
	reOtherBlame     = regexp.MustCompile(`(?i)他|他们|公司|客户|审查员|同事|环境|they|them|manager|boss|client`)
	reHasControl     = regexp.MustCompile(`(?i)有办法|可以|能处理|handle|manage|solution|plan`)
	reNoControl      = regexp.MustCompile(`(?i)没办法|不得不|被迫|无解|hopeless|stuck|can't|no choice`)
	reSurprise       = regexp.MustCompile(`(?i)没想到|突然|意外|surprise|unexpected|居然`)
	reImportant      = regexp.MustCompile(`(?i)很重要|关键|必须|重要|critical|important|must|need|有影响`)
)

// extractTextualSignals 从用户文本中提取量化心理信号
// 信号提取顺序：先匹配强信号，再匹配弱信号
func extractTextualSignals(text string) TextSignals {
	lower := text

	// 情感倾向 — 强信号优先
	sentiment := 0.0
	switch {
	case reStrongNegative.MatchString(lower):
		sentiment = -0.7
	case reStrongPositive.MatchString(lower):
		sentiment = 0.7
	case reWeakNegative.MatchString(lower):
		sentiment = -0.5
	case reWeakPositive.MatchString(lower):
		sentiment = 0.4
	}

	// 不确定性
	uncertainty := 0.2
	if reUncertain.MatchString(lower) {
		uncertainty = 0.7
	}

	// 归因方向 (-1=自己, 1=他人/环境)
	blameDirection := 0.0
	switch {
	case reSelfBlame.MatchString(lower):
		blameDirection = -0.6
	case reOtherBlame.MatchString(lower):
		blameDirection = 0.7
	}

	// 控制感
	perceivedControl := 0.5
	switch {
	case reNoControl.MatchString(lower):
		perceivedControl = 0.2
	case reHasControl.MatchString(lower):
		perceivedControl = 0.8
	}

	// 意外程度
	surpriseLevel := 0.2
	if reSurprise.MatchString(lower) {
		surpriseLevel = 0.8
	}

	// 目标重要性
	goalImportance := 0.4
	if reImportant.MatchString(lower) {
		goalImportance = 0.8
	}

	return TextSignals{
		Sentiment:        sentiment,
		Uncertainty:      uncertainty,
		BlameDirection:   blameDirection,
		PerceivedControl: perceivedControl,
		SurpriseLevel:    surpriseLevel,
		GoalImportance:   goalImportance,
	}
}

// buildAppraisalFrame 从文本信号构建 OCC 评价框架
func buildAppraisalFrame(signals TextSignals) AppraisalFrame {
	deservingness := 0.5
	if signals.Sentiment < 0 {
		deservingness = 0.8
	}

	return AppraisalFrame{
		Desirability:       clamp(signals.Sentiment, -1, 1),
		Likelihood:         clamp(1-signals.Uncertainty, 0, 1),
		Praiseworthiness:   clamp(signals.Sentiment, -1, 1),
		Deservingness:      deservingness,
		Appealingness:      clamp(signals.Sentiment, -1, 1),
		Unexpectedness:     clamp(signals.SurpriseLevel, 0, 1),
		CausalAttribution:  clamp(signals.BlameDirection, -1, 1),
		Controllability:    clamp(signals.PerceivedControl, 0, 1),
	}
}
