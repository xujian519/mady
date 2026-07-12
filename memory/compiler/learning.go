package compiler

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Compiler manages strategy learning and execution traces.
// It is thread-safe and can be shared across agent sessions.
type Compiler struct {
	mu              sync.Mutex
	strategies      []Strategy
	traces          []ExecutionTrace
	explorationRate int // ε for ε-greedy (0-100, default 5)
	maxTraces       int // circular buffer limit
	decayCfg        DecayConfig
	rng             *rand.Rand
	rngMu           sync.Mutex
}

// Config configures a Compiler.
type Config struct {
	Strategies      []Strategy // initial strategies (nil = DefaultStrategies)
	ExplorationRate int        // ε-greedy exploration % (0-100, default 5)
	MaxTraces       int        // max traces to keep (default 1000)
	DecayConfig     DecayConfig
}

// NewCompiler creates a strategy-learning compiler.
func NewCompiler(cfg Config) *Compiler {
	if cfg.Strategies == nil {
		cfg.Strategies = DefaultStrategies()
	}
	if cfg.ExplorationRate < 0 || cfg.ExplorationRate > 100 {
		cfg.ExplorationRate = 5
	}
	if cfg.MaxTraces <= 0 {
		cfg.MaxTraces = 1000
	}
	if cfg.DecayConfig.WeeklyDecayRate == 0 {
		cfg.DecayConfig = DefaultDecayConfig()
	}
	return &Compiler{
		strategies:      cfg.Strategies,
		explorationRate: cfg.ExplorationRate,
		maxTraces:       cfg.MaxTraces,
		decayCfg:        cfg.DecayConfig,
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// StartTurn selects a strategy for the given goal and returns compiled guidance.
// The returned guidance string can be injected into the agent's context.
func (c *Compiler) StartTurn(goal string) (guidance string, strategyID string) {
	c.mu.Lock()
	strategies := make([]Strategy, len(c.strategies))
	copy(strategies, c.strategies)
	expRate := c.explorationRate
	c.mu.Unlock()

	c.rngMu.Lock()
	rng := c.rng
	c.rngMu.Unlock()

	pick := SelectStrategy(goal, strategies, expRate, rng)
	if pick.Strategy == nil {
		return "", ""
	}

	c.mu.Lock()
	for i := range c.strategies {
		if c.strategies[i].ID == pick.Strategy.ID {
			c.strategies[i].LastUsedAt = time.Now()
			break
		}
	}
	c.mu.Unlock()

	guidance = fmt.Sprintf("[策略建议] %s\n指导: %s\n(来源: %s, 成功率: %.0f%%)",
		pick.Strategy.Description, pick.Strategy.Guidance, pick.Reason,
		pick.Strategy.SuccessRate()*100)

	return guidance, pick.Strategy.ID
}

// FinishTurn records the execution outcome and updates strategy statistics.
func (c *Compiler) FinishTurn(trace ExecutionTrace) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update strategy stats
	if trace.StrategyID != "" {
		for i := range c.strategies {
			if c.strategies[i].ID == trace.StrategyID {
				if trace.Outcome.IsPositive() {
					c.strategies[i].Successes++
				} else if trace.Outcome == OutcomeFailure {
					c.strategies[i].Failures++
				}
				break
			}
		}
	}

	// Store trace (circular buffer)
	c.traces = append(c.traces, trace)
	if len(c.traces) > c.maxTraces {
		c.traces = c.traces[len(c.traces)-c.maxTraces:]
	}
}

// Strategies returns a snapshot of all strategies with current stats.
func (c *Compiler) Strategies() []Strategy {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Strategy, len(c.strategies))
	copy(out, c.strategies)
	return out
}

// Traces returns a snapshot of recent traces.
func (c *Compiler) Traces(limit int) []ExecutionTrace {
	c.mu.Lock()
	defer c.mu.Unlock()
	if limit <= 0 || limit > len(c.traces) {
		limit = len(c.traces)
	}
	out := make([]ExecutionTrace, limit)
	copy(out, c.traces[len(c.traces)-limit:])
	return out
}

// StrategyByID returns a strategy by ID.
func (c *Compiler) StrategyByID(id string) (*Strategy, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.strategies {
		if c.strategies[i].ID == id {
			s := c.strategies[i]
			return &s, true
		}
	}
	return nil, false
}

// Stats returns summary statistics.
type Stats struct {
	TotalStrategies int
	TotalTraces     int
	SuccessTraces   int
	FailureTraces   int
}

func (c *Compiler) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Stats{
		TotalStrategies: len(c.strategies),
		TotalTraces:     len(c.traces),
	}
	for _, t := range c.traces {
		switch t.Outcome {
		case OutcomeSuccess:
			s.SuccessTraces++
		case OutcomeFailure:
			s.FailureTraces++
		}
	}
	return s
}
