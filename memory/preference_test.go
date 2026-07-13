package memory

import (
	"context"
	"testing"
)

func TestSaveUserPreference(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{UserID: "user-1"}

	pref := UserPreference{
		Key:      "writing_style",
		Value:    "简洁直接，避免冗余",
		Category: "style",
	}

	id, err := SaveUserPreference(ctx, store, scope, pref)
	if err != nil {
		t.Fatalf("SaveUserPreference failed: %v", err)
	}
	if id == "" {
		t.Fatal("returned empty ID")
	}

	entry, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry.Layer != LayerUser {
		t.Errorf("Layer = %s, want %s", entry.Layer, LayerUser)
	}
	if entry.Metadata["type"] != "preference" {
		t.Errorf("metadata type = %v, want preference", entry.Metadata["type"])
	}
	if entry.Metadata["category"] != "style" {
		t.Errorf("metadata category = %v, want style", entry.Metadata["category"])
	}
}

func TestSaveUserPreference_EmptyValue(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{UserID: "user-1"}

	_, err := SaveUserPreference(ctx, store, scope, UserPreference{Key: "k", Value: ""})
	if err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestSaveUserPreference_DefaultCategory(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{UserID: "user-1"}

	id, err := SaveUserPreference(ctx, store, scope, UserPreference{Key: "k", Value: "v"})
	if err != nil {
		t.Fatalf("SaveUserPreference failed: %v", err)
	}
	entry, _ := store.Get(ctx, id)
	if entry.Metadata["category"] != "general" {
		t.Errorf("category = %v, want general", entry.Metadata["category"])
	}
}

func TestLoadUserPreferences(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{UserID: "user-2"}

	prefs := []UserPreference{
		{Key: "style", Value: "正式法律文书风格", Category: "style"},
		{Key: "format", Value: "使用表格展示对比", Category: "format"},
		{Key: "citation", Value: "优先引用《专利法》第二十二条", Category: "citation"},
	}
	for _, p := range prefs {
		if _, err := SaveUserPreference(ctx, store, scope, p); err != nil {
			t.Fatalf("SaveUserPreference failed: %v", err)
		}
	}

	results, err := LoadUserPreferences(ctx, store, scope, "")
	if err != nil {
		t.Fatalf("LoadUserPreferences failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
}

func TestLoadUserPreferences_ByCategory(t *testing.T) {
	store := NewInMemoryStore()
	ctx := context.Background()
	scope := MemoryScope{UserID: "user-3"}

	prefs := []UserPreference{
		{Key: "style1", Value: "简洁风格", Category: "style"},
		{Key: "format1", Value: "表格格式", Category: "format"},
		{Key: "style2", Value: "正式风格", Category: "style"},
	}
	for _, p := range prefs {
		SaveUserPreference(ctx, store, scope, p)
	}

	results, err := LoadUserPreferences(ctx, store, scope, "style")
	if err != nil {
		t.Fatalf("LoadUserPreferences failed: %v", err)
	}

	for _, r := range results {
		cat, _ := r.Entry.Metadata["category"].(string)
		if cat != "style" && r.Entry.Content != "" {
			t.Errorf("expected category=style, got %s", cat)
		}
	}
}
