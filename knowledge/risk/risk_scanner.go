package risk

import (
	"context"
	"fmt"
	"strings"
)

// CaseSearcher abstracts case retrieval for the risk scanner.
// Implementations can use knowledge.Store, external APIs, or mock data.
type CaseSearcher interface {
	// SearchCases searches for case documents matching the given feature tags.
	// Returns a list of result entries with relevance and metadata.
	SearchCases(ctx context.Context, features []string, maxResults int) ([]CaseResult, error)
}

// CaseResult is one matching case from a risk scan query.
type CaseResult struct {
	DocID    string            `json:"doc_id"`
	Title    string            `json:"title"`
	DocType  string            `json:"doc_type"`
	Score    float64           `json:"score"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ScannerConfig configures the risk scanner.
type ScannerConfig struct {
	// MinCaseCount is the minimum number of historical cases required
	// to issue a non-low severity signal. Default: 3.
	MinCaseCount int
	// HighRiskThreshold: invalidation rate above this is high severity. Default: 0.5.
	HighRiskThreshold float64
	// MedRiskThreshold: invalidation rate above this is medium severity. Default: 0.2.
	MedRiskThreshold float64
	// MaxFeaturesPerScan limits how many feature tags are analyzed at once. Default: 5.
	MaxFeaturesPerScan int
}

// DefaultScannerConfig returns sensible defaults.
func DefaultScannerConfig() ScannerConfig {
	return ScannerConfig{
		MinCaseCount:       3,
		HighRiskThreshold:  0.5,
		MedRiskThreshold:   0.2,
		MaxFeaturesPerScan: 5,
	}
}

// Scanner is the active risk scanning engine.
type Scanner struct {
	searcher CaseSearcher
	config   ScannerConfig
}

// NewScanner creates a risk scanner with the given searcher and config.
func NewScanner(searcher CaseSearcher, config ScannerConfig) *Scanner {
	if config.MinCaseCount <= 0 {
		cfg := DefaultScannerConfig()
		config.MinCaseCount = cfg.MinCaseCount
		config.HighRiskThreshold = cfg.HighRiskThreshold
		config.MedRiskThreshold = cfg.MedRiskThreshold
		config.MaxFeaturesPerScan = cfg.MaxFeaturesPerScan
	}
	return &Scanner{searcher: searcher, config: config}
}

// ScanByFeatures analyzes a set of technical feature tags for invalidation risk.
//
// It searches historical case documents matching the feature combination,
// calculates the invalidation rate, and returns risk signals with severity.
func (s *Scanner) ScanByFeatures(ctx context.Context, features []string) (*ScanResult, error) {
	if len(features) == 0 {
		return &ScanResult{}, nil
	}
	// Limit features.
	if len(features) > s.config.MaxFeaturesPerScan {
		features = features[:s.config.MaxFeaturesPerScan]
	}
	results, err := s.searcher.SearchCases(ctx, features, 100)
	if err != nil {
		return nil, fmt.Errorf("search cases: %w", err)
	}
	result := &ScanResult{
		TotalCases: len(results),
	}
	if len(results) == 0 {
		return result, nil
	}
	// Calculate confidence based on result count.
	result.Confidence = calculateConfidence(len(results))

	// Generate risk signals per feature combination.
	signals := s.analyzeFeatureCombinations(features, results)
	result.Signals = signals
	return result, nil
}

// analyzeFeatureCombinations groups results by feature tags and computes risk metrics.
func (s *Scanner) analyzeFeatureCombinations(features []string, results []CaseResult) []RiskSignal {
	var signals []RiskSignal

	// For each individual feature, count how many results reference it.
	for _, feature := range features {
		totalForFeature := 0
		invalidForFeature := 0
		var relatedCases []string

		for _, r := range results {
			// Check if this result is related to the feature.
			if isRelatedToFeature(r, feature) {
				totalForFeature++
				if r.DocType == "reexam" {
					invalidForFeature++
				}
				if id := extractDocRef(r); id != "" {
					relatedCases = append(relatedCases, id)
				}
			}
		}
		if totalForFeature < s.config.MinCaseCount {
			continue
		}

		// 无效率基于该特征实际匹配的文书计算，而非全局结果。
		invalidRate := float64(invalidForFeature) / float64(totalForFeature)
		if invalidRate > 1.0 {
			invalidRate = 1.0
		}

		severity := s.classifySeverity(totalForFeature, invalidRate)
		signal := RiskSignal{
			ID:       fmt.Sprintf("risk-feat-%d", len(signals)+1),
			Type:     RiskFeatureCombination,
			Severity: severity,
			Title:    fmt.Sprintf("特征「%s」的无效风险", feature),
			Description: fmt.Sprintf("包含「%s」特征的 %d 件历史文书中，无效宣告占比约 %.0f%%",
				feature, totalForFeature, invalidRate*100),
			CaseCount:    totalForFeature,
			InvalidRate:  invalidRate,
			FeatureTags:  []string{feature},
			RelatedCases: relatedCases,
		}
		if severity == SeverityHigh || severity == SeverityMedium {
			signal.Recommendation = fmt.Sprintf(
				"建议在权利要求中重新考虑「%s」特征的表述方式或增加限定条件，参考关联案例中的维持有效的写法。",
				feature)
		}
		signals = append(signals, signal)
	}

	// For feature pairs (combination risk), analyze the intersection.
	if len(features) >= 2 {
		for i := 0; i < len(features)-1; i++ {
			for j := i + 1; j < len(features); j++ {
				pairSignal := s.analyzeFeaturePair(features[i], features[j], results)
				if pairSignal != nil {
					signals = append(signals, *pairSignal)
				}
			}
		}
	}

	if len(signals) == 0 {
		return nil
	}
	return signals
}

// analyzeFeaturePair checks the risk of a specific feature pair combination.
func (s *Scanner) analyzeFeaturePair(f1, f2 string, results []CaseResult) *RiskSignal {
	pairCount := 0
	invalidPairCount := 0
	for _, r := range results {
		if isRelatedToFeature(r, f1) && isRelatedToFeature(r, f2) {
			pairCount++
			if r.DocType == "reexam" {
				invalidPairCount++
			}
		}
	}
	if pairCount < s.config.MinCaseCount {
		return nil
	}
	// 特征对无效率基于同时包含两个特征的文书计算。
	invalidRate := float64(invalidPairCount) / float64(pairCount)
	if invalidRate > 1.0 {
		invalidRate = 1.0
	}

	severity := s.classifySeverity(pairCount, invalidRate)
	return &RiskSignal{
		ID:       fmt.Sprintf("risk-pair-%s-%s", f1, f2),
		Type:     RiskFeatureCombination,
		Severity: severity,
		Title:    fmt.Sprintf("特征组合「%s + %s」的无效风险", f1, f2),
		Description: fmt.Sprintf("同时包含「%s」和「%s」特征的 %d 件文书中，无效占比较高。",
			f1, f2, pairCount),
		CaseCount:   pairCount,
		InvalidRate: invalidRate,
		FeatureTags: []string{f1, f2},
	}
}

// classifySeverity maps case count and rate to a severity level.
func (s *Scanner) classifySeverity(caseCount int, rate float64) Severity {
	if rate >= s.config.HighRiskThreshold && caseCount >= s.config.MinCaseCount {
		return SeverityHigh
	}
	if rate >= s.config.MedRiskThreshold {
		return SeverityMedium
	}
	return SeverityLow
}

// calculateConfidence maps result count to a confidence score (0.0 ~ 1.0).
func calculateConfidence(count int) float64 {
	if count >= 50 {
		return 0.9
	}
	if count >= 20 {
		return 0.75
	}
	if count >= 10 {
		return 0.6
	}
	if count >= 5 {
		return 0.4
	}
	return 0.25
}

// isRelatedToFeature checks if a result entry is related to a given feature tag.
func isRelatedToFeature(r CaseResult, feature string) bool {
	// Check metadata tags.
	if tags, ok := r.Metadata["tags"]; ok {
		if strings.Contains(tags, feature) {
			return true
		}
	}
	if keywords, ok := r.Metadata["keywords"]; ok {
		if strings.Contains(keywords, feature) {
			return true
		}
	}
	// Check title.
	if strings.Contains(r.Title, feature) {
		return true
	}
	// Fallback: check metadata law_refs for related features.
	if lawRefs, ok := r.Metadata["law_refs"]; ok {
		switch feature {
		case "功能性限定", "功能限定", "functional":
			if strings.Contains(lawRefs, "26条第4款") || strings.Contains(lawRefs, "26.4") {
				return true
			}
		case "参数限定", "参数特征", "parameter":
			if strings.Contains(lawRefs, "22条第2款") || strings.Contains(lawRefs, "22.2") {
				return true
			}
		}
	}
	return false
}

// extractDocRef extracts a document reference string for reporting.
func extractDocRef(r CaseResult) string {
	if cn, ok := r.Metadata["case_number"]; ok && cn != "" {
		return cn
	}
	return r.DocID
}
