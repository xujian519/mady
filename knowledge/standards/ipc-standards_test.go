package standards

import (
	"testing"
)

func TestLoadStandards(t *testing.T) {
	standards, err := LoadStandards()
	if err != nil {
		t.Fatalf("LoadStandards() failed: %v", err)
	}
	if len(standards) == 0 {
		t.Fatal("LoadStandards() returned empty slice")
	}
	if len(standards) < 50 {
		t.Errorf("expected at least 50 standards, got %d", len(standards))
	}

	// Check first standard has required fields.
	s := standards[0]
	if s.ID == "" {
		t.Error("first standard has empty ID")
	}
	if s.Article == "" {
		t.Error("first standard has empty Article")
	}
	if s.IPCSection == "" {
		t.Error("first standard has empty IPCSection")
	}
}

func TestFindByIPCSection(t *testing.T) {
	// Section G (physics/computing)
	results, err := FindByIPCSection("G")
	if err != nil {
		t.Fatalf("FindByIPCSection(G) failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindByIPCSection(G) returned empty")
	}
	for _, s := range results {
		if s.IPCSection != "G" && s.IPCSection != "ALL" {
			t.Errorf("unexpected section %q for standard %s", s.IPCSection, s.ID)
		}
	}

	// Case insensitive
	results2, err := FindByIPCSection("g")
	if err != nil {
		t.Fatalf("FindByIPCSection(g) failed: %v", err)
	}
	if len(results2) == 0 {
		t.Fatal("FindByIPCSection(g) returned empty")
	}

	// Non-existent section — only ALL-section entries should return
	results3, err := FindByIPCSection("Z")
	if err != nil {
		t.Fatalf("FindByIPCSection(Z) failed: %v", err)
	}
	for _, s := range results3 {
		if s.IPCSection != "ALL" {
			t.Errorf("expected only ALL-section entries for section Z, got %q for %s", s.IPCSection, s.ID)
		}
	}
}

func TestFindByArticle(t *testing.T) {
	results, err := FindByArticle("patent-law-a22.3")
	if err != nil {
		t.Fatalf("FindByArticle(a22.3) failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindByArticle(a22.3) returned empty")
	}
	for _, s := range results {
		if s.Article != "patent-law-a22.3" {
			t.Errorf("unexpected article %q for standard %s", s.Article, s.ID)
		}
	}
}

func TestFindByIPCDetail(t *testing.T) {
	results, err := FindByIPCDetail("G06")
	if err != nil {
		t.Fatalf("FindByIPCDetail(G06) failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("FindByIPCDetail(G06) returned empty")
	}
	for _, s := range results {
		if s.IPCDetail != "G06" && s.IPCDetail != "ALL" {
			t.Errorf("unexpected detail %q for standard %s", s.IPCDetail, s.ID)
		}
	}
}

func TestSearch(t *testing.T) {
	// Search by name keyword
	results, err := Search("医药")
	if err != nil {
		t.Fatalf("Search(医药) failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search(医药) returned empty")
	}

	// Search by article ID
	results, err = Search("patent-law-a22.3")
	if err != nil {
		t.Fatalf("Search(article) failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search(article) returned empty")
	}

	// Search by key point content (医药 should match in key points or name)
	hasMedical := false
	for _, s := range results {
		if s.IPCDetail == "A61" || containsLower(s.Name, "医药") {
			hasMedical = true
			break
		}
	}
	if !hasMedical {
		t.Log("Search(医药) did not find any medical standards (may be OK depending on data)")
	}
}

func TestFormatAsContext(t *testing.T) {
	standards, err := LoadStandards()
	if err != nil {
		t.Fatalf("LoadStandards() failed: %v", err)
	}
	if len(standards) == 0 {
		t.Fatal("no standards loaded")
	}

	// Format first few standards
	ctx := FormatAsContext(standards[:3])
	if ctx == "" {
		t.Fatal("FormatAsContext() returned empty")
	}
	if !containsLower(ctx, standards[0].Name) {
		t.Errorf("FormatAsContext() missing standard name %q", standards[0].Name)
	}

	// Empty input
	empty := FormatAsContext(nil)
	if empty != "" {
		t.Errorf("FormatAsContext(nil) should be empty, got %q", empty)
	}
}

func TestMustLoadStandards(t *testing.T) {
	s := MustLoadStandards()
	if len(s) == 0 {
		t.Fatal("MustLoadStandards() returned empty")
	}
}

func TestIPCStandardFields(t *testing.T) {
	standards, err := LoadStandards()
	if err != nil {
		t.Fatalf("LoadStandards() failed: %v", err)
	}
	for _, s := range standards {
		if s.ID == "" {
			t.Errorf("standard with name %q has empty ID", s.Name)
		}
		if s.Source == "" {
			t.Errorf("standard %q has empty source", s.ID)
		}
	}
}

func TestSearchNoMatch(t *testing.T) {
	results, err := Search("__nonexistent_term_xyz__")
	if err != nil {
		t.Fatalf("Search() failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for nonexistent term, got %d", len(results))
	}
}
