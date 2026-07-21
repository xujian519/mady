package internal

import (
	"math"
	"testing"
)

func TestFloatsToBytes_RoundTrip(t *testing.T) {
	original := []float32{1.5, -2.0, 3.14159, 0, 1e-10}
	encoded := FloatsToBytes(original)
	decoded := BytesToFloats(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("length: got %d, want %d", len(decoded), len(original))
	}
	for i := range original {
		if math.Abs(float64(decoded[i]-original[i])) > 1e-6 {
			t.Errorf("[%d]: got %f, want %f", i, decoded[i], original[i])
		}
	}
}

func TestFloatsToBytes_Empty(t *testing.T) {
	if got := FloatsToBytes(nil); got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
	if got := FloatsToBytes([]float32{}); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestBytesToFloats_InvalidLength(t *testing.T) {
	if got := BytesToFloats([]byte{1, 2, 3}); got != nil {
		t.Errorf("3 bytes should return nil, got %v", got)
	}
	if got := BytesToFloats(nil); got != nil {
		t.Errorf("nil should return nil, got %v", got)
	}
	if got := BytesToFloats([]byte{}); got != nil {
		t.Errorf("empty should return nil, got %v", got)
	}
}

func TestL2Norm(t *testing.T) {
	tests := []struct {
		vec  []float32
		want float64
	}{
		{[]float32{3, 4}, 5.0},
		{[]float32{1, 0, 0}, 1.0},
		{[]float32{0, 0, 0}, 0.0},
	}

	for _, tt := range tests {
		got := L2Norm(tt.vec)
		if math.Abs(got-tt.want) > 1e-6 {
			t.Errorf("L2Norm(%v) = %f, want %f", tt.vec, got, tt.want)
		}
	}
}

func TestTopKByScore(t *testing.T) {
	items := []string{"a", "bb", "ccc", "dddd", "eeeee"}
	score := func(s string) float64 { return float64(len(s)) }

	t.Run("Top3", func(t *testing.T) {
		got := TopKByScore(items, score, 3)
		if len(got) != 3 {
			t.Fatalf("got %d items, want 3", len(got))
		}
		if got[0] != "eeeee" || got[1] != "dddd" || got[2] != "ccc" {
			t.Errorf("top 3 = %v, want [eeeee dddd ccc]", got)
		}
	})

	t.Run("AllItems", func(t *testing.T) {
		got := TopKByScore(items, score, 10)
		if len(got) != len(items) {
			t.Errorf("got %d items, want %d", len(got), len(items))
		}
	})

	t.Run("Zero", func(t *testing.T) {
		if got := TopKByScore(items, score, 0); got != nil {
			t.Errorf("topK=0 should return nil, got %v", got)
		}
	})

	t.Run("Empty", func(t *testing.T) {
		if got := TopKByScore([]string{}, score, 3); got != nil {
			t.Errorf("empty input should return nil, got %v", got)
		}
	})
}

func TestRRFScore(t *testing.T) {
	tests := []struct {
		rank int
		k    float64
		want float64
	}{
		{0, 60, 1.0 / 60.0},
		{1, 60, 1.0 / 61.0},
		{60, 60, 1.0 / 120.0},
		{0, 1, 1.0},
	}

	for _, tt := range tests {
		got := RRFScore(tt.rank, tt.k)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("RRFScore(%d, %f) = %f, want %f", tt.rank, tt.k, got, tt.want)
		}
	}
}

func TestRRFScore_NegativeRank(t *testing.T) {
	if got := RRFScore(-1, 60); got != 0 {
		t.Errorf("negative rank should return 0, got %f", got)
	}
}
