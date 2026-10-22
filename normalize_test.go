package simdnorm

import (
	"math"
	"math/rand/v2"
	"testing"
)

func randomVector(r *rand.Rand, n int) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	return v
}

// Sizes cover: empty, tail only (< 16), exact blocks, blocks + tail, large.
var testSizes = []int{0, 1, 2, 3, 4, 7, 8, 15, 16, 17, 31, 32, 33, 63, 64, 65, 100, 384, 768, 1536, 4099}

func TestNormalizeKnownValue(t *testing.T) {
	v := []float32{3, 4}
	if norm := Normalize(v); norm != 5 {
		t.Fatalf("norm = %v, want 5", norm)
	}
	if v[0] != 0.6 || v[1] != 0.8 {
		t.Fatalf("v = %v, want [0.6 0.8]", v)
	}
}

func TestNormalizeZeroVector(t *testing.T) {
	v := make([]float32, 40)
	if norm := Normalize(v); norm != 0 {
		t.Fatalf("norm = %v, want 0", norm)
	}
	for i, x := range v {
		if x != 0 {
			t.Fatalf("v[%d] = %v, zero vector must stay unchanged", i, x)
		}
	}
}

func TestNormalizeUnitLength(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, n := range testSizes[1:] {
		v := randomVector(r, n)
		Normalize(v)
		var ss float64
		for _, x := range v {
			ss += float64(x) * float64(x)
		}
		if got := math.Sqrt(ss); math.Abs(got-1) > 1e-5 {
			t.Errorf("n=%d: |v| = %v after Normalize, want 1", n, got)
		}
	}
}

// The SIMD path adds numbers in a different order than the sequential loop,
// so the sum of squares may differ in the last bits; compare with a tolerance.
func TestSumSquaresMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for _, n := range testSizes {
		v := randomVector(r, n)
		got, want := sumSquares(v), sumSquaresScalar(v)
		if math.Abs(float64(got-want)) > 1e-5*math.Max(1, float64(want)) {
			t.Errorf("n=%d: sumSquares = %v, scalar = %v", n, got, want)
		}
	}
}

// Scaling has no ordering differences, so SIMD must be bit-identical,
// including the sign of zero.
func TestScaleBitIdenticalToScalar(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for _, n := range testSizes {
		v := randomVector(r, n)
		if n > 1 {
			v[0], v[1] = float32(math.Copysign(0, -1)), 0 // -0 and +0
		}
		got := append([]float32(nil), v...)
		want := append([]float32(nil), v...)
		scale(got, 0.37)
		scaleScalar(want, 0.37)
		for i := range got {
			if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
				t.Fatalf("n=%d i=%d: scale = %v, scalar = %v", n, i, got[i], want[i])
			}
		}
	}
}

// The assembly must only touch the elements it was given.
func TestDoesNotWriteOutsideSlice(t *testing.T) {
	const guard = 123.0
	buf := make([]float32, 64+2)
	for i := range buf {
		buf[i] = guard
	}
	inner := buf[1 : len(buf)-1] // 64 elements, sentinel on each side
	scale(inner, 2)
	if buf[0] != guard || buf[len(buf)-1] != guard {
		t.Fatalf("scale wrote outside its slice: %v ... %v", buf[0], buf[len(buf)-1])
	}
}

func TestNormalizeMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	for _, n := range testSizes {
		a := randomVector(r, n)
		b := append([]float32(nil), a...)
		na, nb := Normalize(a), NormalizeScalar(b)
		if math.Abs(float64(na-nb)) > 1e-5*math.Max(1, float64(nb)) {
			t.Errorf("n=%d: norm SIMD = %v, scalar = %v", n, na, nb)
		}
		for i := range a {
			if math.Abs(float64(a[i]-b[i])) > 1e-6 {
				t.Fatalf("n=%d i=%d: SIMD = %v, scalar = %v", n, i, a[i], b[i])
			}
		}
	}
}
