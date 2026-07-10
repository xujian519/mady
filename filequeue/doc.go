// Package filequeue provides a mutex-protected, file-backed append-only queue.
//
// Each queue is backed by a single file on disk. Entries are separated by
// newlines. The queue supports Push (append), Pop (shift), Peek, and Len,
// all under a sync.Mutex. Useful for simple, durable task queues where
// at-most-once delivery is acceptable.
package filequeue
