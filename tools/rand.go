package tools

import (
	"crypto/rand"
	"math/big"
)

// cryptoIntn returns a uniform random integer in [0, max) using crypto/rand.
// If crypto/rand fails, it falls back to 0.
func cryptoIntn(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
