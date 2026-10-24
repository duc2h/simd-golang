//go:build arm64

package simdnorm

// Implemented in normalize_arm64.s with NEON (128-bit, 4 x float32) instructions.
// Both process len(x) rounded down to a multiple of neonBlock; the Go wrappers
// below handle the remaining tail elements.

//go:noescape
func sumSquaresNEON(x []float32) float32

//go:noescape
func scaleNEON(x []float32, s float32)

// neonBlock is the number of floats handled per loop iteration:
// 4 registers x 4 lanes.
const neonBlock = 16

// Implementation describes the code path Normalize uses on this machine.
func Implementation() string { return "arm64 NEON (hand-written assembly)" }

// simdName is a short label for the SIMD code path, used in benchmark names.
func simdName() string { return "neon" }

// accumulatorLanes is the number of independent partial sums sumSquares keeps.
func accumulatorLanes() int { return neonBlock }

func sumSquares(v []float32) float32 {
	n := len(v) &^ (neonBlock - 1)
	var s float32
	if n > 0 {
		s = sumSquaresNEON(v[:n])
	}
	return s + sumSquaresScalar(v[n:])
}

func scale(v []float32, s float32) {
	n := len(v) &^ (neonBlock - 1)
	if n > 0 {
		scaleNEON(v[:n], s)
	}
	scaleScalar(v[n:], s)
}
