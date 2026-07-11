package guardian

import "time"

// Circuit breaker constants.
const (
	// maxConsecutiveDenials: after N consecutive denials, trip the breaker.
	maxConsecutiveDenials = 3
	// reviewTimeout is the max time for a single review call.
	reviewTimeout = 30 * time.Second
)

// CircuitBreaker tracks denial patterns to detect runaway blocking.
// When tripped, all subsequent calls are auto-denied without LLM review.
type CircuitBreaker struct {
	consecutiveDenials int
	totalReviews       int
	totalDenials       int
	tripped            bool
}

// Allow reports whether the breaker permits a new review.
func (cb *CircuitBreaker) Allow() bool {
	return !cb.tripped
}

// RecordDenial registers a denial. If consecutive denials reach the threshold,
// the breaker trips.
func (cb *CircuitBreaker) RecordDenial() {
	cb.totalReviews++
	cb.totalDenials++
	cb.consecutiveDenials++
	if cb.consecutiveDenials >= maxConsecutiveDenials {
		cb.tripped = true
	}
}

// RecordAllow registers a successful allow, resetting consecutive denials.
func (cb *CircuitBreaker) RecordAllow() {
	cb.totalReviews++
	cb.consecutiveDenials = 0
}

// IsTripped reports whether the breaker has tripped.
func (cb *CircuitBreaker) IsTripped() bool { return cb.tripped }

// Reset clears the breaker state.
func (cb *CircuitBreaker) Reset() {
	cb.consecutiveDenials = 0
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
	return BreakerStats{
		TotalReviews:       cb.totalReviews,
		TotalDenials:       cb.totalDenials,
		ConsecutiveDenials: cb.consecutiveDenials,
		Tripped:            cb.tripped,
	}
}
