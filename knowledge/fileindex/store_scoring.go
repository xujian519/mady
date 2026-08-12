package fileindex

import (
	"path/filepath"
	"strings"
	"time"
)

// scoreFileName computes a simple BM25-like score for filename matching.
func scoreFileName(query, filename string) float64 {
	// Exact filename match (ignoring extension): high score.
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base == query {
		return 1.0
	}
	// Filename contains query as word.
	if strings.Contains(base, query) {
		return 0.8
	}
	// Query contains in filename.
	if strings.Contains(query, base) {
		return 0.6
	}
	// Partial character overlap.
	intersect := charOverlap(query, base)
	if intersect > 0.3 {
		return intersect * 0.5
	}
	return 0
}

// scorePathSegments scores path segments (directory names) that match the query.
func scorePathSegments(query, path string) float64 {
	normalized := strings.ReplaceAll(path, "\\", "/")
	segments := strings.Split(normalized, "/")
	var score float64
	for _, seg := range segments {
		lower := strings.ToLower(seg)
		if lower == query {
			score += 0.5
		} else if strings.Contains(lower, query) {
			score += 0.3
		}
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// scoreRecency returns a recency score in [0, 1] with a 30-day half-life.
func scoreRecency(now, modTime time.Time) float64 {
	days := now.Sub(modTime).Hours() / 24
	if days <= 0 {
		return 1.0
	}
	// Decay: 1.0 at day 0, ~0.5 at day 30, ~0.25 at day 60.
	return 1.0 / (1.0 + days/30.0)
}

// charOverlap returns the fraction of query characters present in the target.
func charOverlap(query, target string) float64 {
	qChars := make(map[rune]bool)
	for _, c := range query {
		qChars[c] = true
	}
	tChars := make(map[rune]bool)
	for _, c := range target {
		tChars[c] = true
	}
	var matched int
	for c := range qChars {
		if tChars[c] {
			matched++
		}
	}
	if len(qChars) == 0 {
		return 0
	}
	return float64(matched) / float64(len(qChars))
}
