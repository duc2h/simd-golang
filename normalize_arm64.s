//go:build arm64

#include "textflag.h"

// Register naming in Go's arm64 assembler:
//   V4.S4 = vector register V4 viewed as 4 lanes of 32-bit floats (128 bits)
//   F0    = lowest lane of V0 used as a scalar float
// Operand order is source(s) first, destination last.

// func sumSquaresNEON(x []float32) float32
// len(x) must be a multiple of 16.
TEXT ·sumSquaresNEON(SB), NOSPLIT, $0-28
	MOVD	x_base+0(FP), R0
	MOVD	x_len+8(FP), R1
	LSR	$4, R1, R1            // R1 = number of 16-float blocks

	// 4 accumulator registers x 4 lanes = 16 independent running sums.
	// Several accumulators let the CPU overlap FMAs instead of waiting for
	// the previous one to finish.
	VEOR	V0.B16, V0.B16, V0.B16
	VEOR	V1.B16, V1.B16, V1.B16
	VEOR	V2.B16, V2.B16, V2.B16
	VEOR	V3.B16, V3.B16, V3.B16
	CBZ	R1, reduce

loop:
	// Load 16 floats (64 bytes) into V4..V7 and advance R0 by 64.
	VLD1.P	64(R0), [V4.S4, V5.S4, V6.S4, V7.S4]
	// acc += x * x on 4 lanes per instruction (fused multiply-add).
	VFMLA	V4.S4, V4.S4, V0.S4
	VFMLA	V5.S4, V5.S4, V1.S4
	VFMLA	V6.S4, V6.S4, V2.S4
	VFMLA	V7.S4, V7.S4, V3.S4
	SUBS	$1, R1, R1
	BNE	loop

reduce:
	// Combine the 4 accumulators. Go's assembler has no vector FADD
	// mnemonic, so use acc += other * 1.0, which is exactly an add.
	MOVW	$0x3f800000, R2       // bit pattern of float32(1.0)
	VDUP	R2, V8.S4             // V8 = {1, 1, 1, 1}
	VFMLA	V1.S4, V8.S4, V0.S4   // V0 += V1
	VFMLA	V3.S4, V8.S4, V2.S4   // V2 += V3
	VFMLA	V2.S4, V8.S4, V0.S4   // V0 += V2

	// Horizontal sum of the 4 lanes of V0.
	VDUP	V0.S[1], V1           // F1 = lane 1
	VDUP	V0.S[2], V2           // F2 = lane 2
	VDUP	V0.S[3], V3           // F3 = lane 3
	FADDS	F1, F0                // F0 = lane0 + lane1
	FADDS	F3, F2                // F2 = lane2 + lane3
	FADDS	F2, F0
	FMOVS	F0, ret+24(FP)
	RET

// func scaleNEON(x []float32, s float32)
// Multiplies every element of x by s in place. len(x) must be a multiple of 16.
TEXT ·scaleNEON(SB), NOSPLIT, $0-28
	MOVD	x_base+0(FP), R0      // read pointer
	MOVD	x_len+8(FP), R1
	FMOVS	s+24(FP), F8
	VDUP	V8.S[0], V8.S4        // V8 = {s, s, s, s}
	// There is no vector FMUL mnemonic either, so compute -0 + x*s with an
	// FMA. Starting from -0 (not +0) keeps the sign of zero results, making
	// the output bit-identical to the scalar x*s.
	MOVW	$0x80000000, R2       // bit pattern of float32(-0.0)
	VDUP	R2, V9.S4             // V9 = {-0, -0, -0, -0}
	LSR	$4, R1, R1
	CBZ	R1, done
	MOVD	R0, R3                // write pointer

loop:
	VLD1.P	64(R0), [V0.S4, V1.S4, V2.S4, V3.S4]
	VMOV	V9.B16, V4.B16
	VMOV	V9.B16, V5.B16
	VMOV	V9.B16, V6.B16
	VMOV	V9.B16, V7.B16
	VFMLA	V0.S4, V8.S4, V4.S4   // V4 = x[0:4]   * s
	VFMLA	V1.S4, V8.S4, V5.S4   // V5 = x[4:8]   * s
	VFMLA	V2.S4, V8.S4, V6.S4   // V6 = x[8:12]  * s
	VFMLA	V3.S4, V8.S4, V7.S4   // V7 = x[12:16] * s
	VST1.P	[V4.S4, V5.S4, V6.S4, V7.S4], 64(R3)
	SUBS	$1, R1, R1
	BNE	loop

done:
	RET
