//go:build !arm64 && !(amd64 && goexperiment.simd)

package simdnorm

// Fallback for platforms without a SIMD implementation.

// Implementation describes the code path Normalize uses on this machine.
func Implementation() string { return "scalar (pure Go)" }

func accumulatorLanes() int { return 1 }

func sumSquares(v []float32) float32 { return sumSquaresScalar(v) }

func scale(v []float32, s float32) { scaleScalar(v, s) }
