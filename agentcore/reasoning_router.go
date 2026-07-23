package agentcore

import (
	"context"
	"strings"
)

// Complexity rates how demanding a turn is expected to be, driving the
// reasoning effort and token budget the router assigns to it.
type Complexity int

const (
	ComplexityLow Complexity = iota
	ComplexityMedium
	ComplexityHigh
)

func (c Complexity) String() string {
	switch c {
	case ComplexityLow:
		return "low"
	case ComplexityMedium:
		return "medium"
	case ComplexityHigh:
		return "high"
	}
	return "unknown"
}

// ComplexityClassifier scores the complexity of a turn from the latest user
// input and the conversation so far.
type ComplexityClassifier interface {
	Classify(input string, messages []Message) Complexity
}

// ReasoningRouter dynamically adjusts thinking effort and budget per turn
// based on a ComplexityClassifier. It implements LifecycleHook so it composes
// transparently with other hooks (register via WithLifecycle).
//
// When the agent already requests a non-default thinking config, the router
// only refines Effort/Budget for the current complexity; it never clears an
// explicitly configured IncludeThoughts/Display.
type ReasoningRouter struct {
	BaseLifecycleHook
	Classifier ComplexityClassifier
	// Efforts maps a complexity level to a thinking effort.
	Efforts map[Complexity]ThinkingEffort
	// Budgets maps a complexity level to a reasoning-token budget (0 = default).
	Budgets map[Complexity]int64
	// Decision, when set, is invoked with each classification for tracing.
	Decision func(turn int64, c Complexity)
}

// NewReasoningRouter builds a router with sensible defaults:
//
//	Low    → effort low
//	Medium → effort medium
//	High   → effort high
//
// Override Efforts/Budgets on the returned value to customize.
func NewReasoningRouter(classifier ComplexityClassifier) *ReasoningRouter {
	return &ReasoningRouter{
		Classifier: classifier,
		Efforts: map[Complexity]ThinkingEffort{
			ComplexityLow:    ThinkingEffortLow,
			ComplexityMedium: ThinkingEffortMedium,
			ComplexityHigh:   ThinkingEffortHigh,
		},
		Budgets: map[Complexity]int64{},
	}
}

// BeforeModelCall classifies the current turn and refines the request thinking
// effort/budget accordingly.
func (r *ReasoningRouter) BeforeModelCall(_ context.Context, arc *AgentRunContext, mcc *ModelCallContext) error {
	if r.Classifier == nil || mcc == nil || mcc.Request == nil {
		return nil
	}
	input := latestUserInput(arc)
	c := r.Classifier.Classify(input, arc.Messages)
	if r.Decision != nil {
		r.Decision(arc.Turn, c)
	}
	if mcc.Request.Thinking == nil {
		mcc.Request.Thinking = &ThinkingConfig{}
	}
	if effort, ok := r.Efforts[c]; ok && effort != ThinkingEffortDefault {
		mcc.Request.Thinking.Effort = effort
	}
	if budget, ok := r.Budgets[c]; ok {
		mcc.Request.Thinking.Budget = budget
	}
	return nil
}

// DefaultClassifier is a zero-dependency heuristic: keyword signals raise
// complexity to High; otherwise input length and conversation depth decide
// Low / Medium / High.
type DefaultClassifier struct {
	// HighKeywords mark turns that need deep reasoning regardless of length.
	HighKeywords []string
	// MediumRuneLen: inputs longer than this are at least Medium.
	MediumRuneLen int
	// HighRuneLen: inputs longer than this are High (unless keywords apply).
	HighRuneLen int
	// HistoryTurnsForHigh: when the conversation has at least this many prior
	// turns, complexity is bumped one level (context accumulation).
	HistoryTurnsForHigh int64
}

// NewDefaultClassifier returns a classifier tuned for legal/patent and general
// reasoning workloads.
func NewDefaultClassifier() *DefaultClassifier {
	return &DefaultClassifier{
		HighKeywords: []string{
			"分析", "推理", "对比", "论证", "法律", "专利", "侵权", "新颖性", "创造性",
			"审查意见", "权利要求", "架构", "设计", "debug", "排查", "重构",
			// English keywords restricted to domain-specific compound forms
			// to avoid over-triggering High complexity on casual English
			// questions like "why is X?" or "explain this".
			"analyze", "architect", "troubleshoot",
		},
		MediumRuneLen:       200,
		HighRuneLen:         800,
		HistoryTurnsForHigh: 6,
	}
}

// Classify returns the estimated complexity for a turn.
func (d *DefaultClassifier) Classify(input string, messages []Message) Complexity {
	lower := strings.ToLower(input)
	for _, kw := range d.HighKeywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return ComplexityHigh
		}
	}
	medium := d.MediumRuneLen
	if medium <= 0 {
		medium = 200
	}
	high := d.HighRuneLen
	if high <= 0 {
		high = 800
	}
	n := runeLen(input)
	var c Complexity
	switch {
	case n > high:
		c = ComplexityHigh
	case n > medium:
		c = ComplexityMedium
	default:
		c = ComplexityLow
	}
	// Bump on long conversations (accumulated context raises difficulty).
	if d.HistoryTurnsForHigh > 0 && int64(len(messages)) >= d.HistoryTurnsForHigh && c < ComplexityHigh {
		c++
	}
	return c
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func latestUserInput(arc *AgentRunContext) string {
	if arc == nil {
		return ""
	}
	if arc.Input != "" {
		return arc.Input
	}
	for i := len(arc.Messages) - 1; i >= 0; i-- {
		m := arc.Messages[i]
		if m.Role == RoleUser && m.Content != "" {
			return m.Content
		}
	}
	return ""
}
