package agentconfig

// Default endpoints and model names shared across entry points.
// These can be overridden via environment variables at runtime.
const (
	// DefaultOMLXBaseURL is the default OpenAI-compatible endpoint for
	// local MLX embedding / reranking services.
	DefaultOMLXBaseURL = "http://127.0.0.1:8000/v1"

	// DefaultEmbedModel is the default embedding model when OMLX_EMBED_MODEL
	// is not set.
	DefaultEmbedModel = "bge-m3-mlx-8bit"

	// DefaultRerankModel is the default reranker model when OMLX_RERANK_MODEL
	// is not set.
	DefaultRerankModel = "Qwen3-Reranker-4B-4bit-MLX"

	// DefaultPlanModel is the default high-quality planning model used by
	// /plan mode in the TUI.
	DefaultPlanModel = "deepseek-v4-pro"
)
