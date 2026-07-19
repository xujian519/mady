package evidence

import "sync"

// Ledger stores the receipts available for verification during the current turn.
// It is thread-safe and nil-safe: a nil ledger is a no-op for all methods.
type Ledger struct {
	mu       sync.RWMutex
	receipts []Receipt
}

// NewLedger creates an empty ledger.
func NewLedger() *Ledger { return &Ledger{} }

// Reset clears all receipts. Called at the start of each user turn.
func (l *Ledger) Reset() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.receipts = nil
}

// Record appends a receipt. Failed receipts are retained for auditability.
func (l *Ledger) Record(r Receipt) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.receipts = append(l.receipts, r)
}

// Len returns the number of receipts recorded this turn.
func (l *Ledger) Len() int {
	if l == nil {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.receipts)
}

// Snapshot returns a copy of all receipts for the current turn.
func (l *Ledger) Snapshot() []Receipt {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]Receipt, len(l.receipts))
	copy(out, l.receipts)
	return out
}
