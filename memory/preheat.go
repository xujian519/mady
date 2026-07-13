package memory

import (
	"context"
	"fmt"
)

// PreheatCaseMemory stores a case summary as a high-importance long-term
// memory entry so that the Agent can "remember" the case context when
// resuming in a future session. This implements Tier 1 of the memory
// self-learning pipeline (B2): case-memory preheating from persisted
// StageCheckpoints.
//
// The summary string should be produced by the caller (typically from
// reasoning.ExtractCaseSummary + String()). The memory package deliberately
// does not depend on domains/reasoning to respect the dependency-inversion
// rule.
//
// Returns the memory entry ID on success.
func PreheatCaseMemory(ctx context.Context, store MemoryStore, scope MemoryScope, caseID, summary string) (string, error) {
	if summary == "" {
		return "", fmt.Errorf("memory: cannot preheat empty case summary")
	}

	metadata := map[string]any{
		"type":    "case_preheat",
		"case_id": caseID,
	}

	id, err := store.Remember(ctx, summary, scope, LayerLongTerm, metadata)
	if err != nil {
		return "", fmt.Errorf("memory: preheat case %s: %w", caseID, err)
	}
	return id, nil
}
