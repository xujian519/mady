package compiler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
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
// Strategy selection uses time-decayed confidence when DecayConfig is set.
func (c *Compiler) StartTurn(goal string) (guidance string, strategyID string) {
	c.mu.Lock()
	strategies := make([]Strategy, len(c.strategies))
	copy(strategies, c.strategies)
	expRate := c.explorationRate
	decayCfg := c.decayCfg
	c.mu.Unlock()

	c.rngMu.Lock()
	rng := c.rng
	c.rngMu.Unlock()

	pick := SelectStrategyWithDecay(goal, strategies, expRate, decayCfg, rng)
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

	confidence := StrategyConfidence(*pick.Strategy, decayCfg)
	guidance = fmt.Sprintf("[策略建议] %s\n指导: %s\n(来源: %s, 置信度: %.0f%%)",
		pick.Strategy.Description, pick.Strategy.Guidance, pick.Reason,
		confidence*100)

	return guidance, pick.Strategy.ID
}

// FinishTurn records the execution outcome and updates strategy statistics.
// Trace signal quality (ClassifyQuality) affects the weight of the update:
// HIGH_SIGNAL traces count fully, MEDIUM_SIGNAL count as 0.5, NOISE is skipped.
func (c *Compiler) FinishTurn(trace ExecutionTrace) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update strategy stats (quality-weighted)
	if trace.StrategyID != "" {
		quality := ClassifyQuality(trace)
		for i := range c.strategies {
			if c.strategies[i].ID == trace.StrategyID {
				switch {
				case quality == QualityNoise:
					// Noise traces do not affect strategy statistics.
				case trace.Outcome.IsPositive():
					if quality == QualityHigh {
						c.strategies[i].Successes++
					} else {
						// MEDIUM_SIGNAL positive: 50% effective via alternation.
						c.strategies[i].successToggle = !c.strategies[i].successToggle
						if c.strategies[i].successToggle {
							c.strategies[i].Successes++
						}
					}
				case trace.Outcome == OutcomeFailure:
					if quality == QualityHigh {
						c.strategies[i].Failures++
					} else {
						// MEDIUM_SIGNAL failure: 50% effective via alternation.
						c.strategies[i].failureToggle = !c.strategies[i].failureToggle
						if c.strategies[i].failureToggle {
							c.strategies[i].Failures++
						}
					}
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

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

// persistData is the JSON-serializable snapshot of Compiler state.
type persistData struct {
	Strategies      []Strategy  `json:"strategies"`
	ExplorationRate int         `json:"exploration_rate"`
	MaxTraces       int         `json:"max_traces"`
	DecayConfig     DecayConfig `json:"decay_config"`
}

// Save persists the compiler's strategy statistics to a JSON file.
// Traces are not persisted (they are ephemeral diagnostics); only strategy
// stats and configuration are saved. Uses atomic write (temp file + rename).
// Returns an error if writing fails.
func (c *Compiler) Save(path string) error {
	c.mu.Lock()
	data := persistData{
		Strategies:      make([]Strategy, len(c.strategies)),
		ExplorationRate: c.explorationRate,
		MaxTraces:       c.maxTraces,
		DecayConfig:     c.decayCfg,
	}
	copy(data.Strategies, c.strategies)
	c.mu.Unlock()

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("compiler: marshal: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, b, 0o600); err != nil {
		return fmt.Errorf("compiler: write %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // cleanup
		return fmt.Errorf("compiler: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

// Load restores strategy statistics from a previously saved JSON file.
// If the file does not exist, Load returns nil (no-op) so callers can
// unconditionally call Load on startup. Existing in-memory strategies are
// replaced by the loaded data.
func (c *Compiler) Load(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no saved state, start fresh
		}
		return fmt.Errorf("compiler: read %s: %w", path, err)
	}

	var data persistData
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("compiler: unmarshal: %w", err)
	}
	if len(data.Strategies) == 0 {
		return nil // empty file, keep defaults
	}

	c.mu.Lock()
	c.strategies = data.Strategies
	if data.ExplorationRate > 0 {
		c.explorationRate = data.ExplorationRate
	}
	if data.MaxTraces > 0 {
		c.maxTraces = data.MaxTraces
	}
	if data.DecayConfig.WeeklyDecayRate > 0 {
		c.decayCfg = data.DecayConfig
	}
	c.mu.Unlock()
	return nil
}
