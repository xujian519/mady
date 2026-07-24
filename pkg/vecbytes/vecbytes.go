// Package vecbytes provides float32 vector ↔ little-endian byte BLOB encoding
// utilities shared by knowledge, memory, and retrieval subsystems.
//
// The encoding format is a contiguous sequence of little-endian uint32 values,
// each representing one float32 (IEEE 754). This format is used for SQLite
// BLOB storage of embedding vectors across the codebase.
package vecbytes

import (
	"encoding/binary"
	"math"
)

// FloatsToBytes encodes a float32 slice into a little-endian BLOB.
// Returns nil for an empty or nil input.
func FloatsToBytes(vec []float32) []byte {
	if len(vec) == 0 {
		return nil
	}
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// BytesToFloats decodes a little-endian BLOB into a float32 slice.
// Returns nil if the input length is zero or not a multiple of 4.
func BytesToFloats(b []byte) []float32 {
	if len(b)%4 != 0 || len(b) == 0 {
		return nil
	}
	vec := make([]float32, len(b)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return vec
}
