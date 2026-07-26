package agentcore

import (
	"errors"
	"testing"
)

func TestBudgetIsUnlimited(t *testing.T) {
	if !(Budget{}).IsUnlimited() {
		t.Fatal("zero budget should be unlimited")
	}
	if (Budget{MaxTokens: 1}).IsUnlimited() {
		t.Fatal("budget with a limit should not be unlimited")
	}
}

func TestBudgetExceededErrorUnwrap(t *testing.T) {
	wrapped := NewNodeError("lifecycle before_model_call failed",
		&BudgetExceededError{Dimension: BudgetDimTokens, Limit: 10, Used: 12},
		"agent", "turn:1")
	if !IsBudgetExceeded(wrapped) {
		t.Fatal("IsBudgetExceeded should detect wrapped budget error")
	}
	if !errors.Is(wrapped, ErrBudgetExceeded) {
		t.Fatal("errors.Is should match the sentinel through NewNodeError")
	}
	var be *BudgetExceededError
	if !errors.As(wrapped, &be) {
		t.Fatal("errors.As should extract *BudgetExceededError")
	}
	if be.Dimension != BudgetDimTokens {
		t.Fatalf("unexpected dimension %s", be.Dimension)
	}
}
