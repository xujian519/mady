package graph

import "fmt"

// DegradationReason 分类降级原因。
type DegradationReason string

const (
	// DegradationRetrieverNil 检索器未配置。
	DegradationRetrieverNil DegradationReason = "retriever_unavailable"
	// DegradationSearchFailed 检索执行失败。
	DegradationSearchFailed DegradationReason = "search_failed"
	// DegradationNotImplemented 功能尚未实现（占位）。
	DegradationNotImplemented DegradationReason = "not_implemented"
)

// DegradationMark 标记工作流中某阶段的降级状态。
// 存储于 PregelState 中，key 为原始 key + "__degradation" 后缀。
type DegradationMark struct {
	// Reason 降级原因分类。
	Reason DegradationReason `json:"reason"`
	// Message 人类可读的描述。
	Message string `json:"message"`
	// Severity 严重程度："warning" 或 "critical"。
	Severity string `json:"severity"`
}

const degradationKeySuffix = "__degradation"

// degradationKey 返回降级标记的 state key。
func degradationKey(valueKey string) string {
	return valueKey + degradationKeySuffix
}

// IsDegraded 检查 state 中指定 key 的值是否被降级。
func IsDegraded(state PregelState, valueKey string) bool {
	_, ok := state[degradationKey(valueKey)]
	return ok
}

// GetDegradationMark 返回指定 key 的降级标记，nil 表示未降级。
func GetDegradationMark(state PregelState, valueKey string) *DegradationMark {
	raw, ok := state[degradationKey(valueKey)]
	if !ok {
		return nil
	}
	mark, ok := raw.(DegradationMark)
	if !ok {
		return nil
	}
	if mark.Reason == "" {
		return nil
	}
	return &mark
}

// MarkDegraded 在 state 中同时存储降级值和其降级标记。
// fallback 是降级时使用的替代值（如空 slice）。
func MarkDegraded(state PregelState, valueKey string, fallback any, reason DegradationReason, message string) {
	state[valueKey] = fallback
	state[degradationKey(valueKey)] = DegradationMark{
		Reason:   reason,
		Message:  message,
		Severity: "warning",
	}
}

// MarkDegradedCritical 同 MarkDegraded，但严重程度为 critical。
func MarkDegradedCritical(state PregelState, valueKey string, fallback any, reason DegradationReason, message string) {
	state[valueKey] = fallback
	state[degradationKey(valueKey)] = DegradationMark{
		Reason:   reason,
		Message:  message,
		Severity: "critical",
	}
}

// Error 实现 error 接口，便于日志输出。
func (d DegradationMark) Error() string {
	return fmt.Sprintf("[%s] %s", d.Reason, d.Message)
}

// HasDegradation 检查 state 中是否存在任何降级标记。
func HasDegradation(state PregelState) bool {
	for k := range state {
		if len(k) > len(degradationKeySuffix) &&
			k[len(k)-len(degradationKeySuffix):] == degradationKeySuffix {
			return true
		}
	}
	return false
}

// DegradationSummary 返回 state 中所有降级标记的摘要。
func DegradationSummary(state PregelState) []DegradationMark {
	var marks []DegradationMark
	for k, v := range state {
		if len(k) > len(degradationKeySuffix) &&
			k[len(k)-len(degradationKeySuffix):] == degradationKeySuffix {
			if mark, ok := v.(DegradationMark); ok && mark.Reason != "" {
				marks = append(marks, mark)
			}
		}
	}
	return marks
}
