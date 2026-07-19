package writing

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// PatternStore is an in-memory store for WritingPatterns.
// It supports metadata-based search and seed-pattern loading from YAML files.
// FTS5/Vector integration can be added later by syncing to knowledge.Store.
type PatternStore struct {
	mu            sync.RWMutex
	patterns      map[string]*WritingPattern // id → pattern
	categoryIndex map[string][]string        // category → []patternID
}

// NewPatternStore creates an empty pattern store.
func NewPatternStore() *PatternStore {
	return &PatternStore{
		patterns:      make(map[string]*WritingPattern),
		categoryIndex: make(map[string][]string),
	}
}

// AddPattern adds a pattern to the store.
func (s *PatternStore) AddPattern(p *WritingPattern) error {
	if p.ID == "" {
		return fmt.Errorf("pattern ID is required")
	}
	if p.Quality <= 0 {
		p.Quality = 0.8
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patterns[p.ID] = p
	for _, cat := range p.ApplicableCategories() {
		s.categoryIndex[cat] = append(s.categoryIndex[cat], p.ID)
	}
	return nil
}

// Get returns a pattern by ID.
func (s *PatternStore) Get(id string) *WritingPattern {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.patterns[id]
}

// All returns all patterns.
func (s *PatternStore) All() []*WritingPattern {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WritingPattern, 0, len(s.patterns))
	for _, p := range s.patterns {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Count returns the number of patterns.
func (s *PatternStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.patterns)
}

// SearchPatterns searches patterns by query text and optional category filter.
// Returns up to topK results, sorted by relevance.
func (s *PatternStore) SearchPatterns(query string, category string, topK int) []*WritingPattern {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query = strings.ToLower(query)
	terms := strings.Fields(query)

	var candidates []*WritingPattern
	if category != "" {
		// Filter by category first.
		if ids, ok := s.categoryIndex[category]; ok {
			for _, id := range ids {
				if p, exists := s.patterns[id]; exists {
					candidates = append(candidates, p)
				}
			}
		}
	}
	if candidates == nil {
		for _, p := range s.patterns {
			candidates = append(candidates, p)
		}
	}

	type scored struct {
		p     *WritingPattern
		score float64
	}
	var scoredList []scored

	// If no search terms, return all candidates without scoring.
	if len(terms) == 0 {
		if len(candidates) > topK {
			candidates = candidates[:topK]
		}
		out := make([]*WritingPattern, len(candidates))
		copy(out, candidates)
		return out
	}

	for _, p := range candidates {
		score := scorePattern(p, terms)
		if score > 0 {
			scoredList = append(scoredList, scored{p, score})
		}
	}

	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].score > scoredList[j].score
	})
	if topK <= 0 {
		topK = 10
	}
	if len(scoredList) > topK {
		scoredList = scoredList[:topK]
	}
	out := make([]*WritingPattern, len(scoredList))
	for i, s := range scoredList {
		out[i] = s.p
	}
	return out
}

// MatchPatterns matches patterns suitable for a given case type and feature context.
func (s *PatternStore) MatchPatterns(caseType string, features []string) []*WritingPattern {
	s.mu.RLock()
	defer s.mu.RUnlock()

	queryTerms := append([]string{caseType}, features...)
	query := strings.ToLower(strings.Join(queryTerms, " "))

	type scored struct {
		p     *WritingPattern
		score float64
	}
	var results []scored

	for _, p := range s.patterns {
		score := 0.0
		// Category match: high weight.
		if strings.Contains(strings.ToLower(p.Category), strings.ToLower(caseType)) {
			score += 5.0
		}
		// Feature match in name/summary.
		for _, f := range features {
			f = strings.ToLower(f)
			if strings.Contains(strings.ToLower(p.Name), f) {
				score += 3.0
			}
			if strings.Contains(strings.ToLower(p.Summary), f) {
				score += 2.0
			}
			for _, s := range p.Steps {
				if strings.Contains(strings.ToLower(s.Name), f) {
					score += 1.5
				}
			}
		}
		// Keyword match.
		for _, kw := range p.Keywords() {
			if strings.Contains(query, strings.ToLower(kw)) {
				score += 1.0
			}
		}
		if score > 0 {
			results = append(results, scored{p, score})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	if len(results) > 5 {
		results = results[:5]
	}
	out := make([]*WritingPattern, len(results))
	for i, r := range results {
		out[i] = r.p
	}
	return out
}

// LoadSeedDir loads YAML seed pattern files from a directory.
// Each .yaml file should contain one or more WritingPatterns.
func (s *PatternStore) LoadSeedDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read seed dir %s: %w", dir, err)
	}
	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if err := s.loadSeedFile(filepath.Join(dir, e.Name())); err != nil {
			return loaded, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		loaded++
	}
	return loaded, nil
}

// loadSeedFile loads a single YAML seed pattern file.
func (s *PatternStore) loadSeedFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	// Single pattern or list.
	var single WritingPattern
	if err := yaml.Unmarshal(data, &single); err == nil && single.ID != "" {
		if single.Quality == 0 {
			single.Quality = 0.8
		}
		return s.AddPattern(&single)
	}
	// Try as list.
	var list struct {
		Patterns []WritingPattern `yaml:"patterns"`
	}
	if err := yaml.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for i := range list.Patterns {
		if list.Patterns[i].Quality == 0 {
			list.Patterns[i].Quality = 0.8
		}
		if err := s.AddPattern(&list.Patterns[i]); err != nil {
			return fmt.Errorf("add pattern %s: %w", list.Patterns[i].ID, err)
		}
	}
	return nil
}

// scorePattern computes a relevance score for a pattern against search terms.
func scorePattern(p *WritingPattern, terms []string) float64 {
	score := 0.0
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if strings.Contains(strings.ToLower(p.Name), term) {
			score += 3.0
		}
		if strings.Contains(strings.ToLower(p.Summary), term) {
			score += 2.0
		}
		if strings.Contains(strings.ToLower(p.Context), term) {
			score += 1.5
		}
		for _, s := range p.Steps {
			if strings.Contains(strings.ToLower(s.Name), term) {
				score += 1.0
			}
			if strings.Contains(strings.ToLower(s.Instruction), term) {
				score += 0.5
			}
		}
		for _, d := range p.Dos {
			if strings.Contains(strings.ToLower(d.Rule), term) {
				score += 0.5
			}
		}
	}
	return score * p.Quality
}
