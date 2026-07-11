package evidence

import "context"

type ledgerKey struct{}

// WithLedger returns a new context that carries the ledger.
func WithLedger(ctx context.Context, l *Ledger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ledgerKey{}, l)
}

// FromContext extracts the ledger from ctx, if present.
func FromContext(ctx context.Context) (*Ledger, bool) {
	l, ok := ctx.Value(ledgerKey{}).(*Ledger)
	return l, ok && l != nil
}
