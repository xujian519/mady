package vecbytes

import (
	"math"
	"testing"
)

func TestFloatsToBytes_RoundTrip(t *testing.T) {
	tests := [][]float32{
		{0.1, 0.2, 0.3, 0.4, 0.5},
		{1.0, -1.0, 2.5, -2.5},
		{0},
		{math.MaxFloat32, math.SmallestNonzeroFloat32},
	}
	for _, original := range tests {
		encoded := FloatsToBytes(original)
		if len(original) > 0 && len(encoded) != len(original)*4 {
			t.Fatalf("encoded length = %d, want %d", len(encoded), len(original)*4)
		}
		decoded := BytesToFloats(encoded)
		if len(decoded) != len(original) {
			t.Fatalf("decoded length = %d, want %d", len(decoded), len(original))
		}
		for i, v := range original {
			if decoded[i] != v {
				t.Fatalf("decoded[%d] = %v, want %v", i, decoded[i], v)
			}
		}
	}
}

func TestFloatsToBytes_Empty(t *testing.T) {
	if result := FloatsToBytes(nil); result != nil {
		t.Fatalf("FloatsToBytes(nil) = %v, want nil", result)
	}
	if result := FloatsToBytes([]float32{}); result != nil {
		t.Fatalf("FloatsToBytes(empty) = %v, want nil", result)
	}
}

func TestBytesToFloats_InvalidLength(t *testing.T) {
	if result := BytesToFloats(nil); result != nil {
		t.Fatalf("BytesToFloats(nil) = %v, want nil", result)
	}
	if result := BytesToFloats([]byte{1, 2, 3}); result != nil {
		t.Fatalf("BytesToFloats(3 bytes) = %v, want nil", result)
	}
	if result := BytesToFloats([]byte{}); result != nil {
		t.Fatalf("BytesToFloats(empty) = %v, want nil", result)
	}
}
