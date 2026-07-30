package intent

import (
	"context"
	"sync"

	"github.com/xujian519/mady/retrieval"
)

// SemanticClassifier uses vector similarity for intent matching.
// It augments (rather than replaces) keyword/LLM classification by
// finding semantically similar historical inputs and their known intents.
//
// When an Embedder is available, it encodes the input and searches a
// vector store of known intent examples. The closest match (cosine ≥0.85)
// with a known intent label is used as the result.
type SemanticClassifier struct {
	embedder Embedder
	examples []IntentExample
	mu       sync.RWMutex
}

// Embedder is the interface for text-to-vector encoding.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// IntentExample pairs an input text with its known intent.
type IntentExample struct {
	Text   string
	Intent IntentResult
}

// NewSemanticClassifier creates a SemanticClassifier.
func NewSemanticClassifier(embedder Embedder) *SemanticClassifier {
	return &SemanticClassifier{
		embedder: embedder,
	}
}

// Name returns the classifier identifier.
func (s *SemanticClassifier) Name() string { return "semantic" }

// AddExample registers a known intent example for future matching.
func (s *SemanticClassifier) AddExample(text string, intent IntentResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.examples = append(s.examples, IntentExample{Text: text, Intent: intent})
}

// Classify implements Classifier using semantic similarity.
// Returns zero-confidence result if no embedder is available or no match found.
func (s *SemanticClassifier) Classify(input string) IntentResult {
	if s.embedder == nil {
		return IntentResult{Confidence: 0}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.examples) == 0 {
		return IntentResult{Confidence: 0}
	}

	// Encode input
	inputVec, err := s.embedder.Embed(context.Background(), input)
	if err != nil {
		return IntentResult{Confidence: 0}
	}

	// Find best match by cosine similarity
	var bestMatch *IntentExample
	bestSim := 0.0
	for i := range s.examples {
		ex := &s.examples[i]
		exVec, err := s.embedder.Embed(context.Background(), ex.Text)
		if err != nil {
			continue
		}
		sim := retrieval.CosineSimilarity(inputVec, exVec)
		if sim > bestSim {
			bestSim = sim
			bestMatch = ex
		}
	}

	// Require high similarity threshold
	const similarityThreshold = 0.85
	if bestMatch == nil || bestSim < similarityThreshold {
		return IntentResult{Confidence: 0}
	}

	result := bestMatch.Intent
	result.Confidence = bestSim
	result.Sources = []string{"semantic"}
	return result
}
