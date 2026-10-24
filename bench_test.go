package simdnorm

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// Sizes: 3D point, tiny, small, typical ML embeddings (384, 1536),
// L2-cache sized, and larger than cache (memory-bandwidth bound).
var benchSizes = []int{3, 16, 128, 384, 1536, 65536, 1 << 22}

// Normalize and sumSquares use whichever SIMD version was compiled in for this
// GOARCH (NEON on arm64, AVX2 on amd64 with GOEXPERIMENT=simd). The benchmark
// name shows which one, e.g. "simd-neon" or "simd-avx2".

func benchVector(n int) []float32 {
	return randomVector(rand.New(rand.NewPCG(11, 12)), n)
}

func BenchmarkNormalize(b *testing.B) {
	impls := []struct {
		name string
		fn   func([]float32) float32
	}{
		{"simd-" + simdName(), Normalize},
		{"scalar", NormalizeScalar},
	}
	for _, n := range benchSizes {
		for _, impl := range impls {
			b.Run(fmt.Sprintf("n=%d/%s", n, impl.name), func(b *testing.B) {
				v := benchVector(n)
				b.SetBytes(int64(n) * 4)
				for b.Loop() {
					impl.fn(v)
				}
			})
		}
	}
}

// sumSquaresUnrolled is pure Go with 4 independent accumulators. It shows how
// much of the speedup comes from breaking the dependency chain between adds
// (instruction-level parallelism) and how much from SIMD lanes.
func sumSquaresUnrolled(v []float32) float32 {
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(v); i += 4 {
		s0 += v[i] * v[i]
		s1 += v[i+1] * v[i+1]
		s2 += v[i+2] * v[i+2]
		s3 += v[i+3] * v[i+3]
	}
	for ; i < len(v); i++ {
		s0 += v[i] * v[i]
	}
	return (s0 + s1) + (s2 + s3)
}

var sink float32

func BenchmarkSumSquares(b *testing.B) {
	impls := []struct {
		name string
		fn   func([]float32) float32
	}{
		{"simd-" + simdName(), sumSquares},
		{"scalar", sumSquaresScalar},
		{"scalar-4acc", sumSquaresUnrolled},
	}
	for _, n := range []int{1536, 65536} {
		for _, impl := range impls {
			b.Run(fmt.Sprintf("n=%d/%s", n, impl.name), func(b *testing.B) {
				v := benchVector(n)
				b.SetBytes(int64(n) * 4)
				for b.Loop() {
					sink = impl.fn(v)
				}
			})
		}
	}
}
