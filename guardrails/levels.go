package guardrails

import (
	"context"
	"strings"
	"sync"

	"github.com/xujian519/mady/agentcore"
)

// Level defines the strictness tier of a guardrail.
type Level int

const (
	// LevelLight — basic content safety check. Suitable for general chat.
	// Only blocks obviously harmful content.
	LevelLight Level = iota

	// LevelStandard — adds uncertainty declaration requirements.
	// Suitable for professional advisory domains where AI limitations
	// should be disclosed.
	LevelStandard

	// LevelStrict — full disclaimer enforcement + human approval gate.
	// Suitable for legal, patent, and other high-stakes domains where
	// incorrect output could have real-world consequences.
	LevelStrict
)

var (
	customLevelsMu sync.RWMutex
	customLevels   = map[string]Level{}
)

// RegisterLevel registers a custom guardrail level name and associates it
// with the given Level value. The name is also registered with agentcore
// manifest validation so manifest files can reference it.
// Empty names or duplicate registrations are ignored.
func RegisterLevel(name string, level Level) {
	if name == "" {
		return
	}
	customLevelsMu.Lock()
	defer customLevelsMu.Unlock()
	customLevels[name] = level
	agentcore.RegisterValidGuardrailLevel(name)
}

// RegisteredLevel returns the Level value registered for name, if any.
func RegisteredLevel(name string) (Level, bool) {
	customLevelsMu.RLock()
	defer customLevelsMu.RUnlock()
	level, ok := customLevels[name]
	return level, ok
}

// Config configures a guardrail instance.
type Config struct {
	Level Level

	// Disclaimer is the disclaimer text appended to outputs that
	// contain risk-triggering keywords. If empty, a generic
	// disclaimer is used.
	Disclaimer string

	// RiskKeywords triggers the guardrail's intervention when found in
	// the model output. The action taken depends on the Level.
	RiskKeywords []string

	// ApprovalKeywords triggers the human approval gate (LevelStrict only).
	// When these keywords appear in the output, execution pauses and
	// waits for human confirmation.
	ApprovalKeywords []string

	// BlockedPhrases are phrases that, if present, cause the output
	// to be blocked entirely (all levels).
	BlockedPhrases []string

	// TimeoutMsg is shown when waiting for human approval.
	TimeoutMsg string
}

// Option is a functional option for configuring a guardrail.
type Option func(*Config)

// WithLevel sets the guardrail level.
func WithLevel(l Level) Option { return func(c *Config) { c.Level = l } }

// WithDisclaimer sets the disclaimer text.
func WithDisclaimer(d string) Option { return func(c *Config) { c.Disclaimer = d } }

// WithRiskKeywords sets keywords that trigger guardrail intervention.
func WithRiskKeywords(kw []string) Option { return func(c *Config) { c.RiskKeywords = kw } }

// WithApproval sets keywords that trigger human approval (LevelStrict).
func WithApproval(kw []string) Option { return func(c *Config) { c.ApprovalKeywords = kw } }

// WithBlockedPhrases sets phrases that are always blocked.
func WithBlockedPhrases(p []string) Option { return func(c *Config) { c.BlockedPhrases = p } }

// New creates a guardrail LifecycleHook with the given options.
func New(opts ...Option) agentcore.LifecycleHook {
	cfg := Config{
		Level:          LevelLight,
		BlockedPhrases: []string{"恶意代码", "攻击方法", "非法入侵"},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &guardrail{config: cfg}
}

type guardrail struct {
	agentcore.BaseLifecycleHook
	config Config
}

func (g *guardrail) AfterModelCall(_ context.Context, _ *agentcore.AgentRunContext, mcc *agentcore.ModelCallContext) {
	if mcc == nil || mcc.Response == nil || mcc.Err != nil {
		return
	}

	content := mcc.Response.Content

	// Step 1: Blocked phrases — reject entirely (all levels).
	for _, phrase := range g.config.BlockedPhrases {
		if strings.Contains(content, phrase) {
			mcc.Err = agentcore.NewNodeError("内容安全检查未通过", nil, "guardrail", phrase)
			mcc.Response.Content = "抱歉，该回复因内容安全原因被拦截。"
			return
		}
	}

	// Step 2: Risk keywords — append disclaimer (Standard, Strict).
	if g.config.Level >= LevelStandard && len(g.config.RiskKeywords) > 0 {
		disclaimer := g.config.Disclaimer
		if disclaimer == "" {
			disclaimer = "⚠️ 本回复由 AI 生成，仅供参考，不构成专业建议。"
		}
		if g.hasRiskKeyword(content) && !strings.Contains(content, disclaimer) {
			mcc.Response.Content = content + "\n\n---\n" + disclaimer
		}
	}

	// Step 3: Approval keywords — suppress persistence before human review (Strict only).
	// This complements domains.ApprovalGate: the gate pauses execution and prompts
	// the human operator, while SuppressPersist ensures the un-reviewed output is
	// not written to the session store until approval is granted.
	if g.config.Level >= LevelStrict && len(g.config.ApprovalKeywords) > 0 {
		if g.hasApprovalKeyword(content) {
			mcc.Response.SuppressPersist = true
		}
	}
}

func (g *guardrail) hasRiskKeyword(content string) bool {
	for _, kw := range g.config.RiskKeywords {
		if strings.Contains(content, kw) {
			return true
		}
	}
	return false
}

func (g *guardrail) hasApprovalKeyword(content string) bool {
	for _, kw := range g.config.ApprovalKeywords {
		if strings.Contains(content, kw) {
			return true
		}
	}
	return false
}
