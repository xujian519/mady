package tools

import (
	"crypto/rand"
	"math/big"
)

// cryptoIntn returns a uniform random integer in [0, n) using crypto/rand.
// If crypto/rand fails, it falls back to 0.
func cryptoIntn(n int) int {
	if n <= 0 {
		return 0
	}
	ret, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		// 系统熵源不可用→回退 0（仅用于抖动/抽样，不承载安全语义）。
		return 0
	}
	return int(ret.Int64())
}
