package main

// This file would be used for ReindexVectors integration test.
// Not compiled as part of the default build — invoke via `go run`.

/*
Usage (with a real embedding API):
  go run ./example/wiki-import/ -reindex -apikey=sk-xxx -model=text-embedding-3-small

Without API key, use mock mode for pipeline verification:
  go run ./example/wiki-import/ -reindex -mock
*/
