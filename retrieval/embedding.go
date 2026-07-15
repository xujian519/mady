package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Embedder computes vector embeddings for text.
// Implementations may call remote APIs (OpenAI, local models) or use
// on-device inference.
type Embedder interface {
	// Embed returns a vector for each input text. The returned slice has
	// the same length as texts. Each inner slice has Dimensions() elements.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions is the number of dimensions in each embedding vector.
	Dimensions() int
}

// APIEmbedder calls an OpenAI-compatible embeddings endpoint.
// It reuses the same base URL and API key conventions as
// provider/chatcompat, making it compatible with DeepSeek, Zhipu,
// and any OpenAI-compatible embedding service.
type APIEmbedder struct {
	// BaseURL is the embeddings endpoint base URL, e.g.
	// "https://api.openai.com/v1". The /embeddings suffix is appended
	// automatically.
	BaseURL string

	// APIKey is the authentication key.
	APIKey string

	// Model is the embedding model name, e.g. "text-embedding-3-small".
	Model string

	// Dimensions is cached after the first successful call.
	dims     int
	dimsOnce sync.Once

	client *http.Client
}

// NewAPIEmbedder creates an APIEmbedder with sensible defaults.
// model typically is "text-embedding-3-small" (1536 dims) or
// "text-embedding-3-large" (3072 dims).
func NewAPIEmbedder(baseURL, apiKey, model string) *APIEmbedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &APIEmbedder{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 30 * time.Second,
		},
	}
}

// Embed implements Embedder by calling the remote embeddings API.
func (e *APIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := map[string]any{
		"model": e.Model,
		"input": texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embed: marshal request: %w", err)
	}

	url := e.BaseURL + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("embed: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("embed: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed: HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("embed: parse response: %w", err)
	}

	// Sort by index to maintain input order.
	vectors := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index >= 0 && d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}

	// Cache dimensions from first successful call.
	e.dimsOnce.Do(func() {
		if len(vectors) > 0 && len(vectors[0]) > 0 {
			e.dims = len(vectors[0])
		}
	})

	return vectors, nil
}

// Dimensions returns the embedding dimensionality.
func (e *APIEmbedder) Dimensions() int {
	if e.dims > 0 {
		return e.dims
	}
	// Return sensible defaults for known models.
	switch e.Model {
	case "text-embedding-3-large":
		return 3072
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-ada-002":
		return 1536
	default:
		// BGE-M3 models output 1024-dimensional vectors.
		if strings.Contains(strings.ToLower(e.Model), "bge-m3") {
			return 1024
		}
		return 1536
	}
}

// --- Cosine similarity utilities ---

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [-1, 1]. Both vectors must have the same length.
// Uses float32 arithmetic internally for speed; the final result is
// widened to float64 for scoring compatibility.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float64(dot) / (math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
}

// DotProduct computes the dot product of two equal-length vectors.
// Uses float32 arithmetic for consistency with CosineSimilarity.
func DotProduct(a, b []float32) float64 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return float64(sum)
}
