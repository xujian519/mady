package psychological

// clamp 将值限制在 [lo, hi] 范围内
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
