package smartrouter

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xujian519/mady/agentcore"
)

// TaskType labels the kind of work a request represents, used to match it
// against model strengths.
type TaskType string

const (
	TaskCoding    TaskType = "coding"
	TaskReasoning TaskType = "reasoning"
	TaskLegal     TaskType = "legal"
	TaskPatent    TaskType = "patent"
	TaskCreative  TaskType = "creative"
	TaskAnalysis  TaskType = "analysis"
	TaskGeneral   TaskType = "general"
)

// Priority controls how candidate profiles are ranked after task-type filtering.
type Priority string

const (
	PriorityQuality  Priority = "quality"
	PriorityCost     Priority = "cost"
	PriorityBalanced Priority = "balanced"
	PriorityLatency  Priority = "latency"
)

// ModelProfile describes a backend model's capabilities and cost/latency
// characteristics. At least one registered profile should cover TaskGeneral so
// that unmatched task types always have a candidate.
type ModelProfile struct {
	Name           string
	Provider       agentcore.Provider
	Strengths      []TaskType
	QualityScore   float64 // 0–1, higher is better
	CostPerMTokens float64 // USD per million tokens, lower is better
	LatencyMs      float64 // average latency in ms, lower is better
	MaxContext     int64   // max context window in tokens
}

// hasStrength reports whether the profile lists the given task type.
func (p ModelProfile) hasStrength(t TaskType) bool {
	if t == TaskGeneral {
		return true
	}
	for _, s := range p.Strengths {
		if s == t || s == TaskGeneral {
			return true
		}
	}
	return false
}

// TaskClassifier inspects a request and returns its task type.
type TaskClassifier interface {
	Classify(req *agentcore.ProviderRequest) TaskType
}

// RouteDecision captures a single routing decision for observability.
type RouteDecision struct {
	Profile  ModelProfile
	TaskType TaskType
	Reason   string
	Priority Priority
}

// RouteRecord is one entry in the route history.
type RouteRecord struct {
	Timestamp time.Time
	TaskType  TaskType
	Profile   string
	Success   bool
	LatencyMs float64
	Error     string
}

// RouteHistory collects routing decisions for audit and future adaptation.
type RouteHistory struct {
	mu      sync.Mutex
	records []RouteRecord
}

// NewRouteHistory creates an empty RouteHistory.
func NewRouteHistory() *RouteHistory { return &RouteHistory{} }

// Record appends a route record.
func (h *RouteHistory) Record(r RouteRecord) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
}

// Records returns a copy of all route records.
func (h *RouteHistory) Records() []RouteRecord {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RouteRecord, len(h.records))
	copy(out, h.records)
	return out
}

// Stats returns aggregate counts per task type and profile.
func (h *RouteHistory) Stats() map[TaskType]map[string]int {
	if h == nil {
		return nil
	}
	records := h.Records()
	stats := make(map[TaskType]map[string]int)
	for _, r := range records {
		if stats[r.TaskType] == nil {
			stats[r.TaskType] = make(map[string]int)
		}
		key := r.Profile
		if r.Success {
			key += ":success"
		} else {
			key += ":fail"
		}
		stats[r.TaskType][key]++
	}
	return stats
}

// SmartRouter is an [agentcore.Provider] that routes requests to the best
// backend model based on task classification and routing priority.
type SmartRouter struct {
	profiles      []ModelProfile
	classifier    TaskClassifier
	priority      Priority
	history       *RouteHistory
	enableFallback bool

	mu      sync.Mutex // guards lastDecision for observers
	lastDecision *RouteDecision
}

// Option configures a SmartRouter.
type Option func(*SmartRouter)

// WithPriority sets the ranking priority (default: balanced).
func WithPriority(p Priority) Option {
	return func(s *SmartRouter) { s.priority = p }
}

// WithClassifier sets a custom task classifier (default: DefaultClassifier).
func WithClassifier(c TaskClassifier) Option {
	return func(s *SmartRouter) { s.classifier = c }
}

// WithHistory sets a route history collector.
func WithHistory(h *RouteHistory) Option {
	return func(s *SmartRouter) { s.history = h }
}

// EnableFallback enables retrying with the next-best provider on Complete errors.
func EnableFallback() Option {
	return func(s *SmartRouter) { s.enableFallback = true }
}

// New creates a SmartRouter over the given profiles.
func New(profiles []ModelProfile, opts ...Option) *SmartRouter {
	s := &SmartRouter{
		profiles:   profiles,
		classifier: &DefaultClassifier{},
		priority:   PriorityBalanced,
		history:    NewRouteHistory(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Route selects the best profile for a request without executing it.
func (s *SmartRouter) Route(req *agentcore.ProviderRequest) RouteDecision {
	taskType := TaskGeneral
	if s.classifier != nil {
		taskType = s.classifier.Classify(req)
	}
	candidates := s.filterByTask(taskType)
	ranked := s.rank(candidates)
	var reason string
	if len(ranked) == 0 {
		reason = "no matching profile; using first available"
		if len(s.profiles) > 0 {
			ranked = s.profiles[:1]
		}
	} else if len(ranked) == 1 {
		reason = "single matching profile"
	} else {
		reason = "ranked by " + string(s.priority)
	}
	dec := RouteDecision{
		TaskType: taskType,
		Priority: s.priority,
		Reason:   reason,
	}
	if len(ranked) > 0 {
		dec.Profile = ranked[0]
	}
	s.mu.Lock()
	s.lastDecision = &dec
	s.mu.Unlock()
	return dec
}

// LastDecision returns the most recent routing decision, or nil if none.
func (s *SmartRouter) LastDecision() *RouteDecision {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastDecision == nil {
		return nil
	}
	cp := *s.lastDecision
	return &cp
}

// Complete implements agentcore.Provider. It routes the request, delegates to
// the selected backend, and optionally falls back on error.
func (s *SmartRouter) Complete(ctx context.Context, req *agentcore.ProviderRequest) (*agentcore.ProviderResponse, error) {
	taskType := TaskGeneral
	if s.classifier != nil {
		taskType = s.classifier.Classify(req)
	}
	candidates := s.rank(s.filterByTask(taskType))
	if len(candidates) == 0 && len(s.profiles) > 0 {
		candidates = s.profiles
	}

	var lastErr error
	for i, profile := range candidates {
		if !s.enableFallback && i > 0 {
			break
		}
		clonedReq := *req
		if profile.Name != "" {
			clonedReq.Model = profile.Name
		}
		start := time.Now()
		resp, err := profile.Provider.Complete(ctx, &clonedReq)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
			lastErr = err
			s.history.Record(RouteRecord{
				Timestamp: start, TaskType: taskType, Profile: profile.Name,
				Success: false, LatencyMs: latency, Error: errMsg,
			})
			continue
		}
		s.history.Record(RouteRecord{
			Timestamp: start, TaskType: taskType, Profile: profile.Name,
			Success: true, LatencyMs: latency,
		})
		return resp, nil
	}
	return nil, lastErr
}

// Stream implements agentcore.Provider. It routes the request and delegates to
// the selected backend. Fallback is not attempted once a stream begins.
func (s *SmartRouter) Stream(ctx context.Context, req *agentcore.ProviderRequest) (<-chan agentcore.StreamDelta, error) {
	taskType := TaskGeneral
	if s.classifier != nil {
		taskType = s.classifier.Classify(req)
	}
	candidates := s.rank(s.filterByTask(taskType))
	if len(candidates) == 0 && len(s.profiles) > 0 {
		candidates = s.profiles
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	profile := candidates[0]
	clonedReq := *req
	if profile.Name != "" {
		clonedReq.Model = profile.Name
	}
	s.mu.Lock()
	s.lastDecision = &RouteDecision{
		Profile: profile, TaskType: taskType, Reason: "stream routed", Priority: s.priority,
	}
	s.mu.Unlock()
	return profile.Provider.Stream(ctx, &clonedReq)
}

// filterByTask returns profiles whose Strengths include the task type. When no
// profile matches, it returns all profiles (general fallback).
func (s *SmartRouter) filterByTask(t TaskType) []ModelProfile {
	var matched []ModelProfile
	for _, p := range s.profiles {
		if p.hasStrength(t) {
			matched = append(matched, p)
		}
	}
	return matched
}

// rank sorts profiles according to the configured priority.
func (s *SmartRouter) rank(profiles []ModelProfile) []ModelProfile {
	if len(profiles) <= 1 {
		return profiles
	}
	cp := make([]ModelProfile, len(profiles))
	copy(cp, profiles)

	switch s.priority {
	case PriorityQuality:
		sort.Slice(cp, func(i, j int) bool { return cp[i].QualityScore > cp[j].QualityScore })
	case PriorityCost:
		sort.Slice(cp, func(i, j int) bool { return cp[i].CostPerMTokens < cp[j].CostPerMTokens })
	case PriorityLatency:
		sort.Slice(cp, func(i, j int) bool { return cp[i].LatencyMs < cp[j].LatencyMs })
	default: // balanced
		normalized := normalize(cp)
		sort.SliceStable(cp, func(i, j int) bool {
			return balancedScore(normalized[i]) > balancedScore(normalized[j])
		})
	}
	return cp
}

// balancedScore computes a 0–1 composite: 50% quality, 30% cost, 20% latency.
func balancedScore(p ModelProfile) float64 {
	return p.QualityScore*0.5 + (1-p.CostPerMTokens)*0.3 + (1-p.LatencyMs)*0.2
}

// normalize min-max scales CostPerMTokens and LatencyMs to [0,1] across the
// candidate set so that balancedScore produces comparable contributions.
func normalize(profiles []ModelProfile) []ModelProfile {
	if len(profiles) == 0 {
		return profiles
	}
	minCost, maxCost := profiles[0].CostPerMTokens, profiles[0].CostPerMTokens
	minLat, maxLat := profiles[0].LatencyMs, profiles[0].LatencyMs
	for _, p := range profiles[1:] {
		if p.CostPerMTokens < minCost {
			minCost = p.CostPerMTokens
		}
		if p.CostPerMTokens > maxCost {
			maxCost = p.CostPerMTokens
		}
		if p.LatencyMs < minLat {
			minLat = p.LatencyMs
		}
		if p.LatencyMs > maxLat {
			maxLat = p.LatencyMs
		}
	}
	out := make([]ModelProfile, len(profiles))
	for i, p := range profiles {
		out[i] = p
		if maxCost > minCost {
			out[i].CostPerMTokens = (p.CostPerMTokens - minCost) / (maxCost - minCost)
		} else {
			out[i].CostPerMTokens = 0
		}
		if maxLat > minLat {
			out[i].LatencyMs = (p.LatencyMs - minLat) / (maxLat - minLat)
		} else {
			out[i].LatencyMs = 0
		}
	}
	return out
}

// ============================================================================
// DefaultClassifier
// ============================================================================

// DefaultClassifier infers the task type from request content using keyword
// matching. It inspects the last user message and tool descriptions.
type DefaultClassifier struct {
	// KeywordMap overrides the built-in keyword sets. When nil, built-in sets
	// tuned for legal/patent and coding workloads are used.
	KeywordMap map[TaskType][]string
}

// Classify returns the best-matching task type, defaulting to TaskGeneral.
func (d *DefaultClassifier) Classify(req *agentcore.ProviderRequest) TaskType {
	if req == nil {
		return TaskGeneral
	}
	text := requestText(req)
	lower := strings.ToLower(text)
	kw := d.KeywordMap
	if kw == nil {
		kw = defaultKeywords
	}
	var best TaskType
	bestHits := 0
	order := []TaskType{TaskPatent, TaskLegal, TaskCoding, TaskReasoning, TaskCreative, TaskAnalysis}
	for _, tt := range order {
		hits := 0
		for _, k := range kw[tt] {
			if strings.Contains(lower, k) {
				hits++
			}
		}
		if hits > bestHits {
			bestHits = hits
			best = tt
		}
	}
	if best == "" {
		return TaskGeneral
	}
	return best
}

var defaultKeywords = map[TaskType][]string{
	TaskPatent: {
		"专利", "权利要求", "新颖性", "创造性", "实用性", "审查意见", "说明书",
		"侵权", "patent", "claim", "novelty",
	},
	TaskLegal: {
		"法律", "合同", "诉讼", "仲裁", "违约", "法条", "判决", "案件",
		"legal", "contract", "lawsuit",
	},
	TaskCoding: {
		"代码", "编程", "函数", "编译", "bug", "debug", "重构", "api",
		"code", "function", "compile", "refactor",
	},
	TaskReasoning: {
		"分析", "推理", "论证", "对比", "评估", "why", "explain", "reasoning",
	},
	TaskCreative: {
		"创作", "写作", "文案", "创意", "故事", "creative", "write", "story",
	},
	TaskAnalysis: {
		"统计", "数据", "报表", "趋势", "分析报告", "data", "analytics", "report",
	},
}

// requestText concatenates the last user message and tool descriptions into a
// single string for keyword scanning.
func requestText(req *agentcore.ProviderRequest) string {
	var b strings.Builder
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == agentcore.RoleUser {
			b.WriteString(req.Messages[i].Content)
			break
		}
	}
	for _, t := range req.Tools {
		b.WriteString(" ")
		b.WriteString(t.Name)
		b.WriteString(" ")
		b.WriteString(t.Description)
	}
	return b.String()
}
