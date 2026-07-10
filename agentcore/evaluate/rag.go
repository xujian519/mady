package evaluate

import (
	"math"
)

// RetrievedDoc represents one result from a retrieval pipeline with its
// relevance score (higher = more relevant, as produced by Searcher.Search).
type RetrievedDoc struct {
	ID    string
	Score float64
}

// RAGEvaluation holds standard information-retrieval metrics for a single
// retrieval query against a set of known-relevant document IDs.
type RAGEvaluation struct {
	Query       string
	K           int
	PrecisionAtK float64
	RecallAtK    float64
	MRR          float64 // Mean Reciprocal Rank
	NDCG         float64 // Normalized Discounted Cumulative Gain
	HitAtK       bool    // whether any relevant doc appears in top-K
}

// EvaluateRAG computes IR metrics for a single retrieval result set.
//
//   - Precision@K: fraction of top-K results that are relevant.
//   - Recall@K: fraction of relevant docs found in top-K.
//   - MRR: reciprocal of the rank of the first relevant doc (0 if none).
//   - NDCG: normalized DCG using binary relevance, discounted by log2(rank).
func EvaluateRAG(retrieved []RetrievedDoc, relevantIDs []string, k int) RAGEvaluation {
	if k <= 0 {
		k = len(retrieved)
	}
	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	topK := retrieved
	if len(topK) > k {
		topK = topK[:k]
	}

	var hits int
	var firstRelRank int // 1-based, 0 means not found
	for i, doc := range topK {
		if relevant[doc.ID] {
			hits++
			if firstRelRank == 0 {
				firstRelRank = i + 1
			}
		}
	}

	ev := RAGEvaluation{
		K:            k,
		PrecisionAtK: safeDiv(hits, len(topK)),
		HitAtK:       hits > 0,
	}

	if len(relevantIDs) > 0 {
		ev.RecallAtK = float64(hits) / float64(len(relevantIDs))
	}

	if firstRelRank > 0 {
		ev.MRR = 1.0 / float64(firstRelRank)
	}

	ev.NDCG = ndcg(topK, relevant)

	return ev
}

// RAGBatchResult aggregates RAG metrics across multiple queries.
type RAGBatchResult struct {
	Queries        int
	MeanPrecision  float64
	MeanRecall     float64
	MeanMRR        float64
	MeanNDCG       float64
	HitRate        float64 // fraction of queries with at least one hit in top-K
	PerQuery       []RAGEvaluation
}

// EvaluateRAGBatch evaluates retrieval quality across multiple queries.
// Each entry in retrievedSet is paired with the corresponding relevantIDsSet
// entry.
func EvaluateRAGBatch(retrievedSet [][]RetrievedDoc, relevantIDsSet [][]string, k int) RAGBatchResult {
	n := len(retrievedSet)
	if n == 0 {
		return RAGBatchResult{}
	}
	var sumP, sumR, sumMRR, sumNDCG float64
	var hits int
	perQuery := make([]RAGEvaluation, 0, n)
	for i := 0; i < n; i++ {
		var rel []string
		if i < len(relevantIDsSet) {
			rel = relevantIDsSet[i]
		}
		ev := EvaluateRAG(retrievedSet[i], rel, k)
		perQuery = append(perQuery, ev)
		sumP += ev.PrecisionAtK
		sumR += ev.RecallAtK
		sumMRR += ev.MRR
		sumNDCG += ev.NDCG
		if ev.HitAtK {
			hits++
		}
	}
	return RAGBatchResult{
		Queries:       n,
		MeanPrecision: sumP / float64(n),
		MeanRecall:    sumR / float64(n),
		MeanMRR:       sumMRR / float64(n),
		MeanNDCG:      sumNDCG / float64(n),
		HitRate:       float64(hits) / float64(n),
		PerQuery:      perQuery,
	}
}

// ndcg computes Normalized Discounted Cumulative Gain with binary relevance.
// DCG  = Σ rel_i / log2(i+1)  for i = 1..K
// IDCG = DCG of the ideal ranking (all relevant docs first)
// NDCG = DCG / IDCG
func ndcg(topK []RetrievedDoc, relevant map[string]bool) float64 {
	if len(topK) == 0 || len(relevant) == 0 {
		return 0
	}

	// Actual DCG.
	var dcg float64
	for i, doc := range topK {
		if relevant[doc.ID] {
			dcg += 1.0 / math.Log2(float64(i+2)) // i+2 because rank is 1-based
		}
	}

	// Ideal DCG: place min(len(relevant), len(topK)) relevant docs at the top.
	nRel := len(relevant)
	if nRel > len(topK) {
		nRel = len(topK)
	}
	var idcg float64
	for i := 0; i < nRel; i++ {
		idcg += 1.0 / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func safeDiv(num int, den int) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}
