package loader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CardEntry represents one card in card-index.json.
type CardEntry struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Concept         string   `json:"concept"`
	Quality         float64  `json:"quality"`
	Domain          string   `json:"domain"`
	FilePath        string   `json:"file_path"`
	RelatedConcepts []string `json:"related_concepts"`
	GeneratedAt     string   `json:"generated_at"`
	Version         int      `json:"version"`
}

// CardIndex is the top-level structure of card-index.json.
type CardIndex struct {
	TotalCards   int                 `json:"total_cards"`
	LastUpdated  string              `json:"last_updated"`
	Cards        []CardEntry         `json:"cards"`
	ConceptIndex map[string][]string `json:"concept_index,omitempty"`
	DomainIndex  map[string][]string `json:"domain_index,omitempty"`
}

// LoadCardIndex reads and parses card-index.json from the wiki root.
func LoadCardIndex(wikiPath string) (*CardIndex, error) {
	indexPath := filepath.Join(wikiPath, "card-index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("wiki: read card-index.json: %w", err)
	}

	var idx CardIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("wiki: parse card-index.json: %w", err)
	}
	return &idx, nil
}

// LookupCard finds a card entry by its file path.
func (idx *CardIndex) LookupCard(absPath string) *CardEntry {
	for i := range idx.Cards {
		if idx.Cards[i].FilePath == absPath {
			return &idx.Cards[i]
		}
	}
	return nil
}

// QualityCards returns cards with quality >= minQuality, sorted by quality descending.
func (idx *CardIndex) QualityCards(minQuality float64) []CardEntry {
	var result []CardEntry
	for _, c := range idx.Cards {
		if c.Quality >= minQuality {
			result = append(result, c)
		}
	}
	// Sort by quality descending.
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Quality > result[i].Quality {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// Concepts returns all unique concepts from the card index.
func (idx *CardIndex) Concepts() []string {
	seen := make(map[string]bool)
	var concepts []string
	for _, c := range idx.Cards {
		if c.Concept != "" && !seen[c.Concept] {
			seen[c.Concept] = true
			concepts = append(concepts, c.Concept)
		}
	}
	return concepts
}
