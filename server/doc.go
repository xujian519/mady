// Package server provides an HTTP/SSE API server for Mady agents.
//
// Endpoints:
//
//	POST /api/chat         — stream or non-stream chat completion
//	POST /api/threads      — create an empty thread
//	GET  /api/threads      — list persisted threads
//	GET  /api/threads/{key}      — thread metadata & messages
//	POST /api/threads/{key}/branch    — branch from a node
//	DELETE /api/threads/{key}         — delete a thread
//
// Threads persist state across requests (via agentcore.Checkpoint + session.AgentStore).
// Request-level overrides (model, response_format, thinking) can be applied per-call.
//
// Usage:
//
//	srv := server.New(agentcore.Config{Provider: provider})
//	log.Fatal(srv.ListenAndServe(":8080"))
package server
