package writing

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// QualityReport holds the evaluation results for an agent output.
// The auto scores are suggestions — user ratings are the ground truth.
type QualityReport struct {
	OutputID    string          `json:"output_id"`
	AutoScore   float64         `json:"auto_score"`  // 0-100, automatic evaluation
	UserRating  Rating          `json:"user_rating"` // user feedback (zero if pending)
	UserComment string          `json:"user_comment,omitempty"`
	Dimensions  DimensionScores `json:"dimensions"`
	Timestamp   string          `json:"timestamp"`
	AppliedIDs  []string        `json:"applied_pattern_ids,omitempty"`
}

// DimensionScores holds the 4 evaluation dimensions.
type DimensionScores struct {
	Structure   float64 `json:"structure"`   // 结构完整性 (0-100)
	Citation    float64 `json:"citation"`    // 引用精确度 (0-100)
	Argument    float64 `json:"argument"`    // 论证力度 (0-100)
	Terminology float64 `json:"terminology"` // 专业术语 (0-100)
}

// Rating is the user's evaluation level.
type Rating string

const (
	RatingExcellent        Rating = "excellent"
	RatingGood             Rating = "good"
	RatingAcceptable       Rating = "acceptable"
	RatingNeedsImprovement Rating = "needs_improvement"
	RatingPoor             Rating = "poor"
)

// FeedbackStore collects and persists user feedback on agent outputs.
type FeedbackStore struct {
	mu        sync.RWMutex
	feedbacks map[string]*QualityReport // outputID → report
}

// NewFeedbackStore creates a feedback store.
func NewFeedbackStore() *FeedbackStore {
	return &FeedbackStore{
		feedbacks: make(map[string]*QualityReport),
	}
}

// QualityEvaluator evaluates agent writing output against writing patterns.
//
// Evaluation architecture:
//
//	Auto scoring → Suggestion (not authoritative)
//	       ↓
//	User rating → Ground truth (overrides auto score)
//	       ↓
//	Auto vs user delta > threshold → Calibration report
//	       ↓
//	User rating = excellent → Extract new pattern suggestion
type QualityEvaluator struct {
	patterns      *PatternStore
	feedbackStore *FeedbackStore
}

// NewQualityEvaluator creates an evaluator.
func NewQualityEvaluator(patterns *PatternStore, feedbackStore *FeedbackStore) *QualityEvaluator {
	return &QualityEvaluator{
		patterns:      patterns,
		feedbackStore: feedbackStore,
	}
}

// Evaluate performs a 4-dimension automatic evaluation of an agent's output.
// Returns a QualityReport with suggested scores (not authoritative).
func (e *QualityEvaluator) Evaluate(outputID, output string, appliedPatternIDs []string) *QualityReport {
	// 深拷贝 AppliedIDs 以避免调用方后续修改原 slice 污染已存储的报告。
	idsCopy := make([]string, len(appliedPatternIDs))
	copy(idsCopy, appliedPatternIDs)
	r := &QualityReport{
		OutputID:   outputID,
		Timestamp:  time.Now().Format(time.RFC3339),
		AppliedIDs: idsCopy,
	}
	r.Dimensions.Structure = evalStructure(output)
	r.Dimensions.Citation = evalCitation(output)
	r.Dimensions.Argument = evalArgument(output)
	r.Dimensions.Terminology = evalTerminology(output)
	r.AutoScore = computeOverall(r.Dimensions)

	// Associate with feedback store.
	if e.feedbackStore != nil {
		e.feedbackStore.mu.Lock()
		e.feedbackStore.feedbacks[outputID] = r
		e.feedbackStore.mu.Unlock()
	}
	return r
}

// CollectFeedback records user feedback on a previously evaluated output.
func (e *QualityEvaluator) CollectFeedback(outputID string, rating Rating, comment string) error {
	if e.feedbackStore == nil {
		return fmt.Errorf("feedback store is nil")
	}
	e.feedbackStore.mu.Lock()
	defer e.feedbackStore.mu.Unlock()

	r, ok := e.feedbackStore.feedbacks[outputID]
	if !ok {
		return fmt.Errorf("output %s not found in feedback store", outputID)
	}
	r.UserRating = rating
	r.UserComment = comment
	return nil
}

// FeedbackStats returns summary statistics of collected feedback.
type FeedbackStats struct {
	Total        int
	ByRating     map[Rating]int
	AvgAutoScore float64
}

// Stats returns feedback statistics.
func (e *QualityEvaluator) Stats() *FeedbackStats {
	if e.feedbackStore == nil {
		return &FeedbackStats{}
	}
	e.feedbackStore.mu.RLock()
	defer e.feedbackStore.mu.RUnlock()

	stats := &FeedbackStats{
		Total:    len(e.feedbackStore.feedbacks),
		ByRating: make(map[Rating]int),
	}
	totalScore := 0.0
	count := 0
	for _, r := range e.feedbackStore.feedbacks {
		if r.UserRating != "" {
			stats.ByRating[r.UserRating]++
		}
		totalScore += r.AutoScore
		count++
	}
	if count > 0 {
		stats.AvgAutoScore = totalScore / float64(count)
	}
	return stats
}

// CalibrationDiff computes the average difference between auto scores and user ratings.
// A large gap indicates the evaluator needs calibration.
func (e *QualityEvaluator) CalibrationDiff() float64 {
	if e.feedbackStore == nil {
		return 0
	}
	e.feedbackStore.mu.RLock()
	defer e.feedbackStore.mu.RUnlock()

	total := 0
	diffs := 0.0
	for _, r := range e.feedbackStore.feedbacks {
		if r.UserRating == "" {
			continue
		}
		userScore := ratingToScore(r.UserRating)
		diff := r.AutoScore - userScore
		if diff < 0 {
			diff = -diff
		}
		diffs += diff
		total++
	}
	if total == 0 {
		return 0
	}
	return diffs / float64(total)
}

// ---------- 4-dimension evaluation heuristics ----------

// evalStructure evaluates structural completeness.
// Checks for: heading hierarchy, section presence, paragraph structure.
func evalStructure(s string) float64 {
	score := 50.0
	lines := strings.Split(s, "\n")
	hasH1 := false
	hasH2 := false
	paraCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			hasH1 = true
		}
		if strings.HasPrefix(trimmed, "## ") {
			hasH2 = true
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			paraCount++
		}
	}
	if hasH1 {
		score += 15
	}
	if hasH2 {
		score += 15
	}
	if paraCount >= 3 {
		score += 10
	} else if paraCount >= 1 {
		score += 5
	}
	if score > 95 {
		score = 95
	}
	return score
}

// evalCitation evaluates citation accuracy.
// Checks for: law article references, case references, patent document numbers.
func evalCitation(s string) float64 {
	score := 50.0
	hasLawRef := strings.Contains(s, "法第") || strings.Contains(s, "条")
	hasCaseRef := strings.Contains(s, "号决定") || strings.Contains(s, "号案")
	hasPatentRef := strings.Contains(s, "CN") || strings.Contains(s, "US")

	if hasLawRef {
		score += 15
	}
	if hasCaseRef {
		score += 15
	}
	if hasPatentRef {
		score += 10
	}
	// Check for non-attributed claims (negative).
	if strings.Count(s, "根据") > 3 {
		score -= 5
	}
	if score > 95 {
		score = 95
	}
	if score < 20 {
		score = 20
	}
	return score
}

// evalArgument evaluates argument strength.
// Checks for: logical connectors, contrasting analysis, evidence references.
func evalArgument(s string) float64 {
	score := 40.0
	connectors := []string{"因此", "因为", "然而", "但是", "虽然", "如果", "则"}
	evidence := []string{"对比文件", "实施例", "附图", "实验", "测试", "数据"}

	for _, c := range connectors {
		if strings.Contains(s, c) {
			score += 5
		}
	}
	for _, e := range evidence {
		if strings.Contains(s, e) {
			score += 5
		}
	}
	// Penalize slop phrases.
	slopPhrases := []string{"进一步地", "此外", "值得一提的是", "显而易见地"}
	for _, sp := range slopPhrases {
		if strings.Contains(s, sp) {
			score -= 5
		}
	}
	if score > 95 {
		score = 95
	}
	if score < 20 {
		score = 20
	}
	return score
}

// evalTerminology evaluates terminology accuracy.
// Checks for: appropriate technical terms, consistency, no commercial language.
func evalTerminology(s string) float64 {
	score := 60.0
	techTerms := []string{"技术特征", "本领域", "权利要求", "技术方案", "实施例"}
	badPhrases := []string{"最好", "最佳", "最先进", "绝对", "一定"}

	for _, t := range techTerms {
		if strings.Contains(s, t) {
			score += 5
		}
	}
	for _, bp := range badPhrases {
		if strings.Contains(s, bp) {
			score -= 10
		}
	}
	if score > 95 {
		score = 95
	}
	if score < 20 {
		score = 20
	}
	return score
}

func computeOverall(d DimensionScores) float64 {
	return (d.Structure + d.Citation + d.Argument + d.Terminology) / 4.0
}

func ratingToScore(r Rating) float64 {
	switch r {
	case RatingExcellent:
		return 90
	case RatingGood:
		return 75
	case RatingAcceptable:
		return 60
	case RatingNeedsImprovement:
		return 40
	case RatingPoor:
		return 20
	default:
		return 50
	}
}
