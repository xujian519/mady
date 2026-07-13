package memory

import (
	"context"
	"fmt"
	"strings"
)

// UserPreference represents a single user preference entry. Preferences are
// stored in the LayerUser memory layer, persisting across sessions.
//
// Categories:
//   - "style": writing style preferences (e.g., concise, formal)
//   - "citation": preferred citation formats or frequently cited articles
//   - "format": output format preferences (e.g., bullet points, tables)
//   - "domain": domain-specific preferences (e.g., patent draft structure)
type UserPreference struct {
	Key      string `json:"key"`      // short identifier (e.g., "writing_style")
	Value    string `json:"value"`    // preference content
	Category string `json:"category"` // style | citation | format | domain
}

// SaveUserPreference stores a user preference as a LayerUser memory entry.
// The content is formatted as "用户偏好 | 类别: {category} | {key}: {value}"
// for keyword-rich retrieval. Metadata includes type=preference and category.
//
// Returns the memory entry ID on success.
func SaveUserPreference(ctx context.Context, store MemoryStore, scope MemoryScope, pref UserPreference) (string, error) {
	if pref.Value == "" {
		return "", fmt.Errorf("memory: preference value is empty")
	}
	if pref.Category == "" {
		pref.Category = "general"
	}

	content := fmt.Sprintf("用户偏好 | 类别: %s | %s: %s", pref.Category, pref.Key, pref.Value)
	metadata := map[string]any{
		"type":     "preference",
		"category": pref.Category,
		"key":      pref.Key,
	}

	id, err := store.Remember(ctx, content, scope, LayerUser, metadata)
	if err != nil {
		return "", fmt.Errorf("memory: save preference: %w", err)
	}
	return id, nil
}

// LoadUserPreferences retrieves user preferences by category. If category is
// empty, all preferences for the given scope are returned.
func LoadUserPreferences(ctx context.Context, store MemoryStore, scope MemoryScope, category string) ([]ScoredMemory, error) {
	query := "用户偏好"
	if category != "" {
		query = "用户偏好 类别 " + category
	}

	filter := MemoryFilter{
		UserID: scope.UserID,
		Layer:  LayerUser,
		TopK:   50,
	}

	results, err := store.Recall(ctx, query, filter)
	if err != nil {
		return nil, fmt.Errorf("memory: load preferences: %w", err)
	}

	if category == "" {
		return results, nil
	}

	var filtered []ScoredMemory
	for _, r := range results {
		if cat, ok := r.Entry.Metadata["category"].(string); ok && cat == category {
			filtered = append(filtered, r)
		} else if strings.Contains(r.Entry.Content, "类别: "+category) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}
