// Package omlx provides process lifecycle management for the oMLX inference
// server. oMLX ("omlx", installed via Homebrew) serves embedding and
// cross-encoder reranking models on Apple Silicon via an OpenAI-compatible
// HTTP API at http://127.0.0.1:8000/v1.
//
// Usage:
//
//	mgr := omlx.NewManager(8000, os.Getenv("OMLX_API_KEY"))
//	if err := mgr.EnsureRunning(ctx); err != nil {
//	    // Built-in detection order:
//	    //   1. Check if localhost:8000 is already serving
//	    //   2. Check if omlx binary exists in PATH
//	    //   3. Start it as a child process
//	    //   4. Wait up to 30s for it to be ready
//	    slog.Warn("oMLX unavailable, vector search degraded", "err", err)
//	}
//	defer mgr.Stop()
//
// The default oMLX endpoint (http://127.0.0.1:8000/v1) and model names are
// configured in pkg/agentconfig/defaults.go. This package only manages the
// server process — model download and lifecycle are handled by oMLX itself.
//
// Architecture:
//
//	mady (Go) ──HTTP──→ omlx serve (Python child process)
//	                          └── BGE-M3 (embeddings, ~140MB MLX 8bit)
//	                          └── Qwen3-Reranker-4B (~2GB MLX 4bit)
package omlx
