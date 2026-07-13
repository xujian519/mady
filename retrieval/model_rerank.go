package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// QueryReranker extends Reranker with query-aware reranking using
// cross-encoder models. While a plain Reranker re-orders results based
// on intrinsic signals (position, source, diversity), a QueryReranker
// uses the user's query to compute true query-document relevance.
//
// Implementations should also satisfy the base Reranker interface;
// Rerank (without a query) is typically a no-op for model-based rerankers
// since cross-encoder scoring requires the query.
type QueryReranker interface {
	Reranker
	// RerankWithQuery re-orders scored chunks using the query as
	// additional context (typically via a cross-encoder model).
	RerankWithQuery(ctx context.Context, query string, results []ScoredChunk) []ScoredChunk
}

// ModelReranker calls a remote cross-encoder reranking model via the
// Cohere-compatible /v1/rerank API to re-score search results against
// the user's query. This provides semantic relevance scoring that
// surpasses heuristic rerankers for domain-specific queries.
type ModelReranker struct {
	// BaseURL is the rerank service endpoint prefix
	// (e.g. "http://127.0.0.1:8000/v1"). The actual request goes to
	// BaseURL + "/rerank".
	BaseURL string
	// APIKey for authentication (sent as Bearer token). May be empty
	// for local services that don't require auth.
	APIKey string
	// Model is the reranker model name
	// (e.g. "Qwen3-Reranker-4B-4bit-MLX").
	Model string
	// TopN limits the number of returned results. If 0 or negative,
	// all input results are returned (re-sorted by relevance).
	TopN int
	// MaxDocuments caps the number of documents sent to the rerank API
	// to avoid excessive latency. If 0, defaults to 20. Results beyond
	// this cap are appended after the reranked set, preserving their
	// original order.
	MaxDocuments int
	// Client is the HTTP client. A default with 10s timeout is used if nil.
	Client *http.Client
}

// NewModelReranker creates a ModelReranker with a default HTTP client
// and MaxDocuments=20.
func NewModelReranker(baseURL, apiKey, model string) *ModelReranker {
	return &ModelReranker{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		Model:        model,
		MaxDocuments: 20,
		Client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// rerankRequest is the request body for the /v1/rerank API (Cohere-compatible).
type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// rerankResponse is the response from the /v1/rerank API.
type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Model string `json:"model"`
	ID    string `json:"id"`
}

// Rerank implements Reranker.Rerank. Without a query, ModelReranker
// is a no-op — cross-encoder reranking requires the query to compute
// meaningful relevance scores.
func (r *ModelReranker) Rerank(results []ScoredChunk) []ScoredChunk {
	return results
}

// RerankWithQuery implements QueryReranker.RerankWithQuery by calling
// the remote rerank model to re-score results against the query.
//
// On any error (network, API, decode), the original results are returned
// unchanged — reranking is a best-effort enhancement, not a critical path.
func (r *ModelReranker) RerankWithQuery(ctx context.Context, query string, results []ScoredChunk) []ScoredChunk {
	if len(results) == 0 || query == "" {
		return results
	}

	maxDocs := r.MaxDocuments
	if maxDocs <= 0 {
		maxDocs = 20
	}

	// Split results into the head (sent to reranker) and tail (appended as-is).
	headCount := len(results)
	if headCount > maxDocs {
		headCount = maxDocs
	}
	head := results[:headCount]
	tail := results[headCount:]

	docs := make([]string, len(head))
	for i, sc := range head {
		docs[i] = sc.Content
	}

	topN := r.TopN
	if topN <= 0 || topN > len(head) {
		topN = len(head)
	}

	reqBody := rerankRequest{
		Model:     r.Model,
		Query:     query,
		Documents: docs,
		TopN:      topN,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return results
	}

	url := r.BaseURL + "/rerank"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return results
	}
	req.Header.Set("Content-Type", "application/json")
	if r.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.APIKey)
	}

	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return results
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = body // consumed to allow connection reuse; error is non-fatal
		return results
	}

	var rr rerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return results
	}

	if len(rr.Results) == 0 {
		return results
	}

	// Build reranked results using relevance scores from the model.
	returned := make(map[int]bool, len(rr.Results))
	reranked := make([]ScoredChunk, 0, len(head)+len(tail))
	for _, res := range rr.Results {
		if res.Index < 0 || res.Index >= len(head) {
			continue
		}
		sc := head[res.Index]
		sc.Score = res.RelevanceScore
		reranked = append(reranked, sc)
		returned[res.Index] = true
	}

	// Sort reranked portion by relevance score descending.
	sort.Slice(reranked, func(i, j int) bool {
		return reranked[i].Score > reranked[j].Score
	})

	// Append head results not returned by the API (preserving original order).
	for i, sc := range head {
		if !returned[i] {
			reranked = append(reranked, sc)
		}
	}

	// Append tail results that were not sent to the reranker.
	if len(tail) > 0 {
		reranked = append(reranked, tail...)
	}

	return reranked
}

// String returns a human-readable description for logging.
func (r *ModelReranker) String() string {
	return fmt.Sprintf("ModelReranker(model=%s, baseURL=%s)", r.Model, r.BaseURL)
}
