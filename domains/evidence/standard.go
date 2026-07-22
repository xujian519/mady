package evidence

// StandardOfProof 描述证明标准等级。
type StandardOfProof string

const (
	StandardBeyondReasonableDoubt StandardOfProof = "beyond_reasonable_doubt" // 排除合理怀疑
	StandardHighProbability       StandardOfProof = "high_probability"        // 高度盖然性
	StandardPreponderance         StandardOfProof = "preponderance"           // 优势证据
	StandardSubstantialEvidence   StandardOfProof = "substantial_evidence"    // 实质性证据
	StandardPrimaFacie            StandardOfProof = "prima_facie"             // 初步证据
)

// AssessProofStandard 评估是否达到指定证明标准。
// validCount 为有效的支持性证据数量（支持与反对之差），total 为总证据数。
// 使用 validCount 作为分子，避免仅凭支持性证据数得出错误结论。
func AssessProofStandard(standard StandardOfProof, validCount int, total int, gaps []string) *ProofStandardResult {
	result := &ProofStandardResult{
		Standard:           string(standard),
		SupportingCount:    validCount,
		ContradictingCount: total - validCount,
		Met:                false,
		Gaps:               gaps,
	}

	if total <= 0 {
		result.Reasoning = "无可用证据，无法达到任何证明标准"
		return result
	}

	switch standard {
	case StandardBeyondReasonableDoubt:
		// 排除合理怀疑：有效证据占比 >= 0.95
		result.Met = float64(validCount)/float64(total) >= 0.95 && validCount >= 3
		result.Confidence = float64(validCount) / float64(total)
		if !result.Met {
			result.Reasoning = "排除合理怀疑标准：证据链不够完整，存在合理怀疑"
		} else {
			result.Reasoning = "排除合理怀疑标准：证据链完整，足以排除合理怀疑"
		}

	case StandardHighProbability:
		// 高度盖然性：有效证据占比 >= 0.80
		result.Met = float64(validCount)/float64(total) >= 0.80 && validCount >= 2
		result.Confidence = float64(validCount) / float64(total)
		if !result.Met {
			result.Reasoning = "高度盖然性标准：现有证据不足以形成高度盖然性优势"
		} else {
			result.Reasoning = "高度盖然性标准：现有证据已形成高度盖然性优势"
		}

	case StandardPreponderance:
		// 优势证据：有效证据占比 > 0.50
		result.Met = float64(validCount) > float64(total)*0.50
		result.Confidence = float64(validCount) / float64(total)
		if !result.Met {
			result.Reasoning = "优势证据标准：有效证据未超过半数"
		} else {
			result.Reasoning = "优势证据标准：有效证据超过半数，形成优势"
		}

	case StandardSubstantialEvidence:
		// 实质性证据：至少 1 份有效直接证据
		result.Met = validCount >= 1
		result.Confidence = float64(validCount) / float64(total)
		s := "实质性证据标准："
		if !result.Met {
			result.Reasoning = s + "缺乏实质性证据支持"
		} else {
			result.Reasoning = s + "存在实质性证据"
		}

	case StandardPrimaFacie:
		// 初步证据：至少 1 份表面有效证据
		result.Met = validCount >= 1
		result.Confidence = float64(validCount) / float64(total)
		if !result.Met {
			result.Reasoning = "初步证据标准：未达到初步证据要求"
		} else {
			result.Reasoning = "初步证据标准：已达初步证据要求"
		}

	default:
		result.Reasoning = "未知证明标准: " + string(standard)
		result.Confidence = 0
	}

	return result
}

// DetermineStandard 根据场景确定应当适用的证明标准。
func DetermineStandard(scenario BurdenScenario) StandardOfProof {
	switch scenario {
	case BurdenScenarioPatentInfringement:
		return StandardHighProbability
	case BurdenScenarioPriority:
		return StandardHighProbability
	case BurdenScenarioPatentInvalidation:
		return StandardPreponderance
	case BurdenScenarioNoveltyChallenge:
		return StandardPreponderance
	case BurdenScenarioInventiveness:
		return StandardPreponderance
	case BurdenScenarioDisclosure:
		return StandardPreponderance
	default:
		return StandardPreponderance
	}
}
