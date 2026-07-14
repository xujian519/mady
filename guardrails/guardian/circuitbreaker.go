package guardian

import (
	"sync"
	"time"
)

// Circuit breaker constants.
const (
	// maxConsecutiveDenials: after N consecutive denials, trip the breaker.
	maxConsecutiveDenials = 3
	// reviewTimeout is the max time for a single review call.
	reviewTimeout = 30 * time.Second
	// halfOpenSuccessThreshold: number of consecutive successful allows
	// needed to auto-recover from a tripped state (half-open pattern).
	halfOpenSuccessThreshold = 5
)

// CircuitBreaker tracks denial patterns to detect runaway blocking.
// When tripped, all subsequent calls are auto-denied without LLM review.
// The breaker implements a half-open recovery pattern: after tripping,
// it requires N consecutive successes before auto-recovering.
// All methods are safe for concurrent use.
type CircuitBreaker struct {
	mu                 sync.Mutex
	consecutiveDenials int
	consecutiveSuccess int // successes after tripping (half-open recovery)
	totalReviews       int
	totalDenials       int
	tripped            bool
}

// Allow reports whether the breaker permits a new review.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return !cb.tripped
}

// RecordDenial registers a denial. If consecutive denials reach the threshold,
// the breaker trips.
func (cb *CircuitBreaker) RecordDenial() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalReviews++
	cb.totalDenials++
	cb.consecutiveDenials++
	cb.consecutiveSuccess = 0
	if cb.consecutiveDenials >= maxConsecutiveDenials {
		cb.tripped = true
	}
}

// RecordAllow registers a successful allow, resetting consecutive denials.
// When the breaker is tripped, consecutive successes are tracked for
// half-open auto-recovery.
func (cb *CircuitBreaker) RecordAllow() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalReviews++
	cb.consecutiveDenials = 0
	if cb.tripped {
		cb.consecutiveSuccess++
		if cb.consecutiveSuccess >= halfOpenSuccessThreshold {
			cb.tripped = false
			cb.consecutiveSuccess = 0
		}
	}
}

// IsTripped reports whether the breaker has tripped.
func (cb *CircuitBreaker) IsTripped() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.tripped
}

// Reset clears the breaker state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveDenials = 0
	cb.consecutiveSuccess = 0
	cb.tripped = false
}

// Stats returns current breaker statistics.
type BreakerStats struct {
	TotalReviews       int
	TotalDenials       int
	ConsecutiveDenials int
	Tripped            bool
}

func (cb *CircuitBreaker) Stats() BreakerStats {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return BreakerStats{
		TotalReviews:       cb.totalReviews,
		TotalDenials:       cb.totalDenials,
		ConsecutiveDenials: cb.consecutiveDenials,
		Tripped:            cb.tripped,
	}
}
