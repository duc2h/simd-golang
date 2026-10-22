// Package simdnorm normalizes float32 vectors to unit length (||v|| = 1),
// using SIMD instructions where the platform supports them.
//
// Normalizing is two data-parallel passes over the vector:
//
//  1. reduction: ss = Σ v[i]²          (sumSquares)
//  2. map:       v[i] = v[i] * (1/√ss)  (scale)
//
// Both passes are a good fit for SIMD. Each architecture provides its own
// sumSquares/scale (see normalize_arm64.go, normalize_amd64_simd.go,
// normalize_other.go); this file holds the shared logic and the pure-Go
// reference implementation.
package simdnorm

import "math"

// Normalize scales v in place so that its Euclidean length is 1 and returns
// the length it had before. A zero vector is left unchanged and 0 is returned.
func Normalize(v []float32) float32 {
	return normalizeWith(v, sumSquares, scale)
}

// NormalizeScalar is the same as Normalize but always uses the pure-Go
// (non-SIMD) code path. It is the reference used by tests and benchmarks.
func NormalizeScalar(v []float32) float32 {
	return normalizeWith(v, sumSquaresScalar, scaleScalar)
}

func normalizeWith(v []float32, sumSq func([]float32) float32, scl func([]float32, float32)) float32 {
	ss := sumSq(v)
	if ss == 0 {
		return 0
	}
	norm := float32(math.Sqrt(float64(ss)))
	scl(v, 1/norm)
	return norm
}

// sumSquaresScalar is a plain sequential loop. The Go compiler does NOT
// auto-vectorize loops, so this compiles to one scalar multiply-add per element.
//
// The scalar functions are kept out of line so they exist as their own
// symbols in the binary and can be inspected with `go tool objdump`
// (see TestProofDisassembly).
//
//go:noinline
func sumSquaresScalar(v []float32) float32 {
	var s float32
	for _, x := range v {
		s += x * x
	}
	return s
}

//go:noinline
func scaleScalar(v []float32, s float32) {
	for i := range v {
		v[i] *= s
	}
}
