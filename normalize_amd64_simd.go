//go:build amd64 && goexperiment.simd

package simdnorm

// On amd64, Go 1.26+ can generate SIMD code itself through the experimental
// simd/archsimd package (build with GOEXPERIMENT=simd). Its methods are
// compiler intrinsics: each call becomes one AVX instruction, no assembly file.

import "simd/archsimd"

// Only AVX2 is required. Using Mul+Add instead of a fused MulAdd avoids also
// requiring FMA, which some environments (e.g. Rosetta 2) do not provide.
var useAVX2 = archsimd.X86.AVX2()

// avxBlock is the number of floats handled per sumSquares iteration:
// 4 registers x 8 lanes.
const avxBlock = 32

// Implementation describes the code path Normalize uses on this machine.
func Implementation() string {
	if useAVX2 {
		return "amd64 AVX2 (simd/archsimd intrinsics)"
	}
	return "scalar (pure Go; CPU lacks AVX2)"
}

// simdName is a short label for the SIMD code path, used in benchmark names.
func simdName() string {
	if useAVX2 {
		return "avx2"
	}
	return "scalar"
}

// accumulatorLanes is the number of independent partial sums sumSquares keeps.
func accumulatorLanes() int {
	if useAVX2 {
		return avxBlock
	}
	return 1
}

func sumSquares(v []float32) float32 {
	if !useAVX2 {
		return sumSquaresScalar(v)
	}
	n := len(v) &^ (avxBlock - 1)
	var s float32
	if n > 0 {
		s = sumSquaresAVX2(v[:n])
	}
	return s + sumSquaresScalar(v[n:])
}

func scale(v []float32, s float32) {
	if !useAVX2 {
		scaleScalar(v, s)
		return
	}
	n := len(v) &^ 7
	if n > 0 {
		scaleAVX2(v[:n], s)
	}
	scaleScalar(v[n:], s)
}

// sumSquaresAVX2 requires len(x) to be a multiple of avxBlock.
//
//go:noinline
func sumSquaresAVX2(x []float32) float32 {
	var a0, a1, a2, a3 archsimd.Float32x8
	// Checking len(x) >= avxBlock up front lets the compiler drop the bounds
	// checks on the four loads below.
	for len(x) >= avxBlock {
		v0 := archsimd.LoadFloat32x8((*[8]float32)(x[0:8]))
		v1 := archsimd.LoadFloat32x8((*[8]float32)(x[8:16]))
		v2 := archsimd.LoadFloat32x8((*[8]float32)(x[16:24]))
		v3 := archsimd.LoadFloat32x8((*[8]float32)(x[24:32]))
		a0 = a0.Add(v0.Mul(v0)) // VMULPS + VADDPS: a0 += v0 * v0 on 8 lanes
		a1 = a1.Add(v1.Mul(v1))
		a2 = a2.Add(v2.Mul(v2))
		a3 = a3.Add(v3.Mul(v3))
		x = x[avxBlock:]
	}
	a0 = a0.Add(a1).Add(a2.Add(a3))
	s := a0.GetLo().Add(a0.GetHi()) // 8 lanes -> 4
	s = s.AddPairs(s)               // 4 -> 2
	s = s.AddPairs(s)               // 2 -> 1
	return s.GetElem(0)
}

// scaleAVX2 requires len(x) to be a multiple of 8.
//
//go:noinline
func scaleAVX2(x []float32, s float32) {
	sv := archsimd.BroadcastFloat32x8(s)
	for len(x) >= 8 {
		p := (*[8]float32)(x[0:8])
		archsimd.LoadFloat32x8(p).Mul(sv).Store(p) // VMULPS on 8 lanes
		x = x[8:]
	}
}
